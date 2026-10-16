// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The QEMU guest agent speaks the same JSON shape as QMP on a separate socket,
// so it is served from this package with a different command set rather than a
// second client and a second dependency.
//
// Two things differ from the monitor and both matter:
//
//  1. There is no greeting and no capabilities handshake. The agent answers
//     commands immediately, and a stale in-flight reply from a previous session
//     is a real hazard — hence the explicit sync below.
//  2. EVERYTHING THE GUEST RETURNS IS UNTRUSTED. A guest is free to report a
//     hostname of `<img src=x onerror=...>` or an IP that points at something
//     else entirely. Nothing from here is used for authorization, written
//     unescaped into the UI, turned into a link, or interpolated into a
//     command. It is display-only, and the types below are deliberately narrow
//     so nothing else can leak through.

// guestDialTimeout bounds connecting to the guest agent socket. A guest with no
// agent installed simply never opens its end, which is a normal state and not
// an error worth surfacing.
const guestDialTimeout = 5 * time.Second

// guestCommandTimeout bounds a guest agent command. A guest that has hung — the
// exact case fsfreeze is most dangerous in — must not hold a snapshot open
// indefinitely.
const guestCommandTimeout = 15 * time.Second

// maxGuestReply caps how much a guest may return in one reply. A guest that
// answers guest-network-get-interfaces with megabytes of fabricated data is
// misbehaving, and the agent refuses to buffer it.
const maxGuestReply = 1 << 20 // 1 MiB

// GuestAgent is a connection to one guest's QEMU guest agent socket.
type GuestAgent struct {
	mu   sync.Mutex
	conn net.Conn
	br   *bufio.Reader
	seq  int64
}

// DialGuest connects to a guest agent socket. A failure here means the guest
// has no agent running, which the caller records as AgentState "absent".
func DialGuest(ctx context.Context, socketPath string) (*GuestAgent, error) {
	d := net.Dialer{Timeout: guestDialTimeout}
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("qga: dial %s: %w", socketPath, err)
	}
	return &GuestAgent{conn: conn, br: bufio.NewReaderSize(conn, 64*1024)}, nil
}

// Close releases the socket.
func (g *GuestAgent) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.conn.Close()
}

// sync discards any reply left over from a previous session before issuing a
// command.
//
// The guest agent protocol has a documented resynchronization sequence: send
// 0xFF, which the agent treats as a parse reset, then guest-sync-delimited with
// a nonce and skip everything before the matching answer. Without this, the
// first command after a reconnect can be answered by a stale reply from a
// command the previous connection issued — which, for a query whose result the
// UI displays as this guest's IP addresses, is a correctness problem and not
// merely untidy.
func (g *GuestAgent) sync(ctx context.Context) error {
	nonce := time.Now().UnixNano()
	if _, err := g.conn.Write([]byte{0xFF}); err != nil {
		return fmt.Errorf("qga: reset: %w", err)
	}
	payload, err := json.Marshal(command{
		Execute:   "guest-sync-delimited",
		Arguments: map[string]interface{}{"id": nonce},
		ID:        fmt.Sprintf("sync-%d", nonce),
	})
	if err != nil {
		return fmt.Errorf("qga: encode sync: %w", err)
	}
	if _, err := g.conn.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("qga: write sync: %w", err)
	}

	deadline := time.Now().Add(guestCommandTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	errlog.Log(g.conn.SetReadDeadline(deadline))

	// The delimited form prefixes its answer with 0xFF, so everything before
	// that byte is leftovers from the previous session.
	for {
		b, err := g.br.ReadByte()
		if err != nil {
			return fmt.Errorf("qga: sync: %w", err)
		}
		if b == 0xFF {
			break
		}
	}
	line, err := g.readLine()
	if err != nil {
		return fmt.Errorf("qga: sync reply: %w", err)
	}
	var r reply
	if err := json.Unmarshal(line, &r); err != nil {
		return fmt.Errorf("qga: parse sync reply: %w", err)
	}
	if r.Error != nil {
		return fmt.Errorf("qga: sync: %w", r.Error)
	}
	return nil
}

// readLine reads one bounded reply line.
func (g *GuestAgent) readLine() ([]byte, error) {
	line, err := g.br.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) > maxGuestReply {
		return nil, fmt.Errorf("qga: reply exceeds %d bytes", maxGuestReply)
	}
	return line, nil
}

// run issues one guest agent command, resynchronizing first.
func (g *GuestAgent) run(ctx context.Context, execute string, args interface{}, out interface{}) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, guestCommandTimeout)
		defer cancel()
	}
	if err := g.sync(ctx); err != nil {
		return err
	}

	g.seq++
	payload, err := json.Marshal(command{
		Execute:   execute,
		Arguments: args,
		ID:        fmt.Sprintf("qga-%d", g.seq),
	})
	if err != nil {
		return fmt.Errorf("qga: encode %s: %w", execute, err)
	}

	deadline := time.Now().Add(guestCommandTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	errlog.Log(g.conn.SetWriteDeadline(deadline))
	if _, err := g.conn.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("qga: write %s: %w", execute, err)
	}
	errlog.Log(g.conn.SetReadDeadline(deadline))

	line, err := g.readLine()
	if err != nil {
		return fmt.Errorf("qga: read %s: %w", execute, err)
	}
	var r reply
	if err := json.Unmarshal(line, &r); err != nil {
		return fmt.Errorf("qga: parse %s reply: %w", execute, err)
	}
	if r.Error != nil {
		return fmt.Errorf("qga: %s: %w", execute, r.Error)
	}
	if out == nil || len(r.Return) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.Return, out); err != nil {
		return fmt.Errorf("qga: decode %s result: %w", execute, err)
	}
	return nil
}

// Ping checks the agent is alive.
func (g *GuestAgent) Ping(ctx context.Context) error {
	return g.run(ctx, "guest-ping", nil, nil)
}

// GuestInterface is one network interface the guest reported. Untrusted.
type GuestInterface struct {
	Name         string `json:"name"`
	HardwareAddr string `json:"hardware-address,omitempty"`
	IPAddresses  []struct {
		IPAddress     string `json:"ip-address"`
		IPAddressType string `json:"ip-address-type"` // ipv4 | ipv6
		Prefix        int    `json:"prefix"`
	} `json:"ip-addresses,omitempty"`
}

// Interfaces asks the guest for its network interfaces.
//
// The result is what the VM list shows as the guest's address, explicitly
// labelled as guest-reported. It is never used to build a link the operator
// clicks and never used for any authorization decision.
func (g *GuestAgent) Interfaces(ctx context.Context) ([]GuestInterface, error) {
	var out []GuestInterface
	err := g.run(ctx, "guest-network-get-interfaces", nil, &out)
	return out, err
}

// UsableIPs flattens Interfaces into display addresses, dropping loopback and
// link-local entries and anything that does not parse as an IP.
//
// Parsing rather than passing the string through is the point: it is the one
// cheap check that stops a hostile guest from returning a value that renders as
// markup or reads as a different address than it is.
func (g *GuestAgent) UsableIPs(ctx context.Context) ([]string, error) {
	ifaces, err := g.Interfaces(ctx)
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, iface := range ifaces {
		if iface.Name == "lo" {
			continue
		}
		for _, addr := range iface.IPAddresses {
			parsed := net.ParseIP(addr.IPAddress)
			if parsed == nil || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
				continue
			}
			ips = append(ips, parsed.String())
		}
	}
	return ips, nil
}

// GuestOSInfo is what the guest says it is running. Display only.
type GuestOSInfo struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	PrettyName    string `json:"pretty-name,omitempty"`
	Version       string `json:"version,omitempty"`
	VersionID     string `json:"version-id,omitempty"`
	KernelRelease string `json:"kernel-release,omitempty"`
	Machine       string `json:"machine,omitempty"`
}

// OSInfo asks the guest what it is running.
func (g *GuestAgent) OSInfo(ctx context.Context) (GuestOSInfo, error) {
	var out GuestOSInfo
	err := g.run(ctx, "guest-get-osinfo", nil, &out)
	return out, err
}

// Hostname asks the guest for its hostname.
//
// The value is sanitized here rather than at render time as well, because it
// travels through logs and audit rows that have no HTML escaping of their own.
func (g *GuestAgent) Hostname(ctx context.Context) (string, error) {
	var out struct {
		HostName string `json:"host-name"`
	}
	if err := g.run(ctx, "guest-get-host-name", nil, &out); err != nil {
		return "", err
	}
	return sanitizeGuestString(out.HostName, 253), nil
}

// Shutdown asks the guest to power off from inside, which is cleaner than ACPI
// for guests that handle it.
func (g *GuestAgent) Shutdown(ctx context.Context) error {
	// The guest stops answering as it goes down, so a timeout here is the
	// expected outcome rather than a failure.
	err := g.run(ctx, "guest-shutdown", map[string]interface{}{"mode": "powerdown"}, nil)
	if err != nil && (strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "EOF")) {
		return nil
	}
	return err
}

// FsFreeze quiesces the guest's filesystems and returns how many were frozen.
//
// This is what turns a crash-consistent snapshot into a filesystem-consistent
// one. A frozen guest cannot write, so the caller must thaw it even on the
// error paths — a guest left frozen looks like a hung VM to its users.
func (g *GuestAgent) FsFreeze(ctx context.Context) (int, error) {
	var count int
	err := g.run(ctx, "guest-fsfreeze-freeze", nil, &count)
	return count, err
}

// FsThaw releases a freeze and returns how many filesystems were thawed.
func (g *GuestAgent) FsThaw(ctx context.Context) (int, error) {
	var count int
	err := g.run(ctx, "guest-fsfreeze-thaw", nil, &count)
	return count, err
}

// FsFreezeStatus reports "frozen" or "thawed", used to confirm a thaw actually
// took rather than assuming it did.
func (g *GuestAgent) FsFreezeStatus(ctx context.Context) (string, error) {
	var status string
	err := g.run(ctx, "guest-fsfreeze-status", nil, &status)
	return status, err
}

// FsTrim releases unused blocks back to a thin pool after a large delete.
func (g *GuestAgent) FsTrim(ctx context.Context) error {
	return g.run(ctx, "guest-fstrim", nil, nil)
}

// SetSerialSize tells the guest's serial line how big the operator's terminal
// is, by running `stty` on /dev/ttyS0 through guest-exec.
//
// guest-exec is a remote shell into the guest, so this is the only way this
// package reaches it: a fixed program and a fixed device, with nothing but two
// bounded integers taken from the caller. The process is not waited for.
func (g *GuestAgent) SetSerialSize(ctx context.Context, cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 1000 || rows > 1000 {
		return fmt.Errorf("qga: terminal size %dx%d is out of range", cols, rows)
	}
	var out struct {
		PID int `json:"pid"`
	}
	return g.run(ctx, "guest-exec", map[string]interface{}{
		"path": "stty",
		"arg": []string{
			"-F", "/dev/ttyS0",
			"rows", strconv.Itoa(rows),
			"cols", strconv.Itoa(cols),
		},
		"capture-output": false,
	}, &out)
}

// sanitizeGuestString makes a guest-supplied string safe to put in a log line,
// an audit row, or a JSON field: control characters removed, length bounded.
//
// This is defence in depth, not the only defence — the frontend renders every
// guest-reported value as text regardless. It exists because those values also
// reach places that do no escaping of their own.
func sanitizeGuestString(s string, limit int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= limit {
			break
		}
	}
	return strings.TrimSpace(b.String())
}
