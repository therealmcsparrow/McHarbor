// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package docker owns the per-environment Docker client pool and the host-side helpers (protection, secret volumes, self-update) built on it.
package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const hostPathHelperImage = "alpine:3.20"

// systemPaths that should never be removed by bind-mount cleanup.
var systemPaths = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64",
	"/media", "/mnt", "/opt", "/proc", "/root", "/run", "/sbin",
	"/snap", "/srv", "/sys", "/tmp", "/usr", "/var",
	"/var/run/docker.sock",
}

// isSystemPath returns true if the path is a known system directory.
func isSystemPath(p string) bool {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return true
	}
	for _, s := range systemPaths {
		if p == s {
			return true
		}
	}
	return false
}

// EnsureBindMountPaths creates missing bind mount source directories on the Docker host.
func EnsureBindMountPaths(ctx context.Context, cli *client.Client, paths []string) error {
	clean := cleanHostPaths(paths)
	if len(clean) == 0 {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := ensureHostPathHelperImage(opCtx, cli); err != nil {
		return err
	}

	cmds := make([]string, 0, len(clean))
	for _, p := range clean {
		cmds = append(cmds, "mkdir -p -- "+shellQuote("/host"+p))
	}

	body, err := cli.ContainerCreate(opCtx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        hostPathHelperImage,
			Cmd:          []string{"sh", "-lc", strings.Join(cmds, " && ")},
			AttachStdout: true,
			AttachStderr: true,
			Labels:       map[string]string{HelperLabel: "bind-path"},
		},
		HostConfig: &container.HostConfig{
			Binds:      []string{"/:/host:rw"},
			AutoRemove: false,
			Privileged: true,
		},
	})
	if err != nil {
		return fmt.Errorf("creating bind path helper container: %w", err)
	}
	defer removeHelperContainer(ctx, cli, body.ID)

	if _, err := cli.ContainerStart(opCtx, body.ID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting bind path helper container: %w", err)
	}

	wait := cli.ContainerWait(opCtx, body.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case status := <-wait.Result:
		if status.StatusCode != 0 {
			output, readErr := helperContainerLogs(opCtx, cli, body.ID)
			if readErr != nil {
				return fmt.Errorf("bind path helper exited with status %d and logs could not be read: %w", status.StatusCode, readErr)
			}
			return fmt.Errorf("bind path helper exited with status %d: %s", status.StatusCode, strings.TrimSpace(output))
		}
	case err := <-wait.Error:
		if err != nil {
			return fmt.Errorf("waiting for bind path helper container: %w", err)
		}
	case <-opCtx.Done():
		return fmt.Errorf("bind path helper timed out: %w", opCtx.Err())
	}

	return nil
}

func cleanHostPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || !strings.HasPrefix(p, "/") {
			continue
		}
		p = path.Clean(p)
		if p == "." || seen[p] || isSystemPath(p) {
			continue
		}
		seen[p] = true
		clean = append(clean, p)
	}
	return clean
}

func ensureHostPathHelperImage(ctx context.Context, cli *client.Client) error {
	if _, err := cli.ImageInspect(ctx, hostPathHelperImage); err == nil {
		return nil
	}
	reader, err := cli.ImagePull(ctx, hostPathHelperImage, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling bind path helper image: %w", err)
	}
	defer closeQuietly(reader, "image pull stream")
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("draining bind path helper image pull: %w", err)
	}
	return nil
}

func helperContainerLogs(ctx context.Context, cli *client.Client, id string) (string, error) {
	reader, err := cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return "", err
	}
	defer closeQuietly(reader, "helper container logs")
	output, err := io.ReadAll(io.LimitReader(reader, 4096))
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func removeHelperContainer(ctx context.Context, cli *client.Client, id string) {
	rmCtx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	if _, err := cli.ContainerRemove(rmCtx, id, client.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		return
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// RemoveBindMounts removes host directories that were used as bind mounts.
// Since McHarbor runs inside a container it cannot access host paths directly,
// so a short-lived Alpine container is spawned for each path.
func RemoveBindMounts(ctx context.Context, cli *client.Client, paths []string) {
	if len(paths) == 0 {
		return
	}

	// Deduplicate and filter.
	seen := make(map[string]bool, len(paths))
	var clean []string
	for _, p := range paths {
		p = strings.TrimRight(p, "/")
		if p == "" || seen[p] || isSystemPath(p) {
			continue
		}
		seen[p] = true
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return
	}

	// Mount all paths into a single cleanup container and rm -rf them.
	var mounts []mount.Mount
	var cmds []string
	for i, p := range clean {
		target := "/mnt/cleanup_" + strings.ReplaceAll(strings.Trim(p, "/"), "/", "_")
		// Avoid overly long target names.
		if len(target) > 120 {
			target = "/mnt/cleanup_" + strings.Trim(strings.ReplaceAll(p, "/", "_"), "_")[:60]
		}
		_ = i
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: p,
			Target: target,
		})
		cmds = append(cmds, "rm -rf "+target+"/*", "rm -rf "+target)
	}

	createCtx, createCancel := context.WithTimeout(ctx, CallTimeout)
	defer createCancel()
	body, err := cli.ContainerCreate(createCtx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:  "alpine:latest",
			Cmd:    []string{"sh", "-c", strings.Join(cmds, " ; ")},
			Labels: map[string]string{HelperLabel: "bind-cleanup"},
		},
		HostConfig: &container.HostConfig{
			Mounts:     mounts,
			AutoRemove: true,
		},
	})
	if err != nil {
		return
	}

	startCtx, startCancel := context.WithTimeout(ctx, CallTimeout)
	defer startCancel()
	if _, err := cli.ContainerStart(startCtx, body.ID, client.ContainerStartOptions{}); err != nil {
		rmCtx, rmCancel := context.WithTimeout(ctx, CallTimeout)
		defer rmCancel()
		if _, rmErr := cli.ContainerRemove(rmCtx, body.ID, client.ContainerRemoveOptions{Force: true}); rmErr != nil {
			slog.Warn("removing bind path cleanup container failed", "container", body.ID, "error", rmErr)
		}
		return
	}

	// Wait for the cleanup container to finish (AutoRemove handles removal).
	cli.ContainerWait(ctx, body.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
}

// closeQuietly closes c on a read-only path where a close failure cannot change the result, logging it so it is not lost.
func closeQuietly(c io.Closer, what string) {
	if err := c.Close(); err != nil {
		slog.Warn("closing "+what+" failed", "error", err)
	}
}
