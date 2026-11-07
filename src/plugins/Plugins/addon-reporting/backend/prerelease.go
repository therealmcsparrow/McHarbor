// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

var versionCore = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+`)

// isPrerelease reports whether v carries a "-" suffix after its numeric core.
// It is a local copy of the shared rule, checked against versionorder/cases.yaml,
// because plugins share no library code and a suffix test needs no ordering.
func isPrerelease(v string) bool {
	v, _, _ = strings.Cut(strings.TrimSpace(v), "+")
	loc := versionCore.FindStringIndex(v)
	if loc == nil {
		return false
	}
	rest := v[loc[1]:]
	return len(rest) > 1 && rest[0] == '-'
}

// inventoryRow is one subject in the prerelease inventory. It carries a name and
// a version only, so no token or connection detail can reach a report.
type inventoryRow struct {
	Subject    string
	Name       string
	Version    string
	Prerelease bool
}

// prereleaseRows lists McHarbor, every agent and every plugin the snapshot holds,
// so a subject that runs a stable build still prints as an explicit "No".
func prereleaseRows(snap *Snapshot) []inventoryRow {
	rows := []inventoryRow{{
		Subject: "McHarbor", Name: "McHarbor",
		Version: snap.Versions.McHarbor.Version, Prerelease: isPrerelease(snap.Versions.McHarbor.Version),
	}}
	agents := append([]agentRow(nil), snap.Agents...)
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	for _, a := range agents {
		rows = append(rows, inventoryRow{"Agent", orDash(a.Name), a.Version, isPrerelease(a.Version)})
	}
	plugins := append([]pluginRow(nil), snap.Plugins...)
	sort.SliceStable(plugins, func(i, j int) bool { return plugins[i].Slug < plugins[j].Slug })
	for _, p := range plugins {
		rows = append(rows, inventoryRow{"Plugin", orDash(p.Name, p.Slug), p.Version, isPrerelease(p.Version)})
	}
	return rows
}

// prereleaseBlocks renders the summary line and the "Runs a prerelease" table.
// Every row prints, so absence of a prerelease is evidence rather than silence.
func prereleaseBlocks(snap *Snapshot, checked time.Time) []Block {
	rows := prereleaseRows(snap)
	n := 0
	cells := make([][]Cell, 0, len(rows))
	for _, r := range rows {
		answer, st := "No", StatusNeutral
		if r.Prerelease {
			n++
			answer, st = "Yes", StatusWarn
		}
		cells = append(cells, []Cell{cell(r.Subject), cell(r.Name), mono(orDash(r.Version)), pill(answer, st)})
	}
	summary := "No prereleases running. Checked " + fmtDateTime(checked) + "."
	if n > 0 {
		summary = fmtInt(n) + " of " + fmtInt(len(rows)) + " run a prerelease. Checked " + fmtDateTime(checked) + "."
	}
	return []Block{para(summary), table([]Column{
		{Label: "Subject", Width: 1.4},
		{Label: "Name", Width: 2.4},
		{Label: "Version", Width: 2, Mono: true},
		{Label: "Runs a prerelease", Width: 1.6},
	}, cells, "Prerelease inventory", "")}
}
