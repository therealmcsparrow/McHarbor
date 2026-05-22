// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

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
		target := "/mnt/cleanup_" + strings.Replace(strings.Trim(p, "/"), "/", "_", -1)
		// Avoid overly long target names.
		if len(target) > 120 {
			target = "/mnt/cleanup_" + strings.Trim(strings.Replace(p, "/", "_", -1), "_")[:60]
		}
		_ = i
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: p,
			Target: target,
		})
		cmds = append(cmds, "rm -rf "+target+"/*", "rm -rf "+target)
	}

	createCtx, createCancel := context.WithTimeout(ctx, 30*time.Second)
	defer createCancel()
	body, err := cli.ContainerCreate(createCtx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"sh", "-c", strings.Join(cmds, " ; ")},
	}, &container.HostConfig{
		Mounts:     mounts,
		AutoRemove: true,
	}, nil, nil, "")
	if err != nil {
		return
	}

	startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
	defer startCancel()
	if err := cli.ContainerStart(startCtx, body.ID, container.StartOptions{}); err != nil {
		rmCtx, rmCancel := context.WithTimeout(ctx, 30*time.Second)
		defer rmCancel()
		cli.ContainerRemove(rmCtx, body.ID, container.RemoveOptions{Force: true})
		return
	}

	// Wait for the cleanup container to finish (AutoRemove handles removal).
	cli.ContainerWait(ctx, body.ID, container.WaitConditionNotRunning)
}
