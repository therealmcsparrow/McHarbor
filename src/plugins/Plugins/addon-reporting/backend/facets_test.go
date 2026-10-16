// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// fakeFacetHost is a reporting handle whose providers answer facets from
// fixed data, so the consumer can be tested without the host.
type fakeFacetHost struct {
	providers []plugin.ReportFacetProvider
	results   map[string]plugin.ReportFacetResult
	errs      map[string]error
}

func (f *fakeFacetHost) Providers() []plugin.ReportProviderSpec { return nil }
func (f *fakeFacetHost) Build(context.Context, string, plugin.ReportRequest) (plugin.ReportResult, bool, error) {
	return plugin.ReportResult{}, false, nil
}
func (f *fakeFacetHost) Subjects(context.Context, string, string, plugin.ReportRequest) ([]plugin.ReportSubjectOption, bool, error) {
	return nil, false, nil
}
func (f *fakeFacetHost) Available() bool { return len(f.providers) > 0 }

func (f *fakeFacetHost) FacetProviders(facet string) []plugin.ReportFacetProvider {
	var out []plugin.ReportFacetProvider
	for _, p := range f.providers {
		if p.Decl.Facet == facet {
			out = append(out, p)
		}
	}
	return out
}

func (f *fakeFacetHost) CollectFacet(_ context.Context, facet string, _ plugin.ReportFacetRequest) []plugin.ReportFacetContribution {
	var out []plugin.ReportFacetContribution
	for _, p := range f.FacetProviders(facet) {
		k := facet + "/" + p.Decl.Platform
		out = append(out, plugin.ReportFacetContribution{Provider: p, Result: f.results[k], Err: f.errs[k]})
	}
	return out
}

func withFacetHost(t *testing.T, h plugin.ReportingHandle) {
	t.Helper()
	prev := providerHost
	providerHost = h
	t.Cleanup(func() { providerHost = prev })
}

func kvmProvider(facet string) plugin.ReportFacetProvider {
	return plugin.ReportFacetProvider{Slug: "provider-x", DisplayName: "KVM",
		Decl: plugin.ReportFacetDecl{Facet: facet, Platform: "kvm"}}
}

func dockerSnapshot() *Snapshot {
	snap := &Snapshot{Sources: []SourceStatus{
		{Key: srcContainers, OK: true}, {Key: srcBackups, OK: true}, {Key: srcUptime, OK: true},
	}}
	mk := func(id, name, state, status, stack string) containerRow {
		return containerRow{ID: id, Names: []string{"/" + name}, State: state, Status: status, StackName: stack, Image: "nginx:1"}
	}
	web := mk("aaaaaaaaaaaa1111", "web", "running", "Up 2 hours (healthy)", "shop")
	web.Ports = append(web.Ports, containerPort{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"}, containerPort{IP: "127.0.0.1", PrivatePort: 9000, PublicPort: 9000, Type: "tcp"}, containerPort{PrivatePort: 443, Type: "tcp"})
	snap.Containers = []containerRow{
		web,
		mk("bbbbbbbbbbbb2222", "db", "running", "Up 1 hour (unhealthy)", "shop"),
		mk("cccccccccccc3333", "job", "exited", "Exited (1) 3 hours ago", ""),
	}
	snap.BackupPlans = []backupPlanRow{{Name: "db nightly", ContainerName: "db", Enabled: true}}
	snap.BackupRuns = []backupRunRow{{ContainerName: "db", Status: "success", CompletedAt: "2026-09-29T01:00:00Z"}}
	snap.Uptime.Containers = []uptimeRow{{ContainerID: "bbbbbbbbbbbb2222", ContainerName: "db", Outages: 2, Transitions: 5}}
	return snap
}

// TestDockerFacetProjection holds the Docker self-projection to the row
// types every platform uses: identity, the four health values, the public
// and loopback split of published ports, and backup matching by name.
func TestDockerFacetProjection(t *testing.T) {
	snap := dockerSnapshot()
	req := plugin.ReportFacetRequest{EnvironmentID: "env1", Environment: "Production"}

	inv := dockerFacet(snap, plugin.ReportFacetInventory, req)
	if !inv.OK || len(inv.Result.Inventory) != 3 {
		t.Fatalf("inventory = %+v", inv)
	}
	first := inv.Result.Inventory[0]
	if first.Platform != "docker" || first.Kind != "container" || first.Name != "web" ||
		first.Owner != "shop" || first.Environment != "Production" || first.ID != "aaaaaaaaaaaa" {
		t.Errorf("inventory identity = %+v", first.FacetIdentity)
	}

	health := dockerFacet(snap, plugin.ReportFacetHealth, req).Result.Health
	got := map[string]string{}
	for _, h := range health {
		got[h.Name] = h.Status
	}
	want := map[string]string{"web": "up", "db": "degraded", "job": "down"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("health[%s] = %q, want %q", k, got[k], v)
		}
	}
	for _, h := range health {
		if h.Name == "db" && (h.Restarts != 2 || h.Flaps != 5) {
			t.Errorf("db uptime not joined: %+v", h)
		}
	}

	exp := dockerFacet(snap, plugin.ReportFacetExposure, req).Result.Exposure
	if len(exp) != 2 {
		t.Fatalf("exposure = %+v; an unpublished port must not be listed", exp)
	}
	if exp[0].Port != 8080 || exp[0].TargetPort != 80 || exp[0].Scope != "public" {
		t.Errorf("public binding = %+v", exp[0])
	}
	if exp[1].Scope != "internal" {
		t.Errorf("loopback binding scope = %q, want internal", exp[1].Scope)
	}

	prot := dockerFacet(snap, plugin.ReportFacetProtection, req).Result.Protection
	for _, p := range prot {
		switch p.Name {
		case "db":
			if !p.Protected || p.LastGood.IsZero() || p.Method != "backup" {
				t.Errorf("db protection = %+v", p)
			}
		default:
			if p.Protected || p.Method != "none" {
				t.Errorf("%s protection = %+v", p.Name, p)
			}
		}
	}
}

// TestDockerFacetMissingSourceIsNotOK: a projection whose input did not
// load must say so rather than report an estate with nothing in it.
func TestDockerFacetMissingSourceIsNotOK(t *testing.T) {
	snap := &Snapshot{Sources: []SourceStatus{{Key: srcContainers, OK: false, Note: "not permitted for this user"}}}
	p := dockerFacet(snap, plugin.ReportFacetInventory, plugin.ReportFacetRequest{})
	if p.OK || p.Note != "not permitted for this user" {
		t.Fatalf("docker platform = %+v, want not OK with the collector's note", p)
	}
}

// TestCollectFacetsWithoutPlatformsNotesDockerOnly: no reporting bridge is
// a note in the section and the annex, never a failed source.
func TestCollectFacetsWithoutPlatformsNotesDockerOnly(t *testing.T) {
	withFacetHost(t, nil)
	snap := dockerSnapshot()
	collectFacets(context.Background(), snap, []string{plugin.ReportFacetInventory}, plugin.ReportFacetRequest{}, false)

	d := snap.facet(plugin.ReportFacetInventory)
	if d == nil || !strings.Contains(d.Note, "only Docker") {
		t.Fatalf("facet data = %+v, want a Docker-only note", d)
	}
	for _, s := range snap.Sources {
		if !s.OK {
			t.Errorf("source %q recorded as failed; an absent platform plugin is not a failure", s.Key)
		}
	}
	annex := facetCoverageBlocks(snap)
	if len(annex) != 2 || !strings.Contains(annex[1].Pairs[0].V, "only Docker") {
		t.Errorf("annex coverage = %+v", annex)
	}
}

// TestCollectFacetsFailedProviderIsANotOKSource: a provider that errors is
// one not-OK row in the annex with a one-line note, its error text (which
// can carry a hostname) never reaches the document, and the platform that
// did answer still prints.
func TestCollectFacetsFailedProviderIsANotOKSource(t *testing.T) {
	lxc := plugin.ReportFacetProvider{Slug: "provider-y", DisplayName: "LXC",
		Decl: plugin.ReportFacetDecl{Facet: plugin.ReportFacetHealth, Platform: "lxc"}}
	host := &fakeFacetHost{
		providers: []plugin.ReportFacetProvider{kvmProvider(plugin.ReportFacetHealth), lxc},
		results: map[string]plugin.ReportFacetResult{
			"health/kvm": {Health: []plugin.ReportHealthRow{{
				FacetIdentity: plugin.ReportFacetIdentity{Platform: "lxc", Kind: "vm", Name: "vm-1"}, Status: "down"}},
				Blocks: []plugin.ReportBlock{{Kind: "paragraph", Text: "Quorum is intact."}}},
		},
		errs: map[string]error{"health/lxc": errors.New("dial tcp secret-host.internal:8006: refused")},
	}
	withFacetHost(t, host)
	snap := dockerSnapshot()
	collectFacets(context.Background(), snap, []string{plugin.ReportFacetHealth}, plugin.ReportFacetRequest{}, false)

	var failed *SourceStatus
	for i := range snap.Sources {
		if snap.Sources[i].Key == "facet_health_lxc" {
			failed = &snap.Sources[i]
		}
	}
	if failed == nil || failed.OK || failed.Note == "" {
		t.Fatalf("failed provider source = %+v", failed)
	}
	if strings.Contains(failed.Note, "secret-host") {
		t.Fatalf("provider error text leaked into the annex: %q", failed.Note)
	}

	d := snap.facet(plugin.ReportFacetHealth)
	kvm := d.platform("kvm")
	if kvm == nil || !kvm.OK || len(kvm.Result.Health) != 1 || kvm.Result.Health[0].Platform != "kvm" {
		t.Fatalf("kvm platform = %+v; its rows must be stamped with its own platform", kvm)
	}
	blocks := facetSection(d, showFull)
	var sawCallout bool
	for _, b := range blocks {
		if b.Kind == BlockCallout && strings.Contains(b.Text, "LXC") {
			sawCallout = true
		}
	}
	if !sawCallout {
		t.Error("the section does not say which platform did not answer")
	}
	passages := platformPassages(d)
	if len(passages) != 2 || passages[0].Text != "KVM" || passages[1].Text != "Quorum is intact." {
		t.Errorf("platform passages = %+v", passages)
	}
}

// TestFacetTableIsCrossPlatform: one table, a Platform column, Docker
// omitted only when the report already lists its containers.
func TestFacetTableIsCrossPlatform(t *testing.T) {
	d := &facetData{Facet: plugin.ReportFacetInventory, Platforms: []facetPlatform{
		dockerFacet(dockerSnapshot(), plugin.ReportFacetInventory, plugin.ReportFacetRequest{}),
		{Platform: "kvm", Label: "KVM", OK: true, Result: plugin.ReportFacetResult{Inventory: []plugin.ReportInventoryRow{
			{FacetIdentity: plugin.ReportFacetIdentity{Platform: "kvm", Kind: "vm", Name: "vm-1"}, State: "running", CPU: 2, MemoryMB: 2048},
		}}},
	}}
	tbl, ok := facetTable(d, showRows)
	if !ok || len(tbl.Rows) != 4 || tbl.Columns[0].Label != "Platform" {
		t.Fatalf("table = %d rows, columns %+v", len(tbl.Rows), tbl.Columns)
	}
	platforms := map[string]int{}
	for _, r := range tbl.Rows {
		platforms[r[0].Text]++
	}
	if platforms["Docker"] != 3 || platforms["KVM"] != 1 {
		t.Errorf("rows per platform = %v", platforms)
	}
	tbl, _ = facetTable(d, showRows|omitDockerRows)
	if len(tbl.Rows) != 1 || tbl.Note == "" {
		t.Errorf("with Docker omitted: %d rows, note %q", len(tbl.Rows), tbl.Note)
	}
	br, ok := facetBreakdown(d)
	if !ok || len(br.Rows) != 2 {
		t.Fatalf("breakdown = %+v", br)
	}
	k, ok := facetKPI(d, plugin.ReportFacetInventory)
	if !ok || k.Value != "4" {
		t.Errorf("inventory KPI = %+v, want 4 across both platforms", k)
	}
}

// TestAdHocMergesFacetSources: every declared (facet, platform) pair joins
// the planner's menu, a plan naming one validates, and its projection emits
// exactly the promised columns.
func TestAdHocMergesFacetSources(t *testing.T) {
	host := &fakeFacetHost{
		providers: []plugin.ReportFacetProvider{kvmProvider(plugin.ReportFacetInventory)},
		results: map[string]plugin.ReportFacetResult{"inventory/kvm": {Inventory: []plugin.ReportInventoryRow{
			{FacetIdentity: plugin.ReportFacetIdentity{Kind: "vm", Name: "vm-1", Host: "hv-secret"}, State: "running",
				Created: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		}}},
	}
	withFacetHost(t, host)

	key := facetSourceKey(plugin.ReportFacetInventory, "kvm")
	var promised []string
	var sawDocker bool
	for _, d := range sourceCatalog() {
		if d.Key == key {
			promised = append([]string(nil), d.Columns...)
		}
		if d.Key == facetSourceKey(plugin.ReportFacetInventory, facetPlatformDocker) {
			sawDocker = true
		}
	}
	if len(promised) == 0 || !sawDocker {
		t.Fatalf("catalog lacks the kvm (%v) or docker (%v) inventory source", promised, sawDocker)
	}

	plan, err := AdHocPlan{Sources: []string{key}}.validate(nil, nil, nil)
	if err != nil || len(plan.Sources) != 1 {
		t.Fatalf("plan naming a facet source did not validate: %v %+v", err, plan)
	}
	rt := plan.reportType()
	if got := reportFacets(rt); len(got) != 1 || got[0] != plugin.ReportFacetInventory {
		t.Fatalf("facets for the plan = %v", got)
	}

	snap := &Snapshot{}
	collectFacets(context.Background(), snap, reportFacets(rt), plugin.ReportFacetRequest{}, false)
	rows, ok := projectSource(snap, key, 10).([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("projected %v", rows)
	}
	gotCols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		gotCols = append(gotCols, k)
	}
	sort.Strings(gotCols)
	sort.Strings(promised)
	if strings.Join(gotCols, ",") != strings.Join(promised, ",") {
		t.Fatalf("row columns %v, catalog promises %v", gotCols, promised)
	}
	for _, v := range rows[0] {
		if s, ok := v.(string); ok && strings.Contains(s, "hv-secret") {
			t.Fatal("the host name reached the model")
		}
	}
}

// TestEventAndAutohealSourcesAreBuilt holds the two planner invariants for
// the former stubs: a collector job fills each, and each projects exactly
// the columns its catalog entry promises.
func TestEventAndAutohealSourcesAreBuilt(t *testing.T) {
	snap := &Snapshot{
		AlertEvents:      []alertEventRow{{AlertName: "cpu", Severity: "warning", State: "firing", CreatedAt: "2026-09-29T10:00:00Z"}},
		AutohealRestarts: []autohealRow{{EntityName: "web", Timestamp: "2026-09-29T10:00:00Z"}},
	}
	jobs := collectJobs(context.Background(), nil, &Snapshot{}, Period{})
	for _, key := range []string{srcEvents, srcAutoheal} {
		if _, ok := jobs[key]; !ok {
			t.Errorf("no collector job fills %s", key)
		}
		var promised []string
		for _, d := range sourceCatalog() {
			if d.Key == key {
				promised = append([]string(nil), d.Columns...)
			}
		}
		rows, ok := projectSource(snap, key, 10).([]map[string]any)
		if !ok || len(rows) != 1 || len(promised) == 0 {
			t.Fatalf("%s: projected %v, catalog %v", key, rows, promised)
		}
		got := make([]string, 0, len(rows[0]))
		for k := range rows[0] {
			got = append(got, k)
		}
		sort.Strings(got)
		sort.Strings(promised)
		if strings.Join(got, ",") != strings.Join(promised, ",") {
			t.Errorf("%s: row columns %v, catalog promises %v", key, got, promised)
		}
	}
}

// TestInPeriodTrimsToTheWindow: alert events arrive unwindowed and are
// trimmed here.
func TestInPeriodTrimsToTheWindow(t *testing.T) {
	p := Period{Start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}
	rows := []alertEventRow{{ID: "in", CreatedAt: "2026-09-15T00:00:00Z"}, {ID: "out", CreatedAt: "2026-08-15T00:00:00Z"}}
	got := inPeriod(rows, p, func(r alertEventRow) string { return r.CreatedAt })
	if len(got) != 1 || got[0].ID != "in" {
		t.Fatalf("inPeriod = %+v", got)
	}
}

// TestCatalogFacetWiring: the renamed key exists with the inventory facet
// and the Docker sources its projection reads, and the old key is gone.
func TestCatalogFacetWiring(t *testing.T) {
	withFacetHost(t, nil)
	if _, ok := reportTypeByKey("container-inventory"); ok {
		t.Fatal("container-inventory still in the catalog; it was renamed without an alias")
	}
	rt, ok := reportTypeByKey(TypeWorkloadInventory)
	if !ok || len(rt.Facets) != 1 || rt.Facets[0] != plugin.ReportFacetInventory {
		t.Fatalf("workload-inventory = %+v", rt)
	}
	ex, _ := reportTypeByKey(TypeExecutiveOverview)
	if len(ex.Facets) != len(allFacets()) {
		t.Errorf("executive-overview facets = %v, want one KPI per facet", ex.Facets)
	}
	has := map[string]bool{}
	for _, s := range ex.Sources {
		has[s] = true
	}
	for _, s := range []string{srcContainerMetrics, srcCapacity, srcBackups} {
		if !has[s] {
			t.Errorf("executive-overview does not collect %s, which a facet's Docker rows need", s)
		}
	}
	for key := range reportFacetUses() {
		if _, ok := builders()[key]; !ok {
			t.Errorf("facet table names %q, which has no builder", key)
		}
	}
}

// TestFacetReportsRender builds every report that takes a facet end to end
// with a second platform answering: each carries one "Across platforms"
// section, the platform's rows or passage reach the document, and the
// annex names the platforms.
func TestFacetReportsRender(t *testing.T) {
	var providers []plugin.ReportFacetProvider
	results := map[string]plugin.ReportFacetResult{}
	id := plugin.ReportFacetIdentity{Kind: "vm", Name: "vm-sentinel", Environment: "Lab"}
	for _, f := range allFacets() {
		providers = append(providers, kvmProvider(f))
		r := plugin.ReportFacetResult{Blocks: []plugin.ReportBlock{{Kind: "paragraph", Text: "kvm passage " + f}}}
		switch f {
		case plugin.ReportFacetInventory:
			r.Inventory = []plugin.ReportInventoryRow{{FacetIdentity: id, State: "running", CPU: 2}}
		case plugin.ReportFacetCapacity:
			r.Capacity = []plugin.ReportCapacityRow{{FacetIdentity: id, Resource: "cpu", Unit: "cores", Allocated: 6, Capacity: 8}}
		case plugin.ReportFacetProtection:
			r.Protection = []plugin.ReportProtectionRow{{FacetIdentity: id, Protected: true, Method: "snapshot"}}
		case plugin.ReportFacetHealth:
			r.Health = []plugin.ReportHealthRow{{FacetIdentity: id, Status: "up"}}
		case plugin.ReportFacetExposure:
			r.Exposure = []plugin.ReportExposureRow{{FacetIdentity: id, Protocol: "tcp", Port: 22, Scope: "public", Via: "forward"}}
		case plugin.ReportFacetCost:
			r.Cost = []plugin.ReportCostRow{{FacetIdentity: id, CPUCoreHours: 48, Basis: "allocated"}}
		}
		results[f+"/kvm"] = r
	}
	withFacetHost(t, &fakeFacetHost{providers: providers, results: results})

	for key := range reportFacetUses() {
		rt, _ := reportTypeByKey(key)
		snap := dockerSnapshot()
		b := govBuildCtx(t, key, snap)
		collectFacets(context.Background(), snap, reportFacets(rt), plugin.ReportFacetRequest{}, false)
		doc := buildDocument(b)
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		n := 0
		for _, s := range doc.Sections {
			if s.Title == "Across platforms" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s: %d facet sections, want 1", key, n)
		}
		text := string(raw)
		if !strings.Contains(text, "vm-sentinel") && !strings.Contains(text, "kvm passage") && !strings.Contains(text, "\"Lab\"") {
			t.Errorf("%s: nothing the KVM platform contributed reached the document", key)
		}
		if !strings.Contains(text, "Platform coverage") {
			t.Errorf("%s: the annex does not record platform coverage", key)
		}
	}
}
