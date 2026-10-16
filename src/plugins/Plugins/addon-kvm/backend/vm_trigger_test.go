// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

func TestGuestTransitionMatchesAppliesEveryFilter(t *testing.T) {
	ev := guestTransition{GuestID: "g1", GuestName: "Web", EnvironmentID: "env1", State: "stopped", Severity: "warning"}
	cases := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"unconfigured fires on everything", map[string]any{}, true},
		{"environment matches", map[string]any{"environment": "env1"}, true},
		{"environment differs", map[string]any{"environment": "env2"}, false},
		{"guest by id", map[string]any{"vm": "g1"}, true},
		{"guest by typed name, any case", map[string]any{"vm": " web "}, true},
		{"another guest", map[string]any{"vm": "g2"}, false},
		{"state listed as saved", map[string]any{"states": []any{"running", "Stopped"}}, true},
		{"state not listed", map[string]any{"states": []any{"running"}}, false},
		{"state typed as a comma list", map[string]any{"states": "running, stopped"}, true},
		{"severity any", map[string]any{"severity": "any"}, true},
		{"severity matches", map[string]any{"severity": "warning"}, true},
		{"severity differs", map[string]any{"severity": "error"}, false},
	}
	for _, c := range cases {
		if got := guestTransitionMatches(c.config, ev); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGuestTransitionPayloadKeepsTheHostWatcherShape(t *testing.T) {
	ev := guestTransition{GuestID: "g1", GuestName: "Web", EnvironmentID: "env1", State: "crashed",
		Severity: "error", Metadata: map[string]any{"error": "qemu exited"}, Timestamp: "2026-09-30T00:00:00Z"}
	p := guestTransitionPayload(ev, true)
	vm, _ := p["vm"].(map[string]any)
	if p["trigger"] != "vm" || vm["id"] != "g1" || vm["name"] != "Web" || p["environmentId"] != "env1" {
		t.Fatalf("identity fields lost: %+v", p)
	}
	if p["state"] != "crashed" || p["action"] != "crashed" || p["severity"] != "error" || p["autoTriggered"] != true {
		t.Fatalf("transition fields lost: %+v", p)
	}
	if p["error"] != "qemu exited" || p["metadata"] == nil {
		t.Fatalf("a failure must surface as payload.error, got %+v", p)
	}
}

type recordingWorkflows struct {
	key     string
	payload map[string]any
	matched []string
}

func (r *recordingWorkflows) TriggerBindings(context.Context, string) ([]plugin.WorkflowTriggerBinding, error) {
	return nil, nil
}

func (r *recordingWorkflows) Fire(_ context.Context, key string, payload map[string]any, match func(plugin.WorkflowTriggerBinding) bool) (int, error) {
	r.key, r.payload = key, payload
	for _, b := range []plugin.WorkflowTriggerBinding{
		{NodeID: "any", Config: map[string]any{}},
		{NodeID: "other-guest", Config: map[string]any{"vm": "g2"}},
		{NodeID: "this-guest", Config: map[string]any{"vm": "g1", "states": []any{"running"}}},
	} {
		if match(b) {
			r.matched = append(r.matched, b.NodeID)
		}
	}
	return len(r.matched), nil
}

func (r *recordingWorkflows) DockerEnvironmentIDs(context.Context) ([]string, error) { return nil, nil }

type discardLifecycle struct{}

func (discardLifecycle) Record(context.Context, plugin.LifecycleEvent) error { return nil }

// TestRecordLifecycleFiresTheVMTrigger: the transition that reaches the timeline is the
// one that starts workflows, under the node's own key and filtered by each placement.
func TestRecordLifecycleFiresTheVMTrigger(t *testing.T) {
	wf := &recordingWorkflows{}
	s := &Service{lifecycle: discardLifecycle{}, logger: quietLogger{}}
	s.setWorkflowTrigger(wf)

	s.recordLifecycle(context.Background(), lifecycleGuest{id: "g1", name: "Web", envID: "env1", state: StateRunning})

	if wf.key != nodeVMTrigger {
		t.Fatalf("want the %s key fired, got %q", nodeVMTrigger, wf.key)
	}
	if wf.payload["state"] != StateRunning || wf.payload["severity"] != "success" {
		t.Fatalf("payload does not describe the transition: %+v", wf.payload)
	}
	if len(wf.matched) != 2 || wf.matched[0] != "any" || wf.matched[1] != "this-guest" {
		t.Fatalf("want the unfiltered and the matching placement, got %v", wf.matched)
	}
}

func TestExecVMTriggerPassesAFiredTransitionThrough(t *testing.T) {
	p := &KVMPlugin{}
	fired := map[string]any{"payload": map[string]any{"trigger": "vm", "state": "running"}, "topic": nodeVMTrigger}
	out, err := p.execVMTrigger(context.Background(), plugin.NodeExecInput{Key: nodeVMTrigger, Msg: fired})
	if err != nil || out.Port != "output" {
		t.Fatalf("got %q %v", out.Port, err)
	}
	payload, _ := out.Msg["payload"].(map[string]any)
	if payload["state"] != "running" {
		t.Fatalf("a fired transition must pass through unchanged, got %+v", out.Msg)
	}

	manual, err := p.execVMTrigger(context.Background(), plugin.NodeExecInput{Key: nodeVMTrigger, Msg: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ = manual.Msg["payload"].(map[string]any)
	if payload["trigger"] != "vm" || payload["matched"] != false {
		t.Fatalf("a manual run without the service must say nothing matched, got %+v", manual.Msg)
	}
}
