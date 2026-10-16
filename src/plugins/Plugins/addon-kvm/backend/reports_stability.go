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

// The kinds of instability the report counts. Anything else a QEMU event can
// say — a pause, a resume, a requested power-down — is not instability.
const (
	stabilityCrash          = "crash"
	stabilityUnexpectedStop = "unexpected stop"
	stabilityReset          = "unexpected reset"
	stabilityGuestShutdown  = "guest shutdown"
	stabilityIOError        = "disk I/O error"
)

const (
	// flapPeriodEvents is how many unintended downs in one period make a guest
	// flap even when they never bunch into a restart loop.
	flapPeriodEvents = 5

	// stabilityIncidentRows caps the event table, newest kept, so a guest in a
	// restart loop cannot turn the annex into hundreds of pages.
	stabilityIncidentRows = 200
)

// stabilityVMInput is what the stability report reads for one guest.
type stabilityVMInput struct {
	vm      VM
	health  *guestHealth
	events  []vmEvent
	intents []lifecycleIntent
}

// stabilityVMRow is one guest's line in the stability table.
type stabilityVMRow struct {
	vm             VM
	crashes        int
	stops          int
	resets         int
	guestShutdowns int
	ioErrors       int
	failedIntents  int
	stuckIntents   int
	flapping       bool
	agentText      string
	agentStatus    string
	checkedAt      time.Time
}

// unintended is the downs no McHarbor action explains.
func (r stabilityVMRow) unintended() int { return r.crashes + r.stops + r.resets }

// stabilityIncident is one classified event in the operational annex.
type stabilityIncident struct {
	vmName string
	at     time.Time
	kind   string
	detail string
}

// stabilityIntentRow is one failed or stuck lifecycle action.
type stabilityIntentRow struct {
	vmName string
	intent lifecycleIntent
	stuck  bool
}

// stabilityAssessment is the stability report's arithmetic, derived once.
type stabilityAssessment struct {
	rows      []stabilityVMRow
	incidents []stabilityIncident
	intents   []stabilityIntentRow

	guests       int
	crashes      int
	stops        int
	resets       int
	ioErrors     int
	flapping     []string
	failed       int
	stuck        int
	unresponsive []string
	diskAlerts   []string
}

// classifyStabilityEvent names what one QEMU event means for stability, or ""
// when it is routine. A stop or reset covered by a recorded McHarbor action is
// routine: somebody asked for it.
func classifyStabilityEvent(event vmEvent, events []vmEvent, intents []lifecycleIntent) string {
	switch event.kind {
	case qemuEventGuestPanic:
		return stabilityCrash
	case qemuEventBlockIOError:
		return stabilityIOError
	case qemuEventReset:
		if coveredByIntent(intents, event.at, nil) {
			return ""
		}
		return stabilityReset
	case qemuEventShutdown:
	default:
		return ""
	}

	switch event.reason {
	case shutdownReasonGuestShutdown:
		return stabilityGuestShutdown
	case shutdownReasonGuestPanic:
		// QEMU emits the panic event and then, with panic=poweroff, a shutdown
		// carrying the same cause. One crash, not two.
		for _, other := range events {
			if other.kind == qemuEventGuestPanic && !other.at.After(event.at) &&
				event.at.Sub(other.at) <= intentSkew {
				return ""
			}
		}
		return stabilityCrash
	case shutdownReasonGuestReset, shutdownReasonHostQMPReset, shutdownReasonSubsystem:
		if coveredByIntent(intents, event.at, nil) {
			return ""
		}
		return stabilityReset
	}
	if coveredByIntent(intents, event.at, stopIntentActions) {
		return ""
	}
	return stabilityUnexpectedStop
}

// maxBurst is the most moments falling inside any one window, over times
// sorted oldest first.
func maxBurst(times []time.Time, window time.Duration) int {
	best, start := 0, 0
	for end := range times {
		for times[end].Sub(times[start]) > window {
			start++
		}
		if n := end - start + 1; n > best {
			best = n
		}
	}
	return best
}

// isFlapping reports whether a guest's unintended downs form a restart loop,
// or are frequent enough across the period to call the guest unstable.
func isFlapping(downs []time.Time) bool {
	return len(downs) >= flapPeriodEvents || maxBurst(downs, restartLoopWindow) >= restartLoopDowns
}

// intentStuck reports whether a lifecycle action has been pending longer than
// the longest legitimate one; the process that owned it has almost certainly died.
func intentStuck(intent lifecycleIntent, now time.Time) bool {
	return intent.outcome == intentPending && now.Sub(intent.at) > intentPendingCover
}

// guestAgentLabel describes a guest's qemu-ga health as last recorded, with a
// cell status. The record is the latest check, not a history over the period.
func guestAgentLabel(vm VM, health *guestHealth) (string, string) {
	switch {
	case health == nil && vm.Running():
		return "not checked", ""
	case health == nil:
		return "not running", ""
	case health.agentTooOld:
		return "host agent too old", "warning"
	case health.reachable:
		return "answering", "success"
	case health.lastOKAt.IsZero():
		return "never answered", "warning"
	case health.failures >= unresponsiveFailures:
		return fmt.Sprintf("unresponsive (%d missed)", health.failures), "danger"
	default:
		return plural(health.failures, "missed check", "missed checks"), "warning"
	}
}

// assessStability classifies every guest's events and actions in [from, to).
func assessStability(inputs []stabilityVMInput, from, to, now time.Time) stabilityAssessment {
	var out stabilityAssessment
	for _, in := range inputs {
		out.guests++
		row := stabilityVMRow{vm: in.vm}
		row.agentText, row.agentStatus = guestAgentLabel(in.vm, in.health)
		if in.health != nil {
			row.checkedAt = in.health.checkedAt
			if in.health.diskAlert {
				out.diskAlerts = append(out.diskAlerts, in.vm.Name)
			}
			if in.vm.Running() && !in.health.agentTooOld && !in.health.lastOKAt.IsZero() &&
				in.health.failures >= unresponsiveFailures {
				out.unresponsive = append(out.unresponsive, in.vm.Name)
			}
		}

		var downs []time.Time
		for _, event := range in.events {
			if event.at.Before(from) || !event.at.Before(to) {
				continue
			}
			kind := classifyStabilityEvent(event, in.events, in.intents)
			switch kind {
			case "":
				continue
			case stabilityCrash:
				row.crashes++
				downs = append(downs, event.at)
			case stabilityUnexpectedStop:
				row.stops++
				downs = append(downs, event.at)
			case stabilityReset:
				row.resets++
				downs = append(downs, event.at)
			case stabilityGuestShutdown:
				row.guestShutdowns++
			case stabilityIOError:
				row.ioErrors++
			}
			detail := event.reason
			if event.device != "" {
				detail = strings.TrimSpace(detail + " " + event.device)
			}
			out.incidents = append(out.incidents, stabilityIncident{
				vmName: in.vm.Name, at: event.at, kind: kind, detail: detail,
			})
		}
		sort.Slice(downs, func(i, j int) bool { return downs[i].Before(downs[j]) })
		row.flapping = isFlapping(downs)
		if row.flapping {
			out.flapping = append(out.flapping, in.vm.Name)
		}

		for _, intent := range in.intents {
			if intent.at.Before(from) || !intent.at.Before(to) {
				continue
			}
			switch {
			case intent.outcome == intentFailed:
				row.failedIntents++
				out.intents = append(out.intents, stabilityIntentRow{vmName: in.vm.Name, intent: intent})
			case intentStuck(intent, now):
				row.stuckIntents++
				out.intents = append(out.intents, stabilityIntentRow{vmName: in.vm.Name, intent: intent, stuck: true})
			}
		}

		out.crashes += row.crashes
		out.stops += row.stops
		out.resets += row.resets
		out.ioErrors += row.ioErrors
		out.failed += row.failedIntents
		out.stuck += row.stuckIntents
		out.rows = append(out.rows, row)
	}

	sort.Slice(out.rows, func(i, j int) bool {
		a, b := out.rows[i], out.rows[j]
		if a.unintended() != b.unintended() {
			return a.unintended() > b.unintended()
		}
		return a.vm.Name < b.vm.Name
	})
	sort.Slice(out.incidents, func(i, j int) bool { return out.incidents[i].at.After(out.incidents[j].at) })
	sort.Slice(out.intents, func(i, j int) bool { return out.intents[i].intent.at.After(out.intents[j].intent.at) })
	sort.Strings(out.flapping)
	sort.Strings(out.unresponsive)
	sort.Strings(out.diskAlerts)
	return out
}

// stabilityFindings names what the operator must act on, worst first.
func stabilityFindings(a stabilityAssessment) []plugin.ReportFinding {
	var findings []plugin.ReportFinding
	if a.crashes > 0 {
		findings = append(findings, finding("critical",
			plural(a.crashes, "guest crash in this period", "guest crashes in this period"),
			"A guest kernel panicked or QEMU recorded a crash. Whatever the guest was serving was down until it was restarted.",
			"Read the guest's own kernel log for the panic, and the events table below for when it happened."))
	}
	if len(a.flapping) > 0 {
		findings = append(findings, finding("high",
			plural(len(a.flapping), "guest is flapping", "guests are flapping"),
			fmt.Sprintf("%s went down without being asked to at least %d times, or %d times within %d minutes: %s.",
				pluralNoun(len(a.flapping), "It", "They"), flapPeriodEvents, restartLoopDowns,
				int(restartLoopWindow.Minutes()), strings.Join(a.flapping, ", ")),
			"Stop the guest before it corrupts its disks further, then read its log to find what keeps bringing it down."))
	}
	if a.stops > 0 {
		findings = append(findings, finding("high",
			plural(a.stops, "unexpected stop", "unexpected stops"),
			"QEMU stopped these guests with no McHarbor action recorded: a signal on the host, the QEMU monitor, or an unknown cause.",
			"Check the host for an out-of-memory kill or a manual intervention at the times in the events table."))
	}
	if len(a.unresponsive) > 0 {
		findings = append(findings, finding("medium",
			plural(len(a.unresponsive), "guest agent is not answering", "guest agents are not answering"),
			"The QEMU guest agent answered before and has stopped: "+strings.Join(a.unresponsive, ", ")+". Without it, snapshots are crash-consistent and the guest cannot be shut down cleanly.",
			"Restart the qemu-guest-agent service inside each guest, or check whether the guest itself has hung."))
	}
	if a.failed > 0 || a.stuck > 0 {
		findings = append(findings, finding("medium",
			fmt.Sprintf("%d lifecycle actions failed and %d are stuck", a.failed, a.stuck),
			"A failed start, stop or reset leaves the guest in a state nobody chose; a stuck action never recorded an outcome, usually because McHarbor restarted while it ran.",
			"Check the actions table below, confirm each guest is in the state it should be, and retry what failed."))
	}
	if a.ioErrors > 0 {
		findings = append(findings, finding("medium",
			plural(a.ioErrors, "disk I/O error", "disk I/O errors"),
			"QEMU reported a failed read or write on a guest disk. A guest that pauses on I/O errors stays paused until the storage recovers.",
			"Check the free space and health of the pool behind each affected disk."))
	}
	if len(a.diskAlerts) > 0 {
		findings = append(findings, finding("medium",
			plural(len(a.diskAlerts), "guest filesystem is nearly full", "guest filesystems are nearly full"),
			"As last reported by the guest agent: "+strings.Join(a.diskAlerts, ", ")+".",
			"Free space inside the guest or grow its disk before writes start failing."))
	}
	return findings
}

// pluralNoun picks a word by count without printing the count.
func pluralNoun(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

// stabilityLead writes the stability report's opening paragraph.
func stabilityLead(a stabilityAssessment) string {
	if a.guests == 0 {
		return "No virtual machine was readable in this scope, so there is no stability to assess."
	}
	down := a.crashes + a.stops + a.resets
	if down == 0 && a.failed == 0 && a.stuck == 0 {
		return fmt.Sprintf("None of the %s in this scope crashed, stopped or reset without being asked to in this period, and every lifecycle action finished.",
			plural(a.guests, "guest", "guests"))
	}
	body := fmt.Sprintf("Across %s there were %s, %s and %s that no McHarbor action explains. ",
		plural(a.guests, "guest", "guests"), plural(a.crashes, "crash", "crashes"),
		plural(a.stops, "unexpected stop", "unexpected stops"), plural(a.resets, "reset", "resets"))
	if len(a.flapping) > 0 {
		body += fmt.Sprintf("%s repeatedly, which is the pattern to act on first. ",
			plural(len(a.flapping), "guest went down", "guests went down"))
	}
	body += "A stop somebody asked for through McHarbor, and a shutdown from inside the guest, are not counted as instability."
	return body
}

// stabilitySections lays the assessment out as report sections.
func stabilitySections(a stabilityAssessment) []plugin.ReportSection {
	summary := []plugin.ReportBlock{
		leadBlock(stabilityLead(a)),
		{
			Kind: "kpis",
			KPIs: []plugin.ReportKPI{
				{Label: "Guests", Value: fmt.Sprintf("%d", a.guests)},
				{Label: "Crashes", Value: fmt.Sprintf("%d", a.crashes), Status: kpiStatus(a.crashes == 0)},
				{Label: "Unexpected stops", Value: fmt.Sprintf("%d", a.stops), Status: kpiStatus(a.stops == 0)},
				{Label: "Unexpected resets", Value: fmt.Sprintf("%d", a.resets), Status: kpiStatus(a.resets == 0)},
				{Label: "Flapping guests", Value: fmt.Sprintf("%d", len(a.flapping)), Status: kpiStatus(len(a.flapping) == 0)},
				{Label: "Failed or stuck actions", Value: fmt.Sprintf("%d", a.failed+a.stuck), Status: kpiStatus(a.failed+a.stuck == 0)},
			},
		},
	}
	var points []plugin.ReportChartPoint
	for _, row := range a.rows {
		if row.unintended() == 0 || len(points) >= 15 {
			continue
		}
		tone := "warn"
		if row.flapping || row.crashes > 0 {
			tone = "bad"
		}
		points = append(points, chartPoint(row.vm.Name, float64(row.unintended()), tone))
	}
	if len(points) > 0 {
		summary = append(summary, hbarChart("Unintended downs per guest", "events", points, 0))
	}
	if findings := stabilityFindings(a); len(findings) > 0 {
		summary = append(summary, findingsBlock(findings))
	}

	sections := []plugin.ReportSection{
		{Level: "summary", Title: "Summary", Blocks: summary},
		{Level: "tactical", Title: "Stability by guest", Blocks: []plugin.ReportBlock{stabilityGuestTable(a)}},
	}
	if len(a.incidents) > 0 {
		sections = append(sections, plugin.ReportSection{
			Level: "operational", Title: "Events", Blocks: []plugin.ReportBlock{stabilityEventTable(a)},
		})
	}
	if len(a.intents) > 0 {
		sections = append(sections, plugin.ReportSection{
			Level: "operational", Title: "Failed and stuck actions", Blocks: []plugin.ReportBlock{stabilityIntentTable(a)},
		})
	}
	return sections
}

func countCell(count int, status string) plugin.ReportCell {
	cell := plugin.ReportCell{Text: fmt.Sprintf("%d", count)}
	if count > 0 {
		cell.Status = status
	}
	return cell
}

func stabilityGuestTable(a stabilityAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.rows))
	for _, row := range a.rows {
		flap := plugin.ReportCell{Text: "stable", Status: "success", Pill: true}
		if row.flapping {
			flap = plugin.ReportCell{Text: "flapping", Status: "danger", Pill: true}
		}
		checked := "—"
		if !row.checkedAt.IsZero() {
			checked = row.checkedAt.Format("2006-01-02 15:04")
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: row.vm.Name}, {Text: row.vm.EnvironmentID}, stateCell(row.vm.State),
			{Text: row.agentText, Status: row.agentStatus, Pill: row.agentStatus != ""}, {Text: checked},
			countCell(row.crashes, "danger"), countCell(row.stops, "danger"),
			countCell(row.resets, "warning"), {Text: fmt.Sprintf("%d", row.guestShutdowns)},
			countCell(row.failedIntents+row.stuckIntents, "warning"), flap,
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Guest"}, {Label: "Host"}, {Label: "State"},
			{Label: "Guest agent"}, {Label: "Last checked"},
			{Label: "Crashes", Align: "right"}, {Label: "Unexpected stops", Align: "right"},
			{Label: "Resets", Align: "right"}, {Label: "Guest shutdowns", Align: "right"},
			{Label: "Failed actions", Align: "right"}, {Label: "Pattern"},
		},
		Rows:    rows,
		Caption: "Least stable first.",
		Note: fmt.Sprintf("Guest agent health is the latest check, not a history. A guest flaps when it goes down unintentionally %d times in the period, or %d times within %d minutes.",
			flapPeriodEvents, restartLoopDowns, int(restartLoopWindow.Minutes())),
	}
}

func stabilityEventTable(a stabilityAssessment) plugin.ReportBlock {
	incidents := a.incidents
	note := ""
	if len(incidents) > stabilityIncidentRows {
		note = fmt.Sprintf("Showing the newest %d of %d events.", stabilityIncidentRows, len(incidents))
		incidents = incidents[:stabilityIncidentRows]
	}
	rows := make([][]plugin.ReportCell, 0, len(incidents))
	for _, incident := range incidents {
		status := "warning"
		if incident.kind == stabilityCrash || incident.kind == stabilityUnexpectedStop {
			status = "danger"
		}
		if incident.kind == stabilityGuestShutdown {
			status = ""
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: incident.at.Format("2006-01-02 15:04:05")}, {Text: incident.vmName},
			{Text: incident.kind, Status: status, Pill: true}, {Text: incident.detail, Mono: true},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "When"}, {Label: "Guest"}, {Label: "Kind"}, {Label: "QEMU reason", Mono: true},
		},
		Rows: rows,
		Note: note,
	}
}

func stabilityIntentTable(a stabilityAssessment) plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.intents))
	for _, entry := range a.intents {
		outcome := plugin.ReportCell{Text: "failed", Status: "danger", Pill: true}
		if entry.stuck {
			outcome = plugin.ReportCell{Text: "stuck", Status: "warning", Pill: true}
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: entry.intent.at.Format("2006-01-02 15:04")}, {Text: entry.vmName},
			{Text: entry.intent.action}, {Text: entry.intent.source}, outcome,
			{Text: entry.intent.errorText},
		})
	}
	return plugin.ReportBlock{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "When"}, {Label: "Guest"}, {Label: "Action"}, {Label: "Source"},
			{Label: "Outcome"}, {Label: "Error"},
		},
		Rows: rows,
		Note: fmt.Sprintf("An action is stuck when it has had no outcome for more than %d minutes.", int(intentPendingCover.Minutes())),
	}
}

// buildGuestStability reads guest health, QEMU events and lifecycle actions
// and composes the guest stability report.
func (r *reportProvider) buildGuestStability(
	ctx context.Context,
	req plugin.ReportRequest,
	guests []VM,
	sources []plugin.ReportSourceStatus,
	notes []string,
) (plugin.ReportResult, error) {
	svc := r.plugin.svc
	now := time.Now().UTC()
	from, to := reportWindow(req, now)

	if from.Before(now.Add(-intentRetention)) {
		notes = append(notes, fmt.Sprintf(
			"Events and lifecycle actions are kept for %d days, so the start of this period has no record and counts as quiet.",
			int(intentRetention.Hours()/24)))
	}

	started := time.Now()
	health, err := svc.guestHealthRows(ctx, "")
	healthSource := plugin.ReportSourceStatus{Key: "kvm_guest_health", Label: "Guest agent health",
		OK: err == nil, DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		r.plugin.logger.Warn("kvm: report could not read guest health", "error", err)
		healthSource.Note = "Guest agent health could not be read and is shown as not checked."
		health = nil
	}

	started = time.Now()
	events, err := svc.readEvents(ctx, "", from)
	eventSource := plugin.ReportSourceStatus{Key: "kvm_vm_events", Label: "QEMU lifecycle events",
		OK: err == nil, DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		r.plugin.logger.Warn("kvm: report could not read QEMU events", "error", err)
		eventSource.Note = "QEMU events could not be read, so crashes, stops and resets are not counted."
		events = nil
	} else if countGroups(events) >= incidentHistoryRows {
		notes = append(notes, fmt.Sprintf(
			"More than %d QEMU events fell in this period; only the newest were read, so the earliest part of the period is undercounted.",
			incidentHistoryRows))
	}

	started = time.Now()
	intents, err := svc.readIntents(ctx, "", from.Add(-intentPendingCover-intentSkew))
	intentSource := plugin.ReportSourceStatus{Key: "kvm_lifecycle_intents", Label: "Recorded lifecycle actions",
		OK: err == nil, DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		r.plugin.logger.Warn("kvm: report could not read lifecycle actions", "error", err)
		intentSource.Note = "Recorded actions could not be read, so no stop can be told apart from one somebody asked for."
		intents = nil
	}
	sources = append(sources, healthSource, eventSource, intentSource)

	inputs := make([]stabilityVMInput, 0, len(guests))
	for _, vm := range guests {
		inputs = append(inputs, stabilityVMInput{
			vm: vm, health: healthPointer(health, vm.ID),
			events: events[vm.ID], intents: intents[vm.ID],
		})
	}

	assessment := assessStability(inputs, from, to, now)
	return plugin.ReportResult{Sections: stabilitySections(assessment), Sources: sources, Notes: notes}, nil
}

// countGroups totals the rows across a grouped read.
func countGroups[T any](groups map[string][]T) int {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	return total
}
