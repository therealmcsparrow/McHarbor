// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/rs/xid"
)

// Guest console streaming.
//
// # The ordering hazard, stated plainly
//
// This repo has already been bitten once by exactly this bug. In the HTTP
// proxy path, `ReadLoop` dispatched `http_response_start` before the
// StreamReader existed, so chunks that arrived first were silently dropped and
// short responses hung forever. The fix — create the reader before dispatching —
// is recorded in the project guide as a thing not to regress.
//
// The console path has the same shape and a worse failure. A VNC connection
// begins with the RFB handshake, which the *server* sends first. Lose those
// bytes and the client waits for a banner that already went past: no error, no
// timeout, just a console that never paints. A text console loses the login
// prompt instead, which at least looks broken.
//
// So StartConsole registers the session before it writes the start request, and
// the registration is not an optimisation that could be moved later. The order
// below is the contract.

// consoleBuffer is how many output frames may queue for a viewer.
//
// A VNC framebuffer update can be large and bursty, so this is deeper than the
// exec path's. Beyond it, frames are dropped rather than blocking the read
// loop — one slow browser must not stall every other guest on the host.
const consoleBuffer = 256

// Console kinds.
const (
	ConsoleKindVNC    = "vnc"
	ConsoleKindSPICE  = "spice"
	ConsoleKindSerial = "serial"
	ConsoleKindTTY    = "tty"
	ConsoleKindExec   = "exec"
)

// ConsoleSession is one live guest console.
type ConsoleSession struct {
	ID       string
	Kind     string
	OutputCh chan []byte

	mu       sync.Mutex
	doneCh   chan struct{}
	closed   bool
	errValue string

	// write sends a frame back to the agent. It is the transport's WriteJSON
	// behind a closure so the session does not hold a transport reference and
	// cannot outlive one meaningfully.
	write func(WSMessage) error
}

// Done is closed when the session ends, from either side.
func (s *ConsoleSession) Done() <-chan struct{} { return s.doneCh }

// Err returns why the session ended, empty when it ended cleanly.
func (s *ConsoleSession) Err() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errValue
}

func (s *ConsoleSession) setError(message string) {
	s.mu.Lock()
	if s.errValue == "" {
		s.errValue = message
	}
	s.mu.Unlock()
}

// close ends the session exactly once.
func (s *ConsoleSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.doneCh)
}

// Send forwards client bytes to the guest.
//
// For VNC this is the RFB client stream; for a text console it is keystrokes.
// Either way the bytes pass through unmodified — the console is a byte pipe,
// and interpreting its contents here would break both.
func (s *ConsoleSession) Send(data []byte) error {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return fmt.Errorf("console session %s has ended", s.ID)
	}
	return s.write(WSMessage{
		Type:    MsgConsoleInput,
		ID:      s.ID,
		Console: &ConsolePayload{SessionID: s.ID, Data: data},
	})
}

// Resize tells a text console its new dimensions. Graphical consoles negotiate
// their own size in-band, so this is a no-op for them.
func (s *ConsoleSession) Resize(cols, rows int) error {
	if s.Kind == ConsoleKindVNC || s.Kind == ConsoleKindSPICE {
		return nil
	}
	return s.write(WSMessage{
		Type:    MsgConsoleResize,
		ID:      s.ID,
		Console: &ConsolePayload{SessionID: s.ID, Cols: cols, Rows: rows},
	})
}

// Close ends the session and tells the agent to release the guest side.
func (s *ConsoleSession) Close() error {
	s.mu.Lock()
	alreadyClosed := s.closed
	s.mu.Unlock()

	if !alreadyClosed {
		// Best-effort: if the transport is already gone the agent will notice
		// the disconnect and clean up on its own.
		if err := s.write(WSMessage{
			Type:    MsgConsoleEnd,
			ID:      s.ID,
			Console: &ConsolePayload{SessionID: s.ID},
		}); err != nil {
			slog.Debug("console end message not delivered", "session", s.ID, "error", err)
		}
	}
	s.close()
	return nil
}

// StartConsole opens a guest console session.
//
// Read the ordering comments below before changing anything in this function.
func (t *AgentTransport) StartConsole(
	ctx context.Context,
	kind, guestKind, targetID string,
	command []string,
	cols, rows int,
) (*ConsoleSession, error) {
	if !t.ComputeSupported() {
		return nil, fmt.Errorf("%w: this agent is %s and compute needs %s or newer",
			ErrComputeUnsupported, t.conn.Version, ComputeMinVersion)
	}

	sessionID := xid.New().String()
	session := &ConsoleSession{
		ID:       sessionID,
		Kind:     kind,
		OutputCh: make(chan []byte, consoleBuffer),
		doneCh:   make(chan struct{}),
		write:    func(msg WSMessage) error { return t.conn.WriteJSON(msg) },
	}

	// STEP 1 — register the session.
	//
	// This must happen before the request is written. The agent may answer
	// with console_output before its reply to the start request is even
	// serialized, and for VNC it certainly will: QEMU's RFB server sends its
	// version banner the instant the socket is accepted. With no session
	// registered, ReadLoop finds no destination for that frame and drops it,
	// and the client then waits forever for a handshake that already
	// happened. This is the same bug the HTTP proxy path had with
	// http_response_start.
	t.mu.Lock()
	t.consoleSessions[sessionID] = session
	t.mu.Unlock()

	// STEP 2 — only now, ask the agent to open the guest side.
	if err := t.conn.WriteJSON(WSMessage{
		Type: MsgConsoleStart,
		ID:   sessionID,
		Console: &ConsolePayload{
			SessionID: sessionID,
			Kind:      kind,
			GuestKind: guestKind,
			TargetID:  targetID,
			Command:   command,
			Cols:      cols,
			Rows:      rows,
		},
	}); err != nil {
		// Unregister on the failure path so a session nobody can reach does
		// not sit in the map until the transport dies.
		t.mu.Lock()
		delete(t.consoleSessions, sessionID)
		t.mu.Unlock()
		return nil, fmt.Errorf("opening guest console: %w", err)
	}

	// The caller's context governs the session's lifetime. When it ends — the
	// browser disconnected, the request was cancelled — the guest side is
	// released too. Every goroutine gets a stop path.
	go func() {
		select {
		case <-ctx.Done():
			if err := session.Close(); err != nil {
				t.logger.Debug("closing console session after context end failed", "session", session.ID, "error", err)
			}
		case <-session.Done():
		case <-t.done:
			reason := t.CloseReason()
			if reason == "" {
				reason = "the agent disconnected"
			}
			session.setError(reason)
			session.close()
		}
		t.mu.Lock()
		delete(t.consoleSessions, sessionID)
		t.mu.Unlock()
	}()

	return session, nil
}

// ConsoleSessionCount reports how many consoles are open on this transport.
//
// The UI shows this per guest ("2 viewers") because VNC has no multi-seat
// awareness of its own: a second cursor moving on its own is confusing when
// nothing says another person is connected.
func (t *AgentTransport) ConsoleSessionCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.consoleSessions)
}
