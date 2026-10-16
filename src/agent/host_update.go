// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// hostUpdateCheckTimeout bounds a package-list refresh. A repository that is
// unreachable makes apt sit on its own 30s-per-mirror timeout, so this is
// generous rather than tight.
const hostUpdateCheckTimeout = 5 * time.Minute

// hostUpdateApplyTimeout bounds a full upgrade. A kernel and initramfs rebuild
// on a slow disk genuinely takes this long.
const hostUpdateApplyTimeout = 2 * time.Hour

// hostChrootPrefix returns the command prefix that reaches the real host.
//
// A native agent already runs there, so the prefix is empty; a containerised
// one needs the host filesystem bind-mounted at /host, and without it there is
// no host to update and the caller is told so rather than upgrading the
// container it happens to live in.
func hostChrootPrefix() (string, error) {
	if _, err := os.Stat("/etc/os-release"); err == nil && !runningInContainer() {
		return "", nil
	}
	if _, err := os.Stat("/host/etc/os-release"); err == nil {
		return "chroot /host ", nil
	}
	return "", fmt.Errorf("host OS updates need an agent installed natively on the host, or /host mounted")
}

// runningInContainer reports whether this process is inside a container, which
// decides whether the agent's own filesystem is the host's.
func runningInContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	body := string(data)
	return strings.Contains(body, "docker") || strings.Contains(body, "containerd") ||
		strings.Contains(body, "kubepods")
}

// handleHostOSInfoRequest answers with a description of the host operating
// system, or with an Error field the server surfaces verbatim to the operator.
func (a *Agent) handleHostOSInfoRequest(ctx context.Context, conn *websocket.Conn, reqID string) {
	payload := readHostOSInfo(ctx)
	writeMu.Lock()
	errlog.Log(conn.WriteJSON(WSMessage{
		Type:       MsgHostOSInfoResponse,
		ID:         reqID,
		HostOSInfo: payload,
	}))
	writeMu.Unlock()
}

// readHostOSInfo shells out once and parses the KEY=VALUE block back, which is
// cheaper and far less brittle than eight separate probes.
func readHostOSInfo(ctx context.Context) *HostOSInfoPayload {
	prefix, err := hostChrootPrefix()
	if err != nil {
		return &HostOSInfoPayload{Error: err.Error()}
	}

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(runCtx, "sh", "-c", prefix+hostOSInfoScript).Output()
	if err != nil && len(out) == 0 {
		return &HostOSInfoPayload{Error: "reading host OS information failed"}
	}

	info := &HostOSInfoPayload{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "MCHARBOR_PRETTY":
			info.PrettyName = value
		case "MCHARBOR_DISTRO":
			info.Distro = value
		case "MCHARBOR_VERSION":
			info.Version = value
		case "MCHARBOR_KERNEL":
			info.Kernel = value
		case "MCHARBOR_ARCH":
			info.Arch = value
		case "MCHARBOR_HOSTNAME":
			info.Hostname = value
		case "MCHARBOR_UPTIME":
			seconds, convErr := strconv.ParseFloat(value, 64)
			if convErr == nil {
				info.UptimeSeconds = int64(seconds)
			}
		case "MCHARBOR_MANAGER":
			info.Manager = value
		case "MCHARBOR_VIRT":
			info.Virtualization = value
		case "MCHARBOR_REBOOT":
			info.RebootRequired = value == "1"
		}
	}
	return info
}

// handleHostUpdateCheck lists the packages the host could upgrade right now.
func (a *Agent) handleHostUpdateCheck(ctx context.Context, conn *websocket.Conn, reqID string) {
	payload := &HostUpdatePayload{}

	prefix, err := hostChrootPrefix()
	if err != nil {
		payload.Error = err.Error()
	} else {
		runCtx, cancel := context.WithTimeout(ctx, hostUpdateCheckTimeout)
		defer cancel()

		out, runErr := exec.CommandContext(runCtx, "sh", "-c", prefix+hostUpdateCheckScript).Output()
		manager, body := splitManagerMarker(string(out))
		updates := nonEmptyLines(body)
		payload.Manager = manager
		payload.Updates = updates
		payload.Available = len(updates) > 0
		payload.Output = body
		payload.Success = runErr == nil
		if runErr != nil && len(out) == 0 {
			payload.Error = "the package manager could not be queried"
		}
	}

	writeMu.Lock()
	errlog.Log(conn.WriteJSON(WSMessage{
		Type:       MsgHostUpdateResult,
		ID:         reqID,
		HostUpdate: payload,
	}))
	writeMu.Unlock()
}

// handleHostUpdateApply runs the upgrade and streams its output line by line,
// because an operator watching a ten-minute dist-upgrade needs to see it move.
func (a *Agent) handleHostUpdateApply(ctx context.Context, conn *websocket.Conn, reqID string, request *HostUpdatePayload) {
	sendEnd := func(payload *HostUpdatePayload) {
		writeMu.Lock()
		errlog.Log(conn.WriteJSON(WSMessage{
			Type:       MsgHostUpdateEnd,
			ID:         reqID,
			HostUpdate: payload,
		}))
		writeMu.Unlock()
	}

	prefix, err := hostChrootPrefix()
	if err != nil {
		sendEnd(&HostUpdatePayload{Error: err.Error()})
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, hostUpdateApplyTimeout)
	defer cancel()

	opts := hostUpgradeOptions{}
	if request != nil {
		opts.WithNewPackages = request.WithNewPackages
		opts.SkipUnavailable = request.SkipUnavailable
		opts.RemoveUnused = request.RemoveUnused
	}
	cmd := exec.CommandContext(runCtx, "sh", "-c", prefix+hostUpdateApplyScript(opts))
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		sendEnd(&HostUpdatePayload{Error: "the upgrade could not be started"})
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		sendEnd(&HostUpdatePayload{Error: "the upgrade could not be started"})
		return
	}

	var manager string
	var collected strings.Builder
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, managerMarker); ok {
			manager = strings.TrimSpace(name)
			continue
		}
		collected.WriteString(line)
		collected.WriteString("\n")
		writeMu.Lock()
		werr := conn.WriteJSON(WSMessage{
			Type:            MsgHostUpdateChunk,
			ID:              reqID,
			HostUpdateChunk: &HostUpdateChunkPayload{Line: line},
		})
		writeMu.Unlock()
		if werr != nil {
			errlog.Log(cmd.Process.Kill())
			return
		}
	}

	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	sendEnd(&HostUpdatePayload{
		Manager:  manager,
		Output:   collected.String(),
		ExitCode: exitCode,
		Success:  exitCode == 0,
	})
}

// managerMarker prefixes the one line every host script emits naming the
// package manager it chose.
const managerMarker = "MCHARBOR_MANAGER="

// splitManagerMarker peels the manager marker off a script's output.
func splitManagerMarker(output string) (string, string) {
	lines := strings.Split(output, "\n")
	manager := ""
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), managerMarker); ok {
			manager = strings.TrimSpace(name)
			continue
		}
		kept = append(kept, line)
	}
	return manager, strings.Join(kept, "\n")
}

// nonEmptyLines trims a script's output into the lines worth showing.
func nonEmptyLines(output string) []string {
	raw := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// hostOSInfoScript emits one KEY=VALUE line per fact. Every probe is guarded so
// a missing tool yields an empty value instead of failing the whole read.
const hostOSInfoScript = `sh -lc '
. /etc/os-release 2>/dev/null
echo MCHARBOR_PRETTY="$PRETTY_NAME"
echo MCHARBOR_DISTRO="$ID"
echo MCHARBOR_VERSION="$VERSION_ID"
echo MCHARBOR_KERNEL="$(uname -r 2>/dev/null)"
echo MCHARBOR_ARCH="$(uname -m 2>/dev/null)"
echo MCHARBOR_HOSTNAME="$(hostname 2>/dev/null)"
echo MCHARBOR_UPTIME="$(cut -d" " -f1 /proc/uptime 2>/dev/null)"
echo MCHARBOR_VIRT="$(systemd-detect-virt 2>/dev/null || echo none)"
if command -v apt-get >/dev/null 2>&1; then echo MCHARBOR_MANAGER=apt
elif command -v dnf >/dev/null 2>&1; then echo MCHARBOR_MANAGER=dnf
elif command -v yum >/dev/null 2>&1; then echo MCHARBOR_MANAGER=yum
elif command -v apk >/dev/null 2>&1; then echo MCHARBOR_MANAGER=apk
elif command -v pacman >/dev/null 2>&1; then echo MCHARBOR_MANAGER=pacman
elif command -v zypper >/dev/null 2>&1; then echo MCHARBOR_MANAGER=zypper
else echo MCHARBOR_MANAGER=unknown
fi
if [ -f /var/run/reboot-required ] || [ -f /run/reboot-required ]; then echo MCHARBOR_REBOOT=1
elif command -v needs-restarting >/dev/null 2>&1 && ! needs-restarting -r >/dev/null 2>&1; then echo MCHARBOR_REBOOT=1
else echo MCHARBOR_REBOOT=0
fi
'`

// hostUpdateCheckScript lists upgradable packages, one per line, after the
// manager marker. It mirrors the container-side script in modules/system.
const hostUpdateCheckScript = `sh -lc '
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

// hostUpdateApplyScript upgrades every package the host offers, without
// prompting, recovering once from a failed download. Output is unbuffered line
// by line so the stream stays live.
func hostUpdateApplyScript(opts hostUpgradeOptions) string {
	return `sh -lc '` + hostUpgradeBody(opts) + `' 2>&1`
}
