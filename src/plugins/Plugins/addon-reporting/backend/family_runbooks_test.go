// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rbTestSnap(t *testing.T) *Snapshot {
	t.Helper()
	snap := resTestSnap(t, true, srcRbRunbooks, srcRbRuns, srcRbAlerts)
	snap.Runbooks = runbooksData{
		Runbooks: []rbRunbookRow{
			{
				ID: "rb1", Name: "Disk filling up", AttachKind: "alert_condition",
				AttachConditionKey: "disk_usage", Enabled: true,
				Steps: []rbStepRow{
					{Position: 3, Kind: "action", Title: "Prune unused images", ActionKey: "images.prune"},
					{Position: 1, Kind: "instruction", Title: "Confirm the alert is real", Body: "Open the host.", Required: true},
					{Position: 2, Kind: "diagnostic", Title: "Find the largest volumes", DiagnosticKey: "volumes.size"},
				},
				Automation: rbAutomation{PrincipalName: "reclaimer", OnAlert: true},
			},
			{ID: "rb2", Name: "Empty one", AttachKind: "container", AttachSubject: "web"},
		},
		Runs: []runbookRunRow{
			{ID: "r1", RunbookName: "Disk filling up", StartedAt: "2026-09-20T10:00:00Z", Outcome: "completed"},
			{ID: "r2", RunbookName: "Empty one", StartedAt: "2026-09-21T10:00:00Z", Outcome: "abandoned"},
		},
		Alerts: []alertRow{
			{ID: "a1", Name: "Disk", Condition: "disk_usage"},
			{ID: "a2", Name: "CPU", Condition: "cpu_usage"},
		},
	}
	return snap
}

func rbSectionText(secs []Section) string {
	var sb strings.Builder
	for _, s := range secs {
		sb.WriteString(s.Title + "\n")
		for _, b := range s.Blocks {
			sb.WriteString(b.Text + "\n" + b.Title + "\n")
			sb.WriteString(strings.Join(b.Items, "\n") + "\n")
			for _, row := range b.Rows {
				for _, c := range row {
					sb.WriteString(c.Text + " ")
				}
				sb.WriteString("\n")
			}
			for _, p := range b.Pairs {
				sb.WriteString(p.K + ": " + p.V + "\n")
			}
			for _, f := range b.Findings {
				sb.WriteString(f.Title + " " + f.Detail + "\n")
			}
		}
	}
	return sb.String()
}

// A per-runbook document is only useful if the steps come out in the order
// somebody is meant to follow them. The fixture stores them 3, 1, 2 for that
// reason: array order would put the destructive prune first.
func TestASubjectScopedRunbookPrintsItsStepsInOrder(t *testing.T) {
	snap := rbTestSnap(t)
	applySubject(t.Context(), nil, snap, Subject{Kind: SubjectRunbook, ID: "rb1", Name: "Disk filling up"})
	if got := len(snap.Runbooks.Runbooks); got != 1 {
		t.Fatalf("the subject should narrow to one runbook, got %d", got)
	}

	text := rbSectionText(buildRunbooks(rbTestCtx(t, snap)))
	first := strings.Index(text, "Confirm the alert is real")
	second := strings.Index(text, "Find the largest volumes")
	third := strings.Index(text, "Prune unused images")
	if first < 0 || second < 0 || third < 0 {
		t.Fatalf("every step should be printed, got:\n%s", text)
	}
	if !(first < second && second < third) {
		t.Fatalf("steps are out of order (%d, %d, %d):\n%s", first, second, third, text)
	}
	if !strings.Contains(text, "reclaimer") {
		t.Errorf("the document should say it runs unattended, got:\n%s", text)
	}
	if strings.Contains(text, "Empty one") {
		t.Errorf("the other runbook should not appear in a scoped document:\n%s", text)
	}
}

// The gap is the aggregate's reason to exist: an alert that can fire with no
// written procedure behind it.
func TestTheAggregateNamesAnAlertConditionWithNoRunbook(t *testing.T) {
	snap := rbTestSnap(t)
	text := rbSectionText(buildRunbooks(rbTestCtx(t, snap)))
	if !strings.Contains(text, "cpu_usage") {
		t.Fatalf("the uncovered condition should be named, got:\n%s", text)
	}
	if strings.Contains(text, "disk_usage") && !strings.Contains(text, "Alert condition: disk_usage") {
		t.Errorf("a covered condition must not be reported as a gap:\n%s", text)
	}
	if !strings.Contains(text, "Empty one") {
		t.Errorf("the inventory should list every runbook, got:\n%s", text)
	}
}

func TestARunbookWithNoStepsIsAFinding(t *testing.T) {
	snap := rbTestSnap(t)
	fs := rbFindings(rbTestCtx(t, snap))
	if !findingTitled(fs, SevMedium, "no steps") {
		t.Fatalf("a runbook carrying no procedure should be reported, got %+v", fs)
	}
}

func TestOnlyAKnownSubjectKindNarrowsAReport(t *testing.T) {
	if _, ok := parseSubjectKind("runbook"); !ok {
		t.Fatal("runbook should be an accepted subject kind")
	}
	if _, ok := parseSubjectKind("runbooks"); ok {
		t.Fatal("a typo must not be accepted, or a report silently empties")
	}
}

// Every label the studio prints for this type has to exist in all six bundles,
// or the card reads as a raw key in five languages.
func TestTheRunbookReportIsLabelledInEveryLocale(t *testing.T) {
	for _, locale := range []string{"en", "nl", "de", "es", "fr", "pt"} {
		name, desc := localeTypeLabel(t, locale, TypeRunbooks)
		if strings.TrimSpace(name) == "" || strings.TrimSpace(desc) == "" {
			t.Errorf("%s: the runbooks report has no name or description", locale)
		}
	}
}

func rbTestCtx(t *testing.T, snap *Snapshot) *buildCtx {
	t.Helper()
	return resTestCtx(t, TypeRunbooks, snap)
}

// localeTypeLabel reads a type's name and description straight from the shipped
// bundle, because that is the file the host loads -- a constant in this package
// would prove nothing about what the studio renders.
func localeTypeLabel(t *testing.T, locale, key string) (string, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "i18n", locale, "reporting.json"))
	if err != nil {
		t.Fatalf("reading the %s bundle: %v", locale, err)
	}
	var bundle struct {
		Type map[string]struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"type"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("parsing the %s bundle: %v", locale, err)
	}
	return bundle.Type[key].Name, bundle.Type[key].Description
}

func TestRbAttachKindLabelNamesNoPlugin(t *testing.T) {
	cases := map[string]string{
		"container":     "Container",
		"swarm_service": "Swarm service",
		"vm":            "VM",
		"k8s_workload":  "K8S workload",
		"":              "Nothing",
	}
	for kind, want := range cases {
		if got := rbAttachKindLabel(kind); got != want {
			t.Errorf("rbAttachKindLabel(%q) = %q, want %q", kind, got, want)
		}
	}
}
