// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// fakeRBAC grants every key except the ones listed in deny.
type fakeRBAC struct{ deny map[string]bool }

func (f fakeRBAC) HasPermission(_, _, perm string) (bool, error) { return !f.deny[perm], nil }
func (fakeRBAC) RegisterLabel(_, _, _ string)                    {}
func (fakeRBAC) UnregisterLabel(_ string)                        {}

// fakeSwarm is a canned cluster; a key in fail makes that listing error.
type fakeSwarm struct {
	overview ClusterOverview
	nodes    []Node
	services []ServiceSummary
	tasks    []Task
	networks []OverlayNetwork
	specs    map[string]ServiceResources
	fail     map[string]bool
}

var errFake = errors.New("dial unix /var/run/docker.sock: token=secret")

func (f *fakeSwarm) Overview(context.Context, string) (ClusterOverview, error) {
	return f.overview, nil
}

func (f *fakeSwarm) ListNodes(context.Context, string) ([]Node, error) {
	if f.fail["nodes"] {
		return nil, errFake
	}
	return f.nodes, nil
}

func (f *fakeSwarm) ListServices(context.Context, string) ([]ServiceSummary, error) {
	return f.services, nil
}

func (f *fakeSwarm) ListTasks(context.Context, string, TaskFilter) ([]Task, error) {
	return f.tasks, nil
}

func (f *fakeSwarm) ListOverlayNetworks(context.Context, string) ([]OverlayNetwork, error) {
	return f.networks, nil
}

func (f *fakeSwarm) InspectService(_ context.Context, _, id string) (ServiceDetail, error) {
	return ServiceDetail{Resources: f.specs[id]}, nil
}

func newCluster() *fakeSwarm {
	return &fakeSwarm{
		overview: ClusterOverview{
			Capability: Capability{Capable: true, ControlAvailable: true, NodeRole: "manager"},
			NodesTotal: 2, NodesReady: 1, NodesDown: 1, Managers: 1,
			ServicesTotal: 2, ServicesConverged: 1, ServicesDegraded: 1,
		},
		nodes: []Node{
			{ID: "n1", Hostname: "mgr1", Role: "manager", State: "ready", Availability: "active",
				Reachability: "reachable", IsLeader: true, NanoCPUs: 4e9, MemoryBytes: 8 << 30},
			{ID: "n2", Hostname: "wrk1", Role: "worker", State: "down", Availability: "active",
				NanoCPUs: 2e9, MemoryBytes: 4 << 30},
		},
		services: []ServiceSummary{
			{ID: "s1", Name: "web", Image: "nginx:1@sha256:abc", StackName: "shop", Running: 2, Desired: 2,
				Ports: []ServicePort{
					{Protocol: "tcp", TargetPort: 80, PublishedPort: 8080, PublishMode: "ingress"},
					{Protocol: "udp", TargetPort: 53, PublishedPort: 5353, PublishMode: "host"},
					{Protocol: "tcp", TargetPort: 9000},
				}},
			{ID: "s2", Name: "db", Image: "pg:16", StackName: "shop", Running: 0, Desired: 1},
		},
		tasks: []Task{
			{ServiceID: "s1", NodeID: "n1", State: "running"},
			{ServiceID: "s1", NodeID: "n1", State: "running"},
			{ServiceID: "s2", NodeID: "n2", State: "failed"},
		},
		networks: []OverlayNetwork{{Name: "shop_net", Driver: "overlay", Subnets: []string{"10.0.1.0/24"}}},
		specs: map[string]ServiceResources{
			"s1": {ReserveNanoCPUs: 5e8, ReserveMemoryBytes: 1 << 30},
		},
	}
}

func provider(src *fakeSwarm, deny ...string) *reportProvider {
	d := map[string]bool{}
	for _, k := range deny {
		d[k] = true
	}
	return &reportProvider{
		plugin: &SwarmPlugin{host: plugin.HostContext{RBAC: fakeRBAC{deny: d}}},
		src:    src,
	}
}

func facetReq(facet string) plugin.ReportFacetRequest {
	end := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	return plugin.ReportFacetRequest{
		Facet: facet, EnvironmentID: "env1", Environment: "Prod", UserID: "u1",
		PeriodStart: end.Add(-10 * time.Hour), PeriodEnd: end,
	}
}

func contribute(t *testing.T, p *reportProvider, facet string) plugin.ReportFacetResult {
	t.Helper()
	res, err := p.ContributeFacet(context.Background(), facetReq(facet))
	if err != nil {
		t.Fatalf("%s: %v", facet, err)
	}
	return res
}

func TestInventoryProjectsServices(t *testing.T) {
	res := contribute(t, provider(newCluster()), plugin.ReportFacetInventory)
	if len(res.Inventory) != 2 {
		t.Fatalf("rows = %d, want 2", len(res.Inventory))
	}
	db, web := res.Inventory[0], res.Inventory[1]
	if db.Name != "db" || db.State != "failed" {
		t.Errorf("db = %+v", db)
	}
	if web.Platform != "swarm" || web.Kind != "service" || web.Owner != "shop" || web.EnvironmentID != "env1" {
		t.Errorf("identity = %+v", web.FacetIdentity)
	}
	if web.State != "running" || web.Image != "nginx:1" || web.CPU != 1 || web.MemoryMB != 2048 {
		t.Errorf("web = %+v", web)
	}
}

func TestCapacityAllocatesReservationsToNodes(t *testing.T) {
	res := contribute(t, provider(newCluster()), plugin.ReportFacetCapacity)
	if len(res.Capacity) != 4 {
		t.Fatalf("rows = %d, want cpu+memory for 2 nodes", len(res.Capacity))
	}
	cpu, mem := res.Capacity[0], res.Capacity[1]
	if cpu.Kind != "node" || cpu.Resource != "cpu" || cpu.Allocated != 1 || cpu.Capacity != 4 || cpu.HasUsed {
		t.Errorf("cpu = %+v", cpu)
	}
	if mem.Resource != "memory" || mem.Allocated != 2 || mem.Capacity != 8 || mem.Unit != "GiB" {
		t.Errorf("mem = %+v", mem)
	}
}

func TestHealthStatusesAndQuorumBlocks(t *testing.T) {
	res := contribute(t, provider(newCluster()), plugin.ReportFacetHealth)
	got := map[string]string{}
	for _, r := range res.Health {
		got[r.Kind+"/"+r.Name] = r.Status
	}
	want := map[string]string{"node/mgr1": "up", "node/wrk1": "down", "service/web": "up", "service/db": "down"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	for _, r := range res.Health {
		if r.Name == "db" && r.Restarts != 1 {
			t.Errorf("db restarts = %d, want 1", r.Restarts)
		}
	}
	var text []string
	for _, b := range res.Blocks {
		text = append(text, b.Title, b.Text)
		for _, k := range b.KPIs {
			text = append(text, k.Label)
		}
	}
	joined := strings.Join(text, "|")
	if !strings.Contains(joined, "Manager quorum") || !strings.Contains(joined, "not fully converged") {
		t.Errorf("blocks missing quorum digest: %q", joined)
	}
}

func TestExposureRowsAndOverlayBlocks(t *testing.T) {
	res := contribute(t, provider(newCluster()), plugin.ReportFacetExposure)
	if len(res.Exposure) != 2 {
		t.Fatalf("rows = %d, want the two published ports", len(res.Exposure))
	}
	hostPort, mesh := res.Exposure[0], res.Exposure[1]
	if hostPort.Port != 5353 || hostPort.Protocol != "udp" || hostPort.Via != "port" || hostPort.Scope != "public" {
		t.Errorf("host port = %+v", hostPort)
	}
	if mesh.Port != 8080 || mesh.TargetPort != 80 || mesh.Via != "ingress" || mesh.Scope != "public" {
		t.Errorf("ingress port = %+v", mesh)
	}
	if len(res.Blocks) == 0 || res.Blocks[len(res.Blocks)-1].Kind != "table" {
		t.Errorf("blocks = %+v, want the overlay table", res.Blocks)
	}
}

func TestCostIsReservationTimesReplicasTimesHours(t *testing.T) {
	res := contribute(t, provider(newCluster()), plugin.ReportFacetCost)
	if len(res.Cost) != 1 {
		t.Fatalf("rows = %d, want only the reserving service", len(res.Cost))
	}
	c := res.Cost[0]
	if c.Basis != "allocated" || c.Name != "web" || c.CPUCoreHours != 10 || c.MemoryGBHours != 20 {
		t.Errorf("cost = %+v", c)
	}
}

func TestStandaloneDaemonContributesNothing(t *testing.T) {
	src := newCluster()
	src.overview.Capability = Capability{}
	for _, f := range []string{plugin.ReportFacetHealth, plugin.ReportFacetExposure, plugin.ReportFacetInventory} {
		res := contribute(t, provider(src), f)
		if len(res.Inventory)+len(res.Health)+len(res.Exposure) != 0 || len(res.Blocks) != 0 {
			t.Errorf("%s: standalone daemon produced %+v", f, res)
		}
	}
}

func TestDeniedCallerGetsNoRowsAndANotPermittedSource(t *testing.T) {
	res := contribute(t, provider(newCluster(), "swarm.view"), plugin.ReportFacetInventory)
	if len(res.Inventory) != 0 || len(res.Sources) != 1 || res.Sources[0].OK || res.Sources[0].Note != "not permitted" {
		t.Fatalf("denied result = %+v", res)
	}
}

func TestFailedSourceIsNotOKAndLeaksNothing(t *testing.T) {
	src := newCluster()
	src.fail = map[string]bool{"nodes": true}
	res := contribute(t, provider(src), plugin.ReportFacetHealth)
	var found bool
	for _, s := range res.Sources {
		if s.Key != srcSwarmNodes {
			continue
		}
		found = true
		if s.OK || strings.Contains(s.Note, "docker.sock") || strings.Contains(s.Note, "secret") || s.Note == "" {
			t.Errorf("nodes source = %+v", s)
		}
	}
	if !found {
		t.Fatalf("nodes source not recorded: %+v", res.Sources)
	}
}

func TestPerListingDenialIsRecordedNotCountedAsZero(t *testing.T) {
	res := contribute(t, provider(newCluster(), "swarm_nodes.view"), plugin.ReportFacetHealth)
	for _, s := range res.Sources {
		if s.Key == srcSwarmNodes && !s.OK {
			return
		}
	}
	t.Fatalf("a denied listing was not recorded as not-OK: %+v", res.Sources)
}

func TestDeclsAreFacetsOfSwarmPlatform(t *testing.T) {
	spec := swarmReportSpec()
	if len(spec.Facets) != 5 || len(spec.Types) != 1 || spec.Types[0].Key != reportSwarmCapacity {
		t.Fatalf("spec = %+v", spec)
	}
	for _, d := range spec.Facets {
		if d.Platform != "swarm" || d.Facet == plugin.ReportFacetProtection {
			t.Errorf("decl = %+v", d)
		}
	}
}
