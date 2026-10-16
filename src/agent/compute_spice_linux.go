// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// The remote-viewer path into a guest's SPICE console.
//
// The browser needs none of this: spice-html5 rides the console gateway, one
// console stream per SPICE channel, exactly as noVNC does. A native
// remote-viewer cannot speak WebSocket, so McHarbor hands it a .vv file that
// points at a TCP port on this host, and this relay behind that port checks the
// file's one-time password inside SPICE's own ticket exchange before any byte
// reaches QEMU.

// spiceTicketTTL is how long a ticket admits new connections. remote-viewer
// opens all its channels within a second or two of launching; the long tail is
// for the operator double-clicking a file they downloaded a moment ago.
const spiceTicketTTL = 2 * time.Minute

// spiceLinkTimeout bounds the unauthenticated link stage of one connection.
const spiceLinkTimeout = 20 * time.Second

// spiceMaxRelays bounds how many ticket listeners this host keeps open at once.
const spiceMaxRelays = 16

// spiceRelayPortEnv pins the relay to a port the operator opened in their
// firewall. Unset, the kernel picks an ephemeral port per ticket.
const spiceRelayPortEnv = "MCHARBOR_SPICE_RELAY_PORT"

// spiceRelay is one ticket: a listener and the password that opens it.
type spiceRelay struct {
	vmID     string
	password string
	key      spiceRelayKey
	listener net.Listener
	timer    *time.Timer
	once     sync.Once
}

// close stops the relay admitting connections; open sessions carry on.
func (r *spiceRelay) close() {
	r.once.Do(func() {
		r.timer.Stop()
		errlog.Close(r.listener)
	})
}

// spiceRelays holds the live tickets, one per guest.
type spiceRelays struct {
	mu   sync.Mutex
	byVM map[string]*spiceRelay
}

var (
	spiceRelaysOnce sync.Once
	spiceRelaySet   *spiceRelays
)

// spiceRelayRegistry returns the process-wide ticket set.
func spiceRelayRegistry() *spiceRelays {
	spiceRelaysOnce.Do(func() {
		spiceRelaySet = &spiceRelays{byVM: make(map[string]*spiceRelay)}
	})
	return spiceRelaySet
}

// handleSpiceTicket answers vm_spice_ticket with a port and a one-time password.
func (rt *computeRuntime) handleSpiceTicket(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if err := validateGuestID(req.VMID); err != nil {
		return err
	}
	socket := vm.PathsFor(req.VMID).SPICESocket
	if _, err := os.Stat(socket); err != nil {
		return errors.New("compute: this guest has no SPICE server; it has to be running, and started on a " +
			"host whose QEMU supports SPICE")
	}

	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("compute: minting a SPICE password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(secret)
	key, err := newSpiceRelayKey()
	if err != nil {
		return err
	}

	listener, err := listenSpiceRelay(ctx)
	if err != nil {
		return err
	}
	relay := &spiceRelay{vmID: req.VMID, password: password, key: key, listener: listener}
	relay.timer = time.AfterFunc(spiceTicketTTL, relay.close)

	registry := spiceRelayRegistry()
	registry.mu.Lock()
	if previous, ok := registry.byVM[req.VMID]; ok {
		// One ticket per guest: a new download replaces the last one, so an
		// older file stops working the moment a newer one exists.
		previous.close()
	}
	if len(registry.byVM) >= spiceMaxRelays {
		registry.mu.Unlock()
		relay.close()
		return errors.New("compute: too many SPICE tickets are open on this host; try again in a minute")
	}
	registry.byVM[req.VMID] = relay
	registry.mu.Unlock()

	// goroutine lifecycle: ends when the listener closes, at spiceTicketTTL or
	// when a newer ticket for the same guest replaces this one.
	// Detached from the request: the ticket outlives the reply that carried it.
	go rt.acceptSpice(context.WithoutCancel(ctx), relay, socket)

	port := listener.Addr().(*net.TCPAddr).Port
	reply.SpiceTicket = &VMSpiceTicket{
		Port:      port,
		Password:  password,
		Addresses: hostAddresses(),
		ExpiresAt: time.Now().Add(spiceTicketTTL).UTC().Format(time.RFC3339),
	}
	return nil
}

// listenSpiceRelay opens the ticket's TCP port.
func listenSpiceRelay(ctx context.Context) (net.Listener, error) {
	port := 0
	if value := os.Getenv(spiceRelayPortEnv); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return nil, fmt.Errorf("compute: %s=%q is not a port", spiceRelayPortEnv, value)
		}
		port = parsed
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("compute: opening the SPICE relay port: %w", err)
	}
	return listener, nil
}

// acceptSpice admits connections until the ticket's listener closes.
func (rt *computeRuntime) acceptSpice(ctx context.Context, relay *spiceRelay, socket string) {
	defer func() {
		registry := spiceRelayRegistry()
		registry.mu.Lock()
		if registry.byVM[relay.vmID] == relay {
			delete(registry.byVM, relay.vmID)
		}
		registry.mu.Unlock()
	}()
	for {
		conn, err := relay.listener.Accept()
		if err != nil {
			return
		}
		// goroutine lifecycle: ends when either side of the session closes;
		// the link stage is bounded by spiceLinkTimeout.
		go rt.serveSpice(ctx, relay, conn, socket)
	}
}

// serveSpice authenticates one channel and then copies bytes both ways.
func (rt *computeRuntime) serveSpice(ctx context.Context, relay *spiceRelay, client net.Conn, socket string) {
	defer func() { errlog.Close(client) }()

	dialCtx, cancelDial := context.WithTimeout(ctx, localDialTimeout)
	var dialer net.Dialer
	upstream, err := dialer.DialContext(dialCtx, "unix", socket)
	cancelDial()
	if err != nil {
		rt.logger.Debug("spice relay: guest socket unreachable", "vm", relay.vmID, "error", err)
		return
	}
	defer func() { errlog.Close(upstream) }()

	deadline := time.Now().Add(spiceLinkTimeout)
	errlog.Log(client.SetDeadline(deadline))
	errlog.Log(upstream.SetDeadline(deadline))
	if err := relaySpiceLink(client, upstream, relay.key, relay.password); err != nil {
		if errors.Is(err, errSpiceDenied) {
			rt.logger.Warn("spice relay: refused a connection with the wrong password",
				"vm", relay.vmID, "remote", client.RemoteAddr().String())
		} else {
			rt.logger.Debug("spice relay: link stage failed", "vm", relay.vmID, "error", err)
		}
		return
	}
	errlog.Log(client.SetDeadline(time.Time{}))
	errlog.Log(upstream.SetDeadline(time.Time{}))

	done := make(chan struct{}, 2)
	pump := func(dst, src net.Conn) {
		if _, err := io.Copy(dst, src); err != nil && !errors.Is(err, net.ErrClosed) {
			rt.logger.Debug("spice relay: session ended", "vm", relay.vmID, "error", err)
		}
		done <- struct{}{}
	}
	// goroutine lifecycle: both pumps end when either connection closes, which
	// the deferred closes above guarantee once the first one returns.
	go pump(upstream, client)
	go pump(client, upstream)
	<-done
}

// hostAddresses lists this host's usable unicast addresses, IPv4 first, so
// McHarbor can offer the operator one their machine can reach.
func hostAddresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var v4, v6 []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			network, ok := addr.(*net.IPNet)
			if !ok || !network.IP.IsGlobalUnicast() {
				continue
			}
			if network.IP.To4() != nil {
				v4 = append(v4, network.IP.String())
			} else {
				v6 = append(v6, network.IP.String())
			}
		}
	}
	return append(v4, v6...)
}
