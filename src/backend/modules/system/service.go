// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const utilityImage = "alpine:3.20"

var errAgentUnsupported = errors.New("agent environments do not support host OS operations")

// Service runs bounded host OS operations through Docker (local envs) or the
// remote agent (agent envs).
type Service struct {
	pool           *coredocker.ClientPool
	agentPool      *coreagent.AgentPool
	logger         *slog.Logger
	restartPending atomic.Bool
}

// NewService creates a new system service.
func NewService(pool *coredocker.ClientPool, logger *slog.Logger) *Service {
	return &Service{pool: pool, logger: logger}
}

// NewServiceWithAgent creates a system service that can route host
// operations through a connected remote agent.
func NewServiceWithAgent(pool *coredocker.ClientPool, agentPool *coreagent.AgentPool, logger *slog.Logger) *Service {
	return &Service{pool: pool, agentPool: agentPool, logger: logger}
}

// Logs returns a bounded host OS log snapshot. target names one guest and is
// read only by the qemu source, where an empty one asks which guests have a log.
func (s *Service) Logs(ctx context.Context, envID, source, target string, tail int) (*OSLogResult, error) {
	if tail < 1 {
		tail = 200
	}
	if tail > 1000 {
		tail = 1000
	}

	if s.pool.IsAgentEnvContext(ctx, envID) {
		return s.logsViaAgent(ctx, envID, source, target, tail)
	}

	cmd, ok := logCommand(source, tail)
	if !ok {
		return nil, fmt.Errorf("invalid log source")
	}

	result, err := s.runHostCommand(ctx, envID, cmd, false, hostCallTimeout)
	if err != nil {
		return nil, err
	}

	lines, notices, availableFiles, fallbackFile := splitLogMarkers(result.Output)
	return &OSLogResult{
		Source:         source,
		Tail:           tail,
		Lines:          lines,
		Notices:        notices,
		AvailableFiles: availableFiles,
		FallbackFile:   fallbackFile,
		FetchedAt:      time.Now().UTC(),
	}, nil
}

func logTargets(in []coreagent.HostLogTarget) []LogTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]LogTarget, len(in))
	for i, t := range in {
		out[i] = LogTarget{ID: t.ID, Label: t.Label}
	}
	return out
}

// agentOnlyLogSources are the sources no shell fallback can serve, because they
// exist only in the agent's own reader.
var agentOnlyLogSources = map[string]bool{"libvirt": true, "agent": true, "qemu": true}

// ErrLogSourceNeedsNewerAgent is the one log failure an operator can act on, so
// it is a sentinel rather than prose the handler would flatten into "failed".
var ErrLogSourceNeedsNewerAgent = errors.New("log source requires a newer agent")

// logsViaAgent fetches host logs by streaming from the connected agent.
func (s *Service) logsViaAgent(ctx context.Context, envID, source, target string, tail int) (*OSLogResult, error) {
	t := s.agentPool.ResolveTransport(envID)
	if t == nil {
		return nil, fmt.Errorf("agent is not connected for environment %s", envID)
	}
	if !t.HostLogsSupported() {
		return nil, fmt.Errorf("host logs require agent 1.8.0+; please upgrade mcharbor-agent")
	}
	// An agent without these sources answers "invalid log source", which reads
	// as a broken host rather than an agent that needs rebuilding.
	if agentOnlyLogSources[source] && !t.HostLogSourcesSupported() {
		return nil, fmt.Errorf("the %s log requires agent %s build %s or newer: %w",
			source, coreagent.HostLogSourcesMinVersion, coreagent.HostLogSourcesMinBuild,
			ErrLogSourceNeedsNewerAgent)
	}
	logCtx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancel()
	stream, err := t.HostLogs(logCtx, coreagent.HostLogRequestPayload{Source: source, Tail: tail, Target: target})
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, tail)
	for line := range stream.Lines {
		lines = append(lines, line)
	}
	if err := stream.Err(); err != nil {
		return &OSLogResult{
			Source:         source,
			Tail:           tail,
			Lines:          lines,
			Notices:        stream.Notices,
			AvailableFiles: stream.AvailableFiles,
			Targets:        logTargets(stream.Targets),
			FetchedAt:      time.Now().UTC(),
		}, err
	}
	return &OSLogResult{
		Source:         source,
		Tail:           tail,
		Lines:          lines,
		Notices:        stream.Notices,
		AvailableFiles: stream.AvailableFiles,
		Targets:        logTargets(stream.Targets),
		FetchedAt:      time.Now().UTC(),
	}, nil
}

// CheckUpdates returns available host OS package updates on the target host.
func (s *Service) CheckUpdates(ctx context.Context, target HostTarget) (*OSUpdateCheckResult, error) {
	if !target.Self && s.pool.IsAgentEnvContext(ctx, target.EnvID) {
		return nil, fmt.Errorf("operating system update checks are not yet supported over remote agents; run locally and review output on the agent host")
	}

	result, err := s.runTarget(ctx, target, updateCheckScript(), false, 2*time.Minute)
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

// ApplyUpdates runs the host OS package manager update command. opts carries
// the operator's decisions on new dependencies and unavailable packages, which
// decide whether a kernel ABI bump or a vanished mirror file blocks the run.
func (s *Service) ApplyUpdates(ctx context.Context, target HostTarget, opts hostUpgradeOptions) (*OSUpdateApplyResult, error) {
	if !target.Self && s.pool.IsAgentEnvContext(ctx, target.EnvID) {
		return nil, fmt.Errorf("operating system update apply is not yet supported over remote agents; run on the agent host directly")
	}

	result, err := s.runTarget(ctx, target, updateApplyScript(opts), true, 30*time.Minute)
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
	if s.pool.IsAgentEnvContext(ctx, envID) {
		return nil, errAgentUnsupported
	}

	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}
	return s.runHostCommandWith(ctx, cli, script, readWrite, timeout)
}

// runHostCommandWith runs script in a privileged utility container on cli's
// daemon, with that daemon's host filesystem mounted at /host.
func (s *Service) runHostCommandWith(ctx context.Context, cli *dockerclient.Client, script string, readWrite bool, timeout time.Duration) (*commandResult, error) {
	opCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := s.ensureUtilityImage(opCtx, cli); err != nil {
		return nil, err
	}

	mountMode := "ro"
	if readWrite {
		mountMode = "rw"
	}

	resp, err := cli.ContainerCreate(opCtx, dockerclient.ContainerCreateOptions{Config: &container.Config{
		Image:        utilityImage,
		Cmd:          []string{"sh", "-lc", script},
		Tty:          true,
		AttachStdout: true,
		AttachStderr: true,
		Labels:       map[string]string{coredocker.HelperLabel: "host-command"},
	}, HostConfig: &container.HostConfig{
		Binds:      []string{"/:/host:" + mountMode},
		PidMode:    "host",
		Privileged: readWrite,
	}})
	if err != nil {
		return nil, fmt.Errorf("creating host command container: %w", err)
	}
	defer s.removeContainer(ctx, cli, resp.ID)

	if _, err := cli.ContainerStart(opCtx, resp.ID, dockerclient.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("starting host command container: %w", err)
	}

	wait := cli.ContainerWait(opCtx, resp.ID, dockerclient.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	var exitCode int64
	select {
	case status := <-wait.Result:
		exitCode = status.StatusCode
	case err := <-wait.Error:
		if err != nil {
			return nil, fmt.Errorf("waiting for host command container: %w", err)
		}
	case <-opCtx.Done():
		return nil, fmt.Errorf("host command timed out: %w", opCtx.Err())
	}

	logReader, err := cli.ContainerLogs(opCtx, resp.ID, dockerclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("reading host command output: %w", err)
	}
	defer func() { closeutil.Report(logReader.Close(), "logReader") }()

	output, err := io.ReadAll(logReader)
	if err != nil {
		return nil, fmt.Errorf("reading host command stream: %w", err)
	}

	return &commandResult{Output: string(output), ExitCode: exitCode}, nil
}

func (s *Service) ensureUtilityImage(ctx context.Context, cli *dockerclient.Client) error {
	if _, err := cli.ImageInspect(ctx, utilityImage); err == nil {
		return nil
	}

	reader, err := cli.ImagePull(ctx, utilityImage, dockerclient.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pulling utility image: %w", err)
	}
	defer func() { closeutil.Report(reader.Close(), "reader") }()

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("draining utility image pull: %w", err)
	}
	return nil
}

// removeContainer removes the helper container even when ctx is already cancelled, so a dropped request never leaks a privileged container.
func (s *Service) removeContainer(parent context.Context, cli *dockerclient.Client, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), hostProbeTimeout)
	defer cancel()
	if _, err := cli.ContainerRemove(ctx, id, dockerclient.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
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
	// Discover what log files are actually present under /host/var/log.
	// emit_diag prints one MCHARBOR_NOTICE per candidate so the UI can
	// tell the user why a particular source failed (missing file,
	// permission denied, empty file).
	builder.WriteString("emit_diag() {\n")
	builder.WriteString("  echo \"MCHARBOR_NOTICE=$1\"\n")
	builder.WriteString("}\n")
	// List available log files (filtered by name) so the user can see
	// what the host actually exposes.
	builder.WriteString("list_available() {\n")
	builder.WriteString("  if [ -d /host/var/log ]; then\n")
	builder.WriteString("    for f in $(ls /host/var/log 2>/dev/null | grep -E '\\.(log|txt|out)$' | head -n 20); do\n")
	builder.WriteString("      echo \"MCHARBOR_NOTICE=available_file:$f\"\n")
	builder.WriteString("    done\n")
	builder.WriteString("  else\n")
	builder.WriteString("    emit_diag 'no_var_log_dir'\n")
	builder.WriteString("  fi\n")
	builder.WriteString("}\n")
	// Try the configured candidate files first.
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
		builder.WriteString("    emit_diag 'permission_denied'\n")
		builder.WriteString("  fi\n")
	}
	builder.WriteString("  return 1\n")
	builder.WriteString("}\n")
	// Dynamic fallback: pick the largest *.log file under /host/var/log
	// when the configured candidates are missing (common on minimal
	// images and some Docker Desktop VMs).
	builder.WriteString("read_largest_log() {\n")
	builder.WriteString("  candidate=$(ls -S /host/var/log/*.log 2>/dev/null | head -n 1)\n")
	builder.WriteString("  if [ -n \"$candidate\" ] && [ -r \"$candidate\" ]; then\n")
	builder.WriteString("    emit_diag \"largest_log_fallback:${candidate##*/}\"\n")
	builder.WriteString("    tail -n ")
	builder.WriteString(tailArg)
	builder.WriteString(" \"$candidate\"\n")
	builder.WriteString("    return 0\n")
	builder.WriteString("  fi\n")
	builder.WriteString("  return 1\n")
	builder.WriteString("}\n")
	// Try journalctl last — it's the most universally available log
	// source on systemd-based hosts.
	builder.WriteString("read_journal_logs() {\n")
	builder.WriteString("  if [ -x /host/bin/journalctl ] || [ -x /host/usr/bin/journalctl ]; then\n")
	builder.WriteString("    output=$(chroot /host sh -lc '")
	builder.WriteString(journalCommand)
	builder.WriteString("' 2>&1)\n")
	builder.WriteString("    status=$?\n")
	builder.WriteString("    if [ $status -ne 0 ]; then emit_diag 'journalctl_failed'; fi\n")
	builder.WriteString("    printf '%s\\n' \"$output\"\n")
	builder.WriteString("    return 0\n")
	builder.WriteString("  fi\n")
	builder.WriteString("  emit_diag 'no_journalctl'\n")
	builder.WriteString("  return 1\n")
	builder.WriteString("}\n")
	builder.WriteString("list_available\n")
	builder.WriteString("read_file_logs || read_largest_log || read_journal_logs\n")
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

// updateApplyScript upgrades the host through the utility container's /host
// chroot, recovering once from a failed download like the agent does.
func updateApplyScript(opts hostUpgradeOptions) string {
	return `chroot /host sh -lc '` + hostUpgradeBody(opts) + `'`
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

// splitLogMarkers separates real log content from the MCHARBOR_NOTICE markers
// the host script emits.
//
// Three kinds of marker come back and they are not equivalent:
//
//   - available_file:<name> is discovery output, printed on every run for
//     every log file found under /host/var/log. It reports success, not a
//     problem.
//   - largest_log_fallback:<name> names the file that was read when none of
//     the configured candidates existed. Informational.
//   - anything else is a warning the operator should act on.
//
// Flattening all three into one warning list is what made an ordinary
// successful read render a stack of yellow banners.
func splitLogMarkers(raw string) (lines []string, notices []string, availableFiles []string, fallbackFile string) {
	rawLines := outputLines(raw)
	lines = make([]string, 0, len(rawLines))
	notices = make([]string, 0)
	availableFiles = make([]string, 0)
	seenNotices := make(map[string]bool)
	seenFiles := make(map[string]bool)
	for _, line := range rawLines {
		if !strings.HasPrefix(line, "MCHARBOR_NOTICE=") {
			lines = append(lines, line)
			continue
		}
		notice := strings.TrimPrefix(line, "MCHARBOR_NOTICE=")
		switch {
		case strings.HasPrefix(notice, "available_file:"):
			name := strings.TrimPrefix(notice, "available_file:")
			if name != "" && !seenFiles[name] {
				availableFiles = append(availableFiles, name)
				seenFiles[name] = true
			}
		case strings.HasPrefix(notice, "largest_log_fallback:"):
			if fallbackFile == "" {
				fallbackFile = strings.TrimPrefix(notice, "largest_log_fallback:")
			}
		default:
			if !seenNotices[notice] {
				notices = append(notices, notice)
				seenNotices[notice] = true
			}
		}
	}
	return lines, notices, availableFiles, fallbackFile
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

// ErrRestartNotLocal is returned when RestartSelf is invoked while
// the McHarbor container cannot be reached via the local Docker
// socket (e.g. a remote agent is the registered environment).
var ErrRestartNotLocal = errors.New("mcharbor self-restart requires a local Docker socket")

// ErrRestartPending is returned when the current process already scheduled a restart.
var ErrRestartPending = errors.New("mcharbor self-restart is already pending")

// ErrResetNotLocal is returned when FactoryReset is invoked while
// the McHarbor container cannot be reached via the local Docker
// socket. The reset helper needs Docker to wipe `/app/data` and
// recreate the container, so it requires a local socket.
var ErrResetNotLocal = errors.New("mcharbor factory reset requires a local Docker socket")

// ErrResetConfirmMismatch is returned when the request body's
// `confirm` field does not equal the literal string "RESET".
var ErrResetConfirmMismatch = errors.New("factory reset confirmation string did not match")

// FactoryReset schedules a full wipe of the McHarbor data directory
// and an immediate self-restart. The HTTP response returns as soon as
// the helper container is queued; the running McHarbor process stops
// shortly after and is replaced by a fresh container. The next
// request therefore sees an empty database and the setup wizard.
//
// The caller must pass the literal string "RESET" in `confirmToken`
// to authorise the action. Mismatched tokens return
// ErrResetConfirmMismatch so the request fails before any side effect.
func (s *Service) FactoryReset(ctx context.Context, confirmToken string) (ResetResult, error) {
	if confirmToken != "RESET" {
		return ResetResult{}, ErrResetConfirmMismatch
	}

	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return ResetResult{}, fmt.Errorf("creating local docker client: %w", err)
	}
	defer func() { closeutil.Report(cli.Close(), "cli") }()

	inspectCtx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()

	current, err := coredocker.CurrentMcHarborContainer(inspectCtx, cli)
	if err != nil {
		// The factory reset relies on the local Docker socket. If we
		// cannot locate the McHarbor container locally we surface a
		// dedicated sentinel so the handler can map to a clear i18n
		// code.
		if !currentMcHarborContainerLikelyLocal(err) {
			return ResetResult{}, ErrResetNotLocal
		}
		return ResetResult{}, fmt.Errorf("inspecting current mcharbor container: %w", err)
	}

	name := ""
	if current.Name != "" {
		name = strings.TrimPrefix(current.Name, "/")
	}

	scheduled := time.Now().UTC()

	dataDir := strings.TrimSpace(os.Getenv("MCHARBOR_DATA_DIR"))
	if dataDir == "" {
		dataDir = apppaths.DefaultDataDir
	}

	scheduleCtx, cancelSchedule := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancelSchedule()

	if _, err := coredocker.ScheduleDetachedSelfUpdateHelperForImage(
		scheduleCtx, cli, current, dataDir, "", "factory_reset", current.Config.Image,
	); err != nil {
		return ResetResult{}, fmt.Errorf("scheduling factory reset helper: %w", err)
	}

	s.logger.Warn("mcharbor factory reset scheduled", "container", current.ID, "scheduledAt", scheduled)

	return ResetResult{
		ContainerID:   current.ID,
		ContainerName: name,
		ScheduledAt:   scheduled,
	}, nil
}

// currentMcHarborContainerLikelyLocal returns true when the error from
// CurrentMcHarborContainer looks like a transient lookup failure
// (Docker socket hiccup, list fallback failed) and false when the
// error suggests the McHarbor container is unreachable from this
// process — typically because the only registered environment is a
// remote agent. The distinction is best-effort; we mainly want to
// surface a clear error when the operator clicked Reset while
// pointed at an agent environment.
func currentMcHarborContainerLikelyLocal(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no running mcharbor container"):
		return false
	case strings.Contains(msg, "dial unix"):
		// Local socket issue (e.g. Docker not running). Treat as a
		// genuine lookup error, not a "not local" scenario, so the
		// operator sees a clear docker-unreachable error.
		return true
	case strings.Contains(msg, "permission denied"):
		return true
	default:
		return true
	}
}

// RestartSelf schedules a restart of the running McHarbor container
// through the local Docker socket. It returns ErrRestartNotLocal if
// no local Docker socket is reachable.
//
// The restart is fired in a background goroutine after a short delay
// so the HTTP response is delivered to the caller before the
// container is stopped. The Docker daemon handles the actual
// restart independently of this process: even when the McHarbor
// process is killed, the daemon will start the container again.
func (s *Service) RestartSelf(ctx context.Context) (RestartResult, error) {
	if !s.restartPending.CompareAndSwap(false, true) {
		return RestartResult{}, ErrRestartPending
	}
	if !restartInContainer() {
		return s.restartNative()
	}

	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		s.restartPending.Store(false)
		return RestartResult{}, fmt.Errorf("creating local docker client: %w", err)
	}

	inspectCtx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()

	current, err := coredocker.CurrentMcHarborContainer(inspectCtx, cli)
	if err != nil {
		s.restartPending.Store(false)
		if closeErr := cli.Close(); closeErr != nil {
			s.logger.Warn("system: closing docker client after inspect failure", "error", closeErr)
		}
		return RestartResult{}, fmt.Errorf("inspecting current mcharbor container: %w", err)
	}

	name := ""
	if current.Name != "" {
		name = strings.TrimPrefix(current.Name, "/")
	}

	scheduled := time.Now().UTC()

	// Brief delay gives the HTTP handler time to flush the
	// response and the API client time to surface the success
	// toast before the connection dies with the container stop.
	go func() {
		defer func() {
			if err := cli.Close(); err != nil {
				s.logger.Warn("system: closing self-restart docker client", "error", err)
			}
		}()
		bgCtx, bgCancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer bgCancel()

		delay := time.NewTimer(2 * time.Second)
		select {
		case <-delay.C:
		case <-bgCtx.Done():
			delay.Stop()
			s.restartPending.Store(false)
			return
		}

		timeout := 15
		if _, err := cli.ContainerRestart(bgCtx, current.ID, dockerclient.ContainerRestartOptions{Timeout: &timeout}); err != nil {
			s.restartPending.Store(false)
			s.logger.Error("system: mcharbor self-restart failed", "id", current.ID, "error", err)
		}
	}()

	return RestartResult{
		ContainerID:   current.ID,
		ContainerName: name,
		ScheduledAt:   scheduled,
	}, nil
}
