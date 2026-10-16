// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

func TestProxyErrorResponseIsValidJSON(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		errlog.Close(conn)
	}))
	t.Cleanup(daemon.Close)
	addr := strings.TrimPrefix(daemon.URL, "http://")

	proxy := &Proxy{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		httpClient: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
			},
		}},
	}

	agentSide := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		agentSide <- conn
	}))
	t.Cleanup(wsServer.Close)

	serverSide, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		errlog.Log(resp.Body.Close())
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { errlog.Close(serverSide) })
	conn := <-agentSide
	t.Cleanup(func() { errlog.Close(conn) })

	proxy.HandleRequest(context.Background(), conn, "req-1", &WSHTTPRequest{
		Method: http.MethodGet,
		Path:   "/v1.51/networks/bridge",
	})

	if err := serverSide.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg WSMessage
	if err := serverSide.ReadJSON(&msg); err != nil {
		t.Fatalf("read: %v", err)
	}
	if msg.Type != MsgHTTPResponse || msg.HTTPResponse == nil {
		t.Fatalf("got %q, want an http_response", msg.Type)
	}
	if msg.HTTPResponse.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", msg.HTTPResponse.StatusCode)
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(msg.HTTPResponse.Body, &body); err != nil {
		t.Fatalf("error body is not valid JSON (%v): %s", err, msg.HTTPResponse.Body)
	}
	if !strings.Contains(body.Message, `"http://docker/v1.51/networks/bridge"`) || !strings.Contains(body.Message, "EOF") {
		t.Fatalf("message lost the transport error: %q", body.Message)
	}
}
