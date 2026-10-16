// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sort"
	"strings"
)

// TypeControlFrameworkEvidence is the report key for a compliance pack: evidence
// for a named control framework, never a claim of compliance with one.
const TypeControlFrameworkEvidence = "control-framework-evidence"

// srcCompliancePacks is the source key for the pack mapping and its latest run.
const srcCompliancePacks = "compliance_packs"

// srcComplianceAttestations is the sign-off record: who accepted which framework's
// evidence, on which run, when. It is the one source that answers "when was access
// last reviewed, and by whom" rather than measuring a proxy for it.
const srcComplianceAttestations = "compliance_attestations"

// packAttestationRow is one sign-off as GET /api/compliance/attestations returns it.
type packAttestationRow struct {
	ID             string `json:"id"`
	Framework      string `json:"framework"`
	RunID          string `json:"runId"`
	Scope          string `json:"scope"`
	AttestedByName string `json:"attestedByName"`
	AttestedAt     string `json:"attestedAt"`
	Note           string `json:"note"`
}

// packFanOut is how many run detail reads run at once. One per framework, and
// there is one framework today, so it is a bound rather than a throttle.
const packFanOut = 2

type packControlRow struct {
	ID          string   `json:"id"`
	Group       string   `json:"group"`
	Title       string   `json:"title"`
	Requirement string   `json:"requirement"`
	Coverage    string   `json:"coverage"`
	Evidence    []string `json:"evidence"`
}

type packEvidenceRow struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	Notes string `json:"notes"`
}

type packFrameworkRow struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Runnable    bool              `json:"runnable"`
	Controls    []packControlRow  `json:"controls"`
	Evidence    []packEvidenceRow `json:"evidence"`
}

type packRunRow struct {
	ID            string `json:"id"`
	Framework     string `json:"framework"`
	EnvironmentID string `json:"environmentId"`
	Trigger       string `json:"trigger"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
	Status        string `json:"status"`
	LastError     string `json:"lastError"`
	Summary       struct {
		Pass    int `json:"pass"`
		Warn    int `json:"warn"`
		Fail    int `json:"fail"`
		Info    int `json:"info"`
		Unknown int `json:"unknown"`
	} `json:"summary"`
}

type packSummaryRow struct {
	Framework packFrameworkRow `json:"framework"`
	LatestRun *packRunRow      `json:"latestRun"`
}

type packResultRow struct {
	ID      string              `json:"id"`
	Status  string              `json:"status"`
	Facts   []govComplianceFact `json:"facts"`
	Reasons []string            `json:"reasons"`
}

type packRunDetail struct {
	Run     packRunRow      `json:"run"`
	Results []packResultRow `json:"results"`
}

// compliancePacksData is the snapshot slice for this report. Results are keyed by
// run ID and hold only the runs that were actually read, so absence means "not
// read" rather than "no findings".
type compliancePacksData struct {
	Packs   []packSummaryRow
	Results map[string][]packResultRow

	// Attestations are newest first, superseded ones included.
	Attestations []packAttestationRow
}

// packJobs collects the pack mapping and, for each pack with a run, that run's
// control results. Both reads are loopback calls forwarding the caller's cookie,
// so an operator without compliance.view gets nothing rather than a document built
// from data their permissions do not grant.
func packJobs(ctx context.Context, api *hostAPI, snap *Snapshot) map[string]func() {
	return map[string]func(){
		srcCompliancePacks: func() {
			var packs []packSummaryRow
			if !api.get(ctx, srcCompliancePacks, "Compliance packs", "/api/compliance/packs", nil, &packs) {
				return
			}
			snap.Governance.Packs.Packs = packs
			snap.Governance.Packs.Results = map[string][]packResultRow{}
			ids := make([]string, 0, len(packs))
			for _, p := range packs {
				if p.LatestRun != nil && p.LatestRun.ID != "" {
					ids = append(ids, p.LatestRun.ID)
				}
			}
			runFanOut(packFanOut, ids, func(id string) {
				var detail packRunDetail
				if api.get(ctx, srcCompliancePacks, "Compliance pack run", "/api/compliance/packs/runs/"+id, nil, &detail) {
					snap.Governance.Packs.Results[id] = detail.Results
				}
			})
		},
		srcComplianceAttestations: func() {
			api.getList(ctx, srcComplianceAttestations, "Compliance attestations", "/api/compliance/attestations",
				nil, &snap.Governance.Packs.Attestations)
		},
	}
}

// packFacts is the shape the builder reads: one framework, its controls, its
// latest run and the verdicts that run recorded.
type packFacts struct {
	Assessed bool
	Pack     packSummaryRow
	Results  map[string]packResultRow
	Coverage struct {
		Mechanical int
		Evidenced  int
		NotCovered int
	}
}

func packFactsOf(snap *Snapshot) packFacts {
	var f packFacts
	if len(snap.Governance.Packs.Packs) == 0 {
		return f
	}
	f.Assessed = true
	f.Pack = snap.Governance.Packs.Packs[0]
	f.Results = map[string]packResultRow{}
	if f.Pack.LatestRun != nil {
		for _, r := range snap.Governance.Packs.Results[f.Pack.LatestRun.ID] {
			f.Results[r.ID] = r
		}
	}
	for _, c := range f.Pack.Framework.Controls {
		switch c.Coverage {
		case "mechanical":
			f.Coverage.Mechanical++
		case "evidenced":
			f.Coverage.Evidenced++
		default:
			f.Coverage.NotCovered++
		}
	}
	return f
}

// buildControlFrameworkEvidence assembles the pack as a document. The first page
// says what was not assessed, before anything that could be read as a clean
// result.
func buildControlFrameworkEvidence(b *buildCtx) []Section {
	f := packFactsOf(b.snap)
	var out []Section

	st := section(Strategic, "Evidence coverage")
	st.Blocks = append(st.Blocks,
		para("This report is evidence for a named control framework. It is not a claim of compliance "+
			"with one, and a passing check means the check passed — whether the control is met is a "+
			"judgement for the reader and their auditor. Controls McHarbor cannot see are listed rather "+
			"than omitted, because a document listing only what was checked reads as a clean result."))
	if !f.Assessed {
		st.Blocks = append(st.Blocks, govNotAssessed(b.snap, srcCompliancePacks, "The compliance packs"))
		return append(out, st)
	}
	st.Blocks = append(st.Blocks, para(f.Pack.Framework.Name+" — "+f.Pack.Framework.Description))

	run := f.Pack.LatestRun
	if run == nil {
		st.Blocks = append(st.Blocks, callout(StatusWarn, "Never run",
			"This pack has never been run, so no control carries a verdict. The mapping below still "+
				"names every control and the evidence behind it."))
	} else {
		st.Blocks = append(st.Blocks,
			kpis(
				KPI{Label: "Passing", Value: fmtInt(run.Summary.Pass), Status: StatusGood},
				KPI{Label: "Needs attention", Value: fmtInt(run.Summary.Warn), Status: countStatus(run.Summary.Warn, 0, 2)},
				KPI{Label: "Failing", Value: fmtInt(run.Summary.Fail), Status: countStatus(run.Summary.Fail, 0, 0)},
				KPI{Label: "Not assessed", Value: fmtInt(run.Summary.Unknown), Status: countStatus(run.Summary.Unknown, 0, 0)},
			),
			chartBlock(Chart{
				Kind: ChartDonut, Title: "Check verdicts", Unit: "controls", Height: 130,
				Series: []ChartSeries{{Points: []ChartPoint{
					{Label: "Pass", Value: float64(run.Summary.Pass), Status: StatusGood},
					{Label: "Attention", Value: float64(run.Summary.Warn), Status: StatusWarn},
					{Label: "Fail", Value: float64(run.Summary.Fail), Status: StatusBad},
					{Label: "Info", Value: float64(run.Summary.Info), Status: StatusInfo},
					{Label: "Not assessed", Value: float64(run.Summary.Unknown), Status: StatusMuted},
				}}},
				Caption: "Informational controls describe rather than judge and are excluded from the pass rate.",
			}))
		if run.Summary.Unknown > 0 {
			st.Blocks = append(st.Blocks, callout(StatusWarn, "Controls not assessed",
				plural(run.Summary.Unknown, "control", "controls")+" carry no verdict. Not assessed is not a "+
					"pass: the evidence could not be gathered."))
		}
		if strings.TrimSpace(run.LastError) != "" {
			st.Blocks = append(st.Blocks, callout(StatusBad, "The last run failed", run.LastError))
		}
	}
	st.Blocks = append(st.Blocks, keyvalue(
		Pair{K: "Checked mechanically", V: fmtInt(f.Coverage.Mechanical) + " controls"},
		Pair{K: "Evidenced from recorded data", V: fmtInt(f.Coverage.Evidenced) + " controls"},
		Pair{K: "Not covered by McHarbor", V: fmtInt(f.Coverage.NotCovered) + " controls"},
	))
	out = append(out, st)

	ta := section(Tactical, "Controls")
	ta.Blocks = append(ta.Blocks, packControlTable(f))
	out = append(out, ta)

	op := section(Operational, "Evidence sources")
	op.Blocks = append(op.Blocks, packEvidenceTable(f))
	if run != nil {
		op.Blocks = append(op.Blocks, para("Recorded "+fmtDateTime(parseTS(run.StartedAt))+
			" ("+packTriggerWord(run.Trigger)+")."))
	}
	out = append(out, op)
	return out
}

func packControlTable(f packFacts) Block {
	controls := append([]packControlRow(nil), f.Pack.Framework.Controls...)
	sort.SliceStable(controls, func(i, j int) bool { return controls[i].Group < controls[j].Group })
	rows := make([][]Cell, 0, len(controls))
	for _, c := range controls {
		verdict, tone := "Not run", StatusMuted
		detail := "—"
		if r, ok := f.Results[c.ID]; ok {
			verdict, tone = packVerdictWord(r.Status), govVerdictTone(r.Status)
			if len(r.Reasons) > 0 {
				detail = strings.Join(r.Reasons, ", ")
			}
		}
		rows = append(rows, []Cell{
			cell(c.ID),
			cell(c.Title),
			cell(packCoverageWord(c.Coverage)),
			pill(verdict, tone),
			cell(detail),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No controls", "The pack declared no controls.")
	}
	return table([]Column{
		{Label: "ID", Width: 1},
		{Label: "Control", Width: 3},
		{Label: "Coverage", Width: 1},
		{Label: "Verdict", Width: 1},
		{Label: "Reason codes", Width: 2},
	}, rows, "A passing check means the check passed, not that the control is met.",
		"Coverage says whether McHarbor checks the control, records the evidence for it, or cannot see it at all.")
}

func packEvidenceTable(f packFacts) Block {
	rows := make([][]Cell, 0, len(f.Pack.Framework.Evidence))
	for _, e := range f.Pack.Framework.Evidence {
		ref := e.Ref
		if e.Kind == "absent" {
			ref = "—"
		}
		rows = append(rows, []Cell{cell(e.Key), cell(ref), cell(e.Notes)})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No evidence sources", "The pack declared no evidence sources.")
	}
	return table([]Column{
		{Label: "Source", Width: 1},
		{Label: "Where", Width: 1},
		{Label: "What it is", Width: 3},
	}, rows, "Where each control's answer comes from.",
		"A source shown as a dash is one McHarbor does not have; the control is listed so the gap is visible.")
}

// packVerdictWord never says a control is met. "Pass" describes the check; the
// judgement about the control belongs to the reader.
func packVerdictWord(status string) string {
	switch status {
	case "pass":
		return "Check passed"
	case "warn":
		return "Attention"
	case "fail":
		return "Check failed"
	case "info":
		return "Informational"
	default:
		return "Not assessed"
	}
}

func packCoverageWord(coverage string) string {
	switch coverage {
	case "mechanical":
		return "Checked"
	case "evidenced":
		return "Evidenced"
	default:
		return "Not covered"
	}
}

func packTriggerWord(trigger string) string {
	if trigger == "scheduled" {
		return "scheduled sweep"
	}
	return "started by hand"
}

func packLead(f packFacts) string {
	if !f.Assessed {
		return "The compliance packs could not be read, so no control is assessed."
	}
	run := f.Pack.LatestRun
	if run == nil {
		return f.Pack.Framework.Name + " has never been run, so no check carries a verdict yet."
	}
	switch {
	case run.Summary.Fail > 0:
		return plural(run.Summary.Fail, "check fails", "checks fail") + " against " + f.Pack.Framework.Name +
			", and " + plural(f.Coverage.NotCovered, "control is", "controls are") + " outside what McHarbor can see."
	case run.Summary.Unknown > 0:
		return "No check fails, but " + plural(run.Summary.Unknown, "control was", "controls were") +
			" not assessed — which is not the same as passing."
	default:
		return "Every check that ran against " + f.Pack.Framework.Name + " passed. " +
			plural(f.Coverage.NotCovered, "control is", "controls are") + " outside what McHarbor can see."
	}
}

// runFanOut runs fn over ids with at most workers in flight, because each id is a
// loopback request and a pack with many frameworks should not fire them all at once.
func runFanOut(workers int, ids []string, fn func(string)) {
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	done := make(chan struct{}, len(ids))
	for _, id := range ids {
		sem <- struct{}{}
		go func(id string) {
			defer func() { <-sem; done <- struct{}{} }()
			fn(id)
		}(id)
	}
	for range ids {
		<-done
	}
}

// packKPIs are the page-one numbers. "Not assessed" is a first-class figure beside
// the passes, so a reader cannot take a high pass count as coverage.
func packKPIs(f packFacts) []KPI {
	if !f.Assessed || f.Pack.LatestRun == nil {
		return nil
	}
	run := f.Pack.LatestRun
	return []KPI{
		{Label: "Checks passed", Value: fmtInt(run.Summary.Pass), Status: StatusGood},
		{Label: "Checks failed", Value: fmtInt(run.Summary.Fail), Status: countStatus(run.Summary.Fail, 0, 0)},
		{Label: "Not assessed", Value: fmtInt(run.Summary.Unknown), Status: countStatus(run.Summary.Unknown, 0, 0)},
		{Label: "Not covered", Value: fmtInt(f.Coverage.NotCovered)},
	}
}

// packFindings raises a finding per failing and per unassessed check, and one for
// the not-covered set as a whole, so the coverage gap is a matter requiring
// attention rather than a footnote.
func packFindings(f packFacts) []Finding {
	if !f.Assessed {
		return nil
	}
	byID := map[string]packControlRow{}
	for _, c := range f.Pack.Framework.Controls {
		byID[c.ID] = c
	}
	out := []Finding{}
	for id, r := range f.Results {
		c := byID[id]
		switch r.Status {
		case "fail":
			out = append(out, Finding{
				Severity: SevHigh, Title: c.ID + " " + c.Title,
				Detail:         "The check failed. Reason codes: " + strings.Join(r.Reasons, ", ") + ".",
				Recommendation: c.Requirement,
				Evidence:       strings.Join(c.Evidence, ", "),
			})
		case "warn":
			out = append(out, Finding{
				Severity: SevMedium, Title: c.ID + " " + c.Title,
				Detail:         "The check needs attention. Reason codes: " + strings.Join(r.Reasons, ", ") + ".",
				Recommendation: c.Requirement,
				Evidence:       strings.Join(c.Evidence, ", "),
			})
		case "unknown":
			out = append(out, Finding{
				Severity: SevLow, Title: c.ID + " " + c.Title + " — not assessed",
				Detail: "The evidence for this check could not be gathered, so it carries no verdict. " +
					"Not assessed is not a pass.",
				Recommendation: "Confirm the environment is reachable and run the pack again.",
				Evidence:       strings.Join(c.Evidence, ", "),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	if f.Coverage.NotCovered > 0 {
		out = append(out, Finding{
			Severity: SevLow,
			Title:    plural(f.Coverage.NotCovered, "control", "controls") + " outside what McHarbor can see",
			Detail: "Host filesystem permissions, host auditing and organisational process controls cannot " +
				"be read from McHarbor. They are listed in the pack so the gap is visible.",
			Recommendation: "Evidence these from the host and from your own records.",
		})
	}
	return out
}
