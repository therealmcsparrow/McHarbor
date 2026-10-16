// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"sort"
	"strings"
)

// Report builders.
//
// Each returns the sections for its own type; buildDocument filters them
// to the levels the caller asked for and wraps them in the summary and
// the annex. A builder never renders — it only says what the report
// contains — which is what lets one report drive the screen, the PDF and
// the Word file without three copies of the content.

func section(l Level, title string, blocks ...Block) Section {
	return Section{Level: l, Title: title, Intent: levelIntent(l), Blocks: blocks}
}

// --- shared block factories ------------------------------------------

// postureTable renders the scorecard as a table. The chart shows the
// shape; the table carries the reasoning, and a board asks for the
// reasoning.
func postureTable(sc *Scorecard) Block {
	rows := make([][]Cell, 0, len(sc.Domains))
	for _, d := range sc.Domains {
		score := "—"
		if d.Score >= 0 {
			score = fmtInt(d.Score)
		}
		rows = append(rows, []Cell{
			cell(d.Label),
			cellStatus(score, bandStatus(d.Band)),
			pill(bandLabel(d.Band), bandStatus(d.Band)),
			cell(d.Note),
		})
	}
	return table(
		[]Column{
			{Label: "Domain", Width: 1.5},
			{Label: "Score", Width: 0.8, Align: "right"},
			{Label: "Band", Width: 1.2},
			{Label: "Principal driver", Width: 4.5},
		},
		rows,
		"Posture by domain",
		"Scores follow the published rules in the Methodology annex. Domains marked “not assessed” are excluded from the overall figure.")
}

func postureChart(sc *Scorecard) Block {
	var pts []ChartPoint
	for _, d := range sc.Domains {
		if d.Score < 0 {
			continue
		}
		pts = append(pts, ChartPoint{Label: d.Label, Value: float64(d.Score), Status: bandStatus(d.Band)})
	}
	if len(pts) == 0 {
		return divider()
	}
	return chartBlock(Chart{
		Kind: ChartHBar, Title: "Posture by domain", Max: 100, ShowValues: true, Height: 130,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "Higher is better. Bands: 85+ strong, 70–84 stable, 55–69 watch, 35–54 at risk, below 35 critical.",
	})
}

func workloadStateChart(f Facts) Block {
	pts := []ChartPoint{
		{Label: "Running", Value: float64(f.Containers.Running - f.Containers.Unhealthy), Status: StatusGood},
		{Label: "Unhealthy", Value: float64(f.Containers.Unhealthy), Status: StatusBad},
		{Label: "Restarting", Value: float64(f.Containers.Restarting), Status: StatusWarn},
		{Label: "Paused", Value: float64(f.Containers.Paused), Status: StatusInfo},
		{Label: "Stopped", Value: float64(f.Containers.Stopped), Status: StatusMuted},
	}
	var kept []ChartPoint
	for _, p := range pts {
		if p.Value > 0 {
			kept = append(kept, p)
		}
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Workload state", Unit: "workloads", Height: 130,
		Series: []ChartSeries{{Points: kept}},
	})
}

func eventTrendChart(f Facts, title, caption string) Block {
	if len(f.Reliability.EventsByDay) == 0 {
		return divider()
	}
	pts := make([]ChartPoint, 0, len(f.Reliability.EventsByDay))
	for _, d := range f.Reliability.EventsByDay {
		pts = append(pts, ChartPoint{Label: d.Label, Value: float64(d.Count)})
	}
	return chartBlock(Chart{
		Kind: ChartArea, Title: title, Height: 120,
		Series: []ChartSeries{{Name: "Events", Points: pts}}, Caption: caption,
	})
}

// containerTable is the operational inventory every workload report ends
// with. Long tables are truncated with an explicit note rather than
// silently cut — a reader must never wonder whether they saw everything.
func containerTable(snap *Snapshot, limit int) Block {
	rows := make([][]Cell, 0, len(snap.Containers))
	list := append([]containerRow(nil), snap.Containers...)
	sort.Slice(list, func(i, j int) bool {
		if list[i].StackName != list[j].StackName {
			return list[i].StackName < list[j].StackName
		}
		return containerName(list[i]) < containerName(list[j])
	})
	truncated := false
	for i, c := range list {
		if limit > 0 && i >= limit {
			truncated = true
			break
		}
		st := StatusNeutral
		state := strings.ToLower(c.State)
		switch {
		case strings.Contains(strings.ToLower(c.Status), "(unhealthy)"):
			st = StatusBad
		case state == "running":
			st = StatusGood
		case state == "restarting":
			st = StatusWarn
		}
		stack := c.StackName
		if stack == "" {
			stack = "—"
		}
		rows = append(rows, []Cell{
			cell(containerName(c)),
			pill(titleCase(c.State), st),
			cell(shortImage(c.Image)),
			cell(stack),
			cell(c.Status),
		})
	}
	note := ""
	if truncated {
		note = "Showing the first " + fmtInt(limit) + " of " + fmtInt(len(list)) +
			" workloads, ordered by stack then name."
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.4},
			{Label: "State", Width: 1.1},
			{Label: "Image", Width: 3},
			{Label: "Stack", Width: 1.6},
			{Label: "Status", Width: 2.2},
		}, rows, "Workload inventory", note)
}

// --- measured availability, shared across the reports that use it ----
//
// Everything below reads AvailabilityFacts, which comes from the health and
// uptime history rather than from the current state. The distinction is
// stated in the prose deliberately and repeatedly: a reader who confuses
// "healthy now" with "was up all month" will draw the wrong conclusion from
// an otherwise correct number.

// historyCoverageNote is the sentence every availability figure is qualified
// by. It is not optional decoration — the raw series is pruned weekly, so a
// 90-day report may be describing 30 days, and saying so is the difference
// between a report and a misleading one.
func historyCoverageNote(a AvailabilityFacts) string {
	if !a.Assessed {
		return ""
	}
	base := "Measured from " + plural(a.ObservedDays, "day", "days") + " of recorded history across " +
		plural(a.Tracked, "workload", "workloads") + "."
	if a.Partial {
		base += " The period requested was " + plural(a.RequestedDays, "day", "days") +
			"; the platform retains less history than that, so every figure here describes the " +
			"shorter window and not the period on the cover."
	}
	return base
}

func availabilityCoverageCallout(a AvailabilityFacts) Block {
	if !a.Assessed {
		return callout(StatusInfo, "No measured history",
			"The health and uptime history could not be read, so availability in this report is "+
				"inferred from the current state of each workload. That describes the moment the "+
				"report ran, not the period it covers: a workload that was down for hours and came "+
				"back before generation will appear entirely healthy.")
	}
	if a.Partial {
		return callout(StatusWarn, "History is shorter than the period",
			historyCoverageNote(a))
	}
	return callout(StatusInfo, "How availability is measured", historyCoverageNote(a)+
		" Uptime is the share of observed time a workload was running, sampled every "+
		fmtInt(30)+" seconds. It is a floor on service quality: the platform can see that a "+
		"process was up, not that it was answering correctly.")
}

// availabilityKPIs is the headline row for any report that leads with
// measured availability.
func availabilityKPIs(a AvailabilityFacts) []KPI {
	if !a.Assessed {
		return nil
	}
	out := []KPI{
		{Label: "Measured uptime", Value: fmtUptime(a.UptimePercent),
			Target: fmtDecimal(a.TargetPercent, 1) + "%",
			Status: thresholdStatus(a.UptimePercent, a.TargetPercent, 99),
			Hint:   "Time-weighted across the fleet"},
		{Label: "Total downtime", Value: fmtDuration(a.DowntimeMinutes),
			Status: countStatus(a.WithDowntime, 0, 2),
			Hint:   plural(a.WithDowntime, "workload affected", "workloads affected")},
		{Label: "Outages", Value: fmtInt(a.Outages),
			Status: countStatus(a.Outages, 0, 5),
			Hint:   fmtDecimal(a.OutageRatePerDay, 2) + " per day"},
	}
	if a.Outages > 0 {
		out = append(out, KPI{Label: "Mean time to recover", Value: fmtDuration(a.MTTRMinutes),
			Target: "< 30 min", Status: invThresholdStatus(a.MTTRMinutes, 30, 120)})
	}
	if a.Probes > 0 {
		out = append(out, KPI{Label: "Health check pass rate", Value: fmtUptime(a.ProbePassPercent),
			Target: "100%", Status: thresholdStatus(a.ProbePassPercent, 99, 95),
			Hint: fmtInt(a.Probes) + " probes"})
	}
	out = append(out, KPI{Label: "Error budget left", Value: errorBudgetValue(a),
		Target: fmtDecimal(a.TargetPercent, 1) + "%",
		Status: thresholdStatus(a.ErrorBudgetPercent, 50, 0),
		Hint:   "Of the downtime the target permits"})
	return out
}

func errorBudgetValue(a AvailabilityFacts) string {
	if a.ErrorBudgetPercent < 0 {
		return "spent"
	}
	return fmtPercent(a.ErrorBudgetPercent)
}

// availabilityTable is the per-workload record: the table a service owner
// opens the report to find.
func availabilityTable(a AvailabilityFacts, limit int) Block {
	rows := make([][]Cell, 0, len(a.Rows))
	list := append([]uptimeRow(nil), a.Rows...)
	sort.Slice(list, func(i, j int) bool {
		if list[i].UptimePercent != list[j].UptimePercent {
			return list[i].UptimePercent < list[j].UptimePercent
		}
		return uptimeLabel(list[i]) < uptimeLabel(list[j])
	})

	truncated := false
	for _, r := range list {
		if r.Samples == 0 {
			continue
		}
		// Count against rows actually emitted, not the loop index: a skipped
		// row would otherwise consume a slot and the table would stop short
		// of the limit it claims in its own note.
		if limit > 0 && len(rows) >= limit {
			truncated = true
			break
		}
		mttr := "—"
		if r.Outages > 0 {
			mttr = fmtDuration(r.MTTRMinutes)
		}
		probe := "—"
		if r.Probes > 0 {
			probe = fmtUptime(r.ProbePassPercent)
		}
		rows = append(rows, []Cell{
			cell(uptimeLabel(r)),
			cellStatus(fmtUptime(r.UptimePercent), thresholdStatus(r.UptimePercent, a.TargetPercent, 99)),
			cell(fmtDuration(r.DowntimeMinutes)),
			cellStatus(fmtInt(r.Outages), countStatus(r.Outages, 0, 3)),
			cell(mttr),
			cellStatus(probe, probeStatus(r)),
			cell(fmtInt(r.Days)),
		})
	}

	note := "Ordered worst first. Downtime is observed time the workload was not running; " +
		"a workload created part-way through the period is measured only from when it appeared."
	if truncated {
		note = "Showing the " + fmtInt(limit) + " workloads with the lowest availability, of " +
			fmtInt(len(list)) + " tracked. " + note
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.6},
			{Label: "Uptime", Width: 1.1, Align: "right"},
			{Label: "Downtime", Width: 1.2, Align: "right"},
			{Label: "Outages", Width: 1, Align: "right"},
			{Label: "MTTR", Width: 1.1, Align: "right"},
			{Label: "Probes passed", Width: 1.3, Align: "right"},
			{Label: "Days", Width: 0.8, Align: "right"},
		}, rows, "Availability by workload", note)
}

// probeStatus judges a workload's healthcheck pass rate, and says nothing
// when there is no healthcheck to judge.
func probeStatus(r uptimeRow) Status {
	if r.Probes == 0 {
		return StatusNeutral
	}
	return thresholdStatus(r.ProbePassPercent, 99, 95)
}

// worstUptimeChart ranks the workloads that lost the most availability.
func worstUptimeChart(a AvailabilityFacts) Block {
	if len(a.Worst) == 0 {
		return divider()
	}
	pts := make([]ChartPoint, 0, len(a.Worst))
	for _, w := range a.Worst {
		pts = append(pts, ChartPoint{
			Label: w.Label, Value: w.Value,
			Status: thresholdStatus(w.Value, a.TargetPercent, 99),
		})
	}
	return chartBlock(Chart{
		Kind: ChartHBar, Title: "Lowest availability", Unit: "%", Max: 100, ShowValues: true,
		Height: 140, HasTarget: true, Target: a.TargetPercent,
		TargetLabel: fmtDecimal(a.TargetPercent, 1) + "% target",
		Series:      []ChartSeries{{Points: pts}},
		Caption:     "Workloads that did not reach the availability target, worst first.",
	})
}

// errorBudgetGauge renders what is left of the permitted downtime.
func errorBudgetGauge(a AvailabilityFacts) Block {
	if !a.Assessed {
		return divider()
	}
	remaining := math.Max(0, a.ErrorBudgetPercent)
	status := StatusGood
	switch {
	case a.ErrorBudgetPercent <= 0:
		status = StatusBad
	case a.ErrorBudgetPercent < 50:
		status = StatusWarn
	}
	return chartBlock(Chart{
		Kind: ChartGauge, Title: "Error budget remaining", Unit: "%", Max: 100, Height: 120,
		Series: []ChartSeries{{Points: []ChartPoint{{Label: "Remaining", Value: remaining, Status: status}}}},
		Caption: "A " + fmtDecimal(a.TargetPercent, 1) + "% target permits " +
			fmtDuration(a.ObservedMinutes*(100-a.TargetPercent)/100) + " of downtime over the observed period. " +
			fmtDuration(a.DowntimeMinutes) + " was used.",
	})
}

// riskRegister is the strategic block a board expects: what could go
// wrong, how likely, how bad, and who owns it.
func riskRegister(b *buildCtx) Block {
	f := b.facts
	type risk struct {
		title, impact, likelihood, mitigation, owner string
		sev                                          Severity
	}
	var risks []risk

	if f.Security.Assessed && (f.Security.Critical > 0 || f.Security.High > 0) {
		lk := "Medium"
		if f.Security.Critical > 0 {
			lk = "High"
		}
		risks = append(risks, risk{
			title: "Exploitable vulnerability in a running image", sev: SevCritical,
			impact: "Service compromise, data exposure, notifiable incident", likelihood: lk,
			mitigation: "Rebuild affected images on a patched base; enforce scheduled scanning",
			owner:      "Platform engineering",
		})
	}
	if f.Resilience.Assessed && (f.Resilience.CoveragePercent < 90 || f.Resilience.LastSuccessAge > 7) {
		risks = append(risks, risk{
			title: "Unrecoverable data loss after a host or storage failure", sev: SevHigh,
			impact: "Permanent loss of application state; extended outage", likelihood: "Medium",
			mitigation: "Extend backup plans to uncovered workloads; fix failing runs; test a restore",
			owner:      "Platform engineering",
		})
	}
	if f.Capacity.Assessed && f.Capacity.HeadroomPercent < 25 && f.Capacity.MemTotalBytes > 0 {
		risks = append(risks, risk{
			title: "Capacity exhaustion under load", sev: SevMedium,
			impact: "Workload eviction, degraded response, cascading restarts", likelihood: "Medium",
			mitigation: "Reclaim unused images and volumes; size the next capacity step",
			owner:      "Platform engineering",
		})
	}
	if len(f.Reliability.CrashLooping) > 0 || f.Reliability.Crashes > 0 {
		risks = append(risks, risk{
			title: "Recurring workload instability", sev: SevMedium,
			impact: "Intermittent service failure; erosion of confidence in the platform", likelihood: "Medium",
			mitigation: "Root-cause the top offenders; add health checks where missing",
			owner:      "Service owners",
		})
	}
	if f.Access.Assessed && (f.Access.DormantUsers > 0 || f.Access.APIKeysStale > 0 || f.Access.Admins > 5) {
		risks = append(risks, risk{
			title: "Standing access beyond current need", sev: SevLow,
			impact: "Wider blast radius if a credential is compromised", likelihood: "Low",
			mitigation: "Quarterly access review; revoke dormant accounts and unused keys",
			owner:      "Security",
		})
	}
	if len(risks) == 0 {
		return callout(StatusGood, "Risk register",
			"No material platform risks were identified from the data available this period.")
	}

	rows := make([][]Cell, 0, len(risks))
	for _, r := range risks {
		rows = append(rows, []Cell{
			cell(r.title),
			pill(titleCase(string(r.sev)), severityStatus(r.sev)),
			cell(r.likelihood),
			cell(r.impact),
			cell(r.mitigation),
			cell(r.owner),
		})
	}
	return table(
		[]Column{
			{Label: "Risk", Width: 2.6},
			{Label: "Severity", Width: 1.1},
			{Label: "Likelihood", Width: 1.1},
			{Label: "Impact", Width: 2.6},
			{Label: "Mitigation", Width: 3},
			{Label: "Owner", Width: 1.5},
		}, rows, "Risk register",
		"Severity reflects the consequence if the risk is realised; likelihood is assessed from the evidence in this period.")
}

// --- 1. Executive Platform Overview ----------------------------------

func buildExecutiveOverview(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	// Strategic.
	st := section(Strategic, "Platform posture")
	if b.score != nil {
		st.Blocks = append(st.Blocks,
			para("The platform is assessed across six domains. Each is scored independently from "+
				"the platform's own records; the overall figure is their weighted mean, excluding "+
				"anything that could not be measured."),
			postureChart(b.score),
			postureTable(b.score))
	}
	st.Blocks = append(st.Blocks,
		heading(2, "Risk register"),
		riskRegister(b),
		heading(2, "Outlook for the next quarter"),
		bullets(outlook(b)...))
	out = append(out, st)

	// Tactical.
	ta := section(Tactical, "Performance against targets")
	ta.Blocks = append(ta.Blocks,
		heading(2, "Service and workload"),
		kpis(
			KPI{Label: "Workloads running", Value: fmtInt(f.Containers.Running), Target: fmtInt(f.Containers.Total) + " defined"},
			KPI{Label: "Healthy", Value: fmtInt(f.Containers.Healthy), Status: StatusGood},
			KPI{Label: "Unhealthy", Value: fmtInt(f.Containers.Unhealthy), Status: countStatus(f.Containers.Unhealthy, 0, 0)},
			KPI{Label: "No health check", Value: fmtInt(f.Containers.NoHealthcheck), Status: countStatus(f.Containers.NoHealthcheck, 0, 3),
				Hint: "Running workloads the platform cannot assess"},
		),
		workloadStateChart(f))

	// The counts above are the estate as it stands; this is the estate as it
	// behaved. A board that sees only the first will conclude the platform is
	// fine on the strength of a screenshot taken at a good moment.
	if a := f.Availability; a.Assessed {
		ta.Blocks = append(ta.Blocks,
			heading(2, "Measured availability"),
			kpis(availabilityKPIs(a)...),
			errorBudgetGauge(a),
			availabilityCoverageCallout(a))
	}

	if f.Reliability.Assessed {
		ta.Blocks = append(ta.Blocks,
			heading(2, "Stability"),
			eventTrendChart(f, "Workload events over the period",
				"Restarts, stops and health transitions recorded by the platform."),
			metricTargetTable(b))
	}
	if f.Security.Assessed {
		ta.Blocks = append(ta.Blocks, heading(2, "Security exposure"), severityChart(f), severityTable(f))
	}
	if f.Resilience.Assessed {
		ta.Blocks = append(ta.Blocks, heading(2, "Recovery readiness"), resilienceKPIs(f))
	}
	out = append(out, ta)

	// Operational.
	op := section(Operational, "Operational detail")
	if len(f.Reliability.TopOffenders) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Most active workloads by incident count"), offenderChart(f))
	}
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, heading(2, "Workload inventory"), containerTable(b.snap, 40))
		if len(b.snap.Audit) > 0 {
			op.Blocks = append(op.Blocks, heading(2, "Notable administrative activity"), auditTable(b.snap, 25))
		}
	}
	out = append(out, op)
	return out
}

// metricTargetTable is the tactical staple: a value, the target it is
// measured against, and the variance. Without a target a number is
// trivia.
func metricTargetTable(b *buildCtx) Block {
	f := b.facts
	type row struct {
		metric, value, target string
		status                Status
	}
	rows := []row{
		{"Workloads healthy", fmtPercent(pct(f.Containers.Healthy, maxIntOf(f.Containers.Running, 1))), "100%",
			thresholdStatus(pct(f.Containers.Healthy, maxIntOf(f.Containers.Running, 1)), 95, 80)},
		{"Unexpected stops", fmtInt(f.Reliability.Crashes), "0", countStatus(f.Reliability.Crashes, 0, 5)},
		{"Restarts per day", fmtDecimal(f.Reliability.RestartRatePerDay, 1), "< 1.0",
			invThresholdStatus(f.Reliability.RestartRatePerDay, 1, 5)},
		{"Workloads crash-looping", fmtInt(len(f.Reliability.CrashLooping)), "0",
			countStatus(len(f.Reliability.CrashLooping), 0, 0)},
	}
	if f.Reliability.RecoverySamples > 0 {
		rows = append(rows, row{"Mean recovery time", fmtDuration(f.Reliability.MeanRecoveryMinutes), "< 60 min",
			invThresholdStatus(f.Reliability.MeanRecoveryMinutes, 60, 240)})
	}

	cells := make([][]Cell, 0, len(rows))
	for _, r := range rows {
		verdict := "On target"
		switch r.status {
		case StatusWarn:
			verdict = "Drifting"
		case StatusBad:
			verdict = "Off target"
		}
		cells = append(cells, []Cell{cell(r.metric), cellStatus(r.value, r.status), cell(r.target), pill(verdict, r.status)})
	}
	return table(
		[]Column{
			{Label: "Measure", Width: 3},
			{Label: "This period", Width: 1.4, Align: "right"},
			{Label: "Target", Width: 1.2, Align: "right"},
			{Label: "Verdict", Width: 1.3},
		}, cells, "Stability measures against target", "")
}

func severityChart(f Facts) Block {
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Open findings by severity", Unit: "findings", Height: 130,
		Series: []ChartSeries{{Points: []ChartPoint{
			{Label: "Critical", Value: float64(f.Security.Critical), Status: StatusBad},
			{Label: "High", Value: float64(f.Security.High), Status: StatusWarn},
			{Label: "Medium", Value: float64(f.Security.Medium), Status: StatusInfo},
			{Label: "Low", Value: float64(f.Security.Low), Status: StatusMuted},
		}}},
		Caption: "Findings from the most recent scan of each image.",
	})
}

func severityTable(f Facts) Block {
	return table(
		[]Column{
			{Label: "Severity", Width: 1.4},
			{Label: "Open", Width: 1, Align: "right"},
			{Label: "Weight", Width: 1, Align: "right"},
			{Label: "Expectation", Width: 4},
		},
		[][]Cell{
			{pill("Critical", StatusBad), cellStatus(fmtInt(f.Security.Critical), countStatus(f.Security.Critical, 0, 0)), cell("×10"), cell("Zero. Remediate within the current change window.")},
			{pill("High", StatusWarn), cellStatus(fmtInt(f.Security.High), countStatus(f.Security.High, 0, 5)), cell("×5"), cell("Remediate within 30 days.")},
			{pill("Medium", StatusInfo), cell(fmtInt(f.Security.Medium)), cell("×2"), cell("Address at the next base-image refresh.")},
			{pill("Low", StatusMuted), cell(fmtInt(f.Security.Low)), cell("×0.5"), cell("Accept unless clustered in one image.")},
		},
		"Findings by severity",
		"Weighted exposure this period: "+fmtDecimal(f.Security.WeightedExposure, 0)+
			" across "+plural(f.Security.ImagesScanned, "scanned image", "scanned images")+".")
}

func resilienceKPIs(f Facts) Block {
	return kpis(
		KPI{Label: "Coverage", Value: fmtPercent(f.Resilience.CoveragePercent), Target: "100%",
			Status: thresholdStatus(f.Resilience.CoveragePercent, 90, 60)},
		KPI{Label: "Success rate", Value: fmtPercent(f.Resilience.SuccessRate), Target: "100%",
			Status: thresholdStatus(f.Resilience.SuccessRate, 98, 90)},
		KPI{Label: "Recovery point", Value: fmtDays(f.Resilience.LastSuccessAge), Target: "≤ 24 h",
			Status: freshnessStatus(f.Resilience.LastSuccessAge, 2, 7)},
		KPI{Label: "Destinations", Value: fmtInt(f.Resilience.Destinations),
			Status: countStatus(2-minInt(f.Resilience.Destinations, 2), 0, 1),
			Hint:   "Two or more survive losing one"},
	)
}

func offenderChart(f Facts) Block {
	pts := make([]ChartPoint, 0, len(f.Reliability.TopOffenders))
	for _, o := range f.Reliability.TopOffenders {
		pts = append(pts, ChartPoint{Label: o.Label, Value: float64(o.Count)})
	}
	return chartBlock(Chart{
		Kind: ChartHBar, Title: "Workloads by incident count", ShowValues: true,
		Height:  float64(28*len(pts)) + 10,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "Restarts, unexpected stops and out-of-memory kills attributed to each workload.",
	})
}

func auditTable(snap *Snapshot, limit int) Block {
	list := append([]auditRow(nil), snap.Audit...)
	sort.Slice(list, func(i, j int) bool { return parseTS(list[i].Timestamp).After(parseTS(list[j].Timestamp)) })

	rows := make([][]Cell, 0, limit)
	truncated := false
	for i, a := range list {
		if i >= limit {
			truncated = true
			break
		}
		st := StatusNeutral
		switch strings.ToLower(a.Severity) {
		case "critical", "error":
			st = StatusBad
		case "warning", "warn":
			st = StatusWarn
		}
		user := "—"
		if a.Username != nil && *a.Username != "" {
			user = *a.Username
		}
		entity := "—"
		if a.EntityName != nil && *a.EntityName != "" {
			entity = *a.EntityName
		} else if a.EntityType != nil && *a.EntityType != "" {
			entity = *a.EntityType
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(a.Timestamp))),
			cell(user),
			cell(titleCase(a.Action)),
			cell(entity),
			pill(titleCase(a.Severity), st),
		})
	}
	note := ""
	if truncated {
		note = "Showing the most recent " + fmtInt(limit) + " of " + fmtInt(len(list)) + " audited actions in the period."
	}
	return table(
		[]Column{
			{Label: "When", Width: 2},
			{Label: "Who", Width: 1.5},
			{Label: "Action", Width: 2.5},
			{Label: "Subject", Width: 2.5},
			{Label: "Severity", Width: 1.2},
		}, rows, "Administrative activity", note)
}

// outlook is the forward-looking block. It is derived, not predicted:
// each line follows from something already measured.
func outlook(b *buildCtx) []string {
	f := b.facts
	var out []string

	if f.Security.Assessed && f.Security.Critical+f.Security.High > 0 {
		out = append(out, "Closing the "+fmtInt(f.Security.Critical+f.Security.High)+
			" open critical and high findings is the single largest available improvement to the posture score.")
	}
	if f.Resilience.Assessed && f.Resilience.CoveragePercent < 100 {
		out = append(out, "Extending backup coverage from "+fmtPercent(f.Resilience.CoveragePercent)+
			" toward full coverage of running workloads would materially reduce recovery risk.")
	}
	if f.Capacity.Assessed && f.Capacity.MemTotalBytes > 0 {
		if f.Capacity.HeadroomPercent < 25 {
			out = append(out, "Capacity should be reviewed before further workloads are onboarded; "+
				fmtBytes(f.Images.ReclaimableBytes)+" is reclaimable from unused images immediately.")
		} else {
			out = append(out, "Current headroom of "+fmtPercent(f.Capacity.HeadroomPercent)+
				" supports the workload growth seen this period without additional capacity.")
		}
	}
	if f.Delivery.Assessed && f.Delivery.Deployments > 0 && f.Delivery.ChangeFailureRate > 15 {
		out = append(out, "A change failure rate of "+fmtPercent(f.Delivery.ChangeFailureRate)+
			" suggests changes are reaching the platform without sufficient verification.")
	}
	if f.Containers.NoHealthcheck > 0 {
		out = append(out, plural(f.Containers.NoHealthcheck, "running workload has", "running workloads have")+
			" no health check, so the platform cannot detect their failure — these are blind spots in every figure above.")
	}
	if len(out) == 0 {
		out = append(out, "No structural change is indicated. Maintain current scanning, backup and update cadences.")
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
