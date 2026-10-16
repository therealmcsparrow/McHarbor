// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// resDriftRow is one confirmed diff from GET /api/reconciler/report.
type resDriftRow struct {
	DesiredStateID string `json:"desiredStateId"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	EnvID          string `json:"envId"`
	Class          string `json:"class"`
	Field          string `json:"field"`
	Expected       string `json:"expected"`
	Actual         string `json:"actual"`
	FirstSeenAt    string `json:"firstSeenAt"`
	CheckedAt      string `json:"checkedAt"`
}

// resDesiredRow is one watch from GET /api/reconciler. The free-form config
// blob is not decoded, because it can hold environment values.
type resDesiredRow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	EnvID          string `json:"envId"`
	Mode           string `json:"mode"`
	DriftDetected  bool   `json:"driftDetected"`
	LastReconcile  string `json:"lastReconcile"`
	LastEnforcedAt string `json:"lastEnforcedAt"`
	SourceID       string `json:"sourceId"`
	Source         string `json:"source"`
}

// resProposalRow is one plan from GET /api/reconciler/proposals. Steps are
// decoded without their recreate requests, which carry container environments.
type resProposalRow struct {
	ID             string `json:"id"`
	DesiredStateID string `json:"desiredStateId"`
	DesiredName    string `json:"desiredName"`
	Kind           string `json:"kind"`
	EnvID          string `json:"envId"`
	Origin         string `json:"origin"`
	Status         string `json:"status"`
	Steps          []struct {
		Action     string `json:"action"`
		Disruptive bool   `json:"disruptive"`
	} `json:"steps"`
	Results []struct {
		Action string `json:"action"`
		Target string `json:"target"`
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"results"`
	Error      string `json:"error"`
	ResolvedBy string `json:"resolvedBy"`
	ResolvedAt string `json:"resolvedAt"`
	CreatedAt  string `json:"createdAt"`
}

// resReconcilerSettings is GET /api/reconciler/settings: the instance-wide
// enforcement switch and the state of its circuit breaker.
type resReconcilerSettings struct {
	ConfirmPasses       int    `json:"confirmPasses"`
	EnforceEnabled      bool   `json:"enforceEnabled"`
	MaxActionsPerPass   int    `json:"maxActionsPerPass"`
	CooldownSeconds     int    `json:"cooldownSeconds"`
	FailureThreshold    int    `json:"failureThreshold"`
	BreakerOpen         bool   `json:"breakerOpen"`
	BreakerReason       string `json:"breakerReason"`
	BreakerOpenedAt     string `json:"breakerOpenedAt"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
}

// driftFacts is the computed view of the reconciler sources.
type driftFacts struct {
	Assessed         bool
	Rows             []resDriftRow
	StatesDrifting   int
	ByClass          map[string]int
	OldestDays       float64
	StaleDrift       int
	DesiredOK        bool
	Watched          int
	ByMode           map[string]int
	FromGit          int
	ProposalsOK      bool
	InPeriod         []resProposalRow
	ByStatus         map[string]int
	ByOrigin         map[string]map[string]int
	PendingNow       []resProposalRow
	PendingStale     int
	EnforceActions   int
	EnforceFailed    int
	ProposeFailed    int
	ApprovedByPerson int
	SettingsOK       bool
}

// Thresholds, in days, past which drift and waiting proposals are called out.
const (
	driftStaleDays    = 7
	proposalStaleDays = 3
)

func analyzeDrift(snap *Snapshot) driftFacts {
	r := snap.Resilience
	f := driftFacts{
		Assessed: snap.available(srcResDrift), DesiredOK: snap.available(srcResDesired),
		ProposalsOK: snap.available(srcResProposals), SettingsOK: snap.available(srcResReconCfg),
		ByClass: map[string]int{}, ByMode: map[string]int{}, ByStatus: map[string]int{},
		ByOrigin: map[string]map[string]int{}, OldestDays: -1,
	}
	if f.Assessed {
		f.Rows = r.Drift
		states := map[string]bool{}
		for _, d := range r.Drift {
			states[d.DesiredStateID] = true
			f.ByClass[d.Class]++
			age := resAgeDays(snap, d.FirstSeenAt)
			if age > f.OldestDays {
				f.OldestDays = age
			}
			if age > driftStaleDays {
				f.StaleDrift++
			}
		}
		f.StatesDrifting = len(states)
	}
	if f.DesiredOK {
		f.Watched = len(r.Desired)
		for _, d := range r.Desired {
			mode := d.Mode
			if mode == "" {
				mode = "observe"
			}
			f.ByMode[mode]++
			if d.SourceID != "" {
				f.FromGit++
			}
		}
	}
	if f.ProposalsOK {
		for _, p := range r.Proposals {
			if p.Status == "pending" {
				f.PendingNow = append(f.PendingNow, p)
				if resAgeDays(snap, p.CreatedAt) > proposalStaleDays {
					f.PendingStale++
				}
			}
			if !resInPeriod(snap, p.CreatedAt) {
				continue
			}
			f.InPeriod = append(f.InPeriod, p)
			f.ByStatus[p.Status]++
			if f.ByOrigin[p.Origin] == nil {
				f.ByOrigin[p.Origin] = map[string]int{}
			}
			f.ByOrigin[p.Origin][p.Status]++
			switch p.Origin {
			case "enforce":
				f.EnforceActions++
				if p.Status == "failed" {
					f.EnforceFailed++
				}
			default:
				if p.Status == "failed" {
					f.ProposeFailed++
				}
				if p.Status == "applied" && p.ResolvedBy != "" {
					f.ApprovedByPerson++
				}
			}
		}
	}
	return f
}

func driftClassLabel(class string) string {
	switch class {
	case "configuration":
		return "Configuration"
	case "state":
		return "Run state"
	case "existence":
		return "Existence"
	case "image":
		return "Image digest"
	}
	return resHumanise(class)
}

// driftValue withholds environment values, which the reconciler compares in
// full and which routinely carry credentials.
func driftValue(field, v string) string {
	if strings.HasPrefix(field, "env.") {
		return "(withheld)"
	}
	return orDash(v)
}

func buildDriftEnforcement(b *buildCtx) []Section {
	snap := b.snap
	f := analyzeDrift(snap)
	var out []Section

	st := section(Strategic, "Drift position")
	st.Blocks = append(st.Blocks, para(
		"A desired state is a declaration of how a workload should look. Drift is where the running "+
			"estate no longer matches it. This report counts the drift that is confirmed now, then follows "+
			"what the platform did about drift in the period: plans proposed to a person, and changes "+
			"enforcement made on its own."),
		kpis(driftKPIs(b)...))
	if !f.Assessed {
		st.Blocks = append(st.Blocks, resNotAssessed(snap, srcResDrift, "The drift report"))
	} else if len(f.ByClass) > 0 {
		var pts []ChartPoint
		for _, c := range topCounted(f.ByClass, 0) {
			pts = append(pts, ChartPoint{Label: driftClassLabel(c.Label), Value: float64(c.Count)})
		}
		st.Blocks = append(st.Blocks, chartBlock(Chart{Kind: ChartDonut, Title: "Confirmed drift by class",
			Unit: "diffs", Height: 130, ShowLegend: true, Series: []ChartSeries{{Points: pts}}}))
	}
	if fs := driftFindings(b); len(fs) > 0 {
		st.Blocks = append(st.Blocks, heading(2, "Findings"), findings(fs...))
	}
	out = append(out, st)

	ta := section(Tactical, "Response to drift")
	ta.Blocks = append(ta.Blocks, driftSettingsBlock(snap, f))
	if f.DesiredOK && f.Watched > 0 {
		rows := [][]Cell{}
		for _, mode := range []string{"observe", "propose", "enforce"} {
			rows = append(rows, []Cell{cell(titleCase(mode)), cell(fmtInt(f.ByMode[mode])), cell(driftModeMeaning(mode))})
		}
		ta.Blocks = append(ta.Blocks, heading(2, "Watches by mode"), table(
			[]Column{{Label: "Mode", Width: 1.2}, {Label: "Watches", Width: 1, Align: "right"}, {Label: "What happens on drift", Width: 5}},
			rows, "", plural(f.FromGit, "watch is", "watches are")+" owned by a Git source."))
	} else if !f.DesiredOK {
		ta.Blocks = append(ta.Blocks, resNotAssessed(snap, srcResDesired, "The list of desired states"))
	}
	if !f.ProposalsOK {
		ta.Blocks = append(ta.Blocks, resNotAssessed(snap, srcResProposals, "The proposal history"))
	} else if len(f.InPeriod) == 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "No plans in the period",
			"No proposal was written and enforcement did not act in the period."))
	} else {
		ta.Blocks = append(ta.Blocks, heading(2, "Plans in the period"), driftOriginTable(f),
			resCountedChart(ChartBar, "Plans by outcome", driftStatusCounts(f), ""))
	}
	out = append(out, ta)

	op := section(Operational, "Drift and plan register")
	if f.Assessed {
		op.Blocks = append(op.Blocks, driftRegister(snap, f))
	}
	if f.ProposalsOK && len(f.InPeriod) > 0 {
		op.Blocks = append(op.Blocks, driftProposalTable(f))
		op.Blocks = append(op.Blocks, driftFailureDetail(f)...)
	}
	out = append(out, op)
	return out
}

func driftModeMeaning(mode string) string {
	switch mode {
	case "observe":
		return "Drift is recorded and reported; nothing is changed."
	case "propose":
		return "A plan is written for a person to approve or reject."
	case "enforce":
		return "The platform restores the declared state itself, within the enforcement limits."
	}
	return ""
}

func driftSettingsBlock(snap *Snapshot, f driftFacts) Block {
	if !f.SettingsOK {
		return resNotAssessed(snap, srcResReconCfg, "The reconciler settings")
	}
	s := snap.Resilience.ReconSetting
	enforce := "Off — no watch is changed without a person"
	if s.EnforceEnabled {
		enforce = "On"
	}
	breaker := "Closed"
	if s.BreakerOpen {
		breaker = "Open since " + fmtDateTime(parseTS(s.BreakerOpenedAt))
		if s.BreakerReason != "" {
			breaker += " — " + s.BreakerReason
		}
	}
	return keyvalue(
		Pair{K: "Enforcement", V: enforce},
		Pair{K: "Circuit breaker", V: breaker},
		Pair{K: "Consecutive failures", V: fmtInt(s.ConsecutiveFailures) + " of " + fmtInt(s.FailureThreshold) + " allowed"},
		Pair{K: "Actions per pass", V: fmtInt(s.MaxActionsPerPass)},
		Pair{K: "Cooldown per watch", V: fmtDuration(float64(s.CooldownSeconds) / 60)},
		Pair{K: "Passes to confirm drift", V: fmtInt(s.ConfirmPasses)},
	)
}

var driftStatusOrder = []string{"applied", "failed", "rejected", "pending", "applying", "superseded"}

func driftStatusCounts(f driftFacts) []Counted {
	out := make([]Counted, 0, len(driftStatusOrder))
	for _, s := range driftStatusOrder {
		out = append(out, Counted{Label: titleCase(s), Count: f.ByStatus[s]})
	}
	return out
}

func driftOriginTable(f driftFacts) Block {
	origins := make([]string, 0, len(f.ByOrigin))
	for o := range f.ByOrigin {
		origins = append(origins, o)
	}
	sort.Strings(origins)
	rows := make([][]Cell, 0, len(origins))
	for _, o := range origins {
		m := f.ByOrigin[o]
		total := 0
		for _, n := range m {
			total += n
		}
		label := "Proposed for review"
		if o == "enforce" {
			label = "Enforced automatically"
		}
		rows = append(rows, []Cell{
			cell(label), cell(fmtInt(total)), cellStatus(fmtInt(m["applied"]), StatusGood),
			cellStatus(fmtInt(m["failed"]), countStatus(m["failed"], 0, 0)),
			cell(fmtInt(m["rejected"])), cell(fmtInt(m["pending"])), cell(fmtInt(m["superseded"])),
		})
	}
	return table([]Column{
		{Label: "Origin", Width: 2.4}, {Label: "Plans", Width: 0.9, Align: "right"},
		{Label: "Applied", Width: 0.9, Align: "right"}, {Label: "Failed", Width: 0.9, Align: "right"},
		{Label: "Rejected", Width: 0.9, Align: "right"}, {Label: "Pending", Width: 0.9, Align: "right"},
		{Label: "Superseded", Width: 1, Align: "right"},
	}, rows, "", "Superseded plans were replaced by a newer plan for the same drift before anyone acted.")
}

func driftRegister(snap *Snapshot, f driftFacts) Block {
	if len(f.Rows) == 0 {
		return callout(StatusGood, "No confirmed drift", "Every watched workload matches its declared state.")
	}
	rows := make([][]Cell, 0, len(f.Rows))
	for _, d := range f.Rows {
		age := resAgeDays(snap, d.FirstSeenAt)
		rows = append(rows, []Cell{
			cell(orDash(d.Name)), cell(driftClassLabel(d.Class)), mono(d.Field),
			mono(driftValue(d.Field, d.Expected)), mono(driftValue(d.Field, d.Actual)),
			cellStatus(fmtDays(age), freshnessStatus(age, 1, driftStaleDays)),
		})
	}
	return table([]Column{
		{Label: "Watch", Width: 2}, {Label: "Class", Width: 1.3}, {Label: "Field", Width: 1.8},
		{Label: "Declared", Width: 1.8}, {Label: "Running", Width: 1.8}, {Label: "First seen", Width: 1.3},
	}, rows, "Confirmed drift, oldest first", "Environment values are withheld from this report.")
}

func driftProposalTable(f driftFacts) Block {
	list := append([]resProposalRow(nil), f.InPeriod...)
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
	rows := make([][]Cell, 0, len(list))
	for _, p := range list {
		disruptive := 0
		for _, s := range p.Steps {
			if s.Disruptive {
				disruptive++
			}
		}
		steps := fmtInt(len(p.Steps))
		if disruptive > 0 {
			steps += " (" + fmtInt(disruptive) + " disruptive)"
		}
		origin := "Proposed"
		if p.Origin == "enforce" {
			origin = "Enforced"
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(p.CreatedAt))), cell(orDash(p.DesiredName)), cell(origin),
			pill(titleCase(p.Status), driftProposalStatus(p.Status)), cell(steps), cell(orDash(p.ResolvedBy)),
		})
	}
	return table([]Column{
		{Label: "Created", Width: 1.8}, {Label: "Watch", Width: 2}, {Label: "Origin", Width: 1.1},
		{Label: "Outcome", Width: 1.2}, {Label: "Steps", Width: 1.4}, {Label: "Resolved by", Width: 1.5},
	}, rows, "Plans written in the period", "")
}

func driftProposalStatus(s string) Status {
	switch s {
	case "applied":
		return StatusGood
	case "failed":
		return StatusBad
	case "pending", "applying":
		return StatusWarn
	}
	return StatusMuted
}

// driftFailureDetail prints each failed plan's message in full; a truncated
// error in a table cell hides the sentence that names the cause.
func driftFailureDetail(f driftFacts) []Block {
	var items []string
	for _, p := range f.InPeriod {
		if p.Status != "failed" {
			continue
		}
		msg := p.Error
		for _, r := range p.Results {
			if r.Status == "failed" && r.Error != "" {
				msg = strings.TrimSpace(msg + " " + r.Action + " " + r.Target + ": " + r.Error)
			}
		}
		items = append(items, orDash(p.DesiredName)+" ("+fmtDateTime(parseTS(p.CreatedAt))+") — "+orDash(msg))
	}
	if len(items) == 0 {
		return nil
	}
	return []Block{heading(2, "Why plans failed"), bullets(items...)}
}

func driftKPIs(b *buildCtx) []KPI {
	f := analyzeDrift(b.snap)
	var out []KPI
	if f.DesiredOK {
		out = append(out, KPI{Label: "Watches", Value: fmtInt(f.Watched), Hint: fmtInt(f.ByMode["enforce"]) + " enforced"})
	} else {
		out = append(out, resNAKPI("Watches"))
	}
	if f.Assessed {
		out = append(out,
			KPI{Label: "Drifting now", Value: fmtInt(f.StatesDrifting), Status: countStatus(f.StatesDrifting, 0, 2),
				Hint: plural(len(f.Rows), "diff", "diffs")},
			KPI{Label: "Oldest drift", Value: fmtDays(f.OldestDays), Status: driftAgeStatus(f)})
	} else {
		out = append(out, resNAKPI("Drifting now"), resNAKPI("Oldest drift"))
	}
	if f.ProposalsOK {
		out = append(out,
			KPI{Label: "Awaiting approval", Value: fmtInt(len(f.PendingNow)), Status: countStatus(f.PendingStale, 0, 0),
				Hint: fmtInt(f.PendingStale) + " older than " + fmtInt(proposalStaleDays) + " days"},
			KPI{Label: "Enforcement actions", Value: fmtInt(f.EnforceActions), Status: countStatus(f.EnforceFailed, 0, 0),
				Hint: fmtInt(f.EnforceFailed) + " failed"})
	} else {
		out = append(out, resNAKPI("Awaiting approval"), resNAKPI("Enforcement actions"))
	}
	if f.SettingsOK {
		s := b.snap.Resilience.ReconSetting
		k := KPI{Label: "Enforcement", Value: "Off", Status: StatusInfo}
		switch {
		case s.BreakerOpen:
			k = KPI{Label: "Enforcement", Value: "Breaker open", Status: StatusBad}
		case s.EnforceEnabled:
			k = KPI{Label: "Enforcement", Value: "On", Status: StatusGood}
		}
		out = append(out, k)
	}
	return out
}

func driftAgeStatus(f driftFacts) Status {
	if len(f.Rows) == 0 {
		return StatusGood
	}
	return invThresholdStatus(f.OldestDays, 1, driftStaleDays)
}

func driftFindings(b *buildCtx) []Finding {
	f := analyzeDrift(b.snap)
	s := b.snap.Resilience.ReconSetting
	var out []Finding
	if f.SettingsOK && s.BreakerOpen {
		detail := "Enforcement stopped itself after " + plural(s.ConsecutiveFailures, "consecutive failure", "consecutive failures") + "."
		if s.BreakerReason != "" {
			detail += " Reason recorded: " + s.BreakerReason + "."
		}
		out = append(out, Finding{
			Severity: SevHigh, Title: "The enforcement circuit breaker is open",
			Detail:         detail + " No watch in enforce mode is being corrected until it is reset.",
			Recommendation: "Read the failed enforcement plans, fix the cause, then reset the breaker.",
			Owner:          "Platform engineering",
		})
	}
	if f.EnforceFailed > 0 {
		out = append(out, Finding{
			Severity: SevHigh, Title: plural(f.EnforceFailed, "enforcement action failed", "enforcement actions failed"),
			Detail:         "The platform tried to restore a declared state on its own and could not; the workload may be part-changed.",
			Recommendation: "Review each failed plan and the workload it touched before the next pass retries.",
			Owner:          "Platform engineering",
		})
	}
	if f.SettingsOK && f.DesiredOK && !s.EnforceEnabled && f.ByMode["enforce"] > 0 {
		out = append(out, Finding{
			Severity: SevMedium,
			Title:    plural(f.ByMode["enforce"], "watch is set to enforce, but enforcement is off", "watches are set to enforce, but enforcement is off"),
			Detail:   "The instance-wide switch overrides the per-watch mode, so these watches only observe.",
			Recommendation: "Either switch enforcement on under Reconciler settings, or lower these watches to propose " +
				"so their mode says what actually happens.",
			Owner: "Platform engineering",
		})
	}
	if f.StaleDrift > 0 {
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(f.StaleDrift, "drift has been unresolved for over a week", "drifts have been unresolved for over a week"),
			Detail:         "Long-standing drift is either an unapproved change that became permanent or a declaration nobody maintains.",
			Recommendation: "Decide for each one whether the declaration or the workload is right, and correct the other.",
			Owner:          "Service owner",
		})
	}
	if f.PendingStale > 0 {
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(f.PendingStale, "plan has waited over three days for approval", "plans have waited over three days for approval"),
			Detail:         "A proposal nobody reviews leaves drift in place while looking handled.",
			Recommendation: "Approve or reject each pending plan, and name who reviews proposals.",
			Owner:          "Service owner",
		})
	}
	if f.ProposeFailed > 0 {
		out = append(out, Finding{
			Severity: SevMedium, Title: plural(f.ProposeFailed, "approved plan failed to apply", "approved plans failed to apply"),
			Detail:         "A person approved the plan, but applying it failed.",
			Recommendation: "Read the failure detail in the operational section and re-plan once the cause is fixed.",
			Owner:          "Platform engineering",
		})
	}
	return out
}

func driftLead(b *buildCtx) string {
	f := analyzeDrift(b.snap)
	if !f.Assessed {
		return "The drift report could not be read, so the current drift position is unknown."
	}
	if f.StatesDrifting == 0 {
		return "Every watched workload matches its declared state."
	}
	s := plural(f.StatesDrifting, "watch is", "watches are") + " drifting from declared state, the oldest for " +
		fmtDuration(f.OldestDays*1440)
	if f.ProposalsOK {
		s += "; enforcement acted " + plural(f.EnforceActions, "time", "times") + " in the period"
	}
	return s + "."
}
