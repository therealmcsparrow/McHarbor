// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

var facetNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type fakeFacetSource struct {
	guests    []VM
	disks     map[string][]Disk
	snapshots map[string][]Snapshot
	plans     map[string][]BackupPlan
	lastGood  map[string]time.Time
	health    map[string]guestHealth
	events    map[string][]vmEvent
	intents   map[string][]lifecycleIntent

	failGuests, failDisks, failEvents bool
}

var errFakeRead = errors.New("read failed at /var/lib/secret/host-7")

func (f *fakeFacetSource) List(context.Context, string, bool) ([]VM, error) {
	if f.failGuests {
		return nil, errFakeRead
	}
	return f.guests, nil
}

func (f *fakeFacetSource) Disks(_ context.Context, id string) ([]Disk, error) {
	if f.failDisks {
		return nil, errFakeRead
	}
	return f.disks[id], nil
}

func (f *fakeFacetSource) Snapshots(_ context.Context, id string) ([]Snapshot, error) {
	return f.snapshots[id], nil
}

func (f *fakeFacetSource) ListBackupPlans(_ context.Context, id string) ([]BackupPlan, error) {
	return f.plans[id], nil
}

func (f *fakeFacetSource) lastGoodBackupAt(_ context.Context, id string) (time.Time, error) {
	return f.lastGood[id], nil
}

func (f *fakeFacetSource) guestHealthRows(context.Context, string) (map[string]guestHealth, error) {
	return f.health, nil
}

func (f *fakeFacetSource) readEvents(context.Context, string, time.Time) (map[string][]vmEvent, error) {
	if f.failEvents {
		return nil, errFakeRead
	}
	return f.events, nil
}

func (f *fakeFacetSource) readIntents(context.Context, string, time.Time) (map[string][]lifecycleIntent, error) {
	return f.intents, nil
}

type fakeRBAC struct{ grants map[string]bool }

func (f fakeRBAC) HasPermission(_, env, perm string) (bool, error) {
	return f.grants[env+"|"+perm], nil
}

func allowAll(envs ...string) fakeRBAC {
	grants := map[string]bool{}
	for _, env := range envs {
		grants[env+"|vm.view"] = true
		grants[env+"|vm.backup.view"] = true
	}
	return fakeRBAC{grants: grants}
}

func testCollector(src *fakeFacetSource, rbac permissionChecker) facetCollector {
	return facetCollector{src: src, rbac: rbac, now: func() time.Time { return facetNow }}
}

func facetRequest(facet string) plugin.ReportFacetRequest {
	return plugin.ReportFacetRequest{
		Facet: facet, UserID: "u1",
		PeriodStart: facetNow.Add(-24 * time.Hour), PeriodEnd: facetNow,
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func sampleSource() *fakeFacetSource {
	return &fakeFacetSource{
		guests: []VM{
			{ID: "v1", Name: "web", EnvironmentID: "h1", State: StateRunning, VCPUs: 4, MemoryMB: 4096,
				HostAvailable: true, GuestOSName: "Debian", CreatedAt: facetNow.Add(-72 * time.Hour),
				LastStartedAt: timePtr(facetNow.Add(-10 * time.Hour))},
			{ID: "v2", Name: "db", EnvironmentID: "h1", State: StateStopped, VCPUs: 2, MemoryMB: 2048,
				HostAvailable: true, CreatedAt: facetNow.Add(-72 * time.Hour)},
			{ID: "v3", Name: "lost", EnvironmentID: "h2", State: StateRunning, VCPUs: 8, MemoryMB: 8192,
				HostAvailable: false},
		},
		disks: map[string][]Disk{
			"v1": {{PoolID: "p1", SizeBytes: 10 * bytesPerGB}, {PoolID: "p1", SizeBytes: 5 * bytesPerGB, IsCDROM: true}},
			"v2": {{PoolID: "p2", SizeBytes: 20 * bytesPerGB}},
		},
		snapshots: map[string][]Snapshot{
			"v1": {{CreatedAt: facetNow.Add(-2 * time.Hour), Consistency: ConsistencyCrash}},
		},
		lastGood: map[string]time.Time{"v2": facetNow.Add(-5 * time.Hour)},
	}
}

func TestInventoryFacetRows(t *testing.T) {
	res, err := testCollector(sampleSource(), allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 3 {
		t.Fatalf("want 3 rows, got %d", len(res.Inventory))
	}
	web := res.Inventory[0]
	if web.Platform != "kvm" || web.Kind != "vm" || web.Name != "web" || web.State != StateRunning ||
		web.CPU != 4 || web.MemoryMB != 4096 || web.Image != "Debian" || web.Host != "h1" {
		t.Fatalf("unexpected row %+v", web)
	}
	if web.DiskGB != 10 {
		t.Fatalf("optical media must not count as disk, got %v GB", web.DiskGB)
	}
}

func TestCapacityFacetAllocatesRunningGuestsAndPools(t *testing.T) {
	res, err := testCollector(sampleSource(), allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetCapacity))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, row := range res.Capacity {
		got[row.Kind+"/"+row.ID+"/"+row.Resource] = row.Allocated
		if row.HasUsed || row.Capacity != 0 {
			t.Fatalf("the host's capacity is unknown and must not be invented: %+v", row)
		}
	}
	want := map[string]float64{
		"host/h1/cpu": 4, "host/h1/memory": 4096, "pool/p1/disk": 10, "pool/p2/disk": 20,
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %v, want %v (all: %v)", key, got[key], value, got)
		}
	}
	if _, found := got["host/h2/cpu"]; found {
		t.Errorf("a guest on an unreachable host must be excluded")
	}
}

func TestProtectionFacetMethods(t *testing.T) {
	res, err := testCollector(sampleSource(), allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetProtection))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]plugin.ReportProtectionRow{}
	for _, row := range res.Protection {
		rows[row.Name] = row
	}
	web, db := rows["web"], rows["db"]
	if !web.Protected || web.Method != "snapshot" || web.OffHost || web.LastGood.IsZero() {
		t.Errorf("a snapshot-only guest is protected but not off-host: %+v", web)
	}
	if !db.Protected || db.Method != "backup" || !db.OffHost || !db.LastGood.Equal(facetNow.Add(-5*time.Hour)) {
		t.Errorf("a backed-up guest is off-host: %+v", db)
	}
	if _, found := rows["lost"]; found {
		t.Errorf("unreachable guest must be excluded")
	}
}

func TestProtectionRowNone(t *testing.T) {
	row := protectionRow(plugin.ReportFacetIdentity{}, nil, 0, time.Time{})
	if row.Protected || row.Method != "none" {
		t.Fatalf("no copy at all must read as none: %+v", row)
	}
	planned := protectionRow(plugin.ReportFacetIdentity{}, nil, 1, time.Time{})
	if planned.Protected || planned.Note == row.Note {
		t.Fatalf("a plan that never succeeded is still unprotected, with its own note: %+v", planned)
	}
}

func TestHealthFacetStatusAndFlaps(t *testing.T) {
	src := sampleSource()
	src.guests[0].State = StateRunning
	src.guests[2].State = StateRunning
	panicAt := func(min int) vmEvent {
		return vmEvent{at: facetNow.Add(-time.Duration(min) * time.Minute), kind: qemuEventGuestPanic}
	}
	src.events = map[string][]vmEvent{"v2": {panicAt(50), panicAt(40), panicAt(30)}}
	src.guests[1].State = StateCrashed

	res, err := testCollector(src, allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetHealth))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]plugin.ReportHealthRow{}
	for _, row := range res.Health {
		rows[row.Name] = row
	}
	if rows["web"].Status != "up" {
		t.Errorf("web = %+v", rows["web"])
	}
	if rows["db"].Status != "down" || rows["db"].Restarts != 3 || rows["db"].Flaps != 3 {
		t.Errorf("a crash-looping guest is down with its flaps counted: %+v", rows["db"])
	}
	if rows["lost"].Status != "unknown" {
		t.Errorf("an unreachable host is unknown, not up: %+v", rows["lost"])
	}
}

func TestGuestStatusDegradedOnUnresponsiveAgent(t *testing.T) {
	vm := VM{State: StateRunning, HostAvailable: true}
	status, _ := guestStatus(vm, stabilityVMRow{agentStatus: "danger", agentText: "unresponsive"}, nil)
	if status != "degraded" {
		t.Fatalf("got %s", status)
	}
	if status, _ := guestStatus(vm, stabilityVMRow{}, &guestHealth{diskAlert: true}); status != "degraded" {
		t.Fatalf("a full guest filesystem degrades, got %s", status)
	}
}

func TestCostFacetIsAllocatedHours(t *testing.T) {
	res, err := testCollector(sampleSource(), allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetCost))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]plugin.ReportCostRow{}
	for _, row := range res.Cost {
		rows[row.Name] = row
		if row.Basis != "allocated" {
			t.Fatalf("basis must be allocated: %+v", row)
		}
	}
	web, db := rows["web"], rows["db"]
	// Started 10h ago inside a 24h window: 4 vCPU and 4 GB for 10 hours, 10 GB disk for 24.
	if web.CPUCoreHours != 40 || web.MemoryGBHours != 40 || math.Abs(web.DiskGBHours-240) > 1e-9 {
		t.Errorf("web = %+v", web)
	}
	if db.CPUCoreHours != 0 || db.MemoryGBHours != 0 || math.Abs(db.DiskGBHours-480) > 1e-9 {
		t.Errorf("a stopped guest burns disk only: %+v", db)
	}
}

func TestExposureIsNotDeclared(t *testing.T) {
	for _, decl := range kvmFacetDecls() {
		if decl.Facet == plugin.ReportFacetExposure {
			t.Fatal("kvm models no port forwards; it must not declare exposure")
		}
		if decl.Platform != "kvm" {
			t.Fatalf("platform = %q", decl.Platform)
		}
	}
}

func TestFacetDeniedCallerGetsNoRows(t *testing.T) {
	for _, facet := range []string{plugin.ReportFacetInventory, plugin.ReportFacetProtection} {
		req := facetRequest(facet)
		req.EnvironmentID = "h1"
		res, err := testCollector(sampleSource(), fakeRBAC{}).contribute(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Inventory)+len(res.Protection) != 0 {
			t.Fatalf("%s: a denied caller must get zero rows", facet)
		}
		if len(res.Sources) != 1 || res.Sources[0].OK || res.Sources[0].Note != "not permitted" {
			t.Fatalf("%s: want one not-permitted source, got %+v", facet, res.Sources)
		}
	}
}

func TestFacetProtectionNeedsBackupView(t *testing.T) {
	rbac := fakeRBAC{grants: map[string]bool{"h1|vm.view": true}}
	req := facetRequest(plugin.ReportFacetProtection)
	req.EnvironmentID = "h1"
	res, _ := testCollector(sampleSource(), rbac).contribute(context.Background(), req)
	if len(res.Protection) != 0 {
		t.Fatal("vm.view alone must not reveal backup state")
	}
}

func TestFacetWithoutRBACOrUserIsDenied(t *testing.T) {
	res, _ := testCollector(sampleSource(), nil).contribute(context.Background(), facetRequest(plugin.ReportFacetInventory))
	if len(res.Inventory) != 0 {
		t.Fatal("no RBAC handle means no answer, and no answer means no")
	}
	anon := facetRequest(plugin.ReportFacetInventory)
	anon.UserID = ""
	res, _ = testCollector(sampleSource(), allowAll("h1", "h2")).contribute(context.Background(), anon)
	if len(res.Inventory) != 0 {
		t.Fatal("a request with no user must be denied")
	}
}

func TestFacetFiltersHostsTheCallerCannotSee(t *testing.T) {
	res, err := testCollector(sampleSource(), allowAll("h1")).contribute(context.Background(), facetRequest(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 2 {
		t.Fatalf("want only h1's guests, got %d", len(res.Inventory))
	}
	withheld := false
	for _, source := range res.Sources {
		if !source.OK && source.Note == "not permitted" {
			withheld = true
		}
	}
	if !withheld {
		t.Fatal("a partial denial must still say so in the annex")
	}
}

func TestFacetFailedGuestReadIsNotOKSource(t *testing.T) {
	src := sampleSource()
	src.failGuests = true
	res, err := testCollector(src, allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 0 || len(res.Sources) == 0 || res.Sources[0].OK || res.Sources[0].Key != "kvm_guests" {
		t.Fatalf("a failed read must be a not-OK source with no rows: %+v", res.Sources)
	}
	if note := res.Sources[0].Note; note == "" || note == errFakeRead.Error() {
		t.Fatalf("the note must be a fixed line that carries no path: %q", note)
	}
}

func TestFacetFailedDiskReadIsNotOKSource(t *testing.T) {
	src := sampleSource()
	src.failDisks = true
	res, _ := testCollector(src, allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetInventory))
	var disks *plugin.ReportSourceStatus
	for i := range res.Sources {
		if res.Sources[i].Key == "kvm_disks" {
			disks = &res.Sources[i]
		}
	}
	if disks == nil || disks.OK || disks.Note == "" {
		t.Fatalf("a failed disk read must be recorded as not OK: %+v", res.Sources)
	}
}

func TestFacetFailedEventReadIsNotOKSource(t *testing.T) {
	src := sampleSource()
	src.failEvents = true
	res, _ := testCollector(src, allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetHealth))
	found := false
	for _, source := range res.Sources {
		if source.Key == "kvm_vm_events" && !source.OK && source.Note != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a failed event read must be a not-OK source: %+v", res.Sources)
	}
	if len(res.Health) != 3 {
		t.Fatal("health rows are still produced from state alone")
	}
}

func TestFacetUnknownFacetIsAnError(t *testing.T) {
	if _, err := testCollector(sampleSource(), allowAll("h1", "h2")).contribute(context.Background(), facetRequest(plugin.ReportFacetExposure)); err == nil {
		t.Fatal("exposure is not contributed and must be refused")
	}
}
