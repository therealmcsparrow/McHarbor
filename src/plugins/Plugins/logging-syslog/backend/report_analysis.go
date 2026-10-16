// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// forwarderRow is one configured syslog forwarder as the host's registry
// knows it, with its last successful event delivery at or before the
// period end, however old.
type forwarderRow struct {
	ID           string
	Name         string
	Enabled      bool
	EnabledSince time.Time
	LastSuccess  time.Time
}

// totalsRow is one forwarder's delivery counts inside the period. Test
// sends are counted apart because an operator clicking Test while
// debugging would otherwise skew the rate either way. Protocols is every
// transport the attempts used, since a forwarder can be switched mid-period.
type totalsRow struct {
	ForwarderID  string
	Delivered    int
	Failed       int
	Tests        int
	Undelivered  int
	FirstSuccess time.Time
	LastSuccess  time.Time
	LastFailure  time.Time
	Protocols    []string
}

// gapRow is the longest stretch between two consecutive successful
// deliveries of one forwarder inside the period.
type gapRow struct {
	ForwarderID string
	From        time.Time
	To          time.Time
}

// errorRow is one distinct stored error text for one forwarder.
type errorRow struct {
	ForwarderID string
	Error       string
	Count       int
	LastSeen    time.Time
}

// deliveryThresholds are the points at which a figure becomes a finding.
type deliveryThresholds struct {
	WarnRate        float64
	BadRate         float64
	Silent          time.Duration
	RecurringErrors int
}

// defaultThresholds are tuned for an audit trail: 95% still loses one
// event in twenty, and a day without a delivery is long enough to miss
// an incident's whole timeline in the collector.
func defaultThresholds() deliveryThresholds {
	return deliveryThresholds{
		WarnRate:        95,
		BadRate:         80,
		Silent:          24 * time.Hour,
		RecurringErrors: 5,
	}
}

// deliveryInput is everything the analysis reads. It is plain data so
// the arithmetic can be tested without a database.
type deliveryInput struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	Forwarders  []forwarderRow
	Totals      []totalsRow
	Gaps        []gapRow
	Errors      []errorRow
	Thresholds  deliveryThresholds
}

// forwarderStat is one forwarder's computed figures.
type forwarderStat struct {
	ID                  string
	Name                string
	Known               bool
	Enabled             bool
	Delivered           int
	Failed              int
	Tests               int
	Undelivered         int
	Rate                float64
	HasRate             bool
	Gap                 time.Duration
	GapFrom             time.Time
	GapTo               time.Time
	HasGap              bool
	LastSuccess         time.Time
	Silent              time.Duration
	HasSilence          bool
	FailingSinceSuccess bool
	Protocols           []string
}

// errorGroup is one normalised failure reason across forwarders.
type errorGroup struct {
	Pattern    string
	Count      int
	LastSeen   time.Time
	Forwarders []string
}

// deliveryAnalysis is the result of analyseDelivery.
type deliveryAnalysis struct {
	Forwarders     []forwarderStat
	Errors         []errorGroup
	Findings       []plugin.ReportFinding
	Delivered      int
	Failed         int
	Tests          int
	Undelivered    int
	Rate           float64
	HasRate        bool
	LongestGap     time.Duration
	LongestGapName string
}

// maxErrorFindings caps how many recurring-error findings one report
// raises, so a collector returning five variants of one outage does not
// bury every other finding.
const maxErrorFindings = 5

// analyseDelivery turns plain rows into per-forwarder figures, grouped
// failure reasons and findings.
func analyseDelivery(in deliveryInput) deliveryAnalysis {
	th := in.Thresholds
	if th == (deliveryThresholds{}) {
		th = defaultThresholds()
	}

	totals := map[string]totalsRow{}
	for _, t := range in.Totals {
		totals[t.ForwarderID] = t
	}
	gaps := map[string]gapRow{}
	for _, g := range in.Gaps {
		gaps[g.ForwarderID] = g
	}

	var out deliveryAnalysis
	names := map[string]string{}
	known := map[string]bool{}
	for _, f := range in.Forwarders {
		known[f.ID] = true
		names[f.ID] = forwarderName(f.ID, f.Name, true)
		out.Forwarders = append(out.Forwarders, forwarderStats(in, th, f, true, totals[f.ID], gaps[f.ID]))
	}
	for _, t := range in.Totals {
		if known[t.ForwarderID] {
			continue
		}
		names[t.ForwarderID] = forwarderName(t.ForwarderID, "", false)
		out.Forwarders = append(out.Forwarders,
			forwarderStats(in, th, forwarderRow{ID: t.ForwarderID}, false, t, gaps[t.ForwarderID]))
	}
	sort.SliceStable(out.Forwarders, func(i, j int) bool {
		return out.Forwarders[i].Name < out.Forwarders[j].Name
	})

	for _, s := range out.Forwarders {
		out.Delivered += s.Delivered
		out.Failed += s.Failed
		out.Tests += s.Tests
		out.Undelivered += s.Undelivered
		if s.HasGap && s.Gap > out.LongestGap {
			out.LongestGap = s.Gap
			out.LongestGapName = s.Name
		}
	}
	if attempts := out.Delivered + out.Failed; attempts > 0 {
		out.Rate = float64(out.Delivered) / float64(attempts) * 100
		out.HasRate = true
	}

	out.Errors = groupErrors(in.Errors, names)
	out.Findings = deliveryFindings(out, th)
	return out
}

func forwarderStats(in deliveryInput, th deliveryThresholds, f forwarderRow, isKnown bool, t totalsRow, g gapRow) forwarderStat {
	s := forwarderStat{
		ID:          f.ID,
		Name:        forwarderName(f.ID, f.Name, isKnown),
		Known:       isKnown,
		Enabled:     isKnown && f.Enabled,
		Delivered:   t.Delivered,
		Failed:      t.Failed,
		Tests:       t.Tests,
		Undelivered: t.Undelivered,
		LastSuccess: f.LastSuccess,
		Protocols:   t.Protocols,
	}
	if s.LastSuccess.IsZero() || t.LastSuccess.After(s.LastSuccess) {
		s.LastSuccess = t.LastSuccess
	}
	if attempts := t.Delivered + t.Failed; attempts > 0 {
		s.Rate = float64(t.Delivered) / float64(attempts) * 100
		s.HasRate = true
	}

	windowStart := in.PeriodStart
	if f.EnabledSince.After(windowStart) {
		windowStart = f.EnabledSince
	}
	consider := func(from, to time.Time) {
		if from.IsZero() || to.IsZero() || !to.After(from) {
			return
		}
		if d := to.Sub(from); !s.HasGap || d > s.Gap {
			s.Gap, s.GapFrom, s.GapTo, s.HasGap = d, from, to, true
		}
	}
	consider(g.From, g.To)
	// Edge gaps are only meaningful while the forwarder is switched on:
	// a disabled forwarder's quiet tail is the operator's choice, and a
	// deleted one's lifetime is not recorded anywhere to bound it.
	if s.Enabled && windowStart.Before(in.PeriodEnd) {
		if t.FirstSuccess.IsZero() {
			consider(windowStart, in.PeriodEnd)
		} else {
			consider(windowStart, t.FirstSuccess)
			consider(t.LastSuccess, in.PeriodEnd)
		}
	}

	if s.Enabled {
		ref := s.LastSuccess
		if ref.IsZero() {
			ref = f.EnabledSince
		}
		if !ref.IsZero() && in.PeriodEnd.After(ref) {
			s.Silent = in.PeriodEnd.Sub(ref)
			s.HasSilence = s.Silent > th.Silent
		}
		s.FailingSinceSuccess = t.Failed > 0 && t.LastFailure.After(s.LastSuccess)
	}
	return s
}

func forwarderName(id, name string, isKnown bool) string {
	switch {
	case strings.TrimSpace(name) != "":
		return strings.TrimSpace(name)
	case id == "":
		return "Unattributed history"
	case isKnown:
		return id
	default:
		return "Removed forwarder " + id
	}
}

var (
	addressPattern    = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b|\[[0-9a-fA-F:]+\](?::\d+)?`)
	longNumberPattern = regexp.MustCompile(`\d{4,}`)
	spacePattern      = regexp.MustCompile(`\s+`)
)

// maxPatternRunes keeps a grouped reason readable in a table cell and a
// finding title.
const maxPatternRunes = 160

// normaliseError reduces one stored error to a grouping key. Addresses
// and long numbers vary between attempts of the same outage; short
// numbers such as a port are kept because they name which endpoint failed.
func normaliseError(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "(no error text recorded)"
	}
	s = addressPattern.ReplaceAllString(s, "<address>")
	s = longNumberPattern.ReplaceAllString(s, "<n>")
	s = spacePattern.ReplaceAllString(s, " ")
	if utf8.RuneCountInString(s) > maxPatternRunes {
		runes := []rune(s)
		s = string(runes[:maxPatternRunes-1]) + "…"
	}
	return s
}

func groupErrors(rows []errorRow, names map[string]string) []errorGroup {
	byPattern := map[string]*errorGroup{}
	seen := map[string]map[string]bool{}
	for _, r := range rows {
		if r.Count <= 0 {
			continue
		}
		key := normaliseError(r.Error)
		grp, ok := byPattern[key]
		if !ok {
			grp = &errorGroup{Pattern: key}
			byPattern[key] = grp
			seen[key] = map[string]bool{}
		}
		grp.Count += r.Count
		if r.LastSeen.After(grp.LastSeen) {
			grp.LastSeen = r.LastSeen
		}
		name, ok := names[r.ForwarderID]
		if !ok {
			name = forwarderName(r.ForwarderID, "", false)
		}
		if !seen[key][name] {
			seen[key][name] = true
			grp.Forwarders = append(grp.Forwarders, name)
		}
	}
	out := make([]errorGroup, 0, len(byPattern))
	for _, grp := range byPattern {
		sort.Strings(grp.Forwarders)
		out = append(out, *grp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Pattern < out[j].Pattern
	})
	return out
}

func deliveryFindings(a deliveryAnalysis, th deliveryThresholds) []plugin.ReportFinding {
	var out []plugin.ReportFinding
	for _, s := range a.Forwarders {
		// A removed forwarder stays in the table but raises nothing: there
		// is no forwarder left to test or fix.
		if s.Known && s.HasRate && s.Rate < th.WarnRate {
			severity := "medium"
			if s.Rate < th.BadRate {
				severity = "high"
			}
			out = append(out, plugin.ReportFinding{
				Severity: severity,
				Title:    fmt.Sprintf("%s delivered only %s of its attempts", s.Name, formatRate(s.Rate)),
				Detail: fmt.Sprintf("%d of %d delivery attempts failed in the period, and %d audit events had no successful delivery by the end of it.",
					s.Failed, s.Delivered+s.Failed, s.Undelivered),
				Recommendation: "Run Test on this forwarder in Settings > Integrations > Logging and resolve the most frequent failure reason listed in this report. Each event is retried only up to the forwarder's Max Retries, after which it is never sent.",
				Owner:          "Security operations",
				Evidence:       fmt.Sprintf("delivered=%d failed=%d undelivered=%d", s.Delivered, s.Failed, s.Undelivered),
			})
		}
		if s.HasSilence {
			f := plugin.ReportFinding{
				Severity:       "medium",
				Title:          fmt.Sprintf("%s has not delivered an event for %s", s.Name, formatDuration(s.Silent)),
				Detail:         "No attempt failed in that time either, so the audit log may simply have been quiet; confirm events were being recorded before treating this as an outage.",
				Recommendation: "Run Test on this forwarder to confirm the collector still accepts events, and check that the forwarder is still the destination your log pipeline expects.",
				Owner:          "Security operations",
				Evidence:       "last successful delivery: " + formatTime(s.LastSuccess),
			}
			if s.FailingSinceSuccess {
				f.Severity = "high"
				f.Detail = "Delivery attempts have been failing since the last success, so audit events from this window are missing from the collector."
			}
			out = append(out, f)
		}
	}
	raised := 0
	for _, grp := range a.Errors {
		if grp.Count < th.RecurringErrors || raised >= maxErrorFindings {
			continue
		}
		raised++
		out = append(out, plugin.ReportFinding{
			Severity: "medium",
			Title:    "Recurring failure: " + grp.Pattern,
			Detail: fmt.Sprintf("Seen %d times on %s, most recently at %s.",
				grp.Count, strings.Join(grp.Forwarders, ", "), formatTime(grp.LastSeen)),
			Recommendation: "Fix the cause at the collector or in the forwarder's settings; a failure that repeats is a configuration or capacity problem, not a transient one.",
			Owner:          "Security operations",
			Evidence:       fmt.Sprintf("occurrences=%d", grp.Count),
		})
	}
	rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

func formatRate(rate float64) string {
	return fmt.Sprintf("%.1f%%", rate)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	days := int(d / (24 * time.Hour))
	hours := int(d % (24 * time.Hour) / time.Hour)
	minutes := int(d % time.Hour / time.Minute)
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

func rateStatus(rate float64, hasRate bool, th deliveryThresholds) string {
	switch {
	case !hasRate:
		return "muted"
	case rate < th.BadRate:
		return "bad"
	case rate < th.WarnRate:
		return "warn"
	default:
		return "good"
	}
}

// buildDeliverySections lays the analysis out by altitude: the verdict
// for a reader who stops at the summary, the per-forwarder table for the
// one who owns the pipeline, and the raw reasons for the one fixing it.
func buildDeliverySections(a deliveryAnalysis, th deliveryThresholds, start, end time.Time) []plugin.ReportSection {
	if th == (deliveryThresholds{}) {
		th = defaultThresholds()
	}
	if len(a.Forwarders) == 0 {
		return []plugin.ReportSection{{
			Level: "summary",
			Title: "Summary",
			Blocks: []plugin.ReportBlock{{
				Kind:  "callout",
				Tone:  "info",
				Title: "Nothing to report",
				Text:  "No syslog forwarder is configured and no delivery was recorded in this period.",
			}},
		}}
	}

	rate := "—"
	if a.HasRate {
		rate = formatRate(a.Rate)
	}
	gap := "—"
	gapHint := ""
	if a.LongestGap > 0 {
		gap = formatDuration(a.LongestGap)
		gapHint = "On " + a.LongestGapName + ". A quiet audit log also produces a gap, so read it next to the failure count."
	}
	undeliveredStatus := "good"
	if a.Undelivered > 0 {
		undeliveredStatus = "warn"
	}
	lead := fmt.Sprintf("Between %s and %s, %d syslog forwarders made %d event delivery attempts, of which %d succeeded and %d failed.",
		formatTime(start), formatTime(end), len(a.Forwarders), a.Delivered+a.Failed, a.Delivered, a.Failed)

	summary := plugin.ReportSection{
		Level: "summary",
		Title: "Summary",
		Blocks: []plugin.ReportBlock{
			{Kind: "paragraph", Text: lead, Lead: true},
			{Kind: "kpis", KPIs: []plugin.ReportKPI{
				{Label: "Forwarders", Value: fmt.Sprintf("%d", len(a.Forwarders))},
				{Label: "Delivered", Value: fmt.Sprintf("%d", a.Delivered)},
				{Label: "Failed", Value: fmt.Sprintf("%d", a.Failed), Status: failedStatus(a.Failed)},
				{Label: "Success rate", Value: rate, Status: rateStatus(a.Rate, a.HasRate, th),
					Target: formatRate(th.WarnRate)},
				{Label: "Events not delivered", Value: fmt.Sprintf("%d", a.Undelivered), Status: undeliveredStatus,
					Hint: "Audit events with failed attempts and no success yet. Some may still be inside their retry budget."},
				{Label: "Longest gap", Value: gap, Hint: gapHint},
			}},
		},
	}

	strategic := plugin.ReportSection{Level: "strategic", Title: "Assessment"}
	if len(a.Findings) == 0 {
		strategic.Blocks = append(strategic.Blocks, plugin.ReportBlock{
			Kind:  "callout",
			Tone:  "good",
			Title: "No findings",
			Text:  "Every forwarder stayed above the success threshold, delivered within the silence threshold, and no failure reason recurred.",
		})
	} else {
		strategic.Blocks = append(strategic.Blocks, plugin.ReportBlock{Kind: "findings", Findings: a.Findings})
	}
	var points []plugin.ReportChartPoint
	for _, s := range a.Forwarders {
		if s.HasRate {
			points = append(points, plugin.ReportChartPoint{Label: s.Name, Value: s.Rate, Status: rateStatus(s.Rate, true, th)})
		}
	}
	if len(points) > 0 {
		strategic.Blocks = append(strategic.Blocks, plugin.ReportBlock{Kind: "chart", Chart: &plugin.ReportChart{
			Kind:        "hbar",
			Title:       "Success rate by forwarder",
			Unit:        "%",
			Max:         100,
			Target:      th.WarnRate,
			TargetLabel: "Threshold",
			HasTarget:   true,
			ShowValues:  true,
			Series:      []plugin.ReportChartSeries{{Name: "Success rate", Points: points}},
		}})
	}

	rows := make([][]plugin.ReportCell, 0, len(a.Forwarders))
	for _, s := range a.Forwarders {
		rowRate := plugin.ReportCell{Text: "—", Status: "muted"}
		if s.HasRate {
			rowRate = plugin.ReportCell{Text: formatRate(s.Rate), Status: rateStatus(s.Rate, true, th)}
		}
		rowGap := plugin.ReportCell{Text: "—"}
		if s.HasGap {
			rowGap = plugin.ReportCell{Text: formatDuration(s.Gap)}
			if s.Gap > th.Silent {
				rowGap.Status = "warn"
			}
		}
		lastSuccess := plugin.ReportCell{Text: formatTime(s.LastSuccess)}
		if s.HasSilence {
			lastSuccess.Status = "bad"
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: s.Name},
			forwarderStateCell(s),
			transportCell(s.Protocols),
			{Text: fmt.Sprintf("%d", s.Delivered)},
			{Text: fmt.Sprintf("%d", s.Failed), Status: failedStatus(s.Failed)},
			rowRate,
			{Text: fmt.Sprintf("%d", s.Undelivered)},
			rowGap,
			lastSuccess,
			{Text: fmt.Sprintf("%d", s.Tests)},
		})
	}
	tactical := plugin.ReportSection{
		Level: "tactical",
		Title: "Delivery by forwarder",
		Blocks: []plugin.ReportBlock{{
			Kind: "table",
			Columns: []plugin.ReportColumn{
				{Label: "Forwarder", Width: 2}, {Label: "State"}, {Label: "Transport"},
				{Label: "Delivered", Align: "right"}, {Label: "Failed", Align: "right"},
				{Label: "Success rate", Align: "right"}, {Label: "Not delivered", Align: "right"},
				{Label: "Longest gap", Align: "right"}, {Label: "Last success", Width: 1.5},
				{Label: "Test sends", Align: "right"},
			},
			Rows: rows,
			Note: "Counts are delivery attempts for audit events; a retried event counts once per attempt. Test sends are listed apart and excluded from the rate. Last success looks back beyond the period.",
		}},
	}
	if usesUDP(a.Forwarders) {
		tactical.Blocks = append(tactical.Blocks, plugin.ReportBlock{
			Kind:  "callout",
			Tone:  "warn",
			Title: "UDP success is not proof of delivery",
			Text:  "Over UDP a success only means the datagram left this host; the collector never confirms receipt, so a UDP forwarder's success rate overstates what arrived. Switch it to TLS to have delivery confirmed.",
		})
	}

	operational := plugin.ReportSection{Level: "operational", Title: "Failure reasons"}
	if len(a.Errors) == 0 {
		operational.Blocks = append(operational.Blocks, plugin.ReportBlock{
			Kind: "paragraph", Text: "No delivery attempt failed in this period.",
		})
	} else {
		errRows := make([][]plugin.ReportCell, 0, len(a.Errors))
		for _, grp := range a.Errors {
			status := ""
			if grp.Count >= th.RecurringErrors {
				status = "warn"
			}
			errRows = append(errRows, []plugin.ReportCell{
				{Text: grp.Pattern, Mono: true},
				{Text: strings.Join(grp.Forwarders, ", ")},
				{Text: fmt.Sprintf("%d", grp.Count), Status: status},
				{Text: formatTime(grp.LastSeen)},
			})
		}
		operational.Blocks = append(operational.Blocks, plugin.ReportBlock{
			Kind: "table",
			Columns: []plugin.ReportColumn{
				{Label: "Reason", Width: 3, Mono: true}, {Label: "Forwarders", Width: 1.5},
				{Label: "Occurrences", Align: "right"}, {Label: "Last seen", Width: 1.5},
			},
			Rows:    errRows,
			Caption: "Most frequent first.",
			Note:    "Addresses and long numbers are folded together so one outage reads as one reason.",
		})
	}
	var gapRows [][]plugin.ReportCell
	for _, s := range a.Forwarders {
		if !s.HasGap {
			continue
		}
		gapRows = append(gapRows, []plugin.ReportCell{
			{Text: s.Name}, {Text: formatTime(s.GapFrom)}, {Text: formatTime(s.GapTo)}, {Text: formatDuration(s.Gap)},
		})
	}
	if len(gapRows) > 0 {
		operational.Blocks = append(operational.Blocks, plugin.ReportBlock{
			Kind: "table",
			Columns: []plugin.ReportColumn{
				{Label: "Forwarder", Width: 2}, {Label: "From", Width: 1.5}, {Label: "To", Width: 1.5},
				{Label: "Duration", Align: "right"},
			},
			Rows:    gapRows,
			Caption: "Longest stretch without a successful delivery, per forwarder.",
		})
	}

	return []plugin.ReportSection{summary, strategic, tactical, operational}
}

// usesUDP reports whether any forwarder sent events over UDP in the
// period, which is when the success figures need a caveat.
func usesUDP(stats []forwarderStat) bool {
	for _, s := range stats {
		for _, p := range s.Protocols {
			if p == "udp" {
				return true
			}
		}
	}
	return false
}

func transportCell(protocols []string) plugin.ReportCell {
	if len(protocols) == 0 {
		return plugin.ReportCell{Text: "—"}
	}
	cell := plugin.ReportCell{Text: strings.Join(protocols, ", "), Mono: true}
	for _, p := range protocols {
		if p == "udp" {
			cell.Status = "warn"
		}
	}
	return cell
}

func failedStatus(failed int) string {
	if failed > 0 {
		return "warn"
	}
	return "good"
}

func forwarderStateCell(s forwarderStat) plugin.ReportCell {
	switch {
	case !s.Known:
		return plugin.ReportCell{Text: "removed", Status: "muted", Pill: true}
	case !s.Enabled:
		return plugin.ReportCell{Text: "disabled", Status: "muted", Pill: true}
	case s.HasSilence && s.FailingSinceSuccess:
		return plugin.ReportCell{Text: "failing", Status: "bad", Pill: true}
	case s.HasSilence:
		return plugin.ReportCell{Text: "silent", Status: "warn", Pill: true}
	default:
		return plugin.ReportCell{Text: "enabled", Status: "good", Pill: true}
	}
}
