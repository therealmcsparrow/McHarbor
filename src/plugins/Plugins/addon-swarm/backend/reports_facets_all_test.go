// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/services"
)

// fakeEnvs lists a fixed set of Docker environments.
type fakeEnvs struct{ ids []string }

func (f fakeEnvs) TriggerBindings(context.Context, string) ([]services.WorkflowTriggerBinding, error) {
	return nil, nil
}

func (f fakeEnvs) Fire(context.Context, string, map[string]any, func(services.WorkflowTriggerBinding) bool) (int, error) {
	return 0, nil
}

func (f fakeEnvs) DockerEnvironmentIDs(context.Context) ([]string, error) { return f.ids, nil }

// byEnv serves a different canned daemon per environment id.
type byEnv map[string]*fakeSwarm

func (b byEnv) Overview(ctx context.Context, e string) (ClusterOverview, error) {
	return b[e].Overview(ctx, e)
}

func (b byEnv) ListNodes(ctx context.Context, e string) ([]Node, error) {
	return b[e].ListNodes(ctx, e)
}

func (b byEnv) ListServices(ctx context.Context, e string) ([]ServiceSummary, error) {
	return b[e].ListServices(ctx, e)
}

func (b byEnv) ListTasks(ctx context.Context, e string, f TaskFilter) ([]Task, error) {
	return b[e].ListTasks(ctx, e, f)
}

func (b byEnv) ListOverlayNetworks(ctx context.Context, e string) ([]OverlayNetwork, error) {
	return b[e].ListOverlayNetworks(ctx, e)
}

func (b byEnv) InspectService(ctx context.Context, e, id string) (ServiceDetail, error) {
	return b[e].InspectService(ctx, e, id)
}

func allEnvsProvider(envs ...string) *reportProvider {
	p := provider(nil)
	p.plugin.host.Workflows = fakeEnvs{ids: envs}
	p.src = byEnv{
		"swarm1":     newCluster(),
		"standalone": {overview: ClusterOverview{}},
	}
	return p
}

func TestAllEnvironmentsFacetFansOutOverManagers(t *testing.T) {
	p := allEnvsProvider("standalone", "swarm1")
	req := facetReq(plugin.ReportFacetInventory)
	req.EnvironmentID, req.Environment = "", ""
	res, err := p.ContributeFacet(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inventory) != 2 {
		t.Fatalf("rows = %d, want the two services of swarm1", len(res.Inventory))
	}
	for _, row := range res.Inventory {
		if row.EnvironmentID != "swarm1" {
			t.Errorf("row env = %q, want swarm1", row.EnvironmentID)
		}
	}
	for _, s := range res.Sources {
		if !s.OK {
			t.Errorf("source %s not OK (%s): a standalone daemon must not read as a gap", s.Key, s.Note)
		}
	}
}

func TestAllEnvironmentsFacetWithNoManagerIsOneGap(t *testing.T) {
	p := allEnvsProvider("standalone")
	req := facetReq(plugin.ReportFacetHealth)
	req.EnvironmentID = ""
	res, err := p.ContributeFacet(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Health) != 0 || len(res.Sources) != 1 || res.Sources[0].OK {
		t.Fatalf("res = %+v", res)
	}
}
