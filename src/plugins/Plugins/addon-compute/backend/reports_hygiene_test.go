// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"testing"
	"time"
)

var hygieneNow = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func cacheRow(imageID, envID, state string, age time.Duration) imageCacheRecord {
	return imageCacheRecord{
		ImageCache: ImageCache{ID: imageID + envID, ImageID: imageID, EnvironmentID: envID, State: state, SizeBytes: 100},
		UpdatedAt:  hygieneNow.Add(-age),
	}
}

func leaseRow(id, envID string, found bool, age time.Duration, expires *time.Time) leaseRecord {
	return leaseRecord{
		Lease:         Lease{ID: id, NetworkID: "net-" + envID, IP: "10.0.0." + id, UpdatedAt: hygieneNow.Add(-age), ExpiresAt: expires, GuestID: "g" + id},
		EnvironmentID: envID,
		NetworkName:   "lan",
		NetworkFound:  found,
	}
}

func TestClassifyCache(t *testing.T) {
	cases := []struct {
		state string
		age   time.Duration
		want  string
	}{
		{CacheStateReady, 1000 * time.Hour, cacheClassReady},
		{CacheStateFailed, time.Minute, cacheClassFailed},
		{CacheStateDownloading, time.Minute, cacheClassInProgress},
		{CacheStatePending, cacheStuckAfter - time.Minute, cacheClassInProgress},
		{CacheStatePending, cacheStuckAfter + time.Minute, cacheClassStuck},
		{CacheStateDownloading, cacheStuckAfter + time.Minute, cacheClassStuck},
		{"weird", time.Minute, cacheClassStuck},
	}
	for _, tc := range cases {
		if got := classifyCache(cacheRow("i", "e", tc.state, tc.age), hygieneNow); got != tc.want {
			t.Errorf("classifyCache(%s, %s) = %q, want %q", tc.state, tc.age, got, tc.want)
		}
	}
	if got := classifyCache(imageCacheRecord{ImageCache: ImageCache{State: CacheStateDownloading}}, hygieneNow); got != cacheClassStuck {
		t.Errorf("a downloading row with no update time = %q, want stuck", got)
	}
}

func TestClassifyLease(t *testing.T) {
	enabled := map[string]bool{"e1": true}
	past := hygieneNow.Add(-time.Hour)
	future := hygieneNow.Add(time.Hour)
	cases := []struct {
		name string
		row  leaseRecord
		want string
	}{
		{"current", leaseRow("1", "e1", true, time.Hour, &future), leaseClassCurrent},
		{"stale", leaseRow("2", "e1", true, leaseStaleAfter+time.Hour, nil), leaseClassStale},
		{"just fresh", leaseRow("3", "e1", true, leaseStaleAfter-time.Hour, nil), leaseClassCurrent},
		{"expired", leaseRow("4", "e1", true, time.Hour, &past), leaseClassExpired},
		{"network gone", leaseRow("5", "e1", false, time.Hour, &past), leaseClassOrphaned},
		{"host no longer compute", leaseRow("6", "e2", true, time.Hour, nil), leaseClassOrphaned},
	}
	for _, tc := range cases {
		if got := classifyLease(tc.row, hygieneNow, enabled); got != tc.want {
			t.Errorf("%s: classifyLease = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func hygieneFixture(scoped bool) hygieneInput {
	return hygieneInput{
		Now:    hygieneNow,
		Scoped: scoped,
		Environments: []ComputeEnvironment{
			{EnvironmentID: "e1", Name: "alpha", ComputeEnabled: true},
			{EnvironmentID: "e2", Name: "beta", ComputeEnabled: true},
			{EnvironmentID: "e3", Name: "gamma", ComputeEnabled: false},
		},
		Images: []Image{
			{ID: "ubuntu", DisplayName: "Ubuntu"},
			{ID: "debian", DisplayName: "Debian"},
			{ID: "alpine", DisplayName: "Alpine"},
		},
		Caches: []imageCacheRecord{
			cacheRow("ubuntu", "e1", CacheStateReady, time.Hour),
			cacheRow("debian", "e1", CacheStateFailed, time.Hour),
			cacheRow("ubuntu", "e2", CacheStateReady, time.Hour),
			cacheRow("debian", "e2", CacheStateDownloading, 2*cacheStuckAfter),
			cacheRow("ubuntu", "e3", CacheStateReady, time.Hour),
			cacheRow("ubuntu", "gone", CacheStateReady, time.Hour),
		},
		Leases: []leaseRecord{
			leaseRow("1", "e1", true, time.Hour, nil),
			leaseRow("2", "e1", true, 2*leaseStaleAfter, nil),
			leaseRow("3", "e2", false, time.Hour, nil),
			leaseRow("4", "e3", true, time.Hour, nil),
		},
		Pools: []Pool{
			{EnvironmentID: "e1", State: StateReady},
			{EnvironmentID: "e2", State: StateError},
		},
		Locations: LocationsView{
			Global:    Locations{ScopeID: globalLocationScope, ImagePath: "/srv/images"},
			Overrides: []Locations{{ScopeID: "e2", ImagePath: "/data/img"}},
		},
	}
}

func TestAnalyseHygieneUnscoped(t *testing.T) {
	a := analyseHygiene(hygieneFixture(false))

	if len(a.Hosts) != 2 {
		t.Fatalf("hosts = %d, want only the two compute-enabled hosts", len(a.Hosts))
	}
	if a.ReadyCopies != 2 || a.Failed != 1 || a.Stuck != 1 || a.InProgress != 0 {
		t.Fatalf("cache tallies: ready=%d failed=%d stuck=%d inprogress=%d", a.ReadyCopies, a.Failed, a.Stuck, a.InProgress)
	}
	if a.OrphanCaches != 2 {
		t.Fatalf("OrphanCaches = %d, want the rows for the disabled and the deleted host", a.OrphanCaches)
	}
	if a.Unused != 2 {
		t.Fatalf("Unused = %d, want debian (failed/stuck only) and alpine (nowhere)", a.Unused)
	}
	if a.IdleHosts != 1 {
		t.Fatalf("IdleHosts = %d, want beta (ready image, no ready pool)", a.IdleHosts)
	}
	if a.LeasesTotal != 4 || a.Current != 1 || a.Stale != 1 || a.Orphaned != 2 || a.Expired != 0 {
		t.Fatalf("leases: total=%d current=%d stale=%d orphaned=%d expired=%d",
			a.LeasesTotal, a.Current, a.Stale, a.Orphaned, a.Expired)
	}
	if len(a.CacheIssues) != 2 || a.CacheIssues[0].Class != cacheClassFailed {
		t.Fatalf("cache issues = %+v, want failed first", a.CacheIssues)
	}
	if a.LeaseIssues[0].Class != leaseClassOrphaned || a.LeaseIssues[len(a.LeaseIssues)-1].Class != leaseClassStale {
		t.Fatalf("lease issues not ordered orphaned→stale: %+v", a.LeaseIssues)
	}

	alpha, beta := a.Hosts[0], a.Hosts[1]
	if alpha.ImagePath != "/srv/images" || alpha.ImageSource != "global" {
		t.Errorf("alpha location = %q (%s), want the global path", alpha.ImagePath, alpha.ImageSource)
	}
	if beta.ImagePath != "/data/img" || beta.ImageSource != "environment" {
		t.Errorf("beta location = %q (%s), want its override", beta.ImagePath, beta.ImageSource)
	}
	if alpha.BadLeases != 1 || alpha.Leases != 2 || alpha.CachedBytes != 100 {
		t.Errorf("alpha = %+v", alpha)
	}
}

func TestAnalyseHygieneScopedIgnoresOtherHosts(t *testing.T) {
	in := hygieneFixture(true)
	in.Environments = in.Environments[:1]
	a := analyseHygiene(in)

	if a.OrphanCaches != 0 {
		t.Fatalf("a scoped report called %d out-of-scope rows orphaned", a.OrphanCaches)
	}
	if a.LeasesTotal != 2 || a.Orphaned != 0 {
		t.Fatalf("scoped leases: total=%d orphaned=%d", a.LeasesTotal, a.Orphaned)
	}
	if a.Stuck != 0 || a.Failed != 1 {
		t.Fatalf("scoped cache: stuck=%d failed=%d", a.Stuck, a.Failed)
	}
}

func TestHygieneFindingSeverities(t *testing.T) {
	findings := hygieneFindings(analyseHygiene(hygieneFixture(false)))
	want := []string{"high", "medium", "medium", "medium", "low", "low", "info"}
	if len(findings) != len(want) {
		t.Fatalf("got %d findings, want %d: %+v", len(findings), len(want), findings)
	}
	for i, f := range findings {
		if f.Severity != want[i] {
			t.Errorf("finding %d %q severity = %q, want %q", i, f.Title, f.Severity, want[i])
		}
		if f.Recommendation == "" {
			t.Errorf("finding %q has no recommendation", f.Title)
		}
	}
}

func TestHygieneFindingsQuietWhenClean(t *testing.T) {
	in := hygieneInput{
		Now:          hygieneNow,
		Environments: []ComputeEnvironment{{EnvironmentID: "e1", Name: "alpha", ComputeEnabled: true}},
		Images:       []Image{{ID: "ubuntu"}},
		Caches:       []imageCacheRecord{cacheRow("ubuntu", "e1", CacheStateReady, time.Hour)},
		Leases:       []leaseRecord{leaseRow("1", "e1", true, time.Hour, nil)},
		Pools:        []Pool{{EnvironmentID: "e1", State: StateReady}},
	}
	a := analyseHygiene(in)
	if findings := hygieneFindings(a); len(findings) != 0 {
		t.Fatalf("a clean estate produced findings: %+v", findings)
	}
	if a.Hosts[0].ImageSource != "agent" || a.Hosts[0].ImagePath != "" {
		t.Fatalf("no stored location should fall back to the agent default, got %+v", a.Hosts[0])
	}
}
