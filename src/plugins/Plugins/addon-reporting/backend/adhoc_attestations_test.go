// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sort"
	"testing"
)

// TestAttestationSourceProjectsExactlyItsCatalogColumns holds the two planner
// invariants for the sign-off source: it projects real rows, and every row has
// exactly the columns the catalog promised.
func TestAttestationSourceProjectsExactlyItsCatalogColumns(t *testing.T) {
	snap := &Snapshot{}
	snap.Governance.Packs.Attestations = []packAttestationRow{
		{ID: "a2", Framework: "access-review", AttestedByName: "ann", AttestedAt: "2026-09-27T10:00:00Z", Note: "Q3 review, corrected"},
		{ID: "a1", Framework: "access-review", AttestedByName: "ann", AttestedAt: "2026-09-01T10:00:00Z", Note: "Q3 review"},
	}
	var promised []string
	for _, d := range sourceCatalog() {
		if d.Key == srcComplianceAttestations {
			promised = append([]string(nil), d.Columns...)
		}
	}
	if len(promised) == 0 {
		t.Fatal("the attestation source is not in the planner's catalog")
	}
	sort.Strings(promised)

	rows, ok := projectSource(snap, srcComplianceAttestations, 10).([]map[string]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("projected %v; a superseded sign-off must still be reported", rows)
	}
	for _, row := range rows {
		got := make([]string, 0, len(row))
		for k := range row {
			got = append(got, k)
		}
		sort.Strings(got)
		if len(got) != len(promised) {
			t.Fatalf("row columns %v, catalog promises %v", got, promised)
		}
		for i := range got {
			if got[i] != promised[i] {
				t.Fatalf("row columns %v, catalog promises %v", got, promised)
			}
		}
	}
	if _, ok := packJobs(context.Background(), nil, snap)[srcComplianceAttestations]; !ok {
		t.Fatal("no collector job fills the attestation source")
	}
}
