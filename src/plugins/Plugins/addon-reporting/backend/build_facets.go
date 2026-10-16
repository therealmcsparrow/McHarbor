// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The facet sections the built-in reports print.
//
// One builder per facet, shared by every report that takes it: a
// per-platform breakdown, one cross-platform table with a Platform column,
// and whatever passage each platform attached under its own sub-heading.
// Which report takes which facet is the table below, not a branch inside
// each report's builder.

// facetShow is what one report prints of one facet.
type facetShow uint8

const (
	showBreakdown facetShow = 1 << iota
	showRows
	showBlocks
	showKPI
	// showPublicOnly narrows an exposure table to the publicly reachable
	// surface, for the security reader.
	showPublicOnly
	// showEnvRollup adds the per-environment rollup the comparison report
	// reads across.
	showEnvRollup
	// omitDockerRows leaves Docker's rows out of the cross-platform table
	// because the report already lists every container in detail; Docker
	// still counts in the breakdown, so the totals stay whole.
	omitDockerRows

	showFull = showBreakdown | showRows | showBlocks
)

// facetUse is one facet in one report.
type facetUse struct {
	Facet string
	Show  facetShow
}

// reportFacetUses is which built-in report takes which facet.
func reportFacetUses() map[string][]facetUse {
	inv, capy, prot := plugin.ReportFacetInventory, plugin.ReportFacetCapacity, plugin.ReportFacetProtection
	hlth, expo, cost := plugin.ReportFacetHealth, plugin.ReportFacetExposure, plugin.ReportFacetCost
	healthDetail := showFull | omitDockerRows
	return map[string][]facetUse{
		TypeExecutiveOverview: {
			{inv, showKPI}, {capy, showKPI}, {prot, showKPI},
			{hlth, showKPI | showBlocks}, {expo, showKPI}, {cost, showKPI},
		},
		TypePlatformInventory:    {{inv, showFull}, {capy, showFull}, {hlth, showBlocks}},
		TypeWorkloadInventory:    {{inv, showFull | omitDockerRows}},
		TypeEnvironmentHealth:    {{hlth, healthDetail}},
		TypeAvailability:         {{hlth, healthDetail}},
		TypeUptimeAvailability:   {{hlth, healthDetail}},
		TypeStabilityFlapping:    {{hlth, healthDetail}},
		TypeOperationalReadiness: {{hlth, healthDetail}, {prot, showFull | omitDockerRows}},
		TypeCapacity:             {{capy, showFull}},
		TypeResourceEfficiency:   {{capy, showFull}},
		TypeEnvironmentCompare:   {{inv, showBreakdown | showEnvRollup}, {capy, showBreakdown}},
		TypeBackupRecovery:       {{prot, showFull | omitDockerRows}},
		TypeDataProtection:       {{prot, showFull | omitDockerRows}},
		TypeNetworkConnectivity:  {{expo, showFull}},
		TypeSecurityCompliance:   {{expo, showFull | showPublicOnly}},
		TypeCostAllocation:       {{cost, showFull | omitDockerRows}},
	}
}

// facetsForType is the facets a catalog entry collects.
func facetsForType(key string) []string {
	var out []string
	for _, u := range reportFacetUses()[key] {
		out = append(out, u.Facet)
	}
	return out
}

// facetRowCap bounds the cross-platform table; the breakdown above it is
// always complete.
const facetRowCap = 150

// insertFacetSections adds the facet section to a built report, after the
// last section of its level so the altitudes stay in reading order.
func insertFacetSections(b *buildCtx, secs []Section) []Section {
	uses := reportFacetUses()[b.rt.Key]
	if len(uses) == 0 {
		return secs
	}
	level := facetLevel(b)
	sec := section(level, "Across platforms", facetBlocks(b.snap, uses)...)
	if !sec.hasContent() {
		return secs
	}
	at := len(secs)
	for i, s := range secs {
		if s.Level == level {
			at = i + 1
		}
	}
	out := append([]Section(nil), secs[:at]...)
	out = append(out, sec)
	return append(out, secs[at:]...)
}

// facetLevel is the altitude the facet section prints at: Tactical when the
// report has it, else its last level, but only among levels the request asked
// for, because buildDocument drops a section whose level was not requested.
func facetLevel(b *buildCtx) Level {
	var wanted []Level
	for _, l := range b.rt.Levels {
		if len(b.levels) == 0 || b.wants(l) {
			wanted = append(wanted, l)
		}
	}
	if len(wanted) == 0 {
		wanted = b.rt.Levels
	}
	for _, l := range wanted {
		if l == Tactical {
			return Tactical
		}
	}
	if len(wanted) > 0 {
		return wanted[len(wanted)-1]
	}
	return Tactical
}

// facetBlocks renders every use in order.
func facetBlocks(snap *Snapshot, uses []facetUse) []Block {
	var out []Block
	var kpiRow []KPI
	for _, u := range uses {
		if u.Show&showKPI != 0 {
			if k, ok := facetKPI(snap.facet(u.Facet), u.Facet); ok {
				kpiRow = append(kpiRow, k)
			}
		}
	}
	if len(kpiRow) > 0 {
		out = append(out, heading(2, "The estate across platforms"), kpis(kpiRow...),
			para(platformsLine(snap, uses)))
	}
	for _, u := range uses {
		d := snap.facet(u.Facet)
		if d == nil {
			continue
		}
		if u.Show&(showBreakdown|showRows|showEnvRollup) != 0 {
			out = append(out, facetSection(d, u.Show)...)
		}
		if u.Show&showBlocks != 0 {
			out = append(out, platformPassages(d)...)
		}
	}
	return out
}

// platformsLine names who answered, so a table with no KVM rows reads as
// "KVM is not installed" rather than "KVM runs nothing".
func platformsLine(snap *Snapshot, uses []facetUse) string {
	seen := map[string]bool{}
	var names []string
	for _, u := range uses {
		for _, n := range snap.facet(u.Facet).platformNames() {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 {
		return "No platform answered for this report."
	}
	return "Platforms: " + strings.Join(names, ", ") + "."
}

// facetSection is one facet's heading, coverage line, breakdown and table.
func facetSection(d *facetData, show facetShow) []Block {
	out := []Block{heading(2, titleCase(facetNoun(d.Facet))+" across platforms")}
	line := "Platforms: " + orDash(strings.Join(d.platformNames(), ", ")) + "."
	if d.Note != "" {
		line += " " + d.Note
	}
	out = append(out, para(line))
	var failed []string
	for _, p := range d.Platforms {
		if !p.OK || p.Note != "" {
			failed = append(failed, p.Label+": "+orDash(p.Note, "did not answer"))
		}
	}
	if len(failed) > 0 {
		out = append(out, callout(StatusWarn, "Not every platform answered",
			strings.Join(failed, "; ")+". Those platforms are missing from the figures below, not counted as zero."))
	}
	if show&showBreakdown != 0 {
		if blk, ok := facetBreakdown(d); ok {
			out = append(out, blk)
		}
	}
	if show&showEnvRollup != 0 {
		if blk, ok := facetEnvRollup(d); ok {
			out = append(out, blk)
		}
	}
	if show&showRows != 0 {
		if blk, ok := facetTable(d, show); ok {
			out = append(out, blk)
		}
	}
	return out
}

// platformPassages prints each platform's own passage under its name.
func platformPassages(d *facetData) []Block {
	var out []Block
	for _, p := range d.Platforms {
		blocks := providerBlocks(p.Result.Blocks)
		if !p.OK || len(blocks) == 0 {
			continue
		}
		out = append(out, heading(3, p.Label))
		out = append(out, blocks...)
	}
	return out
}

// --- one row shape for six facets --------------------------------------

// facetLine is one facet row reduced to what the shared tables print.
type facetLine struct {
	id    plugin.ReportFacetIdentity
	cells []Cell
}

// facetColumns are the facet-specific columns after the shared ones.
func facetColumns(facet string) []Column {
	switch facet {
	case plugin.ReportFacetInventory:
		return []Column{{Label: "State", Width: 1.1}, {Label: "Image", Width: 2.4},
			{Label: "CPU", Width: 0.8, Align: "right"}, {Label: "Memory", Width: 1, Align: "right"}}
	case plugin.ReportFacetCapacity:
		return []Column{{Label: "Resource", Width: 1}, {Label: "Allocated", Width: 1, Align: "right"},
			{Label: "Used", Width: 1, Align: "right"}, {Label: "Capacity", Width: 1, Align: "right"}}
	case plugin.ReportFacetProtection:
		return []Column{{Label: "Method", Width: 1}, {Label: "Last good", Width: 1.2},
			{Label: "Off-host", Width: 0.9}, {Label: "Note", Width: 2}}
	case plugin.ReportFacetHealth:
		return []Column{{Label: "Status", Width: 1.1}, {Label: "Restarts", Width: 0.9, Align: "right"},
			{Label: "Flaps", Width: 0.8, Align: "right"}, {Label: "Detail", Width: 2.2}}
	case plugin.ReportFacetExposure:
		return []Column{{Label: "Port", Width: 1.2}, {Label: "Address", Width: 1.2, Mono: true},
			{Label: "Scope", Width: 1}, {Label: "Via", Width: 1}}
	case plugin.ReportFacetCost:
		return []Column{{Label: "CPU core-h", Width: 1, Align: "right"}, {Label: "Memory GB-h", Width: 1.1, Align: "right"},
			{Label: "Disk GB-h", Width: 1, Align: "right"}, {Label: "Basis", Width: 1}}
	}
	return nil
}

// facetLines flattens every platform's rows for one facet.
func facetLines(d *facetData) []facetLine {
	var out []facetLine
	for _, p := range d.Platforms {
		if !p.OK {
			continue
		}
		r := p.Result
		switch d.Facet {
		case plugin.ReportFacetInventory:
			for _, x := range r.Inventory {
				out = append(out, facetLine{x.FacetIdentity, []Cell{cell(orDash(titleCase(x.State))),
					cell(orDash(shortImage(x.Image))), cell(fmtCores(x.CPU)), cell(fmtMB(x.MemoryMB))}})
			}
		case plugin.ReportFacetCapacity:
			for _, x := range r.Capacity {
				used := "—"
				if x.HasUsed {
					used = fmtAmount(x.Used, x.Unit)
				}
				alloc := "—"
				if x.Allocated > 0 {
					alloc = fmtAmount(x.Allocated, x.Unit)
				}
				out = append(out, facetLine{x.FacetIdentity, []Cell{cell(titleCase(x.Resource)), cell(alloc),
					cell(used), cell(fmtAmount(x.Capacity, x.Unit))}})
			}
		case plugin.ReportFacetProtection:
			for _, x := range r.Protection {
				st := StatusBad
				if x.Protected {
					st = StatusGood
				}
				out = append(out, facetLine{x.FacetIdentity, []Cell{pill(titleCase(orDash(x.Method, "none")), st),
					cell(fmtDate(x.LastGood)), cell(yesNo(x.OffHost)), cell(orDash(x.Note))}})
			}
		case plugin.ReportFacetHealth:
			for _, x := range r.Health {
				s := healthStatus(x.Status)
				out = append(out, facetLine{x.FacetIdentity, []Cell{pill(titleCase(s), healthTone(s)),
					cell(fmtInt(x.Restarts)), cell(fmtInt(x.Flaps)), cell(truncateText(x.Detail, 60))}})
			}
		case plugin.ReportFacetExposure:
			for _, x := range r.Exposure {
				port := fmtInt(x.Port) + "/" + orDash(strings.ToLower(x.Protocol), "tcp")
				if x.TargetPort > 0 && x.TargetPort != x.Port {
					port += " → " + fmtInt(x.TargetPort)
				}
				tone := StatusNeutral
				if x.Scope == "public" {
					tone = StatusWarn
				}
				out = append(out, facetLine{x.FacetIdentity, []Cell{cell(port), mono(orDash(x.Address)),
					pill(titleCase(orDash(x.Scope, "unknown")), tone), cell(titleCase(orDash(x.Via)))}})
			}
		case plugin.ReportFacetCost:
			for _, x := range r.Cost {
				out = append(out, facetLine{x.FacetIdentity, []Cell{cell(fmtDecimal(x.CPUCoreHours, 1)),
					cell(fmtDecimal(x.MemoryGBHours, 1)), cell(fmtDecimal(x.DiskGBHours, 1)), cell(titleCase(orDash(x.Basis)))}})
			}
		}
	}
	return out
}

// exposurePublic reports whether a line is part of the public surface.
func exposurePublic(l facetLine) bool {
	return len(l.cells) > 2 && strings.EqualFold(l.cells[2].Text, "public")
}

// facetTable is the one cross-platform table.
func facetTable(d *facetData, show facetShow) (Block, bool) {
	lines := facetLines(d)
	var rows [][]Cell
	omitted := 0
	for _, l := range lines {
		if show&omitDockerRows != 0 && l.id.Platform == facetPlatformDocker {
			continue
		}
		if show&showPublicOnly != 0 && !exposurePublic(l) {
			continue
		}
		if len(rows) >= facetRowCap {
			omitted++
			continue
		}
		row := []Cell{cell(platformOf(d, l.id.Platform)), cell(titleCase(orDash(l.id.Kind))),
			cell(orDash(l.id.Name, l.id.ID)), cell(orDash(l.id.Environment)), cell(orDash(l.id.Owner))}
		rows = append(rows, append(row, l.cells...))
	}
	if len(rows) == 0 {
		return Block{}, false
	}
	cols := []Column{{Label: "Platform", Width: 1.1}, {Label: "Kind", Width: 1}, {Label: "Name", Width: 2},
		{Label: "Environment", Width: 1.4}, {Label: "Owner", Width: 1.3}}
	cols = append(cols, facetColumns(d.Facet)...)
	var notes []string
	if show&omitDockerRows != 0 && d.platform(facetPlatformDocker) != nil {
		notes = append(notes, "Docker containers are listed in detail elsewhere in this report and are counted in the breakdown above.")
	}
	if omitted > 0 {
		notes = append(notes, plural(omitted, "further row is", "further rows are")+" omitted; the breakdown above counts every row.")
	}
	caption := titleCase(facetNoun(d.Facet)) + ", every platform"
	if show&showPublicOnly != 0 {
		caption = "Publicly reachable endpoints, every platform"
	}
	return table(cols, rows, caption, strings.Join(notes, " ")), true
}

// platformOf is the label a platform token prints under.
func platformOf(d *facetData, token string) string {
	if p := d.platform(token); p != nil {
		return p.Label
	}
	return titleCase(token)
}

// --- breakdowns ----------------------------------------------------------

// facetBreakdown is one row per platform, with the counts that facet's
// reader compares across them.
func facetBreakdown(d *facetData) (Block, bool) {
	var cols []Column
	var rows [][]Cell
	for _, p := range d.Platforms {
		if !p.OK {
			continue
		}
		r := p.Result
		name := cell(p.Label)
		switch d.Facet {
		case plugin.ReportFacetInventory:
			cols = []Column{{Label: "Platform", Width: 1.4}, {Label: "Objects", Width: 1, Align: "right"},
				{Label: "Running", Width: 1, Align: "right"}, {Label: "Kinds", Width: 2},
				{Label: "CPU", Width: 1, Align: "right"}, {Label: "Memory", Width: 1, Align: "right"}}
			running, kinds := 0, map[string]bool{}
			var cpu float64
			var mem int64
			for _, x := range r.Inventory {
				if isRunningState(x.State) {
					running++
				}
				kinds[titleCase(x.Kind)] = true
				cpu += x.CPU
				mem += x.MemoryMB
			}
			rows = append(rows, []Cell{name, cell(fmtInt(len(r.Inventory))), cell(fmtInt(running)),
				cell(strings.Join(sortedKeys(kinds), ", ")), cell(fmtCores(cpu)), cell(fmtMB(mem))})
		case plugin.ReportFacetCapacity:
			cols = []Column{{Label: "Platform", Width: 1.4}, {Label: "Resource", Width: 1},
				{Label: "Allocated", Width: 1, Align: "right"}, {Label: "Used", Width: 1, Align: "right"},
				{Label: "Capacity", Width: 1, Align: "right"}, {Label: "Utilisation", Width: 1, Align: "right"}}
			for _, a := range capacityTotals(r.Capacity) {
				used, util, tone := "—", "—", StatusNeutral
				basis := a.allocated
				if a.hasUsed {
					used, basis = fmtAmount(a.used, a.unit), a.used
				}
				if a.capacity > 0 && (a.hasUsed || a.allocated > 0) {
					v := 100 * basis / a.capacity
					util, tone = fmtPercent(v), invThresholdStatus(v, 75, 90)
				}
				alloc := "—"
				if a.allocated > 0 {
					alloc = fmtAmount(a.allocated, a.unit)
				}
				rows = append(rows, []Cell{name, cell(titleCase(a.resource)), cell(alloc), cell(used),
					cell(fmtAmount(a.capacity, a.unit)), cellStatus(util, tone)})
			}
		case plugin.ReportFacetProtection:
			cols = []Column{{Label: "Platform", Width: 1.4}, {Label: "Objects", Width: 1, Align: "right"},
				{Label: "Protected", Width: 1, Align: "right"}, {Label: "Off-host copy", Width: 1.2, Align: "right"},
				{Label: "Within target", Width: 1.2, Align: "right"}, {Label: "Never recovered to", Width: 1.4, Align: "right"}}
			prot, off, fresh, never := 0, 0, 0, 0
			for _, x := range r.Protection {
				if x.Protected {
					prot++
				}
				if x.OffHost {
					off++
				}
				if x.LastGood.IsZero() {
					never++
				} else if withinRPO(x) {
					fresh++
				}
			}
			n := len(r.Protection)
			rows = append(rows, []Cell{name, cell(fmtInt(n)),
				cellStatus(fmtInt(prot), thresholdStatus(pct(prot, n), 90, 60)), cell(fmtInt(off)),
				cell(fmtInt(fresh)), cellStatus(fmtInt(never), countStatus(never, 0, 3))})
		case plugin.ReportFacetHealth:
			cols = []Column{{Label: "Platform", Width: 1.4}, {Label: "Objects", Width: 1, Align: "right"},
				{Label: "Up", Width: 0.8, Align: "right"}, {Label: "Degraded", Width: 1, Align: "right"},
				{Label: "Down", Width: 0.8, Align: "right"}, {Label: "Unknown", Width: 1, Align: "right"},
				{Label: "Flapping", Width: 1, Align: "right"}}
			c := map[string]int{}
			flapping := 0
			for _, x := range r.Health {
				c[healthStatus(x.Status)]++
				if x.Flaps > 0 {
					flapping++
				}
			}
			rows = append(rows, []Cell{name, cell(fmtInt(len(r.Health))), cell(fmtInt(c["up"])),
				cellStatus(fmtInt(c["degraded"]), countStatus(c["degraded"], 0, 0)),
				cellStatus(fmtInt(c["down"]), countStatus(c["down"], 0, 0)),
				cell(fmtInt(c["unknown"])), cell(fmtInt(flapping))})
		case plugin.ReportFacetExposure:
			cols = []Column{{Label: "Platform", Width: 1.4}, {Label: "Endpoints", Width: 1, Align: "right"},
				{Label: "Public", Width: 1, Align: "right"}, {Label: "Internal", Width: 1, Align: "right"},
				{Label: "Cluster", Width: 1, Align: "right"}}
			c := map[string]int{}
			for _, x := range r.Exposure {
				c[strings.ToLower(x.Scope)]++
			}
			rows = append(rows, []Cell{name, cell(fmtInt(len(r.Exposure))), cell(fmtInt(c["public"])),
				cell(fmtInt(c["internal"])), cell(fmtInt(c["cluster"]))})
		case plugin.ReportFacetCost:
			cols = []Column{{Label: "Platform", Width: 1.4}, {Label: "Objects", Width: 1, Align: "right"},
				{Label: "CPU core-h", Width: 1, Align: "right"}, {Label: "Memory GB-h", Width: 1.1, Align: "right"},
				{Label: "Disk GB-h", Width: 1, Align: "right"}, {Label: "Basis", Width: 1.2}}
			var cpu, mem, disk float64
			bases := map[string]bool{}
			for _, x := range r.Cost {
				cpu += x.CPUCoreHours
				mem += x.MemoryGBHours
				disk += x.DiskGBHours
				bases[titleCase(orDash(x.Basis))] = true
			}
			rows = append(rows, []Cell{name, cell(fmtInt(len(r.Cost))), cell(fmtDecimal(cpu, 1)),
				cell(fmtDecimal(mem, 1)), cell(fmtDecimal(disk, 1)), cell(strings.Join(sortedKeys(bases), ", "))})
		}
	}
	if len(rows) == 0 {
		return Block{}, false
	}
	note := ""
	if d.Facet == plugin.ReportFacetCost {
		note = "Measured hours come from sampled use; allocated hours from what was reserved. The two are shown side by side and never added."
	}
	return table(cols, rows, titleCase(facetNoun(d.Facet))+" by platform", note), true
}

// facetEnvRollup is inventory per environment and platform.
func facetEnvRollup(d *facetData) (Block, bool) {
	if d.Facet != plugin.ReportFacetInventory {
		return Block{}, false
	}
	type key struct{ env, platform string }
	counts := map[key][2]int{}
	for _, p := range d.Platforms {
		if !p.OK {
			continue
		}
		for _, x := range p.Result.Inventory {
			k := key{orDash(x.Environment), p.Label}
			c := counts[k]
			c[0]++
			if isRunningState(x.State) {
				c[1]++
			}
			counts[k] = c
		}
	}
	if len(counts) == 0 {
		return Block{}, false
	}
	keys := make([]key, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].env != keys[j].env {
			return keys[i].env < keys[j].env
		}
		return keys[i].platform < keys[j].platform
	})
	rows := make([][]Cell, 0, len(keys))
	for _, k := range keys {
		c := counts[k]
		rows = append(rows, []Cell{cell(k.env), cell(k.platform), cell(fmtInt(c[0])), cell(fmtInt(c[1]))})
	}
	return table([]Column{{Label: "Environment", Width: 2}, {Label: "Platform", Width: 1.4},
		{Label: "Objects", Width: 1, Align: "right"}, {Label: "Running", Width: 1, Align: "right"}},
		rows, "Workloads by environment and platform", ""), true
}

// --- the executive KPI per facet ---------------------------------------

// facetKPI is the one number a board reads for a facet, across every
// platform that answered. ok is false when nothing answered.
func facetKPI(d *facetData, facet string) (KPI, bool) {
	if d == nil || len(d.platformNames()) == 0 {
		return KPI{}, false
	}
	hint := strings.Join(d.platformNames(), ", ")
	var n, a, b int
	for _, p := range d.Platforms {
		if !p.OK {
			continue
		}
		r := p.Result
		switch facet {
		case plugin.ReportFacetInventory:
			n += len(r.Inventory)
			for _, x := range r.Inventory {
				if isRunningState(x.State) {
					a++
				}
			}
		case plugin.ReportFacetProtection:
			n += len(r.Protection)
			for _, x := range r.Protection {
				if x.Protected {
					a++
				}
			}
		case plugin.ReportFacetHealth:
			n += len(r.Health)
			for _, x := range r.Health {
				switch healthStatus(x.Status) {
				case "up":
					a++
				case "down":
					b++
				}
			}
		case plugin.ReportFacetExposure:
			n += len(r.Exposure)
			for _, x := range r.Exposure {
				if strings.EqualFold(x.Scope, "public") {
					a++
				}
			}
		}
	}
	switch facet {
	case plugin.ReportFacetInventory:
		return KPI{Label: "Workloads, all platforms", Value: fmtInt(n), Hint: fmtInt(a) + " running · " + hint}, true
	case plugin.ReportFacetProtection:
		if n == 0 {
			return KPI{}, false
		}
		v := pct(a, n)
		return KPI{Label: "Protected", Value: fmtPercent(v), Target: "100%",
			Status: thresholdStatus(v, 90, 60), Hint: fmtInt(a) + " of " + fmtInt(n) + " · " + hint}, true
	case plugin.ReportFacetHealth:
		if n == 0 {
			return KPI{}, false
		}
		v := pct(a, n)
		return KPI{Label: "Up now", Value: fmtPercent(v), Status: thresholdStatus(v, 98, 90),
			Hint: fmtInt(b) + " down · " + hint}, true
	case plugin.ReportFacetExposure:
		return KPI{Label: "Public endpoints", Value: fmtInt(a), Hint: fmtInt(n) + " published · " + hint}, true
	case plugin.ReportFacetCapacity:
		return capacityKPI(d, hint)
	case plugin.ReportFacetCost:
		return costKPI(d, hint)
	}
	return KPI{}, false
}

// capacityKPI is the tightest resource on any platform: the one that runs
// out first decides, however much of the others is free.
func capacityKPI(d *facetData, hint string) (KPI, bool) {
	best, label := -1.0, ""
	for _, p := range d.Platforms {
		if !p.OK {
			continue
		}
		for _, a := range capacityTotals(p.Result.Capacity) {
			basis := a.allocated
			if a.hasUsed {
				basis = a.used
			}
			if a.capacity <= 0 || basis <= 0 {
				continue
			}
			if v := 100 * basis / a.capacity; v > best {
				best, label = v, p.Label+" "+a.resource
			}
		}
	}
	if best < 0 {
		return KPI{}, false
	}
	return KPI{Label: "Tightest resource", Value: fmtPercent(best), Status: invThresholdStatus(best, 75, 90),
		Hint: label + " · " + hint}, true
}

// costKPI totals CPU core-hours, keeping measured and allocated apart.
func costKPI(d *facetData, hint string) (KPI, bool) {
	var measured, allocated float64
	rows := 0
	for _, p := range d.Platforms {
		if !p.OK {
			continue
		}
		for _, x := range p.Result.Cost {
			rows++
			if strings.EqualFold(x.Basis, "measured") {
				measured += x.CPUCoreHours
			} else {
				allocated += x.CPUCoreHours
			}
		}
	}
	if rows == 0 {
		return KPI{}, false
	}
	return KPI{Label: "CPU core-hours", Value: fmtDecimal(measured+allocated, 0),
		Hint: fmtDecimal(measured, 0) + " measured, " + fmtDecimal(allocated, 0) + " allocated · " + hint}, true
}

// --- annex ----------------------------------------------------------------

// facetCoverageBlocks is the annex's record of which platforms answered
// each facet, including the facets only Docker could answer.
func facetCoverageBlocks(snap *Snapshot) []Block {
	if snap == nil || len(snap.Facets) == 0 {
		return nil
	}
	var pairs []Pair
	for _, f := range allFacets() {
		d := snap.facet(f)
		if d == nil {
			continue
		}
		v := orDash(strings.Join(d.platformNames(), ", "), "no platform answered")
		if d.Note != "" {
			v += ". " + d.Note
		}
		pairs = append(pairs, Pair{K: titleCase(facetNoun(f)), V: v})
	}
	if len(pairs) == 0 {
		return nil
	}
	return []Block{heading(2, "Platform coverage"), keyvalue(pairs...)}
}

// --- small helpers --------------------------------------------------------

type capacityTotal struct {
	resource, unit            string
	allocated, used, capacity float64
	hasUsed                   bool
}

// capacityTotals sums one platform's rows per resource and unit.
func capacityTotals(rows []plugin.ReportCapacityRow) []capacityTotal {
	idx := map[string]int{}
	var out []capacityTotal
	for _, x := range rows {
		k := x.Resource + "|" + x.Unit
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, capacityTotal{resource: x.Resource, unit: x.Unit})
		}
		out[i].allocated += x.Allocated
		out[i].capacity += x.Capacity
		if x.HasUsed {
			out[i].used += x.Used
			out[i].hasUsed = true
		}
	}
	return out
}

// healthStatus folds anything outside the four values into unknown.
func healthStatus(s string) string {
	switch v := strings.ToLower(strings.TrimSpace(s)); v {
	case "up", "degraded", "down":
		return v
	}
	return "unknown"
}

func healthTone(s string) Status {
	switch s {
	case "up":
		return StatusGood
	case "degraded":
		return StatusWarn
	case "down":
		return StatusBad
	}
	return StatusMuted
}

// isRunningState reads the state words platforms use for "doing its job".
func isRunningState(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "running", "up", "active", "ready", "online":
		return true
	}
	return false
}

// withinRPO judges a recovery point against its own target, or a day when
// the platform set none.
func withinRPO(x plugin.ReportProtectionRow) bool {
	target := x.RPOTargetHours
	if target <= 0 {
		target = 24
	}
	return time.Since(x.LastGood).Hours() <= target
}

func fmtCores(v float64) string {
	if v <= 0 {
		return "—"
	}
	return fmtDecimal(v, 1)
}

func fmtMB(mb int64) string {
	if mb <= 0 {
		return "—"
	}
	return fmtBytes(mb << 20)
}

func fmtAmount(v float64, unit string) string {
	return strings.TrimSpace(fmtDecimal(v, 1) + " " + unit)
}

func yesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" && k != "—" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
