// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// fakeAgent is a remote agent that echoes what a real one would: exec and console input
// come back as output, a shell names its exec id first, host logs stream and end, and a
// Docker ping answers 200. Everything it receives is logged in arrival order.
type fakeAgent struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	log    []string
	conn   *websocket.Conn
	wmu    sync.Mutex
	gotLog chan string
	// hook, when set, sees every message first and reports whether it handled it.
	hook func(fa *fakeAgent, msg agent.WSMessage) bool
}

// newFakeAgentWith starts a fake agent; hook, when set, is installed before the agent can
// receive anything.
func newFakeAgentWith(t *testing.T, hook func(fa *fakeAgent, msg agent.WSMessage) bool) *fakeAgent {
	t.Helper()
	fa := &fakeAgent{t: t, gotLog: make(chan string, 100000), hook: hook}
	upgrader := websocket.Upgrader{}
	fa.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("fake agent upgrade: %v", err)
			return
		}
		fa.mu.Lock()
		fa.conn = conn
		fa.mu.Unlock()
		defer closeIgnoring(conn)
		for {
			var msg agent.WSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			fa.handle(msg)
		}
	}))
	t.Cleanup(fa.srv.Close)
	return fa
}

func (fa *fakeAgent) record(entry string) {
	fa.mu.Lock()
	fa.log = append(fa.log, entry)
	fa.mu.Unlock()
	fa.gotLog <- entry
}

func (fa *fakeAgent) send(msg agent.WSMessage) {
	fa.wmu.Lock()
	defer fa.wmu.Unlock()
	fa.mu.Lock()
	conn := fa.conn
	fa.mu.Unlock()
	if err := conn.WriteJSON(msg); err != nil {
		fa.t.Logf("fake agent write: %v", err)
	}
}

func (fa *fakeAgent) handle(msg agent.WSMessage) {
	if fa.hook != nil && fa.hook(fa, msg) {
		return
	}
	switch msg.Type {
	case agent.MsgExecStart:
		fa.record("exec_start:" + msg.ID)
	case agent.MsgExecInput:
		fa.record("input:" + string(msg.StreamChunk.Data))
		fa.send(agent.WSMessage{Type: agent.MsgExecOutput, ID: msg.ID, StreamChunk: &agent.WSStreamChunk{Data: msg.StreamChunk.Data}})
	case agent.MsgExecResize:
		fa.record(fmt.Sprintf("resize:%dx%d", msg.ExecResize.Cols, msg.ExecResize.Rows))
	case agent.MsgExecEnd:
		fa.record("exec_end:" + msg.ID)
	case agent.MsgConsoleStart:
		fa.record("console_start:" + msg.ID)
		// The RFB banner goes first, before anything is asked of it.
		fa.send(agent.WSMessage{Type: agent.MsgConsoleOutput, ID: msg.ID, Console: &agent.ConsolePayload{SessionID: msg.ID, Data: []byte("RFB 003.008\n")}})
	case agent.MsgConsoleInput:
		fa.send(agent.WSMessage{Type: agent.MsgConsoleOutput, ID: msg.ID, Console: &agent.ConsolePayload{SessionID: msg.ID, Data: msg.Console.Data}})
	case agent.MsgConsoleEnd:
		fa.record("console_end:" + msg.ID)
	case agent.MsgHostTerminalStart:
		fa.record("host_terminal_start:" + msg.ID)
		fa.send(agent.WSMessage{Type: agent.MsgHostTerminalOutput, ID: msg.ID, StreamChunk: &agent.WSStreamChunk{Data: []byte("MCHARBOR_EXEC_ID=shell-1\n$ ")}})
	case agent.MsgHostTerminalInput:
		fa.record("host_terminal_input:" + string(msg.StreamChunk.Data))
	case agent.MsgHostTerminalEnd:
		fa.record("host_terminal_end:" + msg.HostTerminalEnd.ExecID)
	case agent.MsgHostLogRequest:
		for i := 0; i < 3; i++ {
			fa.send(agent.WSMessage{Type: agent.MsgHostLogChunk, ID: msg.ID, HostLogChunk: &agent.HostLogChunkPayload{Line: fmt.Sprintf("line-%d", i)}})
		}
		fa.send(agent.WSMessage{Type: agent.MsgHostLogEnd, ID: msg.ID, HostLogEnd: &agent.HostLogEndPayload{Notices: []string{"done"}}})
	case agent.MsgHostStatsRequest:
		fa.send(agent.WSMessage{Type: agent.MsgHostStatsResponse, ID: msg.ID, HostStats: &agent.HostStatsPayload{CPUPercent: 42, Supported: true}})
	case agent.MsgHTTPRequest, agent.MsgHTTPRequestStart:
		fa.send(agent.WSMessage{Type: agent.MsgHTTPResponse, ID: msg.ID, HTTPResponse: &agent.WSHTTPResponse{StatusCode: 200, Body: []byte("OK")}})
	}
}

// waitFor waits until entry is in the agent's log.
func (fa *fakeAgent) waitFor(t *testing.T, entry string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for fa.count(entry) == 0 {
		select {
		case <-fa.gotLog:
		case <-deadline:
			t.Fatalf("the agent never received %q; it saw %v", entry, fa.snapshot())
		}
	}
}

func (fa *fakeAgent) snapshot() []string {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	return append([]string(nil), fa.log...)
}

func (fa *fakeAgent) count(entry string) int {
	n := 0
	for _, got := range fa.snapshot() {
		if got == entry {
			n++
		}
	}
	return n
}

// cluster is two in-process nodes sharing one database: node-a holds the agent socket,
// node-b holds nothing and must reach the agent through node-a.
type cluster struct {
	agent   *fakeAgent
	pool    *agent.AgentPool
	owner   *Link
	serving *Link
	ownerAt *httptest.Server
	hub     *statshub.Hub
}

func newCluster(t *testing.T) *cluster {
	t.Helper()
	return newClusterWith(t, nil)
}

// newClusterWith is newCluster whose agent runs hook before its own handling.
func newClusterWith(t *testing.T, hook func(fa *fakeAgent, msg agent.WSMessage) bool) *cluster {
	t.Helper()
	fa := newFakeAgentWith(t, hook)
	ws, _, err := dialWS(nil, "ws"+strings.TrimPrefix(fa.srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial fake agent: %v", err)
	}
	conn := &agent.AgentConnection{EnvID: "env-1", Hostname: "host-1", Version: "2026.09.01", Build: "209901010000", Conn: ws}
	transport := agent.NewAgentTransport(conn, nil, quiet())
	conn.Transport = transport
	pool := agent.NewAgentPool(quiet())
	pool.Register("env-1", conn)
	go func() { ignoreErr(transport.ReadLoop()) }()

	database := migrated(t)
	c := &cluster{agent: fa, pool: pool}
	c.hub = statshub.New(statshub.Options{Pull: func(context.Context, string) ([]statshub.Sample, error) {
		return []statshub.Sample{{ID: "pulled-on-a"}}, nil
	}})
	t.Cleanup(c.hub.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/internal/node/agent-stream/", func(w http.ResponseWriter, r *http.Request) {
		c.owner.ServeAgentStream(w, r, strings.TrimPrefix(r.URL.Path, "/api/internal/node/agent-stream/"))
	})
	mux.HandleFunc("/api/internal/node/agent-rpc/", func(w http.ResponseWriter, r *http.Request) {
		c.owner.ServeAgentRPC(w, r, strings.TrimPrefix(r.URL.Path, "/api/internal/node/agent-rpc/"))
	})
	mux.HandleFunc("/api/internal/node/stats/", func(w http.ResponseWriter, r *http.Request) {
		c.owner.ServeStats(w, r, strings.TrimPrefix(r.URL.Path, "/api/internal/node/stats/"), c.hub)
	})
	c.ownerAt = httptest.NewServer(mux)
	t.Cleanup(c.ownerAt.Close)

	c.owner = New(NewOwners(database, "node-a", c.ownerAt.URL), NewSigner("shared-secret"), pool, quiet())
	c.serving = New(NewOwners(database, "node-b", "http://node-b:8705"), NewSigner("shared-secret"), noAgents{}, quiet())
	c.serving.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	c.owner.SetRequireTLS(false)   // and the owner accepts the httptest peer's plaintext calls
	c.owner.SetPeers(testPeers)
	claimFor(t, database, "env-1", "node-a", c.ownerAt.URL)
	t.Cleanup(func() {
		c.serving.CloseStreams()
		c.owner.CloseStreams()
		ignoreErr(ws.Close())
	})
	return c
}

func (c *cluster) remote(t *testing.T) *agent.AgentConnection {
	t.Helper()
	conn, ok := c.serving.RemoteAgent("env-1")
	if !ok {
		t.Fatalf("node-b must reach the agent node-a holds; stream proof %+v", c.serving.StreamProof())
	}
	return conn
}

func recv(t *testing.T, ch <-chan []byte, what string) []byte {
	t.Helper()
	select {
	case data, ok := <-ch:
		if !ok {
			t.Fatalf("%s: the channel closed", what)
		}
		return data
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: nothing arrived", what)
	}
	return nil
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

// TestStreamRelayCarriesExecBothWaysInOrder is the exec contract: every keystroke reaches
// the agent in the order typed, a resize keeps its place among them, output comes back in
// order, and a session the agent ends ends on the asking node with no error attached.
func TestStreamRelayCarriesExecBothWaysInOrder(t *testing.T) {
	c := newCluster(t)
	conn := c.remote(t)
	if conn.Version != "2026.09.01" || conn.Hostname != "host-1" {
		t.Fatalf("the virtual connection must carry the agent's identity for feature gates, got %q/%q", conn.Version, conn.Hostname)
	}

	session := conn.Transport.StartExecSession("s1")
	if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecStart, ID: "s1", ExecStart: &agent.ExecStartPayload{ExecID: "e1"}}); err != nil {
		t.Fatal(err)
	}
	const inputs = 60
	for i := 0; i < inputs; i++ {
		if i == inputs/2 {
			if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecResize, ID: "s1", ExecResize: &agent.ExecResizePayload{ExecID: "e1", Cols: 120, Rows: 40}}); err != nil {
				t.Fatal(err)
			}
		}
		if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecInput, ID: "s1", StreamChunk: &agent.WSStreamChunk{Data: []byte(fmt.Sprintf("k%02d", i))}}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < inputs; i++ {
		if got, want := string(recv(t, session.OutputCh, "exec output")), fmt.Sprintf("k%02d", i); got != want {
			t.Fatalf("output %d arrived as %q, want %q: the relay reordered the stream", i, got, want)
		}
	}

	var want []string
	want = append(want, "exec_start:s1")
	for i := 0; i < inputs; i++ {
		if i == inputs/2 {
			want = append(want, "resize:120x40")
		}
		want = append(want, fmt.Sprintf("input:k%02d", i))
	}
	got := c.agent.snapshot()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the agent must see the session exactly as typed\n got %v\nwant %v", got, want)
	}

	c.agent.send(agent.WSMessage{Type: agent.MsgExecEnd, ID: "s1"})
	waitClosed(t, session.DoneCh, "the agent ending the shell ending the relayed session")
	if session.Reason() != "" {
		t.Fatalf("a shell the agent ended is a clean end, got reason %q", session.Reason())
	}
}

// TestStreamRelayFreesTheAgentWhenTheAskingNodeDrops is cancellation from the asking side:
// a node that vanishes with sessions open gets them ended on the agent by the owner, once,
// and a session it already ended is not ended again.
func TestStreamRelayFreesTheAgentWhenTheAskingNodeDrops(t *testing.T) {
	c := newCluster(t)
	conn := c.remote(t)

	conn.Transport.StartExecSession("open-1")
	if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecStart, ID: "open-1", ExecStart: &agent.ExecStartPayload{ExecID: "e1"}}); err != nil {
		t.Fatal(err)
	}
	conn.Transport.StartExecSession("closed-1")
	if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecStart, ID: "closed-1", ExecStart: &agent.ExecStartPayload{ExecID: "e2"}}); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecEnd, ID: "closed-1"}); err != nil {
		t.Fatal(err)
	}
	waiter := make(chan *agent.WSMessage, 8)
	conn.Transport.HostTerminalChannel().Set("term-1", waiter)
	if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgHostTerminalStart, ID: "term-1", HostTerminal: &agent.HostTerminalStartPayload{Cols: 80, Rows: 24}}); err != nil {
		t.Fatal(err)
	}
	select {
	case first := <-waiter:
		if first == nil || !strings.HasPrefix(string(first.StreamChunk.Data), "MCHARBOR_EXEC_ID=shell-1") {
			t.Fatalf("the shell's first frame must reach the asking node, got %+v", first)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the host terminal's first frame never arrived")
	}
	if err := conn.Transport.HostTerminalSendInput("shell-1", []byte("uptime")); err != nil {
		t.Fatal(err)
	}
	c.agent.waitFor(t, "host_terminal_input:MCHARBOR_EXEC_ID=shell-1\nuptime")
	c.agent.waitFor(t, "exec_end:closed-1")

	// node-b dies: no close frame, no end frames of its own.
	ignoreErr(conn.Conn.Close())

	c.agent.waitFor(t, "exec_end:open-1")
	c.agent.waitFor(t, "host_terminal_end:shell-1")
	time.Sleep(200 * time.Millisecond)
	if n := c.agent.count("exec_end:closed-1"); n != 1 {
		t.Fatalf("a session the asking node ended must be ended once, got %d end frames", n)
	}
	if taps := c.pool.Transport("env-1").RelayTapCount(); taps != 0 {
		t.Fatalf("a dropped link must release every id it routed, %d are still tapped", taps)
	}
}

// TestStreamRelayEndsSessionsWithTheReasonWhenTheAgentLeaves is the lease moving mid-session:
// the agent leaves node-a, and the shell open on node-b ends at once, saying so, instead
// of hanging on a relay that will never deliver another byte.
func TestStreamRelayEndsSessionsWithTheReasonWhenTheAgentLeaves(t *testing.T) {
	c := newCluster(t)
	conn := c.remote(t)
	session := conn.Transport.StartExecSession("s1")
	if err := conn.WriteJSON(agent.WSMessage{Type: agent.MsgExecStart, ID: "s1", ExecStart: &agent.ExecStartPayload{ExecID: "e1"}}); err != nil {
		t.Fatal(err)
	}
	c.agent.waitFor(t, "exec_start:s1")

	// The agent drops its socket to node-a, as it does when it reconnects elsewhere.
	c.agent.mu.Lock()
	ignoreErr(c.agent.conn.Close())
	c.agent.mu.Unlock()

	waitClosed(t, session.DoneCh, "the relayed session ending when the agent left the owning node")
	if reason := session.Reason(); !strings.Contains(reason, "left node node-a") {
		t.Fatalf("the session must say the agent left node-a, got %q", reason)
	}
	waitClosed(t, conn.Transport.Done(), "the virtual connection ending")
	if _, ok := c.serving.RemoteAgent("env-1"); ok {
		t.Fatal("a node that no longer holds the agent must refuse a new stream rather than relay into nothing")
	}
}

// TestStreamRelayBoundsASlowReader is the backpressure contract: an asking node that stops
// reading costs the owning node a bounded queue and dropped output, never unbounded memory
// and never a stalled agent socket.
func TestStreamRelayBoundsASlowReader(t *testing.T) {
	c := newCluster(t)

	// A node that greets and then never reads again.
	header := http.Header{}
	signed := &http.Request{Header: header}
	empty := sha256.Sum256(nil)
	mustSign(t, c.serving.signer, signed, Claims{Hop: HopAgentStream, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodGet, Path: streamClaimPath},
		xid.New().String(), hex.EncodeToString(empty[:]))
	dialer := websocket.Dialer{ReadBufferSize: 1024}
	stalled, _, err := dialWS(&dialer, "ws"+strings.TrimPrefix(c.ownerAt.URL, "http")+"/api/internal/node/agent-stream/env-1", header)
	if err != nil {
		t.Fatalf("dial the stream: %v", err)
	}
	defer closeIgnoring(stalled)
	var greeting hello
	if err := stalled.ReadJSON(&greeting); err != nil {
		t.Fatal(err)
	}
	if err := stalled.WriteJSON(agent.WSMessage{Type: agent.MsgExecStart, ID: "flood", ExecStart: &agent.ExecStartPayload{ExecID: "e1"}}); err != nil {
		t.Fatal(err)
	}
	c.agent.waitFor(t, "exec_start:flood")

	chunk := []byte(strings.Repeat("x", 16*1024))
	const frames = 3000 // ~48 MiB, far past every socket buffer between the nodes
	flooded := make(chan struct{})
	go func() {
		defer close(flooded)
		for i := 0; i < frames; i++ {
			c.agent.send(agent.WSMessage{Type: agent.MsgExecOutput, ID: "flood", StreamChunk: &agent.WSStreamChunk{Data: chunk}})
		}
	}()
	select {
	case <-flooded:
	case <-time.After(30 * time.Second):
		t.Fatal("the agent could not finish writing: a relay whose reader stopped stalled the agent socket")
	}

	// The agent socket is still served while the relay is saturated.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stats, supported, err := c.pool.Transport("env-1").HostStats(ctx)
	if err != nil || !supported || stats.CPUPercent != 42 {
		t.Fatalf("a saturated relay must not stall the agent socket: %+v %v %v", stats, supported, err)
	}

	relays := c.owner.Relays()
	if len(relays) != 1 {
		t.Fatalf("want one relay, got %+v", relays)
	}
	r := relays[0]
	if r.MaxDepth > relayQueue {
		t.Fatalf("the relay queue grew to %d frames, past its bound of %d", r.MaxDepth, relayQueue)
	}
	if r.Dropped == 0 {
		t.Fatalf("a reader that never reads must cost dropped output, not memory: %+v", r)
	}
}

// TestStreamRelayCarriesConsoleHostLogsAndMessages covers the other relayed paths: the
// console's banner that arrives before anything is asked, host logs, and a request/reply
// agent message.
func TestStreamRelayCarriesConsoleHostLogsAndMessages(t *testing.T) {
	c := newCluster(t)
	conn := c.remote(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	console, err := conn.Transport.StartConsole(ctx, agent.ConsoleKindVNC, "vm", "guest-1", nil, 0, 0)
	if err != nil {
		t.Fatalf("open a relayed console: %v", err)
	}
	if banner := recv(t, console.OutputCh, "the RFB banner"); string(banner) != "RFB 003.008\n" {
		t.Fatalf("the console's first bytes must survive the relay, got %q", banner)
	}
	if err := console.Send([]byte("client-hello")); err != nil {
		t.Fatal(err)
	}
	if echo := recv(t, console.OutputCh, "console echo"); string(echo) != "client-hello" {
		t.Fatalf("console input must round-trip, got %q", echo)
	}
	if err := console.Close(); err != nil {
		t.Fatal(err)
	}
	c.agent.waitFor(t, "console_end:"+console.ID)

	stream, err := conn.Transport.HostLogs(ctx, agent.HostLogRequestPayload{Source: "system", Tail: 3})
	if err != nil {
		t.Fatalf("relayed host logs: %v", err)
	}
	var lines []string
	for line := range stream.Lines {
		lines = append(lines, line)
	}
	if strings.Join(lines, ",") != "line-0,line-1,line-2" || len(stream.Notices) != 1 {
		t.Fatalf("host logs must arrive whole and in order, got %v notices %v (err %v)", lines, stream.Notices, stream.Err())
	}

	stats, supported, err := conn.Transport.HostStats(ctx)
	if err != nil || !supported || stats.CPUPercent != 42 {
		t.Fatalf("a request/reply agent message must be relayed: %+v %v %v", stats, supported, err)
	}
}

// TestStreamRefusesWhatIsNotANodeStream keeps the relay a node-only door: no signature, a
// signature for another hop, or a node that does not hold the agent all refuse.
func TestStreamRefusesWhatIsNotANodeStream(t *testing.T) {
	c := newCluster(t)
	target := "ws" + strings.TrimPrefix(c.ownerAt.URL, "http") + "/api/internal/node/agent-stream/env-1"

	if _, status, err := dialWS(nil, target, nil); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("an unsigned stream must be refused with 401, got %v %v", status, err)
	}

	header := http.Header{}
	empty := sha256.Sum256(nil)
	mustSign(t, c.serving.signer, &http.Request{Header: header}, Claims{Hop: HopAgentRPC, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodPost, Path: "/v1.43/containers/json"},
		xid.New().String(), hex.EncodeToString(empty[:]))
	if _, status, err := dialWS(nil, target, header); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("an agent-rpc signature must not open a stream, got %v %v", status, err)
	}

	other := strings.Replace(target, "env-1", "env-2", 1)
	header = http.Header{}
	mustSign(t, c.serving.signer, &http.Request{Header: header}, Claims{Hop: HopAgentStream, NodeID: "node-b", Audience: "node-a", EnvID: "env-2", Method: http.MethodGet, Path: streamClaimPath},
		xid.New().String(), hex.EncodeToString(empty[:]))
	if _, status, err := dialWS(nil, other, header); err == nil || status != http.StatusConflict {
		t.Fatalf("a node that does not hold the agent must answer 409, got %v %v", status, err)
	}

	header = http.Header{}
	mustSign(t, c.serving.signer, &http.Request{Header: header}, Claims{Hop: HopAgentStream, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodGet, Path: streamClaimPath},
		xid.New().String(), hex.EncodeToString(empty[:]))
	header.Set("Origin", c.ownerAt.URL)
	if _, status, err := dialWS(nil, target, header); err == nil || status != http.StatusForbidden {
		t.Fatalf("a correctly signed stream carrying an Origin is a browser and must answer 403, got %v %v", status, err)
	}

	rpc := c.ownerAt.URL + "/api/internal/node/agent-rpc/env-1"
	req, err := http.NewRequest(http.MethodPost, rpc, nil)
	if err != nil {
		t.Fatalf("building the agent-rpc request: %v", err)
	}
	req.Header.Set("Origin", c.ownerAt.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("agent-rpc request failed: %v", err)
	}
	defer closeIgnoring(resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("agent-rpc carrying an Origin must answer 403 before the signature, got %d", resp.StatusCode)
	}
}

// TestProbeProvesTheStreamRelay is what the readiness gate reads: the routing check pings
// the agent through the session relay, and only a ping that came back is proof.
func TestProbeProvesTheStreamRelay(t *testing.T) {
	c := newCluster(t)
	results := c.serving.Probe(context.Background())
	if len(results) != 1 || !results[0].OK || !results[0].StreamOK {
		t.Fatalf("both hops must be proven by the check, got %+v", results)
	}
	if p := c.serving.StreamProof(); p.Status != ProofProven || p.NodeID != "node-a" {
		t.Fatalf("a ping that came back through the relay is proof, got %+v", p)
	}
}

// TestRemoteSnapshotReadsTheOwnersStats is the stats half: the node that does not hold the
// agent reads the owner's hub rather than sweeping every container through the hop.
func TestRemoteSnapshotReadsTheOwnersStats(t *testing.T) {
	c := newCluster(t)
	c.hub.Push("env-1", statshub.Batch{Session: "s", Seq: 1, SampledAt: time.Now(), Samples: []statshub.Sample{{ID: "pushed-to-a"}}})

	snap, ok, err := c.serving.RemoteSnapshot(context.Background(), "env-1")
	if err != nil || !ok {
		t.Fatalf("node-b must read node-a's stats: ok=%v err=%v", ok, err)
	}
	if len(snap.Samples) != 1 || snap.Samples[0].ID != "pushed-to-a" || snap.Mode != statshub.ModePush {
		t.Fatalf("node-b must get the snapshot node-a's agent pushed, got %+v", snap)
	}
	if _, ok, err := c.owner.RemoteSnapshot(context.Background(), "env-1"); err != nil || ok {
		t.Fatal("the node holding the agent must read its own hub, not ask another node")
	}
}
