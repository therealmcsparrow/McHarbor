// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// burstAgent answers every http_request with start, chunks and end written back to back,
// which is the shape that exposed chunks arriving before RoundTrip had seen the start.
func burstAgent(t *testing.T, chunks [][]byte) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer closeIgnoring(conn)
		for {
			var msg WSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type != MsgHTTPRequest {
				continue
			}
			burst := []WSMessage{{Type: MsgHTTPResponseStart, ID: msg.ID, StreamStart: &WSStreamStart{StatusCode: http.StatusOK}}}
			for _, c := range chunks {
				burst = append(burst, WSMessage{Type: MsgHTTPResponseChunk, ID: msg.ID, StreamChunk: &WSStreamChunk{Data: c}})
			}
			burst = append(burst, WSMessage{Type: MsgHTTPResponseEnd, ID: msg.ID})
			for _, m := range burst {
				if err := conn.WriteJSON(m); err != nil {
					return
				}
			}
		}
	}))
}

// TestStreamingResponseKeepsChunksThatArriveBeforeRoundTripWakes pins the ReadLoop ordering
// constraint: the StreamReader must exist before http_response_start is dispatched, or the
// burst's chunks are dropped and a short response never reaches its end.
func TestStreamingResponseKeepsChunksThatArriveBeforeRoundTripWakes(t *testing.T) {
	chunks := [][]byte{[]byte("alpha-"), []byte("beta-"), []byte("gamma")}
	srv := burstAgent(t, chunks)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := dialWS(wsURL)
	if err != nil {
		t.Fatalf("dial fake agent: %v", err)
	}
	defer closeIgnoring(conn)

	transport := NewAgentTransport(&AgentConnection{EnvID: "env-test", Version: "2026.09.01", Conn: conn}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() { ignoreErr(transport.ReadLoop()) }()

	for i := 0; i < 25; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/c1/logs", nil)
		if err != nil {
			cancel()
			t.Fatalf("build request: %v", err)
		}
		resp, err := transport.RoundTrip(req)
		if err != nil {
			cancel()
			t.Fatalf("round %d: RoundTrip: %v", i, err)
		}
		body, err := io.ReadAll(resp.Body)
		ignoreErr(resp.Body.Close())
		cancel()
		if err != nil {
			t.Fatalf("round %d: reading body: %v (the stream never ended: chunks or the end marker were dropped)", i, err)
		}
		if want := bytes.Join(chunks, nil); !bytes.Equal(body, want) {
			t.Fatalf("round %d: body = %q, want %q (chunks arriving before RoundTrip saw the start were dropped)", i, body, want)
		}
	}
}
