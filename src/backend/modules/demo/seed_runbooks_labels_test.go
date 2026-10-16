// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// runbooksBundlePath is the frontend's English bundle for the runbooks namespace.
const runbooksBundlePath = "../../../frontend/core/i18n/locales/en/runbooks.json"

// loadRunbookLabels returns one label group from the runbooks bundle.
//
// The runbook list renders its badges with computed keys — t(`attach.${kind}`),
// t(`kind.${step.kind}`) — so a seeded value the bundle has no label for renders
// the raw dotted key on screen. i18next does not fail on a miss, nothing type
// checks the string, and no CHECK constraint guards these columns, so the seed is
// the only place the vocabulary can be got wrong and the page is where it shows.
//
// This is not hypothetical: the first version of this seed wrote attach_kind
// "condition" against a bundle that only knows "alert_condition", step kinds
// "decision" and "note" against a bundle that only knows instruction /
// diagnostic / action, and outcome "resolved" against completed / abandoned /
// escalated. All three reached a running instance as visible raw keys.
func loadRunbookLabels(t *testing.T, group string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(runbooksBundlePath))
	if err != nil {
		t.Skipf("runbooks bundle not readable from here: %v", err)
	}
	var bundle map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("parsing the runbooks bundle: %v", err)
	}
	section, ok := bundle[group]
	if !ok {
		t.Fatalf("the runbooks bundle has no %q group", group)
	}
	var labels map[string]string
	if err := json.Unmarshal(section, &labels); err != nil {
		t.Fatalf("parsing the %q group: %v", group, err)
	}
	if len(labels) == 0 {
		t.Fatalf("the %q group is empty, so this test would pass vacuously", group)
	}
	return labels
}

// TestSeededRunbookVocabularyHasLabels checks every value the seed writes into a
// column the UI renders through a computed key.
func TestSeededRunbookVocabularyHasLabels(t *testing.T) {
	attach := loadRunbookLabels(t, "attach")
	kinds := loadRunbookLabels(t, "kind")

	for _, spec := range demoRunbooks() {
		if _, ok := attach[spec.attachKind]; !ok {
			t.Errorf("runbook %q attaches as %q, which has no label in runbooks.json attach.* (have %v)",
				spec.name, spec.attachKind, sortedKeys(attach))
		}
		if len(spec.steps) == 0 {
			t.Errorf("runbook %q has no steps", spec.name)
		}
		for _, step := range spec.steps {
			if _, ok := kinds[step.kind]; !ok {
				t.Errorf("runbook %q step %q is kind %q, which has no label in runbooks.json kind.* (have %v)",
					spec.name, step.title, step.kind, sortedKeys(kinds))
			}
		}
	}
}

// TestSeededRunbookOutcomesHaveLabels covers the run history's outcome column.
func TestSeededRunbookOutcomesHaveLabels(t *testing.T) {
	outcomes := loadRunbookLabels(t, "outcome")

	// The values seedRunbooks writes. Kept in step with the loop there; a value
	// added without a label is what this test exists to catch.
	for _, outcome := range []string{"completed", "abandoned"} {
		if _, ok := outcomes[outcome]; !ok {
			t.Errorf("seeded run outcome %q has no label in runbooks.json outcome.* (have %v)",
				outcome, sortedKeys(outcomes))
		}
	}
}

// TestSeededRunbookStepsNameTheirDiagnosticOrAction: a diagnostic step with no
// diagnostic key, or an action step with no action key, renders a button that does
// nothing.
func TestSeededRunbookStepsNameTheirDiagnosticOrAction(t *testing.T) {
	for _, spec := range demoRunbooks() {
		for _, step := range spec.steps {
			switch step.kind {
			case "diagnostic":
				if step.diagnosticKey == "" {
					t.Errorf("%q step %q is a diagnostic with no diagnostic key", spec.name, step.title)
				}
			case "action":
				if step.actionKey == "" {
					t.Errorf("%q step %q is an action with no action key", spec.name, step.title)
				}
			}
			if step.body == "" {
				t.Errorf("%q step %q has no body", spec.name, step.title)
			}
		}
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
