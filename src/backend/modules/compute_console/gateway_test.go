// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compute_console

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// endedStream is a console the guest side has already ended, with a reason.
type endedStream struct{ done chan struct{} }

func (s endedStream) Output() <-chan []byte { return make(chan []byte) }
func (endedStream) Send([]byte) error       { return nil }
func (endedStream) Resize(int, int) error   { return nil }
func (s endedStream) Done() <-chan struct{} { return s.done }
func (endedStream) Err() string             { return "guest powered off" }
func (endedStream) Close() error            { return nil }

// recordingGateway records the one console the gateway asks the bridge to open.
type recordingGateway struct {
	mu    sync.Mutex
	envID string
	req   agentrpc.ConsoleRequest
}

func (*recordingGateway) Available(context.Context, string) (bool, string) { return true, "" }

func (g *recordingGateway) OpenConsole(_ context.Context, envID string, req agentrpc.ConsoleRequest) (agentrpc.ConsoleStream, error) {
	g.mu.Lock()
	g.envID, g.req = envID, req
	g.mu.Unlock()
	done := make(chan struct{})
	close(done)
	return endedStream{done: done}, nil
}

// TestRedeemedTicketOpensExactlyItsConsole drives the upgrade end to end: the bridge is asked
// for the ticket's environment, guest and console and nothing else (an exec command is never
// taken from a client), and the guest side's reason reaches the browser as the close frame.
func TestRedeemedTicketOpensExactlyItsConsole(t *testing.T) {
	tickets, _ := store(t)
	gateway := &recordingGateway{}
	h := &Handler{
		app:     &router.AppDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		tickets: tickets,
		gateway: gateway,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
			return httpx.IsAllowedWebSocketOrigin(r, nil, false)
		}},
	}
	value, err := tickets.Issue(context.Background(), Ticket{
		UserID: "u1", EnvID: "env-7", GuestKind: "ct", GuestID: "c9", Kind: "exec",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	mux.Get("/console/{ticket}", h.HandleWS)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/console/"+value,
		http.Header{"Origin": {srv.URL}})
	if resp != nil && resp.Body != nil {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Log(cerr)
		}
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer teardownBody(t, conn)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Text != "guest powered off" {
		t.Fatalf("want the guest's reason as the close frame, got %v", err)
	}

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	want := agentrpc.ConsoleRequest{Kind: "exec", GuestKind: "ct", GuestID: "c9"}
	if gateway.envID != "env-7" || gateway.req.Kind != want.Kind || gateway.req.GuestKind != want.GuestKind ||
		gateway.req.GuestID != want.GuestID || gateway.req.Command != nil {
		t.Fatalf("opened %s %+v, want env-7 %+v", gateway.envID, gateway.req, want)
	}
}

func teardownBody(t *testing.T, body io.Closer) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Log(err)
	}
}
