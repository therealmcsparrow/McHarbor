// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
)

// workflowStat is one workflow's record over the period.
type workflowStat struct {
	ID, Name, Status  string
	Runs, Failed, OK  int
	Cancelled         int
	Durations         []float64
	LastRun, LastFail string
	Deleted           bool
}

// workflowFacts is what the workflow-automation report derives.
type workflowFacts struct {
	Assessed     bool
	RunsAssessed bool
	Truncated    bool
	Workflows    int
	Active       int
	Runs         []govWorkflowRunRow
	Completed    int
	Failed       int
	Cancelled    int
	Running      int
	SuccessRate  float64
	MedianMs     float64
	P95Ms        float64
	ByTrigger    map[string]int
	Stats        []*workflowStat
	Failing      []*workflowStat
	IdleActive   []*workflowStat
}

// govWorkflowFailingRate and govWorkflowMinRuns decide when a workflow is
// "repeatedly failing" rather than unlucky: at least three runs, half failed.
const (
	govWorkflowFailingRate = 50.0
	govWorkflowMinRuns     = 3
)

func govWorkflowStats(snap *Snapshot) workflowFacts {
	g := snap.Governance
	p := snap.Period
	f := workflowFacts{
		Assessed:     snap.available(srcWorkflows),
		RunsAssessed: snap.available(srcWorkflowRuns),
		ByTrigger:    map[string]int{},
	}
	byID := map[string]*workflowStat{}
	if f.Assessed {
		for _, w := range g.Workflows {
			f.Workflows++
			if w.Status == "active" {
				f.Active++
			}
			st := &workflowStat{ID: w.ID, Name: w.Name, Status: w.Status}
			byID[w.ID] = st
			f.Stats = append(f.Stats, st)
		}
	}
	if !f.RunsAssessed {
		return f
	}
	if n := len(g.WorkflowRuns); n >= govRunsPage && govInPeriod(g.WorkflowRuns[n-1].StartedAt, p) {
		f.Truncated = true
	}
	var durations []float64
	for _, r := range g.WorkflowRuns {
		if !govInPeriod(r.StartedAt, p) {
			continue
		}
		f.Runs = append(f.Runs, r)
		f.ByTrigger[orDash(r.Trigger)]++
		st := byID[r.WorkflowID]
		if st == nil {
			st = &workflowStat{ID: r.WorkflowID, Name: "(deleted workflow " + shortID(r.WorkflowID) + ")", Deleted: true}
			byID[r.WorkflowID] = st
			f.Stats = append(f.Stats, st)
		}
		st.Runs++
		if parseTS(r.StartedAt).After(parseTS(st.LastRun)) {
			st.LastRun = r.StartedAt
		}
		switch r.Status {
		case "completed":
			f.Completed++
			st.OK++
		case "failed":
			f.Failed++
			st.Failed++
			if parseTS(r.StartedAt).After(parseTS(st.LastFail)) {
				st.LastFail = r.StartedAt
			}
		case "cancelled":
			f.Cancelled++
			st.Cancelled++
		default:
			f.Running++
		}
		if r.DurationMs > 0 && r.Status != "running" {
			durations = append(durations, float64(r.DurationMs))
			st.Durations = append(st.Durations, float64(r.DurationMs))
		}
	}
	f.SuccessRate = pct(f.Completed, f.Completed+f.Failed)
	f.MedianMs = median(durations)
	f.P95Ms = govPercentile(durations, 95)
	for _, st := range f.Stats {
		if st.Runs >= govWorkflowMinRuns && pct(st.Failed, st.OK+st.Failed) >= govWorkflowFailingRate {
			f.Failing = append(f.Failing, st)
		}
		if st.Status == "active" && st.Runs == 0 {
			f.IdleActive = append(f.IdleActive, st)
		}
	}
	sort.SliceStable(f.Stats, func(i, j int) bool {
		if f.Stats[i].Runs != f.Stats[j].Runs {
			return f.Stats[i].Runs > f.Stats[j].Runs
		}
		return f.Stats[i].Name < f.Stats[j].Name
	})
	return f
}

func workflowKPIs(f workflowFacts) []KPI {
	if !f.RunsAssessed {
		return []KPI{}
	}
	return []KPI{
		{Label: "Runs", Value: fmtInt(len(f.Runs)), Hint: fmtInt(f.Active) + " active workflows"},
		{Label: "Success rate", Value: govPercentOrDash(f.SuccessRate, f.Completed+f.Failed),
			Target: "≥ 95%", Status: govRateStatus(f.SuccessRate, f.Completed+f.Failed, 95, 80)},
		{Label: "Failed runs", Value: fmtInt(f.Failed), Status: countStatus(f.Failed, 0, 5)},
		{Label: "Repeatedly failing", Value: fmtInt(len(f.Failing)),
			Status: countStatus(len(f.Failing), 0, 0), Hint: "workflows, half their runs or more"},
		{Label: "Median duration", Value: govMsOrDash(f.MedianMs, len(f.Runs))},
	}
}

func workflowFindings(f workflowFacts) []Finding {
	if !f.RunsAssessed {
		return nil
	}
	var out []Finding
	if n := len(f.Failing); n > 0 {
		names := make([]string, 0, n)
		for _, s := range f.Failing {
			names = append(names, s.Name)
		}
		out = append(out, Finding{
			Severity: SevHigh,
			Title:    plural(n, "workflow fails at least half of its runs", "workflows fail at least half of their runs"),
			Detail: "Affected: " + joinAnd(names) + ". Whatever these automate is not happening reliably, and a failing " +
				"workflow is usually noticed only when the thing it was meant to prevent happens.",
			Recommendation: "Open the failed runs in Workflows, fix or disable each workflow, and add an error branch that notifies someone.",
			Owner:          "Workflow owner",
		})
	}
	if f.Completed+f.Failed > 0 && f.SuccessRate < 95 && len(f.Failing) == 0 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          "Workflow success rate is " + fmtPercent(f.SuccessRate),
			Detail:         plural(f.Failed, "run failed", "runs failed") + " in the period, spread across several workflows rather than concentrated in one.",
			Recommendation: "Review the failure pattern by trigger; failures that cluster on webhook triggers often mean a malformed caller.",
			Owner:          "Workflow owner",
		})
	}
	if n := len(f.IdleActive); n > 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(n, "active workflow did not run in the period", "active workflows did not run in the period"),
			Detail:         "An active workflow with no runs has a trigger that never fired. That is either correct — nothing happened — or a broken trigger.",
			Recommendation: "Confirm each trigger still matches something real, and set workflows that are no longer needed to draft.",
			Owner:          "Workflow owner",
		})
	}
	if f.Truncated {
		out = append(out, Finding{
			Severity:       SevInfo,
			Title:          "Run history exceeds one read",
			Detail:         "The most recent " + fmtInt(govRunsPage) + " runs were read and all fall inside the period, so earlier runs are not counted.",
			Recommendation: "Report on a shorter period for complete figures.",
			Owner:          "Platform engineering",
		})
	}
	return out
}

func workflowLead(f workflowFacts) string {
	switch {
	case !f.RunsAssessed:
		return "Workflow run history could not be read, so automation reliability is not assessed."
	case len(f.Runs) == 0:
		return "No workflow ran in this period."
	case len(f.Failing) > 0:
		return plural(len(f.Failing), "workflow fails", "workflows fail") + " at least half of its runs and needs attention."
	default:
		return plural(len(f.Runs), "workflow run", "workflow runs") + " completed at a " +
			fmtPercent(f.SuccessRate) + " success rate."
	}
}

func buildWorkflowAutomation(b *buildCtx) []Section {
	snap := b.snap
	f := govWorkflowStats(snap)
	var out []Section

	ta := section(Tactical, "Automation reliability")
	ta.Blocks = append(ta.Blocks,
		para("Workflows act on the platform without anyone watching. This report asks whether they ran, "+
			"whether they succeeded, and which triggers drive them. Error text is not reproduced — it can "+
			"echo the URLs and credentials a node called — so open the run in Workflows for detail."))
	if !f.RunsAssessed {
		ta.Blocks = append(ta.Blocks, govNotAssessed(snap, srcWorkflowRuns, "Workflow run history"))
		return append(out, ta)
	}
	if !f.Assessed {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "Workflow names unavailable",
			"The workflow list could not be read ("+snap.note(srcWorkflows)+"), so runs are grouped by identifier only."))
	}
	ta.Blocks = append(ta.Blocks,
		kpis(
			KPI{Label: "Completed", Value: fmtInt(f.Completed), Status: StatusGood},
			KPI{Label: "Failed", Value: fmtInt(f.Failed), Status: countStatus(f.Failed, 0, 5)},
			KPI{Label: "Cancelled", Value: fmtInt(f.Cancelled)},
			KPI{Label: "95th percentile", Value: govMsOrDash(f.P95Ms, len(f.Runs)), Hint: "run duration"},
		))
	if len(f.Runs) > 0 {
		ta.Blocks = append(ta.Blocks, workflowDailyChart(f, snap.Period), workflowTriggerChart(f))
	}
	ta.Blocks = append(ta.Blocks, workflowTable(f, true))
	out = append(out, ta)

	op := section(Operational, "Workflow detail")
	op.Blocks = append(op.Blocks, workflowTable(f, false))
	if len(f.IdleActive) > 0 {
		names := make([]string, 0, len(f.IdleActive))
		for _, s := range f.IdleActive {
			names = append(names, s.Name)
		}
		op.Blocks = append(op.Blocks, heading(2, "Active but idle"), bullets(names...))
	}
	out = append(out, op)
	return out
}

func workflowDailyChart(f workflowFacts, p Period) Block {
	var ok, bad []string
	for _, r := range f.Runs {
		if r.Status == "failed" {
			bad = append(bad, r.StartedAt)
		} else {
			ok = append(ok, r.StartedAt)
		}
	}
	return chartBlock(Chart{
		Kind: ChartStackedBar, Title: "Runs per day", Height: 120, ShowLegend: true,
		Series: []ChartSeries{
			{Name: "Succeeded or cancelled", Status: StatusGood, Points: govDaySeries(ok, p)},
			{Name: "Failed", Status: StatusBad, Points: govDaySeries(bad, p)},
		},
	})
}

func workflowTriggerChart(f workflowFacts) Block {
	ranked := topCounted(f.ByTrigger, 8)
	pts := make([]ChartPoint, 0, len(ranked))
	for _, c := range ranked {
		pts = append(pts, ChartPoint{Label: govTriggerLabel(c.Label), Value: float64(c.Count)})
	}
	return chartBlock(Chart{
		Kind: ChartHBar, Title: "Runs by trigger", ShowValues: true, Height: 110,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "“Event” covers cron schedules and container, image and alert events.",
	})
}

// workflowTable renders the per-workflow record; failingOnly limits it to
// workflows with at least one failure, for the tactical view.
func workflowTable(f workflowFacts, failingOnly bool) Block {
	rows := [][]Cell{}
	for _, s := range f.Stats {
		if failingOnly && s.Failed == 0 {
			continue
		}
		if !failingOnly && s.Runs == 0 && s.Status != "active" {
			continue
		}
		rate := pct(s.OK, s.OK+s.Failed)
		rows = append(rows, []Cell{
			cell(s.Name),
			pill(titleCase(orDash(s.Status, "unknown")), govWorkflowStatusTone(s)),
			cell(fmtInt(s.Runs)),
			cellStatus(fmtInt(s.Failed), countStatus(s.Failed, 0, 2)),
			cellStatus(govPercentOrDash(rate, s.OK+s.Failed), govRateStatus(rate, s.OK+s.Failed, 95, 80)),
			cell(govMsOrDash(median(s.Durations), len(s.Durations))),
			cell(orDash(fmtDateTime(parseTS(s.LastFail)))),
		})
	}
	if len(rows) == 0 {
		if failingOnly {
			return callout(StatusGood, "No failures", "Every workflow run in the period that finished, succeeded.")
		}
		return callout(StatusInfo, "No workflows", "No workflow is active or ran in this period.")
	}
	caption := "Every workflow that ran, or is active"
	if failingOnly {
		caption = "Workflows with failures"
	}
	return table([]Column{
		{Label: "Workflow", Width: 2.4},
		{Label: "State", Width: 1},
		{Label: "Runs", Width: 0.8, Align: "right"},
		{Label: "Failed", Width: 0.8, Align: "right"},
		{Label: "Success", Width: 0.9, Align: "right"},
		{Label: "Median time", Width: 1, Align: "right"},
		{Label: "Last failure", Width: 1.6},
	}, rows, caption, "")
}

func govWorkflowStatusTone(s *workflowStat) Status {
	switch {
	case s.Deleted:
		return StatusMuted
	case s.Status == "active":
		return StatusGood
	default:
		return StatusMuted
	}
}

func govTriggerLabel(t string) string {
	switch t {
	case "manual":
		return "Manual"
	case "auto":
		return "Event"
	case "webhook":
		return "Webhook"
	case "link":
		return "Linked workflow"
	}
	return titleCase(t)
}

func govRateStatus(rate float64, n int, good, warn float64) Status {
	if n == 0 {
		return StatusNeutral
	}
	return thresholdStatus(rate, good, warn)
}

func govMsOrDash(ms float64, n int) string {
	if n == 0 || ms <= 0 {
		return "—"
	}
	if ms < 1000 {
		return fmtDecimal(ms, 0) + " ms"
	}
	if ms < 60000 {
		return fmtDecimal(ms/1000, 1) + " s"
	}
	return fmtDuration(ms / 60000)
}
