// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"testing"
)

// TestInfoViewMatchesService guards the clusterinfo boundary health, compliance
// and support read: no registry is a nil Membership (not a typed nil, which
// would panic their nil checks), and checks and nodes arrive unchanged.
func TestInfoViewMatchesService(t *testing.T) {
	database := migrated(t)
	ctx := context.Background()
	svc := NewService(database, nil, quiet(), nil)

	if m := svc.Info().Membership(); m != nil {
		t.Fatalf("Membership with no registry = %#v, want a nil interface", m)
	}

	reg := NewRegistry(database, "node-a", "http://a:8705", func() int { return 3 }, quiet())
	if err := reg.Beat(ctx); err != nil {
		t.Fatal(err)
	}
	svc.SetRegistry(reg)

	wantOK, wantChecks := svc.Ready(ctx)
	gotOK, gotChecks := svc.Info().Ready(ctx)
	if gotOK != wantOK || len(gotChecks) != len(wantChecks) {
		t.Fatalf("Info().Ready = %v %v, Service.Ready = %v %v", gotOK, gotChecks, wantOK, wantChecks)
	}
	for i := range wantChecks {
		if gotChecks[i].Name != wantChecks[i].Name || gotChecks[i].OK != wantChecks[i].OK {
			t.Fatalf("check %d = %+v, want %+v", i, gotChecks[i], wantChecks[i])
		}
	}

	m := svc.Info().Membership()
	if m == nil || m.NodeID() != "node-a" || m.Role() != reg.Role() || m.State() != reg.State() {
		t.Fatalf("Membership identity = %#v, want node-a / %s / %s", m, reg.Role(), reg.State())
	}
	nodes, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := reg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != len(want) || len(nodes) == 0 || nodes[0].ID != want[0].ID || nodes[0].Agents != want[0].Agents {
		t.Fatalf("Membership.List = %+v, Registry.List = %+v", nodes, want)
	}
}

// TestSharedSecretCheckReadsTheLoadedConfig: main removes MCHARBOR_SECRET from the
// environment once config has loaded, so the migration check must report the configured
// secret from what main recorded, not from the (now empty) variable.
func TestSharedSecretCheckReadsTheLoadedConfig(t *testing.T) {
	t.Setenv("MCHARBOR_SECRET", "")
	svc := NewService(migrated(t), nil, quiet(), nil)
	status := func() string {
		for _, c := range svc.MigrationPlan(context.Background()).Checks {
			if c.ID == "sharedSecret" {
				return c.Status
			}
		}
		return ""
	}
	unset := status()
	svc.SetSecretConfigured(true)
	if got := status(); got == unset || got != statusFor(true) {
		t.Fatalf("a configured secret with the variable removed must pass the check, got %q (unset %q)", got, unset)
	}
}
