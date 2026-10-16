// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTunnelConsoleFollowsTheDeclaration(t *testing.T) {
	vm := []string{"vnc", "serial", "spice"}
	if err := checkTunnelConsole(vm, "vnc"); err != nil {
		t.Fatalf("vm vnc refused: %v", err)
	}
	if err := checkTunnelConsole(vm, "spice"); err == nil || !strings.Contains(err.Error(), "only vnc") {
		t.Fatalf("spice: got %v, want a refusal naming vnc", err)
	}
	if err := checkTunnelConsole([]string{"tty", "exec"}, "vnc"); err == nil || !strings.Contains(err.Error(), "declared: tty, exec") {
		t.Fatalf("undeclared vnc: got %v, want a refusal naming what is declared", err)
	}
}

// gateway stands in for /api/compute/console/{ticket}: it greets with the RFB banner,
// records what the client sends, and optionally ends with a JSON error frame.
func gateway(t *testing.T, got chan<- []byte, failWith string) *websocket.Conn {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { warnErr(conn.Close()) }()
		if failWith != "" {
			warnErr(conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","data":"`+failWith+`"}`)))
			return
		}
		warnErr(conn.WriteMessage(websocket.BinaryMessage, []byte("RFB 003.008\n")))
		kind, msg, err := conn.ReadMessage()
		if err == nil && kind == websocket.BinaryMessage {
			got <- msg
		}
		warnErr(conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")))
	}))
	t.Cleanup(srv.Close)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if resp != nil {
		warnErr(resp.Body.Close())
	}
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestPipeConsoleCarriesRFBBothWays(t *testing.T) {
	got := make(chan []byte, 1)
	conn := gateway(t, got, "")
	local, client := net.Pipe()

	done := make(chan error, 1)
	go func() { done <- pipeConsole(context.Background(), conn, local) }()

	banner := make([]byte, 12)
	if _, err := io.ReadFull(client, banner); err != nil || string(banner) != "RFB 003.008\n" {
		t.Fatalf("client read %q (%v), want the RFB banner", banner, err)
	}
	if _, err := client.Write([]byte("RFB 003.008\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if string(msg) != "RFB 003.008\n" {
			t.Fatalf("gateway got %q, want the client's version as one binary frame", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the client's bytes never reached the gateway")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a normal close ended the pipe with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the pipe did not end when the gateway closed")
	}
}

func TestPipeConsoleReportsTheGatewaysError(t *testing.T) {
	conn := gateway(t, nil, "guest is not running")
	local, client := net.Pipe()
	defer func() { warnErr(client.Close()) }()

	err := pipeConsole(context.Background(), conn, local)
	var cliErr *Error
	if !errors.As(err, &cliErr) || cliErr.Code != ExitRemote || !strings.Contains(cliErr.Message, "guest is not running") {
		t.Fatalf("got %v, want the gateway's message as a remote error", err)
	}
}

func TestTunnelWarnsOnlyOffLoopback(t *testing.T) {
	if !isLoopback(&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5900}) {
		t.Fatal("127.0.0.1 is loopback")
	}
	if isLoopback(&net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 5900}) {
		t.Fatal("0.0.0.0 accepts other machines and must be warned about")
	}
}
