// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// hostTerminalSession tracks a live PTY-backed shell on the agent host.
type hostTerminalSession struct {
	id     string
	cmd    *exec.Cmd
	pty    *os.File
	cancel context.CancelFunc
}

// execIDHeaderPrefix is the ASCII header that prefixes stdin chunks sent
// through MsgHostTerminalInput so the agent can demultiplex stdin from
// multiple concurrent host terminal sessions.
const execIDHeaderPrefix = "MCHARBOR_EXEC_ID="

// parseExecIDFromHeader extracts the exec id from a MsgHostTerminalInput
// chunk header. Returns "" when the header is missing.
func parseExecIDFromHeader(data []byte) string {
	if !bytes.HasPrefix(data, []byte(execIDHeaderPrefix)) {
		return ""
	}
	rest := data[len(execIDHeaderPrefix):]
	for i, b := range rest {
		if b == '\n' || b == '\r' || b == 0 {
			return string(rest[:i])
		}
	}
	return string(rest)
}

// buildHostTerminalInputHeader returns the byte prefix that the server
// must prepend to each stdin chunk so the agent can route it to the right
// PTY session.
func buildHostTerminalInputHeader(execID string) []byte {
	return []byte(execIDHeaderPrefix + execID + "\n")
}

// randHex returns a short random hex string used for unique exec ids.
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a deterministic but unique-enough value.
		return fmt.Sprintf("%x", os.Getpid())
	}
	return hex.EncodeToString(b)
}

// hostTerminalRegistry keeps live PTY-backed shells keyed by exec id.
type hostTerminalRegistry struct {
	mu       sync.Mutex
	sessions map[string]*hostTerminalSession
}

var hostTerminals = &hostTerminalRegistry{sessions: make(map[string]*hostTerminalSession)}

// handleHostTerminalStart spawns /bin/sh (or bash if available) under a PTY
// and pipes its stdout/stderr to the WebSocket until the session ends.
func (a *Agent) handleHostTerminalStart(ctx context.Context, conn *websocket.Conn, reqID string, payload HostTerminalStartPayload) {
	shellPath, err := detectShell()
	if err != nil {
		a.sendHostTerminalError(ctx, conn, reqID, err)
		return
	}

	id := "host-term-" + randHex(8)

	cmd := exec.Command(shellPath)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"MCHARBOR_HOST_TERMINAL=1",
	)
	winSize := pty.Winsize{Rows: 24, Cols: 80}
	if payload.Rows > 0 {
		winSize.Rows = uint16(payload.Rows)
	}
	if payload.Cols > 0 {
		winSize.Cols = uint16(payload.Cols)
	}
	pt, err := pty.StartWithSize(cmd, &winSize)
	if err != nil {
		a.sendHostTerminalError(ctx, conn, reqID, fmt.Errorf("starting pty: %w", err))
		return
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	sess := &hostTerminalSession{
		id:     id,
		cmd:    cmd,
		pty:    pt,
		cancel: cancel,
	}
	hostTerminals.mu.Lock()
	hostTerminals.sessions[id] = sess
	hostTerminals.mu.Unlock()

	// Reply with the assigned exec id (using the StreamChunk field as a
	// generic data channel for the id so we don't add another envelope
	// field just for this).
	writeMu.Lock()
	startErr := conn.WriteJSON(WSMessage{
		Type:        MsgHostTerminalOutput,
		ID:          reqID,
		StreamChunk: &WSStreamChunk{Data: buildHostTerminalInputHeader(id)},
	})
	writeMu.Unlock()
	if startErr != nil {
		cancel()
		return
	}

	go a.pumpHostTerminal(sessionCtx, conn, reqID, sess)
}

// pumpHostTerminal forwards PTY output to the server until the process
// exits or the session is cancelled.
func (a *Agent) pumpHostTerminal(ctx context.Context, conn *websocket.Conn, reqID string, sess *hostTerminalSession) {
	defer func() {
		hostTerminals.mu.Lock()
		delete(hostTerminals.sessions, sess.id)
		hostTerminals.mu.Unlock()
		_ = sess.pty.Close()
		if sess.cmd.Process != nil {
			_ = sess.cmd.Process.Kill()
		}
		_ = sess.cmd.Wait()
		writeMu.Lock()
		_ = conn.WriteJSON(WSMessage{Type: MsgHostTerminalEnd, ID: reqID})
		writeMu.Unlock()
	}()

	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := sess.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			writeMu.Lock()
			werr := conn.WriteJSON(WSMessage{
				Type:        MsgHostTerminalOutput,
				ID:          reqID,
				StreamChunk: &WSStreamChunk{Data: chunk},
			})
			writeMu.Unlock()
			if werr != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				a.logger.Debug("host terminal read ended", "id", sess.id, "error", err)
			}
			return
		}
	}
}

// writeHostTerminalInput feeds stdin into an active PTY-backed shell.
func writeHostTerminalInput(id string, data []byte) error {
	hostTerminals.mu.Lock()
	sess, ok := hostTerminals.sessions[id]
	hostTerminals.mu.Unlock()
	if !ok {
		return fmt.Errorf("host terminal session %q not found", id)
	}
	_, err := sess.pty.Write(data)
	return err
}

// resizeHostTerminal updates the TTY size of an active shell session.
func resizeHostTerminal(id string, cols, rows uint) error {
	hostTerminals.mu.Lock()
	sess, ok := hostTerminals.sessions[id]
	hostTerminals.mu.Unlock()
	if !ok {
		return fmt.Errorf("host terminal session %q not found", id)
	}
	ws := pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}
	if ws.Rows == 0 {
		ws.Rows = 24
	}
	if ws.Cols == 0 {
		ws.Cols = 80
	}
	return pty.Setsize(sess.pty, &ws)
}

// closeHostTerminal ends a PTY-backed shell session.
func closeHostTerminal(id string) {
	hostTerminals.mu.Lock()
	sess, ok := hostTerminals.sessions[id]
	if ok {
		delete(hostTerminals.sessions, id)
	}
	hostTerminals.mu.Unlock()
	if !ok {
		return
	}
	sess.cancel()
}

// detectShell picks a shell binary that exists on the agent host.
func detectShell() (string, error) {
	for _, candidate := range []string{"/bin/bash", "/usr/bin/bash", "/bin/sh", "/usr/bin/sh"} {
		if info, err := os.Stat(candidate); err == nil && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no usable shell binary found on host")
}

// sendHostTerminalError reports a startup failure to the server.
func (a *Agent) sendHostTerminalError(ctx context.Context, conn *websocket.Conn, reqID string, err error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	_ = conn.WriteJSON(WSMessage{
		Type:        MsgHostTerminalOutput,
		ID:          reqID,
		StreamChunk: &WSStreamChunk{Data: []byte("MCHARBOR_ERROR=" + err.Error() + "\n")},
	})
	_ = conn.WriteJSON(WSMessage{Type: MsgHostTerminalEnd, ID: reqID})
}