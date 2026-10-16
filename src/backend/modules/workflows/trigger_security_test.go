// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/nodelink"
	"github.com/therealmcsparrow/mcharbor/core/pluginslug"
)

// peersOf lists the given nodes as a static peer source.
func peersOf(nodes ...*fwdNode) nodelink.PeerSource {
	return func(context.Context) ([]nodelink.Peer, error) {
		out := make([]nodelink.Peer, 0, len(nodes))
		for _, n := range nodes {
			out = append(out, nodelink.Peer{NodeID: n.coord.id, AdvertiseURL: n.server.URL})
		}
		return out, nil
	}
}

// sharedDB is the database fwdCluster's nodes use, read back through the service.
func sharedDB(n *fwdNode) *sql.DB { return n.ts.service.db }

// TestRetriedFireOnANewLeaderStartsNothing: the fire ids a leader evaluated are in the
// shared database, so the same fire delivered again after the leadership moved to another
// node is recognised there and starts no second run.
func TestRetriedFireOnANewLeaderStartsNothing(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	successor := newFwdNode(t, sharedDB(leader), "node-c", quietFwdLogger())
	fire := nodelink.TriggerFire{
		FireID: "fire-replay", ActionKey: fwdAction, Source: "addon-demo", Origin: "node-b",
		Bindings: []nodelink.TriggerFireBinding{{WorkflowID: "wf1", NodeID: "trig"}},
	}
	if res, err := follower.link.ForwardTriggerFire(context.Background(), fire); err != nil || res.Started != 1 {
		t.Fatalf("first delivery: %+v, %v", res, err)
	}

	leader.coord.leader.Store(false)
	successor.coord.leader.Store(true)
	follower.link.SetPeers(peersOf(successor))
	res, err := follower.link.ForwardTriggerFire(context.Background(), fire)
	if err != nil {
		t.Fatalf("delivery to the new leader: %v", err)
	}
	if !res.Duplicate || res.Started != 0 || res.ExecutedBy != "node-c" {
		t.Fatalf("the new leader must report the fire as a duplicate, got %+v", res)
	}
	if got := successor.runCount(200 * time.Millisecond); got != 0 {
		t.Fatalf("the new leader started %d runs for a fire the old leader already started", got)
	}
	if got := leader.runCount(200 * time.Millisecond); got != 1 {
		t.Fatalf("want the one original run on the old leader, got %d", got)
	}
}

// TestCrashAfterStartRetryIsDeduplicated: the leader starts the runs and dies before its
// reply reaches the sender; the sender's retry lands on the next leader, which must see
// the fire id the dead leader recorded and start nothing.
func TestCrashAfterStartRetryIsDeduplicated(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	successor := newFwdNode(t, sharedDB(leader), "node-c", quietFwdLogger())
	var crashed atomic.Bool
	leader.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(nodelink.HeaderPath) != "/workflow-trigger/fire" || crashed.Load() {
			leader.link.ServeTriggerFire(w, r)
			return
		}
		leader.link.ServeTriggerFire(httptest.NewRecorder(), r)
		crashed.Store(true)
		// The runs have started; the connection drops before the reply is written.
		panic(http.ErrAbortHandler)
	})
	follower.link.SetPeers(peersOf(leader, successor))
	follower.ts.fires.first = 150 * time.Millisecond

	started, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-demo", fwdAction, nil, acceptAll)
	if err != nil || started != 0 {
		t.Fatalf("a lost reply reports 0 and retries, got %d, %v", started, err)
	}
	if !crashed.Load() {
		t.Fatal("the leader never received the fire")
	}
	leader.coord.leader.Store(false)
	successor.coord.leader.Store(true)

	if got := leader.runCount(300 * time.Millisecond); got != 1 {
		t.Fatalf("want the one run the crashed leader started, got %d", got)
	}
	if got := successor.runCount(time.Second); got != 0 {
		t.Fatalf("the retry started %d runs on the new leader; the fire had already run", got)
	}
}

// TestPluginCannotFireAnotherPluginsKey: a plugin fires only its own `plugin:<slug>:`
// keys, on the leader and through a forward alike.
func TestPluginCannotFireAnotherPluginsKey(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())

	if _, err := leader.ts.FirePluginTriggerFor(context.Background(), "addon-other", fwdAction, nil, acceptAll); !errors.Is(err, nodelink.ErrFireNotAllowed) {
		t.Fatalf("a local fire of another plugin's key must be refused, got %v", err)
	}
	if _, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-other", fwdAction, nil, acceptAll); !errors.Is(err, nodelink.ErrFireNotAllowed) {
		t.Fatalf("a follower fire of another plugin's key must be refused before forwarding, got %v", err)
	}
	if _, err := leader.ts.FirePluginTriggerFor(context.Background(), FireSourceHost, NodeITSMStatus, nil, acceptAll); !errors.Is(err, nodelink.ErrFireNotAllowed) {
		t.Fatalf("a plugin may not pose as the host, got %v", err)
	}
	if _, err := leader.ts.FirePluginTrigger(context.Background(), fwdAction, nil, acceptAll); !errors.Is(err, nodelink.ErrFireNotAllowed) {
		t.Fatalf("the host may not fire a plugin's key, got %v", err)
	}

	forged := nodelink.TriggerFire{
		FireID: "fire-forged", ActionKey: fwdAction, Source: "addon-other", Origin: "node-b",
		Bindings: []nodelink.TriggerFireBinding{{WorkflowID: "wf1", NodeID: "trig"}},
	}
	if _, err := follower.link.ForwardTriggerFire(context.Background(), forged); !errors.Is(err, nodelink.ErrFireNotAllowed) {
		t.Fatalf("the leader must refuse a forwarded fire of another plugin's key, got %v", err)
	}
	if got := leader.runCount(200*time.Millisecond) + follower.runCount(50*time.Millisecond); got != 0 {
		t.Fatalf("a refused fire started %d runs", got)
	}
}

// TestHostAllowListKeyStillFires: the host's named key (the ITSM status trigger) fires on
// the leader and through a forward from a follower.
func TestHostAllowListKeyStillFires(t *testing.T) {
	leader, follower := fwdCluster(t, quietFwdLogger())
	canvas := `{"nodes":[{"id":"itsm","action":"` + NodeITSMStatus + `","config":{}}],"edges":[]}`
	if _, err := sharedDB(leader).Exec(`INSERT INTO workflows (id, name, status, canvas_data, created_at, updated_at)
		VALUES ('wf-itsm', 'itsm', 'active', ?, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z')`, canvas); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if started, err := leader.ts.FirePluginTrigger(context.Background(), NodeITSMStatus, nil, acceptAll); err != nil || started != 1 {
		t.Fatalf("the host key must fire on the leader, got %d, %v", started, err)
	}
	if started, err := follower.ts.FirePluginTrigger(context.Background(), NodeITSMStatus, nil, acceptAll); err != nil || started != 1 {
		t.Fatalf("the host key must fire through a forward, got %d, %v", started, err)
	}
	if got := leader.runCount(300 * time.Millisecond); got != 2 {
		t.Fatalf("want two host-fired runs on the leader, got %d", got)
	}
}

// TestForgedReplyIsRetried: a peer answering 200 without the leader's signature is not a
// delivery; the fire goes to the background retry instead of being counted as started.
func TestForgedReplyIsRetried(t *testing.T) {
	_, follower := fwdCluster(t, quietFwdLogger())
	var hits atomic.Int32
	// It echoes the fire id and claims to lead, so only the missing signature gives it away.
	forged := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var fire nodelink.TriggerFire
		if err := json.NewDecoder(r.Body).Decode(&fire); err != nil {
			fire.FireID = ""
		}
		reply, err := json.Marshal(map[string]any{"fireId": fire.FireID, "started": 1, "leader": true, "node": "node-a"})
		if err != nil {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(reply); err != nil {
			return
		}
	}))
	t.Cleanup(forged.Close)
	follower.link.SetPeers(func(context.Context) ([]nodelink.Peer, error) {
		return []nodelink.Peer{{NodeID: "node-a", AdvertiseURL: forged.URL}}, nil
	})

	started, err := follower.ts.FirePluginTriggerFor(context.Background(), "addon-demo", fwdAction, nil, acceptAll)
	if err != nil || started != 0 {
		t.Fatalf("a forged 200 must not count as started runs, got %d, %v", started, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the fire was not retried after a forged reply (%d requests)", hits.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestHostFireSourceIsAReservedSlug: the host's own fire source must be a slug no plugin can
// install under, or that plugin would pass fireAllowed as the host.
func TestHostFireSourceIsAReservedSlug(t *testing.T) {
	if !pluginslug.Reserved(FireSourceHost) {
		t.Fatalf("the fire source %q must be a reserved plugin slug", FireSourceHost)
	}
}
