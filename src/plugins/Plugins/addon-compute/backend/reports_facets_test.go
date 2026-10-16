// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type fakeFacetSource struct {
	envs      []ComputeEnvironment
	envsErr   error
	pools     map[string][]Pool
	poolErr   map[string]error
	images    []Image
	imagesErr error
}

func (f fakeFacetSource) ListEnvironments(context.Context) ([]ComputeEnvironment, error) {
	return f.envs, f.envsErr
}

func (f fakeFacetSource) ListPools(_ context.Context, envID string) ([]Pool, error) {
	if err := f.poolErr[envID]; err != nil {
		return nil, err
	}
	return f.pools[envID], nil
}

func (f fakeFacetSource) ListImages(context.Context, string, string) ([]Image, error) {
	return f.images, f.imagesErr
}

type grants map[string]bool

func (g grants) HasPermission(userID, envID, perm string) (bool, error) {
	return g[userID+"|"+envID+"|"+perm], nil
}

type brokenGrants struct{}

func (brokenGrants) HasPermission(string, string, string) (bool, error) {
	return false, errors.New("lookup failed")
}

func facetFixture() fakeFacetSource {
	probed := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	return fakeFacetSource{
		envs: []ComputeEnvironment{
			{
				EnvironmentID: "e1", Name: "alpha", ComputeEnabled: true, Available: true, ProbedAt: &probed,
				Capability: &Capability{
					OS: "linux", Arch: "amd64", HostCPUs: 16, HostMemoryBytes: 32 << 30,
					Items: []CapabilityItem{{Key: "kvm", Critical: true, Present: true}},
				},
			},
			{
				EnvironmentID: "e2", Name: "beta", ComputeEnabled: true, Available: true,
				Capability: &Capability{Items: []CapabilityItem{
					{Key: "qemu", Critical: true, Present: false},
					{Key: "opt", Critical: false, Present: false},
				}},
			},
			{EnvironmentID: "e3", Name: "gamma", ComputeEnabled: true, Available: false, UnavailableReason: "agent_too_old"},
			{EnvironmentID: "e4", Name: "delta", ComputeEnabled: true, Available: true},
			{EnvironmentID: "e5", Name: "off", ComputeEnabled: false, Available: true},
		},
		pools: map[string][]Pool{
			"e1": {{ID: "p1", Name: "fast", Backend: "zfs", CapacityBytes: 100 << 30, UsedBytes: 40 << 30, AllocatedBytes: 150 << 30, State: "ready"}},
		},
		images: []Image{{ID: "i1", Kind: "vm", Distro: "debian", Release: "12", Arch: "amd64", DisplayName: "Debian 12", SizeBytes: 2 << 30}},
	}
}

func allowAll() grants {
	g := grants{}
	for _, e := range []string{"e1", "e2", "e3", "e4"} {
		g["u1|"+e+"|compute.view"] = true
	}
	return g
}

func facetReq(facet string) plugin.ReportFacetRequest {
	return plugin.ReportFacetRequest{Facet: facet, UserID: "u1"}
}

func TestInventoryFacetRows(t *testing.T) {
	res, err := collectComputeFacet(context.Background(), facetFixture(), allowAll(), facetReq(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string]int{}
	for _, row := range res.Inventory {
		byKind[row.Kind]++
		if row.Platform != computePlatform {
			t.Errorf("row %q stamped %q, want %q", row.Name, row.Platform, computePlatform)
		}
	}
	if byKind["host"] != 4 || byKind["pool"] != 1 || byKind["image"] != 1 {
		t.Fatalf("rows by kind = %v, want 4 hosts, 1 pool, 1 image", byKind)
	}
	host := res.Inventory[0]
	if host.Name != "alpha" || host.State != "ready" || host.CPU != 16 || host.MemoryMB != 32768 || host.Image != "linux / amd64" {
		t.Errorf("alpha host row = %+v", host)
	}
	var unreachable bool
	for _, row := range res.Inventory {
		if row.Name == "gamma" {
			unreachable = row.State == "unreachable"
		}
	}
	if !unreachable {
		t.Error("an unreachable host must stay in the inventory as unreachable")
	}
	pool := res.Inventory[1]
	if pool.Kind != "pool" || pool.Owner != "alpha" || pool.DiskGB != 100 || pool.Image != "zfs" {
		t.Errorf("pool row = %+v", pool)
	}
	img := res.Inventory[len(res.Inventory)-1]
	if img.Kind != "image" || img.Name != "Debian 12" || img.Image != "debian 12 amd64" || img.DiskGB != 2 || img.State != "catalog" {
		t.Errorf("image row = %+v", img)
	}
	for _, s := range res.Sources {
		if !s.OK {
			t.Errorf("source %s not OK: %s", s.Key, s.Note)
		}
	}
}

func TestCapacityFacetRows(t *testing.T) {
	src := facetFixture()
	src.pools["e2"] = []Pool{{ID: "p2", Name: "unsampled", Backend: "dir", State: "ready"}}
	res, err := collectComputeFacet(context.Background(), src, allowAll(), facetReq(plugin.ReportFacetCapacity))
	if err != nil {
		t.Fatal(err)
	}
	if last := res.Capacity[len(res.Capacity)-1]; last.Name != "unsampled" || last.HasUsed {
		t.Errorf("a pool with no capacity must not claim a measured use: %+v", last)
	}
	res.Capacity = res.Capacity[:3]
	if len(res.Capacity) != 3 {
		t.Fatalf("capacity rows = %d, want cpu+memory for the probed host and one pool", len(res.Capacity))
	}
	cpu, mem, disk := res.Capacity[0], res.Capacity[1], res.Capacity[2]
	if cpu.Resource != "cpu" || cpu.Capacity != 16 || cpu.Unit != "cores" || cpu.HasUsed {
		t.Errorf("cpu row = %+v", cpu)
	}
	if mem.Resource != "memory" || mem.Capacity != 32 || mem.Unit != "GiB" {
		t.Errorf("memory row = %+v", mem)
	}
	if disk.Resource != "disk" || disk.Kind != "pool" || disk.Capacity != 100 || disk.Used != 40 || !disk.HasUsed || disk.Allocated != 150 {
		t.Errorf("disk row = %+v", disk)
	}
}

func TestHealthFacetStatuses(t *testing.T) {
	res, err := collectComputeFacet(context.Background(), facetFixture(), allowAll(), facetReq(plugin.ReportFacetHealth))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"alpha": "up", "beta": "degraded", "gamma": "down", "delta": "unknown"}
	if len(res.Health) != len(want) {
		t.Fatalf("health rows = %d, want %d", len(res.Health), len(want))
	}
	for _, row := range res.Health {
		if want[row.Name] != row.Status {
			t.Errorf("%s status = %q, want %q", row.Name, row.Status, want[row.Name])
		}
	}
}

func TestFacetDeniedCallerGetsNothing(t *testing.T) {
	for _, perms := range []facetPermissions{grants{}, nil, brokenGrants{}} {
		res, err := collectComputeFacet(context.Background(), facetFixture(), perms, facetReq(plugin.ReportFacetInventory))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Inventory)+len(res.Capacity)+len(res.Health) != 0 {
			t.Errorf("a denied caller received rows: %+v", res)
		}
		if len(res.Sources) != 1 || res.Sources[0].OK || res.Sources[0].Note != "not permitted" {
			t.Errorf("sources = %+v, want one not-OK 'not permitted'", res.Sources)
		}
	}
}

func TestFacetPartialPermissionOmitsHosts(t *testing.T) {
	g := grants{"u1|e1|compute.view": true}
	res, err := collectComputeFacet(context.Background(), facetFixture(), g, facetReq(plugin.ReportFacetHealth))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Health) != 1 || res.Health[0].Name != "alpha" {
		t.Errorf("health rows = %+v, want only alpha", res.Health)
	}
	if len(res.Notes) == 0 {
		t.Error("omitting hosts the caller may not view must be noted")
	}
}

func TestFacetFailedSourcesAreRecorded(t *testing.T) {
	src := facetFixture()
	src.poolErr = map[string]error{"e1": errors.New("dial tcp 10.0.0.5:8705: refused")}
	src.imagesErr = errors.New("db down")
	res, err := collectComputeFacet(context.Background(), src, allowAll(), facetReq(plugin.ReportFacetInventory))
	if err != nil {
		t.Fatal(err)
	}
	failed := map[string]plugin.ReportSourceStatus{}
	for _, s := range res.Sources {
		if !s.OK {
			failed[s.Key] = s
		}
	}
	if len(failed) != 2 || failed["compute_pools"].Note == "" || failed["compute_images"].Note == "" {
		t.Fatalf("failed sources = %+v, want pools and images not-OK with notes", failed)
	}
	for _, s := range failed {
		if s.Note == "dial tcp 10.0.0.5:8705: refused" || s.Note == "db down" {
			t.Errorf("source note leaks the raw error: %q", s.Note)
		}
	}
	for _, row := range res.Inventory {
		if row.Kind == "pool" || row.Kind == "image" {
			t.Errorf("a failed read produced a %s row", row.Kind)
		}
	}

	down := fakeFacetSource{envsErr: errors.New("boom")}
	res, err = collectComputeFacet(context.Background(), down, allowAll(), facetReq(plugin.ReportFacetHealth))
	if err != nil || len(res.Sources) != 1 || res.Sources[0].OK {
		t.Errorf("host list failure: err=%v sources=%+v", err, res.Sources)
	}
}

func TestFacetScopedToEnvironment(t *testing.T) {
	req := facetReq(plugin.ReportFacetHealth)
	req.EnvironmentID = "e2"
	res, err := collectComputeFacet(context.Background(), facetFixture(), allowAll(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Health) != 1 || res.Health[0].EnvironmentID != "e2" {
		t.Errorf("scoped health rows = %+v", res.Health)
	}
}

func TestFacetUnknownRefused(t *testing.T) {
	if _, err := collectComputeFacet(context.Background(), facetFixture(), allowAll(), facetReq(plugin.ReportFacetCost)); err == nil {
		t.Error("a facet this plugin does not contribute must be refused")
	}
}
