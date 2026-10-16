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

const (
	// staleBackupDays and lostBackupDays grade the newest good backup's age:
	// a warning past a week, danger past a month, as the protection report does.
	staleBackupDays = 7
	lostBackupDays  = 30

	// backupSuccessTarget is the run success rate below which failures stop
	// being noise and become a finding.
	backupSuccessTarget = 0.9

	// defaultReportDays is the window a period report falls back to when the
	// request carries none.
	defaultReportDays = 30
)

// reportWindow resolves the period a report covers. An empty or inverted
// window falls back to the last defaultReportDays, and the end never passes now.
func reportWindow(req plugin.ReportRequest, now time.Time) (time.Time, time.Time) {
	from, to := req.PeriodStart, req.PeriodEnd
	if to.IsZero() || to.After(now) {
		to = now
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-defaultReportDays * 24 * time.Hour)
	}
	return from, to
}

// backupChainState is one disk's chain with the number of runs it holds since
// (and including) its last full copy.
type backupChainState struct {
	chain  BackupChain
	length int
}

// backupVMInput is everything the backup report reads for one guest, gathered
// so the assessment below is a pure function a test can feed by hand.
type backupVMInput struct {
	vm       VM
	plans    []BackupPlan
	runs     []BackupRun
	lastGood time.Time
	chains   []backupChainState
}

// backupVMRow is one guest's line in the backup health table.
type backupVMRow struct {
	vm         VM
	plans      int
	covered    bool
	manualOnly bool
	succeeded  int
	failed     int
	partial    int
	lastGood   time.Time
	ageDays    int
}

// destinationDelivery counts how often one destination was owed a copy in the
// period and how often it actually received a complete one.
type destinationDelivery struct {
	id        string
	name      string
	expected  int
	delivered int
}

// backupChainRow is one chain in the operational annex.
type backupChainRow struct {
	vmName string
	state  backupChainState
}

// backupRunRow is one failed or partial run in the operational annex.
type backupRunRow struct {
	vmName  string
	run     BackupRun
	missing []string
}

// backupPlanRow is one plan in the coverage table.
type backupPlanRow struct {
	vm           VM
	plan         BackupPlan
	destinations []string
}

// backupAssessment is the whole backup report's arithmetic, derived once so
// the summary, the tables and the findings cannot disagree.
type backupAssessment struct {
	rows         []backupVMRow
	plans        []backupPlanRow
	destinations []destinationDelivery
	chains       []backupChainRow
	problemRuns  []backupRunRow
	noPlan       []string

	guests      int
	covered     int
	manualOnly  int
	unprotected int
	succeeded   int
	finished    int
	stale       int
	never       int
	broken      int
	partialRuns int
}

// successRate is the share of finished runs that succeeded, or -1 when none
// finished, which is "unknown" rather than "all failed".
func successRate(succeeded, finished int) float64 {
	if finished <= 0 {
		return -1
	}
	return float64(succeeded) / float64(finished)
}

// ageInDays is how many whole days lie between t and now, or -1 for never.
func ageInDays(t, now time.Time) int {
	if t.IsZero() {
		return -1
	}
	if now.Before(t) {
		return 0
	}
	return int(now.Sub(t).Hours() / 24)
}

// planCovers reports whether a plan backs its guest up without anybody
// remembering to: enabled and on a schedule.
func planCovers(plan BackupPlan) bool {
	return plan.Enabled && strings.TrimSpace(plan.Cron) != ""
}

// runDelivery compares what one run was owed against what arrived. A run that
// never reached the upload stage owes nothing here; its failure is the run's.
func runDelivery(run BackupRun, planDests []string) (expected []string, missing []string) {
	if len(run.Destinations) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	ok := map[string]bool{}
	for _, dest := range run.Destinations {
		if _, known := ok[dest.LocationID]; !known {
			ok[dest.LocationID] = true
		}
		if dest.Status != DestinationSuccess {
			ok[dest.LocationID] = false
		}
	}
	for _, id := range planDests {
		if !seen[id] {
			seen[id] = true
			expected = append(expected, id)
		}
	}
	for _, dest := range run.Destinations {
		if !seen[dest.LocationID] {
			seen[dest.LocationID] = true
			expected = append(expected, dest.LocationID)
		}
	}
	for _, id := range expected {
		if !ok[id] {
			missing = append(missing, id)
		}
	}
	return expected, missing
}

// assessBackups turns the per-guest reads into the report's figures.
func assessBackups(inputs []backupVMInput, names map[string]string, now time.Time) backupAssessment {
	var out backupAssessment
	deliveries := map[string]*destinationDelivery{}
	known := map[string]string{}
	for id, name := range names {
		known[id] = name
	}
	for _, in := range inputs {
		for _, run := range in.runs {
			for _, dest := range run.Destinations {
				if dest.LocationName != "" && known[dest.LocationID] == "" {
					known[dest.LocationID] = dest.LocationName
				}
			}
		}
	}
	nameOf := func(id string) string {
		if name := known[id]; name != "" {
			return name
		}
		return id
	}

	for _, in := range inputs {
		out.guests++
		row := backupVMRow{vm: in.vm, plans: len(in.plans), lastGood: in.lastGood,
			ageDays: ageInDays(in.lastGood, now)}

		planDests := map[string][]string{}
		for _, plan := range in.plans {
			planDests[plan.ID] = plan.DestinationIDs
			if planCovers(plan) {
				row.covered = true
			}
			dests := make([]string, 0, len(plan.DestinationIDs))
			for _, id := range plan.DestinationIDs {
				dests = append(dests, nameOf(id))
			}
			out.plans = append(out.plans, backupPlanRow{vm: in.vm, plan: plan, destinations: dests})
		}
		row.manualOnly = len(in.plans) > 0 && !row.covered

		for _, run := range in.runs {
			switch run.Status {
			case BackupStatusSuccess:
				row.succeeded++
			case BackupStatusFailure, BackupStatusCancelled:
				row.failed++
			default:
				continue
			}
			expected, missing := runDelivery(run, planDests[run.PlanID])
			for _, id := range expected {
				entry, found := deliveries[id]
				if !found {
					entry = &destinationDelivery{id: id}
					deliveries[id] = entry
				}
				entry.expected++
			}
			missed := map[string]bool{}
			for _, id := range missing {
				missed[id] = true
			}
			for _, id := range expected {
				if !missed[id] {
					deliveries[id].delivered++
				}
			}
			if len(missing) > 0 && len(missing) < len(expected) {
				row.partial++
				out.partialRuns++
			}
			if run.Status != BackupStatusSuccess || len(missing) > 0 {
				missingNames := make([]string, 0, len(missing))
				for _, id := range missing {
					missingNames = append(missingNames, nameOf(id))
				}
				out.problemRuns = append(out.problemRuns,
					backupRunRow{vmName: in.vm.Name, run: run, missing: missingNames})
			}
		}

		for _, chain := range in.chains {
			if chain.chain.Invalidated {
				out.broken++
			}
			out.chains = append(out.chains, backupChainRow{vmName: in.vm.Name, state: chain})
		}

		switch {
		case len(in.plans) == 0:
			out.unprotected++
			out.noPlan = append(out.noPlan, in.vm.Name)
		case row.ageDays < 0:
			out.never++
		case row.ageDays > staleBackupDays:
			out.stale++
		}
		if row.covered {
			out.covered++
		}
		if row.manualOnly {
			out.manualOnly++
		}
		out.succeeded += row.succeeded
		out.finished += row.succeeded + row.failed
		out.rows = append(out.rows, row)
	}

	for id, entry := range deliveries {
		entry.name = nameOf(id)
		out.destinations = append(out.destinations, *entry)
	}
	sort.Slice(out.destinations, func(i, j int) bool {
		return out.destinations[i].name < out.destinations[j].name
	})
	sort.Slice(out.rows, func(i, j int) bool {
		a, b := out.rows[i], out.rows[j]
		if (a.plans == 0) != (b.plans == 0) {
			return a.plans == 0
		}
		if a.ageDays != b.ageDays {
			if a.ageDays < 0 || b.ageDays < 0 {
				return a.ageDays < 0
			}
			return a.ageDays > b.ageDays
		}
		return a.vm.Name < b.vm.Name
	})
	sort.Strings(out.noPlan)
	return out
}

// failingDestinations are the destinations that missed at least one copy.
func (a backupAssessment) failingDestinations() []destinationDelivery {
	var out []destinationDelivery
	for _, dest := range a.destinations {
		if dest.delivered < dest.expected {
			out = append(out, dest)
		}
	}
	return out
}

// backupFindings names what the operator must act on, worst first.
func backupFindings(a backupAssessment) []plugin.ReportFinding {
	var findings []plugin.ReportFinding
	if a.unprotected > 0 {
		findings = append(findings, finding("critical",
			plural(a.unprotected, "guest has no backup plan", "guests have no backup plan"),
			"A snapshot lives on the same pool as the disk it protects. Without a backup, losing that pool or host loses the guest: "+strings.Join(a.noPlan, ", ")+".",
			"Create a backup plan for each, on a schedule, writing to at least one destination away from the host."))
	}
	if a.never > 0 {
		findings = append(findings, finding("high",
			plural(a.never, "guest with a plan has never been backed up", "guests with a plan have never been backed up"),
			"The plan exists, but no run has ever succeeded, so there is nothing to restore.",
			"Run each plan once by hand and read the error of the failed run if it does not succeed."))
	}
	if a.stale > 0 {
		findings = append(findings, finding("high",
			fmt.Sprintf("%s older than %d days", plural(a.stale, "guest's last good backup is", "guests' last good backups are"), staleBackupDays),
			"Restoring would lose everything written since, and a plan that has stopped succeeding usually fails quietly.",
			"Check the failed runs below and the plan's schedule; a disabled plan or a full destination are the usual causes."))
	}
	if failing := a.failingDestinations(); len(failing) > 0 {
		names := make([]string, 0, len(failing))
		for _, dest := range failing {
			names = append(names, fmt.Sprintf("%s (%d of %d)", dest.name, dest.delivered, dest.expected))
		}
		findings = append(findings, finding("high",
			plural(len(failing), "destination did not receive every backup", "destinations did not receive every backup"),
			"Delivered against owed in this period: "+strings.Join(names, ", ")+". A copy that reached one destination but not another leaves a single point of failure the plan was written to avoid.",
			"Check the destination's credentials and free space, then run the affected plans again."))
	}
	if rate := successRate(a.succeeded, a.finished); rate >= 0 && rate < backupSuccessTarget {
		findings = append(findings, finding("high",
			fmt.Sprintf("Only %s of backup runs succeeded", percentLabel(rate)),
			fmt.Sprintf("%d of %s finished without a usable copy in this period.", a.finished-a.succeeded, plural(a.finished, "run", "runs")),
			"Read the errors in the failed runs table; the same message repeating points at one cause."))
	}
	if a.manualOnly > 0 {
		findings = append(findings, finding("medium",
			plural(a.manualOnly, "guest is covered by manual plans only", "guests are covered by manual plans only"),
			"Their plans are disabled or have no schedule, so a backup happens only when somebody remembers to run one.",
			"Enable the plan and give it a schedule."))
	}
	if a.broken > 0 {
		findings = append(findings, finding("medium",
			plural(a.broken, "backup chain is marked unusable", "backup chains are marked unusable"),
			"The next backup of each affected disk will be a full copy, which takes longer and uses more space than the schedule assumes.",
			"No action is required beyond allowing the next full run to complete; if it keeps happening, read the reason in the chains table."))
	}
	return findings
}

// percentLabel renders a rate for the page, or a dash when unknown.
func percentLabel(rate float64) string {
	if rate < 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", rate*100)
}

// backupLead writes the backup report's opening paragraph.
func backupLead(a backupAssessment) string {
	if a.guests == 0 {
		return "No virtual machine was readable in this scope, so there is no backup coverage to assess."
	}
	body := fmt.Sprintf("%d of %s run on a backup schedule. ",
		a.covered, plural(a.guests, "guest", "guests"))
	if a.unprotected > 0 {
		body += fmt.Sprintf("%s no plan at all and would be lost with their host. ",
			plural(a.unprotected, "guest has", "guests have"))
	}
	if a.finished > 0 {
		body += fmt.Sprintf("%s of the %s that finished in this period succeeded. ",
			percentLabel(successRate(a.succeeded, a.finished)), plural(a.finished, "run", "runs"))
	} else {
		body += "No backup run finished in this period. "
	}
	body += "A backup counts only once it reached every destination its plan names; a copy on one of two destinations is reported as partial."
	return body
}

// backupSections lays the assessment out as report sections.
func backupSections(a backupAssessment) []plugin.ReportSection {
	rate := successRate(a.succeeded, a.finished)
	buckets := map[string]int{}
	for _, row := range a.rows {
		if row.plans == 0 {
			continue
		}
		if row.ageDays < 0 {
			buckets["Never"]++
			continue
		}
		buckets[ageBucket(row.ageDays)]++
	}

	summary := []plugin.ReportBlock{
		leadBlock(backupLead(a)),
		{
			Kind: "kpis",
			KPIs: []plugin.ReportKPI{
				{Label: "Guests", Value: fmt.Sprintf("%d", a.guests)},
				{Label: "On a schedule", Value: fmt.Sprintf("%d", a.covered),
					Status: kpiStatus(a.covered == a.guests)},
				{Label: "No plan", Value: fmt.Sprintf("%d", a.unprotected),
					Status: kpiStatus(a.unprotected == 0)},
				{Label: "Run success rate", Value: percentLabel(rate),
					Status: kpiStatus(rate < 0 || rate >= backupSuccessTarget),
					Hint:   fmt.Sprintf("%d of %d finished runs in the period.", a.succeeded, a.finished)},
				{Label: "Stale or never backed up", Value: fmt.Sprintf("%d", a.stale+a.never),
					Status: kpiStatus(a.stale+a.never == 0),
					Hint:   fmt.Sprintf("Last good backup older than %d days, among guests with a plan.", staleBackupDays)},
			},
		},
	}
	if len(buckets) > 0 {
		points := make([]plugin.ReportChartPoint, 0, len(ageBucketOrder)+1)
		for _, bucket := range append(append([]string{}, ageBucketOrder...), "Never") {
			if buckets[bucket] == 0 {
				continue
			}
			tone := ageBucketTone(bucket)
			if bucket == "Never" {
				tone = "bad"
			}
			points = append(points, chartPoint(bucket, float64(buckets[bucket]), tone))
		}
		summary = append(summary, barChart("Last good backup, by age", "guests", points))
	}
	if findings := backupFindings(a); len(findings) > 0 {
		summary = append(summary, findingsBlock(findings))
	}

	coverage := []plugin.ReportBlock{backupPlanTable(a)}
	if len(a.noPlan) > 0 {
		coverage = append(coverage, calloutBlock("danger", "Guests with no backup plan",
			strings.Join(a.noPlan, ", ")))
	}

	sections := []plugin.ReportSection{
		{Level: "summary", Title: "Summary", Blocks: summary},
		{Level: "strategic", Title: "Coverage", Blocks: coverage},
		{Level: "tactical", Title: "Backup health by guest", Blocks: []plugin.ReportBlock{backupGuestTable(a)}},
		{Level: "tactical", Title: "Destinations", Blocks: []plugin.ReportBlock{backupDestinationTable(a)}},
		{Level: "operational", Title: "Backup chains", Blocks: []plugin.ReportBlock{backupChainTable(a)}},
	}
	if len(a.problemRuns) > 0 {
		sections = append(sections, plugin.ReportSection{
			Level: "operational", Title: "Failed and partial runs",
			Blocks: []plugin.ReportBlock{backupProblemTable(a)},
		})
	}
	return sections
}

func backupPlanTable(a backupAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.plans))
	for _, entry := range a.plans {
		schedule := plugin.ReportCell{Text: entry.plan.Cron, Mono: true}
		if strings.TrimSpace(entry.plan.Cron) == "" {
			schedule = plugin.ReportCell{Text: "manual", Status: "warning", Pill: true}
		}
		enabled := plugin.ReportCell{Text: "enabled", Status: "success", Pill: true}
		if !entry.plan.Enabled {
			enabled = plugin.ReportCell{Text: "disabled", Status: "warning", Pill: true}
		}
		next := "—"
		if entry.plan.NextRunAt != nil && entry.plan.Enabled {
			next = entry.plan.NextRunAt.Format("2006-01-02 15:04")
		}
		retention := retentionLabel(entry.plan.RetentionCount, entry.plan.RetentionDays)
		rows = append(rows, []plugin.ReportCell{
			{Text: entry.plan.Name}, {Text: entry.vm.Name}, schedule, enabled,
			{Text: next}, {Text: strings.Join(entry.destinations, ", ")}, {Text: retention},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Plan"}, {Label: "Guest"}, {Label: "Schedule", Mono: true},
			{Label: "State"}, {Label: "Next run"}, {Label: "Destinations"}, {Label: "Retention"},
		},
		Rows: rows,
		Note: "Schedules are five-field cron expressions evaluated in the server's time zone.",
	}
}

// retentionLabel renders a plan's retention, where zero means unbounded.
func retentionLabel(count, days int) string {
	var parts []string
	if count > 0 {
		parts = append(parts, plural(count, "copy", "copies"))
	}
	if days > 0 {
		parts = append(parts, plural(days, "day", "days"))
	}
	if len(parts) == 0 {
		return "unbounded"
	}
	return strings.Join(parts, ", ")
}

func backupGuestTable(a backupAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.rows))
	for _, row := range a.rows {
		coverage := plugin.ReportCell{Text: "scheduled", Status: "success", Pill: true}
		switch {
		case row.plans == 0:
			coverage = plugin.ReportCell{Text: "no plan", Status: "danger", Pill: true}
		case row.manualOnly:
			coverage = plugin.ReportCell{Text: "manual only", Status: "warning", Pill: true}
		}
		last := plugin.ReportCell{Text: "never", Status: "danger"}
		if row.ageDays >= 0 {
			last = plugin.ReportCell{Text: row.lastGood.Format("2006-01-02"), Status: backupAgeStatus(row.ageDays)}
		}
		if row.plans == 0 && row.ageDays < 0 {
			last.Status = ""
		}
		rate := successRate(row.succeeded, row.succeeded+row.failed)
		rateCell := plugin.ReportCell{Text: percentLabel(rate)}
		if rate >= 0 && rate < backupSuccessTarget {
			rateCell.Status = "danger"
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: row.vm.Name}, {Text: row.vm.EnvironmentID}, coverage,
			{Text: fmt.Sprintf("%d", row.succeeded)}, {Text: fmt.Sprintf("%d", row.failed)},
			{Text: fmt.Sprintf("%d", row.partial)}, rateCell, last,
			{Text: dayAgeLabel(row.ageDays)},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Guest"}, {Label: "Host"}, {Label: "Coverage"},
			{Label: "Succeeded", Align: "right"}, {Label: "Failed", Align: "right"},
			{Label: "Partial", Align: "right"}, {Label: "Success rate", Align: "right"},
			{Label: "Last good backup"}, {Label: "Age"},
		},
		Rows:    rows,
		Caption: "Least protected first. Run counts cover the report period; the last good backup is read without a period bound.",
		Note:    "Cancelled runs count as failed: they left no usable copy.",
	}
}

// dayAgeLabel renders an age in days, where -1 means never and 0 means today.
func dayAgeLabel(days int) string {
	switch {
	case days < 0:
		return "—"
	case days == 0:
		return "today"
	default:
		return plural(days, "day", "days")
	}
}

// backupAgeStatus grades the age of a guest's newest good backup.
func backupAgeStatus(days int) string {
	switch {
	case days < 0 || days > lostBackupDays:
		return "danger"
	case days > staleBackupDays:
		return "warning"
	default:
		return "success"
	}
}

func backupDestinationTable(a backupAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.destinations))
	for _, dest := range a.destinations {
		status := "success"
		if dest.delivered < dest.expected {
			status = "danger"
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: dest.name},
			{Text: fmt.Sprintf("%d", dest.expected)},
			{Text: fmt.Sprintf("%d", dest.delivered)},
			{Text: fmt.Sprintf("%d", dest.expected-dest.delivered), Status: status},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Destination"}, {Label: "Owed", Align: "right"},
			{Label: "Received", Align: "right"}, {Label: "Missed", Align: "right"},
		},
		Rows: rows,
		Note: "A destination is owed a copy by every run that reached the upload stage, whether its plan named it then or names it now. Runs that failed before uploading count against the run, not the destination.",
	}
}

func backupChainTable(a backupAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.chains))
	for _, entry := range a.chains {
		chain := entry.state.chain
		health := plugin.ReportCell{Text: "healthy", Status: "success", Pill: true}
		if chain.Invalidated {
			health = plugin.ReportCell{Text: "unusable", Status: "warning", Pill: true}
		}
		lastFull := "—"
		if chain.LastFullAt != nil {
			lastFull = chain.LastFullAt.Format("2006-01-02")
		}
		reason := chain.InvalidWhy
		if !chain.Invalidated {
			reason = ""
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: entry.vmName}, {Text: chain.DiskID, Mono: true}, health,
			{Text: fmt.Sprintf("%d", entry.state.length)}, {Text: lastFull}, {Text: reason},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Guest"}, {Label: "Disk", Mono: true}, {Label: "Health"},
			{Label: "Length", Align: "right"}, {Label: "Last full"}, {Label: "Why unusable"},
		},
		Rows: rows,
		Note: "Length counts the successful runs a restore has to replay: the last full copy and every incremental after it. An unusable chain restarts with a full copy on its next run.",
	}
}

func backupProblemTable(a backupAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.problemRuns))
	for _, entry := range a.problemRuns {
		status := plugin.ReportCell{Text: entry.run.Status, Status: "danger", Pill: true}
		if entry.run.Status == BackupStatusSuccess {
			status = plugin.ReportCell{Text: "partial", Status: "warning", Pill: true}
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: entry.run.StartedAt.Format("2006-01-02 15:04")},
			{Text: entry.vmName}, status, {Text: entry.run.Kind},
			{Text: strings.Join(entry.missing, ", ")}, {Text: entry.run.Error},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Started"}, {Label: "Guest"}, {Label: "Outcome"}, {Label: "Kind"},
			{Label: "Missed destinations"}, {Label: "Error"},
		},
		Rows: rows,
	}
}

// buildBackupAssurance reads plans, runs and chains per guest and composes the
// backup assurance report.
func (r *reportProvider) buildBackupAssurance(
	ctx context.Context,
	req plugin.ReportRequest,
	guests []VM,
	sources []plugin.ReportSourceStatus,
	notes []string,
) (plugin.ReportResult, error) {
	svc := r.plugin.svc
	now := time.Now().UTC()
	from, to := reportWindow(req, now)

	started := time.Now()
	var inputs []backupVMInput
	var failed []string
	for _, vm := range guests {
		in, err := r.readBackupInput(ctx, vm, from, to)
		if err != nil {
			r.plugin.logger.Warn("kvm: report could not read a guest's backups", "vm", vm.ID, "error", err)
			failed = append(failed, vm.Name)
			continue
		}
		inputs = append(inputs, in)
	}
	source := plugin.ReportSourceStatus{
		Key: "kvm_backups", Label: "Backup plans, runs, destinations and chains", OK: len(failed) == 0,
		DurationMS: time.Since(started).Milliseconds(),
	}
	if len(failed) > 0 {
		source.Note = fmt.Sprintf("%s could not be read and are excluded.", plural(len(failed), "guest", "guests"))
		notes = append(notes, fmt.Sprintf("Backup records for %s could not be read and are excluded from every figure.",
			strings.Join(failed, ", ")))
	}
	sources = append(sources, source)

	names := map[string]string{}
	if svc.storage != nil {
		for _, location := range svc.storage.Locations() {
			names[location.ID] = location.Name
		}
		sources = append(sources, plugin.ReportSourceStatus{
			Key: "kvm_backup_destinations", Label: "Storage destinations", OK: true,
		})
	} else {
		sources = append(sources, plugin.ReportSourceStatus{
			Key: "kvm_backup_destinations", Label: "Storage destinations", OK: false,
			Note: "No storage destinations are configured, so destinations are named by id.",
		})
	}

	assessment := assessBackups(inputs, names, now)
	return plugin.ReportResult{Sections: backupSections(assessment), Sources: sources, Notes: notes}, nil
}

// readBackupInput gathers one guest's plans, runs in the window, newest good
// backup and chains.
func (r *reportProvider) readBackupInput(ctx context.Context, vm VM, from, to time.Time) (backupVMInput, error) {
	svc := r.plugin.svc
	in := backupVMInput{vm: vm}
	var err error
	if in.plans, err = svc.ListBackupPlans(ctx, vm.ID); err != nil {
		return in, fmt.Errorf("plans: %w", err)
	}
	if in.runs, err = svc.backupRunsBetween(ctx, vm.ID, from, to); err != nil {
		return in, fmt.Errorf("runs: %w", err)
	}
	if in.lastGood, err = svc.lastGoodBackupAt(ctx, vm.ID); err != nil {
		return in, fmt.Errorf("last good backup: %w", err)
	}
	chains, err := svc.chainsForVM(ctx, vm.ID)
	if err != nil {
		return in, fmt.Errorf("chains: %w", err)
	}
	for _, chain := range chains {
		length := 0
		if chain.LastFullAt != nil {
			since, err := svc.runsSinceFull(ctx, chain.ID)
			if err != nil {
				return in, fmt.Errorf("chain length: %w", err)
			}
			length = since + 1
		}
		in.chains = append(in.chains, backupChainState{chain: chain, length: length})
	}
	return in, nil
}
