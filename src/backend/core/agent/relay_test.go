// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// scriptedAgent writes the frames it is handed, then closes with closeText when one is set.
func scriptedAgent(t *testing.T, frames []WSMessage, closeText string, release <-chan struct{}) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer closeIgnoring(conn)
		<-release
		for _, f := range frames {
			if err := conn.WriteJSON(f); err != nil {
				return
			}
		}
		if closeText != "" {
			ignoreErr(conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, closeText), time.Now().Add(time.Second)))
		}
		time.Sleep(200 * time.Millisecond)
	}))
}

type recordingTap struct{ got chan *WSMessage }

func (r *recordingTap) DeliverRelay(_ []byte, msg *WSMessage) { r.got <- msg }

func dialScripted(t *testing.T, srv *httptest.Server) *AgentTransport {
	t.Helper()
	conn, _, err := dialWS("ws" + strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ignoreErr(conn.Close()) })
	return NewAgentTransport(&AgentConnection{EnvID: "env-1", Version: "2026.09.01", Conn: conn}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestRelayTapTakesItsFramesAndNothingElse is the owning node's half of the relay: a tapped
// id's frames go to the other node and never into this node's own session, while an
// untapped session on the same socket is served locally as before.
func TestRelayTapTakesItsFramesAndNothingElse(t *testing.T) {
	release := make(chan struct{})
	srv := scriptedAgent(t, []WSMessage{
		{Type: MsgExecOutput, ID: "relayed", StreamChunk: &WSStreamChunk{Data: []byte("to-node-b")}},
		{Type: MsgExecOutput, ID: "local", StreamChunk: &WSStreamChunk{Data: []byte("stays-here")}},
		{Type: MsgTransferResult, Transfer: &TransferPayload{TransferID: "xfer-1"}},
	}, "", release)
	defer srv.Close()
	transport := dialScripted(t, srv)

	tap := &recordingTap{got: make(chan *WSMessage, 4)}
	transport.AddRelayTap("relayed", tap)
	transport.AddRelayTap("xfer-1", tap)
	relayedLocal := transport.StartExecSession("relayed")
	local := transport.StartExecSession("local")
	go func() { ignoreErr(transport.ReadLoop()) }()
	close(release)

	for _, want := range []string{"relayed", "xfer-1"} {
		select {
		case msg := <-tap.got:
			keys := RelayKeys(msg)
			if len(keys) == 0 || keys[len(keys)-1] != want {
				t.Fatalf("the tap got %+v, want the frame keyed %q", msg, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("the frame keyed %q never reached the tap", want)
		}
	}
	select {
	case data := <-local.OutputCh:
		if string(data) != "stays-here" {
			t.Fatalf("the local session got %q", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an untapped session must still be served locally")
	}
	select {
	case data := <-relayedLocal.OutputCh:
		t.Fatalf("a tapped frame must not also reach a local session, got %q", data)
	default:
	}

	transport.RemoveRelayTap("relayed", &recordingTap{})
	if transport.RelayTapCount() != 2 {
		t.Fatal("a relay must not be able to remove another relay's tap")
	}
	transport.RemoveRelayTap("relayed", tap)
	transport.RemoveRelayTap("xfer-1", tap)
	if transport.RelayTapCount() != 0 {
		t.Fatal("a relay must be able to remove its own taps")
	}
}

// TestReadLoopEndsSessionsWithTheCloseReason is what makes a relayed shell say why it
// ended: the far end's close text reaches the exec session and the transport, and a
// shell does not wait forever on a socket that is gone.
func TestReadLoopEndsSessionsWithTheCloseReason(t *testing.T) {
	release := make(chan struct{})
	srv := scriptedAgent(t, nil, "the agent for env-1 left node node-a", release)
	defer srv.Close()
	transport := dialScripted(t, srv)
	session := transport.StartExecSession("s1")
	waiter := make(chan *WSMessage, 1)
	transport.HostTerminalChannel().Set("term-1", waiter)

	loopDone := make(chan struct{})
	go func() { ignoreErr(transport.ReadLoop()); close(loopDone) }()
	close(release)

	select {
	case <-session.DoneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("a session must end when its transport does")
	}
	if got := session.Reason(); got != "the agent for env-1 left node node-a" {
		t.Fatalf("the session must carry the close reason, got %q", got)
	}
	if got := transport.CloseReason(); got != "the agent for env-1 left node node-a" {
		t.Fatalf("the transport must carry the close reason, got %q", got)
	}
	select {
	case msg := <-waiter:
		if msg != nil {
			t.Fatalf("a host terminal must be told with its nil end signal, got %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("a host terminal waiting on a dead transport must be released")
	}
	<-loopDone
	// Close after the read loop ended used to be a double close of done.
	transport.Close()
}
