// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/nodelink"
)

const fwdAction = "plugin:addon-demo:demo-trigger"

// fwdCoord is a coordinator whose leadership the test flips by hand.
type fwdCoord struct {
	id     string
	leader atomic.Bool
}

func (c *fwdCoord) LeaderOf(context.Context, string) bool { return c.leader.Load() }
func (c *fwdCoord) NodeID() string                        { return c.id }

type fwdNoAgents struct{}

func (fwdNoAgents) Transport(string) *agent.AgentTransport { return nil }
func (fwdNoAgents) List() []agent.AgentConnection          { return nil }

// fwdNode is one McHarbor node of the test cluster: its trigger service, its link and the
// node-to-node endpoint the other node reaches it on.
type fwdNode struct {
	coord  *fwdCoord
	ts     *TriggerService
	link   *nodelink.Link
	server *httptest.Server
	runs   chan string
}

func (n *fwdNode) runCount(wait time.Duration) int {
	deadline := time.After(wait)
	count := 0
	for {
		select {
		case <-n.runs:
			count++
		case <-deadline:
			return count
		}
	}
}

func fwdDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(db.Config{Driver: db.DriverSQLite, Path: filepath.Join(t.TempDir(), "cluster.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	canvas := `{"nodes":[{"id":"trig","action":"` + fwdAction + `","config":{"vm":"web"}}],"edges":[]}`
	if _, err := database.Exec(`INSERT INTO workflows (id, name, status, canvas_data, created_at, updated_at)
		VALUES ('wf1', 'wf', 'active', ?, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z')`, canvas); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return database
}

func newFwdNode(t *testing.T, database *sql.DB, id string, logger *slog.Logger) *fwdNode {
	t.Helper()
	n := &fwdNode{coord: &fwdCoord{id: id}, runs: make(chan string, 16)}
	n.server = httptest.NewServer(nil)
	t.Cleanup(n.server.Close)
	n.link = nodelink.New(nodelink.NewOwners(database, id, n.server.URL), nodelink.NewSigner("shared-secret"), fwdNoAgents{}, logger)
	n.link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	n.server.Config.Handler = http.HandlerFunc(n.link.ServeTriggerFire)
	n.ts = &TriggerService{
		service:     &Service{db: database, logger: logger},
		logger:      logger,
		coordinator: n.coord,
		nodeID:      id,
		hasCoord:    true,
		fires:       newFireRelay(n.link, database),
		startRun: func(_ context.Context, workflowID, triggerNodeID, _ string, _ Msg) {
			n.runs <- workflowID + ":" + triggerNodeID
		},
	}
	n.ts.fires.first, n.ts.fires.max, n.ts.fires.window = 20*time.Millisecond, 80*time.Millisecond, 2*time.Second
	t.Cleanup(n.ts.fires.close)
	n.link.SetTriggerFireReceiver(n.ts)
	// Every test node is live in the registry, so a fire any of them signs passes the
	// live-instance check; fwdCluster replaces the list for the nodes that forward.
	n.link.SetPeers(func(context.Context) ([]nodelink.Peer, error) {
		return []nodelink.Peer{{NodeID: "node-a", AdvertiseURL: "https://node-a.invalid"},
			{NodeID: "node-b", AdvertiseURL: "https://node-b.invalid"}, {NodeID: "node-c", AdvertiseURL: "https://node-c.invalid"}}, nil
	})
	return n
}

// fwdCluster is two nodes over one shared database, each listing the other as its peer.
func fwdCluster(t *testing.T, logger *slog.Logger) (leader, follower *fwdNode) {
	t.Helper()
	database := fwdDB(t)
	leader = newFwdNode(t, database, "node-a", logger)
	follower = newFwdNode(t, database, "node-b", logger)
	peersOf := func(other *fwdNode) nodelink.PeerSource {
		return func(context.Context) ([]nodelink.Peer, error) {
			return []nodelink.Peer{{NodeID: other.coord.id, AdvertiseURL: other.server.URL}}, nil
		}
	}
	leader.link.SetPeers(peersOf(follower))
	follower.link.SetPeers(peersOf(leader))
	leader.coord.leader.Store(true)
	return leader, follower
}

func acceptAll(PluginTriggerBinding) bool { return true }

// TestFollowerFireStartsExactlyOneRunOnTheLeader is the bug this relay exists for: a
// plugin firing on a node that does not lead the workflow trigger used to start nothing.
func TestFollowerFireStartsExactlyOneRunOnTheLeader(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())

	started, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-demo", fwdAction, map[string]interface{}{"vm": "web"}, acceptAll)
	if err != nil {
		t.Fatalf("fire: %v", err)
	}
	if started != 1 {
		t.Fatalf("the follower's fire should report the leader's one started run, got %d", started)
	}
	if got := leader.runCount(300 * time.Millisecond); got != 1 {
		t.Fatalf("want exactly one run on the leader, got %d", got)
	}
	if got := follower.runCount(50 * time.Millisecond); got != 0 {
		t.Fatalf("the follower must not run the workflow itself, got %d", got)
	}
}

// TestFollowerMatchIsHonouredByTheLeader: the plugin's match closure cannot cross the hop,
// so a binding it rejected on the follower must not start on the leader.
func TestFollowerMatchIsHonouredByTheLeader(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	started, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-demo", fwdAction, nil,
		func(PluginTriggerBinding) bool { return false })
	if err != nil || started != 0 {
		t.Fatalf("a fire no binding matches starts nothing, got %d, %v", started, err)
	}
	if got := leader.runCount(150 * time.Millisecond); got != 0 {
		t.Fatalf("a rejected binding started on the leader: %d runs", got)
	}
}

// TestDuplicateForwardedFireIsDeduplicated: a retry of a fire the leader already
// evaluated (its reply was lost) must not start the workflow a second time.
func TestDuplicateForwardedFireIsDeduplicated(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	fire := nodelink.TriggerFire{
		FireID: "fire-1", ActionKey: fwdAction, Source: "addon-demo", Origin: "node-b",
		Bindings: []nodelink.TriggerFireBinding{{WorkflowID: "wf1", NodeID: "trig"}},
	}
	first, err := follower.link.ForwardTriggerFire(context.Background(), fire)
	if err != nil || first.Started != 1 || first.Duplicate {
		t.Fatalf("first delivery: %+v, %v", first, err)
	}
	second, err := follower.link.ForwardTriggerFire(context.Background(), fire)
	if err != nil {
		t.Fatalf("second delivery: %v", err)
	}
	if !second.Duplicate || second.Started != 0 {
		t.Fatalf("the repeated fire id must be reported as a duplicate, got %+v", second)
	}
	if got := leader.runCount(300 * time.Millisecond); got != 1 {
		t.Fatalf("want one run for one fire id, got %d", got)
	}
}

// TestLeaderStartsOnlyBindingsTheFollowerMatched: the leader intersects the forwarded
// match result with its own bindings, so a fire naming a placement the plugin never
// accepted, or one that no longer exists, starts nothing.
func TestLeaderStartsOnlyBindingsTheFollowerMatched(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	res, err := follower.link.ForwardTriggerFire(context.Background(), nodelink.TriggerFire{
		FireID: "fire-x", ActionKey: fwdAction, Source: "addon-demo", Origin: "node-b",
		Bindings: []nodelink.TriggerFireBinding{{WorkflowID: "wf1", NodeID: "not-a-trigger"}},
	})
	if err != nil || res.Started != 0 {
		t.Fatalf("a fire naming no real binding starts nothing, got %+v, %v", res, err)
	}
	if got := leader.runCount(150 * time.Millisecond); got != 0 {
		t.Fatalf("the leader started a binding the follower did not match: %d runs", got)
	}
}

// TestFireDuringLeaderLossIsRetried: with no leader at the moment of the fire, the fire
// is kept and delivered once a node wins the lock, instead of being dropped.
func TestFireDuringLeaderLossIsRetried(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	leader.coord.leader.Store(false)

	started, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-demo", fwdAction, nil, acceptAll)
	if err != nil || started != 0 {
		t.Fatalf("with no leader the fire is queued and reports 0, got %d, %v", started, err)
	}
	time.Sleep(60 * time.Millisecond)
	leader.coord.leader.Store(true)
	if got := leader.runCount(time.Second); got != 1 {
		t.Fatalf("the retried fire should start exactly one run once node-a leads, got %d", got)
	}
	if got := follower.runCount(50 * time.Millisecond); got != 0 {
		t.Fatalf("the follower ran the workflow itself: %d", got)
	}
}

// TestFireNobodyLeadsIsReported: when no leader appears inside the retry window the fire
// is dropped loudly, naming the fire, rather than retried forever or lost in silence.
func TestFireNobodyLeadsIsReported(t *testing.T) {
	var buf safeBuffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	leader, follower := fwdCluster(t, logger)
	leader.coord.leader.Store(false)
	follower.ts.fires.window = 150 * time.Millisecond

	if _, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-demo", fwdAction, nil, acceptAll); err != nil {
		t.Fatalf("fire: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(buf.String(), "no leader accepted the plugin fire; dropped") {
		if time.Now().After(deadline) {
			t.Fatalf("the dropped fire was not reported; log:\n%s", buf.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(buf.String(), "fire=") || !strings.Contains(buf.String(), "action="+fwdAction) {
		t.Fatalf("the report must name the fire and its action; log:\n%s", buf.String())
	}
	if got := leader.runCount(50*time.Millisecond) + follower.runCount(50*time.Millisecond); got != 0 {
		t.Fatalf("no node leads, so nothing may run: %d", got)
	}
}

func quietFwdLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
