// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
)

// defaultTunnelListen is where a VNC client is pointed: loopback, so the console is
// reachable only from this machine, and 5900, the port every VNC client assumes.
const defaultTunnelListen = "127.0.0.1:5900"

// pluginTunnelConsoles are the framebuffer consoles the CLI can hand to a native client
// over a local port. SPICE is absent: it opens one socket per channel, which a single
// listener cannot map onto tickets.
var pluginTunnelConsoles = map[string]bool{"vnc": true}

// checkTunnelConsole refuses a tunnel the guest kind does not declare, naming what it does.
func checkTunnelConsole(declared []string, kind string) error {
	if !pluginTunnelConsoles[kind] {
		return usageError("a %s console cannot be tunnelled; only vnc can", kind)
	}
	if !slices.Contains(declared, kind) {
		return usageError("this guest kind declares no %s console (declared: %s)", kind, consoleList(declared))
	}
	return nil
}

// tunnel serves a VNC console on a local port until interrupted. Each client
// connection redeems its own ticket, because a ticket is single-use and 30 seconds
// old: one minted up front would be spent or stale by the time a client connects.
func (e *pluginExecutor) tunnel(ctx context.Context, a *API, env, guestID string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", e.listen)
	if err != nil {
		return usageError("listening on %s: %v (pass --listen 127.0.0.1:5901 for another port)", e.listen, err)
	}
	// goroutine lifecycle: ends with ctx, and closing the listener ends Accept below.
	go func() {
		<-ctx.Done()
		warnErr(ln.Close())
	}()

	emitf(e.g.stderr, "VNC console of %s on %s. Point a VNC client at it; it needs no password. Ctrl-C stops.\n",
		guestID, ln.Addr())
	if !isLoopback(ln.Addr()) {
		emitf(e.g.stderr, "warning: %s is reachable from other machines, and anyone who reaches it gets this console without signing in.\n",
			ln.Addr())
	}

	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		client, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return transportError("vncTunnel", err)
		}
		wg.Add(1)
		// goroutine lifecycle: serveTunnelClient returns when either side closes or ctx ends.
		go func() {
			defer wg.Done()
			if err := e.serveTunnelClient(ctx, a, env, guestID, client); err != nil {
				emitf(e.g.stderr, "vnc client %s: %v\n", client.RemoteAddr(), err)
			}
		}()
	}
}

// serveTunnelClient connects one VNC client to the guest.
func (e *pluginExecutor) serveTunnelClient(ctx context.Context, a *API, env, guestID string, client net.Conn) error {
	defer func() { warnErr(client.Close()) }()
	ticket, err := e.consoleTicket(ctx, a, env, guestID)
	if err != nil {
		return err
	}
	conn, err := a.dialConsole(ctx, ticket)
	if err != nil {
		return err
	}
	defer func() { warnErr(conn.Close()) }()
	emitf(e.g.stderr, "vnc client %s connected\n", client.RemoteAddr())
	return pipeConsole(ctx, conn, client)
}

// pipeConsole copies RFB both ways: client bytes as binary frames, guest frames as
// bytes. A text frame from the gateway is its JSON error, reported rather than
// written into the client's protocol stream.
func pipeConsole(ctx context.Context, conn *websocket.Conn, client io.ReadWriteCloser) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// goroutine lifecycle: ends with ctx; closing both ends unblocks the reads below.
	go func() {
		<-ctx.Done()
		warnErr(conn.Close())
		warnErr(client.Close())
	}()
	// goroutine lifecycle: ends when the client closes or a write to the gateway fails.
	go func() {
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := client.Read(buf)
			if n > 0 && conn.WriteMessage(websocket.BinaryMessage, buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		kind, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) || errors.Is(err, io.EOF) {
				return nil
			}
			return transportError("computeConsoleWS", err)
		}
		if kind == websocket.TextMessage {
			var f terminalFrame
			if json.Unmarshal(msg, &f) == nil && f.Type == "error" {
				return &Error{Code: ExitRemote, Operation: "computeConsoleWS", Message: f.Data}
			}
			continue
		}
		if _, err := client.Write(msg); err != nil {
			return nil
		}
	}
}

// isLoopback reports whether addr only accepts connections from this machine.
func isLoopback(addr net.Addr) bool {
	tcp, ok := addr.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}

// tunnelHelp is the --listen help, shared by the flag and its tests.
func tunnelHelp() string {
	return fmt.Sprintf("with --console vnc: local address a VNC client connects to (default %s)", defaultTunnelListen)
}
