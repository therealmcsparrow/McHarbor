// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

type fakeITSM struct {
	channel string
	events  []incident.Event
	err     error
}

func (f *fakeITSM) Emit(_ context.Context, channelID string, ev incident.Event) error {
	f.channel = channelID
	f.events = append(f.events, ev)
	return f.err
}

func withFakeITSM(t *testing.T) *fakeITSM {
	t.Helper()
	f := &fakeITSM{}
	SetITSMEmitter(f)
	t.Cleanup(func() { SetITSMEmitter(nil) })
	return f
}

func quietService() *Service {
	return &Service{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// TestITSMNodesEmitTheirStateWithRenderedKey guards the node contract: each
// action maps to its own incident state and the dedup key is rendered from msg.
func TestITSMNodesEmitTheirStateWithRenderedKey(t *testing.T) {
	f := withFakeITSM(t)
	s := quietService()
	msg := Msg{"_msgid": "m1", "payload": map[string]interface{}{"host": "db-1"}}
	want := map[string]incident.State{NodeITSMOpen: incident.StateFiring, NodeITSMUpdate: incident.StateUpdate, NodeITSMResolve: incident.StateResolved}
	for action, state := range want {
		node := &CanvasNode{Action: action, Label: "n", Config: map[string]interface{}{
			"channel_id": "ch1", "dedup_key": "disk:{{payload.host}}", "title": "Disk on {{payload.host}}", "severity": "critical",
		}}
		port, out, err := s.ExecuteNode(context.Background(), node, msg, nil, "")
		if err != nil || port != "output" {
			t.Fatalf("%s: port %q err %v", action, port, err)
		}
		ev := f.events[len(f.events)-1]
		if f.channel != "ch1" || ev.State != state || ev.DedupKey != "disk:db-1" || ev.Title != "Disk on db-1" || ev.Severity != incident.SeverityCritical {
			t.Fatalf("%s: emitted %+v on %q", action, ev, f.channel)
		}
		if p, _ := out["payload"].(map[string]interface{}); p["dedupKey"] != "disk:db-1" {
			t.Fatalf("%s: payload %+v", action, out["payload"])
		}
	}
}

// TestITSMNodeFailureTakesTheErrorPort guards that a refused emit or a missing
// channel is routed to the error port rather than silently succeeding.
func TestITSMNodeFailureTakesTheErrorPort(t *testing.T) {
	f := withFakeITSM(t)
	s := quietService()
	node := &CanvasNode{Action: NodeITSMOpen, Config: map[string]interface{}{"channel_id": ""}}
	if port, _, err := s.ExecuteNode(context.Background(), node, Msg{}, nil, ""); err != nil || port != "error" || len(f.events) != 0 {
		t.Fatalf("missing channel: port %q, %d emits", port, len(f.events))
	}
	f.err = errors.New("channel is not incident-capable")
	node.Config["channel_id"] = "ch1"
	if port, _, err := s.ExecuteNode(context.Background(), node, Msg{}, nil, ""); err != nil || port != "error" {
		t.Fatalf("refused emit: port %q", port)
	}
}

// TestITSMStatusMatches guards the trigger filter: an empty filter accepts
// everything, a channel or state filter excludes the rest.
func TestITSMStatusMatches(t *testing.T) {
	cases := []struct {
		cfg   map[string]interface{}
		ch    string
		state string
		want  bool
	}{
		{map[string]interface{}{}, "a", "acknowledged", true},
		{map[string]interface{}{"channel_id": "a"}, "b", "acknowledged", false},
		{map[string]interface{}{"states": []interface{}{"resolved", "closed"}}, "a", "acknowledged", false},
		{map[string]interface{}{"channel_id": "a", "states": []interface{}{"acknowledged"}}, "a", "acknowledged", true},
	}
	for i, c := range cases {
		if got := ITSMStatusMatches(c.cfg, c.ch, c.state); got != c.want {
			t.Fatalf("case %d: got %v", i, got)
		}
	}
}

// TestSavingITSMNodesNeedsAlertsManage guards the server-side gate: a canvas
// with a ticket action saves only for a holder of alerts.manage.
func TestSavingITSMNodesNeedsAlertsManage(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "wf.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('op', 'op', 'x'), ('boss', 'boss', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-wf', 'r-wf', '["workflows.view","workflows.manage"]'), ('r-al', 'r-al', '["workflows.manage","alerts.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('u1', 'op', 'r-wf'), ('u2', 'boss', 'r-al')`,
	} {
		if _, err := database.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	h := &Handler{app: &router.AppDeps{RBACService: rbac.NewService(database), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	ticketing := `{"nodes":[{"id":"n1","action":"itsm-open","config":{}}],"edges":[]}`
	plain := `{"nodes":[{"id":"n1","action":"debug","config":{}}],"edges":[]}`
	for _, c := range []struct {
		user, canvas string
		want         bool
	}{{"op", plain, true}, {"op", ticketing, false}, {"boss", ticketing, true}} {
		r := httptest.NewRequest("PUT", "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, &auth.User{ID: c.user}))
		if got := h.mayUseITSMNodes(r, c.canvas); got != c.want {
			t.Fatalf("%s saving %s: got %v", c.user, c.canvas, got)
		}
	}
}
