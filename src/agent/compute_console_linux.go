// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Guest console streaming, agent side.
//
// A console is a byte pipe in both directions and nothing here interprets its
// contents. For VNC the payload is the RFB protocol; for a serial or tty
// console it is terminal output. Touching either would break it.
//
// # The ordering rule, restated on this side
//
// The server registers its session before it sends console_start, so no frame
// it receives can be lost. This side has the same obligation in reverse: the
// reader goroutine must be running before anything is written toward the guest,
// or the guest's own opening bytes — QEMU's RFB banner, a login prompt — go
// into a socket nobody is draining.

// consoleReadBuffer is how much is read from the guest socket per frame.
//
// A VNC framebuffer update can be large, and a small buffer would fragment it
// into many WebSocket frames with a per-frame cost that shows up as a sluggish
// console.
const consoleReadBuffer = 32 * 1024

// consoleIdleTimeout closes a session nobody is using, matching the existing
// session idle convention. The UI shows a visible countdown before it fires.
const consoleIdleTimeout = 15 * time.Minute

// errConsoleIdle is the reason given when the idle timeout closes a session.
var errConsoleIdle = errors.New("the console was idle for 15 minutes and was closed")

// consoleWriteTimeout bounds one write toward a guest socket. Input is written
// on the agent's read loop, so an unbounded write would stall every session.
const consoleWriteTimeout = 10 * time.Second

// localDialTimeout bounds a dial to a guest console or SPICE socket on this host;
// a socket that does not accept in 10s is gone, and waiting only stalls the session.
const localDialTimeout = 10 * time.Second

// guestConsole is one open guest console, whatever carries it: a QEMU socket,
// a container's PID-1 console viewer, or an exec process. Read blocks until
// output or the end; Close must unblock it, and Write must bound itself.
type guestConsole interface {
	io.Reader
	Write(p []byte) (int, error)
	Resize(cols, rows int) error
	Close() error
}

// consoleSession is one live guest console on the agent side.
type consoleSession struct {
	id     string
	kind   string
	guest  guestConsole
	cancel context.CancelFunc

	// closeOnce keeps teardown idempotent: the server can send console_end
	// while the reader is simultaneously discovering the socket closed.
	closeOnce sync.Once
}

// consoleRegistry holds the agent's live console sessions.
type consoleRegistry struct {
	mu       sync.Mutex
	sessions map[string]*consoleSession
}

func newConsoleRegistry() *consoleRegistry {
	return &consoleRegistry{sessions: make(map[string]*consoleSession)}
}

func (r *consoleRegistry) add(session *consoleSession) {
	r.mu.Lock()
	r.sessions[session.id] = session
	r.mu.Unlock()
}

func (r *consoleRegistry) get(id string) (*consoleSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	return session, ok
}

func (r *consoleRegistry) remove(id string) {
	r.mu.Lock()
	session, ok := r.sessions[id]
	delete(r.sessions, id)
	r.mu.Unlock()
	if ok {
		session.close()
	}
}

// close tears a session down exactly once. For a container console that only
// detaches this viewer; for an exec session it kills the process it started.
func (s *consoleSession) close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.guest != nil {
			errlog.Close(s.guest)
		}
	})
}

// handleConsoleStart opens a guest console and begins pumping it.
func (a *Agent) handleConsoleStart(parent context.Context, id string, req ConsolePayload) {
	rt, err := a.compute(parent)
	if err != nil {
		writeConsoleEnd(a.sendCompute, id, req.SessionID, err)
		return
	}

	guest, err := rt.openGuestConsole(parent, req)
	if err != nil {
		writeConsoleEnd(a.sendCompute, id, req.SessionID,
			fmt.Errorf("this guest's %s console is not available: %w", req.Kind, err))
		return
	}
	rt.startConsoleSession(parent, id, req.Kind, guest)
}

// startConsoleSession registers a session and starts its pump.
func (rt *computeRuntime) startConsoleSession(parent context.Context, id, kind string, guest guestConsole) {
	ctx, cancel := context.WithCancel(parent)
	session := &consoleSession{
		id:     id,
		kind:   kind,
		guest:  guest,
		cancel: cancel,
	}

	// Registered before the reader starts and before anything is written, so
	// an input frame that arrives immediately has somewhere to go.
	rt.console.add(session)

	// The reader is this goroutine's whole purpose and ctx is its stop path:
	// cancelled by console_end, by the transport closing, or by the idle
	// timeout below.
	go rt.pumpConsole(ctx, session)
}

// openGuestConsole resolves and opens what a console request refers to.
//
// The mapping is deliberately explicit rather than taking a path from the
// request: this process is root, and a console request that could name an
// arbitrary unix socket would be a way to make the agent connect to anything on
// the host.
func (rt *computeRuntime) openGuestConsole(parent context.Context, req ConsolePayload) (guestConsole, error) {
	if req.TargetID == "" {
		return nil, errors.New("console request named no guest")
	}

	switch req.GuestKind {
	case "vm":
		socketPath, err := vmConsoleSocket(req)
		if err != nil {
			return nil, err
		}
		// A short dial: the socket either exists because the guest is running,
		// or it does not and the operator should be told rather than left waiting.
		dialCtx, dialCancel := context.WithTimeout(parent, localDialTimeout)
		defer dialCancel()
		var dialer net.Dialer
		conn, err := dialer.DialContext(dialCtx, "unix", socketPath)
		if err != nil {
			if req.Kind == "spice" {
				return nil, fmt.Errorf("%w (a guest started before SPICE was available has "+
					"no SPICE socket until its next start)", err)
			}
			return nil, err
		}
		if req.Kind == "serial" {
			serial := newSerialConsole(parent, conn, req.TargetID, rt.logger)
			if req.Cols > 0 && req.Rows > 0 {
				errlog.Log(serial.Resize(req.Cols, req.Rows))
			}
			return serial, nil
		}
		return socketConsole{conn}, nil

	case "ct":
		if rt.cts == nil {
			return nil, errors.New("this host cannot run system containers")
		}
		return openContainerConsole(parent, lxcConsoles{rt.cts}, req)

	default:
		return nil, fmt.Errorf("%q is not a guest kind", req.GuestKind)
	}
}

// containerConsoles is the part of the container manager a console needs.
type containerConsoles interface {
	AttachConsole(ctID string) (guestConsole, error)
	OpenExec(ctx context.Context, ctID string, opts lxc.ExecOptions) (guestConsole, error)
}

// lxcConsoles adapts the manager, returning a nil interface on error rather
// than an interface holding a nil pointer.
type lxcConsoles struct{ cts *lxc.Manager }

func (l lxcConsoles) AttachConsole(ctID string) (guestConsole, error) {
	attachment, err := l.cts.AttachConsole(ctID)
	if err != nil {
		return nil, err
	}
	return attachment, nil
}

func (l lxcConsoles) OpenExec(ctx context.Context, ctID string, opts lxc.ExecOptions) (guestConsole, error) {
	session, err := l.cts.OpenExec(ctx, ctID, opts)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// openContainerConsole maps a container console kind onto the manager: tty is
// PID 1's own console, shared and replayed; exec is a new process, with a pty
// for an interactive shell and plain pipes when the request names a command.
func openContainerConsole(ctx context.Context, cts containerConsoles, req ConsolePayload) (guestConsole, error) {
	switch req.Kind {
	case "tty":
		attachment, err := cts.AttachConsole(req.TargetID)
		if err != nil {
			return nil, err
		}
		if req.Cols > 0 && req.Rows > 0 {
			errlog.Log(attachment.Resize(req.Cols, req.Rows))
		}
		return attachment, nil
	case "exec":
		return cts.OpenExec(ctx, req.TargetID, lxc.ExecOptions{
			Argv: req.Command,
			TTY:  len(req.Command) == 0,
			Cols: req.Cols,
			Rows: req.Rows,
		})
	default:
		return nil, fmt.Errorf("%q is not a system container console kind", req.Kind)
	}
}

// vmConsoleSocket resolves which host socket a VM console request refers to.
func vmConsoleSocket(req ConsolePayload) (string, error) {
	paths := vm.PathsFor(req.TargetID)
	switch req.Kind {
	case "vnc":
		return paths.VNCSocket, nil
	case "serial":
		return paths.SerialSock, nil
	case "spice":
		// One connection per SPICE channel: the gateway opens a session for
		// each, and QEMU's SPICE server accepts them all on this one socket.
		return paths.SPICESocket, nil
	default:
		return "", fmt.Errorf("%q is not a virtual machine console kind", req.Kind)
	}
}

// socketConsole is a guest console carried by a unix socket QEMU serves.
type socketConsole struct {
	net.Conn
}

// Write bounds the write, so a guest that stops reading cannot stall input.
func (c socketConsole) Write(p []byte) (int, error) {
	if err := c.SetWriteDeadline(time.Now().Add(consoleWriteTimeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}

// Resize is a no-op for a graphical socket console: VNC and SPICE negotiate
// size in their own protocols. The serial console overrides it.
func (c socketConsole) Resize(int, int) error { return nil }

// serialResizeSettle is how long a burst of resizes — a window being dragged —
// is allowed to settle before the last size is sent into the guest.
const serialResizeSettle = 300 * time.Millisecond

// serialResizeTimeout bounds one guest-agent round trip for a resize. A guest
// whose agent does not answer loses the resize, never the console.
const serialResizeTimeout = 5 * time.Second

// errNoGuestAgent is why a serial resize did nothing. The UI shows "resize needs
// the guest agent" from the guest's own agent state, which it already reads.
var errNoGuestAgent = errors.New("resizing a serial console needs the QEMU guest agent running in the guest")

// serialConsole is a VM's serial line. It has no window-size channel, so a
// resize is carried in-band: `stty` on /dev/ttyS0, run through the guest agent.
//
// Resizes arrive on the agent's read loop and a guest-agent round trip can take
// seconds, so they are handed to a worker that keeps only the latest size.
type serialConsole struct {
	socketConsole
	vmID      string
	logger    *slog.Logger
	sizes     chan [2]int
	done      chan struct{}
	closeOnce sync.Once
}

// newSerialConsole wraps a serial socket and starts its resize worker.
func newSerialConsole(ctx context.Context, conn net.Conn, vmID string, logger *slog.Logger) *serialConsole {
	c := &serialConsole{
		socketConsole: socketConsole{conn},
		vmID:          vmID,
		logger:        logger,
		sizes:         make(chan [2]int, 1),
		done:          make(chan struct{}),
	}
	// goroutine lifecycle: ends when Close closes done or ctx is cancelled;
	// each iteration is bounded by serialResizeTimeout.
	go c.applySizes(ctx)
	return c
}

// Resize queues a size, replacing one not yet applied. It never blocks.
func (c *serialConsole) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	select {
	case <-c.sizes:
	default:
	}
	select {
	case c.sizes <- [2]int{cols, rows}:
	default:
	}
	return nil
}

// Close stops the resize worker and closes the socket.
func (c *serialConsole) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	return c.socketConsole.Close()
}

// applySizes sends the latest settled size into the guest until the console ends.
func (c *serialConsole) applySizes(ctx context.Context) {
	for {
		var size [2]int
		select {
		case <-c.done:
			return
		case <-ctx.Done():
			return
		case size = <-c.sizes:
		}
		settle := time.NewTimer(serialResizeSettle)
	settling:
		for {
			select {
			case <-c.done:
				settle.Stop()
				return
			case next := <-c.sizes:
				size = next
			case <-settle.C:
				break settling
			}
		}
		if err := setGuestSerialSize(ctx, c.vmID, size[0], size[1]); err != nil {
			c.logger.Debug("serial console resize not applied", "vm", c.vmID,
				"cols", size[0], "rows", size[1], "error", err)
		}
	}
}

// setGuestSerialSize runs the stty through the guest agent, or reports that
// there is no agent to run it.
func setGuestSerialSize(parent context.Context, vmID string, cols, rows int) error {
	ctx, cancel := context.WithTimeout(parent, serialResizeTimeout)
	defer cancel()
	socket := vm.PathsFor(vmID).GuestAgent
	if _, err := os.Stat(socket); err != nil {
		return errNoGuestAgent
	}
	guest, err := qmp.DialGuest(ctx, socket)
	if err != nil {
		return fmt.Errorf("%w: %w", errNoGuestAgent, err)
	}
	defer func() { errlog.Close(guest) }()
	return guest.SetSerialSize(ctx, cols, rows)
}

// pumpConsole forwards guest bytes to the server until the session ends.
func (rt *computeRuntime) pumpConsole(ctx context.Context, session *consoleSession) {
	defer rt.console.remove(session.id)

	buf := make([]byte, consoleReadBuffer)
	idle := time.NewTimer(consoleIdleTimeout)
	defer idle.Stop()

	// Read blocks, so the stop paths unblock it by closing the guest: a
	// cancelled session (console_end, transport gone) or the idle timeout.
	// The idle timer is reset on every output frame. A console with a guest
	// printing to it is not idle even if nobody is typing.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-watchDone:
		case <-ctx.Done():
			session.close()
		case <-idle.C:
			// Told before the close: the read that close unblocks sees a cancelled
			// session and would otherwise end it without a word to the server.
			writeConsoleEnd(rt.send, session.id, session.id, errConsoleIdle)
			session.close()
		}
	}()

	for {
		n, err := session.guest.Read(buf)

		if n > 0 {
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(consoleIdleTimeout)

			// The slice is copied because buf is reused on the next iteration
			// and WriteJSON may still be serializing this one.
			frame := make([]byte, n)
			copy(frame, buf[:n])

			if err := rt.send(WSMessage{
				Type: MsgConsoleOutput,
				ID:   session.id,
				Console: &ConsolePayload{
					SessionID: session.id,
					Kind:      session.kind,
					Data:      frame,
				},
			}); err != nil {
				// The transport is gone; the guest side goes with it.
				return
			}
		}

		if err != nil {
			if ctx.Err() != nil {
				// Ended from this side; the server already knows.
				return
			}
			var cause error
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				rt.logger.Debug("guest console read ended", "session", session.id, "error", err)
				if errors.Is(err, lxc.ErrViewerBehind) {
					cause = err
				}
			}
			writeConsoleEnd(rt.send, session.id, session.id, cause)
			return
		}
	}
}

// handleConsoleFrame handles input, resize and end.
//
// This runs on the read loop rather than a goroutine, deliberately: dispatching
// each input frame concurrently would let keystrokes overtake one another, and
// a console that reorders what the operator typed is worse than a slow one.
func (a *Agent) handleConsoleFrame(ctx context.Context, messageType, id string, payload ConsolePayload) {
	rt, err := a.compute(ctx)
	if err != nil {
		return
	}
	rt.consoleFrame(messageType, id, payload)
}

// consoleFrame applies one server frame to a live session.
func (rt *computeRuntime) consoleFrame(messageType, id string, payload ConsolePayload) {
	session, ok := rt.console.get(id)
	if !ok {
		return
	}

	switch messageType {
	case MsgConsoleInput:
		if len(payload.Data) == 0 {
			return
		}
		if _, err := session.guest.Write(payload.Data); err != nil {
			rt.logger.Debug("guest console write failed", "session", id, "error", err)
			rt.console.remove(id)
		}

	case MsgConsoleResize:
		// Only text consoles have a size. VNC and SPICE negotiate their own
		// in-band, so a resize for them is correctly a no-op rather than an
		// error.
		if session.kind == "serial" || session.kind == "tty" || session.kind == "exec" {
			if err := session.guest.Resize(payload.Cols, payload.Rows); err != nil {
				rt.logger.Debug("guest console resize failed",
					"session", id, "cols", payload.Cols, "rows", payload.Rows, "error", err)
			}
		}

	case MsgConsoleEnd:
		rt.console.remove(id)
	}
}

// writeConsoleEnd tells the server a session has ended, with a reason when
// there is one.
func writeConsoleEnd(send func(WSMessage) error, id, sessionID string, cause error) {
	payload := &ConsolePayload{SessionID: sessionID}
	if cause != nil {
		payload.Error = cause.Error()
	}
	errlog.Log(send(WSMessage{Type: MsgConsoleEnd, ID: id, Console: payload}))
}
