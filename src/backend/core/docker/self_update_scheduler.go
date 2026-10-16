// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/rs/xid"
	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// ScheduleDetachedSelfUpdateHelper starts short-lived helper containers that can
// recreate McHarbor after the API process stops. The caller must pass the
// inspected currently running McHarbor container, not an arbitrary container.
func ScheduleDetachedSelfUpdateHelper(ctx context.Context, cli *client.Client, current container.InspectResponse, dataDir, dockerHost, operation string) (string, error) {
	return ScheduleDetachedSelfUpdateHelperForImage(ctx, cli, current, dataDir, dockerHost, operation, current.Config.Image)
}

// ScheduleDetachedSelfUpdateHelperForImage schedules the self-update helper and
// lets callers choose the image used by the replacement McHarbor container.
func ScheduleDetachedSelfUpdateHelperForImage(ctx context.Context, cli *client.Client, current container.InspectResponse, dataDir, dockerHost, operation, targetImage string) (string, error) {
	return ScheduleDetachedSelfUpdateHelperForImageWithEnvOverrides(ctx, cli, current, dataDir, dockerHost, operation, targetImage, nil)
}

// ScheduleDetachedSelfUpdateHelperForImageWithEnvOverrides schedules the
// self-update helper and asks it to replace selected environment variables on
// the cloned McHarbor container before it starts the replacement.
func ScheduleDetachedSelfUpdateHelperForImageWithEnvOverrides(ctx context.Context, cli *client.Client, current container.InspectResponse, dataDir, dockerHost, operation, targetImage string, replacementEnv map[string]string) (string, error) {
	return ScheduleDetachedSelfUpdateHelperForImageWithEnvChanges(ctx, cli, current, dataDir, dockerHost, operation, targetImage, replacementEnv, nil)
}

// ScheduleDetachedSelfUpdateHelperForImageWithEnvChanges is the full form:
// replacementEnv adds or replaces variables on the cloned McHarbor container,
// removeEnv deletes them.
//
// Removal is a separate list because the helper merges onto the *current*
// container's environment — there is no way to express "this variable is gone"
// with an override map alone, and a config edit that deletes a variable would
// otherwise report success and leave it in place.
func ScheduleDetachedSelfUpdateHelperForImageWithEnvChanges(ctx context.Context, cli *client.Client, current container.InspectResponse, dataDir, dockerHost, operation, targetImage string, replacementEnv map[string]string, removeEnv []string) (string, error) {
	helperMounts, err := selfUpdateHelperMounts(current, dataDir)
	if err != nil {
		slog.Error("docker: resolve self-update helper mounts failed", "error", err, "container", current.ID)
		return "", fmt.Errorf("preparing self-update helper container: %w", err)
	}

	helperName := "mcharbor-compose-helper-" + xid.New().String()
	watchdogName := helperName + "-watchdog"
	envOverrides := map[string]string{
		"MCHARBOR_SELF_UPDATE_CONTAINER_ID": current.ID,
		"MCHARBOR_SELF_UPDATE_CONTAINER":    strings.TrimPrefix(current.Name, "/"),
		"MCHARBOR_SELF_UPDATE_IMAGE":        selfUpdateTargetImage(current, targetImage),
		"MCHARBOR_SELF_UPDATE_LOG":          "/app/data/self-update/" + helperName + ".log",
		"MCHARBOR_SELF_UPDATE_OPERATION":    operation,
	}
	if len(replacementEnv) > 0 {
		payload, err := json.Marshal(replacementEnv)
		if err != nil {
			return "", fmt.Errorf("encoding self-update env overrides: %w", err)
		}
		envOverrides["MCHARBOR_SELF_UPDATE_ENV_OVERRIDES_JSON"] = string(payload)
	}
	if len(removeEnv) > 0 {
		payload, err := json.Marshal(removeEnv)
		if err != nil {
			return "", fmt.Errorf("encoding self-update env removals: %w", err)
		}
		envOverrides["MCHARBOR_SELF_UPDATE_ENV_REMOVE_JSON"] = string(payload)
	}
	if dockerHost != "" {
		envOverrides["DOCKER_HOST"] = dockerHost
	}
	// The helpers rebuild McHarbor from the inspected container's own Config.Env and never
	// read a key themselves, so the root of trust is left out of their environment rather
	// than copied into two more containers whose inspect output would show it.
	baseEnv := helperBaseEnv(os.Environ())
	env := mergeSelfUpdateEnvVars(baseEnv, envOverrides)
	watchdogEnvOverrides := make(map[string]string, len(envOverrides))
	for key, value := range envOverrides {
		watchdogEnvOverrides[key] = value
	}
	watchdogEnvOverrides["MCHARBOR_SELF_UPDATE_LOG"] = "/app/data/self-update/" + watchdogName + ".log"
	watchdogEnv := mergeSelfUpdateEnvVars(baseEnv, watchdogEnvOverrides)

	baseCtx := context.WithoutCancel(ctx)
	createCtx, createCancel := context.WithTimeout(baseCtx, CallTimeout)
	defer createCancel()

	watchdogResp, err := cli.ContainerCreate(createCtx, client.ContainerCreateOptions{Config: &container.Config{
		Image:      current.Config.Image,
		Entrypoint: []string{"./mcharbor"},
		Cmd:        []string{"self-start-watchdog"},
		WorkingDir: "/app",
		Env:        watchdogEnv,
		Labels: map[string]string{
			"com.mcharbor.helper": "self-start-watchdog",
		},
	}, HostConfig: &container.HostConfig{
		AutoRemove: true,
		Mounts:     helperMounts,
	}, Name: watchdogName})
	if err != nil {
		slog.Error("docker: create self-start watchdog container failed", "error", err, "helper", watchdogName)
		return "", fmt.Errorf("creating self-start watchdog container: %w", err)
	}
	if _, err := cli.ContainerStart(createCtx, watchdogResp.ID, client.ContainerStartOptions{}); err != nil {
		slog.Error("docker: start self-start watchdog container failed", "error", err, "helper", watchdogName, "container", watchdogResp.ID)
		if _, rmErr := cli.ContainerRemove(createCtx, watchdogResp.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
			slog.Error("docker: cleanup self-start watchdog container failed", "error", rmErr, "helper", watchdogName, "container", watchdogResp.ID)
		}
		return "", fmt.Errorf("starting self-start watchdog container: %w", err)
	}

	resp, err := cli.ContainerCreate(createCtx, client.ContainerCreateOptions{Config: &container.Config{
		Image:      current.Config.Image,
		Entrypoint: []string{"./mcharbor"},
		Cmd:        []string{"self-update-helper"},
		WorkingDir: "/app",
		Env:        env,
		Labels: map[string]string{
			"com.mcharbor.helper": "self-update",
		},
	}, HostConfig: &container.HostConfig{
		AutoRemove: true,
		Mounts:     helperMounts,
	}, Name: helperName})
	if err != nil {
		slog.Error("docker: create self-update helper container failed", "error", err, "helper", helperName)
		return "", fmt.Errorf("creating self-update helper container: %w", err)
	}

	startCtx, startCancel := context.WithTimeout(baseCtx, CallTimeout)
	defer startCancel()

	if _, err := cli.ContainerStart(startCtx, resp.ID, client.ContainerStartOptions{}); err != nil {
		slog.Error("docker: start self-update helper container failed", "error", err, "helper", helperName, "container", resp.ID)
		removeCtx, removeCancel := context.WithTimeout(baseCtx, CallTimeout)
		defer removeCancel()
		if _, rmErr := cli.ContainerRemove(removeCtx, resp.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
			slog.Error("docker: cleanup self-update helper container failed", "error", rmErr, "helper", helperName, "container", resp.ID)
		}
		return "", fmt.Errorf("starting self-update helper container: %w", err)
	}

	return "scheduled detached self-update helper; waiting for McHarbor to restart", nil
}

func selfUpdateTargetImage(current container.InspectResponse, targetImage string) string {
	targetImage = strings.TrimSpace(targetImage)
	if targetImage != "" {
		return targetImage
	}
	return current.Config.Image
}

func selfUpdateHelperMounts(current container.InspectResponse, dataDir string) ([]mount.Mount, error) {
	var dataMount mount.Mount
	var ok bool
	for _, destination := range dataDirMountDestinations(dataDir) {
		dataMount, ok = helperMountForDestination(current.Mounts, destination)
		if ok {
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("data directory mount %s not found", dataDir)
	}

	socketMount, ok := helperMountForDestination(current.Mounts, "/var/run/docker.sock")
	if !ok {
		return nil, fmt.Errorf("docker socket mount not found")
	}

	return []mount.Mount{socketMount, dataMount}, nil
}

func dataDirMountDestinations(dataDir string) []string {
	seen := map[string]struct{}{}
	var destinations []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		value = filepath.Clean(value)
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		destinations = append(destinations, value)
	}

	add(dataDir)
	if !filepath.IsAbs(dataDir) {
		if cwd, err := os.Getwd(); err == nil {
			add(filepath.Join(cwd, dataDir))
		}
	}
	add(apppaths.DefaultDataDir)

	return destinations
}

func helperMountForDestination(mounts []container.MountPoint, destination string) (mount.Mount, bool) {
	for _, mp := range mounts {
		if filepath.Clean(mp.Destination) != destination {
			continue
		}

		source := mp.Source
		if mp.Type == mount.TypeVolume {
			source = mp.Name
		}
		if source == "" {
			return mount.Mount{}, false
		}

		return mount.Mount{
			Type:     mount.Type(mp.Type),
			Source:   source,
			Target:   mp.Destination,
			ReadOnly: !mp.RW,
		}, true
	}

	return mount.Mount{}, false
}

// helperBaseEnv drops the root-of-trust variables, and the _FILE paths that point at them,
// from the environment the self-update helpers inherit.
func helperBaseEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if envredact.RootOfTrustName(name) || envredact.RootOfTrustName(strings.TrimSuffix(name, "_FILE")) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func mergeSelfUpdateEnvVars(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}

	skip := make(map[string]struct{}, len(overrides))
	for key := range overrides {
		skip[key] = struct{}{}
	}

	merged := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, blocked := skip[key]; blocked {
				continue
			}
		}
		merged = append(merged, entry)
	}

	for key, value := range overrides {
		merged = append(merged, fmt.Sprintf("%s=%s", key, value))
	}

	return merged
}
