// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rs/xid"
)

// ExecRequest is the JSON body for POST /containers/{id}/exec.
type ExecRequest struct {
	Command []string `json:"command"`
	WorkDir string   `json:"workDir,omitempty"`
	User    string   `json:"user,omitempty"`
	// Shell overrides the exec binary. Empty means "let Docker's
	// container entrypoint handle it" (i.e. exec the command verbatim).
	// When set, the command is wrapped as `<shell> -c '<cmd joined>'`.
	Shell string `json:"shell,omitempty"`
}

// ExecResult is the JSON response for POST /containers/{id}/exec.
type ExecResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMs int    `json:"durationMs"`
}

// FilesystemChangeResult is one entry of GET /containers/{id}/diff.
type FilesystemChangeResult struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // "modified" | "added" | "deleted"
}

// ContainerRuntimeInfo is the JSON response for GET /containers/{id}/runtime.
type ContainerRuntimeInfo struct {
	Runtime            string   `json:"runtime"`
	RuntimeVersion     string   `json:"runtimeVersion"`
	OperatingSystem    string   `json:"operatingSystem"`
	KernelVersion      string   `json:"kernelVersion"`
	Architecture       string   `json:"architecture"`
	CgroupVersion      string   `json:"cgroupVersion"`
	SecurityOptions    []string `json:"securityOptions"`
	IsRootless         bool     `json:"isRootless"`
	IsDockerInDocker   bool     `json:"isDockerInDocker"`
	RootlessVolumePath string   `json:"rootlessVolumePath,omitempty"`
	ContainerID        string   `json:"containerId,omitempty"`
}

// RunExec executes a one-shot command in a running container, captures
// stdout/stderr/exit code, records the invocation in exec_history, and
// returns the combined result. WorkDir and User are passed through to
// the underlying exec instance when non-empty.
func (s *Service) RunExec(ctx context.Context, envID, containerID, username, userID string, req ExecRequest) (ExecResult, error) {
	if len(req.Command) == 0 {
		return ExecResult{}, fmt.Errorf("command is required")
	}

	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return ExecResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, longDockerOpTimeout)
	defer cancel()

	execConfig := client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
	}
	if req.Shell != "" {
		// Wrap the user's command so the chosen shell interprets it.
		// This matches the docker-cli `docker exec <container> <shell> -c <cmd>`
		// behavior and lets the dropdown's "Auto-detect" (empty) still
		// pass-through unmodified.
		execConfig.Cmd = append([]string{req.Shell, "-c"}, strings.Join(req.Command, " "))
	} else {
		execConfig.Cmd = req.Command
	}
	if req.WorkDir != "" {
		execConfig.WorkingDir = req.WorkDir
	}
	if req.User != "" {
		execConfig.User = req.User
	}

	execResp, err := cli.ExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return ExecResult{}, fmt.Errorf("creating exec: %w", err)
	}

	start := time.Now()
	hijack, err := cli.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("attaching exec: %w", err)
	}
	defer hijack.Close()

	var stdout, stderr bytes.Buffer
	done := make(chan struct{})
	go func() {
		if _, err := stdcopy.StdCopy(&stdout, &stderr, hijack.Reader); err != nil {
			slog.Debug("containers: exec output stream ended with error", "error", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ExecResult{}, ctx.Err()
	}

	inspect, err := cli.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("inspecting exec: %w", err)
	}

	duration := int(time.Since(start) / time.Millisecond)
	result := ExecResult{
		Stdout:     stripAnsi(stdout.String()),
		Stderr:     stripAnsi(stderr.String()),
		ExitCode:   inspect.ExitCode,
		DurationMs: duration,
	}

	// Best-effort audit/history persistence. A failure here must not
	// mask the command result the caller is waiting on.
	name := normalizeContainerName(containerID, containerID)
	if info, inspectErr := cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{}); inspectErr == nil {
		name = normalizeContainerName(info.Container.Name, containerID)
	}
	historyEntry := ExecHistoryEntry{
		ID:            xid.New().String(),
		UserID:        userID,
		Username:      username,
		EnvironmentID: envID,
		ContainerID:   containerID,
		ContainerName: name,
		Command:       req.Command,
		WorkDir:       req.WorkDir,
		User:          req.User,
		ExitCode:      result.ExitCode,
		Stdout:        result.Stdout,
		Stderr:        result.Stderr,
		DurationMs:    result.DurationMs,
	}
	if err := s.recordExecHistory(context.WithoutCancel(ctx), historyEntry); err != nil {
		slog.Warn("containers: recording exec history failed", "error", err, "container", containerID)
	}

	return result, nil
}

// Diff returns the filesystem changes recorded against the container's
// root filesystem since it was started. Container must be running for
// Docker to track diffs; stopped containers return an empty list.
func (s *Service) Diff(ctx context.Context, envID, containerID string) ([]FilesystemChangeResult, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	changesRes, err := cli.ContainerDiff(ctx, containerID, client.ContainerDiffOptions{})
	changes := changesRes.Changes
	if err != nil {
		return nil, fmt.Errorf("getting container changes: %w", err)
	}

	results := make([]FilesystemChangeResult, 0, len(changes))
	for _, c := range changes {
		results = append(results, FilesystemChangeResult{
			Path: c.Path,
			Kind: changeKindString(c.Kind),
		})
	}
	return results, nil
}

// Runtime returns a snapshot of the container host's runtime — daemon
// version, OS, kernel, architecture, cgroup version, security options —
// plus best-effort flags for rootless and Docker-in-Docker
// detection derived from the daemon info and the container's own config.
//
// Heuristics are intentionally cheap; if a deployment sits behind
// proxies or wrappers that mask these signals the flags fall back to
// false rather than misreporting.
func (s *Service) Runtime(ctx context.Context, envID, containerID string) (ContainerRuntimeInfo, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return ContainerRuntimeInfo{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	infoRes, err := cli.Info(ctx, client.InfoOptions{})
	info := infoRes.Info
	if err != nil {
		return ContainerRuntimeInfo{}, fmt.Errorf("getting daemon info: %w", err)
	}

	out := ContainerRuntimeInfo{
		Runtime:         info.DefaultRuntime,
		RuntimeVersion:  info.ServerVersion,
		OperatingSystem: info.OperatingSystem,
		KernelVersion:   info.KernelVersion,
		Architecture:    info.Architecture,
		CgroupVersion:   info.CgroupVersion,
		SecurityOptions: append([]string{}, info.SecurityOptions...),
		ContainerID:     containerID,
	}

	if info.DefaultRuntime == "" {
		out.Runtime = "runc"
	}

	// Rootless Docker sets DockerRootDir under $HOME rather than /var/lib/docker.
	if strings.HasPrefix(info.DockerRootDir, "/home/") ||
		strings.Contains(info.DockerRootDir, "/.local/share/docker") ||
		strings.Contains(strings.ToLower(info.OperatingSystem), "rootless") {
		out.IsRootless = true
		out.RootlessVolumePath = info.DockerRootDir
	}

	// DinD detection: Docker Desktop reports "Docker Desktop" in the
	// operating system string; on Linux a container running with the
	// host docker socket mounted is also a DinD marker.
	if strings.Contains(info.OperatingSystem, "Docker Desktop") ||
		strings.Contains(strings.ToLower(info.OperatingSystem), "boot2docker") ||
		strings.Contains(strings.ToLower(info.OperatingSystem), "docker desktop") {
		out.IsDockerInDocker = true
	}
	return out, nil
}

// changeKindString maps Docker's numeric change kind to a stable string
// identifier. Unknown values fall back to "modified" so the UI can
// still render the entry without crashing.
func changeKindString(k container.ChangeType) string {
	switch k {
	case 1:
		return "added"
	case 2:
		return "deleted"
	default:
		return "modified"
	}
}
