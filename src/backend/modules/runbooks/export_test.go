// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// seedExportRunbook writes a runbook with the fields an export has to carry, and
// a couple it must deliberately drop.
func seedExportRunbook(t *testing.T) (*Store, string) {
	t.Helper()
	store := NewStore(openDB(t))
	id, err := store.Save(context.Background(), "", RunbookInput{
		Name:               "Container is restarting in a loop",
		Description:        "Triage a restart loop before it takes the stack with it.",
		AttachKind:         AttachAlertCondition,
		AttachConditionKey: "container_restart_loop",
		EnvironmentID:      "env-local-only",
		Enabled:            true,
		Steps: []StepInput{
			{Kind: "diagnostic", Title: "Read the logs", Body: "Look for the last line before each restart.",
				DiagnosticKey: "container.logs", Required: true},
			{Kind: "action", Title: "Restart once", Body: "One restart tells you whether it is deterministic.",
				ActionKey: "container.restart", ActionParams: map[string]string{"timeout": "10"}},
		},
	}, "tester")
	if err != nil {
		t.Fatalf("seeding a runbook: %v", err)
	}
	return store, id
}

// TestExportFileCarriesTheRunbookAndNotTheInstance: an export is a file somebody
// hands to another McHarbor, so it must carry what the runbook *is* and none of
// the ids that only mean something here. An import that reused them would either
// collide or point at a principal that does not exist.
func TestExportFileCarriesTheRunbookAndNotTheInstance(t *testing.T) {
	store, id := seedExportRunbook(t)
	rb, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	out := ExportFile(rb, now)

	if out.Kind != "mcharbor.runbook" {
		t.Errorf("kind = %q, want mcharbor.runbook", out.Kind)
	}
	if out.Version != 1 {
		t.Errorf("version = %d, want 1", out.Version)
	}
	if out.ExportedAt != "2026-09-28T03:00:00Z" {
		t.Errorf("exportedAt = %q", out.ExportedAt)
	}
	if out.Runbook.Name != rb.Name || out.Runbook.Description != rb.Description {
		t.Error("the name and description must survive the export")
	}
	if out.Runbook.AttachKind != AttachAlertCondition || out.Runbook.AttachConditionKey != "container_restart_loop" {
		t.Errorf("the attachment must survive: %+v", out.Runbook)
	}
	if out.Runbook.OriginalID != id {
		t.Error("originalId is a breadcrumb for the reader and should be present")
	}

	// Steps, in order, with their action parameters.
	if len(out.Runbook.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(out.Runbook.Steps))
	}
	// The store assigns positions from 1, and the export preserves whatever it
	// assigned rather than renumbering: a finished run refers to them by number.
	if out.Runbook.Steps[0].Position != 1 || out.Runbook.Steps[1].Position != 2 {
		t.Errorf("step positions must be preserved: %d, %d",
			out.Runbook.Steps[0].Position, out.Runbook.Steps[1].Position)
	}
	if out.Runbook.Steps[0].DiagnosticKey != "container.logs" {
		t.Error("a diagnostic step must keep its key, or the button does nothing after import")
	}
	if out.Runbook.Steps[1].ActionParams["timeout"] != "10" {
		t.Error("action parameters must survive the export")
	}

	// The instance-local half must not be in the file at all. Checked against the
	// marshalled JSON, because a field that is absent from the struct cannot be
	// asserted any other way.
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, leaked := range []string{"env-local-only", "environmentId", "createdBy", "principalId", "runbookId"} {
		if strings.Contains(body, leaked) {
			t.Errorf("the export leaks %q, which is local to this instance:\n%s", leaked, body)
		}
	}
}

// TestExportFileIsNilForAMissingRunbook keeps the handler's 404 path honest.
func TestExportFileIsNilForAMissingRunbook(t *testing.T) {
	if ExportFile(nil, time.Now()) != nil {
		t.Error("a nil runbook must produce no export file")
	}
}

// TestSafeRunbookExportFilename: the name reaches a filesystem, so it has to
// survive every platform — and a runbook named only in punctuation must still
// produce something openable rather than a bare extension or a leading dash.
func TestSafeRunbookExportFilename(t *testing.T) {
	for name, want := range map[string]string{
		"Container is restarting in a loop": "container-is-restarting-in-a-loop.mcharbor-runbook.json",
		"Host disk / filling up!":           "host-disk-filling-up.mcharbor-runbook.json",
		"  Agent   offline  ":               "agent-offline.mcharbor-runbook.json",
		"":                                  "runbook.mcharbor-runbook.json",
		"!!!":                               "runbook.mcharbor-runbook.json",
		"CVE-2026-1234":                     "cve-2026-1234.mcharbor-runbook.json",
	} {
		if got := safeRunbookExportFilename(name); got != want {
			t.Errorf("safeRunbookExportFilename(%q) = %q, want %q", name, got, want)
		}
	}
}
