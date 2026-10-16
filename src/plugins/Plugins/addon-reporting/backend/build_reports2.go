// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// --- 2. Security & Compliance ----------------------------------------

func buildSecurityCompliance(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	st := section(Strategic, "Security posture")
	if !f.Security.Assessed {
		st.Blocks = append(st.Blocks, callout(StatusWarn, "Not assessed",
			"Vulnerability scanning did not respond ("+b.snap.note(srcScans)+"), so exposure "+
				"cannot be reported for this period. The absence of findings below is an absence "+
				"of evidence, not an absence of risk."))
	} else {
		st.Blocks = append(st.Blocks,
			para("Exposure is measured as severity-weighted findings across the images in use, "+
				"normalised by estate size, together with how much of the estate has been "+
				"assessed at all and how recently."),
			kpis(
				KPI{Label: "Weighted exposure", Value: fmtDecimal(f.Security.WeightedExposure, 0),
					Hint: "critical×10 + high×5 + medium×2"},
				KPI{Label: "Critical", Value: fmtInt(f.Security.Critical), Target: "0",
					Status: countStatus(f.Security.Critical, 0, 0)},
				KPI{Label: "Coverage", Value: fmtPercent(f.Security.CoveragePercent), Target: "100%",
					Status: thresholdStatus(f.Security.CoveragePercent, 90, 60)},
				KPI{Label: "Most recent scan", Value: fmtDays(f.Security.LastScanAge),
					Status: freshnessStatus(f.Security.LastScanAge, 7, 30)},
			),
			severityChart(f))
	}
	st.Blocks = append(st.Blocks, heading(2, "Risk register"), riskRegister(b))
	out = append(out, st)

	ta := section(Tactical, "Controls and exposure")
	if f.Security.Assessed {
		ta.Blocks = append(ta.Blocks, heading(2, "Findings against remediation expectations"), severityTable(f))
		if len(f.Security.TopVulnerableImages) > 0 {
			pts := make([]ChartPoint, 0, len(f.Security.TopVulnerableImages))
			for _, im := range f.Security.TopVulnerableImages {
				pts = append(pts, ChartPoint{Label: im.Label, Value: im.Value})
			}
			ta.Blocks = append(ta.Blocks,
				heading(2, "Where the exposure is concentrated"),
				chartBlock(Chart{
					Kind: ChartHBar, Title: "Images by weighted exposure", ShowValues: true,
					Height:  float64(28*len(pts)) + 10,
					Series:  []ChartSeries{{Points: pts}},
					Caption: "Remediating the top image typically closes a disproportionate share of total exposure.",
				}))
		}
	}
	ta.Blocks = append(ta.Blocks, heading(2, "Access controls"), accessControlTable(b))
	if f.Security.AuditEvents > 0 {
		ta.Blocks = append(ta.Blocks,
			heading(2, "Audit and accountability"),
			kpis(
				KPI{Label: "Audited actions", Value: fmtInt(f.Security.AuditEvents)},
				KPI{Label: "Privileged changes", Value: fmtInt(f.Security.PrivilegedActions),
					Hint: "Role, permission, user, key and settings changes"},
				KPI{Label: "Failed sign-ins", Value: fmtInt(f.Security.FailedLogins),
					Status: countStatus(f.Security.FailedLogins, 0, 10)},
				KPI{Label: "Critical audit events", Value: fmtInt(f.Security.AuditCritical),
					Status: countStatus(f.Security.AuditCritical, 0, 0)},
			))
	}
	out = append(out, ta)

	op := section(Operational, "Detail")
	if b.opt(OptIncludeDetail) {
		if len(b.snap.Scans) > 0 {
			op.Blocks = append(op.Blocks, heading(2, "Scan results"), scanTable(b.snap, 30))
		}
		if len(b.snap.Audit) > 0 {
			op.Blocks = append(op.Blocks, heading(2, "Audit log"), auditTable(b.snap, 30))
		}
	}
	out = append(out, op)
	return out
}

func accessControlTable(b *buildCtx) Block {
	f := b.facts.Access
	if !f.Assessed {
		return callout(StatusInfo, "Not assessed",
			"User and role data could not be read, so access controls are not covered in this report.")
	}
	rows := [][]Cell{
		{cell("User accounts"), cell(fmtInt(f.Users)), cell("—"), cell(fmtInt(f.ActiveUsers) + " active, " + fmtInt(f.InactiveUsers) + " disabled")},
		{cell("Administrators"), cellStatus(fmtInt(f.Admins), countStatus(f.Admins, 3, 6)), cell("≤ 3"), cell("Least privilege: administrator rights should be exceptional")},
		{cell("Dormant accounts (90 days)"), cellStatus(fmtInt(f.DormantUsers), countStatus(f.DormantUsers, 0, 3)), cell("0"), cell("Accounts that retain permissions but are not in use")},
		{cell("Never signed in"), cellStatus(fmtInt(f.NeverLoggedIn), countStatus(f.NeverLoggedIn, 0, 2)), cell("0"), cell("Provisioned but unused")},
		{cell("Custom roles"), cell(fmtInt(f.CustomRoles)), cell("—"), cell(fmtInt(f.Roles) + " roles in total")},
		{cell("Groups"), cell(fmtInt(f.Groups)), cell("—"), cell("Group-based assignment scales better than per-user grants")},
		{cell("API keys"), cellStatus(fmtInt(f.APIKeys), countStatus(f.APIKeysStale, 0, 2)), cell("—"), cell(fmtInt(f.APIKeysActive) + " active, " + fmtInt(f.APIKeysStale) + " unused for 90 days, " + fmtInt(f.APIKeysExpired) + " expired")},
		{cell("External identity providers"), cell(fmtInt(f.IdentityEnabled)), cell("—"), cell("Central identity reduces standing local credentials")},
	}
	return table(
		[]Column{
			{Label: "Control", Width: 2.4},
			{Label: "Count", Width: 1, Align: "right"},
			{Label: "Expectation", Width: 1.2, Align: "right"},
			{Label: "Note", Width: 5},
		}, rows, "Access controls", "")
}

func scanTable(snap *Snapshot, limit int) Block {
	list := append([]scanRow(nil), snap.Scans...)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Critical != list[j].Critical {
			return list[i].Critical > list[j].Critical
		}
		return list[i].High > list[j].High
	})
	rows := make([][]Cell, 0, limit)
	truncated := false
	for i, s := range list {
		if i >= limit {
			truncated = true
			break
		}
		rows = append(rows, []Cell{
			cell(shortImage(s.ImageRef)),
			cell(s.Scanner),
			cellStatus(fmtInt(s.Critical), countStatus(s.Critical, 0, 0)),
			cellStatus(fmtInt(s.High), countStatus(s.High, 0, 5)),
			cell(fmtInt(s.Medium)),
			cell(fmtInt(s.Low)),
			cell(fmtDate(firstTS(s.CompletedAt, s.CreatedAt))),
		})
	}
	note := ""
	if truncated {
		note = "Showing the " + fmtInt(limit) + " images with the highest severity counts, of " + fmtInt(len(list)) + " scans."
	}
	return table(
		[]Column{
			{Label: "Image", Width: 3.4},
			{Label: "Scanner", Width: 1.2},
			{Label: "Crit", Width: 0.7, Align: "right"},
			{Label: "High", Width: 0.7, Align: "right"},
			{Label: "Med", Width: 0.7, Align: "right"},
			{Label: "Low", Width: 0.7, Align: "right"},
			{Label: "Scanned", Width: 1.4},
		}, rows, "Scan results by image", note)
}

// --- 3. Incident & Problem Review ------------------------------------

func buildIncidentProblem(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	st := section(Strategic, "Service reliability")
	st.Blocks = append(st.Blocks,
		para("This section reports what went wrong in the period, how quickly it was resolved, "+
			"and whether the same things keep going wrong. Repeat failures are the ones worth "+
			"a decision; one-off failures rarely are."),
		kpis(
			KPI{Label: "Unexpected stops", Value: fmtInt(f.Reliability.Crashes), Target: "0",
				Status: countStatus(f.Reliability.Crashes, 0, 5)},
			KPI{Label: "Restarts", Value: fmtInt(f.Reliability.Restarts)},
			KPI{Label: "Days with incidents", Value: fmtInt(f.Reliability.IncidentDays),
				Hint: "of " + fmtDecimal(b.snap.Period.Days, 0) + " in the period"},
			KPI{Label: "Mean recovery", Value: fmtDuration(f.Reliability.MeanRecoveryMinutes), Target: "< 60 min",
				Status: invThresholdStatus(f.Reliability.MeanRecoveryMinutes, 60, 240)},
		),
		eventTrendChart(f, "Incidents over the period", "Each point counts restarts, stops and health transitions."))

	// Measured downtime turns an event count into a service impact. "Fourteen
	// restarts" is a number an operator argues with; "fourteen restarts, four
	// hours of downtime, one workload carrying most of it" is a finding.
	if a := f.Availability; a.Assessed {
		st.Blocks = append(st.Blocks,
			heading(2, "Measured impact"),
			kpis(
				KPI{Label: "Observed downtime", Value: fmtDuration(a.DowntimeMinutes),
					Hint: plural(a.WithDowntime, "workload affected", "workloads affected")},
				KPI{Label: "Outages", Value: fmtInt(a.Outages),
					Status: countStatus(a.Outages, 0, 5), Hint: fmtDecimal(a.OutageRatePerDay, 2) + " per day"},
				KPI{Label: "Mean time to recover", Value: fmtDuration(a.MTTRMinutes),
					Target: "< 30 min", Status: invThresholdStatus(a.MTTRMinutes, 30, 120)},
				KPI{Label: "Mean time between failures", Value: fmtDuration(a.MTBFHours * 60),
					Status: thresholdStatus(a.MTBFHours, 168, 24)},
			),
			para("Downtime is counted from the platform's own 30-second sampling, so it is a "+
				"lower bound: an outage shorter than one sample can fall between two observations. "+
				"The event counts above and the downtime here are independent measurements of the "+
				"same period and should agree in direction, not in magnitude."),
			worstUptimeChart(a))
	}

	st.Blocks = append(st.Blocks, heading(2, "Risk register"), riskRegister(b))
	out = append(out, st)

	ta := section(Tactical, "Problem management")
	ta.Blocks = append(ta.Blocks, metricTargetTable(b))
	if a := f.Availability; a.Assessed && len(a.Rows) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Availability by workload"), availabilityTable(a, 20))
	}
	if len(f.Reliability.TopOffenders) > 0 {
		ta.Blocks = append(ta.Blocks,
			heading(2, "Recurring offenders"),
			offenderChart(f),
			para("A workload appearing here repeatedly is a problem, not an incident: it will keep "+
				"consuming attention until its cause is addressed."))
	}
	if len(f.Reliability.CrashLooping) > 0 {
		names := make([]string, 0, len(f.Reliability.CrashLooping))
		for _, c := range f.Reliability.CrashLooping {
			names = append(names, c.Label)
		}
		ta.Blocks = append(ta.Blocks, callout(StatusBad, "Crash-looping now",
			joinAnd(names)+" — restarting repeatedly without staying up. These are consuming "+
				"capacity and serving nothing."))
	}
	if len(b.snap.Alerts) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Alerting coverage"), alertTable(b.snap))
	}
	out = append(out, ta)

	op := section(Operational, "Event detail")
	if b.opt(OptIncludeDetail) && len(b.snap.Activity) > 0 {
		op.Blocks = append(op.Blocks, activityTable(b.snap, 40))
	}
	out = append(out, op)
	return out
}

func alertTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Alerts))
	for _, a := range snap.Alerts {
		st := StatusGood
		label := "Enabled"
		if !a.Enabled {
			st, label = StatusWarn, "Disabled"
		}
		rows = append(rows, []Cell{
			cell(a.Name), cell(titleCase(a.Type)), cell(a.Condition),
			cell(a.Target), pill(label, st),
		})
	}
	return table(
		[]Column{
			{Label: "Rule", Width: 2.4},
			{Label: "Type", Width: 1.4},
			{Label: "Condition", Width: 2.4},
			{Label: "Target", Width: 2},
			{Label: "State", Width: 1.2},
		}, rows, "Alert rules",
		"A disabled rule is a blind spot: the condition it watches will not be noticed.")
}

func activityTable(snap *Snapshot, limit int) Block {
	list := append([]activityRow(nil), snap.Activity...)
	sort.Slice(list, func(i, j int) bool { return parseTS(list[i].Timestamp).After(parseTS(list[j].Timestamp)) })
	rows := make([][]Cell, 0, limit)
	truncated := false
	for i, e := range list {
		if i >= limit {
			truncated = true
			break
		}
		st := StatusNeutral
		a := strings.ToLower(e.Action)
		switch {
		case strings.Contains(a, "die") || strings.Contains(a, "kill") || strings.Contains(a, "oom"):
			st = StatusBad
		case strings.Contains(a, "restart"):
			st = StatusWarn
		case strings.Contains(a, "start") || strings.Contains(a, "healthy"):
			st = StatusGood
		}
		name := e.ContainerName
		if name == "" {
			name = shortID(e.ContainerID)
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(e.Timestamp))),
			cell(name),
			pill(titleCase(e.Action), st),
			cell(titleCase(e.EventType)),
		})
	}
	note := ""
	if truncated {
		note = "Showing the most recent " + fmtInt(limit) + " of " + fmtInt(len(list)) + " events in the period."
	}
	return table(
		[]Column{
			{Label: "When", Width: 2.2},
			{Label: "Workload", Width: 2.6},
			{Label: "Action", Width: 1.8},
			{Label: "Type", Width: 1.8},
		}, rows, "Workload events", note)
}

// --- 4. Availability & Service Levels --------------------------------

func buildAvailability(b *buildCtx) []Section {
	f := b.facts
	a := f.Availability
	var out []Section

	healthyPct := pct(f.Containers.Healthy, maxIntOf(f.Containers.Running, 1))
	st := section(Strategic, "Service levels")

	// With history, this report states what happened; without it, it can only
	// describe the present and must say so. The two openings are different
	// claims and are worded as such.
	if a.Assessed {
		st.Blocks = append(st.Blocks,
			para("Availability here is measured, not inferred: the platform samples every "+
				"workload's run state every 30 seconds and keeps the series, so the figures below "+
				"describe the period rather than the instant this report was generated. It remains "+
				"a floor on service quality — the platform can see that a process was running, not "+
				"that it was answering correctly."),
			kpis(availabilityKPIs(a)...),
			errorBudgetGauge(a),
			availabilityCoverageCallout(a))
	} else {
		st.Blocks = append(st.Blocks,
			para("Availability here means workloads that are running and reporting themselves healthy. "+
				"The platform cannot observe end-user experience, so this is a floor on service "+
				"quality rather than a measure of it — a workload can be healthy and still be slow."),
			kpis(
				KPI{Label: "Running", Value: fmtInt(f.Containers.Running), Target: fmtInt(f.Containers.Total) + " defined"},
				KPI{Label: "Reporting healthy", Value: fmtPercent(healthyPct), Target: "100%",
					Status: thresholdStatus(healthyPct, 95, 80)},
				KPI{Label: "Unhealthy now", Value: fmtInt(f.Containers.Unhealthy), Target: "0",
					Status: countStatus(f.Containers.Unhealthy, 0, 0)},
				KPI{Label: "Observability gap", Value: fmtInt(f.Containers.NoHealthcheck),
					Status: countStatus(f.Containers.NoHealthcheck, 0, 3),
					Hint:   "Running workloads with no health check"},
			),
			availabilityCoverageCallout(a))
	}
	st.Blocks = append(st.Blocks, workloadStateChart(f))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Against target")
	if a.Assessed {
		ta.Blocks = append(ta.Blocks,
			worstUptimeChart(a),
			availabilityTable(a, 25))
	}
	ta.Blocks = append(ta.Blocks, metricTargetTable(b),
		eventTrendChart(f, "Availability events", "Restarts and unexpected stops per day."))
	if f.Containers.NoHealthcheck > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Health checks are missing",
			plural(f.Containers.NoHealthcheck, "running workload has", "running workloads have")+
				" no health check defined. The platform reports them as running because the process "+
				"exists, not because the service works. Every availability figure in this report is "+
				"optimistic by that margin."))
	}
	if len(b.snap.Environments) > 1 {
		ta.Blocks = append(ta.Blocks, heading(2, "Environments"), environmentTable(b.snap))
	}
	out = append(out, ta)
	return out
}

// --- 5. Change & Delivery (DORA) -------------------------------------

func buildChangeDelivery(b *buildCtx) []Section {
	f := b.facts.Delivery
	var out []Section

	st := section(Strategic, "Delivery performance")
	if !f.Assessed || (f.Deployments == 0 && f.PromotionsTotal == 0) {
		st.Blocks = append(st.Blocks, callout(StatusInfo, "No change observed",
			"No deployments, updates or promotions were recorded in this period. Either the "+
				"estate was deliberately frozen, or change is reaching it by a route the platform "+
				"does not see — both are worth confirming."))
		out = append(out, st)
		return out
	}
	st.Blocks = append(st.Blocks,
		para("Delivery is assessed against the DORA four keys, as far as a container platform can "+
			"observe them. Deployment frequency and change failure rate are measured directly. "+
			"Lead time for changes begins in source control, which this platform does not see, "+
			"and is excluded rather than estimated."),
		kpis(
			KPI{Label: "Deployments", Value: fmtInt(f.Deployments)},
			KPI{Label: "Per week", Value: fmtDecimal(f.DeploymentsPerWeek, 1), Target: "≥ 7 elite",
				Status: thresholdStatus(f.DeploymentsPerWeek, 7, 1)},
			KPI{Label: "Change failure rate", Value: fmtPercent(f.ChangeFailureRate), Target: "≤ 5%",
				Status: invThresholdStatus(f.ChangeFailureRate, 5, 20)},
			KPI{Label: "Rollbacks", Value: fmtInt(f.Rollbacks), Target: "0",
				Status: countStatus(f.Rollbacks, 0, 2)},
		),
		doraTable(f))
	out = append(out, st)

	ta := section(Tactical, "Change detail")
	if len(f.DeploymentsByDay) > 0 {
		pts := make([]ChartPoint, 0, len(f.DeploymentsByDay))
		for _, d := range f.DeploymentsByDay {
			pts = append(pts, ChartPoint{Label: d.Label, Value: float64(d.Count)})
		}
		ta.Blocks = append(ta.Blocks, chartBlock(Chart{
			Kind: ChartBar, Title: "Deployments over the period", Height: 120, ShowValues: false,
			Series:  []ChartSeries{{Name: "Deployments", Points: pts}},
			Caption: "A steady rhythm is healthier than the same total delivered in one batch.",
		}))
	}
	if f.PromotionsTotal > 0 {
		ta.Blocks = append(ta.Blocks,
			heading(2, "Promotions"),
			kpis(
				KPI{Label: "Promotions", Value: fmtInt(f.PromotionsTotal)},
				KPI{Label: "Succeeded", Value: fmtInt(f.PromotionsSucceeded),
					Status: thresholdStatus(pct(f.PromotionsSucceeded, f.PromotionsTotal), 95, 80)},
				KPI{Label: "Mean time to promote", Value: fmtDuration(f.MeanPromotionHours * 60)},
			))
	}
	if f.UpdatesApplied+f.UpdatesFailed > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Automated updates"), kpis(
			KPI{Label: "Applied", Value: fmtInt(f.UpdatesApplied), Status: StatusGood},
			KPI{Label: "Failed", Value: fmtInt(f.UpdatesFailed), Target: "0",
				Status: countStatus(f.UpdatesFailed, 0, 1)},
		))
	}
	out = append(out, ta)
	return out
}

func doraTable(f DeliveryFacts) Block {
	freqBand, freqStatus := doraFreqBand(f.DeploymentsPerWeek)
	cfrBand, cfrStatus := doraCFRBand(f.ChangeFailureRate, f.Deployments)
	return table(
		[]Column{
			{Label: "DORA key", Width: 2},
			{Label: "This period", Width: 1.4, Align: "right"},
			{Label: "Band", Width: 1.3},
			{Label: "Elite benchmark", Width: 4},
		},
		[][]Cell{
			{cell("Deployment frequency"), cell(fmtDecimal(f.DeploymentsPerWeek, 1) + " / week"),
				pill(freqBand, freqStatus), cell("On demand — several times a day")},
			{cell("Change failure rate"), cell(fmtPercent(f.ChangeFailureRate)),
				pill(cfrBand, cfrStatus), cell("At or below 5% of deployments")},
			{cell("Failed deployment recovery"), cell("—"), pill("Not measured", StatusNeutral),
				cell("Under one hour. Requires incident data this platform does not hold.")},
			{cell("Lead time for changes"), cell("—"), pill("Not measured", StatusNeutral),
				cell("Under one day. Begins at commit, outside this platform's view.")},
		},
		"DORA four keys",
		"Bands follow the DORA State of DevOps benchmarks. Two keys are reported as not measured rather than estimated from partial data.")
}

func doraFreqBand(perWeek float64) (string, Status) {
	switch {
	case perWeek >= 7:
		return "Elite", StatusGood
	case perWeek >= 1:
		return "High", StatusGood
	case perWeek >= 0.25:
		return "Medium", StatusWarn
	default:
		return "Low", StatusBad
	}
}

func doraCFRBand(rate float64, deployments int) (string, Status) {
	if deployments == 0 {
		return "No data", StatusNeutral
	}
	switch {
	case rate <= 5:
		return "Elite", StatusGood
	case rate <= 10:
		return "High", StatusGood
	case rate <= 20:
		return "Medium", StatusWarn
	default:
		return "Low", StatusBad
	}
}

// --- 6. Environment Health -------------------------------------------

func buildEnvironmentHealth(b *buildCtx) []Section {
	f := b.facts
	var out []Section

	ta := section(Tactical, "Environment health")
	ta.Blocks = append(ta.Blocks,
		kpis(
			KPI{Label: "Environments", Value: fmtInt(len(b.snap.Environments))},
			KPI{Label: "Workloads running", Value: fmtInt(f.Containers.Running), Target: fmtInt(f.Containers.Total) + " defined"},
			KPI{Label: "Unhealthy", Value: fmtInt(f.Containers.Unhealthy), Status: countStatus(f.Containers.Unhealthy, 0, 0)},
			KPI{Label: "Headroom", Value: fmtPercent(f.Capacity.HeadroomPercent), Target: "≥ 25%",
				Status: thresholdStatus(f.Capacity.HeadroomPercent, 25, 15)},
		),
		environmentTable(b.snap),
		workloadStateChart(f))
	if a := f.Availability; a.Assessed {
		ta.Blocks = append(ta.Blocks,
			heading(2, "Measured availability"),
			kpis(availabilityKPIs(a)...),
			availabilityCoverageCallout(a))
	}
	if len(b.snap.Agents) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Remote agents"), agentTable(b.snap))
	}
	out = append(out, ta)

	op := section(Operational, "Engine and resources")
	op.Blocks = append(op.Blocks, engineTable(b), resourceTable(b))
	if a := f.Availability; a.Assessed && len(a.Rows) > 0 {
		op.Blocks = append(op.Blocks, availabilityTable(a, 30))
	}
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, containerTable(b.snap, 40))
	}
	out = append(out, op)
	return out
}

func environmentTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Environments))
	for _, e := range snap.Environments {
		st, label := StatusGood, "Active"
		if !e.IsActive {
			st, label = StatusWarn, "Inactive"
		}
		version := "—"
		if e.DockerVersion != nil && *e.DockerVersion != "" {
			version = *e.DockerVersion
		} else if e.K8sVersion != nil && *e.K8sVersion != "" {
			version = *e.K8sVersion
		}
		conn := titleCase(e.Connection)
		if e.AgentStatus != nil && *e.AgentStatus != "" {
			conn = "Agent (" + *e.AgentStatus + ")"
		}
		last := "—"
		if e.LastConnected != nil {
			last = fmtDate(parseTS(*e.LastConnected))
		}
		rows = append(rows, []Cell{
			cell(e.Name), cell(titleCase(e.Orchestrator)), cell(conn), cell(version), cell(last), pill(label, st),
		})
	}
	return table(
		[]Column{
			{Label: "Environment", Width: 2.2},
			{Label: "Orchestrator", Width: 1.4},
			{Label: "Connection", Width: 1.8},
			{Label: "Engine", Width: 1.4},
			{Label: "Last seen", Width: 1.4},
			{Label: "State", Width: 1.1},
		}, rows, "Environments",
		"Connection details, credentials and socket paths are deliberately omitted from this report.")
}

func agentTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Agents))
	for _, a := range snap.Agents {
		st := StatusGood
		if !strings.EqualFold(a.Status, "connected") && !strings.EqualFold(a.Status, "online") {
			st = StatusWarn
		}
		rows = append(rows, []Cell{
			cell(a.Name), pill(titleCase(a.Status), st), cell(a.Version),
			cell(a.OS + " / " + a.Arch), cell(fmtDate(parseTS(a.LastSeen))),
		})
	}
	return table(
		[]Column{
			{Label: "Agent", Width: 2.2},
			{Label: "State", Width: 1.2},
			{Label: "Version", Width: 1.2},
			{Label: "Platform", Width: 1.8},
			{Label: "Last seen", Width: 1.4},
		}, rows, "Remote agents",
		"Agent tokens are never included in a report.")
}

func engineTable(b *buildCtx) Block {
	d := b.snap.Docker
	h := b.snap.Host
	pairs := []Pair{
		{K: "Engine version", V: orDash(d.ServerVersion, h.ServerVersion)},
		{K: "API version", V: orDash(d.APIVersion)},
		{K: "Operating system", V: orDash(d.OS, h.OS)},
		{K: "Kernel", V: orDash(d.KernelVersion, h.KernelVersion), Mono: true},
		{K: "Architecture", V: orDash(d.Architecture)},
		{K: "Storage driver", V: orDash(d.StorageDriver)},
		{K: "Cgroup driver", V: orDash(d.CgroupDriver + " v" + d.CgroupVersion)},
		{K: "Default runtime", V: orDash(d.DefaultRuntime)},
		{K: "Host uptime", V: fmtDecimal(b.facts.Capacity.UptimeDays, 1) + " days"},
	}
	// The hostname is host configuration, not a secret, but it is the
	// kind of detail that should not travel further than it needs to.
	return Block{Kind: BlockKeyValue, Pairs: pairs, Title: "Container engine"}
}

// resourceTable reports CPU and memory as a period rather than as an
// instant.
//
// The average and the peak are shown side by side because neither is the
// whole answer: an average of 34 % hides an hour at 91 %, and a peak of
// 91 % hides that the host spent the rest of the month idle. The colour
// is applied to the average cell but graded on the p95, so a single
// excursion cannot turn a month red and sustained saturation cannot hide
// behind a flattering mean.
//
// Images and volumes are counts, not time series, so their peak cell is
// an em dash. Repeating the count there would read as a measurement.
func resourceTable(b *buildCtx) Block {
	f := b.facts
	cpu, mem := f.Capacity.CPUStat, f.Capacity.MemStat

	return table(
		[]Column{
			{Label: "Resource", Width: 2},
			{Label: "Average", Width: 1.1, Align: "right"},
			{Label: "Peak", Width: 1.1, Align: "right"},
			{Label: "Total", Width: 1.2, Align: "right"},
			{Label: "Note", Width: 3.4},
		},
		[][]Cell{
			{cell("CPU"),
				cellStatus(fmtPercent(f.Capacity.CPUPercent),
					invThresholdStatus(cpu.grade(f.Capacity.CPUPercent), 75, 90)),
				cell(peakCell(cpu)),
				cell(fmtInt(f.Capacity.CPUCount) + " cores"),
				cell("Run queue " + fmtDecimal(f.Capacity.Load1PerCore, 2) + " per core")},
			{cell("Memory"),
				cellStatus(fmtPercent(f.Capacity.MemPercent),
					invThresholdStatus(mem.grade(f.Capacity.MemPercent), 75, 90)),
				cell(peakCell(mem)),
				cell(fmtBytes(f.Capacity.MemTotalBytes)),
				cell(fmtBytes(f.Capacity.MemUsedBytes) + " allocated")},
			{cell("Images"), cell(fmtInt(f.Images.InUse)), cell("—"), cell(fmtInt(f.Images.Total)),
				cell(fmtBytes(f.Images.ReclaimableBytes) + " reclaimable from " + fmtInt(f.Images.Unused) + " unused")},
			{cell("Volumes"), cell(fmtInt(f.Capacity.VolumeCount - f.Capacity.UnusedVolumes)), cell("—"), cell(fmtInt(f.Capacity.VolumeCount)),
				cell(fmtInt(f.Capacity.UnusedVolumes) + " unreferenced, " + fmtBytes(f.Capacity.VolumeBytes) + " total")},
		}, rows2Caption(b), "")
}

// peakCell renders a period peak, or an em dash when the figure in the
// average column was sampled once rather than measured over time — a
// single reading has no peak, and printing the same number twice would
// imply it does.
func peakCell(st periodStat) string {
	if st.Samples == 0 {
		return "—"
	}
	return fmtPercent(st.Peak)
}

func rows2Caption(b *buildCtx) string {
	if b.facts.Capacity.HistoryAvailable {
		return "Resource utilisation over the period"
	}
	return "Resource utilisation at the time of generation"
}

func orDash(candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(strings.TrimSuffix(c, " v"))
		if c != "" && c != "v" {
			return c
		}
	}
	return "—"
}
