// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// Workload reports: one object, answered in depth.
//
// Everything in this file exists because of the health and uptime history.
// Before it, the platform could describe what the estate looked like; these
// reports describe how it behaved, which is the only thing a service owner
// can actually be held to.

// --- 13. Uptime & Availability History -------------------------------

func buildUptimeAvailability(b *buildCtx) []Section {
	a := b.facts.Availability
	var out []Section

	st := section(Strategic, "Availability against target")
	if !a.Assessed {
		st.Blocks = append(st.Blocks,
			availabilityCoverageCallout(a),
			para("This report has nothing to measure. The health and uptime history is written by "+
				"the container health collector, which samples every workload every 30 seconds. If "+
				"this platform was started recently, or the collector has not yet completed a tick, "+
				"come back once a day of history exists."))
		return append(out, st)
	}

	st.Blocks = append(st.Blocks,
		para("Availability is the share of observed time each workload was running, sampled every "+
			"30 seconds and measured against a "+fmtDecimal(a.TargetPercent, 1)+"% target. "+
			"It is a floor on service quality rather than a measure of it: the platform can see "+
			"that a process was up, not that it was serving correct responses quickly."),
		kpis(availabilityKPIs(a)...),
		errorBudgetGauge(a),
		availabilityCoverageCallout(a))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	if items := availabilityFindings(a); len(items) > 0 {
		st.Blocks = append(st.Blocks, heading(2, "Findings"), findings(items...))
	} else {
		st.Blocks = append(st.Blocks, callout(StatusGood, "Nothing requiring attention",
			"Availability met its target, the error budget is intact, and every workload the "+
				"platform observed has a health check that passed."))
	}
	out = append(out, st)

	ta := section(Tactical, "Where availability was lost")
	ta.Blocks = append(ta.Blocks,
		worstUptimeChart(a),
		availabilityTable(a, 25),
		heading(2, "Reliability intervals"),
		keyvalue(
			Pair{K: "Mean time to recover (MTTR)", V: fmtDuration(a.MTTRMinutes) +
				" — average outage length across " + plural(a.Outages, "outage", "outages")},
			Pair{K: "Mean time between failures (MTBF)", V: fmtDuration(a.MTBFHours*60) +
				" — average run of uninterrupted service"},
			Pair{K: "Outage rate", V: fmtDecimal(a.OutageRatePerDay, 2) + " per day across the fleet"},
			Pair{K: "Workloads with any downtime", V: fmtInt(a.WithDowntime) + " of " + fmtInt(a.Tracked)},
			Pair{K: "Observed time", V: fmtDuration(a.ObservedMinutes) +
				" of sampled workload-time over " + plural(a.ObservedDays, "day", "days")},
		),
		para("MTTR and MTBF are computed from run-state transitions, not from incident tickets. "+
			"An outage the platform recovered from automatically counts here exactly as one a "+
			"person resolved, which is the point: the service was unavailable either way."))
	out = append(out, ta)

	if b.opt(OptIncludeDetail) {
		op := section(Operational, "Full availability record")
		op.Blocks = append(op.Blocks, availabilityTable(a, 0),
			para("Every workload the platform observed in the period, including any that has since "+
				"been removed. A workload with a low day count appeared part-way through the "+
				"period and is measured only from when it was first seen."))
		out = append(out, op)
	}
	return out
}

// availabilityFindings turns the measured record into things to act on.
func availabilityFindings(a AvailabilityFacts) []Finding {
	var out []Finding

	if a.UptimePercent < a.TargetPercent {
		sev := SevMedium
		if a.UptimePercent < 99 {
			sev = SevHigh
		}
		if a.UptimePercent < 95 {
			sev = SevCritical
		}
		out = append(out, Finding{
			Severity: sev,
			Title:    "Fleet availability of " + fmtUptime(a.UptimePercent) + " is below the " + fmtDecimal(a.TargetPercent, 1) + "% target",
			Detail: fmtDuration(a.DowntimeMinutes) + " of downtime was observed across " +
				plural(a.WithDowntime, "workload", "workloads") + ".",
			Recommendation: "Start with the lowest-availability workloads in the table below; fleet " +
				"availability is usually dominated by two or three of them.",
			Owner:    "Platform engineering",
			Evidence: plural(a.ObservedDays, "day", "days") + " of sampled history",
		})
	}
	if a.ErrorBudgetPercent <= 0 && a.DowntimeMinutes > 0 {
		out = append(out, Finding{
			Severity:       SevHigh,
			Title:          "The availability error budget is spent",
			Detail:         "Downtime has exceeded what a " + fmtDecimal(a.TargetPercent, 1) + "% target permits for the observed period.",
			Recommendation: "Hold non-essential change until the budget recovers, and prioritise the causes over new work.",
			Owner:          "Platform engineering",
		})
	}
	if a.MTTRMinutes > 60 && a.Outages > 0 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          "Mean time to recover is " + fmtDuration(a.MTTRMinutes),
			Detail:         "Outages are lasting long enough that recovery, not prevention, is the larger lever.",
			Recommendation: "Add automatic restart policies, or an autoheal rule, for the workloads that recover slowest.",
			Owner:          "Service owner",
		})
	}
	if a.Probes > 0 && a.ProbePassPercent < 99 {
		out = append(out, Finding{
			Severity: SevMedium,
			Title:    "Health checks failed " + fmtPercent(100-a.ProbePassPercent) + " of the time",
			Detail: fmtInt(a.ProbesFailed) + " of " + fmtInt(a.Probes) + " probes did not pass. " +
				"A workload can be running and still be failing its own check.",
			Recommendation: "Review the failing checks: either the service is degraded, or the check is wrong.",
			Owner:          "Service owner",
		})
	}
	if a.NoHealthcheck > 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(a.NoHealthcheck, "running workload has", "running workloads have") + " no health check",
			Detail:         "For these, \"running\" means the process exists. Uptime here is optimistic by that margin.",
			Recommendation: "Add a HEALTHCHECK to the image, or a healthcheck block to the compose service.",
			Owner:          "Service owner",
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return severityRank(out[i].Severity) < severityRank(out[j].Severity)
	})
	return out
}

// --- 14. Container Service Report ------------------------------------

func buildContainerService(b *buildCtx) []Section {
	snap := b.snap
	a := b.facts.Availability
	var out []Section

	ta := section(Tactical, "Service overview")
	ta.Blocks = append(ta.Blocks,
		para("One row per workload, drawing every fact the platform holds about it into a single "+
			"line: how available it was, what image it runs, whether its data is backed up, and "+
			"whether anything updates it. This is the view a service owner needs before a "+
			"conversation about a particular container."),
		kpis(
			KPI{Label: "Workloads", Value: fmtInt(b.facts.Containers.Total),
				Hint: fmtInt(b.facts.Containers.Running) + " running"},
			KPI{Label: "Measured", Value: fmtInt(a.Tracked),
				Hint: "with uptime history"},
			KPI{Label: "Backed up", Value: fmtInt(b.facts.Resilience.ContainersCovered),
				Status: thresholdStatus(b.facts.Resilience.CoveragePercent, 90, 60)},
			KPI{Label: "Health checked", Value: fmtInt(a.WithHealthcheck),
				Status: countStatus(a.NoHealthcheck, 0, 3),
				Hint:   fmtInt(a.NoHealthcheck) + " without"},
		),
		containerServiceTable(b))
	if a.Assessed {
		ta.Blocks = append(ta.Blocks, availabilityCoverageCallout(a))
	}
	out = append(out, ta)

	op := section(Operational, "Per-workload detail")
	if a.Assessed {
		op.Blocks = append(op.Blocks, heading(2, "Availability"), availabilityTable(a, 40))
	}
	if len(snap.Activity) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Recent activity"), activityTable(snap, 40))
	}
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, heading(2, "Inventory"), containerTable(snap, 60))
	}
	out = append(out, op)
	return out
}

// containerServiceTable is the cross-cut: availability, image, protection and
// change automation for each workload in one row.
func containerServiceTable(b *buildCtx) Block {
	snap := b.snap
	uptime := uptimeByName(b.facts.Availability)
	backed := backupCoverage(snap)
	updated := updateCoverage(snap)

	list := append([]containerRow(nil), snap.Containers...)
	sort.Slice(list, func(i, j int) bool { return containerName(list[i]) < containerName(list[j]) })

	rows := make([][]Cell, 0, len(list))
	for _, c := range list {
		name := containerName(c)

		up := cell("—")
		if r, ok := uptime[name]; ok {
			up = cellStatus(fmtUptime(r.UptimePercent),
				thresholdStatus(r.UptimePercent, b.facts.Availability.TargetPercent, 99))
		}

		stack := c.StackName
		if stack == "" {
			stack = "—"
		}
		rows = append(rows, []Cell{
			cell(name),
			pill(titleCase(c.State), containerStateStatus(c)),
			up,
			cell(shortImage(c.Image)),
			cell(stack),
			verdictCell(boolStatus(backed[name])),
			verdictCell(boolStatus(updated[name])),
		})
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.4},
			{Label: "State", Width: 1.1},
			{Label: "Uptime", Width: 1, Align: "right"},
			{Label: "Image", Width: 2.8},
			{Label: "Stack", Width: 1.5},
			{Label: "Backed up", Width: 1.1},
			{Label: "Auto-update", Width: 1.2},
		}, rows, "Service record by workload",
		"“Backed up” means at least one enabled backup plan names this workload. “Auto-update” "+
			"means an enabled update policy matches it. Neither says the last run succeeded — see "+
			"the Backup & Recovery and Patch Currency reports for that.")
}

// --- 15. Health Check Quality & Coverage -----------------------------

func buildHealthcheckQuality(b *buildCtx) []Section {
	a := b.facts.Availability
	f := b.facts
	var out []Section

	ta := section(Tactical, "Health check quality")
	ta.Blocks = append(ta.Blocks,
		para("A health check is the platform's only way to tell a working service from a running "+
			"process. This report asks three questions of them: are they present, are they "+
			"passing, and are they fast enough that a failure is noticed before a user notices it."))

	if !a.Assessed {
		ta.Blocks = append(ta.Blocks, availabilityCoverageCallout(a))
		return append(out, ta)
	}

	coverage := pct(a.WithHealthcheck, maxIntOf(a.Tracked, 1))
	ta.Blocks = append(ta.Blocks,
		kpis(
			KPI{Label: "Check coverage", Value: fmtPercent(coverage), Target: "100%",
				Status: thresholdStatus(coverage, 90, 60),
				Hint:   fmtInt(a.WithHealthcheck) + " of " + fmtInt(a.Tracked) + " workloads"},
			KPI{Label: "Probes recorded", Value: fmtInt(a.Probes),
				Hint: "over " + plural(a.ObservedDays, "day", "days")},
			KPI{Label: "Pass rate", Value: fmtUptime(a.ProbePassPercent), Target: "100%",
				Status: thresholdStatus(a.ProbePassPercent, 99, 95)},
			KPI{Label: "Mean probe time", Value: fmtDecimal(a.MeanProbeMs, 0) + " ms",
				Status: invThresholdStatus(a.MeanProbeMs, 1000, 5000),
				Hint:   "slowest " + fmtInt(a.SlowestProbeMs) + " ms"},
		),
		probeQualityTable(a),
		availabilityCoverageCallout(a))

	if f.Containers.NoHealthcheck > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Workloads the platform cannot assess",
			plural(f.Containers.NoHealthcheck, "running workload has", "running workloads have")+
				" no health check. The platform reports them as running because the process exists. "+
				"Every availability figure for them is a statement about the process, not the service."))
	}
	out = append(out, ta)

	op := section(Operational, "Probe detail")
	op.Blocks = append(op.Blocks, missingHealthcheckTable(b))
	out = append(out, op)
	return out
}

// probeQualityTable ranks workloads by how well their health check behaves.
func probeQualityTable(a AvailabilityFacts) Block {
	list := make([]uptimeRow, 0, len(a.Rows))
	for _, r := range a.Rows {
		if r.Probes > 0 {
			list = append(list, r)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ProbePassPercent != list[j].ProbePassPercent {
			return list[i].ProbePassPercent < list[j].ProbePassPercent
		}
		return list[i].MeanProbeMs > list[j].MeanProbeMs
	})

	rows := make([][]Cell, 0, len(list))
	for _, r := range list {
		last := "—"
		if r.LastFailureAt != "" {
			last = fmtDate(parseTS(r.LastFailureAt))
		}
		rows = append(rows, []Cell{
			cell(uptimeLabel(r)),
			cellStatus(fmtUptime(r.ProbePassPercent), thresholdStatus(r.ProbePassPercent, 99, 95)),
			cell(fmtInt(r.Probes)),
			cellStatus(fmtInt(r.ProbesFailed), countStatus(r.ProbesFailed, 0, 3)),
			cellStatus(fmtDecimal(r.MeanProbeMs, 0)+" ms", invThresholdStatus(r.MeanProbeMs, 1000, 5000)),
			cell(fmtInt(r.MaxProbeMs) + " ms"),
			cell(last),
		})
	}
	if len(rows) == 0 {
		return callout(StatusWarn, "No health check probes recorded",
			"No workload in this period ran a health check the platform could observe. Until at "+
				"least one does, the availability figures in every report describe processes rather "+
				"than services.")
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.6},
			{Label: "Pass rate", Width: 1.1, Align: "right"},
			{Label: "Probes", Width: 1, Align: "right"},
			{Label: "Failed", Width: 1, Align: "right"},
			{Label: "Mean time", Width: 1.2, Align: "right"},
			{Label: "Slowest", Width: 1.1, Align: "right"},
			{Label: "Last failure", Width: 1.4},
		}, rows, "Health check behaviour by workload",
		"Ordered by pass rate, then by how slow the check is. A slow check is a latent problem: "+
			"if it approaches its own timeout, an otherwise healthy service starts being marked "+
			"unhealthy under load.")
}

// missingHealthcheckTable names the workloads with nothing to measure.
func missingHealthcheckTable(b *buildCtx) Block {
	measured := map[string]bool{}
	for _, r := range b.facts.Availability.Rows {
		if r.Probes > 0 && r.ContainerName != "" {
			measured[r.ContainerName] = true
		}
	}

	var rows [][]Cell
	for _, c := range b.snap.Containers {
		if !strings.EqualFold(c.State, "running") {
			continue
		}
		name := containerName(c)
		if measured[name] {
			continue
		}
		stack := c.StackName
		if stack == "" {
			stack = "—"
		}
		rows = append(rows, []Cell{cell(name), cell(shortImage(c.Image)), cell(stack), cell(c.Status)})
	}
	if len(rows) == 0 {
		return callout(StatusGood, "Every running workload is health checked",
			"No running workload lacks an observable health check.")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0].Text < rows[j][0].Text })
	return table(
		[]Column{
			{Label: "Workload", Width: 2.4},
			{Label: "Image", Width: 3},
			{Label: "Stack", Width: 1.6},
			{Label: "Status", Width: 2.2},
		}, rows, "Running workloads with no observable health check",
		"Add a HEALTHCHECK instruction to the image, or a healthcheck block to the compose "+
			"service. Until then the platform cannot distinguish a working service from a hung one.")
}

// --- 16. Stability & Flapping Analysis -------------------------------

func buildStabilityFlapping(b *buildCtx) []Section {
	a := b.facts.Availability
	f := b.facts
	var out []Section

	ta := section(Tactical, "Stability")
	ta.Blocks = append(ta.Blocks,
		para("A workload that flaps — moving repeatedly between running and stopped — can show "+
			"good availability and still be failing. Total downtime hides it; the count of state "+
			"transitions does not. This report separates the two so a workload that is briefly "+
			"down forty times is not filed alongside one that was down once for the same total."))

	if !a.Assessed {
		ta.Blocks = append(ta.Blocks, availabilityCoverageCallout(a))
		return append(out, ta)
	}

	ta.Blocks = append(ta.Blocks,
		kpis(
			KPI{Label: "State transitions", Value: fmtInt(a.Transitions),
				Hint: "across " + plural(a.Tracked, "workload", "workloads")},
			KPI{Label: "Outages", Value: fmtInt(a.Outages),
				Status: countStatus(a.Outages, 0, 5)},
			KPI{Label: "Mean time between failures", Value: fmtDuration(a.MTBFHours * 60),
				Status: thresholdStatus(a.MTBFHours, 168, 24)},
			KPI{Label: "Unstable workloads", Value: fmtInt(len(a.Flappiest)),
				Status: countStatus(len(a.Flappiest), 0, 3),
				Hint:   "changed state at least once"},
		),
		flappingChart(a),
		flappingTable(a),
		availabilityCoverageCallout(a))

	if len(f.Reliability.CrashLooping) > 0 {
		names := make([]string, 0, len(f.Reliability.CrashLooping))
		for _, c := range f.Reliability.CrashLooping {
			names = append(names, c.Label)
		}
		ta.Blocks = append(ta.Blocks, callout(StatusBad, "Crash-looping now",
			joinAnd(names)+" — restarting repeatedly without staying up. A crash loop is the "+
				"extreme of flapping: the workload consumes scheduling capacity continuously and "+
				"serves nothing at any point."))
	}
	out = append(out, ta)

	op := section(Operational, "Event detail")
	op.Blocks = append(op.Blocks,
		eventTrendChart(f, "Workload events per day",
			"Restarts, stops and health transitions recorded by the platform."))
	if len(b.snap.Activity) > 0 {
		op.Blocks = append(op.Blocks, activityTable(b.snap, 50))
	}
	out = append(out, op)
	return out
}

func flappingChart(a AvailabilityFacts) Block {
	if len(a.Flappiest) == 0 {
		return callout(StatusGood, "No instability observed",
			"No workload changed run state during the observed period.")
	}
	pts := make([]ChartPoint, 0, len(a.Flappiest))
	for _, w := range a.Flappiest {
		pts = append(pts, ChartPoint{Label: w.Label, Value: w.Value,
			Status: invThresholdStatus(w.Value, 0.2, 1)})
	}
	return chartBlock(Chart{
		Kind: ChartHBar, Title: "State changes per day", ShowValues: true, Height: 140,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "Higher is worse. A workload above one change per day is not stable, whatever its uptime percentage says.",
	})
}

func flappingTable(a AvailabilityFacts) Block {
	list := make([]uptimeRow, 0, len(a.Rows))
	for _, r := range a.Rows {
		if r.Transitions > 0 {
			list = append(list, r)
		}
	}
	if len(list) == 0 {
		return divider()
	}
	sort.Slice(list, func(i, j int) bool { return list[i].FlapsPerDay > list[j].FlapsPerDay })

	rows := make([][]Cell, 0, len(list))
	for _, r := range list {
		// The verdict is the point of this table: transitions and downtime
		// together say something neither says alone.
		verdict, st := stabilityVerdict(r)
		rows = append(rows, []Cell{
			cell(uptimeLabel(r)),
			cellStatus(fmtInt(r.Transitions), invThresholdStatus(float64(r.Transitions), 2, 10)),
			cell(fmtDecimal(r.FlapsPerDay, 2)),
			cell(fmtInt(r.Outages)),
			cell(fmtDuration(r.DowntimeMinutes)),
			cell(fmtUptime(r.UptimePercent)),
			pill(verdict, st),
		})
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.4},
			{Label: "Transitions", Width: 1.2, Align: "right"},
			{Label: "Per day", Width: 1, Align: "right"},
			{Label: "Outages", Width: 1, Align: "right"},
			{Label: "Downtime", Width: 1.2, Align: "right"},
			{Label: "Uptime", Width: 1, Align: "right"},
			{Label: "Verdict", Width: 1.4},
		}, rows, "Instability by workload",
		"“Flapping” is many short transitions; “unstable” is fewer but longer ones. The first is "+
			"usually a failing health check or a dependency that is not ready; the second is "+
			"usually the workload itself.")
}

// stabilityVerdict classifies a workload from the shape of its downtime, not
// its total. Forty one-minute outages and one forty-minute outage have very
// different causes and want very different first questions.
func stabilityVerdict(r uptimeRow) (string, Status) {
	switch {
	case r.Transitions == 0:
		return "Stable", StatusGood
	case r.FlapsPerDay >= 2:
		return "Flapping", StatusBad
	case r.Outages > 0 && r.MTTRMinutes >= 60:
		return "Long outages", StatusBad
	case r.FlapsPerDay >= 0.5:
		return "Unstable", StatusWarn
	default:
		return "Occasional", StatusWarn
	}
}

// --- 17. Stack & Application Portfolio -------------------------------

func buildStackPortfolio(b *buildCtx) []Section {
	snap := b.snap
	var out []Section

	ta := section(Tactical, "Application portfolio")
	ta.Blocks = append(ta.Blocks,
		para("Workloads are managed one at a time and owned in groups. This report rolls the "+
			"per-container record up to the stack, which is the unit a team is usually "+
			"responsible for and the unit a conversation about ownership actually uses."),
		kpis(
			KPI{Label: "Stacks", Value: fmtInt(len(snap.Stacks))},
			KPI{Label: "Workloads in stacks", Value: fmtInt(b.facts.Containers.Total - b.facts.Containers.Unmanaged)},
			KPI{Label: "Unmanaged workloads", Value: fmtInt(b.facts.Containers.Unmanaged),
				Status: countStatus(b.facts.Containers.Unmanaged, 0, 5),
				Hint:   "not part of any stack"},
			KPI{Label: "Measured", Value: fmtInt(b.facts.Availability.Tracked),
				Hint: "with uptime history"},
		),
		stackPortfolioTable(b))
	if b.facts.Containers.Unmanaged > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Workloads outside any stack",
			plural(b.facts.Containers.Unmanaged, "workload is", "workloads are")+" not part of a "+
				"stack. These were created directly rather than from a compose file, so their "+
				"configuration exists only on the host: they cannot be reviewed, versioned or "+
				"recreated from source."))
	}
	out = append(out, ta)

	op := section(Operational, "Stack detail")
	if len(snap.Stacks) > 0 {
		op.Blocks = append(op.Blocks, stackTable(snap))
	}
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, containerTable(snap, 60))
	}
	out = append(out, op)
	return out
}

// stackAgg accumulates one stack's workloads.
type stackAgg struct {
	name                      string
	total, running, unhealthy int
	uptimeSum                 float64
	uptimeCount               int
	downtimeMinutes           float64
	outages                   int
}

func stackPortfolioTable(b *buildCtx) Block {
	uptime := uptimeByName(b.facts.Availability)
	byStack := map[string]*stackAgg{}

	for _, c := range b.snap.Containers {
		key := c.StackName
		if key == "" {
			key = "(unmanaged)"
		}
		agg, ok := byStack[key]
		if !ok {
			agg = &stackAgg{name: key}
			byStack[key] = agg
		}
		agg.total++
		if strings.EqualFold(c.State, "running") {
			agg.running++
		}
		if strings.Contains(strings.ToLower(c.Status), "(unhealthy)") {
			agg.unhealthy++
		}
		if r, ok := uptime[containerName(c)]; ok {
			agg.uptimeSum += r.UptimePercent
			agg.uptimeCount++
			agg.downtimeMinutes += r.DowntimeMinutes
			agg.outages += r.Outages
		}
	}

	list := make([]*stackAgg, 0, len(byStack))
	for _, agg := range byStack {
		list = append(list, agg)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })

	target := b.facts.Availability.TargetPercent
	rows := make([][]Cell, 0, len(list))
	for _, agg := range list {
		up := cell("—")
		if agg.uptimeCount > 0 {
			mean := agg.uptimeSum / float64(agg.uptimeCount)
			up = cellStatus(fmtUptime(mean), thresholdStatus(mean, target, 99))
		}
		rows = append(rows, []Cell{
			cell(agg.name),
			cell(fmtInt(agg.total)),
			cellStatus(fmtInt(agg.running), boolStatus(agg.running == agg.total)),
			cellStatus(fmtInt(agg.unhealthy), countStatus(agg.unhealthy, 0, 0)),
			up,
			cell(fmtDuration(agg.downtimeMinutes)),
			cellStatus(fmtInt(agg.outages), countStatus(agg.outages, 0, 3)),
		})
	}
	return table(
		[]Column{
			{Label: "Stack", Width: 2.4},
			{Label: "Workloads", Width: 1.1, Align: "right"},
			{Label: "Running", Width: 1, Align: "right"},
			{Label: "Unhealthy", Width: 1.1, Align: "right"},
			{Label: "Mean uptime", Width: 1.3, Align: "right"},
			{Label: "Downtime", Width: 1.2, Align: "right"},
			{Label: "Outages", Width: 1, Align: "right"},
		}, rows, "Portfolio by stack",
		"Mean uptime is the unweighted average across the stack's workloads, so a stack is not "+
			"flattered by having many healthy sidecars around one failing service.")
}

func stackTable(snap *Snapshot) Block {
	list := append([]stackRow(nil), snap.Stacks...)
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	rows := make([][]Cell, 0, len(list))
	for _, s := range list {
		st := StatusGood
		if !strings.EqualFold(s.Status, "running") && !strings.EqualFold(s.Status, "active") {
			st = StatusWarn
		}
		rows = append(rows, []Cell{
			cell(s.Name),
			pill(titleCase(s.Status), st),
			cell(fmtInt(s.Services)),
			cell(fmtInt(s.Containers)),
			cell(orDash(titleCase(s.Source), "—")),
			cell(orDash(fmtDate(parseTS(s.UpdatedAt)), "—")),
		})
	}
	return table(
		[]Column{
			{Label: "Stack", Width: 2.4},
			{Label: "Status", Width: 1.2},
			{Label: "Services", Width: 1, Align: "right"},
			{Label: "Containers", Width: 1.2, Align: "right"},
			{Label: "Source", Width: 1.4},
			{Label: "Updated", Width: 1.4},
		}, rows, "Stack inventory", "")
}

// --- 18. Operational Readiness Scorecard -----------------------------

func buildOperationalReadiness(b *buildCtx) []Section {
	var out []Section

	scores := readinessScores(b)

	st := section(Strategic, "Readiness")
	st.Blocks = append(st.Blocks,
		para("Readiness is not the same as health. A workload can be running perfectly and still "+
			"be unready: no health check, so nobody would know it had stopped working; no backup, "+
			"so it could not be restored; no update policy, so it will drift. This report scores "+
			"each workload against five basics that are cheap to fix and expensive to be without."),
		kpis(readinessKPIs(scores)...),
		readinessChart(scores),
		callout(StatusInfo, "How this is scored",
			"Each workload scores one point for each of: an observable health check, measured "+
				"availability at or above target, an enabled backup plan, a matching update policy, "+
				"and a scanned image. A workload the platform has no history for cannot score the "+
				"availability point and is marked accordingly rather than failed."))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Readiness by workload")
	ta.Blocks = append(ta.Blocks, readinessTable(scores))
	out = append(out, ta)

	op := section(Operational, "What to fix first")
	op.Blocks = append(op.Blocks, readinessActions(scores))
	out = append(out, op)
	return out
}

// readiness is one workload's checklist result.
type readiness struct {
	name                                             string
	healthcheck, availability, backup, updates, scan bool
	measured                                         bool
	score                                            int
}

func readinessScores(b *buildCtx) []readiness {
	uptime := uptimeByName(b.facts.Availability)
	backed := backupCoverage(b.snap)
	updated := updateCoverage(b.snap)
	scanned := scanCoverage(b.snap)
	target := b.facts.Availability.TargetPercent

	out := make([]readiness, 0, len(b.snap.Containers))
	for _, c := range b.snap.Containers {
		name := containerName(c)
		r := readiness{name: name, backup: backed[name], updates: updated[name]}
		r.scan = scanned[shortImage(c.Image)]

		if u, ok := uptime[name]; ok {
			r.measured = true
			r.healthcheck = u.Probes > 0
			r.availability = u.UptimePercent >= target
		}
		for _, ok := range []bool{r.healthcheck, r.availability, r.backup, r.updates, r.scan} {
			if ok {
				r.score++
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].name < out[j].name
	})
	return out
}

func readinessKPIs(scores []readiness) []KPI {
	var hc, av, bu, up, sc int
	for _, r := range scores {
		if r.healthcheck {
			hc++
		}
		if r.availability {
			av++
		}
		if r.backup {
			bu++
		}
		if r.updates {
			up++
		}
		if r.scan {
			sc++
		}
	}
	total := maxIntOf(len(scores), 1)
	return []KPI{
		{Label: "Health checked", Value: fmtPercent(pct(hc, total)), Target: "100%",
			Status: thresholdStatus(pct(hc, total), 90, 60)},
		{Label: "Meeting availability", Value: fmtPercent(pct(av, total)), Target: "100%",
			Status: thresholdStatus(pct(av, total), 90, 70)},
		{Label: "Backed up", Value: fmtPercent(pct(bu, total)), Target: "100%",
			Status: thresholdStatus(pct(bu, total), 90, 60)},
		{Label: "Update policy", Value: fmtPercent(pct(up, total)), Target: "100%",
			Status: thresholdStatus(pct(up, total), 80, 40)},
		{Label: "Image scanned", Value: fmtPercent(pct(sc, total)), Target: "100%",
			Status: thresholdStatus(pct(sc, total), 90, 60)},
	}
}

func readinessChart(scores []readiness) Block {
	buckets := make([]int, 6)
	for _, r := range scores {
		buckets[clampInt(r.score, 0, 5)]++
	}
	pts := make([]ChartPoint, 0, 6)
	statuses := []Status{StatusBad, StatusBad, StatusWarn, StatusWarn, StatusGood, StatusGood}
	for i, n := range buckets {
		if n == 0 {
			continue
		}
		pts = append(pts, ChartPoint{Label: fmtInt(i) + " of 5", Value: float64(n), Status: statuses[i]})
	}
	if len(pts) == 0 {
		return divider()
	}
	return chartBlock(Chart{
		Kind: ChartBar, Title: "Workloads by readiness score", Unit: "workloads",
		ShowValues: true, Height: 130,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "How many workloads meet how many of the five basics.",
	})
}

func readinessTable(scores []readiness) Block {
	rows := make([][]Cell, 0, len(scores))
	for _, r := range scores {
		avail := verdictCell(boolStatus(r.availability))
		if !r.measured {
			avail = pill("No history", StatusMuted)
		}
		rows = append(rows, []Cell{
			cell(r.name),
			cellStatus(fmtInt(r.score)+" / 5", readinessStatus(r.score)),
			verdictCell(boolStatus(r.healthcheck)),
			avail,
			verdictCell(boolStatus(r.backup)),
			verdictCell(boolStatus(r.updates)),
			verdictCell(boolStatus(r.scan)),
		})
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.6},
			{Label: "Score", Width: 1, Align: "right"},
			{Label: "Health check", Width: 1.3},
			{Label: "Availability", Width: 1.3},
			{Label: "Backup", Width: 1.1},
			{Label: "Updates", Width: 1.1},
			{Label: "Scanned", Width: 1.1},
		}, rows, "Readiness by workload",
		"Ordered worst first. A workload marked “no history” has not been observed long enough "+
			"to judge its availability; that is a gap in the evidence, not a failure of the workload.")
}

func readinessStatus(score int) Status {
	switch {
	case score >= 4:
		return StatusGood
	case score >= 2:
		return StatusWarn
	default:
		return StatusBad
	}
}

// readinessActions turns the scorecard into the shortest list of fixes that
// moves the most workloads. Naming the gap with the largest population first
// is deliberate: it is almost always one missing policy rather than fifty
// individual problems.
func readinessActions(scores []readiness) Block {
	type gap struct {
		label, action string
		count         int
	}
	gaps := []gap{
		{label: "no health check", action: "Add a HEALTHCHECK to the image or a healthcheck block to the compose service."},
		{label: "below the availability target", action: "Review the outage record for these workloads before adding anything new."},
		{label: "no backup plan", action: "Create a backup plan, or record a decision that the workload is disposable."},
		{label: "no update policy", action: "Add an update policy so the image does not drift silently."},
		{label: "an unscanned image", action: "Enable scheduled scanning so every image in use is assessed."},
	}
	for _, r := range scores {
		for i, ok := range []bool{r.healthcheck, r.availability, r.backup, r.updates, r.scan} {
			if !ok {
				gaps[i].count++
			}
		}
	}
	sort.SliceStable(gaps, func(i, j int) bool { return gaps[i].count > gaps[j].count })

	var items []string
	for _, g := range gaps {
		if g.count == 0 {
			continue
		}
		items = append(items, plural(g.count, "workload has "+g.label, "workloads have "+g.label)+
			". "+g.action)
	}
	if len(items) == 0 {
		return callout(StatusGood, "Every workload meets the basics",
			"All five readiness checks pass for every workload in scope.")
	}
	return numbered(items...)
}

// --- shared lookups ---------------------------------------------------

// uptimeByName indexes the measured history by container name.
//
// Name rather than id: every other host endpoint identifies a container by
// name, and matching on the 64-character id would fail for any workload that
// was recreated during the period — which is precisely the population these
// reports are about.
func uptimeByName(a AvailabilityFacts) map[string]uptimeRow {
	out := make(map[string]uptimeRow, len(a.Rows))
	for _, r := range a.Rows {
		if r.ContainerName == "" {
			continue
		}
		// Keep the row with the most samples when a name was reused: the
		// short-lived one is almost always a failed recreate.
		if prev, ok := out[r.ContainerName]; ok && prev.Samples >= r.Samples {
			continue
		}
		out[r.ContainerName] = r
	}
	return out
}

// backupCoverage maps container name -> has at least one enabled plan.
func backupCoverage(snap *Snapshot) map[string]bool {
	out := map[string]bool{}
	for _, p := range snap.BackupPlans {
		if p.Enabled && p.ContainerName != "" {
			out[p.ContainerName] = true
		}
	}
	return out
}

// updateCoverage maps container name -> matched by an enabled update policy.
//
// The match is the policy's own container pattern, treated as a prefix or a
// substring rather than a glob: the host stores an operator-written pattern
// and re-implementing its matcher here would drift from it. A false positive
// reads as "covered" and is the safer error in a report that is used to find
// what is NOT covered — it under-reports the gap rather than inventing one.
func updateCoverage(snap *Snapshot) map[string]bool {
	out := map[string]bool{}
	for _, c := range snap.Containers {
		name := containerName(c)
		for _, p := range snap.Updates {
			if !p.Enabled {
				continue
			}
			if matchesPattern(p.ContainerName, name) || matchesPattern(p.ImageMatch, c.Image) {
				out[name] = true
				break
			}
		}
	}
	return out
}

// scanCoverage maps short image ref -> has been scanned.
func scanCoverage(snap *Snapshot) map[string]bool {
	out := map[string]bool{}
	for _, s := range snap.Scans {
		if s.ImageRef != "" {
			out[shortImage(s.ImageRef)] = true
		}
	}
	return out
}

// matchesPattern is a deliberately forgiving comparison. An empty pattern
// matches nothing rather than everything: a policy with no target is a
// misconfiguration, and treating it as universal would report the whole
// estate as covered by it.
func matchesPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(strings.Trim(pattern, "*"))
	if pattern == "" || value == "" {
		return false
	}
	return strings.Contains(strings.ToLower(value), strings.ToLower(pattern))
}

func boolStatus(ok bool) Status {
	if ok {
		return StatusGood
	}
	return StatusBad
}

func containerStateStatus(c containerRow) Status {
	switch {
	case strings.Contains(strings.ToLower(c.Status), "(unhealthy)"):
		return StatusBad
	case strings.EqualFold(c.State, "running"):
		return StatusGood
	case strings.EqualFold(c.State, "restarting"):
		return StatusWarn
	default:
		return StatusNeutral
	}
}
