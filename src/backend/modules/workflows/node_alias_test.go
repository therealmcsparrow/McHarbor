// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

const (
	aliasAction     = "legacy-trigger"
	aliasPluginNode = "plugin:addon-demo:demo-trigger"
)

func claimLegacyTrigger(action string) (string, bool) {
	if action == aliasAction {
		return aliasPluginNode, true
	}
	return "", false
}

type recordingPluginNodes struct{ ran []string }

func (r *recordingPluginNodes) IsPluginNode(action string) bool { return action == aliasPluginNode }

func (r *recordingPluginNodes) ExecutePluginNode(_ context.Context, action string, in PluginNodeInput) (string, map[string]interface{}, error) {
	r.ran = append(r.ran, action)
	return "output", in.Msg, nil
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestExecuteNodeRunsAClaimedBareActionAsThePluginNode: a canvas saved while the host
// still shipped a node carries its bare action; once a plugin claims it, the run goes to
// that plugin's executor, and the node itself keeps the id it was stored with.
func TestExecuteNodeRunsAClaimedBareActionAsThePluginNode(t *testing.T) {
	plugins := &recordingPluginNodes{}
	s := &Service{logger: quietLogger(), pluginNodes: plugins, nodeAlias: claimLegacyTrigger}
	node := &CanvasNode{ID: "n1", Action: aliasAction, Config: map[string]interface{}{}}

	port, _, err := s.ExecuteNode(context.Background(), node, nil, &FlowContext{}, "")
	if err != nil || port != "output" {
		t.Fatalf("want the plugin's output port, got %q %v", port, err)
	}
	if len(plugins.ran) != 1 || plugins.ran[0] != aliasPluginNode {
		t.Fatalf("want one run of %s, got %v", aliasPluginNode, plugins.ran)
	}
	if node.Action != aliasAction {
		t.Fatalf("the stored node must keep its action, got %q", node.Action)
	}

	// With nothing claiming it the node is an unknown action again: it must not reach
	// any plugin executor, and it must not fail the run.
	s.nodeAlias = func(string) (string, bool) { return "", false }
	if _, _, err := s.ExecuteNode(context.Background(), node, nil, &FlowContext{}, ""); err != nil {
		t.Fatalf("an unclaimed action must degrade, not fail: %v", err)
	}
	if len(plugins.ran) != 1 {
		t.Fatalf("an unclaimed action reached a plugin executor: %v", plugins.ran)
	}
}

func openAliasTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "wf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestPluginTriggerBindingsIncludeAStoredAliasNode: the plugin fires its own composite
// key, so a workflow saved with the bare action only starts if the binding scan resolves it.
func TestPluginTriggerBindingsIncludeAStoredAliasNode(t *testing.T) {
	database := openAliasTestDB(t)
	canvas := `{"nodes":[` +
		`{"id":"old","action":"legacy-trigger","config":{"vm":"web"}},` +
		`{"id":"new","action":"plugin:addon-demo:demo-trigger","config":{}},` +
		`{"id":"other","action":"manual-trigger","config":{}}],"edges":[]}`
	if _, err := database.Exec(`INSERT INTO workflows (id, name, status, canvas_data, created_at, updated_at) VALUES ('wf1', 'wf', 'active', ?, '2026-09-30T00:00:00Z', '2026-09-30T00:00:00Z')`, canvas); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := &Service{db: database, logger: quietLogger(), nodeAlias: claimLegacyTrigger}
	ts := &TriggerService{service: s, logger: quietLogger()}
	bindings, err := ts.PluginTriggerBindings(context.Background(), aliasPluginNode)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, b := range bindings {
		got[b.NodeID] = true
	}
	if !got["old"] || !got["new"] || got["other"] || len(bindings) != 2 {
		t.Fatalf("want the aliased and the composite placement only, got %+v", bindings)
	}

	s.nodeAlias = func(string) (string, bool) { return "", false }
	bindings, err = ts.PluginTriggerBindings(context.Background(), aliasPluginNode)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].NodeID != "new" {
		t.Fatalf("with no claim only the composite placement binds, got %+v", bindings)
	}
}
