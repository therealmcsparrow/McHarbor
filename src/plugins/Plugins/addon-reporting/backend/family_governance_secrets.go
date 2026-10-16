// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"time"
)

// Rotation thresholds, in days since the value last changed. Ninety days is
// the common policy interval; a year is where "not yet" becomes "never".
const (
	govRotationDueDays     = 90
	govRotationOverdueDays = 365
	govWideReachRefs       = 10
	govWideReachEnvs       = 3
)

// secretState is one secret with its derived rotation and reach.
type secretState struct {
	Row govSecretRow
	// RotationKnown is false when the value was rotated but the history
	// could not be read, so the age of the current value is unknown.
	RotationKnown bool
	Rotated       bool
	LastChange    time.Time
	AgeDays       float64
	ImpactRead    bool
	Envs          int
	Missing       int
}

// secretFacts is what the secrets-hygiene report derives. It is built from
// metadata alone: the collector never calls an endpoint that returns a value.
type secretFacts struct {
	Assessed         bool
	VersionsRead     bool
	ImpactRead       bool
	States           []secretState
	Total            int
	System           int
	NeverRotated     []secretState
	RotationOverdue  []secretState
	RotationUnknown  int
	Unreferenced     []secretState
	Stale            []secretState
	DanglingRefs     int
	WideReach        []secretState
	RevealedRecently []secretState
	AgeBuckets       [5]int
	ByKind           map[string]int
	ByScope          map[string]int
	TotalRefs        int
}

func govSecrets(snap *Snapshot) secretFacts {
	g := snap.Governance
	now := govNow(snap.Period)
	f := secretFacts{
		Assessed:     snap.available(srcSecrets),
		VersionsRead: snap.available(srcSecretVersions) && len(g.SecretVersions) > 0,
		ImpactRead:   snap.available(srcSecretImpact) && len(g.SecretImpact) > 0,
		ByKind:       map[string]int{},
		ByScope:      map[string]int{},
	}
	if !f.Assessed {
		return f
	}
	for _, s := range g.Secrets {
		st := secretState{Row: s, RotationKnown: true}
		created := parseTS(s.CreatedAt)
		if versions, ok := g.SecretVersions[s.ID]; ok {
			st.Rotated = len(versions) > 0
			st.LastChange = created
			for _, v := range versions {
				if t := parseTS(v.CreatedAt); t.After(st.LastChange) {
					st.LastChange = t
				}
			}
		} else {
			// Without the history, version 1 still proves "never rotated";
			// anything higher proves a rotation but not when.
			st.Rotated = s.Version > 1
			if st.Rotated {
				st.RotationKnown = false
			} else {
				st.LastChange = created
			}
		}
		st.AgeDays = -1
		if st.RotationKnown {
			st.AgeDays = govDaysBetween(st.LastChange, now)
		}
		if imp, ok := g.SecretImpact[s.ID]; ok {
			st.ImpactRead = true
			envs := map[string]bool{}
			for _, r := range imp.Resources {
				if !r.Missing {
					envs[govStr(r.EnvironmentID)] = true
				}
			}
			st.Envs = len(envs)
			st.Missing = imp.MissingCount
		}
		f.States = append(f.States, st)
	}
	sort.SliceStable(f.States, func(i, j int) bool { return f.States[i].Row.Name < f.States[j].Row.Name })

	for _, st := range f.States {
		s := st.Row
		f.Total++
		f.TotalRefs += s.ReferenceCount
		f.ByKind[orDash(s.Kind)]++
		f.ByScope[orDash(s.Scope)]++
		if s.System {
			f.System++
		}
		switch {
		case !st.RotationKnown:
			f.RotationUnknown++
			f.AgeBuckets[4]++
		case st.AgeDays < govRotationDueDays:
			f.AgeBuckets[0]++
		case st.AgeDays < 180:
			f.AgeBuckets[1]++
		case st.AgeDays < govRotationOverdueDays:
			f.AgeBuckets[2]++
		default:
			f.AgeBuckets[3]++
		}
		if st.RotationKnown && !st.Rotated && st.AgeDays >= govRotationDueDays {
			f.NeverRotated = append(f.NeverRotated, st)
		}
		if st.RotationKnown && st.Rotated && st.AgeDays >= govRotationOverdueDays {
			f.RotationOverdue = append(f.RotationOverdue, st)
		}
		// System secrets belong to McHarbor itself and are resolved
		// internally, so a zero reference count says nothing about them.
		if s.ReferenceCount == 0 && !s.System {
			f.Unreferenced = append(f.Unreferenced, st)
		}
		if s.StaleCount > 0 {
			f.Stale = append(f.Stale, st)
		}
		f.DanglingRefs += st.Missing
		if s.ReferenceCount >= govWideReachRefs || st.Envs >= govWideReachEnvs {
			f.WideReach = append(f.WideReach, st)
		}
		if r := parseTS(govStr(s.LastRevealedAt)); !r.IsZero() && govDaysBetween(r, now) <= 30 {
			f.RevealedRecently = append(f.RevealedRecently, st)
		}
	}
	return f
}

func (f secretFacts) neverRotatedOverdue() int {
	n := 0
	for _, s := range f.NeverRotated {
		if s.AgeDays >= govRotationOverdueDays {
			n++
		}
	}
	return n
}

func secretKPIs(f secretFacts) []KPI {
	if !f.Assessed {
		return []KPI{}
	}
	current := f.AgeBuckets[0]
	return []KPI{
		{Label: "Secrets", Value: fmtInt(f.Total), Hint: fmtInt(f.System) + " belong to McHarbor"},
		{Label: "Rotated in 90 days", Value: govPercentOrDash(pct(current, f.Total-f.RotationUnknown), f.Total-f.RotationUnknown),
			Target: "100%", Status: thresholdStatus(pct(current, maxIntOf(f.Total-f.RotationUnknown, 1)), 90, 60)},
		{Label: "Never rotated", Value: fmtInt(len(f.NeverRotated)),
			Status: countStatus(len(f.NeverRotated), 0, 3), Hint: "older than 90 days"},
		{Label: "Unreferenced", Value: fmtInt(len(f.Unreferenced)),
			Status: countStatus(len(f.Unreferenced), 0, 3), Hint: "nothing uses them"},
		{Label: "Running old values", Value: fmtInt(len(f.Stale)),
			Status: countStatus(len(f.Stale), 0, 0), Hint: "rotated, not yet applied"},
	}
}

func secretFindings(f secretFacts) []Finding {
	if !f.Assessed {
		return nil
	}
	var out []Finding
	if n := len(f.NeverRotated); n > 0 {
		sev := SevMedium
		if f.neverRotatedOverdue() > 0 {
			sev = SevHigh
		}
		out = append(out, Finding{
			Severity: sev,
			Title:    plural(n, "secret has never been rotated", "secrets have never been rotated"),
			Detail: "Each has held its first value for more than 90 days; " +
				plural(f.neverRotatedOverdue(), "has", "have") + " held it for over a year. A value that never changes " +
				"stays valid in every copy that has ever been made of it.",
			Recommendation: "Rotate the oldest first, starting with tokens and passwords, and enable auto-apply where the consumers tolerate a restart.",
			Owner:          "Secret owner",
		})
	}
	if n := len(f.RotationOverdue); n > 0 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          plural(n, "secret was last rotated over a year ago", "secrets were last rotated over a year ago"),
			Detail:         "Rotation happened once and then stopped. The practice exists; the cadence does not.",
			Recommendation: "Put these secrets on a scheduled rotation and record the owner of each.",
			Owner:          "Secret owner",
		})
	}
	if n := len(f.Stale); n > 0 {
		out = append(out, Finding{
			Severity: SevMedium,
			Title:    plural(n, "secret was rotated but its consumers still run the old value", "secrets were rotated but their consumers still run the old value"),
			Detail: "The vault holds the new value, but containers or stacks built with the previous one have not been " +
				"recreated. If the rotation was a response to exposure, the exposure is still open.",
			Recommendation: "Apply each rotation from the Secrets page, or enable auto-apply for secrets whose consumers can restart unattended.",
			Owner:          "Platform engineering",
		})
	}
	if n := len(f.WideReach); n > 0 {
		out = append(out, Finding{
			Severity: SevMedium,
			Title:    plural(n, "secret has a wide blast radius", "secrets have a wide blast radius"),
			Detail: "Each is referenced by at least " + fmtInt(govWideReachRefs) + " resources or across " +
				fmtInt(govWideReachEnvs) + " or more environments, so one leaked value reaches all of them at once.",
			Recommendation: "Split shared credentials per environment, so production does not share a value with anything else.",
			Owner:          "Security",
		})
	}
	if n := len(f.Unreferenced); n > 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(n, "secret is referenced by nothing", "secrets are referenced by nothing"),
			Detail:         "An unused secret is still a credential somebody could reveal, with none of the benefit of being used.",
			Recommendation: "Confirm with each owner and delete the ones that are genuinely unused; revoke the credential at its issuer too.",
			Owner:          "Secret owner",
		})
	}
	if f.DanglingRefs > 0 {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(f.DanglingRefs, "reference points at a resource that no longer exists", "references point at resources that no longer exist"),
			Detail:         "Dangling references inflate reach figures and keep secrets looking used after their consumers have gone.",
			Recommendation: "Prune references from each secret's impact view.",
			Owner:          "Platform engineering",
		})
	}
	if n := len(f.RevealedRecently); n > 0 {
		out = append(out, Finding{
			Severity:       SevInfo,
			Title:          plural(n, "secret was revealed in the last 30 days", "secrets were revealed in the last 30 days"),
			Detail:         "Revealing requires an approved elevation and a stated reason, both recorded in the audit log.",
			Recommendation: "Rotate a revealed secret once the task that needed it is done.",
			Owner:          "Security",
		})
	}
	return out
}

func secretLead(f secretFacts) string {
	switch {
	case !f.Assessed:
		return "The secrets vault could not be read, so secrets hygiene is not assessed."
	case f.Total == 0:
		return "The vault holds no secrets."
	case len(f.NeverRotated) > 0:
		return plural(len(f.NeverRotated), "secret has", "secrets have") + " never been rotated and " +
			plural(len(f.Unreferenced), "secret is", "secrets are") + " referenced by nothing."
	case len(f.Stale) > 0:
		return plural(len(f.Stale), "rotated secret has", "rotated secrets have") + " not yet reached the workloads that use it."
	default:
		return "Every secret has been rotated within tolerance and is in use."
	}
}

func buildSecretsHygiene(b *buildCtx) []Section {
	snap := b.snap
	f := govSecrets(snap)
	var out []Section

	st := section(Strategic, "Secrets exposure")
	st.Blocks = append(st.Blocks,
		para("This report reads the vault's metadata only — names, kinds, versions, dates and references. "+
			"No secret value was requested, and none appears in this document."))
	if !f.Assessed {
		st.Blocks = append(st.Blocks, govNotAssessed(snap, srcSecrets, "The secrets vault"))
		return append(out, st)
	}
	st.Blocks = append(st.Blocks,
		kpis(
			KPI{Label: "Secrets", Value: fmtInt(f.Total)},
			KPI{Label: "References", Value: fmtInt(f.TotalRefs), Hint: "resources that resolve a secret"},
			KPI{Label: "Wide blast radius", Value: fmtInt(len(f.WideReach)),
				Status: countStatus(len(f.WideReach), 0, 2)},
			KPI{Label: "Revealed in 30 days", Value: fmtInt(len(f.RevealedRecently))},
		),
		secretAgeChart(f))
	if !f.VersionsRead && f.Total > 0 {
		st.Blocks = append(st.Blocks, callout(StatusInfo, "Rotation dates estimated",
			"Rotation history could not be read ("+snap.note(srcSecretVersions)+"). A secret still on version 1 "+
				"is known never to have rotated; the age of a rotated one is shown as unknown."))
	}
	out = append(out, st)

	ta := section(Tactical, "Rotation and reach")
	ta.Blocks = append(ta.Blocks,
		heading(2, "Oldest values"), secretAgeTable(f),
		heading(2, "Reach of a leaked value"), secretReachTable(f, snap),
		keyvalue(govKindPairs(f.ByScope, "Scope: ")...))
	if len(f.Unreferenced) > 0 {
		ta.Blocks = append(ta.Blocks, heading(2, "Secrets nothing uses"), secretListTable(f.Unreferenced, "Unreferenced secrets"))
	}
	out = append(out, ta)

	op := section(Operational, "Vault inventory")
	op.Blocks = append(op.Blocks, secretInventoryTable(f))
	out = append(out, op)
	return out
}

func secretAgeChart(f secretFacts) Block {
	labels := []string{"Under 90 days", "90–180 days", "180–365 days", "Over a year", "Unknown"}
	tones := []Status{StatusGood, StatusWarn, StatusWarn, StatusBad, StatusMuted}
	pts := make([]ChartPoint, 0, 5)
	for i, l := range labels {
		if f.AgeBuckets[i] == 0 && i == 4 {
			continue
		}
		pts = append(pts, ChartPoint{Label: l, Value: float64(f.AgeBuckets[i]), Status: tones[i]})
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Age of current values", Unit: "secrets", Height: 130,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "Age since the value last changed — the last rotation, or creation if it never rotated.",
	})
}

func secretAgeTable(f secretFacts) Block {
	list := append([]secretState(nil), f.States...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].AgeDays > list[j].AgeDays })
	rows := [][]Cell{}
	for _, s := range list {
		if len(rows) >= 15 || s.AgeDays < govRotationDueDays {
			break
		}
		rotated := "Never"
		if s.Rotated {
			rotated = fmtDate(s.LastChange)
		}
		rows = append(rows, []Cell{
			cell(s.Row.Name),
			mono(orDash(s.Row.Kind)),
			cell(rotated),
			cellStatus(fmtDays(s.AgeDays), freshnessStatus(s.AgeDays, govRotationDueDays, govRotationOverdueDays)),
			cell(fmtInt(s.Row.ReferenceCount)),
		})
	}
	if len(rows) == 0 {
		return callout(StatusGood, "Nothing overdue", "Every secret whose age is known has changed in the last 90 days.")
	}
	return table([]Column{
		{Label: "Secret", Width: 2.4},
		{Label: "Kind", Width: 1.2},
		{Label: "Last rotated", Width: 1.4},
		{Label: "Value age", Width: 1.3},
		{Label: "References", Width: 1, Align: "right"},
	}, rows, "Values older than 90 days", "Oldest first, up to fifteen.")
}

func secretReachTable(f secretFacts, snap *Snapshot) Block {
	list := append([]secretState(nil), f.States...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Row.ReferenceCount > list[j].Row.ReferenceCount })
	rows := [][]Cell{}
	for _, s := range list {
		if len(rows) >= 12 || s.Row.ReferenceCount == 0 {
			break
		}
		envs := "—"
		if s.ImpactRead {
			envs = fmtInt(s.Envs)
		}
		rows = append(rows, []Cell{
			cell(s.Row.Name),
			cell(titleCase(orDash(s.Row.Scope))),
			cellStatus(fmtInt(s.Row.ReferenceCount), countStatus(s.Row.ReferenceCount, 3, govWideReachRefs-1)),
			cell(envs),
			cellStatus(fmtInt(s.Row.StaleCount), countStatus(s.Row.StaleCount, 0, 0)),
			pill(govYesNo(s.Row.AutoApply), govBoolTone(s.Row.AutoApply)),
		})
	}
	if len(rows) == 0 {
		return para("No secret is referenced by any resource.")
	}
	note := "Environments counts distinct environments among live consumers."
	if !f.ImpactRead {
		note = "Environment spread could not be read (" + snap.note(srcSecretImpact) + ")."
	}
	return table([]Column{
		{Label: "Secret", Width: 2.4},
		{Label: "Scope", Width: 1.2},
		{Label: "References", Width: 1, Align: "right"},
		{Label: "Environments", Width: 1.1, Align: "right"},
		{Label: "On old value", Width: 1.1, Align: "right"},
		{Label: "Auto-apply", Width: 1},
	}, rows, "Most-referenced secrets", note)
}

func secretListTable(list []secretState, caption string) Block {
	rows := make([][]Cell, 0, len(list))
	for _, s := range list {
		rows = append(rows, []Cell{
			cell(s.Row.Name),
			mono(orDash(s.Row.Kind)),
			cell(titleCase(orDash(s.Row.Scope))),
			cell(fmtDate(parseTS(s.Row.CreatedAt))),
			cell(orDash(fmtDate(parseTS(govStr(s.Row.LastUsedAt))))),
		})
	}
	return table([]Column{
		{Label: "Secret", Width: 2.4},
		{Label: "Kind", Width: 1.2},
		{Label: "Scope", Width: 1.2},
		{Label: "Created", Width: 1.3},
		{Label: "Last used", Width: 1.3},
	}, rows, caption, "")
}

func secretInventoryTable(f secretFacts) Block {
	rows := make([][]Cell, 0, len(f.States))
	for _, s := range f.States {
		age := "Unknown"
		ageTone := StatusMuted
		if s.RotationKnown {
			age = fmtDays(s.AgeDays)
			ageTone = freshnessStatus(s.AgeDays, govRotationDueDays, govRotationOverdueDays)
		}
		where := titleCase(orDash(s.Row.Scope))
		if n := govStr(s.Row.EnvironmentName); n != "" {
			where += " · " + n
		}
		if n := govStr(s.Row.StackName); n != "" {
			where += " · " + n
		}
		name := s.Row.Name
		if s.Row.System {
			name += " (system)"
		}
		rows = append(rows, []Cell{
			cell(name),
			mono(orDash(s.Row.Kind)),
			cell(where),
			cell("v" + fmtInt(s.Row.Version)),
			cellStatus(age, ageTone),
			cell(fmtInt(s.Row.ReferenceCount)),
			cell(orDash(fmtDate(parseTS(govStr(s.Row.LastUsedAt))))),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "Empty vault", "The vault holds no secrets visible to this user.")
	}
	return table([]Column{
		{Label: "Secret", Width: 2.4},
		{Label: "Kind", Width: 1.1},
		{Label: "Scope", Width: 2},
		{Label: "Version", Width: 0.8, Align: "right"},
		{Label: "Value age", Width: 1.2},
		{Label: "Refs", Width: 0.7, Align: "right"},
		{Label: "Last used", Width: 1.3},
	}, rows, "Every secret, metadata only", "")
}

func govKindPairs(m map[string]int, prefix string) []Pair {
	ranked := topCounted(m, 0)
	out := make([]Pair, 0, len(ranked))
	for _, c := range ranked {
		out = append(out, Pair{K: prefix + titleCase(c.Label), V: fmtInt(c.Count)})
	}
	return out
}

func govYesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func govBoolTone(v bool) Status {
	if v {
		return StatusGood
	}
	return StatusMuted
}
