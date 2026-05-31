// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

const composeProjectLabel = "com.docker.compose.project"
const composeServiceLabel = "com.docker.compose.service"
const moveProgressTotal = 10
const moveImageLoadHeartbeat = 15 * time.Second
const moveAgentSpoolMinVersion = "1.3.1"

type moveProgressEmitter func(MoveContainerEvent)

// MovePlan returns a preview of the Docker resources needed to move a container.
func (s *Service) MovePlan(ctx context.Context, envID, id string, req MoveContainerPlanRequest) (MoveContainerPlan, error) {
	if strings.TrimSpace(req.TargetEnvID) == "" {
		return MoveContainerPlan{}, fmt.Errorf("target environment is required")
	}
	if req.TargetEnvID == envID {
		return MoveContainerPlan{}, fmt.Errorf("target environment must be different from source")
	}

	sourceCli, err := s.getClient(envID)
	if err != nil {
		return MoveContainerPlan{}, err
	}
	targetCli, err := s.getClient(req.TargetEnvID)
	if err != nil {
		return MoveContainerPlan{}, err
	}

	opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	info, err := sourceCli.ContainerInspect(opCtx, id)
	if err != nil {
		return MoveContainerPlan{}, fmt.Errorf("inspecting source container: %w", err)
	}

	plan, err := s.buildMovePlan(opCtx, envID, req.TargetEnvID, req.TargetName, req.NetworkMode, req.Networks, sourceCli, targetCli, info)
	if err != nil {
		return MoveContainerPlan{}, err
	}
	return plan, nil
}

// Move creates an equivalent container in another environment and optionally removes the source.
func (s *Service) Move(ctx context.Context, envID, id string, req MoveContainerRequest) (MoveContainerResult, error) {
	return s.move(ctx, envID, id, req, nil)
}

// MoveWithProgress creates an equivalent container while streaming progress events.
func (s *Service) MoveWithProgress(ctx context.Context, envID, id string, req MoveContainerRequest, events chan<- MoveContainerEvent) (MoveContainerResult, error) {
	defer close(events)

	emit := func(event MoveContainerEvent) {
		select {
		case events <- event:
		case <-ctx.Done():
		}
	}

	result, err := s.move(ctx, envID, id, req, emit)
	if err != nil {
		emitMoveProgress(emit, moveProgressTotal, "error", moveProgressErrorMessage(err), "error")
		return result, err
	}

	emit(MoveContainerEvent{
		Step:              moveProgressTotal,
		Total:             moveProgressTotal,
		Message:           "Move complete.",
		Status:            "done",
		Phase:             "done",
		TargetContainerID: result.TargetContainerID,
		TargetName:        result.TargetName,
	})
	return result, nil
}

func moveProgressErrorMessage(err error) string {
	if err != nil && strings.Contains(err.Error(), "streaming docker request bodies require") {
		return "The target agent must be updated to mcharbor-agent 1.3.1 or newer before moving images or volume data."
	}
	if err != nil && strings.Contains(err.Error(), "does not support staged image loading") {
		return err.Error()
	}
	return "Move failed. Check server logs for details."
}

func (s *Service) move(ctx context.Context, envID, id string, req MoveContainerRequest, emit moveProgressEmitter) (MoveContainerResult, error) {
	if strings.TrimSpace(req.TargetEnvID) == "" {
		return MoveContainerResult{}, fmt.Errorf("target environment is required")
	}
	if req.TargetEnvID == envID {
		return MoveContainerResult{}, fmt.Errorf("target environment must be different from source")
	}

	emitMoveProgress(emit, 1, "connect", "Connecting to Docker environments.", "progress")
	sourceCli, err := s.getClient(envID)
	if err != nil {
		return MoveContainerResult{}, err
	}
	targetCli, err := s.getClient(req.TargetEnvID)
	if err != nil {
		return MoveContainerResult{}, err
	}

	opCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := docker.EnsureContainerMutable(opCtx, sourceCli, id); err != nil {
		return MoveContainerResult{}, err
	}

	emitMoveProgress(emit, 2, "inspect", "Inspecting source container.", "progress")
	info, err := sourceCli.ContainerInspect(opCtx, id)
	if err != nil {
		return MoveContainerResult{}, fmt.Errorf("inspecting source container: %w", err)
	}

	emitMoveProgress(emit, 3, "plan", "Preparing target resources.", "progress")
	plan, err := s.buildMovePlan(opCtx, envID, req.TargetEnvID, req.TargetName, req.NetworkMode, req.Networks, sourceCli, targetCli, info)
	if err != nil {
		return MoveContainerResult{}, err
	}
	result := MoveContainerResult{
		TargetName: plan.TargetName,
		Warnings:   append([]string{}, plan.Warnings...),
	}

	if plan.Image.WillTransfer {
		if !req.TransferImage {
			return MoveContainerResult{}, fmt.Errorf("image %s is missing on target and transfer is disabled", plan.Image.Reference)
		}
		if s.pool.IsAgentEnv(req.TargetEnvID) && !s.pool.AgentAtLeast(req.TargetEnvID, moveAgentSpoolMinVersion) {
			version, _ := s.pool.AgentVersion(req.TargetEnvID)
			if version == "" {
				version = "unknown"
			}
			return MoveContainerResult{}, fmt.Errorf("target agent %s does not support staged image loading; update mcharbor-agent to %s or newer", version, moveAgentSpoolMinVersion)
		}
		if plan.Image.Size > 0 {
			emitMoveProgress(emit, 4, "image", "Image size is "+formatMoveBytes(plan.Image.Size)+". Large images can take several minutes.", "progress")
		} else {
			emitMoveProgress(emit, 4, "image", "Image size is unknown. Large images can take several minutes.", "progress")
		}
		emitMoveProgress(emit, 4, "image", "Transferring container image to target.", "progress")
		if err := transferImage(opCtx, sourceCli, targetCli, plan.Image.Reference, plan.Image.Size, emit); err != nil {
			return MoveContainerResult{}, err
		}
		result.ImageTransferred = true
	} else {
		emitMoveProgress(emit, 4, "image", "Image already exists on target.", "progress")
	}

	emitMoveProgress(emit, 5, "networks", "Preparing target networks.", "progress")
	for _, networkPlan := range plan.Networks {
		if networkPlan.Builtin || networkPlan.Exists {
			continue
		}
		if !req.CreateMissingNetworks {
			return MoveContainerResult{}, fmt.Errorf("network %s is missing on target and creation is disabled", networkPlan.Name)
		}
		if err := createTargetNetwork(opCtx, sourceCli, targetCli, networkPlan); err != nil {
			return MoveContainerResult{}, err
		}
		result.NetworksCreated = append(result.NetworksCreated, networkPlan.TargetName)
	}

	emitMoveProgress(emit, 6, "volumes", "Preparing target volumes.", "progress")
	for _, volumePlan := range plan.Volumes {
		if volumePlan.Type != "volume" || volumePlan.Exists {
			continue
		}
		if !req.CreateMissingVolumes {
			return MoveContainerResult{}, fmt.Errorf("volume %s is missing on target and creation is disabled", volumePlan.Name)
		}
		if err := createTargetVolume(opCtx, sourceCli, targetCli, volumePlan.Name); err != nil {
			return MoveContainerResult{}, err
		}
		result.VolumesCreated = append(result.VolumesCreated, volumePlan.Name)
	}

	if req.StopSource && info.State != nil && info.State.Running {
		emitMoveProgress(emit, 7, "stop-source", "Stopping source container.", "progress")
		timeout := 10
		if err := sourceCli.ContainerStop(opCtx, id, container.StopOptions{Timeout: &timeout}); err != nil {
			return MoveContainerResult{}, fmt.Errorf("stopping source container: %w", err)
		}
		result.SourceStopped = true
	} else {
		emitMoveProgress(emit, 7, "stop-source", "Source container stop skipped.", "progress")
	}

	cfg, hc, netConfig, err := replacementContainerSpec(info, RecreateRequest{}, plan.Image.Reference)
	if err != nil {
		return MoveContainerResult{}, err
	}
	applyMoveNetworkSettings(info, hc, netConfig, plan.NetworkMode, req.Networks)

	emitMoveProgress(emit, 8, "create-target", "Creating target container.", "progress")
	resp, err := targetCli.ContainerCreate(opCtx, cfg, hc, netConfig, nil, plan.TargetName)
	if err != nil {
		return MoveContainerResult{}, fmt.Errorf("creating target container: %w", err)
	}
	result.TargetContainerID = resp.ID

	if req.CopyNamedVolumes {
		emitMoveProgress(emit, 9, "copy-volumes", "Copying named volume data.", "progress")
		for _, volumePlan := range plan.Volumes {
			if volumePlan.Type != "volume" || volumePlan.Destination == "" {
				continue
			}
			if err := copyContainerPath(opCtx, sourceCli, targetCli, id, resp.ID, volumePlan.Destination); err != nil {
				return MoveContainerResult{}, fmt.Errorf("copying volume %s: %w", volumePlan.Name, err)
			}
			result.VolumesCopied = append(result.VolumesCopied, volumePlan.Name)
		}
	} else {
		emitMoveProgress(emit, 9, "copy-volumes", "Volume data copy skipped.", "progress")
	}

	emitMoveProgress(emit, 10, "finalize", "Finalizing target container.", "progress")
	if req.StartTarget {
		if err := targetCli.ContainerStart(opCtx, resp.ID, container.StartOptions{}); err != nil {
			return MoveContainerResult{}, fmt.Errorf("starting target container: %w", err)
		}
	}

	if req.RemoveSource {
		if err := sourceCli.ContainerRemove(opCtx, id, container.RemoveOptions{Force: true, RemoveVolumes: false}); err != nil {
			return MoveContainerResult{}, fmt.Errorf("removing source container: %w", err)
		}
		result.SourceRemoved = true
	}

	return result, nil
}

func emitMoveProgress(emit moveProgressEmitter, step int, phase, message, status string) {
	emitMoveProgressBytes(emit, step, phase, message, status, 0, 0)
}

func emitMoveProgressBytes(emit moveProgressEmitter, step int, phase, message, status string, bytesTransferred, bytesTotal int64) {
	if emit == nil {
		return
	}
	emit(MoveContainerEvent{
		Step:             step,
		Total:            moveProgressTotal,
		Message:          message,
		Status:           status,
		Phase:            phase,
		BytesTransferred: bytesTransferred,
		BytesTotal:       bytesTotal,
	})
}

func (s *Service) buildMovePlan(ctx context.Context, sourceEnvID, targetEnvID, targetName, networkMode string, networkConfigs []MoveNetworkConfig, sourceCli, targetCli *client.Client, info types.ContainerJSON) (MoveContainerPlan, error) {
	containerName := normalizedMoveName(info.Name, info.ID)
	targetName = normalizedMoveName(targetName, containerName)
	imageRef := moveImageReference(info)
	imageSize := sourceImageSize(ctx, sourceCli, imageRef, info.Image)

	imageExists, err := imageExists(ctx, targetCli, imageRef)
	if err != nil {
		return MoveContainerPlan{}, err
	}

	plan := MoveContainerPlan{
		SourceEnvID:   sourceEnvID,
		TargetEnvID:   targetEnvID,
		ContainerID:   info.ID,
		ContainerName: containerName,
		TargetName:    targetName,
		Image: MoveImagePlan{
			Reference:    imageRef,
			ID:           info.Image,
			Size:         imageSize,
			Exists:       imageExists,
			WillTransfer: !imageExists,
		},
		Volumes:     moveVolumePlans(ctx, targetCli, info),
		Networks:    moveNetworkPlans(ctx, sourceCli, targetCli, info, networkConfigs),
		NetworkMode: effectiveMoveNetworkMode(info, networkMode, networkConfigs),
		Ports:       movePortPlans(info),
		Warnings:    moveWarnings(info),
	}

	if info.Config != nil && info.Config.Labels != nil {
		plan.Stack.Name = info.Config.Labels[composeProjectLabel]
		plan.Stack.Service = info.Config.Labels[composeServiceLabel]
		plan.Stack.LabelsPreserve = plan.Stack.Name != ""
	}
	if plan.Stack.LabelsPreserve {
		plan.RequiredChanges = append(plan.RequiredChanges, "Compose stack labels will be preserved on the target container.")
		plan.Warnings = append(plan.Warnings, "Managed stack metadata is not moved; relink or import the stack definition after migration if needed.")
	}
	if plan.Image.WillTransfer {
		plan.RequiredChanges = append(plan.RequiredChanges, "Transfer the container image to the target environment.")
	}
	for _, vol := range plan.Volumes {
		if vol.WillCreate {
			plan.RequiredChanges = append(plan.RequiredChanges, "Create target volume "+vol.Name+".")
		}
		if vol.WillCopy {
			plan.RequiredChanges = append(plan.RequiredChanges, "Copy data for named volume "+vol.Name+".")
		}
		if vol.Manual {
			plan.RequiredChanges = append(plan.RequiredChanges, "Verify bind mount "+vol.Source+" exists on the target host.")
		}
	}
	for _, net := range plan.Networks {
		if net.WillCreate {
			plan.RequiredChanges = append(plan.RequiredChanges, "Create target network "+net.TargetName+" with driver "+net.Driver+".")
		}
		if net.TargetName != net.SourceName {
			plan.RequiredChanges = append(plan.RequiredChanges, "Attach source network "+net.SourceName+" as target network "+net.TargetName+".")
		}
	}
	if plan.NetworkMode != "" && info.HostConfig != nil && string(info.HostConfig.NetworkMode) != plan.NetworkMode {
		plan.RequiredChanges = append(plan.RequiredChanges, "Use network mode "+plan.NetworkMode+" on the target container.")
	}
	if len(plan.Ports) > 0 {
		plan.RequiredChanges = append(plan.RequiredChanges, "Reuse host port bindings on the target environment; confirm they are free before starting.")
	}
	if len(plan.RequiredChanges) == 0 {
		plan.RequiredChanges = append(plan.RequiredChanges, "Create an equivalent container on the target environment.")
	}

	return plan, nil
}

func normalizedMoveName(name, fallback string) string {
	name = strings.TrimSpace(strings.TrimPrefix(name, "/"))
	if name != "" {
		return name
	}
	return strings.TrimSpace(strings.TrimPrefix(fallback, "/"))
}

func moveImageReference(info types.ContainerJSON) string {
	if info.Config != nil && strings.TrimSpace(info.Config.Image) != "" {
		return info.Config.Image
	}
	return info.Image
}

func imageExists(ctx context.Context, cli *client.Client, ref string) (bool, error) {
	if ref == "" {
		return false, nil
	}
	if _, _, err := cli.ImageInspectWithRaw(ctx, ref); err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting target image %s: %w", ref, err)
	}
	return true, nil
}

func sourceImageSize(ctx context.Context, cli *client.Client, ref, id string) int64 {
	refs := []string{ref}
	if id != "" && id != ref {
		refs = append(refs, id)
	}
	for _, imageRef := range refs {
		if imageRef == "" {
			continue
		}
		info, _, err := cli.ImageInspectWithRaw(ctx, imageRef)
		if err == nil && info.Size > 0 {
			return info.Size
		}
	}
	return 0
}

func moveVolumePlans(ctx context.Context, targetCli *client.Client, info types.ContainerJSON) []MoveVolumePlan {
	plans := make([]MoveVolumePlan, 0, len(info.Mounts))
	for _, mount := range info.Mounts {
		plan := MoveVolumePlan{
			Type:        string(mount.Type),
			Name:        mount.Name,
			Source:      mount.Source,
			Destination: mount.Destination,
			Mode:        mount.Mode,
		}
		switch string(mount.Type) {
		case "volume":
			plan.Exists = targetVolumeExists(ctx, targetCli, mount.Name)
			plan.WillCreate = !plan.Exists
			plan.WillCopy = true
		case "bind":
			plan.Manual = true
		default:
			plan.Manual = true
		}
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Destination < plans[j].Destination
	})
	return plans
}

func targetVolumeExists(ctx context.Context, cli *client.Client, name string) bool {
	if name == "" {
		return false
	}
	if _, err := cli.VolumeInspect(ctx, name); err != nil {
		return false
	}
	return true
}

func moveNetworkPlans(ctx context.Context, sourceCli, targetCli *client.Client, info types.ContainerJSON, configs []MoveNetworkConfig) []MoveNetworkPlan {
	if info.NetworkSettings == nil || len(info.NetworkSettings.Networks) == 0 {
		return nil
	}
	configBySource := moveNetworkConfigBySource(configs)
	plans := make([]MoveNetworkPlan, 0, len(info.NetworkSettings.Networks))
	for name, endpoint := range info.NetworkSettings.Networks {
		cfg, hasCfg := configBySource[name]
		targetName := normalizedMoveName(cfg.TargetName, name)
		plan := MoveNetworkPlan{
			Name:             name,
			SourceName:       name,
			TargetName:       targetName,
			ID:               endpoint.NetworkID,
			Aliases:          endpoint.Aliases,
			TargetAliases:    nonNilStrings(cfg.Aliases, endpoint.Aliases),
			IPAddress:        endpoint.IPAddress,
			TargetIPAddress:  strings.TrimSpace(cfg.IPAddress),
			MacAddress:       endpoint.MacAddress,
			TargetMacAddress: strings.TrimSpace(cfg.MacAddress),
			Builtin:          isBuiltinNetwork(targetName),
			Internal:         cfg.Internal,
			Attachable:       cfg.Attachable,
			IPAM:             cfg.IPAM,
			Options:          cfg.Options,
			Labels:           cfg.Labels,
		}
		if sourceNet, err := sourceCli.NetworkInspect(ctx, name, networkTypes.InspectOptions{}); err == nil {
			plan.Driver = firstNonEmpty(cfg.Driver, sourceNet.Driver, "bridge")
			plan.Internal = sourceNet.Internal
			plan.Attachable = sourceNet.Attachable
			plan.Options = nonNilMap(cfg.Options, sourceNet.Options)
			plan.Labels = nonNilMap(cfg.Labels, sourceNet.Labels)
			if cfg.IPAM == nil {
				ipam := sourceNet.IPAM
				plan.IPAM = &ipam
			}
			if hasCfg {
				plan.Internal = cfg.Internal
				plan.Attachable = cfg.Attachable
			}
			plan.Builtin = plan.Builtin || isBuiltinNetwork(sourceNet.Name)
		} else {
			plan.Driver = firstNonEmpty(cfg.Driver, "bridge")
		}
		if plan.Builtin {
			plan.Exists = true
		} else if _, err := targetCli.NetworkInspect(ctx, targetName, networkTypes.InspectOptions{}); err == nil {
			plan.Exists = true
		}
		plan.WillCreate = !plan.Exists && !plan.Builtin
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Name < plans[j].Name
	})
	return plans
}

func moveNetworkConfigBySource(configs []MoveNetworkConfig) map[string]MoveNetworkConfig {
	result := make(map[string]MoveNetworkConfig, len(configs))
	for _, cfg := range configs {
		sourceName := strings.TrimSpace(cfg.SourceName)
		if sourceName == "" {
			sourceName = strings.TrimSpace(cfg.TargetName)
		}
		if sourceName != "" {
			result[sourceName] = cfg
		}
	}
	return result
}

func nonNilStrings(primary, fallback []string) []string {
	if primary != nil {
		return primary
	}
	return fallback
}

func nonNilMap(primary, fallback map[string]string) map[string]string {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isBuiltinNetwork(name string) bool {
	switch name {
	case "bridge", "host", "none":
		return true
	default:
		return false
	}
}

func movePortPlans(info types.ContainerJSON) []MovePortPlan {
	if info.NetworkSettings == nil || len(info.NetworkSettings.Ports) == 0 {
		return nil
	}
	ports := make([]MovePortPlan, 0)
	for port, bindings := range info.NetworkSettings.Ports {
		if len(bindings) == 0 {
			ports = append(ports, MovePortPlan{ContainerPort: string(port)})
			continue
		}
		for _, binding := range bindings {
			ports = append(ports, MovePortPlan{
				ContainerPort: string(port),
				HostIP:        binding.HostIP,
				HostPort:      binding.HostPort,
			})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].ContainerPort < ports[j].ContainerPort
	})
	return ports
}

func moveWarnings(info types.ContainerJSON) []string {
	warnings := make([]string, 0)
	if info.State != nil && info.State.Running {
		warnings = append(warnings, "The source container is running; stop it during the move for consistent volume data.")
	}
	if info.HostConfig != nil && info.HostConfig.NetworkMode.IsContainer() {
		warnings = append(warnings, "Container network mode references another container and may need manual adjustment on the target.")
	}
	for _, mount := range info.Mounts {
		if string(mount.Type) == "bind" {
			warnings = append(warnings, "Bind mount "+mount.Source+" is host-specific and must exist on the target host.")
		}
	}
	return warnings
}

type moveProgressReader struct {
	reader    io.Reader
	emit      moveProgressEmitter
	total     int64
	formatter func(transferred, total int64) string
	bytes     atomic.Int64
	lastEmit  time.Time
}

func (r *moveProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		total := r.bytes.Add(int64(n))
		if r.lastEmit.IsZero() || time.Since(r.lastEmit) >= 3*time.Second {
			r.lastEmit = time.Now()
			displayTotal := moveProgressTotalBytes(total, r.total)
			message := formatMoveTransferProgress(total, displayTotal)
			if r.formatter != nil {
				message = r.formatter(total, displayTotal)
			}
			emitMoveProgressBytes(r.emit, 4, "image", message, "progress", total, displayTotal)
		}
	}
	return n, err
}

func transferImage(ctx context.Context, sourceCli, targetCli *client.Client, ref string, imageSize int64, emit moveProgressEmitter) error {
	emitMoveProgress(emit, 4, "image", "Opening source image archive.", "progress")
	reader, err := sourceCli.ImageSave(ctx, []string{ref})
	if err != nil {
		return fmt.Errorf("exporting image %s: %w", ref, err)
	}
	defer reader.Close()

	archiveFile, err := os.CreateTemp("", "mcharbor-image-move-*.tar")
	if err != nil {
		return fmt.Errorf("creating temporary image archive: %w", err)
	}
	archivePath := archiveFile.Name()
	defer func() {
		if err := archiveFile.Close(); err != nil {
			slog.Warn("containers: close temporary image archive failed", "error", err, "path", archivePath)
		}
		if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
			slog.Warn("containers: remove temporary image archive failed", "error", err, "path", archivePath)
		}
	}()

	sourceProgress := &moveProgressReader{
		reader:    reader,
		emit:      emit,
		total:     imageSize,
		formatter: formatMoveTransferProgress,
	}
	if _, err := io.Copy(archiveFile, sourceProgress); err != nil {
		return fmt.Errorf("saving image archive %s: %w", ref, err)
	}
	archiveSize := sourceProgress.bytes.Load()
	if archiveSize > 0 {
		emitMoveProgressBytes(emit, 4, "image", formatMoveTransferProgress(archiveSize, archiveSize), "progress", archiveSize, archiveSize)
	}
	emitMoveProgress(emit, 4, "image", "Source image archive transfer finished; loading target image.", "progress")

	loadDone := make(chan struct{})
	if _, err := archiveFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding image archive %s: %w", ref, err)
	}
	targetProgress := &moveProgressReader{
		reader:    archiveFile,
		emit:      emit,
		total:     archiveSize,
		formatter: formatMoveTargetLoadProgress,
	}
	go emitImageLoadHeartbeat(ctx, loadDone, targetProgress, archiveSize, emit)

	resp, err := targetCli.ImageLoad(ctx, targetProgress, client.ImageLoadWithQuiet(true))
	close(loadDone)
	if err != nil {
		return fmt.Errorf("loading image %s: %w", ref, err)
	}
	defer resp.Body.Close()
	transferred := targetProgress.bytes.Load()
	if transferred > 0 {
		displayTotal := moveProgressTotalBytes(transferred, archiveSize)
		emitMoveProgressBytes(emit, 4, "image", formatMoveTargetLoadProgress(transferred, displayTotal), "progress", transferred, displayTotal)
	}
	emitMoveProgress(emit, 4, "image", "Target Docker accepted the image archive; reading load result.", "progress")
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("reading image load response: %w", err)
	}
	emitMoveProgress(emit, 4, "image", "Image transfer finished.", "progress")
	return nil
}

func emitImageLoadHeartbeat(ctx context.Context, done <-chan struct{}, progressReader *moveProgressReader, imageSize int64, emit moveProgressEmitter) {
	if emit == nil {
		return
	}
	ticker := time.NewTicker(moveImageLoadHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			transferred := progressReader.bytes.Load()
			if transferred <= 0 {
				continue
			}
			displayTotal := moveProgressTotalBytes(transferred, imageSize)
			emitMoveProgressBytes(emit, 4, "image", moveImageLoadHeartbeatMessage(transferred, displayTotal), "progress", transferred, displayTotal)
		}
	}
}

func moveImageLoadHeartbeatMessage(transferred, total int64) string {
	if total > 0 && transferred >= total {
		return "Target Docker is loading the image archive. This can take several minutes for large images."
	}
	return "Target Docker is receiving the staged image archive."
}

func formatMoveBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}

func formatMoveTransferProgress(transferred, total int64) string {
	if total > 0 {
		return "Transferred " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " from source image archive."
	}
	return "Transferred " + formatMoveBytes(transferred) + " from source image archive."
}

func formatMoveTargetLoadProgress(transferred, total int64) string {
	if total > 0 {
		return "Sent " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " to target Docker."
	}
	return "Sent " + formatMoveBytes(transferred) + " to target Docker."
}

func moveProgressTotalBytes(transferred, total int64) int64 {
	if total > 0 && transferred > total {
		return transferred
	}
	return total
}

func createTargetNetwork(ctx context.Context, sourceCli, targetCli *client.Client, plan MoveNetworkPlan) error {
	sourceNet, err := sourceCli.NetworkInspect(ctx, plan.SourceName, networkTypes.InspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting source network %s: %w", plan.SourceName, err)
	}
	ipam := sourceNet.IPAM
	if plan.IPAM != nil {
		ipam = *plan.IPAM
	}
	opts := networkTypes.CreateOptions{
		Driver:     firstNonEmpty(plan.Driver, sourceNet.Driver),
		Internal:   plan.Internal,
		Attachable: plan.Attachable,
		Options:    nonNilMap(plan.Options, sourceNet.Options),
		Labels:     nonNilMap(plan.Labels, sourceNet.Labels),
		IPAM:       &ipam,
	}
	if _, err := targetCli.NetworkCreate(ctx, plan.TargetName, opts); err != nil {
		return fmt.Errorf("creating target network %s: %w", plan.TargetName, err)
	}
	return nil
}

func effectiveMoveNetworkMode(info types.ContainerJSON, requested string, configs []MoveNetworkConfig) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	if info.HostConfig == nil {
		return ""
	}
	mode := string(info.HostConfig.NetworkMode)
	configBySource := moveNetworkConfigBySource(configs)
	if cfg, ok := configBySource[mode]; ok {
		return normalizedMoveName(cfg.TargetName, mode)
	}
	return mode
}

func applyMoveNetworkSettings(info types.ContainerJSON, hc *container.HostConfig, netConfig *networkTypes.NetworkingConfig, networkMode string, configs []MoveNetworkConfig) {
	if hc != nil && strings.TrimSpace(networkMode) != "" {
		hc.NetworkMode = container.NetworkMode(strings.TrimSpace(networkMode))
	}
	if netConfig == nil || len(netConfig.EndpointsConfig) == 0 {
		return
	}
	configBySource := moveNetworkConfigBySource(configs)
	endpoints := make(map[string]*networkTypes.EndpointSettings, len(netConfig.EndpointsConfig))
	for sourceName, endpoint := range netConfig.EndpointsConfig {
		cfg, ok := configBySource[sourceName]
		targetName := sourceName
		if ok {
			targetName = normalizedMoveName(cfg.TargetName, sourceName)
			if cfg.Aliases != nil {
				endpoint.Aliases = cfg.Aliases
			}
			if strings.TrimSpace(cfg.MacAddress) != "" {
				endpoint.MacAddress = strings.TrimSpace(cfg.MacAddress)
			}
			if strings.TrimSpace(cfg.IPAddress) != "" {
				if endpoint.IPAMConfig == nil {
					endpoint.IPAMConfig = &networkTypes.EndpointIPAMConfig{}
				}
				endpoint.IPAMConfig.IPv4Address = strings.TrimSpace(cfg.IPAddress)
			}
		}
		endpoints[targetName] = endpoint
	}
	netConfig.EndpointsConfig = endpoints
	_ = info
}

func createTargetVolume(ctx context.Context, sourceCli, targetCli *client.Client, name string) error {
	sourceVolume, err := sourceCli.VolumeInspect(ctx, name)
	if err != nil {
		return fmt.Errorf("inspecting source volume %s: %w", name, err)
	}
	_, err = targetCli.VolumeCreate(ctx, volume.CreateOptions{
		Name:       sourceVolume.Name,
		Driver:     sourceVolume.Driver,
		DriverOpts: sourceVolume.Options,
		Labels:     sourceVolume.Labels,
	})
	if err != nil {
		return fmt.Errorf("creating target volume %s: %w", name, err)
	}
	return nil
}

func copyContainerPath(ctx context.Context, sourceCli, targetCli *client.Client, sourceID, targetID, containerPath string) error {
	reader, _, err := sourceCli.CopyFromContainer(ctx, sourceID, containerPath)
	if err != nil {
		return fmt.Errorf("copying from source path %s: %w", containerPath, err)
	}
	defer reader.Close()

	destinationParent := path.Dir(containerPath)
	if destinationParent == "." {
		destinationParent = "/"
	}
	if err := targetCli.CopyToContainer(ctx, targetID, destinationParent, reader, container.CopyToContainerOptions{AllowOverwriteDirWithFile: false}); err != nil {
		return fmt.Errorf("copying to target path %s: %w", containerPath, err)
	}
	return nil
}
