// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package qmp speaks the QEMU Machine Protocol over a guest's unix monitor
// socket.
//
// # Why this is written here rather than taken from a library
//
// The roadmap's dependency policy allows any well-maintained library and bans
// only competing management products, and it named
// `github.com/digitalocean/go-qemu/qmp` as the intended client. That package
// turns out to link `github.com/digitalocean/go-libvirt` — it ships a
// LibvirtMonitor alongside the socket one, and Go links the whole libvirt RPC
// client either way. `go-libvirt` is on the policy's explicit ban list, and
// this binary runs as root on the operator's host, so pulling in an unused
// second control plane's RPC stack is exactly the trade the policy exists to
// refuse.
//
// The same document sanctions the alternative directly: "If it goes
// unmaintained, vendor it — roughly 600 lines under a permissive licence. That
// is the correct fallback." This is that fallback, taken up front rather than
// later. What is implemented here is only the socket monitor: the
// greeting-then-capabilities handshake, `id` correlation, and demultiplexing
// asynchronous events onto a channel. The wire protocol is versioned by QEMU,
// not by any library, so there is nothing here that upstream would have kept
// current for us.
//
// Everything above the transport — typed commands, per-VM serialization,
// capability gating from query-qmp-schema, and event translation — lives in
// this package too, because those four things are McHarbor policy and were
// never in the library to begin with.
package qmp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// dialTimeout bounds the initial connect. A VM whose monitor socket is not yet
// listening is a normal race immediately after unit start, so callers retry
// rather than treating this as fatal.
const dialTimeout = 10 * time.Second

// defaultCommandTimeout bounds a single command. QMP commands are expected to
// answer immediately; the ones that do not (block jobs, migration) report
// progress through events instead of blocking the monitor.
const defaultCommandTimeout = 30 * time.Second

// eventBuffer is how many asynchronous events may queue for a slow consumer
// before the oldest are dropped. Events are advisory — the authoritative state
// always comes from query-status — so dropping is preferable to stalling the
// read loop and wedging every in-flight command behind it.
const eventBuffer = 64

// ErrClosed is returned by Run once the monitor has been closed, or once its
// socket has gone away and the reconnect budget is exhausted.
var ErrClosed = errors.New("qmp: monitor closed")

// Error is a QMP command error. QEMU answers a failed command with
// {"error": {"class": ..., "desc": ...}}.
//
// Class is stable across QEMU versions and is what callers match on. Desc is
// free text whose wording changes between versions, is written partly from
// guest-influenced values, and per the Go standards is logged server-side and
// never returned raw to a client — the service layer maps Class to an i18n
// code and passes a support id instead.
type Error struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

func (e *Error) Error() string {
	return "qmp: " + e.Class + ": " + e.Desc
}

// Event is one asynchronous QMP event.
//
// Data is guest-influenced for several event kinds (RTC_CHANGE, GUEST_PANICKED
// with a guest-supplied payload), so it is carried as raw values and escaped at
// the point of rendering, never interpolated into a command or a link.
type Event struct {
	Event     string                 `json:"event"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp struct {
		Seconds      int64 `json:"seconds"`
		Microseconds int64 `json:"microseconds"`
	} `json:"timestamp"`
}

// Time returns the event's host timestamp.
func (e Event) Time() time.Time {
	return time.Unix(e.Timestamp.Seconds, e.Timestamp.Microseconds*1000)
}

// command is one outbound QMP command. Every command carries an id so replies
// can be correlated: without it, a reply arriving while two commands are in
// flight cannot be attributed, which is the entire reason QMP has the field.
type command struct {
	Execute   string      `json:"execute"`
	Arguments interface{} `json:"arguments,omitempty"`
	ID        string      `json:"id"`
}

// reply is the shape QEMU answers with. Exactly one of Return and Error is
// populated; an object carrying neither and an "event" key is an asynchronous
// event and is routed to the event channel instead.
type reply struct {
	Return json.RawMessage `json:"return,omitempty"`
	Error  *Error          `json:"error,omitempty"`
	ID     string          `json:"id,omitempty"`
	Event  string          `json:"event,omitempty"`
}

// greeting is QEMU's banner, sent unsolicited on connect. The monitor is in
// capabilities-negotiation mode until qmp_capabilities succeeds, and no other
// command is accepted before that.
type greeting struct {
	QMP struct {
		Version struct {
			QEMU struct {
				Major int `json:"major"`
				Minor int `json:"minor"`
				Micro int `json:"micro"`
			} `json:"qemu"`
			Package string `json:"package"`
		} `json:"version"`
		Capabilities []string `json:"capabilities"`
	} `json:"QMP"`
}

// Version is the QEMU version a monitor is connected to, used for the
// capability gating that keeps the UI from offering a command this QEMU does
// not have.
type Version struct {
	Major int
	Minor int
	Micro int
}

// AtLeast reports whether v is at least major.minor.
func (v Version) AtLeast(major, minor int) bool {
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// String renders the version as QEMU prints it.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Micro)
}

// Monitor is a single guest's QMP connection.
//
// One monitor per VM, serialized: QMP itself tolerates pipelining, but
// serializing here means a command's error can be attributed to the command
// that caused it, and it bounds how much a wedged guest can queue.
//
// A monitor whose socket has gone away reports itself not ready rather than
// guessing. The caller surfaces that VM as state "unknown"; it never assumes
// the guest is still running because it was a moment ago.
type Monitor struct {
	path string

	// mu serializes command submission. It is not held across the wait for a
	// reply — that would make one slow command block the whole monitor — but
	// it is held long enough to register the pending entry before the write,
	// so a reply can never arrive before there is somewhere to put it.
	mu   sync.Mutex
	conn net.Conn
	enc  *json.Encoder

	// pending maps command id to the channel its reply will be delivered on.
	pendingMu sync.Mutex
	pending   map[string]chan reply

	events chan Event
	seq    atomic.Uint64

	version Version

	// schema caches which commands this QEMU actually implements, read once
	// per connection from query-qmp-schema. An optional command absent here is
	// disabled in the UI with a tooltip naming the required version — never
	// attempted and failed.
	schemaOnce sync.Once
	schema     map[string]bool

	closeOnce sync.Once
	closed    chan struct{}
	// readErr records why the read loop ended, so a caller that was waiting on
	// a reply learns the cause rather than only that its context expired.
	readErrMu sync.Mutex
	readErr   error
}

// Dial connects to a guest's QMP socket and completes the handshake: read the
// greeting, then issue qmp_capabilities. Both halves are mandatory — QEMU
// rejects every other command until capabilities negotiation finishes.
func Dial(ctx context.Context, socketPath string) (*Monitor, error) {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("qmp: dial %s: %w", socketPath, err)
	}

	m := &Monitor{
		path:    socketPath,
		conn:    conn,
		enc:     json.NewEncoder(conn),
		pending: make(map[string]chan reply),
		events:  make(chan Event, eventBuffer),
		closed:  make(chan struct{}),
	}

	// The greeting is read synchronously, before the read loop starts, so the
	// banner cannot be mistaken for a command reply.
	br := bufio.NewReader(conn)
	if err := m.readGreeting(ctx, br); err != nil {
		errlog.Close(conn)
		return nil, err
	}

	go m.readLoop(br)

	if _, err := m.Run(ctx, "qmp_capabilities", nil); err != nil {
		errlog.Close(m)
		return nil, fmt.Errorf("qmp: capabilities handshake: %w", err)
	}
	return m, nil
}

// readGreeting consumes QEMU's banner and records the version behind it.
func (m *Monitor) readGreeting(ctx context.Context, br *bufio.Reader) error {
	if deadline, ok := ctx.Deadline(); ok {
		errlog.Log(m.conn.SetReadDeadline(deadline))
	} else {
		errlog.Log(m.conn.SetReadDeadline(time.Now().Add(dialTimeout)))
	}
	defer func() { errlog.Log(m.conn.SetReadDeadline(time.Time{})) }()

	line, err := br.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("qmp: read greeting: %w", err)
	}
	var g greeting
	if err := json.Unmarshal(line, &g); err != nil {
		return fmt.Errorf("qmp: parse greeting: %w", err)
	}
	m.version = Version{
		Major: g.QMP.Version.QEMU.Major,
		Minor: g.QMP.Version.QEMU.Minor,
		Micro: g.QMP.Version.QEMU.Micro,
	}
	return nil
}

// readLoop demultiplexes the socket: replies go to whoever is waiting on their
// id, events go to the event channel.
//
// It owns the connection's read side for the monitor's whole lifetime and exits
// only when the socket fails or Close is called, which satisfies the "every
// goroutine gets a lifecycle" rule — Close is the stop path.
func (m *Monitor) readLoop(br *bufio.Reader) {
	defer m.finish()

	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			m.setReadErr(err)
			return
		}
		if len(line) == 0 {
			continue
		}

		var r reply
		if err := json.Unmarshal(line, &r); err != nil {
			// A frame we cannot parse is not a reason to tear down a working
			// monitor: skip it and keep reading.
			continue
		}

		if r.Event != "" {
			var ev Event
			if err := json.Unmarshal(line, &ev); err == nil {
				m.deliverEvent(ev)
			}
			continue
		}

		m.deliverReply(r)
	}
}

// deliverEvent hands an event to the consumer, dropping the oldest queued event
// rather than blocking the read loop behind a slow reader. Events are advisory;
// query-status is authoritative.
func (m *Monitor) deliverEvent(ev Event) {
	select {
	case m.events <- ev:
	default:
		select {
		case <-m.events:
		default:
		}
		select {
		case m.events <- ev:
		default:
		}
	}
}

// deliverReply routes a reply to the command waiting on its id. A reply with no
// waiter is discarded: it belongs to a command whose caller already gave up.
func (m *Monitor) deliverReply(r reply) {
	m.pendingMu.Lock()
	ch, ok := m.pending[r.ID]
	if ok {
		delete(m.pending, r.ID)
	}
	m.pendingMu.Unlock()
	if !ok {
		return
	}
	// Buffered with room for exactly this send, so it never blocks.
	ch <- r
}

// finish releases every waiter when the read loop ends, so a caller learns the
// monitor died instead of waiting out its own timeout.
func (m *Monitor) finish() {
	m.pendingMu.Lock()
	for id, ch := range m.pending {
		delete(m.pending, id)
		close(ch)
	}
	m.pendingMu.Unlock()
	m.closeOnce.Do(func() {
		close(m.closed)
		errlog.Close(m.conn)
	})
}

func (m *Monitor) setReadErr(err error) {
	m.readErrMu.Lock()
	if m.readErr == nil {
		m.readErr = err
	}
	m.readErrMu.Unlock()
}

func (m *Monitor) lastReadErr() error {
	m.readErrMu.Lock()
	defer m.readErrMu.Unlock()
	return m.readErr
}

// Run issues one command and waits for its reply.
//
// A *Error return means QEMU refused the command and the monitor is still
// healthy; any other error means the monitor itself is in doubt.
func (m *Monitor) Run(ctx context.Context, execute string, args interface{}) (json.RawMessage, error) {
	select {
	case <-m.closed:
		if err := m.lastReadErr(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrClosed, err)
		}
		return nil, ErrClosed
	default:
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCommandTimeout)
		defer cancel()
	}

	id := fmt.Sprintf("mch-%d", m.seq.Add(1))
	ch := make(chan reply, 1)

	// Register before writing. If the reply were allowed to arrive first there
	// would be no waiter to hand it to and the command would hang until its
	// own timeout — the same ordering hazard the console stream has, in a
	// smaller place.
	m.pendingMu.Lock()
	m.pending[id] = ch
	m.pendingMu.Unlock()

	m.mu.Lock()
	err := m.enc.Encode(command{Execute: execute, Arguments: args, ID: id})
	m.mu.Unlock()
	if err != nil {
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
		return nil, fmt.Errorf("qmp: write %s: %w", execute, err)
	}
	return m.await(ctx, execute, id, ch)
}

// await waits for the reply registered under id. Split from Run so a command
// written another way, with a descriptor attached, waits the same way.
func (m *Monitor) await(ctx context.Context, execute, id string, ch chan reply) (json.RawMessage, error) {
	select {
	case r, ok := <-ch:
		if !ok {
			if rerr := m.lastReadErr(); rerr != nil {
				return nil, fmt.Errorf("%w: %w", ErrClosed, rerr)
			}
			return nil, ErrClosed
		}
		if r.Error != nil {
			// Wrapped with %w so the class stays matchable upstream.
			return nil, fmt.Errorf("qmp: %s: %w", execute, r.Error)
		}
		return r.Return, nil
	case <-ctx.Done():
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
		return nil, fmt.Errorf("qmp: %s: %w", execute, ctx.Err())
	case <-m.closed:
		if rerr := m.lastReadErr(); rerr != nil {
			return nil, fmt.Errorf("%w: %w", ErrClosed, rerr)
		}
		return nil, ErrClosed
	}
}

// RunInto issues a command and decodes its return value into out.
func (m *Monitor) RunInto(ctx context.Context, execute string, args interface{}, out interface{}) error {
	raw, err := m.Run(ctx, execute, args)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("qmp: decode %s result: %w", execute, err)
	}
	return nil
}

// Events returns the asynchronous event stream. The channel is closed when the
// monitor closes.
func (m *Monitor) Events() <-chan Event { return m.events }

// Version reports the QEMU version from the connection greeting.
func (m *Monitor) Version() Version { return m.version }

// Ready reports whether the monitor still has a live socket. A false answer
// means the owning VM's state is unknown, not that it stopped.
func (m *Monitor) Ready() bool {
	select {
	case <-m.closed:
		return false
	default:
		return true
	}
}

// Done is closed when the monitor closes, for callers that supervise it.
func (m *Monitor) Done() <-chan struct{} { return m.closed }

// Supports reports whether this QEMU implements a command, from
// query-qmp-schema read once per connection.
//
// The schema query is itself optional on very old QEMU; when it cannot be read,
// every command is reported supported so behaviour falls back to "attempt and
// surface the error" rather than disabling the whole feature set.
func (m *Monitor) Supports(ctx context.Context, command string) bool {
	m.schemaOnce.Do(func() { m.loadSchema(ctx) })
	if m.schema == nil {
		return true
	}
	return m.schema[command]
}

// schemaEntry is one element of the query-qmp-schema array. Only commands
// matter here; the type, event and object entries are ignored.
type schemaEntry struct {
	Name string `json:"name"`
	Meta string `json:"meta-type"`
}

func (m *Monitor) loadSchema(ctx context.Context) {
	var entries []schemaEntry
	if err := m.RunInto(ctx, "query-qmp-schema", nil, &entries); err != nil {
		// Leave m.schema nil: Supports then answers true for everything.
		return
	}
	schema := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Meta == "command" {
			schema[e.Name] = true
		}
	}
	m.schema = schema
}

// Close shuts the monitor down and releases every waiter.
func (m *Monitor) Close() error {
	m.closeOnce.Do(func() {
		close(m.closed)
		errlog.Close(m.conn)
	})
	return nil
}

// Path returns the socket this monitor is connected to, for logging.
func (m *Monitor) Path() string { return m.path }
