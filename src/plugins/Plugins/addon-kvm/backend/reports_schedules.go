// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// maxCronFiringsWindow bounds how far cronFirings will walk, so a request for a
// year-long period cannot spend the report's whole deadline counting minutes.
const maxCronFiringsWindow = 400 * 24 * time.Hour

// cronFirings counts the minutes in [from, to) a five-field spec fires on, in
// the server's time zone as the scheduler reads it. An invalid spec fires never.
func cronFirings(spec string, from, to time.Time) int {
	fields := strings.Fields(spec)
	if len(fields) != 5 || !validCronSpec(spec) || !from.Before(to) {
		return 0
	}
	if to.Sub(from) > maxCronFiringsWindow {
		from = to.Add(-maxCronFiringsWindow)
	}
	start := from.In(time.Local).Truncate(time.Minute)
	if start.Before(from) {
		start = start.Add(time.Minute)
	}
	count := 0
	for t := start; t.Before(to); t = t.Add(time.Minute) {
		if cronFieldMatches(fields[0], t.Minute(), 0, 59, false) &&
			cronFieldMatches(fields[1], t.Hour(), 0, 23, false) &&
			cronFieldMatches(fields[2], t.Day(), 1, 31, false) &&
			cronFieldMatches(fields[3], int(t.Month()), 1, 12, false) &&
			cronFieldMatches(fields[4], int(t.Weekday()), 0, 7, true) {
			count++
		}
	}
	return count
}

// scheduleInput is what the schedule report reads for one schedule.
type scheduleInput struct {
	vm       VM
	schedule SnapshotSchedule
	runs     []SnapshotRun
	kept     int
}

// scheduleRow is one schedule's line in the report, and the unit the tests
// check: expected against produced is the report's whole argument.
type scheduleRow struct {
	vm            VM
	schedule      SnapshotSchedule
	expected      int
	succeeded     int
	failed        int
	running       int
	missed        int
	kept          int
	overRetention bool
	failures      []SnapshotRun
}

// assessSchedule compares what a schedule should have fired in [from, to)
// with what it recorded. A disabled schedule owes nothing: when it was turned
// off is not recorded, so any expectation would be a guess.
func assessSchedule(in scheduleInput, from, to time.Time) scheduleRow {
	row := scheduleRow{vm: in.vm, schedule: in.schedule, kept: in.kept}
	if in.schedule.Enabled {
		start := from
		if in.schedule.CreatedAt.After(start) {
			start = in.schedule.CreatedAt
		}
		row.expected = cronFirings(in.schedule.Cron, start, to)
	}
	for _, run := range in.runs {
		if run.StartedAt.Before(from) || !run.StartedAt.Before(to) {
			continue
		}
		switch run.Status {
		case "success":
			row.succeeded++
		case "failure":
			row.failed++
			row.failures = append(row.failures, run)
		default:
			row.running++
		}
	}
	if gap := row.expected - (row.succeeded + row.failed + row.running); gap > 0 {
		row.missed = gap
	}
	row.overRetention = in.schedule.KeepCount > 0 && in.kept > in.schedule.KeepCount
	return row
}

// scheduleAssessment is the schedule report's arithmetic, derived once.
type scheduleAssessment struct {
	rows          []scheduleRow
	guests        int
	unscheduled   []string
	enabled       int
	expected      int
	succeeded     int
	failed        int
	missed        int
	overRetention []string
	disabled      []string
	missing       []string
	failing       []string
}

// assessSchedules rolls every schedule up and names the guests that have no
// enabled schedule at all.
func assessSchedules(guests []VM, inputs []scheduleInput, from, to time.Time) scheduleAssessment {
	out := scheduleAssessment{guests: len(guests)}
	scheduled := map[string]bool{}
	for _, in := range inputs {
		row := assessSchedule(in, from, to)
		label := in.vm.Name + " / " + in.schedule.Name
		if in.schedule.Enabled {
			out.enabled++
			scheduled[in.vm.ID] = true
		} else {
			out.disabled = append(out.disabled, label)
		}
		out.expected += row.expected
		out.succeeded += row.succeeded
		out.failed += row.failed
		out.missed += row.missed
		if row.missed > 0 {
			out.missing = append(out.missing, label)
		}
		if row.failed > 0 {
			out.failing = append(out.failing, label)
		}
		if row.overRetention {
			out.overRetention = append(out.overRetention, label)
		}
		out.rows = append(out.rows, row)
	}
	for _, vm := range guests {
		if !scheduled[vm.ID] {
			out.unscheduled = append(out.unscheduled, vm.Name)
		}
	}
	sort.Slice(out.rows, func(i, j int) bool {
		a, b := out.rows[i], out.rows[j]
		if a.missed+a.failed != b.missed+b.failed {
			return a.missed+a.failed > b.missed+b.failed
		}
		if a.vm.Name != b.vm.Name {
			return a.vm.Name < b.vm.Name
		}
		return a.schedule.Name < b.schedule.Name
	})
	sort.Strings(out.unscheduled)
	return out
}

// scheduleFindings names what the operator must act on, worst first.
func scheduleFindings(a scheduleAssessment) []plugin.ReportFinding {
	var findings []plugin.ReportFinding
	if a.missed > 0 {
		findings = append(findings, finding("high",
			fmt.Sprintf("%s did not fire (%s)", plural(len(a.missing), "schedule", "schedules"), plural(a.missed, "missed run", "missed runs")),
			"The schedule was due and no run was recorded: "+strings.Join(a.missing, ", ")+". The scheduler was down, the host was unreachable, or McHarbor was restarting at the time.",
			"Check that McHarbor was running at the missed times, then take a snapshot by hand so coverage does not wait for the next firing."))
	}
	if a.failed > 0 {
		findings = append(findings, finding("high",
			plural(a.failed, "scheduled snapshot failed", "scheduled snapshots failed"),
			"Failing schedules: "+strings.Join(a.failing, ", ")+". A schedule that keeps failing looks configured and protects nothing.",
			"Read the errors in the failed runs table; a full pool or a guest agent that stopped answering are the usual causes."))
	}
	if len(a.overRetention) > 0 {
		findings = append(findings, finding("medium",
			plural(len(a.overRetention), "schedule holds more snapshots than it keeps", "schedules hold more snapshots than they keep"),
			"Pruning did not keep up: "+strings.Join(a.overRetention, ", ")+". Every extra snapshot lengthens the chain and holds pool space.",
			"Look for a prune error in the schedule's runs; a snapshot with two children cannot be merged away and blocks the ones behind it."))
	}
	if len(a.unscheduled) > 0 {
		findings = append(findings, finding("medium",
			plural(len(a.unscheduled), "guest has no enabled snapshot schedule", "guests have no enabled snapshot schedule"),
			"Their snapshots depend on somebody remembering to take one: "+strings.Join(a.unscheduled, ", ")+".",
			"Add a schedule to each guest that has state worth rolling back to."))
	}
	if len(a.disabled) > 0 {
		findings = append(findings, finding("low",
			plural(len(a.disabled), "schedule is disabled", "schedules are disabled"),
			"Disabled: "+strings.Join(a.disabled, ", ")+". A disabled schedule is not counted as missing runs.",
			"Enable it again, or delete it so it does not read as coverage."))
	}
	return findings
}

// scheduleLead writes the schedule report's opening paragraph.
func scheduleLead(a scheduleAssessment) string {
	if len(a.rows) == 0 {
		if a.guests == 0 {
			return "No virtual machine was readable in this scope, so there are no snapshot schedules to assess."
		}
		return fmt.Sprintf("None of the %s in this scope has a snapshot schedule, so every snapshot depends on somebody taking it by hand.",
			plural(a.guests, "guest", "guests"))
	}
	body := fmt.Sprintf("%s were due to fire %s in this period and recorded %d successful snapshots. ",
		plural(a.enabled, "enabled schedule", "enabled schedules"), plural(a.expected, "time", "times"), a.succeeded)
	if a.missed == 0 && a.failed == 0 {
		body += "Every due run was recorded and none failed. "
	} else {
		body += fmt.Sprintf("%s never started and %s. ",
			plural(a.missed, "due run", "due runs"), plural(a.failed, "run failed", "runs failed"))
	}
	body += "Expected runs are counted from each schedule's cron expression, from the later of the period start and the schedule's creation."
	return body
}

// scheduleSections lays the assessment out as report sections.
func scheduleSections(a scheduleAssessment) []plugin.ReportSection {
	produced := successRate(a.succeeded, a.expected)
	summary := []plugin.ReportBlock{
		leadBlock(scheduleLead(a)),
		{
			Kind: "kpis",
			KPIs: []plugin.ReportKPI{
				{Label: "Schedules", Value: fmt.Sprintf("%d", len(a.rows)),
					Hint: fmt.Sprintf("%d enabled.", a.enabled)},
				{Label: "Runs due", Value: fmt.Sprintf("%d", a.expected)},
				{Label: "Snapshots taken", Value: fmt.Sprintf("%d", a.succeeded),
					Hint: "Share of due runs: " + percentLabel(produced) + "."},
				{Label: "Missed", Value: fmt.Sprintf("%d", a.missed), Status: kpiStatus(a.missed == 0)},
				{Label: "Failed", Value: fmt.Sprintf("%d", a.failed), Status: kpiStatus(a.failed == 0)},
				{Label: "Guests without a schedule", Value: fmt.Sprintf("%d", len(a.unscheduled)),
					Status: kpiStatus(len(a.unscheduled) == 0)},
			},
		},
	}
	if a.expected > 0 || a.failed > 0 {
		summary = append(summary, donutChart("Due runs by outcome", []plugin.ReportChartPoint{
			chartPoint("Succeeded", float64(a.succeeded), "good"),
			chartPoint("Failed", float64(a.failed), "bad"),
			chartPoint("Missed", float64(a.missed), "warn"),
		}))
	}
	if findings := scheduleFindings(a); len(findings) > 0 {
		summary = append(summary, findingsBlock(findings))
	}

	sections := []plugin.ReportSection{
		{Level: "summary", Title: "Summary", Blocks: summary},
		{Level: "tactical", Title: "Schedules", Blocks: []plugin.ReportBlock{scheduleTable(a)}},
	}
	if failures := scheduleFailureTable(a); len(failures.Rows) > 0 {
		sections = append(sections, plugin.ReportSection{
			Level: "operational", Title: "Failed runs", Blocks: []plugin.ReportBlock{failures},
		})
	}
	return sections
}

func scheduleTable(a scheduleAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.rows))
	for _, row := range a.rows {
		state := plugin.ReportCell{Text: "enabled", Status: "success", Pill: true}
		if !row.schedule.Enabled {
			state = plugin.ReportCell{Text: "disabled", Status: "warning", Pill: true}
		}
		next := "—"
		if row.schedule.NextRunAt != nil && row.schedule.Enabled {
			next = row.schedule.NextRunAt.Format("2006-01-02 15:04")
		}
		retention := plugin.ReportCell{Text: fmt.Sprintf("%d of %d", row.kept, row.schedule.KeepCount)}
		if row.overRetention {
			retention.Status = "warning"
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: row.vm.Name}, {Text: row.schedule.Name},
			{Text: row.schedule.Cron, Mono: true}, state, {Text: next},
			{Text: fmt.Sprintf("%d", row.expected)}, {Text: fmt.Sprintf("%d", row.succeeded)},
			countCell(row.failed, "danger"), countCell(row.missed, "danger"), retention,
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Guest"}, {Label: "Schedule"}, {Label: "Cron", Mono: true},
			{Label: "State"}, {Label: "Next run"},
			{Label: "Due", Align: "right"}, {Label: "Taken", Align: "right"},
			{Label: "Failed", Align: "right"}, {Label: "Missed", Align: "right"},
			{Label: "Kept / limit"},
		},
		Rows:    rows,
		Caption: "Schedules with the most missed and failed runs first.",
		Note:    "Cron expressions are evaluated in the server's time zone. A run started by hand from the schedule counts as a recorded run, so it can mask one missed firing.",
	}
}

func scheduleFailureTable(a scheduleAssessment) plugin.ReportBlock {
	var rows [][]plugin.ReportCell
	for _, row := range a.rows {
		for _, run := range row.failures {
			rows = append(rows, []plugin.ReportCell{
				{Text: run.StartedAt.Format("2006-01-02 15:04")},
				{Text: row.vm.Name}, {Text: row.schedule.Name}, {Text: run.Error},
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i][0].Text > rows[j][0].Text })
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Started"}, {Label: "Guest"}, {Label: "Schedule"}, {Label: "Error"},
		},
		Rows: rows,
	}
}

// buildSnapshotSchedules reads each guest's schedules, their runs in the period
// and the snapshots they still hold, and composes the schedule report.
func (r *reportProvider) buildSnapshotSchedules(
	ctx context.Context,
	req plugin.ReportRequest,
	guests []VM,
	sources []plugin.ReportSourceStatus,
	notes []string,
) (plugin.ReportResult, error) {
	now := time.Now().UTC()
	from, to := reportWindow(req, now)

	started := time.Now()
	var inputs []scheduleInput
	var failed []string
	var readable []VM
	for _, vm := range guests {
		entries, err := r.readScheduleInputs(ctx, vm, from, to)
		if err != nil {
			r.plugin.logger.Warn("kvm: report could not read a guest's snapshot schedules", "vm", vm.ID, "error", err)
			failed = append(failed, vm.Name)
			continue
		}
		readable = append(readable, vm)
		inputs = append(inputs, entries...)
	}
	source := plugin.ReportSourceStatus{
		Key: "kvm_snapshot_schedules", Label: "Snapshot schedules, runs and retained snapshots",
		OK: len(failed) == 0, DurationMS: time.Since(started).Milliseconds(),
	}
	if len(failed) > 0 {
		source.Note = fmt.Sprintf("%s could not be read and are excluded.", plural(len(failed), "guest", "guests"))
		notes = append(notes, fmt.Sprintf("Snapshot schedules for %s could not be read and are excluded from every figure.",
			strings.Join(failed, ", ")))
	}
	sources = append(sources, source)

	assessment := assessSchedules(readable, inputs, from, to)
	return plugin.ReportResult{Sections: scheduleSections(assessment), Sources: sources, Notes: notes}, nil
}

// readScheduleInputs gathers one guest's schedules with their runs in the
// window and the snapshots each still holds.
func (r *reportProvider) readScheduleInputs(ctx context.Context, vm VM, from, to time.Time) ([]scheduleInput, error) {
	svc := r.plugin.svc
	schedules, err := svc.ListSchedules(ctx, vm.ID)
	if err != nil {
		return nil, fmt.Errorf("schedules: %w", err)
	}
	if len(schedules) == 0 {
		return nil, nil
	}
	runs, err := svc.scheduleRunsBetween(ctx, vm.ID, from, to)
	if err != nil {
		return nil, fmt.Errorf("runs: %w", err)
	}
	kept, err := svc.scheduledSnapshotCounts(ctx, vm.ID)
	if err != nil {
		return nil, fmt.Errorf("retained snapshots: %w", err)
	}
	byschedule := map[string][]SnapshotRun{}
	for _, run := range runs {
		byschedule[run.ScheduleID] = append(byschedule[run.ScheduleID], run)
	}
	out := make([]scheduleInput, 0, len(schedules))
	for _, schedule := range schedules {
		out = append(out, scheduleInput{
			vm: vm, schedule: schedule, runs: byschedule[schedule.ID], kept: kept[schedule.ID],
		})
	}
	return out, nil
}
