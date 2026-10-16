// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/compute/supervisor"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// handleHostLogRequest reads a bounded host-log snapshot and streams it back
// to the server as MsgHostLogChunk + MsgHostLogEnd messages. Sources supported:
// "system", "kernel", "auth", "docker".
func (a *Agent) handleHostLogRequest(ctx context.Context, conn *websocket.Conn, reqID string, payload HostLogRequestPayload) {
	if payload.Tail <= 0 {
		payload.Tail = 200
	}
	if payload.Tail > 1000 {
		payload.Tail = 1000
	}

	// An empty qemu target asks what there is to read: the guests this host runs.
	if payload.Source == "qemu" && payload.Target == "" {
		end := listQEMUTargets(ctx)
		writeMu.Lock()
		errlog.Log(conn.WriteJSON(WSMessage{Type: MsgHostLogEnd, ID: reqID, HostLogEnd: end}))
		writeMu.Unlock()
		return
	}

	lines, notices, available, err := readHostLog(ctx, payload.Source, payload.Target, payload.Tail)
	if err != nil {
		writeMu.Lock()
		errlog.Log(conn.WriteJSON(WSMessage{
			Type:       MsgHostLogEnd,
			ID:         reqID,
			HostLogEnd: &HostLogEndPayload{Notices: []string{"read_failed: " + err.Error()}},
		}))
		writeMu.Unlock()
		return
	}

	for _, line := range lines {
		select {
		case <-ctx.Done():
			return
		default:
		}
		writeMu.Lock()
		werr := conn.WriteJSON(WSMessage{
			Type:         MsgHostLogChunk,
			ID:           reqID,
			HostLogChunk: &HostLogChunkPayload{Line: line},
		})
		writeMu.Unlock()
		if werr != nil {
			return
		}
	}

	writeMu.Lock()
	errlog.Log(conn.WriteJSON(WSMessage{
		Type:       MsgHostLogEnd,
		ID:         reqID,
		HostLogEnd: &HostLogEndPayload{Notices: notices, AvailableFiles: available},
	}))
	writeMu.Unlock()
}

// readHostLog returns up to tail lines for the requested source, any non-fatal
// notices, and — for a target-less qemu request — the guests that have a log.
func readHostLog(ctx context.Context, source, target string, tail int) ([]string, []string, []string, error) {
	tailArg := fmt.Sprintf("%d", tail)
	var candidates []string
	var journalCmd string
	switch source {
	case "system":
		candidates = []string{"/var/log/syslog", "/var/log/messages"}
		journalCmd = "journalctl -n " + tailArg + " --no-pager"
	case "kernel":
		candidates = []string{"/var/log/kern.log", "/var/log/dmesg"}
		journalCmd = "journalctl -k -n " + tailArg + " --no-pager"
	case "auth":
		candidates = []string{"/var/log/auth.log", "/var/log/secure"}
		journalCmd = "journalctl -u ssh -u sshd -n " + tailArg + " --no-pager"
	case "docker":
		candidates = []string{"/var/log/docker.log"}
		journalCmd = "journalctl -u docker -n " + tailArg + " --no-pager"
	case "libvirt":
		candidates = []string{"/var/log/libvirt/libvirtd.log"}
		journalCmd = "journalctl -u libvirtd -n " + tailArg + " --no-pager"
	case "agent":
		journalCmd = "journalctl -u mcharbor-agent -n " + tailArg + " --no-pager"
	case "qemu":
		name, ok := safeLogTarget(target)
		if !ok {
			return nil, nil, nil, fmt.Errorf("invalid log target")
		}
		// McHarbor runs each guest as a systemd unit with QEMU's output in its
		// journal; the libvirt directory is only for a host that really has libvirt.
		if isGuestUnitTarget(name) {
			journalCmd = "journalctl -u " + supervisor.UnitPrefix + name + ".service -n " + tailArg + " --no-pager"
			break
		}
		candidates = []string{filepath.Join(qemuLogDir, name+".log")}
	default:
		return nil, nil, nil, fmt.Errorf("invalid log source")
	}

	// fileNotices survives a miss: a candidate that exists but cannot be read
	// is worth reporting even when a later source succeeds.
	lines, fileNotices, ok := readFileLogs(candidates, tail)
	if ok {
		return lines, fileNotices, nil, nil
	}

	// A source with no journal unit — a guest log — has nowhere else to look,
	// and falling through would run an empty command.
	if journalCmd != "" {
		if lines, notices, ok := readJournalLogs(ctx, journalCmd, tail); ok {
			return lines, append(fileNotices, notices...), nil, nil
		}
	}

	return nil, append(fileNotices, "no_supported_log_source"), nil, fmt.Errorf("no log source available")
}

// qemuLogDir is where libvirt keeps one log per domain.
const qemuLogDir = "/var/log/libvirt/qemu"

// safeLogTarget accepts only a bare guest name, so a crafted target cannot walk
// out of qemuLogDir into any other file the agent is allowed to read.
func safeLogTarget(target string) (string, bool) {
	if target == "" || len(target) > 128 || target == "." || target == ".." {
		return "", false
	}
	for _, r := range target {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return "", false
		}
	}
	if path.Dir(path.Join(qemuLogDir, target+".log")) != qemuLogDir {
		return "", false
	}
	return target, true
}

// listQEMUTargets answers a target-less qemu request with McHarbor's own guest
// units, named from their unit descriptions, plus any libvirt logs the host has.
func listQEMUTargets(ctx context.Context) *HostLogEndPayload {
	end := &HostLogEndPayload{Targets: listGuestUnits(ctx)}
	if files, err := listQEMULogs(); err == nil {
		end.AvailableFiles = files
	}
	if len(end.Targets) == 0 && len(end.AvailableFiles) == 0 {
		end.Notices = []string{"no_guests"}
	}
	return end
}

// guestUnitKinds maps a unit stem's prefix to the description prefix the agent
// started it with, so the label can be the bare guest name.
var guestUnitKinds = map[string]string{"vm-": "McHarbor VM ", "ct-": "McHarbor CT "}

// isGuestUnitTarget reports whether target names one of McHarbor's guest units
// (vm-<id> or ct-<id>) rather than a libvirt log file.
func isGuestUnitTarget(target string) bool {
	for prefix := range guestUnitKinds {
		if strings.HasPrefix(target, prefix) && len(target) > len(prefix) {
			return true
		}
	}
	return false
}

// listGuestUnits lists McHarbor's VM and container units, running or not, so a
// guest that crashed on start still offers the log that says why.
func listGuestUnits(ctx context.Context) []HostLogTarget {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "systemctl", "list-units", "--all", "--plain", "--no-legend", "--full",
		supervisor.UnitPrefix+"vm-*.service", supervisor.UnitPrefix+"ct-*.service")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseGuestUnits(string(out))
}

// parseGuestUnits reads `systemctl list-units --plain --no-legend` output:
// UNIT LOAD ACTIVE SUB DESCRIPTION, where the description may contain spaces.
func parseGuestUnits(out string) []HostLogTarget {
	var targets []HostLogTarget
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimPrefix(fields[0], supervisor.UnitPrefix), ".service")
		name, ok := safeLogTarget(stem)
		if !ok || !isGuestUnitTarget(name) {
			continue
		}
		label := strings.TrimSpace(strings.Join(fields[4:], " "))
		for prefix, desc := range guestUnitKinds {
			if strings.HasPrefix(name, prefix) {
				label = strings.TrimPrefix(label, desc)
			}
		}
		if label == "" {
			label = name
		}
		targets = append(targets, HostLogTarget{ID: name, Label: label})
	}
	return targets
}

// listQEMULogs names the guests this host has a libvirt log for.
func listQEMULogs() ([]string, error) {
	entries, err := os.ReadDir(qemuLogDir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		files = append(files, entry.Name())
	}
	return files, nil
}

// readFileLogs tries the given log files in order, returning the first
// readable one's tail plus notices for candidates that exist but could not be
// opened.
//
// Those notices used to be collected into a local slice and dropped on the
// floor, so an unreadable /var/log/auth.log reached the UI as an empty log
// with no explanation at all.
func readFileLogs(files []string, tail int) ([]string, []string, bool) {
	notices := make([]string, 0)
	seen := make(map[string]bool)
	addNotice := func(code string) {
		if seen[code] {
			return
		}
		notices = append(notices, code)
		seen[code] = true
	}

	for _, logPath := range files {
		info, err := os.Stat(logPath)
		if err != nil || info.IsDir() {
			continue
		}
		lines, err := readTailFile(logPath, tail)
		if err != nil {
			if os.IsPermission(err) {
				addNotice("permission_denied")
			}
			continue
		}
		return lines, notices, true
	}
	return nil, notices, false
}

// tailChunkSize is how much is pulled back from the end of a log file per
// round. Large enough that a 200-line tail is usually one read.
const tailChunkSize = 64 * 1024

// tailReadCap bounds how far back readTailFile will walk. A file whose lines
// are pathologically long stops here rather than loading itself into memory.
const tailReadCap = 32 * 1024 * 1024

// readTailFile reads the last tail lines of one file by seeking backwards from
// the end, so a 200-line request off a 140 MB syslog touches 64 KiB and not the
// whole file, which used to blow the server's 30s call budget on a cold cache.
func readTailFile(path string, tail int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer errlog.Close(file)

	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}

	var window []byte
	var read int64
	for read < size && read < tailReadCap {
		step := int64(tailChunkSize)
		if remaining := size - read; remaining < step {
			step = remaining
		}
		buf := make([]byte, step)
		if _, err := file.ReadAt(buf, size-read-step); err != nil {
			return nil, err
		}
		window = append(buf, window...)
		read += step
		// One extra newline so a window starting mid-line drops that fragment
		// rather than reporting it as a whole line.
		if bytes.Count(window, []byte{'\n'}) > tail {
			break
		}
	}

	lines := strings.Split(strings.TrimRight(string(window), "\n"), "\n")
	if read < size && len(lines) > 0 {
		lines = lines[1:]
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

// readJournalLogs runs journalctl and returns its tail. On failure it
// returns journalctl_failed notice so the UI can warn the user.
func readJournalLogs(ctx context.Context, journalCommand string, tail int) ([]string, []string, bool) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, []string{"no_supported_log_source"}, false
	}
	cmd := exec.CommandContext(ctx, "sh", "-lc", journalCommand)
	out, err := cmd.Output()
	if err != nil {
		// journalctl may exit non-zero if no entries; treat as failure.
		notices := []string{"journalctl_failed"}
		if len(out) > 0 {
			lines := tailLines(string(out), tail)
			return lines, notices, true
		}
		return nil, notices, false
	}
	return tailLines(string(out), tail), nil, true
}

func tailLines(s string, tail int) []string {
	normalized := strings.ReplaceAll(s, "\r\n", "\n")
	raw := strings.Split(strings.TrimRight(normalized, "\n"), "\n")
	if len(raw) > tail {
		raw = raw[len(raw)-tail:]
	}
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, strings.TrimRight(line, "\r"))
	}
	return lines
}
