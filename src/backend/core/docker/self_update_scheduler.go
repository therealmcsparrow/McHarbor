// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/rs/xid"
)

// ScheduleDetachedSelfUpdateHelper starts short-lived helper containers that can
// recreate McHarbor after the API process stops. The caller must pass the
// inspected currently running McHarbor container, not an arbitrary container.
func ScheduleDetachedSelfUpdateHelper(ctx context.Context, cli *client.Client, current types.ContainerJSON, dataDir, dockerHost, operation string) (string, error) {
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
		"MCHARBOR_SELF_UPDATE_IMAGE":        current.Config.Image,
		"MCHARBOR_SELF_UPDATE_LOG":          "/app/data/self-update/" + helperName + ".log",
		"MCHARBOR_SELF_UPDATE_OPERATION":    operation,
	}
	if dockerHost != "" {
		envOverrides["DOCKER_HOST"] = dockerHost
	}
	env := mergeSelfUpdateEnvVars(os.Environ(), envOverrides)
	watchdogEnvOverrides := make(map[string]string, len(envOverrides))
	for key, value := range envOverrides {
		watchdogEnvOverrides[key] = value
	}
	watchdogEnvOverrides["MCHARBOR_SELF_UPDATE_LOG"] = "/app/data/self-update/" + watchdogName + ".log"
	watchdogEnv := mergeSelfUpdateEnvVars(os.Environ(), watchdogEnvOverrides)

	baseCtx := context.WithoutCancel(ctx)
	createCtx, createCancel := context.WithTimeout(baseCtx, 30*time.Second)
	defer createCancel()

	watchdogResp, err := cli.ContainerCreate(createCtx, &container.Config{
		Image:      current.Config.Image,
		Entrypoint: []string{"./mcharbor"},
		Cmd:        []string{"self-start-watchdog"},
		WorkingDir: "/app",
		Env:        watchdogEnv,
		Labels: map[string]string{
			"com.mcharbor.helper": "self-start-watchdog",
		},
	}, &container.HostConfig{
		AutoRemove: true,
		Mounts:     helperMounts,
	}, nil, nil, watchdogName)
	if err != nil {
		slog.Error("docker: create self-start watchdog container failed", "error", err, "helper", watchdogName)
		return "", fmt.Errorf("creating self-start watchdog container: %w", err)
	}
	if err := cli.ContainerStart(createCtx, watchdogResp.ID, container.StartOptions{}); err != nil {
		slog.Error("docker: start self-start watchdog container failed", "error", err, "helper", watchdogName, "container", watchdogResp.ID)
		if rmErr := cli.ContainerRemove(createCtx, watchdogResp.ID, container.RemoveOptions{Force: true}); rmErr != nil {
			slog.Error("docker: cleanup self-start watchdog container failed", "error", rmErr, "helper", watchdogName, "container", watchdogResp.ID)
		}
		return "", fmt.Errorf("starting self-start watchdog container: %w", err)
	}

	resp, err := cli.ContainerCreate(createCtx, &container.Config{
		Image:      current.Config.Image,
		Entrypoint: []string{"./mcharbor"},
		Cmd:        []string{"self-update-helper"},
		WorkingDir: "/app",
		Env:        env,
		Labels: map[string]string{
			"com.mcharbor.helper": "self-update",
		},
	}, &container.HostConfig{
		AutoRemove: true,
		Mounts:     helperMounts,
	}, nil, nil, helperName)
	if err != nil {
		slog.Error("docker: create self-update helper container failed", "error", err, "helper", helperName)
		return "", fmt.Errorf("creating self-update helper container: %w", err)
	}

	startCtx, startCancel := context.WithTimeout(baseCtx, 30*time.Second)
	defer startCancel()

	if err := cli.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		slog.Error("docker: start self-update helper container failed", "error", err, "helper", helperName, "container", resp.ID)
		removeCtx, removeCancel := context.WithTimeout(baseCtx, 30*time.Second)
		defer removeCancel()
		if rmErr := cli.ContainerRemove(removeCtx, resp.ID, container.RemoveOptions{Force: true}); rmErr != nil {
			slog.Error("docker: cleanup self-update helper container failed", "error", rmErr, "helper", helperName, "container", resp.ID)
		}
		return "", fmt.Errorf("starting self-update helper container: %w", err)
	}

	return "scheduled detached self-update helper; waiting for McHarbor to restart", nil
}

func selfUpdateHelperMounts(current types.ContainerJSON, dataDir string) ([]mount.Mount, error) {
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
	add("/app/data")

	return destinations
}

func helperMountForDestination(mounts []types.MountPoint, destination string) (mount.Mount, bool) {
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
