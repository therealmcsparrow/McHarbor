// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"strings"
	"testing"
	"time"
)

var scheduleDay = time.Date(2026, 9, 20, 0, 0, 0, 0, time.Local)

func scheduleDays(days int) time.Time {
	return scheduleDay.AddDate(0, 0, days)
}

func snapshotRun(status string, at time.Time) SnapshotRun {
	return SnapshotRun{Status: status, StartedAt: at}
}

func dailySchedule() SnapshotSchedule {
	return SnapshotSchedule{ID: "s1", Name: "nightly", Cron: "0 3 * * *", Enabled: true,
		KeepCount: 7, CreatedAt: scheduleDays(-30)}
}

func TestCronFirings(t *testing.T) {
	cases := []struct {
		spec     string
		from, to time.Time
		want     int
	}{
		{"0 3 * * *", scheduleDay, scheduleDays(3), 3},
		{"0 3 * * *", scheduleDay.Add(3 * time.Hour), scheduleDays(1), 1},
		{"0 3 * * *", scheduleDay.Add(3*time.Hour + time.Second), scheduleDays(1), 0},
		{"*/15 * * * *", scheduleDay, scheduleDay.Add(time.Hour), 4},
		{"0 3 * * 1", scheduleDay, scheduleDays(14), 2},
		{"not a cron", scheduleDay, scheduleDays(3), 0},
		{"0 3 * * *", scheduleDays(3), scheduleDay, 0},
	}
	for _, c := range cases {
		if got := cronFirings(c.spec, c.from, c.to); got != c.want {
			t.Errorf("cronFirings(%q, %v, %v) = %d, want %d", c.spec, c.from, c.to, got, c.want)
		}
	}
}

func TestAssessScheduleMissedRuns(t *testing.T) {
	var runs []SnapshotRun
	for day := 0; day < 5; day++ {
		runs = append(runs, snapshotRun("success", scheduleDays(day).Add(3*time.Hour)))
	}
	runs = append(runs, snapshotRun("failure", scheduleDays(5).Add(3*time.Hour)))
	runs = append(runs, snapshotRun("success", scheduleDays(-2)))

	row := assessSchedule(scheduleInput{vm: VM{Name: "db"}, schedule: dailySchedule(), runs: runs, kept: 7},
		scheduleDay, scheduleDays(7))
	if row.expected != 7 || row.succeeded != 5 || row.failed != 1 || row.missed != 1 {
		t.Fatalf("expected/succeeded/failed/missed = %d/%d/%d/%d, want 7/5/1/1",
			row.expected, row.succeeded, row.failed, row.missed)
	}
	if row.overRetention {
		t.Fatal("7 kept of 7 is over retention")
	}
}

func TestAssessScheduleCountsFromCreation(t *testing.T) {
	schedule := dailySchedule()
	schedule.CreatedAt = scheduleDays(5)
	row := assessSchedule(scheduleInput{schedule: schedule}, scheduleDay, scheduleDays(7))
	if row.expected != 2 || row.missed != 2 {
		t.Fatalf("expected/missed = %d/%d, want 2/2: runs before the schedule existed are not owed", row.expected, row.missed)
	}
}

func TestAssessScheduleDisabledOwesNothing(t *testing.T) {
	schedule := dailySchedule()
	schedule.Enabled = false
	row := assessSchedule(scheduleInput{schedule: schedule, kept: 9}, scheduleDay, scheduleDays(7))
	if row.expected != 0 || row.missed != 0 {
		t.Fatalf("expected/missed = %d/%d, want 0/0 for a disabled schedule", row.expected, row.missed)
	}
	if !row.overRetention {
		t.Fatal("9 kept of 7 is not over retention")
	}
}

func TestAssessSchedulesAndFindings(t *testing.T) {
	guests := []VM{{ID: "1", Name: "db"}, {ID: "2", Name: "web"}}
	disabled := dailySchedule()
	disabled.ID, disabled.Name, disabled.Enabled = "s2", "weekly", false

	a := assessSchedules(guests, []scheduleInput{
		{vm: guests[0], schedule: dailySchedule(), kept: 8,
			runs: []SnapshotRun{snapshotRun("failure", scheduleDay.Add(3*time.Hour))}},
		{vm: guests[1], schedule: disabled},
	}, scheduleDay, scheduleDays(3))

	if a.expected != 3 || a.failed != 1 || a.missed != 2 {
		t.Fatalf("expected/failed/missed = %d/%d/%d, want 3/1/2", a.expected, a.failed, a.missed)
	}
	if len(a.unscheduled) != 1 || a.unscheduled[0] != "web" {
		t.Fatalf("unscheduled = %v, want [web]: a disabled schedule is not coverage", a.unscheduled)
	}
	titles := findingTitles(scheduleFindings(a))
	for _, want := range []string{"high: 1 schedule did not fire (2 missed runs)", "high: 1 scheduled snapshot failed",
		"medium: 1 schedule holds more snapshots", "medium: 1 guest has no enabled", "low: 1 schedule is disabled"} {
		if !strings.Contains(titles, want) {
			t.Errorf("findings %q do not contain %q", titles, want)
		}
	}
	if len(scheduleFindings(scheduleAssessment{guests: 2})) != 0 {
		t.Fatal("an assessment with nothing wrong raised findings")
	}
}
