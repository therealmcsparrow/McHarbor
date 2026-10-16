// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
)

// govLongElevationMinutes is where a "temporary" elevation stops being
// temporary: a grant longer than a working day outlives the task it was for.
const govLongElevationMinutes = 8 * 60

// approvalFacts is everything the change-approvals report derives, computed
// once per call so the builder, the KPIs and the findings cannot disagree.
type approvalFacts struct {
	Assessed  bool
	Truncated bool
	Requests  []govApprovalRow

	Approved, Denied, Pending, Cancelled int
	ExpiredUndecided, ExpiredUnexecuted  int
	Executed, ExecFailed                 int

	Turnaround       []float64
	MedianTurnaround float64
	P90Turnaround    float64
	ApprovalRate     float64
	LapseRate        float64

	SelfApproved []govApprovalRow
	ByCategory   map[string]*approvalCategoryStat

	RulesAssessed   bool
	RulesEnabled    int
	SingleApprover  int
	Uncovered       []string
	CategoriesKnown int

	ElevAssessed        bool
	Elevations          []govElevationRow
	ElevGranted         int
	ElevDenied          int
	ElevPending         int
	ElevGrantedMinutes  int
	ElevSelfApproved    []govElevationRow
	ElevNoJustification []govElevationRow
	ElevLong            []govElevationRow
	ElevActive          []govElevationRow
	ElevByPermission    map[string]int
}

type approvalCategoryStat struct {
	Requests, Approved, Denied, Lapsed int
	Turnaround                         []float64
}

func govApprovals(snap *Snapshot) approvalFacts {
	g := snap.Governance
	p := snap.Period
	f := approvalFacts{
		Assessed:         snap.available(srcApprovals),
		RulesAssessed:    snap.available(srcApprovalRules),
		ElevAssessed:     snap.available(srcElevations),
		ByCategory:       map[string]*approvalCategoryStat{},
		ElevByPermission: map[string]int{},
	}
	now := govNow(p)

	if f.Assessed {
		// The list is newest first; a full page whose oldest row is still in
		// the window means older in-period requests were not read.
		if n := len(g.Approvals); n >= govApprovalsPage && govInPeriod(g.Approvals[n-1].CreatedAt, p) {
			f.Truncated = true
		}
		for _, r := range g.Approvals {
			if !govInPeriod(r.CreatedAt, p) {
				continue
			}
			f.Requests = append(f.Requests, r)
			cat := f.ByCategory[r.Category]
			if cat == nil {
				cat = &approvalCategoryStat{}
				f.ByCategory[r.Category] = cat
			}
			cat.Requests++

			decided := govStr(r.DecidedAt) != ""
			switch r.Status {
			case "approved", "executing", "executed":
				f.Approved++
				cat.Approved++
				if r.Status == "executed" {
					f.Executed++
					if r.ResultStatus != nil && *r.ResultStatus >= 400 {
						f.ExecFailed++
					}
				}
			case "denied":
				f.Denied++
				cat.Denied++
			case "pending":
				f.Pending++
			case "cancelled":
				f.Cancelled++
			case "expired":
				cat.Lapsed++
				// The host expires both undecided requests and approved
				// ones nobody executed; a decision timestamp tells them apart.
				if decided {
					f.ExpiredUnexecuted++
				} else {
					f.ExpiredUndecided++
				}
			}
			if decided && (r.Status != "expired") {
				mins := parseTS(govStr(r.DecidedAt)).Sub(parseTS(r.CreatedAt)).Minutes()
				if mins >= 0 {
					f.Turnaround = append(f.Turnaround, mins)
					cat.Turnaround = append(cat.Turnaround, mins)
				}
			}
			for _, d := range r.Decisions {
				if d.Decision == "approve" && d.ApproverID != "" && d.ApproverID == r.RequesterID {
					f.SelfApproved = append(f.SelfApproved, r)
					break
				}
			}
		}
		f.MedianTurnaround = median(f.Turnaround)
		f.P90Turnaround = govPercentile(f.Turnaround, 90)
		f.ApprovalRate = pct(f.Approved, f.Approved+f.Denied)
		f.LapseRate = pct(f.ExpiredUndecided, f.Approved+f.Denied+f.ExpiredUndecided)
	}

	if f.RulesAssessed {
		covered := map[string]bool{}
		for _, r := range g.ApprovalRules {
			if !r.Enabled {
				continue
			}
			f.RulesEnabled++
			covered[r.Category] = true
			if r.RequiredApprovers <= 1 {
				f.SingleApprover++
			}
		}
		if snap.available(srcApprovalCategories) {
			f.CategoriesKnown = len(g.ApprovalCategories)
			for _, c := range g.ApprovalCategories {
				if !covered[c.Key] {
					f.Uncovered = append(f.Uncovered, c.Key)
				}
			}
			sort.Strings(f.Uncovered)
		}
	}

	if f.ElevAssessed {
		for _, e := range g.Elevations {
			if !govInPeriod(e.CreatedAt, p) {
				continue
			}
			f.Elevations = append(f.Elevations, e)
			granted := e.Status == "approved" || (e.Status == "expired" && govStr(e.ApproverID) != "")
			switch {
			case granted:
				f.ElevGranted++
				f.ElevGrantedMinutes += e.RequestedMinutes
				f.ElevByPermission[e.PermissionKey]++
				if govStr(e.ApproverID) == e.RequesterID && e.RequesterID != "" {
					f.ElevSelfApproved = append(f.ElevSelfApproved, e)
				}
				if e.RequestedMinutes > govLongElevationMinutes {
					f.ElevLong = append(f.ElevLong, e)
				}
				if len([]rune(strings.TrimSpace(e.Justification))) < 10 {
					f.ElevNoJustification = append(f.ElevNoJustification, e)
				}
				if until := parseTS(govStr(e.ApprovedUntil)); e.Status == "approved" && until.After(now) {
					f.ElevActive = append(f.ElevActive, e)
				}
			case e.Status == "denied":
				f.ElevDenied++
			case e.Status == "pending":
				f.ElevPending++
			}
		}
	}
	return f
}

func approvalKPIs(f approvalFacts) []KPI {
	out := []KPI{}
	if f.Assessed {
		out = append(out,
			KPI{Label: "Held changes", Value: fmtInt(len(f.Requests)), Hint: "requests raised in the period"},
			KPI{Label: "Approval rate", Value: govPercentOrDash(f.ApprovalRate, f.Approved+f.Denied),
				Hint: "of decided requests"},
			KPI{Label: "Median turnaround", Value: govMinutesOrDash(f.MedianTurnaround, len(f.Turnaround)),
				Target: "< 60 min", Status: govTurnaroundStatus(f.MedianTurnaround, len(f.Turnaround))},
			KPI{Label: "Lapsed undecided", Value: fmtInt(f.ExpiredUndecided),
				Status: countStatus(f.ExpiredUndecided, 0, 2), Hint: "expired with no decision"},
		)
	}
	if f.ElevAssessed {
		out = append(out,
			KPI{Label: "Elevations granted", Value: fmtInt(f.ElevGranted),
				Hint: fmtDuration(float64(f.ElevGrantedMinutes)) + " of privilege"},
			KPI{Label: "Active now", Value: fmtInt(len(f.ElevActive)),
				Status: countStatus(len(f.ElevActive), 0, 2)},
		)
	}
	return out
}

func approvalFindings(f approvalFacts) []Finding {
	var out []Finding
	if n := len(f.SelfApproved) + len(f.ElevSelfApproved); n > 0 {
		out = append(out, Finding{
			Severity: SevCritical,
			Title:    plural(n, "request was approved by its own requester", "requests were approved by their own requesters"),
			Detail: "The approval control exists to put a second person between an intention and a " +
				"destructive change. A self-approval means that person was absent, whatever the audit trail says.",
			Recommendation: "Review each self-approved change with its requester, and check which role let the requester decide their own request.",
			Owner:          "Security",
		})
	}
	if f.ExecFailed > 0 {
		out = append(out, Finding{
			Severity: SevHigh,
			Title:    plural(f.ExecFailed, "approved change failed when it was executed", "approved changes failed when they were executed"),
			Detail: "The change was held, approved and replayed, and the replayed call returned an error. " +
				"The approver signed off on an outcome that did not happen.",
			Recommendation: "Open each failed request in Approvals and confirm whether the change needs to be raised again.",
			Owner:          "Change owner",
		})
	}
	if f.ExpiredUndecided > 0 && f.LapseRate >= 10 {
		out = append(out, Finding{
			Severity: SevMedium,
			Title:    fmtPercent(f.LapseRate) + " of requests lapsed without a decision",
			Detail: plural(f.ExpiredUndecided, "request", "requests") + " expired while waiting. Requesters whose " +
				"changes silently lapse learn to route around the control.",
			Recommendation: "Widen the approver pool for the busiest categories, or lengthen their expiry window.",
			Owner:          "Change manager",
		})
	}
	if len(f.Turnaround) > 0 && f.MedianTurnaround > 240 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          "Median approval turnaround is " + fmtDuration(f.MedianTurnaround),
			Detail:         "A held change waits for most of a working day. Slow approval is the usual reason a team asks for a control to be switched off.",
			Recommendation: "Agree a response target for approvers and add approvers to the categories that wait longest.",
			Owner:          "Change manager",
		})
	}
	if len(f.ElevLong) > 0 {
		out = append(out, Finding{
			Severity: SevMedium,
			Title:    plural(len(f.ElevLong), "elevation was granted for more than eight hours", "elevations were granted for more than eight hours"),
			Detail:   "A temporary grant that outlives the working day is a standing privilege with an expiry date.",
			Recommendation: "Cap elevation length for sensitive permissions, or grant the permission through a role " +
				"if the need is genuinely ongoing.",
			Owner: "Security",
		})
	}
	if len(f.ElevNoJustification) > 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(len(f.ElevNoJustification), "elevation was granted without a meaningful justification", "elevations were granted without a meaningful justification"),
			Detail:         "The stated purpose is what an auditor compares the elevated activity against. Without one there is nothing to compare.",
			Recommendation: "Ask approvers to deny elevation requests that do not say what the privilege is for.",
			Owner:          "Security",
		})
	}
	if f.RulesAssessed && len(f.Uncovered) > 0 {
		out = append(out, Finding{
			Severity: SevLow,
			Title:    plural(len(f.Uncovered), "risk category has no approval rule", "risk categories have no approval rule"),
			Detail:   "Uncovered: " + joinAnd(f.Uncovered) + ". Changes in these categories go through immediately.",
			Recommendation: "Decide deliberately whether each category needs a second pair of eyes, and record " +
				"the decision either way.",
			Owner: "Change manager",
		})
	}
	return out
}

func approvalLead(f approvalFacts) string {
	switch {
	case !f.Assessed && !f.ElevAssessed:
		return "Neither the approval queue nor the elevation log could be read, so change control is not assessed."
	case len(f.SelfApproved)+len(f.ElevSelfApproved) > 0:
		return "At least one request was approved by the person who raised it, which defeats the control."
	case f.Assessed && len(f.Requests) == 0 && f.ElevGranted == 0:
		return "No change was held for approval and no elevation was granted in this period."
	case f.Assessed:
		return plural(len(f.Requests), "change was", "changes were") + " held for approval, with a median turnaround of " +
			govMinutesOrDash(f.MedianTurnaround, len(f.Turnaround)) + ", and " +
			plural(f.ElevGranted, "temporary elevation was", "temporary elevations were") + " granted."
	default:
		return plural(f.ElevGranted, "temporary elevation was", "temporary elevations were") + " granted in this period."
	}
}

func buildChangeApprovals(b *buildCtx) []Section {
	snap := b.snap
	f := govApprovals(snap)
	var out []Section

	st := section(Strategic, "Change control at a glance")
	st.Blocks = append(st.Blocks,
		para("An approval rule holds a risky change until someone other than its author agrees to it; "+
			"an elevation lends a permission for a short, stated purpose. Both are only controls if they "+
			"are used, decided promptly and never decided by the person who asked."))
	if !f.Assessed {
		st.Blocks = append(st.Blocks, govNotAssessed(snap, srcApprovals, "The approval queue"))
	} else {
		st.Blocks = append(st.Blocks, kpis(
			KPI{Label: "Approved", Value: fmtInt(f.Approved), Status: StatusGood},
			KPI{Label: "Denied", Value: fmtInt(f.Denied)},
			KPI{Label: "Lapsed", Value: fmtInt(f.ExpiredUndecided + f.ExpiredUnexecuted),
				Status: countStatus(f.ExpiredUndecided+f.ExpiredUnexecuted, 0, 2)},
			KPI{Label: "Self-approved", Value: fmtInt(len(f.SelfApproved)),
				Status: countStatus(len(f.SelfApproved), 0, 0)},
		))
		if f.Truncated {
			st.Blocks = append(st.Blocks, callout(StatusInfo, "Partial read",
				"The approval queue holds more requests than one read returns, so the oldest part of "+
					"this period is not counted. Shorten the period for a complete figure."))
		}
	}
	if f.RulesAssessed {
		st.Blocks = append(st.Blocks, keyvalue(
			Pair{K: "Enabled approval rules", V: fmtInt(f.RulesEnabled)},
			Pair{K: "Rules needing a single approver", V: fmtInt(f.SingleApprover)},
			Pair{K: "Risk categories without a rule", V: govCountOf(len(f.Uncovered), f.CategoriesKnown)},
		))
	}
	out = append(out, st)

	ta := section(Tactical, "Approval flow")
	if f.Assessed && len(f.Requests) > 0 {
		ta.Blocks = append(ta.Blocks,
			approvalOutcomeChart(f),
			approvalCategoryTable(f),
			chartBlock(Chart{
				Kind: ChartBar, Title: "Requests raised per day", Height: 110,
				Series:  []ChartSeries{{Name: "Requests", Points: govDaySeries(govApprovalStamps(f.Requests), snap.Period)}},
				Caption: "A steady trickle is normal. A burst usually follows a rule being enabled or a release.",
			}),
			approvalRequesterTable(f))
	} else if f.Assessed {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "No held changes",
			"No request was held for approval in this period. If rules are enabled, that means nobody "+
				"attempted a change they cover."))
	}
	if f.RulesAssessed {
		ta.Blocks = append(ta.Blocks, heading(2, "Approval rules"), approvalRuleTable(snap.Governance.ApprovalRules))
	} else {
		ta.Blocks = append(ta.Blocks, govNotAssessed(snap, srcApprovalRules, "The approval rules"))
	}
	ta.Blocks = append(ta.Blocks, heading(2, "Temporary elevations"))
	if !f.ElevAssessed {
		ta.Blocks = append(ta.Blocks, govNotAssessed(snap, srcElevations, "The elevation log"))
	} else {
		ta.Blocks = append(ta.Blocks,
			kpis(
				KPI{Label: "Requested", Value: fmtInt(len(f.Elevations))},
				KPI{Label: "Granted", Value: fmtInt(f.ElevGranted)},
				KPI{Label: "Denied", Value: fmtInt(f.ElevDenied)},
				KPI{Label: "Longer than 8 h", Value: fmtInt(len(f.ElevLong)),
					Status: countStatus(len(f.ElevLong), 0, 1)},
			),
			elevationPermissionTable(f))
	}
	out = append(out, ta)

	op := section(Operational, "Request and elevation log")
	if f.Assessed {
		op.Blocks = append(op.Blocks, approvalRequestTable(f.Requests))
	}
	if f.ElevAssessed {
		op.Blocks = append(op.Blocks, elevationTable(f.Elevations))
	}
	out = append(out, op)
	return out
}

func approvalOutcomeChart(f approvalFacts) Block {
	pts := []ChartPoint{
		{Label: "Approved", Value: float64(f.Approved), Status: StatusGood},
		{Label: "Denied", Value: float64(f.Denied), Status: StatusWarn},
		{Label: "Lapsed", Value: float64(f.ExpiredUndecided + f.ExpiredUnexecuted), Status: StatusBad},
		{Label: "Pending", Value: float64(f.Pending), Status: StatusInfo},
		{Label: "Cancelled", Value: float64(f.Cancelled), Status: StatusMuted},
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Outcome of held changes", Unit: "requests", Height: 130,
		Series: []ChartSeries{{Points: pts}},
	})
}

func approvalCategoryTable(f approvalFacts) Block {
	keys := make([]string, 0, len(f.ByCategory))
	for k := range f.ByCategory {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := f.ByCategory[keys[i]], f.ByCategory[keys[j]]
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return keys[i] < keys[j]
	})
	rows := make([][]Cell, 0, len(keys))
	for _, k := range keys {
		c := f.ByCategory[k]
		med := median(c.Turnaround)
		rows = append(rows, []Cell{
			mono(orDash(k)),
			cell(fmtInt(c.Requests)),
			cell(fmtInt(c.Approved)),
			cell(fmtInt(c.Denied)),
			cellStatus(fmtInt(c.Lapsed), countStatus(c.Lapsed, 0, 1)),
			cellStatus(govMinutesOrDash(med, len(c.Turnaround)), govTurnaroundStatus(med, len(c.Turnaround))),
		})
	}
	return table([]Column{
		{Label: "Risk category", Width: 2.2},
		{Label: "Requests", Width: 1, Align: "right"},
		{Label: "Approved", Width: 1, Align: "right"},
		{Label: "Denied", Width: 1, Align: "right"},
		{Label: "Lapsed", Width: 1, Align: "right"},
		{Label: "Median turnaround", Width: 1.4, Align: "right"},
	}, rows, "Held changes by risk category",
		"Turnaround runs from the request being held to the decision that settled it.")
}

func approvalRequesterTable(f approvalFacts) Block {
	counts := map[string]int{}
	for _, r := range f.Requests {
		counts[orDash(r.RequesterName, shortID(r.RequesterID))]++
	}
	ranked := topCounted(counts, 10)
	rows := make([][]Cell, 0, len(ranked))
	for _, c := range ranked {
		rows = append(rows, []Cell{cell(c.Label), cell(fmtInt(c.Count))})
	}
	return table([]Column{{Label: "Requester", Width: 3}, {Label: "Held changes", Width: 1.2, Align: "right"}},
		rows, "Who raises held changes", "")
}

func approvalRuleTable(rules []govApprovalRuleRow) Block {
	list := append([]govApprovalRuleRow(nil), rules...)
	sort.Slice(list, func(i, j int) bool { return list[i].Category < list[j].Category })
	rows := make([][]Cell, 0, len(list))
	for _, r := range list {
		state, st := "Enabled", StatusGood
		if !r.Enabled {
			state, st = "Disabled", StatusMuted
		}
		scope := "All environments"
		if govStr(r.EnvironmentID) != "" {
			scope = "One environment"
		}
		rows = append(rows, []Cell{
			mono(r.Category),
			cell(scope),
			cellStatus(fmtInt(r.RequiredApprovers), countStatus(2-r.RequiredApprovers, 0, 1)),
			mono(orDash(r.ApproverPermission)),
			cell(fmtDuration(float64(r.ExpiryMinutes))),
			pill(state, st),
		})
	}
	if len(rows) == 0 {
		return callout(StatusWarn, "No approval rules",
			"No approval rule exists, so every destructive change takes effect the moment it is made.")
	}
	return table([]Column{
		{Label: "Category", Width: 2},
		{Label: "Scope", Width: 1.5},
		{Label: "Approvers", Width: 1, Align: "right"},
		{Label: "Approver permission", Width: 2},
		{Label: "Expires after", Width: 1.2},
		{Label: "State", Width: 1},
	}, rows, "", "A rule needing one approver still separates author and approver; two is the usual bar for production.")
}

func elevationPermissionTable(f approvalFacts) Block {
	ranked := topCounted(f.ElevByPermission, 12)
	rows := make([][]Cell, 0, len(ranked))
	for _, c := range ranked {
		rows = append(rows, []Cell{mono(c.Label), cell(fmtInt(c.Count))})
	}
	if len(rows) == 0 {
		return para("No elevation was granted in this period.")
	}
	return table([]Column{{Label: "Permission lent", Width: 3}, {Label: "Grants", Width: 1, Align: "right"}},
		rows, "Permissions granted temporarily",
		"A permission that is elevated every week is a role assignment that has not been made.")
}

func approvalRequestTable(reqs []govApprovalRow) Block {
	rows := make([][]Cell, 0, len(reqs))
	for i, r := range reqs {
		if i >= 60 {
			break
		}
		turn := "—"
		if d := govStr(r.DecidedAt); d != "" {
			turn = fmtDuration(parseTS(d).Sub(parseTS(r.CreatedAt)).Minutes())
		}
		approvers := make([]string, 0, len(r.Decisions))
		for _, d := range r.Decisions {
			if d.Decision == "approve" {
				approvers = append(approvers, orDash(d.ApproverName, shortID(d.ApproverID)))
			}
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(r.CreatedAt))),
			mono(r.Category),
			cell(orDash(r.ResourceName, r.RoutePattern)),
			cell(orDash(r.RequesterName, shortID(r.RequesterID))),
			cell(orDash(strings.Join(approvers, ", "))),
			pill(titleCase(r.Status), govApprovalStatus(r)),
			cell(turn),
		})
	}
	if len(rows) == 0 {
		return divider()
	}
	note := ""
	if len(reqs) > 60 {
		note = "The 60 most recent of " + fmtInt(len(reqs)) + " requests are listed."
	}
	return table([]Column{
		{Label: "Raised", Width: 1.6},
		{Label: "Category", Width: 1.4},
		{Label: "Target", Width: 1.8},
		{Label: "Requester", Width: 1.3},
		{Label: "Approved by", Width: 1.5},
		{Label: "Outcome", Width: 1.1},
		{Label: "Turnaround", Width: 1, Align: "right"},
	}, rows, "Held changes", note)
}

func elevationTable(list []govElevationRow) Block {
	rows := make([][]Cell, 0, len(list))
	for i, e := range list {
		if i >= 60 {
			break
		}
		just := govCap(e.Justification, 90)
		justCell := cell(orDash(just))
		if len([]rune(strings.TrimSpace(e.Justification))) < 10 {
			justCell = cellStatus(orDash(just), StatusWarn)
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(e.CreatedAt))),
			mono(e.PermissionKey),
			mono(shortID(e.RequesterID)),
			cellStatus(fmtDuration(float64(e.RequestedMinutes)),
				invThresholdStatus(float64(e.RequestedMinutes), 240, govLongElevationMinutes)),
			justCell,
			pill(titleCase(e.Status), govElevationStatus(e)),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No elevations", "Nobody asked for a temporary permission in this period.")
	}
	return table([]Column{
		{Label: "Requested", Width: 1.6},
		{Label: "Permission", Width: 1.8},
		{Label: "Requester", Width: 1.1},
		{Label: "Length", Width: 0.9, Align: "right"},
		{Label: "Justification", Width: 2.6},
		{Label: "State", Width: 1},
	}, rows, "Temporary elevations",
		"“Expired” on a granted elevation means it ran its course; the permission has been withdrawn.")
}

func govApprovalStatus(r govApprovalRow) Status {
	switch r.Status {
	case "approved", "executed", "executing":
		if r.ResultStatus != nil && *r.ResultStatus >= 400 {
			return StatusBad
		}
		return StatusGood
	case "denied":
		return StatusWarn
	case "expired":
		return StatusBad
	case "pending":
		return StatusInfo
	}
	return StatusMuted
}

func govElevationStatus(e govElevationRow) Status {
	switch e.Status {
	case "approved":
		return StatusWarn
	case "denied":
		return StatusMuted
	case "pending":
		return StatusInfo
	}
	return StatusGood
}

func govApprovalStamps(reqs []govApprovalRow) []string {
	out := make([]string, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, r.CreatedAt)
	}
	return out
}

func govTurnaroundStatus(mins float64, n int) Status {
	if n == 0 {
		return StatusNeutral
	}
	return invThresholdStatus(mins, 60, 240)
}

func govMinutesOrDash(mins float64, n int) string {
	if n == 0 {
		return "—"
	}
	return fmtDuration(mins)
}

func govPercentOrDash(v float64, n int) string {
	if n == 0 {
		return "—"
	}
	return fmtPercent(v)
}

func govCountOf(n, of int) string {
	if of == 0 {
		return fmtInt(n)
	}
	return fmtInt(n) + " of " + fmtInt(of)
}
