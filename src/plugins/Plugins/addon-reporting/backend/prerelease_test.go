// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var prereleaseCase = regexp.MustCompile(`^\s*-\s*\{\s*input:\s*("(?:[^"\\]|\\.)*"),\s*want:\s*(true|false)\s*\}`)

func TestIsPrereleaseMatchesSharedCases(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/versionorder/cases.yaml")
	if err != nil {
		t.Fatalf("read cases.yaml: %v", err)
	}
	in := false
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "prerelease:") {
			in = true
			continue
		}
		if in && line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		m := prereleaseCase.FindStringSubmatch(line)
		if !in || m == nil {
			continue
		}
		input, err := strconv.Unquote(m[1])
		if err != nil {
			t.Fatalf("unquote %s: %v", m[1], err)
		}
		n++
		if got, want := isPrerelease(input), m[2] == "true"; got != want {
			t.Errorf("isPrerelease(%q) = %v, want %v", input, got, want)
		}
	}
	if n < 7 {
		t.Fatalf("read %d prerelease cases, want at least 7", n)
	}
}

func testSnapshot() *Snapshot {
	snap := &Snapshot{
		Agents: []agentRow{
			{Name: "homelab01", Version: "2026.11.02-rc.1"},
			{Name: "nas", Version: "2026.11.01"},
		},
		Plugins: []pluginRow{
			{Slug: "addon-kvm", Name: "KVM", Version: "2026.11.02-rc.1"},
			{Slug: "addon-backup", Name: "Backup", Version: "2026.11.01"},
		},
	}
	snap.Versions.McHarbor.Version = "2026.11.02-rc.1"
	return snap
}

func TestRowsCarryPrerelease(t *testing.T) {
	rows := prereleaseRows(testSnapshot())
	want := map[string]bool{
		"McHarbor/McHarbor": true,
		"Agent/homelab01":   true,
		"Agent/nas":         false,
		"Plugin/KVM":        true,
		"Plugin/Backup":     false,
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for _, r := range rows {
		w, ok := want[r.Subject+"/"+r.Name]
		if !ok {
			t.Errorf("unexpected row %+v", r)
			continue
		}
		if r.Prerelease != w {
			t.Errorf("%s/%s Prerelease = %v, want %v", r.Subject, r.Name, r.Prerelease, w)
		}
	}
}

func TestPrereleaseBlocksPrintYesNoAndSummary(t *testing.T) {
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	blocks := prereleaseBlocks(testSnapshot(), at)
	var summary string
	var tbl Block
	for _, b := range blocks {
		switch b.Kind {
		case BlockParagraph:
			summary = b.Text
		case BlockTable:
			tbl = b
		}
	}
	if !strings.HasPrefix(summary, "3 of 5 run a prerelease. Checked ") {
		t.Errorf("summary = %q", summary)
	}
	if len(tbl.Columns) < 4 || tbl.Columns[3].Label != "Runs a prerelease" {
		t.Fatalf("columns = %+v", tbl.Columns)
	}
	for _, row := range tbl.Rows {
		if v := row[3].Text; v != "Yes" && v != "No" {
			t.Errorf("Runs a prerelease cell = %q", v)
		}
	}
	if len(tbl.Rows) != 5 {
		t.Errorf("rows = %d, want 5", len(tbl.Rows))
	}
}

func TestPrereleaseBlocksNoneRunning(t *testing.T) {
	snap := &Snapshot{Agents: []agentRow{{Name: "nas", Version: "2026.11.01"}}}
	blocks := prereleaseBlocks(snap, time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC))
	if blocks[0].Kind != BlockParagraph || !strings.HasPrefix(blocks[0].Text, "No prereleases running. Checked ") {
		t.Errorf("first block = %+v", blocks[0])
	}
}

func TestAdhocProjectionCarriesPrerelease(t *testing.T) {
	snap := testSnapshot()
	agents := projectSource(snap, srcAgents, 10).([]map[string]any)
	if agents[0]["prerelease"] != true || agents[1]["prerelease"] != false {
		t.Errorf("agents = %+v", agents)
	}
	plugins := projectSource(snap, srcPlugins, 10).([]map[string]any)
	if plugins[0]["prerelease"] != true || plugins[1]["prerelease"] != false {
		t.Errorf("plugins = %+v", plugins)
	}
	for _, d := range builtinSourceCatalog() {
		if d.Key != srcAgents && d.Key != srcPlugins {
			continue
		}
		found := false
		for _, c := range d.Columns {
			found = found || c == "prerelease"
		}
		if !found {
			t.Errorf("%s catalog columns lack prerelease: %v", d.Key, d.Columns)
		}
	}
	if projectSource(snap, srcVersions, 10).(map[string]any)["mcharbor_prerelease"] != true {
		t.Error("versions projection lacks mcharbor_prerelease")
	}
}

func TestPlatformInventoryReportCarriesThePrereleaseTable(t *testing.T) {
	b := resTestCtx(t, TypePlatformInventory, testSnapshot())
	b.meta.GeneratedAt = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	var summary string
	rows := 0
	for _, s := range buildPlatformInventory(b) {
		for _, bl := range s.Blocks {
			if bl.Kind == BlockParagraph && strings.Contains(bl.Text, "run a prerelease") {
				summary = bl.Text
			}
			if bl.Kind == BlockTable && bl.Caption == "Prerelease inventory" {
				rows = len(bl.Rows)
			}
		}
	}
	if !strings.HasPrefix(summary, "3 of 5 run a prerelease. Checked ") {
		t.Fatalf("summary = %q", summary)
	}
	if rows != 5 {
		t.Fatalf("prerelease table rows = %d, want 5", rows)
	}
}

// TestVersionsProjectionAgentRowsCarryPrerelease: the nested agent rows of the
// versions source answer the same question the agents source does.
func TestVersionsProjectionAgentRowsCarryPrerelease(t *testing.T) {
	snap := testSnapshot()
	for _, a := range []struct{ name, version string }{{"homelab01", "2026.11.02-rc.1"}, {"nas", "2026.11.01"}} {
		snap.Versions.Agents = append(snap.Versions.Agents, struct {
			EnvironmentID string `json:"environmentId"`
			Name          string `json:"name"`
			Version       string `json:"version"`
		}{Name: a.name, Version: a.version})
	}
	agents := projectSource(snap, srcVersions, 10).(map[string]any)["agents"].([]map[string]any)
	if len(agents) != 2 || agents[0]["prerelease"] != true || agents[1]["prerelease"] != false {
		t.Fatalf("versions agents = %+v", agents)
	}
}
