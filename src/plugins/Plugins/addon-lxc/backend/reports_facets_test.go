// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

var facetNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type fakeFacetSource struct {
	cts         []Container
	ctsErr      error
	snaps       map[string][]Snapshot
	snapErr     map[string]error
	backups     map[string]time.Time
	backupErr   map[string]error
	hostCPUs    int
	hostMem     int64
	hostErr     error
	hostCapCall int
}

func (f *fakeFacetSource) containers(context.Context, string) ([]Container, error) {
	return f.cts, f.ctsErr
}

func (f *fakeFacetSource) snapshots(_ context.Context, id string) ([]Snapshot, error) {
	return f.snaps[id], f.snapErr[id]
}

func (f *fakeFacetSource) lastGoodBackup(_ context.Context, id string) (time.Time, error) {
	return f.backups[id], f.backupErr[id]
}

func (f *fakeFacetSource) hostCapacity(context.Context, string) (int, int64, error) {
	f.hostCapCall++
	return f.hostCPUs, f.hostMem, f.hostErr
}

type fakePerms struct {
	allowEnv map[string]bool
	err      error
	saw      []string
}

func (f *fakePerms) HasPermission(_, envID, perm string) (bool, error) {
	f.saw = append(f.saw, perm)
	return f.allowEnv[envID], f.err
}

func facetProvider(src *fakeFacetSource, perms *fakePerms) *reportProvider {
	return &reportProvider{plugin: &LXCPlugin{}, source: src, perms: perms}
}

func facetReq(facet string) plugin.ReportFacetRequest {
	return plugin.ReportFacetRequest{
		Facet: facet, UserID: "u1",
		PeriodStart: facetNow.Add(-48 * time.Hour), PeriodEnd: facetNow,
	}
}

func startedAgo(d time.Duration) *time.Time {
	t := time.Now().Add(-d)
	return &t
}

func facetFixture() *fakeFacetSource {
	return &fakeFacetSource{
		cts: []Container{
			{ID: "a", Name: "web", EnvironmentID: "h1", PoolID: "p1", State: StateRunning, HostAvailable: true,
				OSFamily: "debian", OSVersion: "12", CreatedAt: facetNow.Add(-72 * time.Hour),
				LastStartedAt: startedAgo(10 * time.Hour),
				Limits:        Limits{CPUMax: "200000 100000", MemoryMaxBytes: 2 << 30, DiskQuotaBytes: 10 << 30}},
			{ID: "b", Name: "db", EnvironmentID: "h1", State: StateStopped, HostAvailable: true,
				Limits: Limits{MemoryMaxBytes: 1 << 30}},
			{ID: "c", Name: "gone", EnvironmentID: "h2", State: StateRunning, HostAvailable: false},
			{ID: "d", Name: "crash", EnvironmentID: "h1", State: StateCrashed, HostAvailable: true, ExitReason: "oom"},
			{ID: "e", Name: "frz", EnvironmentID: "h1", State: StateFrozen, HostAvailable: true,
				Limits: Limits{CPUMax: "50000", MemoryMaxBytes: 1 << 30}},
		},
		hostCPUs: 8, hostMem: 16 << 30,
	}
}

func allowAll() *fakePerms { return &fakePerms{allowEnv: map[string]bool{"h1": true, "h2": true}} }

func TestFacetInventoryRows(t *testing.T) {
	res, err := facetProvider(facetFixture(), allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 5 {
		t.Fatalf("want 5 rows, got %d", len(res.Inventory))
	}
	web := res.Inventory[0]
	if web.Platform != "lxc" || web.Kind != "ct" || web.ID != "a" || web.Owner != "p1" || web.Host != "h1" {
		t.Fatalf("identity: %+v", web.FacetIdentity)
	}
	if web.State != "running" || web.Image != "debian 12" || web.CPU != 2 || web.MemoryMB != 2048 || web.DiskGB != 10 {
		t.Fatalf("row: %+v", web)
	}
	if res.Inventory[2].State != StateUnknown {
		t.Fatalf("an unreachable host must read unknown, got %q", res.Inventory[2].State)
	}
	if !hasNote(res, "unreachable host") {
		t.Fatalf("notes: %v", res.Notes)
	}
}

func TestFacetHealthStatuses(t *testing.T) {
	res, err := facetProvider(facetFixture(), allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetHealth))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a": "up", "b": "down", "c": "unknown", "d": "down", "e": "degraded"}
	for _, row := range res.Health {
		if want[row.ID] != row.Status {
			t.Errorf("%s: want %s, got %s", row.ID, want[row.ID], row.Status)
		}
	}
	for _, row := range res.Health {
		if row.ID == "d" && row.Detail != "crashed: oom" {
			t.Errorf("crash detail: %q", row.Detail)
		}
		if row.ID == "a" && row.Since.IsZero() {
			t.Error("a running container carries its start time")
		}
	}
}

func TestFacetCostIsAllocatedAndClippedToThePeriod(t *testing.T) {
	src := facetFixture()
	req := facetReq(plugin.ReportFacetCost)
	req.PeriodStart, req.PeriodEnd = time.Now().Add(-48*time.Hour), time.Time{}
	res, err := facetProvider(src, allowAll()).ContributeFacet(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// Running or frozen on a reachable host: a and e. b stopped, c unreachable, d crashed.
	if len(res.Cost) != 2 {
		t.Fatalf("want 2 cost rows, got %d: %+v", len(res.Cost), res.Cost)
	}
	web := res.Cost[0]
	if web.Basis != "allocated" || web.ID != "a" {
		t.Fatalf("row: %+v", web)
	}
	if math.Abs(web.CPUCoreHours-20) > 0.1 || math.Abs(web.MemoryGBHours-20) > 0.1 || math.Abs(web.DiskGBHours-100) > 0.5 {
		t.Fatalf("10 hours at 2 cores, 2 GiB, 10 GiB: %+v", web)
	}
}

func TestFacetCapacityAllocatesOnlyRunningAndProbesEachHostOnce(t *testing.T) {
	src := facetFixture()
	res, err := facetProvider(src, allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetCapacity))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]plugin.ReportCapacityRow{}
	for _, row := range res.Capacity {
		got[row.ID+"/"+row.Resource] = row
	}
	cpu, mem := got["h1/cpu"], got["h1/memory"]
	if cpu.Kind != "host" || cpu.Capacity != 8 || math.Abs(cpu.Allocated-2.5) > 1e-9 || cpu.HasUsed {
		t.Fatalf("cpu: %+v", cpu)
	}
	if mem.Capacity != 16 || mem.Allocated != 3 || mem.Unit != "GiB" {
		t.Fatalf("memory counts running a (2) and frozen e (1), not stopped b: %+v", mem)
	}
	if src.hostCapCall != 2 {
		t.Fatalf("one probe per host, got %d", src.hostCapCall)
	}
}

func TestFacetCapacityProbeFailureIsANotOKSourceWithoutLeakingTheError(t *testing.T) {
	src := facetFixture()
	src.hostErr = errors.New("dial tcp 10.0.0.9:22: connection refused")
	res, err := facetProvider(src, allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetCapacity))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Capacity) != 0 {
		t.Fatalf("no rows against an unknown capacity: %+v", res.Capacity)
	}
	var found bool
	for _, s := range res.Sources {
		if s.Key == srcHostCaps {
			found = true
			if s.OK || s.Note == "" || strings.Contains(s.Note, "10.0.0.9") {
				t.Fatalf("source: %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("the failed read must still be recorded")
	}
}

func TestFacetProtectionPrefersBackupAndMarksSnapshotsOnHost(t *testing.T) {
	src := facetFixture()
	src.snaps = map[string][]Snapshot{
		"a": {{CreatedAt: facetNow.Add(-24 * time.Hour)}},
		"b": {{CreatedAt: facetNow.Add(-48 * time.Hour)}, {CreatedAt: facetNow.Add(-72 * time.Hour)}},
	}
	src.backups = map[string]time.Time{"a": facetNow.Add(-5 * time.Hour)}
	res, err := facetProvider(src, allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetProtection))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]plugin.ReportProtectionRow{}
	for _, r := range res.Protection {
		rows[r.ID] = r
	}
	a, b, d := rows["a"], rows["b"], rows["d"]
	if !a.Protected || a.Method != "backup" || !a.OffHost || !a.LastGood.Equal(facetNow.Add(-5*time.Hour)) {
		t.Fatalf("a: %+v", a)
	}
	if !b.Protected || b.Method != "snapshot" || b.OffHost || !b.LastGood.Equal(facetNow.Add(-48*time.Hour)) {
		t.Fatalf("b: a snapshot is protected but never off-host, and the newest decides: %+v", b)
	}
	if d.Protected || d.Method != "none" || !d.LastGood.IsZero() {
		t.Fatalf("d: %+v", d)
	}
}

func TestFacetProtectionUnreadableSnapshotsLeaveTheContainerOutAndFailTheSource(t *testing.T) {
	src := facetFixture()
	src.snapErr = map[string]error{"a": errors.New("sql: database is locked at /data/plugins/x.db")}
	src.backupErr = map[string]error{"b": errors.New("boom")}
	res, err := facetProvider(src, allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetProtection))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Protection {
		if r.ID == "a" {
			t.Fatal("a container whose snapshots could not be read must not be reported as unprotected")
		}
		if r.ID == "b" && !strings.Contains(r.Note, "could not be read") {
			t.Fatalf("b's note: %q", r.Note)
		}
	}
	bySrc := map[string]plugin.ReportSourceStatus{}
	for _, s := range res.Sources {
		bySrc[s.Key] = s
	}
	if bySrc[srcSnapshots].OK || bySrc[srcBackups].OK {
		t.Fatalf("both reads failed: %+v", bySrc)
	}
	if strings.Contains(bySrc[srcSnapshots].Note, "/data") || strings.Contains(bySrc[srcSnapshots].Note, "locked") {
		t.Fatalf("the note must carry no path or driver text: %q", bySrc[srcSnapshots].Note)
	}
}

func TestFacetDeniedCallerGetsNoRowsAndANotPermittedSource(t *testing.T) {
	perms := &fakePerms{allowEnv: map[string]bool{}}
	for _, facet := range lxcFacets {
		res, err := facetProvider(facetFixture(), perms).ContributeFacet(context.Background(), facetReq(facet))
		if err != nil {
			t.Fatal(err)
		}
		rows := len(res.Inventory) + len(res.Capacity) + len(res.Protection) + len(res.Health) + len(res.Cost)
		if rows != 0 {
			t.Fatalf("%s: a denied caller got %d rows", facet, rows)
		}
		if len(res.Sources) != 1 || res.Sources[0].OK || res.Sources[0].Note != "not permitted" {
			t.Fatalf("%s: sources %+v", facet, res.Sources)
		}
	}
	for _, key := range perms.saw {
		if key != "ct.view" {
			t.Fatalf("gated on %q, want the key the list routes use", key)
		}
	}
}

func TestFacetOnlyPermittedHostsAreRead(t *testing.T) {
	perms := &fakePerms{allowEnv: map[string]bool{"h2": true}}
	res, err := facetProvider(facetFixture(), perms).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 1 || res.Inventory[0].ID != "c" || !hasNote(res, "may not view") {
		t.Fatalf("rows %+v notes %v", res.Inventory, res.Notes)
	}
}

func TestFacetPermissionErrorAndMissingUserDeny(t *testing.T) {
	errPerms := &fakePerms{allowEnv: map[string]bool{"h1": true, "h2": true}, err: errors.New("rbac down")}
	res, _ := facetProvider(facetFixture(), errPerms).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetInventory))
	if len(res.Inventory) != 0 || res.Sources[0].Note != "not permitted" {
		t.Fatalf("an errored check is a denial: %+v", res)
	}
	req := facetReq(plugin.ReportFacetInventory)
	req.UserID = ""
	res, _ = facetProvider(facetFixture(), allowAll()).ContributeFacet(context.Background(), req)
	if len(res.Inventory) != 0 || res.Sources[0].Note != "not permitted" {
		t.Fatalf("no caller, no rows: %+v", res)
	}
}

func TestFacetContainerListFailureIsANotOKSource(t *testing.T) {
	src := facetFixture()
	src.ctsErr = errors.New("listing containers: open /data/x.db: no such file")
	res, err := facetProvider(src, allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetHealth))
	if err != nil {
		t.Fatalf("a failed source must not fail the contribution: %v", err)
	}
	if len(res.Health) != 0 || len(res.Sources) != 1 || res.Sources[0].OK || strings.Contains(res.Sources[0].Note, "/data") {
		t.Fatalf("sources %+v", res.Sources)
	}
}

func TestFacetUnknownFacetIsRefused(t *testing.T) {
	_, err := facetProvider(facetFixture(), allowAll()).ContributeFacet(context.Background(), facetReq(plugin.ReportFacetExposure))
	if err == nil {
		t.Fatal("exposure is not contributed")
	}
}

func TestLXCSpecDeclaresFacetsAndKeepsOnlyIsolation(t *testing.T) {
	spec := lxcReportSpec()
	if len(spec.Types) != 1 || spec.Types[0].Key != "lxc-isolation" {
		t.Fatalf("types: %+v", spec.Types)
	}
	seen := map[string]bool{}
	for _, f := range spec.Facets {
		if f.Platform != "lxc" || f.PlatformLabelKey != "ct:reports.platform" {
			t.Fatalf("decl %+v", f)
		}
		seen[f.Facet] = true
	}
	if len(seen) != 5 || seen[plugin.ReportFacetExposure] {
		t.Fatalf("facets: %v", seen)
	}
}

func TestCPUCores(t *testing.T) {
	cases := map[string]float64{"": 0, "max": 0, "max 100000": 0, "150000 100000": 1.5, "50000": 0.5, "junk": 0}
	for in, want := range cases {
		if got := cpuCores(in); math.Abs(got-want) > 1e-9 {
			t.Errorf("%q: want %v got %v", in, want, got)
		}
	}
}

func hasNote(res plugin.ReportFacetResult, sub string) bool {
	for _, n := range res.Notes {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}
