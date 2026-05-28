// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

const utilityImage = "alpine:3.20"

var errAgentUnsupported = errors.New("agent environments do not support host OS operations")

// Service runs bounded host OS operations through Docker.
type Service struct {
	pool   *coredocker.ClientPool
	logger *slog.Logger
}

// NewService creates a new system service.
func NewService(pool *coredocker.ClientPool, logger *slog.Logger) *Service {
	return &Service{pool: pool, logger: logger}
}

// Logs returns a bounded host OS log snapshot.
func (s *Service) Logs(ctx context.Context, envID, source string, tail int) (*OSLogResult, error) {
	if tail < 1 {
		tail = 200
	}
	if tail > 1000 {
		tail = 1000
	}

	cmd, ok := logCommand(source, tail)
	if !ok {
		return nil, fmt.Errorf("invalid log source")
	}

	result, err := s.runHostCommand(ctx, envID, cmd, false, 30*time.Second)
	if err != nil {
		return nil, err
	}

	lines, notices := splitLogMarkers(result.Output)
	return &OSLogResult{
		Source:    source,
		Tail:      tail,
		Lines:     lines,
		Notices:   notices,
		FetchedAt: time.Now().UTC(),
	}, nil
}

// CheckUpdates returns available host OS package updates.
func (s *Service) CheckUpdates(ctx context.Context, envID string) (*OSUpdateCheckResult, error) {
	result, err := s.runHostCommand(ctx, envID, updateCheckScript(), false, 2*time.Minute)
	if err != nil {
		return nil, err
	}

	manager, output := splitManagerMarker(result.Output)
	updates := outputLines(output)

	return &OSUpdateCheckResult{
		Manager:   manager,
		Available: len(updates) > 0,
		Updates:   updates,
		Output:    output,
		CheckedAt: time.Now().UTC(),
	}, nil
}

// ApplyUpdates runs the host OS package manager update command.
func (s *Service) ApplyUpdates(ctx context.Context, envID string) (*OSUpdateApplyResult, error) {
	result, err := s.runHostCommand(ctx, envID, updateApplyScript(), true, 30*time.Minute)
	if err != nil {
		return nil, err
	}

	manager, output := splitManagerMarker(result.Output)
	return &OSUpdateApplyResult{
		Manager:  manager,
		ExitCode: result.ExitCode,
		Success:  result.ExitCode == 0,
		Output:   output,
		RanAt:    time.Now().UTC(),
	}, nil
}

type commandResult struct {
	Output   string
	ExitCode int64
}

func (s *Service) runHostCommand(ctx context.Context, envID, script string, readWrite bool, timeout time.Duration) (*commandResult, error) {
	if s.pool.IsAgentEnv(envID) {
		return nil, errAgentUnsupported
	}

	cli, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := s.ensureUtilityImage(opCtx, cli); err != nil {
		return nil, err
	}

	mountMode := "ro"
	if readWrite {
		mountMode = "rw"
	}

	resp, err := cli.ContainerCreate(opCtx, &container.Config{
		Image:        utilityImage,
		Cmd:          []string{"sh", "-lc", script},
		Tty:          true,
		AttachStdout: true,
		AttachStderr: true,
	}, &container.HostConfig{
		Binds:      []string{"/:/host:" + mountMode},
		PidMode:    "host",
		Privileged: readWrite,
	}, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("creating host command container: %w", err)
	}
	defer s.removeContainer(cli, resp.ID)

	if err := cli.ContainerStart(opCtx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("starting host command container: %w", err)
	}

	statusCh, errCh := cli.ContainerWait(opCtx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int64
	select {
	case status := <-statusCh:
		exitCode = status.StatusCode
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("waiting for host command container: %w", err)
		}
	case <-opCtx.Done():
		return nil, fmt.Errorf("host command timed out: %w", opCtx.Err())
	}

	logReader, err := cli.ContainerLogs(opCtx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("reading host command output: %w", err)
	}
	defer logReader.Close()

	output, err := io.ReadAll(logReader)
	if err != nil {
		return nil, fmt.Errorf("reading host command stream: %w", err)
	}

	return &commandResult{Output: string(output), ExitCode: exitCode}, nil
}

func (s *Service) ensureUtilityImage(ctx context.Context, cli *dockerclient.Client) error {
	if _, _, err := cli.ImageInspectWithRaw(ctx, utilityImage); err == nil {
		return nil
	}

	reader, err := cli.ImagePull(ctx, utilityImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling utility image: %w", err)
	}
	defer reader.Close()

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("draining utility image pull: %w", err)
	}
	return nil
}

func (s *Service) removeContainer(cli *dockerclient.Client, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
		s.logger.Warn("system: cleanup container failed", "error", err, "container", id)
	}
}

func logCommand(source string, tail int) (string, bool) {
	tailArg := fmt.Sprintf("%d", tail)
	switch source {
	case "system":
		return logScript(tailArg,
			[]string{"/host/var/log/syslog", "/host/var/log/messages"},
			"journalctl -n "+tailArg+" --no-pager",
		), true
	case "kernel":
		return logScript(tailArg,
			[]string{"/host/var/log/kern.log", "/host/var/log/dmesg"},
			"journalctl -k -n "+tailArg+" --no-pager",
		), true
	case "auth":
		return logScript(tailArg,
			[]string{"/host/var/log/auth.log", "/host/var/log/secure"},
			"journalctl -u ssh -u sshd -n "+tailArg+" --no-pager",
		), true
	case "docker":
		return logScript(tailArg,
			[]string{"/host/var/log/docker.log"},
			"journalctl -u docker -n "+tailArg+" --no-pager",
		), true
	default:
		return "", false
	}
}

func logScript(tailArg string, files []string, journalCommand string) string {
	var builder strings.Builder
	builder.WriteString("set +e\n")
	builder.WriteString("read_file_logs() {\n")
	for _, file := range files {
		builder.WriteString("  if [ -f '")
		builder.WriteString(file)
		builder.WriteString("' ]; then\n")
		builder.WriteString("    if [ -r '")
		builder.WriteString(file)
		builder.WriteString("' ]; then tail -n ")
		builder.WriteString(tailArg)
		builder.WriteString(" '")
		builder.WriteString(file)
		builder.WriteString("'; return 0; fi\n")
		builder.WriteString("    echo 'MCHARBOR_NOTICE=permission_denied'\n")
		builder.WriteString("  fi\n")
	}
	builder.WriteString("  return 1\n")
	builder.WriteString("}\n")
	builder.WriteString("read_journal_logs() {\n")
	builder.WriteString("  if [ -x /host/bin/journalctl ] || [ -x /host/usr/bin/journalctl ]; then\n")
	builder.WriteString("    output=$(chroot /host sh -lc '")
	builder.WriteString(journalCommand)
	builder.WriteString("' 2>&1)\n")
	builder.WriteString("    status=$?\n")
	builder.WriteString("    if [ $status -ne 0 ]; then echo 'MCHARBOR_NOTICE=journalctl_failed'; fi\n")
	builder.WriteString("    printf '%s\\n' \"$output\"\n")
	builder.WriteString("    return 0\n")
	builder.WriteString("  fi\n")
	builder.WriteString("  echo 'MCHARBOR_NOTICE=no_supported_log_source'\n")
	builder.WriteString("  return 0\n")
	builder.WriteString("}\n")
	builder.WriteString("read_file_logs || read_journal_logs\n")
	return builder.String()
}

func updateCheckScript() string {
	return `chroot /host sh -lc '
if command -v apt >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=apt
  apt list --upgradable 2>/dev/null | tail -n +2 | head -n 200 || true
elif command -v dnf >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=dnf
  dnf check-update 2>/dev/null | tail -n +2 | head -n 200 || true
elif command -v yum >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=yum
  yum check-update 2>/dev/null | tail -n +2 | head -n 200 || true
elif command -v apk >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=apk
  apk version -l "<" 2>/dev/null | head -n 200 || true
elif command -v pacman >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=pacman
  pacman -Qu 2>/dev/null | head -n 200 || true
elif command -v zypper >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=zypper
  zypper --non-interactive list-updates 2>/dev/null | tail -n +5 | head -n 200 || true
else
  echo MCHARBOR_MANAGER=unknown
fi
'`
}

func updateApplyScript() string {
	return `chroot /host sh -lc '
if command -v apt-get >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=apt
  apt-get update && DEBIAN_FRONTEND=noninteractive apt-get -y upgrade
elif command -v dnf >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=dnf
  dnf -y upgrade
elif command -v yum >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=yum
  yum -y update
elif command -v apk >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=apk
  apk update && apk upgrade
elif command -v pacman >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=pacman
  pacman -Syu --noconfirm
elif command -v zypper >/dev/null 2>&1; then
  echo MCHARBOR_MANAGER=zypper
  zypper --non-interactive refresh && zypper --non-interactive update
else
  echo MCHARBOR_MANAGER=unknown
  echo "No supported package manager found."
  exit 1
fi
'`
}

func splitManagerMarker(raw string) (string, string) {
	lines := outputLines(raw)
	manager := "unknown"
	if len(lines) > 0 && strings.HasPrefix(lines[0], "MCHARBOR_MANAGER=") {
		manager = strings.TrimPrefix(lines[0], "MCHARBOR_MANAGER=")
		lines = lines[1:]
	}
	return manager, strings.Join(lines, "\n")
}

func splitLogMarkers(raw string) ([]string, []string) {
	rawLines := outputLines(raw)
	lines := make([]string, 0, len(rawLines))
	notices := make([]string, 0)
	seenNotices := make(map[string]bool)
	for _, line := range rawLines {
		if strings.HasPrefix(line, "MCHARBOR_NOTICE=") {
			notice := strings.TrimPrefix(line, "MCHARBOR_NOTICE=")
			if !seenNotices[notice] {
				notices = append(notices, notice)
				seenNotices[notice] = true
			}
			continue
		}
		lines = append(lines, line)
	}
	return lines, notices
}

func outputLines(output string) []string {
	normalized := strings.ReplaceAll(output, "\r\n", "\n")
	rawLines := strings.Split(strings.TrimSpace(normalized), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
