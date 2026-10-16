// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package agentrpc is the host bridge between plugins and the agent transport, in builtin
// types only, so a plugin can assert it locally and the plugin ABI never has to name it.
//
// It carries no compute meaning. What may be sent, with which budget and version floor, is
// core/agent's plugin route ceiling; what one plugin may send or observe is that plugin's
// agent-rpc declaration, enforced by its View. The agent runs as root, so the ceiling is
// the only source of routable types and a declaration can only narrow it.
package agentrpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// Handle is what a plugin asserts its host handle against. Every type in it is builtin, so
// a plugin declares the same interface locally and never imports this package.
type Handle interface {
	// Available reports whether envID can take agent calls, and why not when it cannot.
	Available(ctx context.Context, envID string) (bool, string)
	// Call sends one declared message and returns the agent's reply slot as JSON.
	Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(msgType string, frame []byte)) ([]byte, error)
	// Subscribe replaces the caller's sink for declared unsolicited frames; nil removes it.
	Subscribe(sink func(envID, msgType string, frame []byte))
}

// AgentPool is the slice of the Docker client pool the bridge needs, the same shape
// snapshotdrivers takes, so the pool satisfies both unadapted.
type AgentPool interface {
	// IsAgentEnv reports whether an environment is reached through an agent.
	IsAgentEnv(envID string) bool
	// AgentConnection returns the live connection for an environment.
	AgentConnection(envID string) (*coreagent.AgentConnection, bool)
}

// The codes Error.AgentRPCCode returns. A plugin branches on these, never on error text.
const (
	// CodeNotAgentEnv means the environment is not reached through an agent at all.
	CodeNotAgentEnv = "not_agent_env"
	// CodeAgentOffline means no agent is connected, or it left mid-call. Never buffered.
	CodeAgentOffline = "agent_offline"
	// CodeAgentOutdated means the agent is below a floor; the message says "needs agent".
	CodeAgentOutdated = "agent_outdated"
	// CodeUplinkBusy means the host refused the adapter a network was to be built on.
	CodeUplinkBusy = "uplink_busy"
	// CodeNameTaken means the host already has an interface of that name that McHarbor does not own.
	CodeNameTaken = "name_taken"
	// CodeParentNotFound means the adapter a host interface stacks on is not on the host.
	CodeParentNotFound = "parent_not_found"
	// CodePermissionDenied means the agent may not change the host's interfaces.
	CodePermissionDenied = "permission_denied"
	// CodeRefused covers an undeclared or unroutable type and every agent-side failure.
	CodeRefused = "refused"
)

// ErrUndeclared means a plugin called a type its agent-rpc declaration does not list.
var ErrUndeclared = errors.New("agentrpc: message type not declared by the plugin")

// Error is every error Call returns. It unwraps to the transport's own error, so
// errors.Is against context and core/agent sentinels still works inside the host.
type Error struct {
	code string
	err  error
}

// Error keeps the transport's wording, which is what addon-lxc's "needs agent" match reads.
func (e *Error) Error() string { return e.err.Error() }

// Unwrap exposes the transport error.
func (e *Error) Unwrap() error { return e.err }

// AgentRPCCode names the failure class, the one thing a plugin should branch on.
func (e *Error) AgentRPCCode() string { return e.code }

// classify turns a transport error into an Error with the code a plugin can act on.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var already *Error
	if errors.As(err, &already) {
		return err
	}
	code := CodeRefused
	switch {
	case errors.Is(err, coreagent.ErrComputeUnsupported):
		code = CodeAgentOutdated
	case errors.Is(err, coreagent.ErrAgentOffline):
		code = CodeAgentOffline
	case errors.Is(err, coreagent.ErrUplinkBusy):
		code = CodeUplinkBusy
	case errors.Is(err, coreagent.ErrHostLinkNameTaken):
		code = CodeNameTaken
	case errors.Is(err, coreagent.ErrHostLinkParentNotFound):
		code = CodeParentNotFound
	case errors.Is(err, coreagent.ErrHostLinkPermissionDenied):
		code = CodePermissionDenied
	}
	return &Error{code: code, err: err}
}

// Bridge is the host side: one per process, shared by every plugin View.
type Bridge struct {
	pool   AgentPool
	logger *slog.Logger

	mu   sync.Mutex
	subs map[string]*subscription
}

// New returns a bridge over pool. It observes nothing until Install is called, so tests
// can build several without fighting over the transport's process-wide observer.
func New(pool AgentPool, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bridge{pool: pool, logger: logger, subs: map[string]*subscription{}}
}

// Install makes this bridge the transport's observer of unsolicited plugin frames.
func (b *Bridge) Install() {
	coreagent.SetPluginFrameObserver(b.Observe)
}

// Available keeps the reason strings the compute handle has always returned, because the
// compute plugins' interfaces render them: not_agent_environment, agent_offline and
// agent_too_old.
func (b *Bridge) Available(_ context.Context, envID string) (bool, string) {
	if b.pool == nil || !b.pool.IsAgentEnv(envID) {
		return false, "not_agent_environment"
	}
	conn, ok := b.pool.AgentConnection(envID)
	if !ok || conn == nil || conn.Transport == nil {
		return false, "agent_offline"
	}
	if !conn.Transport.ComputeSupported() {
		return false, "agent_too_old"
	}
	return true, ""
}

// Call sends any message inside the ceiling. It is the host's own entry point; a plugin is
// only ever handed a View, which checks its declaration first.
func (b *Bridge) Call(
	ctx context.Context,
	envID, msgType string,
	payload []byte,
	onProgress func(msgType string, frame []byte),
) ([]byte, error) {
	transport, err := b.transport(envID)
	if err != nil {
		return nil, err
	}
	reply, err := transport.Call(ctx, envID, msgType, payload, onProgress)
	return reply, classify(err)
}

// transport resolves envID's agent transport, failing fast: a call to an offline agent is
// refused rather than queued, because a replayed "start this guest" is a hazard.
func (b *Bridge) transport(envID string) (*coreagent.AgentTransport, error) {
	if envID == "" {
		return nil, &Error{code: CodeNotAgentEnv, err: errors.New("agentrpc: no environment given")}
	}
	if b.pool == nil || !b.pool.IsAgentEnv(envID) {
		return nil, &Error{code: CodeNotAgentEnv, err: fmt.Errorf(
			"agentrpc: environment %s is not reached through an agent", envID)}
	}
	conn, ok := b.pool.AgentConnection(envID)
	if !ok || conn == nil || conn.Transport == nil {
		return nil, &Error{code: CodeAgentOffline, err: fmt.Errorf(
			"%w: no agent is connected for environment %s", coreagent.ErrAgentOffline, envID)}
	}
	if !conn.Transport.ComputeSupported() {
		return nil, &Error{code: CodeAgentOutdated, err: fmt.Errorf(
			"%w: environment %s needs agent %s or newer", coreagent.ErrComputeUnsupported,
			envID, coreagent.ComputeMinVersion)}
	}
	return conn.Transport, nil
}

// Subscribe installs the host's own unkeyed sink for every observable frame.
func (b *Bridge) Subscribe(sink func(envID, msgType string, frame []byte)) {
	b.subscribe("", nil, sink)
}

// ForPlugin returns slug's view, narrowed to its declaration. Types outside the ceiling are
// dropped here as well as at validation, so a hand-built declaration cannot widen it.
func (b *Bridge) ForPlugin(slug string, send, observe []string) *View {
	v := &View{b: b, slug: slug, send: map[string]bool{}, observe: map[string]bool{}}
	for _, t := range send {
		if _, ok := coreagent.PluginRouteFor(t); ok {
			v.send[t] = true
		}
	}
	for _, t := range observe {
		if _, ok := coreagent.PluginObservedFor(t); ok {
			v.observe[t] = true
		}
	}
	return v
}

// View is one plugin's bridge: the ceiling narrowed to what its manifest declares.
type View struct {
	b       *Bridge
	slug    string
	send    map[string]bool
	observe map[string]bool
}

var _ Handle = (*View)(nil)

// Available is the bridge's; availability says nothing a plugin may not know.
func (v *View) Available(ctx context.Context, envID string) (bool, string) {
	return v.b.Available(ctx, envID)
}

// Call refuses any type the plugin did not declare, before resolving the environment, so
// an undeclared type never costs the agent anything.
func (v *View) Call(
	ctx context.Context,
	envID, msgType string,
	payload []byte,
	onProgress func(msgType string, frame []byte),
) ([]byte, error) {
	if !v.send[msgType] {
		return nil, &Error{code: CodeRefused, err: fmt.Errorf("%w: %s did not declare %q in agent-rpc send",
			ErrUndeclared, v.slug, msgType)}
	}
	return v.b.Call(ctx, envID, msgType, payload, onProgress)
}

// Subscribe replaces only this plugin's sink, which is what a reload needs. A plugin that
// declared nothing to observe gets a sink that is never called.
func (v *View) Subscribe(sink func(envID, msgType string, frame []byte)) {
	v.b.subscribe(v.slug, v.observe, sink)
}

// ConsoleRequest names one guest console to open. Command is set only for an exec console,
// and the gateway chooses it server-side.
type ConsoleRequest struct {
	Kind      string
	GuestKind string
	GuestID   string
	Command   []string
	Cols      int
	Rows      int
}

// ConsoleStream is one live guest console. Bytes pass through unmodified both ways, because
// for VNC and SPICE they are the protocol itself.
type ConsoleStream interface {
	// Output delivers guest bytes and is closed when the session ends.
	Output() <-chan []byte
	// Send forwards client bytes to the guest.
	Send(data []byte) error
	// Resize reports new dimensions; graphical consoles negotiate theirs in-band.
	Resize(cols, rows int) error
	// Done is closed when the session ends from either side.
	Done() <-chan struct{}
	// Err says why the session ended, empty when it ended cleanly.
	Err() string
	// Close ends the session and releases the guest side.
	Close() error
}

// OpenConsole opens a guest console on envID's agent for the host's console gateway. No plugin
// View offers it: a console is a hijacked byte stream only the host can bridge, and which
// guest kinds and consoles exist comes from console-subject declarations, not from here.
func (b *Bridge) OpenConsole(ctx context.Context, envID string, req ConsoleRequest) (ConsoleStream, error) {
	transport, err := b.transport(envID)
	if err != nil {
		return nil, err
	}
	session, err := transport.StartConsole(ctx, req.Kind, req.GuestKind, req.GuestID, req.Command, req.Cols, req.Rows)
	if err != nil {
		return nil, classify(err)
	}
	return consoleSession{session}, nil
}

// consoleSession adapts the transport's session to ConsoleStream.
type consoleSession struct {
	s *coreagent.ConsoleSession
}

func (c consoleSession) Output() <-chan []byte       { return c.s.OutputCh }
func (c consoleSession) Send(data []byte) error      { return c.s.Send(data) }
func (c consoleSession) Resize(cols, rows int) error { return c.s.Resize(cols, rows) }
func (c consoleSession) Done() <-chan struct{}       { return c.s.Done() }
func (c consoleSession) Err() string                 { return c.s.Err() }
func (c consoleSession) Close() error                { return c.s.Close() }

// NewInstalled returns the process's one bridge, already installed as the transport's observer
// of unsolicited plugin frames. main.go calls it once at wiring time, which is what replaced
// the compute event sink a handle used to set on itself.
func NewInstalled(pool AgentPool, logger *slog.Logger) *Bridge {
	b := New(pool, logger)
	b.Install()
	return b
}
