// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type fakeReportClient struct{}

func (fakeReportClient) Build(context.Context, plugin.ReportRequest) (plugin.ReportResult, error) {
	return plugin.ReportResult{}, nil
}

func (fakeReportClient) Subjects(context.Context, string, plugin.ReportRequest) ([]plugin.ReportSubjectOption, error) {
	return nil, nil
}

type fakeFacetClient struct {
	fakeReportClient
	platform string
	err      error
	panics   bool
}

func (f fakeFacetClient) ContributeFacet(_ context.Context, req plugin.ReportFacetRequest) (plugin.ReportFacetResult, error) {
	if f.panics {
		panic("boom")
	}
	if f.err != nil {
		return plugin.ReportFacetResult{}, f.err
	}
	row := plugin.ReportInventoryRow{}
	row.Platform, row.Name = f.platform, req.Facet+"-"+req.UserID
	return plugin.ReportFacetResult{Inventory: []plugin.ReportInventoryRow{row}}, nil
}

func facetSpec(slug, platform string, facets ...string) plugin.ReportProviderSpec {
	spec := plugin.ReportProviderSpec{Slug: slug, DisplayName: slug}
	for _, f := range facets {
		spec.Facets = append(spec.Facets, plugin.ReportFacetDecl{Facet: f, Platform: platform})
	}
	return spec
}

func TestReportingFacetRegisterValidation(t *testing.T) {
	cases := []struct {
		name    string
		spec    plugin.ReportProviderSpec
		client  plugin.ReportProviderClient
		wantErr string
	}{
		{"facet-only spec is accepted", facetSpec("a", "kvm", plugin.ReportFacetInventory), fakeFacetClient{platform: "kvm"}, ""},
		{"no types and no facets", facetSpec("a", "kvm"), fakeFacetClient{}, "at least one report type or facet"},
		{"unknown facet", facetSpec("a", "kvm", "weather"), fakeFacetClient{}, "is not one of"},
		{"bad platform token", facetSpec("a", "KVM!", plugin.ReportFacetHealth), fakeFacetClient{}, "platform"},
		{"duplicate pair in one spec", facetSpec("a", "kvm", plugin.ReportFacetHealth, plugin.ReportFacetHealth), fakeFacetClient{}, "declared twice"},
		{"client cannot contribute", facetSpec("a", "kvm", plugin.ReportFacetHealth), fakeReportClient{}, "does not implement ContributeFacet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := newAdapterRegistry().Reporting().Register(tc.spec, tc.client)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestReportingFacetPairHasOneOwner(t *testing.T) {
	reg := newAdapterRegistry()
	if err := reg.Reporting().Register(facetSpec("addon-a", "kvm", plugin.ReportFacetHealth), fakeFacetClient{}); err != nil {
		t.Fatal(err)
	}
	err := reg.Reporting().Register(facetSpec("addon-b", "kvm", plugin.ReportFacetHealth), fakeFacetClient{})
	if err == nil || !strings.Contains(err.Error(), "already registered by addon-a") {
		t.Fatalf("second owner of health/kvm must be refused, got %v", err)
	}
}

func TestReportingCollectFacetIsolatesFailures(t *testing.T) {
	reg := newAdapterRegistry()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.Reporting().Register(facetSpec("addon-kvm", "kvm", plugin.ReportFacetInventory), fakeFacetClient{platform: "kvm"}))
	must(reg.Reporting().Register(facetSpec("addon-bad", "bad", plugin.ReportFacetInventory), fakeFacetClient{err: errors.New("down")}))
	must(reg.Reporting().Register(facetSpec("addon-pan", "pan", plugin.ReportFacetInventory), fakeFacetClient{panics: true}))
	must(reg.Reporting().Register(facetSpec("addon-hl", "hl", plugin.ReportFacetHealth), fakeFacetClient{platform: "hl"}))

	h, ok := reportingHandleFor(reg).(plugin.ReportFacetHandle)
	if !ok {
		t.Fatal("reporting handle must implement ReportFacetHandle")
	}
	got := h.CollectFacet(context.Background(), plugin.ReportFacetInventory, plugin.ReportFacetRequest{UserID: "u1"})
	if len(got) != 3 {
		t.Fatalf("want 3 inventory contributions (health provider excluded), got %d", len(got))
	}
	if got[0].Err != nil || len(got[0].Result.Inventory) != 1 || got[0].Result.Inventory[0].Name != "inventory-u1" {
		t.Fatalf("kvm contribution wrong: %+v", got[0])
	}
	if got[1].Err == nil || got[2].Err == nil || !errors.Is(got[2].Err, errFacetPanic) {
		t.Fatalf("failing and panicking providers must surface as errors: %v / %v", got[1].Err, got[2].Err)
	}

	reg.removeFor("addon-kvm")
	if n := len(h.FacetProviders(plugin.ReportFacetInventory)); n != 2 {
		t.Fatalf("uninstall must prune the provider's facets, still %d", n)
	}
}
