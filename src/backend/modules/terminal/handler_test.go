// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package terminal

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// bridgeServer runs bridgeExec behind a real WebSocket, with dockerSide as the
// exec stream, and reports when the bridge returns.
func bridgeServer(t *testing.T, dockerSide net.Conn) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	h := &Handler{app: &router.AppDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	done := make(chan struct{})
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { closeutil.Report(c.Close(), "c") }()
		h.bridgeExec(r.Context(), c, nil, "exec1", dockerSide, bufio.NewReader(dockerSide))
		close(done)
	}))
	t.Cleanup(srv.Close)
	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil && resp.Body != nil {
		closeutil.Report(resp.Body.Close(), "resp.Body")
	}
	t.Cleanup(func() { closeutil.Report(client.Close(), "client") })
	return client, done
}

func TestSessionClosesWhenTheShellExits(t *testing.T) {
	docker, shell := net.Pipe()
	client, done := bridgeServer(t, docker)

	go func() {
		if _, err := shell.Write([]byte("bye\r\n")); err != nil {
			return
		}
		closeutil.Report(shell.Close(), "shell")
	}()

	if err := client.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, msg, err := client.ReadMessage()
	if err != nil || string(msg) != "bye\r\n" {
		t.Fatalf("output %q, %v", msg, err)
	}
	_, _, err = client.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatalf("after the shell exited the client read %v, want a normal close", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the bridge is still running after the shell exited")
	}
}

func TestSessionReleasesTheShellWhenTheClientLeaves(t *testing.T) {
	docker, shell := net.Pipe()
	client, done := bridgeServer(t, docker)

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	err := shell.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err == nil {
		_, err = shell.Read(make([]byte, 1))
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("the exec stream was not closed when the client left: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the bridge is still running after the client left")
	}
}
