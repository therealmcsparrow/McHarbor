// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	networkTypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

const composeProjectLabel = "com.docker.compose.project"
const composeServiceLabel = "com.docker.compose.service"
const moveProgressTotal = 10
const moveImageLoadHeartbeat = 15 * time.Second
const moveImageLoadTimeout = 30 * time.Minute
const moveOperationTimeout = 2 * time.Hour
const moveAgentDirectTransferMinVersion = "1.3.5"
const moveAgentPullTransferMinVersion = "1.5.1"

type moveProgressEmitter func(MoveContainerEvent)

type moveTransferRoute string

const (
	moveTransferRouteDefault        moveTransferRoute = ""
	moveTransferRouteAgentHostAgent moveTransferRoute = "agent-host-agent"
)

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

	plan, err := s.buildMovePlan(opCtx, envID, req.TargetEnvID, req.TargetName, req.NetworkMode, req.Networks, req.Volumes, sourceCli, targetCli, info)
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
		return "The target agent must be updated to mcharbor-agent 1.3.3 or newer before moving container data."
	}
	if err != nil && strings.Contains(err.Error(), "does not support staged image loading") {
		return "The target agent does not support staged image loading. Update the target agent to a newer version."
	}
	if err != nil && strings.Contains(err.Error(), "image load timed out") {
		return "The target Docker daemon took too long to load the image archive. Check target disk space and Docker daemon health, then retry."
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

	opCtx, cancel := context.WithTimeout(ctx, moveOperationTimeout)
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
	plan, err := s.buildMovePlan(opCtx, envID, req.TargetEnvID, req.TargetName, req.NetworkMode, req.Networks, req.Volumes, sourceCli, targetCli, info)
	if err != nil {
		return MoveContainerResult{}, err
	}
	result := MoveContainerResult{
		TargetName: plan.TargetName,
		Warnings:   append([]string{}, plan.Warnings...),
	}

	if req.StopSource && info.State != nil && info.State.Running {
		emitMoveProgress(emit, 4, "stop-source", "Stopping source container before snapshot.", "progress")
		timeout := 10
		if err := sourceCli.ContainerStop(opCtx, id, container.StopOptions{Timeout: &timeout}); err != nil {
			return MoveContainerResult{}, fmt.Errorf("stopping source container: %w", err)
		}
		result.SourceStopped = true
		info, err = sourceCli.ContainerInspect(opCtx, id)
		if err != nil {
			return MoveContainerResult{}, fmt.Errorf("inspecting stopped source container: %w", err)
		}
	} else {
		emitMoveProgress(emit, 4, "stop-source", "Source container stop skipped; snapshot will pause the container briefly.", "progress")
	}

	if !req.TransferImage {
		return MoveContainerResult{}, fmt.Errorf("container filesystem data requires image transfer")
	}
	if s.pool.IsAgentEnv(req.TargetEnvID) && !s.pool.AgentAtLeast(req.TargetEnvID, moveAgentPullTransferMinVersion) {
		version, _ := s.pool.AgentVersion(req.TargetEnvID)
		if version == "" {
			version = "unknown"
		}
		return MoveContainerResult{}, fmt.Errorf("target agent %s does not support pull-based move transfers; update mcharbor-agent to %s or newer", version, moveAgentPullTransferMinVersion)
	}

	emitMoveProgress(emit, 4, "image", "Creating source container filesystem snapshot.", "progress")
	snapshotRef, snapshotSize, cleanupSnapshot, err := createMoveSnapshotImage(opCtx, sourceCli, info, plan.TargetName)
	if err != nil {
		return MoveContainerResult{}, err
	}
	defer cleanupSnapshot()

	if snapshotSize > 0 {
		emitMoveProgress(emit, 4, "image", "Snapshot image size is "+formatMoveBytes(snapshotSize)+". Large images can take several minutes.", "progress")
	} else if plan.Image.Size > 0 {
		emitMoveProgress(emit, 4, "image", "Base image size is "+formatMoveBytes(plan.Image.Size)+". Snapshot transfer can take several minutes.", "progress")
	} else {
		emitMoveProgress(emit, 4, "image", "Snapshot image size is unknown. Large images can take several minutes.", "progress")
	}
	emitMoveProgress(emit, 4, "image", "Transferring container filesystem snapshot to target.", "progress")
	if err := s.transferImage(opCtx, envID, req.TargetEnvID, sourceCli, targetCli, snapshotRef, firstPositive(snapshotSize, plan.Image.Size), emit); err != nil {
		return MoveContainerResult{}, err
	}
	result.ImageTransferred = true

	originalImageRef := moveImageReference(info)
	if originalImageRef != "" && originalImageRef != snapshotRef {
		emitMoveProgress(emit, 4, "image", "Tagging snapshot with original image reference.", "progress")
		if err := targetCli.ImageTag(opCtx, snapshotRef, originalImageRef); err != nil {
			slog.Warn("containers: failed to tag transferred snapshot with original image reference", "source", snapshotRef, "target", originalImageRef, "error", err)
		}
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
		if err := createTargetVolume(opCtx, sourceCli, targetCli, volumePlan.Name, volumePlan.TargetName); err != nil {
			return MoveContainerResult{}, err
		}
		result.VolumesCreated = append(result.VolumesCreated, volumePlan.TargetName)
	}
	if bindSources := moveBindMountSources(plan.Volumes); len(bindSources) > 0 {
		emitMoveProgress(emit, 6, "volumes", "Preparing target bind mount paths.", "progress")
		if err := docker.EnsureBindMountPaths(opCtx, targetCli, bindSources); err != nil {
			return MoveContainerResult{}, fmt.Errorf("preparing target bind mount paths: %w", err)
		}
	}

	creationImageRef := originalImageRef
	if creationImageRef == "" {
		creationImageRef = snapshotRef
	}
	cfg, hc, netConfig, err := replacementContainerSpec(info, RecreateRequest{}, creationImageRef)
	if err != nil {
		return MoveContainerResult{}, err
	}
	applyMoveVolumeSettings(info, hc, plan.Volumes)
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
			if volumePlan.Type != "volume" || volumePlan.Destination == "" || volumePlan.TargetDestination == "" {
				continue
			}
			if err := s.copyContainerPath(opCtx, envID, req.TargetEnvID, sourceCli, targetCli, id, resp.ID, volumePlan.Destination, volumePlan.TargetDestination, volumePlan.Name, emit); err != nil {
				return MoveContainerResult{}, fmt.Errorf("copying volume %s: %w", volumePlan.Name, err)
			}
			result.VolumesCopied = append(result.VolumesCopied, volumePlan.TargetName)
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

func (s *Service) buildMovePlan(ctx context.Context, sourceEnvID, targetEnvID, targetName, networkMode string, networkConfigs []MoveNetworkConfig, volumeConfigs []MoveVolumeConfig, sourceCli, targetCli *client.Client, info types.ContainerJSON) (MoveContainerPlan, error) {
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
		Volumes:     moveVolumePlans(ctx, targetCli, info, volumeConfigs),
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
	plan.RequiredChanges = append(plan.RequiredChanges, "Snapshot the source container filesystem so writable-layer data is preserved on the target.")
	for _, vol := range plan.Volumes {
		if vol.WillCreate {
			plan.RequiredChanges = append(plan.RequiredChanges, "Create target volume "+vol.TargetName+".")
		}
		if vol.WillCopy {
			plan.RequiredChanges = append(plan.RequiredChanges, "Copy data for named volume "+vol.Name+" to "+vol.TargetName+" at "+vol.TargetDestination+".")
		}
		if vol.Type == "volume" && (vol.TargetName != "" && vol.TargetName != vol.Name || vol.TargetDestination != "" && vol.TargetDestination != vol.Destination) {
			plan.RequiredChanges = append(plan.RequiredChanges, "Mount source volume "+vol.Name+" as "+vol.TargetName+" at "+vol.TargetDestination+".")
		}
		if vol.Type == "bind" {
			plan.RequiredChanges = append(plan.RequiredChanges, "Create target bind mount path "+firstNonEmpty(vol.TargetSource, vol.Source)+" if it is missing.")
		} else if vol.Manual {
			plan.RequiredChanges = append(plan.RequiredChanges, "Verify mount "+firstNonEmpty(vol.TargetSource, vol.Source)+" exists on the target host.")
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

func createMoveSnapshotImage(ctx context.Context, cli *client.Client, info types.ContainerJSON, targetName string) (string, int64, func(), error) {
	if info.Config == nil {
		return "", 0, func() {}, fmt.Errorf("inspected container has no config")
	}

	ref := moveSnapshotImageRef(info.ID, targetName)
	cfg := *info.Config
	commit, err := cli.ContainerCommit(ctx, info.ID, container.CommitOptions{
		Reference: ref,
		Comment:   "McHarbor container move filesystem snapshot",
		Author:    "McHarbor",
		Pause:     info.State != nil && info.State.Running,
		Config:    &cfg,
	})
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if _, err := cli.ImageRemove(cleanupCtx, ref, image.RemoveOptions{Force: true, PruneChildren: true}); err != nil && !client.IsErrNotFound(err) {
			slog.Warn("containers: remove temporary move snapshot image failed", "image", ref, "error", err)
		}
	}
	if err != nil {
		return "", 0, func() {}, fmt.Errorf("creating source container filesystem snapshot: %w", err)
	}

	size := sourceImageSize(ctx, cli, ref, commit.ID)
	return ref, size, cleanup, nil
}

func moveSnapshotImageRef(containerID, targetName string) string {
	name := strings.ToLower(normalizedMoveName(targetName, "container"))
	var builder strings.Builder
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('-')
	}
	name = strings.Trim(builder.String(), ".-_")
	if name == "" {
		name = "container"
	}
	shortID := strings.ToLower(strings.TrimSpace(containerID))
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	if shortID == "" {
		shortID = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "mcharbor/move-" + name + ":" + shortID + "-" + strconv.FormatInt(time.Now().Unix(), 36)
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (s *Service) transferImage(ctx context.Context, sourceEnvID, targetEnvID string, sourceCli, targetCli *client.Client, ref string, imageSize int64, emit moveProgressEmitter) error {
	directStarted, err := s.tryDirectAgentImageTransfer(ctx, sourceEnvID, targetEnvID, ref, imageSize, emit)
	if err == nil && directStarted {
		return nil
	}
	if err != nil {
		if !directStarted || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		slog.Warn("direct agent image transfer failed; falling back to host relay", "source_env", sourceEnvID, "target_env", targetEnvID, "error", err)
		emitMoveProgress(emit, 4, "image", "Direct agent-to-agent transfer failed; using target agent pull route.", "progress")
	}
	pullStarted, pullErr := s.tryTargetAgentImagePullTransfer(ctx, sourceEnvID, targetEnvID, ref, imageSize, emit)
	if pullErr == nil && pullStarted {
		return nil
	}
	if pullErr != nil {
		if pullStarted {
			return pullErr
		}
		slog.Warn("target agent pull image transfer unavailable; falling back to host relay", "source_env", sourceEnvID, "target_env", targetEnvID, "error", pullErr)
	}
	route := moveTransferRouteDefault
	if s.pool.IsAgentEnv(sourceEnvID) && s.pool.IsAgentEnv(targetEnvID) {
		route = moveTransferRouteAgentHostAgent
		if !directStarted {
			emitMoveProgress(emit, 4, "image", "Direct agent-to-agent transfer unavailable; using McHarbor host relay (agent-host-agent route).", "progress")
		}
	} else {
		emitMoveProgress(emit, 4, "image", "Using McHarbor relay for snapshot transfer.", "progress")
	}
	return transferImage(ctx, sourceCli, targetCli, ref, imageSize, emit, route)
}

func (s *Service) tryTargetAgentImagePullTransfer(ctx context.Context, sourceEnvID, targetEnvID, ref string, imageSize int64, emit moveProgressEmitter) (bool, error) {
	if !s.pool.IsAgentEnv(targetEnvID) {
		return false, nil
	}
	if !s.pool.AgentAtLeast(targetEnvID, moveAgentPullTransferMinVersion) {
		version, _ := s.pool.AgentVersion(targetEnvID)
		if version == "" {
			version = "unknown"
		}
		return false, fmt.Errorf("target agent %s does not support pull-based move transfers; update mcharbor-agent to %s or newer", version, moveAgentPullTransferMinVersion)
	}
	targetConn, ok := s.pool.AgentConnection(targetEnvID)
	if !ok || targetConn.Transport == nil {
		return false, nil
	}
	entry, err := moveTransfers.create(moveTransferEntry{
		Kind:        moveTransferKindImage,
		SourceEnvID: sourceEnvID,
		ImageRef:    ref,
	})
	if err != nil {
		return false, err
	}
	transferURL := "/api/containers/internal/move-transfers/" + entry.ID
	started := true
	completed := false
	defer func() {
		if !completed {
			moveTransfers.cancel(entry.ID)
			targetConn.Transport.CancelTransfer(entry.ID)
		}
	}()

	var lastBytes atomic.Int64
	emitMoveProgress(emit, 4, "image", "Target agent is pulling the snapshot image from McHarbor.", "progress")
	err = targetConn.Transport.StartImagePullTransfer(ctx, entry.ID, transferURL, entry.Token, func(transferred int64, stage string) {
		if transferred <= 0 {
			return
		}
		lastBytes.Store(transferred)
		displayTotal := moveProgressTotalBytes(transferred, imageSize)
		message := formatMoveTargetAgentPullProgress(transferred, displayTotal)
		if strings.EqualFold(stage, "apply") {
			message = "Target Docker is loading the staged snapshot image."
		}
		emitMoveProgressBytes(emit, 4, "image", message, "progress", transferred, displayTotal)
	})
	if err != nil {
		return started, err
	}
	completed = true
	transferred := lastBytes.Load()
	if transferred > 0 {
		displayTotal := moveProgressTotalBytes(transferred, imageSize)
		emitMoveProgressBytes(emit, 4, "image", formatMoveTargetAgentPullProgress(transferred, displayTotal), "progress", transferred, displayTotal)
	}
	emitMoveProgress(emit, 4, "image", "Target agent finished loading the snapshot image.", "progress")
	return started, nil
}

func (s *Service) tryDirectAgentImageTransfer(ctx context.Context, sourceEnvID, targetEnvID, ref string, imageSize int64, emit moveProgressEmitter) (bool, error) {
	if sourceEnvID == targetEnvID {
		return false, nil
	}
	if !s.pool.IsAgentEnv(sourceEnvID) || !s.pool.IsAgentEnv(targetEnvID) {
		return false, nil
	}
	if !s.pool.AgentAtLeast(sourceEnvID, moveAgentDirectTransferMinVersion) || !s.pool.AgentAtLeast(targetEnvID, moveAgentDirectTransferMinVersion) {
		return false, nil
	}

	sourceConn, ok := s.pool.AgentConnection(sourceEnvID)
	if !ok || sourceConn.Transport == nil {
		return false, nil
	}
	targetConn, ok := s.pool.AgentConnection(targetEnvID)
	if !ok || targetConn.Transport == nil || strings.TrimSpace(targetConn.TransferURL) == "" {
		return false, nil
	}

	token, err := newMoveTransferToken()
	if err != nil {
		return false, fmt.Errorf("creating direct transfer token: %w", err)
	}
	transferID, err := newMoveTransferID()
	if err != nil {
		return false, fmt.Errorf("creating direct transfer id: %w", err)
	}

	emitMoveProgress(emit, 4, "image", "Direct agent-to-agent transfer available; preparing target receiver (agent-to-agent route).", "progress")
	prepareCtx, prepareCancel := context.WithTimeout(ctx, 30*time.Second)
	uploadURL, err := targetConn.Transport.PrepareTransfer(prepareCtx, transferID, token)
	prepareCancel()
	if err != nil {
		targetConn.Transport.CancelTransfer(transferID)
		return false, nil
	}

	started := true
	completed := false
	defer func() {
		if !completed {
			sourceConn.Transport.CancelTransfer(transferID)
			targetConn.Transport.CancelTransfer(transferID)
		}
	}()

	var lastBytes atomic.Int64
	emitMoveProgress(emit, 4, "image", "Sending snapshot directly from source agent to target agent (agent-to-agent route).", "progress")
	err = sourceConn.Transport.StartImageTransfer(ctx, transferID, ref, uploadURL, token, func(transferred int64) {
		if transferred <= 0 {
			return
		}
		lastBytes.Store(transferred)
		displayTotal := moveProgressTotalBytes(transferred, imageSize)
		emitMoveProgressBytes(emit, 4, "image", formatMoveDirectTransferProgress(transferred, displayTotal), "progress", transferred, displayTotal)
	})
	if err != nil {
		return started, err
	}

	completed = true
	transferred := lastBytes.Load()
	if transferred > 0 {
		displayTotal := moveProgressTotalBytes(transferred, imageSize)
		emitMoveProgressBytes(emit, 4, "image", formatMoveDirectTransferProgress(transferred, displayTotal), "progress", transferred, displayTotal)
	}
	emitMoveProgress(emit, 4, "image", "Direct agent-to-agent image transfer finished.", "progress")
	return started, nil
}

func newMoveTransferToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func newMoveTransferID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "move-" + hex.EncodeToString(buf[:]), nil
}

func moveVolumePlans(ctx context.Context, targetCli *client.Client, info types.ContainerJSON, configs []MoveVolumeConfig) []MoveVolumePlan {
	configBySource := moveVolumeConfigBySource(configs)
	plans := make([]MoveVolumePlan, 0, len(info.Mounts))
	for _, mount := range info.Mounts {
		mountType := moveMountType(mount)
		cfg := configBySource[moveVolumeConfigKey(mount.Name, mount.Destination)]
		targetName := strings.TrimSpace(firstNonEmpty(cfg.TargetName, mount.Name, dockerVolumeNameFromPath(mount.Source)))
		targetSource := strings.TrimSpace(firstNonEmpty(cfg.TargetSource, mount.Source))
		targetDestination := cleanContainerPath(firstNonEmpty(cfg.TargetDestination, mount.Destination))
		plan := MoveVolumePlan{
			Type:              mountType,
			Name:              mount.Name,
			TargetName:        targetName,
			Source:            mount.Source,
			TargetSource:      targetSource,
			Destination:       mount.Destination,
			TargetDestination: targetDestination,
			Mode:              mount.Mode,
		}
		switch mountType {
		case "volume":
			plan.Exists = targetVolumeExists(ctx, targetCli, targetName)
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

func moveMountType(mountPoint types.MountPoint) string {
	if mountPoint.Type == mount.TypeVolume || strings.TrimSpace(mountPoint.Name) != "" {
		return "volume"
	}
	if mountPoint.Type == mount.TypeBind {
		return "bind"
	}
	if name := dockerVolumeNameFromPath(mountPoint.Source); name != "" {
		return "volume"
	}
	return string(mountPoint.Type)
}

func dockerVolumeNameFromPath(source string) string {
	parts := strings.Split(path.Clean(strings.TrimSpace(source)), "/")
	for i := 0; i < len(parts)-3; i++ {
		if parts[i] == "docker" && parts[i+1] == "volumes" && parts[i+3] == "_data" && parts[i+2] != "" {
			return parts[i+2]
		}
	}
	return ""
}

func moveVolumeConfigBySource(configs []MoveVolumeConfig) map[string]MoveVolumeConfig {
	result := make(map[string]MoveVolumeConfig, len(configs))
	for _, cfg := range configs {
		key := moveVolumeConfigKey(cfg.SourceName, cfg.SourceDestination)
		if key == "" {
			continue
		}
		result[key] = cfg
	}
	return result
}

func moveVolumeConfigKey(name, destination string) string {
	name = strings.TrimSpace(name)
	destination = strings.TrimSpace(destination)
	if name != "" {
		return "name:" + name
	}
	if destination != "" {
		return "destination:" + path.Clean(destination)
	}
	return ""
}

func moveBindMountSources(plans []MoveVolumePlan) []string {
	sources := make([]string, 0, len(plans))
	for _, plan := range plans {
		if plan.Type != "bind" || strings.TrimSpace(plan.TargetSource) == "" {
			continue
		}
		sources = append(sources, plan.TargetSource)
	}
	return sources
}

func cleanContainerPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
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

func transferImage(ctx context.Context, sourceCli, targetCli *client.Client, ref string, imageSize int64, emit moveProgressEmitter, route moveTransferRoute) error {
	emitMoveProgress(emit, 4, "image", moveRelayOpeningMessage(route), "progress")
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
		formatter: moveRelaySourceProgressFormatter(route),
	}
	if _, err := io.Copy(archiveFile, sourceProgress); err != nil {
		return fmt.Errorf("saving image archive %s: %w", ref, err)
	}
	archiveSize := sourceProgress.bytes.Load()
	if archiveSize > 0 {
		emitMoveProgressBytes(emit, 4, "image", moveRelaySourceProgressFormatter(route)(archiveSize, archiveSize), "progress", archiveSize, archiveSize)
	}
	emitMoveProgress(emit, 4, "image", moveRelayLoadStartMessage(route), "progress")

	loadDone := make(chan struct{})
	defer close(loadDone)
	if _, err := archiveFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding image archive %s: %w", ref, err)
	}
	targetProgress := &moveProgressReader{
		reader:    archiveFile,
		emit:      emit,
		total:     archiveSize,
		formatter: moveRelayTargetProgressFormatter(route),
	}
	go emitImageLoadHeartbeat(ctx, loadDone, targetProgress, archiveSize, emit, route)

	loadCtx, loadCancel := context.WithTimeout(ctx, moveImageLoadTimeout)
	defer loadCancel()

	resp, err := targetCli.ImageLoad(loadCtx, targetProgress, client.ImageLoadWithQuiet(true))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("image load timed out after %s: %w", moveImageLoadTimeout, err)
		}
		return fmt.Errorf("loading image %s: %w", ref, err)
	}
	defer resp.Body.Close()
	transferred := targetProgress.bytes.Load()
	if transferred > 0 {
		displayTotal := moveProgressTotalBytes(transferred, archiveSize)
		emitMoveProgressBytes(emit, 4, "image", moveRelayTargetProgressFormatter(route)(transferred, displayTotal), "progress", transferred, displayTotal)
	}
	emitMoveProgress(emit, 4, "image", moveRelayAcceptedMessage(route), "progress")
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("reading image load response: %w", err)
	}
	emitMoveProgress(emit, 4, "image", moveRelayFinishedMessage(route), "progress")
	return nil
}

func emitImageLoadHeartbeat(ctx context.Context, done <-chan struct{}, progressReader *moveProgressReader, imageSize int64, emit moveProgressEmitter, route moveTransferRoute) {
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
			emitMoveProgressBytes(emit, 4, "image", moveImageLoadHeartbeatMessage(transferred, displayTotal, route), "progress", transferred, displayTotal)
		}
	}
}

func moveImageLoadHeartbeatMessage(transferred, total int64, routes ...moveTransferRoute) string {
	route := moveTransferRouteDefault
	if len(routes) > 0 {
		route = routes[0]
	}
	if route == moveTransferRouteAgentHostAgent {
		if total > 0 && transferred >= total {
			return "Target Docker is loading the image archive through the McHarbor host relay (agent-host-agent route)."
		}
		return "Target Docker is receiving the staged image archive through the McHarbor host relay (agent-host-agent route)."
	}
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

func moveRelayOpeningMessage(route moveTransferRoute) string {
	if route == moveTransferRouteAgentHostAgent {
		return "Opening source image archive through the McHarbor host relay (agent-host-agent route)."
	}
	return "Opening source image archive."
}

func moveRelayLoadStartMessage(route moveTransferRoute) string {
	if route == moveTransferRouteAgentHostAgent {
		return "Source image archive reached McHarbor host; loading target image through target agent (agent-host-agent route)."
	}
	return "Source image archive transfer finished; loading target image."
}

func moveRelayAcceptedMessage(route moveTransferRoute) string {
	if route == moveTransferRouteAgentHostAgent {
		return "Target Docker accepted the image archive through the McHarbor host relay (agent-host-agent route); reading load result."
	}
	return "Target Docker accepted the image archive; reading load result."
}

func moveRelayFinishedMessage(route moveTransferRoute) string {
	if route == moveTransferRouteAgentHostAgent {
		return "Image transfer finished through the McHarbor host relay (agent-host-agent route)."
	}
	return "Image transfer finished."
}

func moveRelaySourceProgressFormatter(route moveTransferRoute) func(transferred, total int64) string {
	if route == moveTransferRouteAgentHostAgent {
		return formatMoveAgentHostAgentSourceProgress
	}
	return formatMoveTransferProgress
}

func moveRelayTargetProgressFormatter(route moveTransferRoute) func(transferred, total int64) string {
	if route == moveTransferRouteAgentHostAgent {
		return formatMoveAgentHostAgentTargetProgress
	}
	return formatMoveTargetLoadProgress
}

func formatMoveAgentHostAgentSourceProgress(transferred, total int64) string {
	if total > 0 {
		return "Transferred " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " from source agent to McHarbor host (agent-host-agent route)."
	}
	return "Transferred " + formatMoveBytes(transferred) + " from source agent to McHarbor host (agent-host-agent route)."
}

func formatMoveTargetLoadProgress(transferred, total int64) string {
	if total > 0 {
		return "Sent " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " to target Docker."
	}
	return "Sent " + formatMoveBytes(transferred) + " to target Docker."
}

func formatMoveAgentHostAgentTargetProgress(transferred, total int64) string {
	if total > 0 {
		return "Sent " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " from McHarbor host to target agent/Docker (agent-host-agent route)."
	}
	return "Sent " + formatMoveBytes(transferred) + " from McHarbor host to target agent/Docker (agent-host-agent route)."
}

func formatMoveDirectTransferProgress(transferred, total int64) string {
	if total > 0 {
		return "Sent " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " directly between agents (agent-to-agent route)."
	}
	return "Sent " + formatMoveBytes(transferred) + " directly between agents (agent-to-agent route)."
}

func formatMoveTargetAgentPullProgress(transferred, total int64) string {
	if total > 0 {
		return "Target agent pulled " + formatMoveBytes(transferred) + " of " + formatMoveBytes(total) + " from McHarbor."
	}
	return "Target agent pulled " + formatMoveBytes(transferred) + " from McHarbor."
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

func applyMoveVolumeSettings(info types.ContainerJSON, hc *container.HostConfig, volumePlans []MoveVolumePlan) {
	if hc == nil {
		return
	}

	volumePlansByDestination := moveVolumePlansByDestination(volumePlans)
	covered := make(map[string]struct{})
	binds := hc.Binds[:0]
	for _, bind := range hc.Binds {
		destination := bindMountDestination(bind)
		if _, ok := volumePlansByDestination[path.Clean(destination)]; ok {
			continue
		}
		binds = append(binds, bind)
		if destination != "" {
			covered[path.Clean(destination)] = struct{}{}
		}
	}
	hc.Binds = binds

	mounts := hc.Mounts[:0]
	for _, existingMount := range hc.Mounts {
		if _, ok := volumePlansByDestination[path.Clean(existingMount.Target)]; ok {
			continue
		}
		mounts = append(mounts, existingMount)
		if existingMount.Target != "" {
			covered[path.Clean(existingMount.Target)] = struct{}{}
		}
	}
	hc.Mounts = mounts
	for destination := range hc.Tmpfs {
		if destination != "" {
			covered[path.Clean(destination)] = struct{}{}
		}
	}

	for _, volumePlan := range volumePlans {
		targetMount, ok := moveTargetMount(info, volumePlan)
		if !ok {
			continue
		}
		destination := path.Clean(targetMount.Target)
		if _, exists := covered[destination]; exists {
			continue
		}
		hc.Mounts = append(hc.Mounts, targetMount)
		covered[destination] = struct{}{}
	}
}

func moveTargetMount(info types.ContainerJSON, volumePlan MoveVolumePlan) (mount.Mount, bool) {
	targetDestination := cleanContainerPath(volumePlan.TargetDestination)
	if targetDestination == "" {
		return mount.Mount{}, false
	}
	switch volumePlan.Type {
	case "volume":
		if volumePlan.TargetName == "" {
			return mount.Mount{}, false
		}
		return mount.Mount{
			Type:     mount.TypeVolume,
			Source:   volumePlan.TargetName,
			Target:   targetDestination,
			ReadOnly: !moveVolumeReadWrite(info, volumePlan),
		}, true
	case "bind":
		if volumePlan.TargetSource == "" {
			return mount.Mount{}, false
		}
		return mount.Mount{
			Type:     mount.TypeBind,
			Source:   volumePlan.TargetSource,
			Target:   targetDestination,
			ReadOnly: !moveMountReadWrite(info, volumePlan),
		}, true
	default:
		return mount.Mount{}, false
	}
}

func moveVolumePlansByDestination(volumePlans []MoveVolumePlan) map[string]MoveVolumePlan {
	result := make(map[string]MoveVolumePlan, len(volumePlans))
	for _, plan := range volumePlans {
		if plan.Destination == "" || (plan.Type != "volume" && plan.Type != "bind") {
			continue
		}
		result[path.Clean(plan.Destination)] = plan
	}
	return result
}

func moveVolumeReadWrite(info types.ContainerJSON, plan MoveVolumePlan) bool {
	return moveMountReadWrite(info, plan)
}

func moveMountReadWrite(info types.ContainerJSON, plan MoveVolumePlan) bool {
	for _, mountPoint := range info.Mounts {
		if string(mountPoint.Type) != plan.Type || path.Clean(mountPoint.Destination) != path.Clean(plan.Destination) {
			continue
		}
		if plan.Type == "volume" && mountPoint.Name != plan.Name {
			continue
		}
		if plan.Type == "bind" && mountPoint.Source != plan.Source {
			continue
		}
		return mountPoint.RW
	}
	return !strings.Contains(plan.Mode, "ro")
}

func bindMountDestination(bind string) string {
	parts := strings.Split(bind, ":")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if strings.HasPrefix(part, "/") {
			return part
		}
	}
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func createTargetVolume(ctx context.Context, sourceCli, targetCli *client.Client, sourceName, targetName string) error {
	sourceVolume, err := sourceCli.VolumeInspect(ctx, sourceName)
	if err != nil {
		return fmt.Errorf("inspecting source volume %s: %w", sourceName, err)
	}
	_, err = targetCli.VolumeCreate(ctx, volume.CreateOptions{
		Name:       firstNonEmpty(targetName, sourceVolume.Name),
		Driver:     sourceVolume.Driver,
		DriverOpts: sourceVolume.Options,
		Labels:     sourceVolume.Labels,
	})
	if err != nil {
		return fmt.Errorf("creating target volume %s: %w", targetName, err)
	}
	return nil
}

func (s *Service) copyContainerPath(ctx context.Context, sourceEnvID, targetEnvID string, sourceCli, targetCli *client.Client, sourceID, targetID, sourcePath, targetPath, label string, emit moveProgressEmitter) error {
	if s.pool.IsAgentEnv(sourceEnvID) && s.pool.IsAgentEnv(targetEnvID) {
		started, err := s.tryDirectAgentArchiveTransfer(ctx, sourceEnvID, targetEnvID, sourceID, targetID, sourcePath, targetPath, label, emit)
		if err == nil && started {
			return nil
		}
		if err != nil {
			if started {
				return err
			}
			slog.Warn("direct agent archive transfer unavailable; trying target pull route", "source_env", sourceEnvID, "target_env", targetEnvID, "error", err)
		}
	}
	if s.pool.IsAgentEnv(targetEnvID) {
		started, err := s.tryTargetAgentArchivePullTransfer(ctx, sourceEnvID, targetEnvID, sourceID, targetID, sourcePath, targetPath, label, emit)
		if err == nil && started {
			return nil
		}
		if err != nil {
			if started {
				return err
			}
			slog.Warn("target agent archive pull transfer unavailable; falling back to docker copy", "source_env", sourceEnvID, "target_env", targetEnvID, "error", err)
		}
	}

	sourceCopyPath := strings.TrimRight(sourcePath, "/") + "/."
	reader, _, err := sourceCli.CopyFromContainer(ctx, sourceID, sourceCopyPath)
	if err != nil {
		return fmt.Errorf("copying from source path %s: %w", sourcePath, err)
	}
	defer reader.Close()

	if err := targetCli.CopyToContainer(ctx, targetID, targetPath, reader, container.CopyToContainerOptions{AllowOverwriteDirWithFile: false}); err != nil {
		return fmt.Errorf("copying to target path %s: %w", targetPath, err)
	}
	return nil
}

func (s *Service) tryDirectAgentArchiveTransfer(ctx context.Context, sourceEnvID, targetEnvID, sourceID, targetID, sourcePath, targetPath, label string, emit moveProgressEmitter) (bool, error) {
	if !s.pool.AgentAtLeast(sourceEnvID, moveAgentPullTransferMinVersion) || !s.pool.AgentAtLeast(targetEnvID, moveAgentPullTransferMinVersion) {
		return false, nil
	}
	sourceConn, ok := s.pool.AgentConnection(sourceEnvID)
	if !ok || sourceConn.Transport == nil {
		return false, nil
	}
	targetConn, ok := s.pool.AgentConnection(targetEnvID)
	if !ok || targetConn.Transport == nil || strings.TrimSpace(targetConn.TransferURL) == "" {
		return false, nil
	}

	token, err := newMoveTransferToken()
	if err != nil {
		return false, fmt.Errorf("creating direct archive transfer token: %w", err)
	}
	transferID, err := newMoveTransferID()
	if err != nil {
		return false, fmt.Errorf("creating direct archive transfer id: %w", err)
	}
	sourceCopyPath := strings.TrimRight(sourcePath, "/") + "/."

	prepareCtx, prepareCancel := context.WithTimeout(ctx, 30*time.Second)
	uploadURL, err := targetConn.Transport.PrepareArchiveTransfer(prepareCtx, transferID, token, targetID, targetPath)
	prepareCancel()
	if err != nil {
		targetConn.Transport.CancelTransfer(transferID)
		return false, nil
	}

	started := true
	completed := false
	defer func() {
		if !completed {
			sourceConn.Transport.CancelTransfer(transferID)
			targetConn.Transport.CancelTransfer(transferID)
		}
	}()

	var lastBytes atomic.Int64
	emitMoveProgress(emit, 9, "copy-volumes", "Sending "+moveTransferLabel(label)+" directly between agents.", "progress")
	err = sourceConn.Transport.StartArchiveTransfer(ctx, transferID, sourceID, sourceCopyPath, uploadURL, token, func(transferred int64) {
		if transferred <= 0 {
			return
		}
		lastBytes.Store(transferred)
		emitMoveProgressBytes(emit, 9, "copy-volumes", "Sent "+formatMoveBytes(transferred)+" for "+moveTransferLabel(label)+" directly between agents.", "progress", transferred, 0)
	})
	if err != nil {
		return started, err
	}
	completed = true
	if transferred := lastBytes.Load(); transferred > 0 {
		emitMoveProgressBytes(emit, 9, "copy-volumes", "Sent "+formatMoveBytes(transferred)+" for "+moveTransferLabel(label)+" directly between agents.", "progress", transferred, 0)
	}
	return started, nil
}

func (s *Service) tryTargetAgentArchivePullTransfer(ctx context.Context, sourceEnvID, targetEnvID, sourceID, targetID, sourcePath, targetPath, label string, emit moveProgressEmitter) (bool, error) {
	if !s.pool.AgentAtLeast(targetEnvID, moveAgentPullTransferMinVersion) {
		version, _ := s.pool.AgentVersion(targetEnvID)
		if version == "" {
			version = "unknown"
		}
		return false, fmt.Errorf("target agent %s does not support pull-based move transfers; update mcharbor-agent to %s or newer", version, moveAgentPullTransferMinVersion)
	}
	targetConn, ok := s.pool.AgentConnection(targetEnvID)
	if !ok || targetConn.Transport == nil {
		return false, nil
	}
	sourceCopyPath := strings.TrimRight(sourcePath, "/") + "/."
	entry, err := moveTransfers.create(moveTransferEntry{
		Kind:        moveTransferKindArchive,
		SourceEnvID: sourceEnvID,
		ContainerID: sourceID,
		SourcePath:  sourceCopyPath,
	})
	if err != nil {
		return false, err
	}
	transferURL := "/api/containers/internal/move-transfers/" + entry.ID
	started := true
	completed := false
	defer func() {
		if !completed {
			moveTransfers.cancel(entry.ID)
			targetConn.Transport.CancelTransfer(entry.ID)
		}
	}()

	var lastBytes atomic.Int64
	emitMoveProgress(emit, 9, "copy-volumes", "Target agent is pulling "+moveTransferLabel(label)+" from McHarbor.", "progress")
	err = targetConn.Transport.StartRestoreTransfer(ctx, entry.ID, targetID, targetPath, transferURL, entry.Token, 0, func(transferred int64, stage string) {
		if transferred <= 0 {
			return
		}
		lastBytes.Store(transferred)
		message := "Pulled " + formatMoveBytes(transferred) + " for " + moveTransferLabel(label) + " to the target agent."
		if strings.EqualFold(stage, "apply") {
			message = "Target Docker is applying " + moveTransferLabel(label) + "."
		}
		emitMoveProgressBytes(emit, 9, "copy-volumes", message, "progress", transferred, 0)
	})
	if err != nil {
		return started, err
	}
	completed = true
	if transferred := lastBytes.Load(); transferred > 0 {
		emitMoveProgressBytes(emit, 9, "copy-volumes", "Pulled "+formatMoveBytes(transferred)+" for "+moveTransferLabel(label)+" to the target agent.", "progress", transferred, 0)
	}
	return started, nil
}

func moveTransferLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "container data"
	}
	return label
}
