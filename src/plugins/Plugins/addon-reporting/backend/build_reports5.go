// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
	"time"
)

// Assurance and operations reports.
//
// Assurance answers "who holds what, and who did what" — the questions an
// auditor asks and the ones a platform team is least able to answer from
// memory. Operations answers "is the machinery that keeps this running
// actually running", which is the question nobody asks until it has not been.

// --- 19. User Activity & Operational Footprint -----------------------

func buildUserActivity(b *buildCtx) []Section {
	snap := b.snap
	stats := userFootprint(snap)
	var out []Section

	ta := section(Tactical, "Operational footprint")
	ta.Blocks = append(ta.Blocks,
		para("Who is operating this platform, how much, and when. This is not a performance "+
			"measure — a low count may mean a well-automated estate rather than an idle "+
			"administrator. It is here to answer two other questions: is operational knowledge "+
			"concentrated in one person, and is anything happening at hours nobody scheduled."),
		kpis(userActivityKPIs(snap, stats)...),
		userActivityChart(stats),
		userFootprintTable(stats))

	if concentration(stats) > 70 && len(stats) > 1 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Operational knowledge is concentrated",
			"One account performed "+fmtPercent(concentration(stats))+" of all recorded actions. "+
				"That is a continuity risk before it is a security one: if that person is "+
				"unavailable, the platform is operated by people who have not recently operated it."))
	}
	out = append(out, ta)

	op := section(Operational, "Action detail")
	op.Blocks = append(op.Blocks, actionBreakdownTable(snap))
	if len(snap.Activity) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Workload activity"), activityTable(snap, 50))
	}
	out = append(out, op)
	return out
}

// userStat is one account's recorded footprint.
type userStat struct {
	name       string
	total      int
	mutating   int
	critical   int
	warnings   int
	offHours   int
	firstSeen  time.Time
	lastSeen   time.Time
	topActions map[string]int
}

// userFootprint tallies the audit log by account.
func userFootprint(snap *Snapshot) []userStat {
	byUser := map[string]*userStat{}
	for _, a := range snap.Audit {
		name := "(system)"
		if a.Username != nil && *a.Username != "" {
			name = *a.Username
		}
		st, ok := byUser[name]
		if !ok {
			st = &userStat{name: name, topActions: map[string]int{}}
			byUser[name] = st
		}
		st.total++
		st.topActions[a.Action]++

		switch strings.ToLower(a.Severity) {
		case "critical":
			st.critical++
		case "warning", "warn":
			st.warnings++
		}
		if isMutatingAction(a.Action) {
			st.mutating++
		}

		ts := parseTS(a.Timestamp)
		if !ts.IsZero() {
			if st.firstSeen.IsZero() || ts.Before(st.firstSeen) {
				st.firstSeen = ts
			}
			if ts.After(st.lastSeen) {
				st.lastSeen = ts
			}
			if h := ts.Hour(); h < 7 || h >= 20 || ts.Weekday() == time.Saturday || ts.Weekday() == time.Sunday {
				st.offHours++
			}
		}
	}

	out := make([]userStat, 0, len(byUser))
	for _, st := range byUser {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].total != out[j].total {
			return out[i].total > out[j].total
		}
		return out[i].name < out[j].name
	})
	return out
}

// isMutatingAction distinguishes the actions that changed something from the
// ones that only looked. The audit log records both, and a footprint that
// counts a page view alongside a container deletion tells a reader nothing.
func isMutatingAction(action string) bool {
	a := strings.ToLower(action)
	for _, verb := range []string{"create", "update", "delete", "remove", "start", "stop",
		"restart", "kill", "pause", "deploy", "install", "uninstall", "rename", "recreate",
		"prune", "grant", "revoke", "enable", "disable", "rotate", "reset", "import", "restore"} {
		if strings.Contains(a, verb) {
			return true
		}
	}
	return false
}

func userActivityKPIs(snap *Snapshot, stats []userStat) []KPI {
	var total, mutating, offHours int
	for _, s := range stats {
		total += s.total
		mutating += s.mutating
		offHours += s.offHours
	}
	return []KPI{
		{Label: "Recorded actions", Value: fmtInt(total),
			Hint: "in the audit log for this period"},
		{Label: "Changed something", Value: fmtInt(mutating),
			Hint: fmtPercent(pct(mutating, maxIntOf(total, 1))) + " of all actions"},
		{Label: "Active accounts", Value: fmtInt(len(stats)),
			Target: fmtInt(len(snap.Users)) + " total"},
		{Label: "Outside working hours", Value: fmtInt(offHours),
			Status: countStatus(offHours, 0, 20),
			Hint:   "before 07:00, after 20:00, or at a weekend"},
	}
}

// concentration is the share of all actions performed by the busiest account.
func concentration(stats []userStat) float64 {
	if len(stats) == 0 {
		return 0
	}
	total := 0
	for _, s := range stats {
		total += s.total
	}
	if total == 0 {
		return 0
	}
	return float64(stats[0].total) / float64(total) * 100
}

func userActivityChart(stats []userStat) Block {
	if len(stats) == 0 {
		return divider()
	}
	pts := make([]ChartPoint, 0, len(stats))
	for i, s := range stats {
		if i >= 10 {
			break
		}
		pts = append(pts, ChartPoint{Label: s.name, Value: float64(s.total)})
	}
	return chartBlock(Chart{
		Kind: ChartHBar, Title: "Actions by account", ShowValues: true, Height: 140,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "Every recorded action, read or write, attributed to the account that performed it.",
	})
}

func userFootprintTable(stats []userStat) Block {
	rows := make([][]Cell, 0, len(stats))
	for _, s := range stats {
		top := "—"
		if len(s.topActions) > 0 {
			if ranked := topCounted(s.topActions, 1); len(ranked) > 0 {
				top = ranked[0].Label + " (" + fmtInt(ranked[0].Count) + ")"
			}
		}
		rows = append(rows, []Cell{
			cell(s.name),
			cell(fmtInt(s.total)),
			cell(fmtInt(s.mutating)),
			cellStatus(fmtInt(s.offHours), countStatus(s.offHours, 0, 10)),
			cellStatus(fmtInt(s.critical), countStatus(s.critical, 0, 0)),
			cell(top),
			cell(orDash(fmtDate(s.lastSeen), "—")),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No recorded activity",
			"The audit log holds no entries for this period. Either nothing happened, or audit "+
				"logging is not capturing the actions being taken — the Audit Trail Assurance "+
				"report distinguishes the two.")
	}
	return table(
		[]Column{
			{Label: "Account", Width: 1.8},
			{Label: "Actions", Width: 1, Align: "right"},
			{Label: "Changes", Width: 1, Align: "right"},
			{Label: "Off-hours", Width: 1.1, Align: "right"},
			{Label: "Critical", Width: 1, Align: "right"},
			{Label: "Most frequent action", Width: 2.4},
			{Label: "Last seen", Width: 1.3},
		}, rows, "Footprint by account",
		"“Changes” counts actions that modified something; the remainder were reads. Off-hours "+
			"activity is not suspicious by itself — it is worth asking about only when nobody "+
			"expected it.")
}

func actionBreakdownTable(snap *Snapshot) Block {
	counts := map[string]int{}
	sev := map[string]string{}
	for _, a := range snap.Audit {
		counts[a.Action]++
		if strings.EqualFold(a.Severity, "critical") || sev[a.Action] == "" {
			sev[a.Action] = a.Severity
		}
	}
	ranked := topCounted(counts, 30)
	rows := make([][]Cell, 0, len(ranked))
	for _, c := range ranked {
		kind, st := "Read", StatusNeutral
		if isMutatingAction(c.Label) {
			kind, st = "Change", StatusInfo
		}
		rows = append(rows, []Cell{
			mono(c.Label),
			pill(kind, st),
			cell(fmtInt(c.Count)),
			cell(orDash(titleCase(sev[c.Label]), "—")),
		})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{
			{Label: "Action", Width: 3},
			{Label: "Kind", Width: 1.1},
			{Label: "Count", Width: 1, Align: "right"},
			{Label: "Severity", Width: 1.2},
		}, rows, "Actions by frequency",
		"The thirty most frequent actions in the period.")
}

// --- 20. Role & Permission Matrix ------------------------------------

func buildPermissionMatrix(b *buildCtx) []Section {
	snap := b.snap
	var out []Section

	ta := section(Tactical, "Roles and permissions")
	ta.Blocks = append(ta.Blocks,
		para("What each role can do, and who holds it. A permission model is only reviewable if "+
			"it can be read on one page: this lays out the roles against the breadth of what they "+
			"grant, so an over-broad role is visible without opening each one."),
		kpis(
			KPI{Label: "Roles", Value: fmtInt(len(snap.Roles))},
			KPI{Label: "Groups", Value: fmtInt(len(snap.Groups))},
			KPI{Label: "Users", Value: fmtInt(len(snap.Users)),
				Hint: fmtInt(b.facts.Access.Admins) + " with administrator rights"},
			KPI{Label: "Custom roles", Value: fmtInt(customRoles(snap)),
				Hint: "defined by this installation"},
		),
		permissionBreadthTable(snap),
		roleMembershipTable(snap))

	if b.facts.Access.Admins > 3 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Administrator rights are widely held",
			plural(b.facts.Access.Admins, "account holds", "accounts hold")+" administrator "+
				"rights. Each one can change every setting, read every secret and remove every "+
				"workload. The number should be the smallest that still allows the platform to be "+
				"operated when someone is away."))
	}
	out = append(out, ta)

	op := section(Operational, "Detail")
	op.Blocks = append(op.Blocks, roleTable(snap), groupTable(snap), userTable(snap))
	out = append(out, op)
	return out
}

func customRoles(snap *Snapshot) int {
	n := 0
	for _, r := range snap.Roles {
		if !r.IsSystem {
			n++
		}
	}
	return n
}

// permissionBreadthTable ranks roles by how much they grant.
//
// Breadth is counted rather than listed: a role with sixty permissions is
// over-broad whatever they are, and printing sixty strings per row would make
// the table unreadable at exactly the point it matters most.
func permissionBreadthTable(snap *Snapshot) Block {
	list := append([]roleRow(nil), snap.Roles...)
	sort.Slice(list, func(i, j int) bool {
		if len(list[i].Permissions) != len(list[j].Permissions) {
			return len(list[i].Permissions) > len(list[j].Permissions)
		}
		return list[i].Name < list[j].Name
	})

	widest := 0
	for _, r := range list {
		if len(r.Permissions) > widest {
			widest = len(r.Permissions)
		}
	}

	rows := make([][]Cell, 0, len(list))
	for _, r := range list {
		kind, kindStatus := "Custom", StatusInfo
		if r.IsSystem {
			kind, kindStatus = "Built in", StatusNeutral
		}
		breadth, breadthStatus := "Narrow", StatusGood
		switch {
		case widest > 0 && len(r.Permissions) >= widest:
			breadth, breadthStatus = "Full control", StatusWarn
		case widest > 0 && float64(len(r.Permissions)) > float64(widest)*0.6:
			breadth, breadthStatus = "Broad", StatusWarn
		case float64(len(r.Permissions)) > float64(widest)*0.25:
			breadth, breadthStatus = "Moderate", StatusNeutral
		}
		rows = append(rows, []Cell{
			cell(r.Name),
			pill(kind, kindStatus),
			cell(fmtInt(len(r.Permissions))),
			pill(breadth, breadthStatus),
			cell(truncateText(r.Description, 90)),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No roles readable",
			"The roles endpoint did not answer for this user, so the permission model could not be assessed.")
	}
	return table(
		[]Column{
			{Label: "Role", Width: 1.8},
			{Label: "Origin", Width: 1.1},
			{Label: "Permissions", Width: 1.2, Align: "right"},
			{Label: "Breadth", Width: 1.3},
			{Label: "Purpose", Width: 3.6},
		}, rows, "Roles by breadth of access",
		"Breadth is relative to the widest role on this installation. A custom role rated “full "+
			"control” grants as much as the built-in administrator and should be justified.")
}

func roleMembershipTable(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, u := range snap.Users {
		counts[strings.ToLower(u.Role)]++
	}

	rows := make([][]Cell, 0, len(snap.Roles))
	for _, r := range snap.Roles {
		n := counts[strings.ToLower(r.Name)]
		st := StatusNeutral
		switch {
		case n == 0:
			st = StatusMuted
		case strings.Contains(strings.ToLower(r.Name), "admin") && n > 3:
			st = StatusWarn
		}
		note := ""
		if n == 0 {
			note = "Held by nobody — a candidate for removal"
		}
		rows = append(rows, []Cell{cell(r.Name), cellStatus(fmtInt(n), st), cell(note)})
	}
	if len(rows) == 0 {
		return divider()
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0].Text < rows[j][0].Text })
	return table(
		[]Column{
			{Label: "Role", Width: 2},
			{Label: "Accounts", Width: 1.1, Align: "right"},
			{Label: "Note", Width: 4},
		}, rows, "Who holds each role",
		"A role nobody holds is not harmless: it still exists to be assigned, and it is rarely "+
			"reviewed because it appears to affect nobody.")
}

func groupTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Groups))
	for _, g := range snap.Groups {
		kind, st := "Custom", StatusInfo
		if g.IsSystem {
			kind, st = "Built in", StatusNeutral
		}
		rows = append(rows, []Cell{
			cell(g.Name),
			pill(kind, st),
			cellStatus(fmtInt(g.MemberCount), boolStatus(g.MemberCount > 0)),
			cell(truncateText(g.Description, 90)),
		})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{
			{Label: "Group", Width: 1.8},
			{Label: "Origin", Width: 1.1},
			{Label: "Members", Width: 1, Align: "right"},
			{Label: "Purpose", Width: 3.6},
		}, rows, "Groups", "An empty group grants nothing today and everything to whoever is added tomorrow.")
}

// --- 21. Credential & API Key Hygiene --------------------------------

func buildCredentialHygiene(b *buildCtx) []Section {
	snap := b.snap
	f := b.facts.Access
	var out []Section

	ta := section(Tactical, "Credential hygiene")
	ta.Blocks = append(ta.Blocks,
		para("Machine credentials are the ones nobody notices. A person whose account is removed "+
			"stops being able to sign in; an API key issued two years ago for a script that no "+
			"longer runs keeps working until somebody revokes it. This report is a revocation "+
			"list, not an inventory."),
		kpis(
			KPI{Label: "API keys", Value: fmtInt(f.APIKeys),
				Hint: fmtInt(f.APIKeysActive) + " active"},
			KPI{Label: "Never used", Value: fmtInt(neverUsedKeys(snap)),
				Status: countStatus(neverUsedKeys(snap), 0, 1),
				Hint:   "issued but never presented"},
			KPI{Label: "Unused 90 days", Value: fmtInt(f.APIKeysStale),
				Status: countStatus(f.APIKeysStale, 0, 2)},
			KPI{Label: "Expired but active", Value: fmtInt(f.APIKeysExpired),
				Status: countStatus(f.APIKeysExpired, 0, 0)},
		),
		apiKeyTable(snap),
		keyScopeTable(snap))

	if f.APIKeysStale > 0 || neverUsedKeys(snap) > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Credentials to revoke",
			plural(f.APIKeysStale+neverUsedKeys(snap), "key has", "keys have")+" no recent use. "+
				"An unused credential carries the full risk of a used one and none of the benefit. "+
				"Revoking is reversible — a key that turns out to be needed can be reissued in a minute."))
	}
	out = append(out, ta)

	op := section(Operational, "Accounts and identity")
	op.Blocks = append(op.Blocks, userTable(snap), identityTable(snap))
	out = append(out, op)
	return out
}

func neverUsedKeys(snap *Snapshot) int {
	n := 0
	for _, k := range snap.APIKeys {
		if k.LastUsedAt == nil || *k.LastUsedAt == "" {
			n++
		}
	}
	return n
}

// keyScopeTable groups keys by how much they can do.
func keyScopeTable(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, k := range snap.APIKeys {
		if len(k.Scopes) == 0 {
			counts["(no scope recorded)"]++
			continue
		}
		for _, s := range k.Scopes {
			counts[s]++
		}
	}
	ranked := topCounted(counts, 20)
	rows := make([][]Cell, 0, len(ranked))
	for _, c := range ranked {
		st := StatusNeutral
		lower := strings.ToLower(c.Label)
		if strings.Contains(lower, "admin") || strings.Contains(lower, "*") || strings.Contains(lower, "manage") {
			st = StatusWarn
		}
		rows = append(rows, []Cell{mono(c.Label), cellStatus(fmtInt(c.Count), st)})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{{Label: "Scope", Width: 3}, {Label: "Keys holding it", Width: 1.4, Align: "right"}},
		rows, "Scopes in issue",
		"A scope granting management or administration should be held by as few keys as the "+
			"integrations genuinely require.")
}

func identityTable(snap *Snapshot) Block {
	rows := make([][]Cell, 0, len(snap.Identity))
	for _, p := range snap.Identity {
		label, st := "Disabled", StatusMuted
		if p.Enabled {
			label, st = "Enabled", StatusGood
		}
		rows = append(rows, []Cell{cell(p.Name), cell(titleCase(p.Type)), pill(label, st)})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No external identity provider",
			"Every account on this installation is local. That is workable at a small scale, but "+
				"it means joiner and leaver changes are made here rather than inherited from a "+
				"directory, and it is the usual reason dormant accounts accumulate.")
	}
	return table(
		[]Column{{Label: "Provider", Width: 2}, {Label: "Type", Width: 1.5}, {Label: "State", Width: 1.2}},
		rows, "Identity providers", "")
}

// --- 22. Audit Trail Assurance ---------------------------------------

func buildAuditAssurance(b *buildCtx) []Section {
	snap := b.snap
	f := b.facts.Security
	var out []Section

	ta := section(Tactical, "Audit assurance")
	ta.Blocks = append(ta.Blocks,
		para("An audit trail is evidence only if it is complete. This report examines the trail "+
			"itself rather than what it records: how much was captured, how it is distributed "+
			"across severities and actors, and whether the pattern is consistent with the "+
			"activity the rest of the platform reports."),
		kpis(
			KPI{Label: "Entries", Value: fmtInt(f.AuditEvents),
				Hint: "recorded in this period"},
			KPI{Label: "Critical", Value: fmtInt(f.AuditCritical),
				Status: countStatus(f.AuditCritical, 0, 0)},
			KPI{Label: "Warnings", Value: fmtInt(f.AuditWarn),
				Status: countStatus(f.AuditWarn, 0, 10)},
			KPI{Label: "Failed sign-ins", Value: fmtInt(f.FailedLogins),
				Status: countStatus(f.FailedLogins, 0, 5)},
		),
		auditSeverityChart(snap),
		auditEntityTable(snap))

	// A silent trail during a period the platform was demonstrably being
	// changed is the finding that matters most here, and it is easy to miss
	// because the report otherwise looks clean.
	if f.AuditEvents == 0 && len(snap.Activity) > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusBad, "The audit trail is empty but the platform was active",
			"Container activity was recorded in this period while the audit log captured nothing. "+
				"Either audit logging is disabled, its retention is shorter than this report's "+
				"period, or the actions being taken are not covered. None of the three is a "+
				"position from which an access review can be signed off."))
	} else if f.AuditEvents == 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "No audit entries in this period",
			"Nothing was recorded. Confirm this reflects a quiet period rather than a gap in capture."))
	}
	if f.FailedLogins > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Failed sign-in attempts",
			plural(f.FailedLogins, "failed sign-in was", "failed sign-ins were")+" recorded. A "+
				"handful is ordinary; a sustained run against one account is not."))
	}
	out = append(out, ta)

	op := section(Operational, "Trail detail")
	op.Blocks = append(op.Blocks, auditRecentTable(snap, 50))
	out = append(out, op)
	return out
}

func auditSeverityChart(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, a := range snap.Audit {
		counts[strings.ToLower(orDash(a.Severity, "info"))]++
	}
	order := []struct {
		key    string
		label  string
		status Status
	}{
		{"critical", "Critical", StatusBad},
		{"error", "Error", StatusBad},
		{"warning", "Warning", StatusWarn},
		{"warn", "Warning", StatusWarn},
		{"info", "Information", StatusInfo},
	}
	seen := map[string]bool{}
	var pts []ChartPoint
	for _, o := range order {
		n := counts[o.key]
		if n == 0 || seen[o.label] {
			continue
		}
		seen[o.label] = true
		pts = append(pts, ChartPoint{Label: o.label, Value: float64(n), Status: o.status})
	}
	if len(pts) == 0 {
		return divider()
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Audit entries by severity", Unit: "entries", Height: 130,
		Series: []ChartSeries{{Points: pts}},
	})
}

// auditEntityTable shows what kinds of object the trail actually covers,
// which is how a coverage gap becomes visible.
func auditEntityTable(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, a := range snap.Audit {
		key := "(unspecified)"
		if a.EntityType != nil && *a.EntityType != "" {
			key = *a.EntityType
		}
		counts[key]++
	}
	ranked := topCounted(counts, 20)
	rows := make([][]Cell, 0, len(ranked))
	for _, c := range ranked {
		rows = append(rows, []Cell{cell(titleCase(c.Label)), cell(fmtInt(c.Count))})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{{Label: "Object type", Width: 2.4}, {Label: "Entries", Width: 1.2, Align: "right"}},
		rows, "What the trail covers",
		"An object type the platform manages but that never appears here is a coverage gap: "+
			"changes to it would leave no record.")
}

func auditRecentTable(snap *Snapshot, limit int) Block {
	list := append([]auditRow(nil), snap.Audit...)
	sort.Slice(list, func(i, j int) bool {
		return parseTS(list[i].Timestamp).After(parseTS(list[j].Timestamp))
	})
	if len(list) > limit {
		list = list[:limit]
	}

	rows := make([][]Cell, 0, len(list))
	for _, a := range list {
		st := StatusNeutral
		switch strings.ToLower(a.Severity) {
		case "critical", "error":
			st = StatusBad
		case "warning", "warn":
			st = StatusWarn
		}
		user := "(system)"
		if a.Username != nil && *a.Username != "" {
			user = *a.Username
		}
		rows = append(rows, []Cell{
			cell(fmtDateTime(parseTS(a.Timestamp))),
			cell(user),
			mono(a.Action),
			cell(derefOr(a.EntityName, "—")),
			pill(titleCase(orDash(a.Severity, "info")), st),
		})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{
			{Label: "When", Width: 1.8},
			{Label: "Account", Width: 1.4},
			{Label: "Action", Width: 2.4},
			{Label: "Object", Width: 2.2},
			{Label: "Severity", Width: 1.1},
		}, rows, "Most recent entries",
		"The newest "+fmtInt(limit)+" entries, most recent first.")
}

// --- 23. Patch & Update Currency -------------------------------------

func buildPatchCurrency(b *buildCtx) []Section {
	snap := b.snap
	f := b.facts
	var out []Section

	covered := updateCoverage(snap)
	uncovered := 0
	for _, c := range snap.Containers {
		if !covered[containerName(c)] {
			uncovered++
		}
	}

	st := section(Strategic, "Currency")
	st.Blocks = append(st.Blocks,
		para("Software does not stay still: an image that was current when it was deployed is "+
			"behind within weeks. This report measures how far the estate has drifted and how "+
			"much of that drift is being closed automatically rather than by someone remembering."),
		kpis(
			KPI{Label: "Median image age", Value: fmtDecimal(f.Images.MedianAgeDays, 0) + " days",
				Status: invThresholdStatus(f.Images.MedianAgeDays, 30, 90)},
			KPI{Label: "Older than 90 days", Value: fmtInt(f.Images.StaleOver90Days),
				Status: countStatus(f.Images.StaleOver90Days, 0, 5),
				Hint:   "of " + fmtInt(f.Images.Total) + " images"},
			KPI{Label: "Update policies", Value: fmtInt(len(snap.Updates)),
				Hint: fmtInt(enabledPolicies(snap)) + " enabled"},
			KPI{Label: "Workloads uncovered", Value: fmtInt(uncovered),
				Status: countStatus(uncovered, 0, 5),
				Hint:   "no policy matches them"},
		),
		imageAgeChart(f))
	if b.score != nil {
		st.Blocks = append(st.Blocks, postureTable(b.score))
	}
	out = append(out, st)

	ta := section(Tactical, "Automation and drift")
	ta.Blocks = append(ta.Blocks, updatePolicyTable(snap), updateHistoryTable(snap), platformUpdateBlock(snap))
	if uncovered > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Workloads no policy covers",
			plural(uncovered, "workload is", "workloads are")+" not matched by any enabled update "+
				"policy. They will stay on their current image until someone updates them by hand, "+
				"which in practice means until something breaks."))
	}
	out = append(out, ta)

	op := section(Operational, "Image detail")
	op.Blocks = append(op.Blocks, imageTable(snap, 40), registryTable(snap))
	out = append(out, op)
	return out
}

func enabledPolicies(snap *Snapshot) int {
	n := 0
	for _, p := range snap.Updates {
		if p.Enabled {
			n++
		}
	}
	return n
}

func imageAgeChart(f Facts) Block {
	if f.Images.Total == 0 {
		return divider()
	}
	fresh := f.Images.Total - f.Images.StaleOver90Days
	pts := []ChartPoint{
		{Label: "Under 90 days", Value: float64(fresh), Status: StatusGood},
		{Label: "Over 90 days", Value: float64(f.Images.StaleOver90Days), Status: StatusWarn},
	}
	var kept []ChartPoint
	for _, p := range pts {
		if p.Value > 0 {
			kept = append(kept, p)
		}
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Image age", Unit: "images", Height: 130,
		Series:  []ChartSeries{{Points: kept}},
		Caption: "Age is measured from the image build date, not from when it was pulled.",
	})
}

func updateHistoryTable(snap *Snapshot) Block {
	list := append([]updateHistoryRow(nil), snap.UpdateHist...)
	sort.Slice(list, func(i, j int) bool {
		return parseTS(list[i].ExecutedAt).After(parseTS(list[j].ExecutedAt))
	})
	if len(list) > 30 {
		list = list[:30]
	}

	rows := make([][]Cell, 0, len(list))
	for _, h := range list {
		st := StatusGood
		if !strings.EqualFold(h.Status, "success") && !strings.EqualFold(h.Status, "succeeded") {
			st = StatusBad
		}
		rows = append(rows, []Cell{
			cell(fmtDate(parseTS(h.ExecutedAt))),
			cell(h.Container),
			cell(shortImage(h.OldImage)),
			cell(shortImage(h.NewImage)),
			pill(titleCase(h.Status), st),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No updates applied in this period",
			"No image was updated by automation during the period. Combined with the image ages "+
				"above, that tells you whether the estate is stable or simply untended.")
	}
	return table(
		[]Column{
			{Label: "When", Width: 1.3},
			{Label: "Workload", Width: 2},
			{Label: "From", Width: 2.4},
			{Label: "To", Width: 2.4},
			{Label: "Outcome", Width: 1.2},
		}, rows, "Updates applied", "")
}

func platformUpdateBlock(snap *Snapshot) Block {
	s := snap.SelfUpdate
	if s.CurrentVersion == "" && s.LatestVersion == "" {
		return divider()
	}
	tone, title := StatusGood, "McHarbor is current"
	if s.UpdateAvailable {
		tone, title = StatusWarn, "A McHarbor update is available"
	}
	return callout(tone, title,
		"Running "+orDash(s.CurrentVersion, "unknown")+"; latest known is "+
			orDash(s.LatestVersion, "unknown")+". Last checked "+
			orDash(fmtDate(parseTS(s.LastCheckedAt)), "never")+".")
}

// --- 24. Scheduled Work & Automation ---------------------------------

func buildAutomationSchedules(b *buildCtx) []Section {
	snap := b.snap
	var out []Section

	ta := section(Tactical, "Automation")
	ta.Blocks = append(ta.Blocks,
		para("Everything on this platform that runs without being asked. Automation is invisible "+
			"when it works, which is exactly why it needs a report: a schedule that stopped firing "+
			"six weeks ago looks identical, from the outside, to one that has nothing to do."),
		kpis(
			KPI{Label: "Schedules", Value: fmtInt(len(snap.Schedules)),
				Hint: fmtInt(enabledSchedules(snap)) + " enabled"},
			KPI{Label: "Update policies", Value: fmtInt(len(snap.Updates)),
				Hint: fmtInt(enabledPolicies(snap)) + " enabled"},
			KPI{Label: "Backup plans", Value: fmtInt(len(snap.BackupPlans)),
				Hint: fmtInt(b.facts.Resilience.PlansEnabled) + " enabled"},
			KPI{Label: "Never run", Value: fmtInt(neverRunSchedules(snap)),
				Status: countStatus(neverRunSchedules(snap), 0, 1),
				Hint:   "enabled but no recorded run"},
		),
		scheduleTable(snap))

	if n := disabledAutomation(snap); n > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "Disabled automation",
			plural(n, "automation is", "automations are")+" defined but switched off. Disabled is "+
				"a legitimate state; it is listed here because a disabled schedule is frequently a "+
				"temporary measure that outlived its reason."))
	}
	out = append(out, ta)

	op := section(Operational, "Automation detail")
	op.Blocks = append(op.Blocks, updatePolicyTable(snap), backupPlanTable(snap))
	if len(snap.Alerts) > 0 {
		op.Blocks = append(op.Blocks, heading(2, "Alert rules"), alertTable(snap))
	}
	out = append(out, op)
	return out
}

func enabledSchedules(snap *Snapshot) int {
	n := 0
	for _, s := range snap.Schedules {
		if s.Enabled {
			n++
		}
	}
	return n
}

func neverRunSchedules(snap *Snapshot) int {
	n := 0
	for _, s := range snap.Schedules {
		if s.Enabled && strings.TrimSpace(s.LastRunAt) == "" {
			n++
		}
	}
	return n
}

func disabledAutomation(snap *Snapshot) int {
	n := 0
	for _, s := range snap.Schedules {
		if !s.Enabled {
			n++
		}
	}
	for _, p := range snap.Updates {
		if !p.Enabled {
			n++
		}
	}
	for _, p := range snap.BackupPlans {
		if !p.Enabled {
			n++
		}
	}
	return n
}

func scheduleTable(snap *Snapshot) Block {
	list := append([]scheduleRow(nil), snap.Schedules...)
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	rows := make([][]Cell, 0, len(list))
	for _, s := range list {
		state, st := "Enabled", StatusGood
		if !s.Enabled {
			state, st = "Disabled", StatusMuted
		}
		last := strings.TrimSpace(s.LastRunAt)
		lastCell := pill("Never run", StatusWarn)
		if last != "" {
			age := daysSince(last)
			lastCell = cellStatus(fmtDays(age), freshnessStatus(age, 7, 30))
		} else if !s.Enabled {
			lastCell = cell("—")
		}
		rows = append(rows, []Cell{
			cell(s.Name),
			mono(orDash(s.Cron, "—")),
			cell(titleCase(orDash(s.Action, "—"))),
			cell(orDash(s.Target, "—")),
			pill(state, st),
			lastCell,
			cell(orDash(fmtDate(parseTS(s.NextRunAt)), "—")),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No schedules defined",
			"Nothing on this platform runs on a timer. Backups, updates and pruning are therefore "+
				"manual tasks, and will be done as reliably as they are remembered.")
	}
	return table(
		[]Column{
			{Label: "Schedule", Width: 2},
			{Label: "Cron", Width: 1.6},
			{Label: "Action", Width: 1.4},
			{Label: "Target", Width: 1.8},
			{Label: "State", Width: 1.1},
			{Label: "Last run", Width: 1.3},
			{Label: "Next run", Width: 1.3},
		}, rows, "Scheduled work",
		"An enabled schedule that has never run, or last ran long before its cron expression "+
			"implies, is not doing what it appears to.")
}

// --- 25. Alerting & Monitoring Coverage ------------------------------

func buildAlertingCoverage(b *buildCtx) []Section {
	snap := b.snap
	a := b.facts.Availability
	var out []Section

	watched := alertTargets(snap)
	unwatched := unwatchedWorkloads(snap, watched)

	ta := section(Tactical, "Coverage")
	ta.Blocks = append(ta.Blocks,
		para("A failure nobody is told about is discovered by a user. This report asks which "+
			"workloads have an alert rule pointing at them, and — using the measured history — "+
			"whether the ones that actually failed were among them."),
		kpis(
			KPI{Label: "Alert rules", Value: fmtInt(len(snap.Alerts)),
				Hint: fmtInt(enabledAlerts(snap)) + " enabled"},
			KPI{Label: "Workloads watched", Value: fmtInt(len(snap.Containers) - len(unwatched)),
				Target: fmtInt(len(snap.Containers)) + " total",
				Status: thresholdStatus(pct(len(snap.Containers)-len(unwatched), maxIntOf(len(snap.Containers), 1)), 80, 40)},
			KPI{Label: "Unwatched", Value: fmtInt(len(unwatched)),
				Status: countStatus(len(unwatched), 0, 5)},
			KPI{Label: "Failed unwatched", Value: fmtInt(failedAndUnwatched(a, unwatched)),
				Status: countStatus(failedAndUnwatched(a, unwatched), 0, 0),
				Hint:   "had downtime and no alert rule"},
		),
		alertTable(snap))

	if n := failedAndUnwatched(a, unwatched); n > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusBad, "Failures nobody was told about",
			plural(n, "workload had downtime", "workloads had downtime")+" in this period with no "+
				"alert rule watching it. Whatever the response time was, it started when somebody "+
				"noticed rather than when the platform did."))
	} else if len(unwatched) > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Unwatched workloads",
			plural(len(unwatched), "workload has", "workloads have")+" no alert rule. None of them "+
				"failed this period, which is luck rather than coverage."))
	}
	out = append(out, ta)

	op := section(Operational, "Uncovered workloads")
	op.Blocks = append(op.Blocks, unwatchedTable(snap, unwatched, a))
	out = append(out, op)
	return out
}

func enabledAlerts(snap *Snapshot) int {
	n := 0
	for _, a := range snap.Alerts {
		if a.Enabled {
			n++
		}
	}
	return n
}

// alertTargets collects the targets named by enabled rules. A rule with an
// empty target is treated as covering everything, which is how the alerting
// module interprets it.
func alertTargets(snap *Snapshot) map[string]bool {
	out := map[string]bool{}
	for _, a := range snap.Alerts {
		if !a.Enabled {
			continue
		}
		if strings.TrimSpace(a.Target) == "" {
			out["*"] = true
			continue
		}
		out[strings.ToLower(strings.TrimSpace(a.Target))] = true
	}
	return out
}

func unwatchedWorkloads(snap *Snapshot, watched map[string]bool) []string {
	if watched["*"] {
		return nil
	}
	var out []string
	for _, c := range snap.Containers {
		name := containerName(c)
		covered := watched[strings.ToLower(name)]
		if !covered {
			for target := range watched {
				if matchesPattern(target, name) || matchesPattern(target, c.StackName) {
					covered = true
					break
				}
			}
		}
		if !covered {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func failedAndUnwatched(a AvailabilityFacts, unwatched []string) int {
	if !a.Assessed {
		return 0
	}
	byName := uptimeByName(a)
	n := 0
	for _, name := range unwatched {
		if r, ok := byName[name]; ok && r.DownSamples > 0 {
			n++
		}
	}
	return n
}

func unwatchedTable(snap *Snapshot, unwatched []string, a AvailabilityFacts) Block {
	if len(unwatched) == 0 {
		return callout(StatusGood, "Every workload is covered",
			"An enabled alert rule matches every workload in scope.")
	}
	byName := uptimeByName(a)
	index := map[string]containerRow{}
	for _, c := range snap.Containers {
		index[containerName(c)] = c
	}

	rows := make([][]Cell, 0, len(unwatched))
	for _, name := range unwatched {
		c := index[name]
		history := cell("—")
		if r, ok := byName[name]; ok {
			history = cellStatus(fmtUptime(r.UptimePercent),
				thresholdStatus(r.UptimePercent, a.TargetPercent, 99))
		}
		stack := c.StackName
		if stack == "" {
			stack = "—"
		}
		rows = append(rows, []Cell{
			cell(name),
			pill(titleCase(orDash(c.State, "unknown")), containerStateStatus(c)),
			cell(stack),
			history,
		})
	}
	return table(
		[]Column{
			{Label: "Workload", Width: 2.6},
			{Label: "State", Width: 1.2},
			{Label: "Stack", Width: 1.8},
			{Label: "Measured uptime", Width: 1.5, Align: "right"},
		}, rows, "Workloads with no alert rule",
		"Uptime is shown so the list can be triaged: an unwatched workload that has never failed "+
			"is a lower priority than an unwatched workload that has.")
}

// --- 26. Remote Agent Fleet ------------------------------------------

func buildAgentFleet(b *buildCtx) []Section {
	snap := b.snap
	var out []Section

	ta := section(Tactical, "Agent fleet")
	ta.Blocks = append(ta.Blocks,
		para("Remote agents connect outbound from each host, which is what allows McHarbor to "+
			"manage a Docker daemon behind NAT without exposing its socket. The trade is that a "+
			"host is manageable only while its agent is connected and current: an agent that is "+
			"offline is not an alert, it is an absence."),
		kpis(
			KPI{Label: "Agents", Value: fmtInt(len(snap.Agents))},
			KPI{Label: "Connected", Value: fmtInt(connectedAgents(snap)),
				Target: fmtInt(len(snap.Agents)) + " total",
				Status: boolStatus(connectedAgents(snap) == len(snap.Agents))},
			KPI{Label: "Version spread", Value: fmtInt(agentVersionSpread(snap)),
				Status: countStatus(agentVersionSpread(snap), 1, 2),
				Hint:   "distinct agent versions in use"},
			KPI{Label: "Stale", Value: fmtInt(staleAgents(snap)),
				Status: countStatus(staleAgents(snap), 0, 0),
				Hint:   "not seen in over a day"},
		),
		agentTable(snap),
		agentVersionTable(snap))

	if agentVersionSpread(snap) > 1 {
		ta.Blocks = append(ta.Blocks, callout(StatusWarn, "Agents are not on one version",
			"Different agent versions support different features — non-detached exec, for "+
				"instance, needs agent v1.1.0 or later. A mixed fleet means a capability works on "+
				"some hosts and silently does not on others."))
	}
	out = append(out, ta)

	op := section(Operational, "Environments")
	op.Blocks = append(op.Blocks, environmentTable(snap))
	out = append(out, op)
	return out
}

func connectedAgents(snap *Snapshot) int {
	n := 0
	for _, a := range snap.Agents {
		if strings.EqualFold(a.Status, "connected") || strings.EqualFold(a.Status, "online") {
			n++
		}
	}
	return n
}

func staleAgents(snap *Snapshot) int {
	n := 0
	for _, a := range snap.Agents {
		if age := daysSince(a.LastSeen); age > 1 || age < 0 {
			n++
		}
	}
	return n
}

func agentVersionSpread(snap *Snapshot) int {
	seen := map[string]bool{}
	for _, a := range snap.Agents {
		if a.Version != "" {
			seen[a.Version] = true
		}
	}
	return len(seen)
}

func agentVersionTable(snap *Snapshot) Block {
	counts := map[string]int{}
	for _, a := range snap.Agents {
		counts[orDash(a.Version, "(unreported)")]++
	}
	ranked := topCounted(counts, 20)
	rows := make([][]Cell, 0, len(ranked))
	for _, c := range ranked {
		rows = append(rows, []Cell{mono(c.Label), cell(fmtInt(c.Count))})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table(
		[]Column{{Label: "Agent version", Width: 2}, {Label: "Hosts", Width: 1.2, Align: "right"}},
		rows, "Versions in the fleet", "")
}
