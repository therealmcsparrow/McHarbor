// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gorilla/websocket"
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

	lines, notices, err := readHostLog(ctx, payload.Source, payload.Tail)
	if err != nil {
		writeMu.Lock()
		_ = conn.WriteJSON(WSMessage{
			Type:        MsgHostLogEnd,
			ID:          reqID,
			HostLogEnd:  &HostLogEndPayload{Notices: []string{"read_failed: " + err.Error()}},
		})
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
	_ = conn.WriteJSON(WSMessage{
		Type:        MsgHostLogEnd,
		ID:          reqID,
		HostLogEnd:  &HostLogEndPayload{Notices: notices},
	})
	writeMu.Unlock()
}

// readHostLog returns up to tail lines for the requested source plus any
// non-fatal notices (e.g. permission denied, journalctl missing).
func readHostLog(ctx context.Context, source string, tail int) ([]string, []string, error) {
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
	default:
		return nil, nil, fmt.Errorf("invalid log source")
	}

	if lines, ok := readFileLogs(candidates, tail); ok {
		return lines, nil, nil
	}

	if lines, notices, ok := readJournalLogs(ctx, journalCmd, tail); ok {
		return lines, notices, nil
	}

	return nil, []string{"no_supported_log_source"}, fmt.Errorf("no log source available")
}

// readFileLogs tries the given log files in order, returning the first
// readable one's tail. Reports permission_denied notices for unreadable files.
func readFileLogs(files []string, tail int) ([]string, bool) {
	var permissionDenied []string
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			if os.IsPermission(err) {
				permissionDenied = append(permissionDenied, "permission_denied")
			}
			continue
		}
		defer file.Close()
		var lines []string
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err == nil {
			if len(lines) > tail {
				lines = lines[len(lines)-tail:]
			}
			return lines, true
		}
	}
	return nil, false
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