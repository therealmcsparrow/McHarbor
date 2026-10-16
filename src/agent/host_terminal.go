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
	"regexp"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
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

// hostUserPattern bounds the account name the server may ask us to become.
// The server allowlists it too; this is the second half of that check,
// because a compromised or simply older server must not be able to turn a
// user name into extra arguments for runuser or su.
var hostUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// handleHostTerminalStart spawns a shell under a PTY and pipes its
// stdout/stderr to the WebSocket until the session ends.
//
// Shell, User and WorkingDir are honoured when the payload carries them and
// ignored when it does not, so an older server talking to this agent gets
// exactly the previous behaviour.
func (a *Agent) handleHostTerminalStart(ctx context.Context, conn *websocket.Conn, reqID string, payload HostTerminalStartPayload) {
	shellPath, err := resolveHostShell(payload.Shell)
	if err != nil {
		a.sendHostTerminalError(ctx, conn, reqID, err)
		return
	}

	id := "host-term-" + randHex(8)

	cmd, err := buildHostShellCommand(ctx, shellPath, payload.User)
	if err != nil {
		a.sendHostTerminalError(ctx, conn, reqID, err)
		return
	}
	if payload.WorkingDir != "" {
		// An unusable directory would make exec fail with a message the
		// operator cannot act on, so check it here and say which path.
		info, statErr := os.Stat(payload.WorkingDir)
		if statErr != nil || !info.IsDir() {
			a.sendHostTerminalError(ctx, conn, reqID,
				fmt.Errorf("working directory %s is not a directory on this host", payload.WorkingDir))
			return
		}
		cmd.Dir = payload.WorkingDir
	}
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		// The 24-bit hint, to match the two Docker-backed paths.
		"COLORTERM=truecolor",
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
		errlog.Close(sess.pty)
		if sess.cmd.Process != nil {
			errlog.Log(sess.cmd.Process.Kill())
		}
		errlog.Log(sess.cmd.Wait())
		writeMu.Lock()
		errlog.Log(conn.WriteJSON(WSMessage{Type: MsgHostTerminalEnd, ID: reqID}))
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
// resolveHostShell returns the shell to start. A requested path is used only
// when it exists and is executable - falling back to detection rather than
// failing, because "the host does not have zsh" is not worth refusing a
// session over, and detectShell is never wrong.
func resolveHostShell(requested string) (string, error) {
	if requested != "" {
		if info, err := os.Stat(requested); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return requested, nil
		}
	}
	return detectShell()
}

// buildHostShellCommand builds the command that starts the shell, switching
// user when one was requested.
//
// runuser is preferred over su: it is the util-linux tool meant for exactly
// this, skips PAM's password prompt when the caller is root, and takes the
// command after a -- separator so a user name can never become an option.
// su is the fallback for hosts without util-linux (BusyBox, mainly).
func buildHostShellCommand(ctx context.Context, shellPath, user string) (*exec.Cmd, error) {
	// The shell outlives the request that opened it and is torn down by the
	// session, so the command gets the request values but not its cancellation.
	ctx = context.WithoutCancel(ctx)
	if user == "" {
		return exec.CommandContext(ctx, shellPath), nil
	}
	if !hostUserPattern.MatchString(user) {
		return nil, fmt.Errorf("invalid host user name")
	}
	if runuser, err := exec.LookPath("runuser"); err == nil {
		return exec.CommandContext(ctx, runuser, "-u", user, "--", shellPath), nil
	}
	su, err := exec.LookPath("su")
	if err != nil {
		return nil, fmt.Errorf("cannot switch to user %s: neither runuser nor su is available on this host", user)
	}
	// Deliberately not `su -`: a login shell would cd to the account's home
	// and discard the working directory the operator asked for.
	return exec.CommandContext(ctx, su, "-s", shellPath, user), nil
}

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
	errlog.Log(conn.WriteJSON(WSMessage{
		Type:        MsgHostTerminalOutput,
		ID:          reqID,
		StreamChunk: &WSStreamChunk{Data: []byte("MCHARBOR_ERROR=" + err.Error() + "\n")},
	}))
	errlog.Log(conn.WriteJSON(WSMessage{Type: MsgHostTerminalEnd, ID: reqID}))
}
