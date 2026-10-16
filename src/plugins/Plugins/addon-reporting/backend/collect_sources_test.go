// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// sourceStatus finds one source's outcome in a stand-in collection.
func sourceStatus(t *testing.T, api *hostAPI, key string) SourceStatus {
	t.Helper()
	for _, s := range api.sources() {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("source %q was never collected", key)
	return SourceStatus{}
}

func TestPluginsSourceUsesTheAppstoreRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/appstore/plugins/installed" {
			http.NotFound(w, r)
			return
		}
		writeSourceBody(t, w, `{"success":true,"data":[{"slug":"addon-kvm","version":"2026.09.01","enabled":true,"loaded":true},{"slug":"addon-lxc","version":"2026.09.02","enabled":false,"loaded":false}]}`)
	}))
	defer srv.Close()
	api := newHostAPI("", "en", "")
	api.baseURLs = []string{srv.URL}
	snap := &Snapshot{}
	collectJobs(context.Background(), api, snap, Period{})[srcPlugins]()
	if st := sourceStatus(t, api, srcPlugins); !st.OK {
		t.Fatalf("plugins source not ok: %q", st.Note)
	}
	if len(snap.Plugins) != 2 || snap.Plugins[0].Version != "2026.09.01" || !snap.Plugins[0].Enabled || snap.Plugins[1].Enabled {
		t.Fatalf("plugins = %+v", snap.Plugins)
	}
}

func TestMetricsHistoryOfAPointInTimeReportIsANonEmptyWindow(t *testing.T) {
	var gotFrom, gotTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/metrics/host/history" {
			gotFrom, gotTo = r.URL.Query().Get("from"), r.URL.Query().Get("to")
			from, _ := time.Parse(time.RFC3339, gotFrom)
			to, _ := time.Parse(time.RFC3339, gotTo)
			if !to.After(from) {
				http.Error(w, "empty window", http.StatusBadRequest)
				return
			}
		}
		writeSourceBody(t, w, `{"success":true,"data":{}}`)
	}))
	defer srv.Close()
	api := newHostAPI("", "en", "")
	api.baseURLs = []string{srv.URL}
	now := time.Date(2026, 10, 1, 14, 46, 59, 0, time.UTC)
	collectJobs(context.Background(), api, &Snapshot{}, Period{Start: now, End: now, PointInTime: true})[srcMetrics]()
	if st := sourceStatus(t, api, srcMetricsHistory); !st.OK {
		t.Fatalf("metrics_history not ok: %q (from=%s to=%s)", st.Note, gotFrom, gotTo)
	}
	if gotTo != now.Format(time.RFC3339) {
		t.Fatalf("to = %q", gotTo)
	}
}

func TestMetricsHistoryKeepsARealPeriod(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	p := Period{Start: start, End: start.AddDate(0, 0, 30)}
	if got, want := historyWindowStart(p, start.Format(time.RFC3339)), start.Format(time.RFC3339); got != want {
		t.Fatalf("from = %q, want %q", got, want)
	}
}

func TestFacetSectionFollowsTheRequestedLevel(t *testing.T) {
	withFacetHost(t, &fakeFacetHost{
		providers: []plugin.ReportFacetProvider{kvmProvider(plugin.ReportFacetHealth)},
		results: map[string]plugin.ReportFacetResult{plugin.ReportFacetHealth + "/kvm": {
			Health: []plugin.ReportHealthRow{{FacetIdentity: plugin.ReportFacetIdentity{Kind: "vm", Name: "vm-sentinel"}, Status: "up"}},
		}},
	})
	for _, tc := range []struct {
		name   string
		levels map[Level]bool
		want   Level
	}{
		{"operational only", map[Level]bool{Operational: true}, Operational},
		{"tactical only", map[Level]bool{Tactical: true}, Tactical},
		{"both", map[Level]bool{Tactical: true, Operational: true}, Tactical},
	} {
		snap := dockerSnapshot()
		b := govBuildCtx(t, TypeEnvironmentHealth, snap)
		b.levels = tc.levels
		rt, _ := reportTypeByKey(TypeEnvironmentHealth)
		collectFacets(context.Background(), snap, reportFacets(rt), plugin.ReportFacetRequest{}, false)
		found := false
		for _, s := range buildDocument(b).Sections {
			if s.Title == "Across platforms" {
				found = true
				if s.Level != tc.want {
					t.Errorf("%s: level %v, want %v", tc.name, s.Level, tc.want)
				}
			}
		}
		if !found {
			t.Errorf("%s: no Across platforms section", tc.name)
		}
	}
}

func writeSourceBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := fmt.Fprint(w, body); err != nil {
		t.Error(err)
	}
}
