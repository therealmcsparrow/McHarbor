// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// socketRegex redacts Docker endpoints and socket paths out of diagnostic
// output, which is persisted and therefore leaves the server. Copied in
// deliberately from the incident assembler rather than shared, so neither
// module can widen the other's redaction by accident.
var socketRegex = regexp.MustCompile(`(?i)(unix|npipe|tcp)://[^\s"']+|[^\s"']*docker\.sock\b`)

// describeState renders the part of an inspect an operator at 03:00 reads:
// the state, how long, the exit code and the health. Never the whole inspect —
// that carries mounts, env and the endpoint.
func describeState(info container.InspectResponse) string {
	if info.State == nil {
		return "state unavailable"
	}
	lines := []string{
		"status: " + string(info.State.Status),
		fmt.Sprintf("running: %t", info.State.Running),
		fmt.Sprintf("restarting: %t", info.State.Restarting),
		fmt.Sprintf("exit code: %d", info.State.ExitCode),
		"started at: " + info.State.StartedAt,
		"finished at: " + info.State.FinishedAt,
	}
	if info.State.OOMKilled {
		lines = append(lines, "killed by the kernel OOM killer")
	}
	if info.State.Health != nil {
		lines = append(lines, "health: "+string(info.State.Health.Status),
			fmt.Sprintf("failing streak: %d", info.State.Health.FailingStreak))
	}
	if info.RestartCount > 0 {
		lines = append(lines, fmt.Sprintf("restart count: %d", info.RestartCount))
	}
	return redact(strings.Join(lines, "\n"))
}

// readLogs pulls the tail of a container's log, demultiplexing the 8-byte
// stream header when the container has no TTY, and bounds both the read and the
// stored result.
func readLogs(ctx context.Context, rt Runtime, target string) (string, error) {
	infoRes, err := rt.ContainerInspect(ctx, target, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	info := infoRes.Container
	rc, err := rt.ContainerLogs(ctx, target, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: fmt.Sprint(MaxLogLines),
	})
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			slog.Warn("runbooks: closing container log stream failed", "error", cerr)
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return "", err
	}
	text := raw
	if info.Config == nil || !info.Config.Tty {
		var out bytes.Buffer
		if _, copyErr := stdcopy.StdCopy(&out, &out, bytes.NewReader(raw)); copyErr == nil {
			text = out.Bytes()
		}
	}
	return redact(string(text)), nil
}

// redact strips socket paths and endpoints, then truncates to MaxOutputBytes
// saying so, because a silently cut diagnostic reads as a complete one.
func redact(text string) string {
	clean := socketRegex.ReplaceAllString(text, "[redacted]")
	if len(clean) <= MaxOutputBytes {
		return clean
	}
	return clean[:MaxOutputBytes] + "\n[truncated]"
}
