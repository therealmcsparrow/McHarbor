// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

// dockerAPI is the whole of the Docker surface the debug shell touches. It has no
// ExecAttach on purpose: attach is postHijacked, which bypasses the agent transport.
type dockerAPI interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error)
	ExecCreate(ctx context.Context, containerID string, options client.ExecCreateOptions) (client.ExecCreateResult, error)
	ExecStart(ctx context.Context, execID string, options client.ExecStartOptions) (client.ExecStartResult, error)
	ExecInspect(ctx context.Context, execID string, options client.ExecInspectOptions) (client.ExecInspectResult, error)
	CopyToContainer(ctx context.Context, containerID string, options client.CopyToContainerOptions) (client.CopyToContainerResult, error)
}

// dockerCallTimeout bounds every Docker call; the agent transport has no
// deadline of its own, so a wedged remote host would otherwise hold the request.
const dockerCallTimeout = 30 * time.Second

// execPollInterval is how often a detached exec is re-inspected for its exit code.
var execPollInterval = 50 * time.Millisecond

// archFor maps the daemon's uname -m onto the bundled binary's suffix.
func archFor(info system.Info) (string, bool) {
	if info.OSType != "" && info.OSType != "linux" {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(info.Architecture)) {
	case "x86_64", "amd64":
		return "amd64", true
	case "aarch64", "arm64", "armv8", "armv8l":
		return "arm64", true
	}
	return "", false
}

// runDetached starts cmd as a detached exec and waits for its exit code. Detached
// is the only exec form that travels the agent's HTTP transport.
func runDetached(ctx context.Context, cli dockerAPI, containerID, user string, cmd []string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerCallTimeout)
	defer cancel()
	created, err := cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{Cmd: cmd, User: user})
	if err != nil {
		return -1, fmt.Errorf("creating exec %v: %w", cmd, err)
	}
	if _, err := cli.ExecStart(ctx, created.ID, client.ExecStartOptions{Detach: true}); err != nil {
		return -1, fmt.Errorf("starting exec %v: %w", cmd, err)
	}
	for {
		inspect, err := cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
		if err != nil {
			return -1, fmt.Errorf("inspecting exec %v: %w", cmd, err)
		}
		if !inspect.Running {
			return inspect.ExitCode, nil
		}
		select {
		case <-ctx.Done():
			return -1, fmt.Errorf("waiting for exec %v: %w", cmd, ctx.Err())
		case <-time.After(execPollInterval):
		}
	}
}

// missingExecutable recognises the runtime's refusal to start a binary that is not
// in the image, which on a detached exec arrives as the start error itself.
func missingExecutable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory") || strings.Contains(msg, "executable file not found")
}

// hasShell probes for /bin/sh with a detached exec. A container that has one is
// refused, because installing a binary it does not need is a change with no benefit.
func hasShell(ctx context.Context, cli dockerAPI, containerID string) (bool, error) {
	code, err := runDetached(ctx, cli, containerID, "", []string{"/bin/sh", "-c", ":"})
	if err != nil {
		if missingExecutable(err) {
			return false, nil
		}
		return false, err
	}
	return code == 0, nil
}

// archiveFor builds the tar CopyToContainer extracts at "/": one 0755 directory and
// one 0755 file, both 0:0 with a fixed mtime, so the upload is byte-identical per binary.
func archiveFor(binary []byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	mtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dir := strings.TrimPrefix(InstallDir, "/") + "/"
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: dir, Mode: 0o755, ModTime: mtime, Format: tar.FormatPAX}); err != nil {
		return nil, fmt.Errorf("writing directory header: %w", err)
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: strings.TrimPrefix(InstallPath, "/"), Mode: 0o755, Size: int64(len(binary)), ModTime: mtime, Format: tar.FormatPAX}); err != nil {
		return nil, fmt.Errorf("writing binary header: %w", err)
	}
	if _, err := tw.Write(binary); err != nil {
		return nil, fmt.Errorf("writing binary: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing archive: %w", err)
	}
	return &buf, nil
}
