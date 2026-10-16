// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/incidentsources"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

type fakeSource struct {
	signals []incidentsource.Signal
	settled bool
}

func (f *fakeSource) IncidentSubjectTypes() []incidentsource.SubjectType {
	return []incidentsource.SubjectType{{Type: "virtual_machine", Label: "Virtual machine", Signals: []incidentsource.SignalDecl{
		{Key: "vm_paused_error", Label: "Paused on error", Strength: 3},
		{Key: "vm_crashed", Label: "Crashed", Strength: 5},
	}}}
}

func (f *fakeSource) IncidentSignals(context.Context, time.Time) ([]incidentsource.Signal, error) {
	return f.signals, nil
}

func (f *fakeSource) IncidentEvidence(_ context.Context, _, _, id string, _, _ time.Time) (*incidentsource.Evidence, error) {
	return &incidentsource.Evidence{Facts: map[string]string{"state": "crashed"}, Sections: []incidentsource.Section{
		{Title: "Guest log", Kind: "log", Lines: []string{"qemu exited", "password=hunter2secret"}},
	}}, nil
}

func (f *fakeSource) IncidentSettled(context.Context, string, string, string) (bool, error) {
	return f.settled, nil
}

// TestPluginIncidentsFollowTheirSource guards the opt-in contract: a plugin signal
// opens one incident keyed on the subject id, a persisting report does not inflate
// it, a stronger one escalates it, recovery is the plugin's call, and an unloaded
// plugin never resolves what it raised.
func TestPluginIncidentsFollowTheirSource(t *testing.T) {
	database := openDB(t)
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	store := &Store{db: database, now: func() time.Time { return now }}
	d := &Detector{DB: database, Store: store, Now: func() time.Time { return now }}
	src := &fakeSource{signals: []incidentsource.Signal{{EnvironmentID: "", SubjectType: "virtual_machine", SubjectID: "vm-1",
		SubjectName: "db-vm", Signal: "vm_paused_error", At: now}}}
	incidentsources.Register("t-kvm", src)
	t.Cleanup(func() { incidentsources.Unregister("t-kvm") })
	ctx := context.Background()

	res, err := d.Scan(ctx)
	if err != nil || len(res.Opened) != 1 {
		t.Fatalf("first scan: %+v %v", res, err)
	}
	in := res.Opened[0]
	if in.Source != "t-kvm" || in.SubjectID != "vm-1" || in.SubjectType != "virtual_machine" || in.Signal != "vm_paused_error" {
		t.Fatalf("opened %+v", in)
	}
	if other, err := store.OpenFor(ctx, "", "db-vm"); err != nil || other != nil {
		t.Fatal("a container lookup must not find the plugin's incident")
	}

	if res, err := d.Scan(ctx); err != nil || len(res.Opened)+len(res.Escalated) != 0 {
		t.Fatalf("a persisting failure must neither reopen nor escalate: %+v", res)
	}
	got, err := store.Get(ctx, in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SignalCount != 1 {
		t.Fatalf("persisting report inflated the count to %d", got.SignalCount)
	}

	src.signals[0].Signal, src.signals[0].At = "vm_crashed", now.Add(time.Minute)
	if res, err := d.Scan(ctx); err != nil || len(res.Escalated) != 1 || res.Escalated[0].Signal != "vm_crashed" {
		t.Fatalf("a stronger signal must escalate: %+v", res)
	}

	if ok, err := d.Settled(ctx, got); err != nil || ok {
		t.Fatal("still failing according to the plugin")
	}
	src.settled = true
	if ok, err := d.Settled(ctx, got); err != nil || !ok {
		t.Fatal("the plugin says it recovered")
	}
	incidentsources.Unregister("t-kvm")
	if ok, err := d.Settled(ctx, got); err != nil || ok {
		t.Fatal("an unloaded plugin must not resolve its incidents")
	}
}

// TestPluginEvidenceIsRedacted guards what reaches storage and the model: plugin
// sections pass through the same redaction as container logs.
func TestPluginEvidenceIsRedacted(t *testing.T) {
	database := openDB(t)
	incidentsources.Register("t-kvm", &fakeSource{})
	t.Cleanup(func() { incidentsources.Unregister("t-kvm") })
	asm := &Assembler{DB: database, Now: func() time.Time { return time.Date(2026, 9, 27, 10, 5, 0, 0, time.UTC) }}
	ev, err := asm.Assemble(context.Background(), &Incident{Source: "t-kvm", SubjectType: "virtual_machine", SubjectID: "vm-1",
		SubjectName: "db-vm", FirstSignalAt: "2026-09-27T10:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Subject == nil || ev.Subject.Facts["state"] != "crashed" || len(ev.Sections) != 1 {
		t.Fatalf("evidence %+v", ev)
	}
	red, err := Redact(ev)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range red.Sections[0].Lines {
		if line == "password=hunter2secret" {
			t.Fatal("a secret in a plugin section survived redaction")
		}
	}
}
