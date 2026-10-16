// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// resIncidentRow is one incident from GET /api/incidents. Evidence, the model
// prompt and the timeline are left out: the list does not carry them and a
// report has no business reproducing a prompt that may quote container logs.
type resIncidentRow struct {
	ID              string                `json:"id"`
	EnvID           string                `json:"envId"`
	SubjectType     string                `json:"subjectType"`
	SubjectName     string                `json:"subjectName"`
	Source          string                `json:"source"`
	StackName       string                `json:"stackName"`
	Signal          string                `json:"signal"`
	Status          string                `json:"status"`
	SignalCount     int                   `json:"signalCount"`
	FirstSignalAt   string                `json:"firstSignalAt"`
	LastSignalAt    string                `json:"lastSignalAt"`
	AlertEventID    string                `json:"alertEventId"`
	Narrative       *resIncidentNarrative `json:"narrative"`
	NarrativeStatus string                `json:"narrativeStatus"`
	NarratedAt      string                `json:"narratedAt"`
	Summary         string                `json:"summary"`
	ResolvedAt      string                `json:"resolvedAt"`
	CreatedAt       string                `json:"createdAt"`
}

type resIncidentNarrative struct {
	Summary      string             `json:"summary"`
	Causes       []resIncidentCause `json:"causes"`
	Confidence   string             `json:"confidence"`
	Undetermined bool               `json:"undetermined"`
}

type resIncidentCause struct {
	Title      string `json:"title"`
	Confidence string `json:"confidence"`
}

// resIncidentFindingRow is one behavioural finding from GET /api/incidents/findings.
type resIncidentFindingRow struct {
	ID          string             `json:"id"`
	EnvID       string             `json:"envId"`
	SubjectType string             `json:"subjectType"`
	SubjectName string             `json:"subjectName"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status"`
	Metrics     map[string]float64 `json:"metrics"`
	FirstSeenAt string             `json:"firstSeenAt"`
	LastSeenAt  string             `json:"lastSeenAt"`
}

// resIncidentSourceRow is a plugin that raises incidents, read only for the
// labels of the signals the host itself cannot name.
type resIncidentSourceRow struct {
	Source       string `json:"source"`
	SubjectTypes []struct {
		Type    string `json:"type"`
		Label   string `json:"label"`
		Signals []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
		} `json:"signals"`
	} `json:"subjectTypes"`
}

// incidentFacts is the computed view of the incident sources for one period.
type incidentFacts struct {
	Assessed     bool
	Opened       []resIncidentRow
	ResolvedOf   int
	OpenNow      []resIncidentRow
	CarriedOver  int
	RecoveryMins []float64
	MTTR         float64
	MedianTTR    float64
	P90TTR       float64
	LongestTTR   float64
	ExplainMins  []float64
	Narrated     int
	Undetermined int
	Recurring    []Counted
	BySignal     map[string]int
	ByDay        map[string]int
	StaleOpen    int
	FindingsOpen []resIncidentFindingRow
}

// incidentStaleHours is how long an incident may stay open before the report
// calls it out; a day is past any reasonable acknowledgement window.
const incidentStaleHours = 24

func analyzeIncidents(snap *Snapshot) incidentFacts {
	r := snap.Resilience
	f := incidentFacts{Assessed: snap.available(srcResIncidents), BySignal: map[string]int{}, ByDay: map[string]int{}}
	if !f.Assessed {
		return f
	}
	perSubject := map[string]int{}
	for _, in := range r.Incidents {
		if in.Status == "open" {
			f.OpenNow = append(f.OpenNow, in)
			if !resInPeriod(snap, in.FirstSignalAt) {
				f.CarriedOver++
			}
			if resAgeDays(snap, in.FirstSignalAt)*24 > incidentStaleHours {
				f.StaleOpen++
			}
		}
		if in.ResolvedAt != "" && resInPeriod(snap, in.ResolvedAt) {
			if m, ok := resMinutesBetween(in.FirstSignalAt, in.ResolvedAt); ok {
				f.RecoveryMins = append(f.RecoveryMins, m)
			}
		}
		if !resInPeriod(snap, in.FirstSignalAt) {
			continue
		}
		f.Opened = append(f.Opened, in)
		if in.Status == "resolved" {
			f.ResolvedOf++
		}
		f.BySignal[in.Signal]++
		if d := resDayKey(in.FirstSignalAt); d != "" {
			f.ByDay[d]++
		}
		perSubject[incidentSubject(in)]++
		if in.NarrativeStatus == "ready" && in.Narrative != nil {
			f.Narrated++
			if in.Narrative.Undetermined {
				f.Undetermined++
			}
			if m, ok := resMinutesBetween(in.FirstSignalAt, in.NarratedAt); ok {
				f.ExplainMins = append(f.ExplainMins, m)
			}
		}
	}
	f.MTTR = mean(f.RecoveryMins)
	f.MedianTTR = median(f.RecoveryMins)
	f.P90TTR = resPercentile(f.RecoveryMins, 90)
	for _, m := range f.RecoveryMins {
		if m > f.LongestTTR {
			f.LongestTTR = m
		}
	}
	for _, c := range topCounted(perSubject, 0) {
		if c.Count >= 3 {
			f.Recurring = append(f.Recurring, c)
		}
	}
	sort.Slice(f.OpenNow, func(i, j int) bool { return f.OpenNow[i].FirstSignalAt < f.OpenNow[j].FirstSignalAt })
	if snap.available(srcResIncidentFindings) {
		for _, fd := range r.IncidentFindings {
			if fd.Status == "open" {
				f.FindingsOpen = append(f.FindingsOpen, fd)
			}
		}
	}
	return f
}

func incidentSubject(in resIncidentRow) string {
	name := in.SubjectName
	if name == "" {
		name = shortID(in.ID)
	}
	if in.StackName != "" {
		return in.StackName + " / " + name
	}
	return name
}

// incidentSignalLabel names a signal, preferring the host's own vocabulary and
// then the raising plugin's label, so a plugin signal never prints as a key.
func incidentSignalLabel(snap *Snapshot, signal string) string {
	switch signal {
	case "crash_loop":
		return "Crash loop"
	case "exit_nonzero":
		return "Repeated non-zero exit"
	case "oom":
		return "Out of memory"
	case "unhealthy":
		return "Health check failing"
	case "alert":
		return "Alert"
	}
	for _, s := range snap.Resilience.IncidentSources {
		for _, st := range s.SubjectTypes {
			for _, sig := range st.Signals {
				if sig.Key == signal && sig.Label != "" {
					return sig.Label
				}
			}
		}
	}
	return resHumanise(signal)
}

func incidentFindingKindLabel(kind string) string {
	switch kind {
	case "memory_climb":
		return "Memory climbing"
	case "host_memory_forecast":
		return "Host memory forecast to fill"
	case "scheduled_restarts":
		return "Restarts on a schedule"
	case "post_deploy_failures":
		return "Failures after deployment"
	}
	return resHumanise(kind)
}

func buildIncidentNarrative(b *buildCtx) []Section {
	snap := b.snap
	f := analyzeIncidents(snap)
	var out []Section

	st := section(Strategic, "Incident position")
	st.Blocks = append(st.Blocks, para(
		"An incident is one problem with one workload, from its first signal until it has been quiet "+
			"long enough to call resolved. This review counts what was opened in the period, how long "+
			"recovery took, and what the evidence says caused it."))
	if !f.Assessed {
		st.Blocks = append(st.Blocks, resNotAssessed(snap, srcResIncidents, "The incident register"))
		return append(out, st)
	}
	st.Blocks = append(st.Blocks, kpis(incidentKPIs(b)...))
	if len(f.BySignal) > 0 {
		var pts []ChartPoint
		for _, c := range topCounted(f.BySignal, 0) {
			pts = append(pts, ChartPoint{Label: incidentSignalLabel(snap, c.Label), Value: float64(c.Count)})
		}
		st.Blocks = append(st.Blocks, chartBlock(Chart{Kind: ChartDonut, Title: "Incidents by signal",
			Unit: "incidents", Height: 130, ShowLegend: true, Series: []ChartSeries{{Points: pts}}}))
	}
	if fs := incidentFindings(b); len(fs) > 0 {
		st.Blocks = append(st.Blocks, heading(2, "Findings"), findings(fs...))
	}
	out = append(out, st)

	ta := section(Tactical, "Recovery performance")
	if len(f.RecoveryMins) > 0 {
		ta.Blocks = append(ta.Blocks, keyvalue(
			Pair{K: "Incidents resolved in the period", V: fmtInt(len(f.RecoveryMins))},
			Pair{K: "Mean time to recover", V: fmtDuration(f.MTTR)},
			Pair{K: "Median time to recover", V: fmtDuration(f.MedianTTR)},
			Pair{K: "90th percentile", V: fmtDuration(f.P90TTR)},
			Pair{K: "Longest recovery", V: fmtDuration(f.LongestTTR)},
		))
	} else {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "No recoveries in the period",
			"No incident was resolved inside the period, so there is no recovery time to report."))
	}
	if len(f.Opened) > 0 {
		ta.Blocks = append(ta.Blocks, resCountedChart(ChartBar, "Incidents opened per day",
			seriesByDay(f.ByDay, snap.Period), "Quiet days are shown as zero."))
	}
	if len(f.ExplainMins) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Time to explanation"), para(
			"McHarbor has no separate acknowledgement step; the nearest measure of how quickly an "+
				"incident was understood is when its narrative was produced. "+
				plural(f.Narrated, "incident was", "incidents were")+" explained, a median of "+
				fmtDuration(median(f.ExplainMins))+" after the first signal"+
				incidentUndeterminedClause(f)+"."))
	}
	if len(f.Recurring) > 0 {
		rows := make([][]Cell, 0, len(f.Recurring))
		for _, c := range f.Recurring {
			rows = append(rows, []Cell{cell(c.Label), cellStatus(fmtInt(c.Count), StatusWarn)})
		}
		ta.Blocks = append(ta.Blocks, heading(2, "Recurring subjects"), table(
			[]Column{{Label: "Subject", Width: 4}, {Label: "Incidents", Width: 1, Align: "right"}},
			rows, "Subjects with three or more incidents",
			"A subject that keeps returning is a problem, not a run of incidents; it needs a root cause."))
	}
	if len(f.OpenNow) > 0 {
		rows := make([][]Cell, 0, len(f.OpenNow))
		for _, in := range f.OpenNow {
			age := resAgeDays(snap, in.FirstSignalAt)
			rows = append(rows, []Cell{
				cell(incidentSubject(in)), cell(incidentSignalLabel(snap, in.Signal)),
				cell(fmtDateTime(parseTS(in.FirstSignalAt))),
				cellStatus(fmtDuration(age*1440), invThresholdStatus(age*24, 4, incidentStaleHours)),
				cell(fmtInt(in.SignalCount)),
			})
		}
		ta.Blocks = append(ta.Blocks, heading(2, "Open incidents"), table(
			[]Column{{Label: "Subject", Width: 3}, {Label: "Signal", Width: 2}, {Label: "First signal", Width: 2},
				{Label: "Open for", Width: 1.2, Align: "right"}, {Label: "Signals", Width: 1, Align: "right"}},
			rows, "Oldest first", ""))
	}
	out = append(out, ta)

	op := section(Operational, "Incident register")
	op.Blocks = append(op.Blocks, incidentRegister(snap, f))
	if b.opt(OptIncludeDetail) {
		op.Blocks = append(op.Blocks, incidentNarratives(f)...)
	}
	if snap.available(srcResIncidentFindings) {
		op.Blocks = append(op.Blocks, heading(2, "Behavioural findings"), incidentFindingTable(snap))
	} else {
		op.Blocks = append(op.Blocks, resNotAssessed(snap, srcResIncidentFindings, "Behavioural findings"))
	}
	out = append(out, op)
	return out
}

func incidentUndeterminedClause(f incidentFacts) string {
	if f.Undetermined == 0 {
		return ""
	}
	return "; in " + plural(f.Undetermined, "case", "cases") + " the evidence did not determine a cause"
}

func incidentRegister(snap *Snapshot, f incidentFacts) Block {
	if len(f.Opened) == 0 {
		return callout(StatusGood, "No incidents opened", "No incident was opened in the period.")
	}
	list := append([]resIncidentRow(nil), f.Opened...)
	sort.Slice(list, func(i, j int) bool { return list[i].FirstSignalAt < list[j].FirstSignalAt })
	rows := make([][]Cell, 0, len(list))
	for _, in := range list {
		state := pill("Open", StatusWarn)
		dur := "—"
		if in.Status == "resolved" {
			state = pill("Resolved", StatusGood)
			if m, ok := resMinutesBetween(in.FirstSignalAt, in.ResolvedAt); ok {
				dur = fmtDuration(m)
			}
		}
		explained := cell("—")
		switch in.NarrativeStatus {
		case "ready":
			explained = cellStatus("Explained", StatusGood)
		case "failed", "refused":
			explained = cellStatus(titleCase(in.NarrativeStatus), StatusWarn)
		}
		rows = append(rows, []Cell{
			cell(incidentSubject(in)), cell(incidentSignalLabel(snap, in.Signal)),
			cell(fmtDateTime(parseTS(in.FirstSignalAt))), state, cell(dur),
			cell(fmtInt(in.SignalCount)), explained,
		})
	}
	return table([]Column{
		{Label: "Subject", Width: 2.6}, {Label: "Signal", Width: 1.8}, {Label: "Opened", Width: 1.8},
		{Label: "State", Width: 1.1}, {Label: "Recovery", Width: 1.1, Align: "right"},
		{Label: "Signals", Width: 0.9, Align: "right"}, {Label: "Narrative", Width: 1.2},
	}, rows, "Incidents opened in the period", "")
}

// incidentNarrativeLimit caps how many narratives are reproduced, so a noisy
// period does not turn the operational section into a book.
const incidentNarrativeLimit = 15

func incidentNarratives(f incidentFacts) []Block {
	var blocks []Block
	n := 0
	for _, in := range f.Opened {
		if in.Narrative == nil || in.NarrativeStatus != "ready" || strings.TrimSpace(in.Narrative.Summary) == "" {
			continue
		}
		if n == 0 {
			blocks = append(blocks, heading(2, "What the evidence says"), para(
				"These summaries were written by a model from the redacted evidence bundle each incident "+
					"assembled. They are explanations, not measurements; the figures above are the platform's own."))
		}
		n++
		if n > incidentNarrativeLimit {
			blocks = append(blocks, para("Further narratives are available on each incident in McHarbor."))
			break
		}
		blocks = append(blocks, heading(3, incidentSubject(in)), para(in.Narrative.Summary))
		var causes []string
		for _, c := range in.Narrative.Causes {
			if c.Title == "" {
				continue
			}
			line := c.Title
			if c.Confidence != "" {
				line += " (" + c.Confidence + " confidence)"
			}
			causes = append(causes, line)
		}
		if len(causes) > 0 {
			blocks = append(blocks, bullets(causes...))
		}
	}
	return blocks
}

func incidentFindingTable(snap *Snapshot) Block {
	list := append([]resIncidentFindingRow(nil), snap.Resilience.IncidentFindings...)
	if len(list) == 0 {
		return para("The behaviour detector has recorded no patterns.")
	}
	sort.Slice(list, func(i, j int) bool { return list[i].LastSeenAt > list[j].LastSeenAt })
	rows := make([][]Cell, 0, len(list))
	for _, fd := range list {
		st := StatusMuted
		if fd.Status == "open" {
			st = StatusWarn
		}
		rows = append(rows, []Cell{
			cell(orDash(fd.SubjectName)), cell(incidentFindingKindLabel(fd.Kind)), pill(titleCase(fd.Status), st),
			cell(fmtDate(parseTS(fd.FirstSeenAt))), cell(fmtDate(parseTS(fd.LastSeenAt))),
		})
	}
	return table([]Column{
		{Label: "Subject", Width: 2.4}, {Label: "Pattern", Width: 2.4}, {Label: "Status", Width: 1.1},
		{Label: "First seen", Width: 1.3}, {Label: "Last seen", Width: 1.3},
	}, rows, "Patterns found by the deterministic detector", "")
}

func incidentKPIs(b *buildCtx) []KPI {
	f := analyzeIncidents(b.snap)
	if !f.Assessed {
		return []KPI{resNAKPI("Incidents opened"), resNAKPI("Mean time to recover"), resNAKPI("Open now")}
	}
	mttr := KPI{Label: "Mean time to recover", Value: "—", Status: StatusMuted, Hint: "no recoveries in the period"}
	if len(f.RecoveryMins) > 0 {
		mttr = KPI{Label: "Mean time to recover", Value: fmtDuration(f.MTTR), Target: "< 4 h",
			Status: invThresholdStatus(f.MTTR, 60, 240), Hint: "median " + fmtDuration(f.MedianTTR)}
	}
	return []KPI{
		{Label: "Incidents opened", Value: fmtInt(len(f.Opened)), Hint: fmtInt(f.ResolvedOf) + " since resolved"},
		mttr,
		{Label: "Open now", Value: fmtInt(len(f.OpenNow)), Status: countStatus(len(f.OpenNow), 0, 2),
			Hint: fmtInt(f.CarriedOver) + " opened before the period"},
		{Label: "Open over a day", Value: fmtInt(f.StaleOpen), Status: countStatus(f.StaleOpen, 0, 0)},
		{Label: "Recurring subjects", Value: fmtInt(len(f.Recurring)), Status: countStatus(len(f.Recurring), 0, 1),
			Hint: "three or more incidents"},
		{Label: "Explained", Value: fmtPercent(pct(f.Narrated, len(f.Opened))), Hint: plural(f.Narrated, "narrative", "narratives")},
	}
}

func incidentFindings(b *buildCtx) []Finding {
	f := analyzeIncidents(b.snap)
	if !f.Assessed {
		return nil
	}
	var out []Finding
	if f.StaleOpen > 0 {
		out = append(out, Finding{
			Severity: SevHigh,
			Title:    plural(f.StaleOpen, "incident has been open for more than a day", "incidents have been open for more than a day"),
			Detail: "An incident closes itself once its subject has been quiet long enough, so one that stays open " +
				"is still producing signals. It is not recovering on its own.",
			Recommendation: "Assign an owner to each long-running incident and read its evidence bundle.",
			Owner:          "Service owner",
		})
	}
	if len(f.RecoveryMins) > 0 && f.MTTR > 240 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          "Mean time to recover is " + fmtDuration(f.MTTR),
			Detail:         "Recovery averaged more than four hours across " + plural(len(f.RecoveryMins), "incident", "incidents") + ".",
			Recommendation: "Review the slowest recoveries for missing alerting, health checks or runbooks.",
			Owner:          "Platform engineering",
		})
	}
	if len(f.Recurring) > 0 {
		names := make([]string, 0, len(f.Recurring))
		for _, c := range f.Recurring {
			names = append(names, c.Label)
		}
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          plural(len(f.Recurring), "subject had three or more incidents", "subjects had three or more incidents"),
			Detail:         "Recurring: " + joinAnd(names) + ". Repeated incidents on one subject point to a problem that recovery keeps masking.",
			Recommendation: "Open a problem record for each recurring subject and fix the cause rather than the symptom.",
			Owner:          "Service owner",
		})
	}
	forecast := 0
	for _, fd := range f.FindingsOpen {
		if fd.Kind == "host_memory_forecast" {
			forecast++
		}
	}
	if forecast > 0 {
		out = append(out, Finding{
			Severity:       SevHigh,
			Title:          "Host memory is forecast to run out",
			Detail:         "The behaviour detector projects host memory to fill at the current rate of growth.",
			Recommendation: "Find the workload whose memory is climbing, and add capacity or limits before the forecast date.",
			Owner:          "Platform engineering",
		})
	}
	if other := len(f.FindingsOpen) - forecast; other > 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(other, "behavioural pattern is open", "behavioural patterns are open"),
			Detail:         "Patterns such as climbing memory or failures after deployment precede incidents.",
			Recommendation: "Review each pattern and dismiss the ones that are expected.",
			Owner:          "Service owner",
		})
	}
	if len(f.Opened) >= 3 && f.Narrated == 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          "No incident in the period was explained",
			Detail:         "Incidents carry an evidence bundle, but none was narrated, so causes are recorded nowhere.",
			Recommendation: "Opt the environment in to incident narration, or record causes on each incident by hand.",
			Owner:          "Platform engineering",
		})
	}
	return out
}

func incidentLead(b *buildCtx) string {
	f := analyzeIncidents(b.snap)
	if !f.Assessed {
		return "The incident register could not be read, so this review cannot say what failed."
	}
	if len(f.Opened) == 0 {
		if len(f.OpenNow) > 0 {
			return "No new incident was opened, but " + plural(len(f.OpenNow), "incident remains", "incidents remain") + " open from earlier."
		}
		return "No incident was opened in the period and none is open."
	}
	s := plural(len(f.Opened), "incident was", "incidents were") + " opened in the period"
	if len(f.RecoveryMins) > 0 {
		s += ", with a mean time to recover of " + fmtDuration(f.MTTR)
	}
	if len(f.OpenNow) > 0 {
		s += "; " + plural(len(f.OpenNow), "remains", "remain") + " open"
	}
	return s + "."
}
