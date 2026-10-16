// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

var backupNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func backupDaysAgo(days int) time.Time {
	return backupNow.Add(-time.Duration(days) * 24 * time.Hour)
}

func scheduledPlan(id string, dests ...string) BackupPlan {
	return BackupPlan{ID: id, Name: id, Cron: "0 2 * * *", Enabled: true, DestinationIDs: dests}
}

func backupRun(status string, dests ...BackupRunDestination) BackupRun {
	return BackupRun{ID: "r", PlanID: "p1", Status: status, Kind: BackupKindFull,
		StartedAt: backupDaysAgo(1), Destinations: dests}
}

func delivered(location, status string) BackupRunDestination {
	return BackupRunDestination{LocationID: location, Status: status}
}

func TestSuccessRateUnknownWhenNothingFinished(t *testing.T) {
	if got := successRate(0, 0); got != -1 {
		t.Fatalf("successRate(0, 0) = %v, want -1 (unknown, not zero)", got)
	}
	if got := successRate(9, 10); got != 0.9 {
		t.Fatalf("successRate(9, 10) = %v, want 0.9", got)
	}
}

func TestAgeInDays(t *testing.T) {
	cases := []struct {
		at   time.Time
		want int
	}{
		{time.Time{}, -1},
		{backupNow.Add(time.Hour), 0},
		{backupNow.Add(-23 * time.Hour), 0},
		{backupDaysAgo(8), 8},
	}
	for _, c := range cases {
		if got := ageInDays(c.at, backupNow); got != c.want {
			t.Errorf("ageInDays(%v) = %d, want %d", c.at, got, c.want)
		}
	}
}

func TestAssessBackupsCoverage(t *testing.T) {
	manual := scheduledPlan("p2", "a")
	manual.Enabled = false
	noCron := scheduledPlan("p3", "a")
	noCron.Cron = ""

	a := assessBackups([]backupVMInput{
		{vm: VM{ID: "1", Name: "web"}, plans: []BackupPlan{scheduledPlan("p1", "a")}, lastGood: backupDaysAgo(1)},
		{vm: VM{ID: "2", Name: "db"}, plans: []BackupPlan{manual, noCron}, lastGood: backupDaysAgo(1)},
		{vm: VM{ID: "3", Name: "cache"}},
	}, nil, backupNow)

	if a.covered != 1 || a.manualOnly != 1 || a.unprotected != 1 {
		t.Fatalf("covered/manual/unprotected = %d/%d/%d, want 1/1/1", a.covered, a.manualOnly, a.unprotected)
	}
	if len(a.noPlan) != 1 || a.noPlan[0] != "cache" {
		t.Fatalf("noPlan = %v, want [cache]", a.noPlan)
	}
	if len(a.plans) != 3 {
		t.Fatalf("plan rows = %d, want 3", len(a.plans))
	}
	if a.rows[0].vm.Name != "cache" {
		t.Fatalf("first row = %s, want the unprotected guest first", a.rows[0].vm.Name)
	}
}

func TestAssessBackupsPartialDelivery(t *testing.T) {
	a := assessBackups([]backupVMInput{{
		vm:       VM{ID: "1", Name: "web"},
		plans:    []BackupPlan{scheduledPlan("p1", "a", "b")},
		lastGood: backupDaysAgo(1),
		runs: []BackupRun{
			backupRun(BackupStatusSuccess, delivered("a", DestinationSuccess), delivered("b", DestinationFailure)),
			backupRun(BackupStatusSuccess, delivered("a", DestinationSuccess), delivered("b", DestinationSuccess)),
			backupRun(BackupStatusFailure),
			backupRun(BackupStatusCancelled),
			backupRun(BackupStatusRunning),
		},
	}}, map[string]string{"a": "Offsite", "b": "NAS"}, backupNow)

	if a.succeeded != 2 || a.finished != 4 {
		t.Fatalf("succeeded/finished = %d/%d, want 2/4 (cancelled fails, running excluded)", a.succeeded, a.finished)
	}
	if a.partialRuns != 1 {
		t.Fatalf("partialRuns = %d, want 1", a.partialRuns)
	}
	byName := map[string]destinationDelivery{}
	for _, dest := range a.destinations {
		byName[dest.name] = dest
	}
	if got := byName["NAS"]; got.expected != 2 || got.delivered != 1 {
		t.Fatalf("NAS = %d/%d delivered/owed, want 1/2", got.delivered, got.expected)
	}
	if got := byName["Offsite"]; got.expected != 2 || got.delivered != 2 {
		t.Fatalf("Offsite = %d/%d delivered/owed, want 2/2: a run that failed before upload owes no destination", got.delivered, got.expected)
	}
	failing := a.failingDestinations()
	if len(failing) != 1 || failing[0].name != "NAS" {
		t.Fatalf("failingDestinations = %v, want only NAS", failing)
	}
}

func TestAssessBackupsDestinationMissingFromRunIsMissed(t *testing.T) {
	a := assessBackups([]backupVMInput{{
		vm:    VM{ID: "1", Name: "web"},
		plans: []BackupPlan{scheduledPlan("p1", "a", "b")},
		runs:  []BackupRun{backupRun(BackupStatusSuccess, delivered("a", DestinationSuccess))},
	}}, nil, backupNow)
	if a.partialRuns != 1 {
		t.Fatalf("partialRuns = %d, want 1: the plan named b and b got no row", a.partialRuns)
	}
}

func TestAssessBackupsStaleness(t *testing.T) {
	plans := []BackupPlan{scheduledPlan("p1", "a")}
	a := assessBackups([]backupVMInput{
		{vm: VM{ID: "1", Name: "fresh"}, plans: plans, lastGood: backupDaysAgo(2)},
		{vm: VM{ID: "2", Name: "edge"}, plans: plans, lastGood: backupDaysAgo(staleBackupDays)},
		{vm: VM{ID: "3", Name: "stale"}, plans: plans, lastGood: backupDaysAgo(staleBackupDays + 1)},
		{vm: VM{ID: "4", Name: "never"}, plans: plans},
		{vm: VM{ID: "5", Name: "unplanned"}},
	}, nil, backupNow)
	if a.stale != 1 || a.never != 1 || a.unprotected != 1 {
		t.Fatalf("stale/never/unprotected = %d/%d/%d, want 1/1/1", a.stale, a.never, a.unprotected)
	}
}

func TestBackupAgeStatus(t *testing.T) {
	cases := map[int]string{-1: "danger", 0: "success", 7: "success", 8: "warning", 30: "warning", 31: "danger"}
	for days, want := range cases {
		if got := backupAgeStatus(days); got != want {
			t.Errorf("backupAgeStatus(%d) = %q, want %q", days, got, want)
		}
	}
}

func findingTitles(findings []plugin.ReportFinding) string {
	titles := make([]string, 0, len(findings))
	for _, f := range findings {
		titles = append(titles, f.Severity+": "+f.Title)
	}
	return strings.Join(titles, " | ")
}

func TestBackupFindingsSuccessRateThreshold(t *testing.T) {
	atTarget := backupFindings(backupAssessment{succeeded: 9, finished: 10})
	if strings.Contains(findingTitles(atTarget), "succeeded") {
		t.Fatalf("90%% success raised a finding: %s", findingTitles(atTarget))
	}
	below := backupFindings(backupAssessment{succeeded: 8, finished: 10})
	if !strings.Contains(findingTitles(below), "Only 80% of backup runs succeeded") {
		t.Fatalf("80%% success raised no finding: %s", findingTitles(below))
	}
	none := backupFindings(backupAssessment{})
	if len(none) != 0 {
		t.Fatalf("an empty assessment raised findings: %s", findingTitles(none))
	}
}

func TestBackupFindingsUnprotectedIsCriticalAndFirst(t *testing.T) {
	findings := backupFindings(backupAssessment{
		unprotected: 2, noPlan: []string{"a", "b"}, stale: 1, broken: 1,
		destinations: []destinationDelivery{{name: "NAS", expected: 3, delivered: 1}},
	})
	if len(findings) != 4 {
		t.Fatalf("findings = %s, want unprotected, stale, destination, chain", findingTitles(findings))
	}
	if findings[0].Severity != "critical" || !strings.Contains(findings[0].Title, "2 guests have no backup plan") {
		t.Fatalf("first finding = %s, want the unprotected guests as critical", findingTitles(findings[:1]))
	}
	if !strings.Contains(findings[2].Detail, "NAS (1 of 3)") {
		t.Fatalf("destination finding does not name the destination: %q", findings[2].Detail)
	}
}

func TestReportWindowDefaultsAndClamps(t *testing.T) {
	from, to := reportWindow(plugin.ReportRequest{}, backupNow)
	if !to.Equal(backupNow) || !from.Equal(backupDaysAgo(defaultReportDays)) {
		t.Fatalf("empty window = %v..%v, want the last %d days", from, to, defaultReportDays)
	}
	future := backupNow.Add(48 * time.Hour)
	from, to = reportWindow(plugin.ReportRequest{PeriodStart: backupDaysAgo(3), PeriodEnd: future}, backupNow)
	if !to.Equal(backupNow) || !from.Equal(backupDaysAgo(3)) {
		t.Fatalf("future end = %v..%v, want the end clamped to now", from, to)
	}
}
