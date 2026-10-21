// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestReleaseNewerComparesTheBuildWhenTheVersionMatches(t *testing.T) {
	cases := []struct {
		name                                       string
		current, currentBuild, latest, latestBuild string
		want                                       bool
	}{
		{"newer version", "2026.09.01", "202609181858", "2026.09.02", "202609010000", true},
		{"older version", "2026.09.02", "202609181858", "2026.09.01", "202612312359", false},
		{"same version, newer build", "2026.09.01", "202609181858", "2026.09.01", "202610220131", true},
		{"same version, same build", "2026.09.01", "202609181858", "2026.09.01", "202609181858", false},
		{"same version, older build", "2026.09.01", "202610220131", "2026.09.01", "202609181858", false},
		{"v prefix on the tag", "2026.09.01", "202609181858", "v2026.09.01", "202610220131", true},
		{"release carries no build", "2026.09.01", "202609181858", "2026.09.01", "", false},
		{"unstamped local build", "2026.09.01", "dev", "2026.09.01", "202610220131", false},
		{"malformed release build", "2026.09.01", "202609181858", "2026.09.01", "99999999999999", false},
		{"no release at all", "2026.09.01", "202609181858", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseNewer(tc.current, tc.currentBuild, tc.latest, tc.latestBuild); got != tc.want {
				t.Fatalf("releaseNewer(%q, %q, %q, %q) = %v, want %v",
					tc.current, tc.currentBuild, tc.latest, tc.latestBuild, got, tc.want)
			}
		})
	}
}

func TestReleaseAssetsAreFetchedOnlyFromGitHub(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/therealmcsparrow/mcharbor/releases/download/v2026.09.01/BUILD": true,
		"https://release-assets.githubusercontent.com/github-production-release-asset/1":   true,
		"http://github.com/therealmcsparrow/mcharbor/releases/download/v2026.09.01/BUILD":  false,
		"https://example.com/BUILD":                   false,
		"https://githubusercontent.com.example.com/x": false,
		"https://github.com.example.com/BUILD":        false,
		"https://169.254.169.254/latest/meta-data":    false,
		"https://github.com:8443/BUILD":               false,
	}
	for raw, want := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := releaseAssetAllowed(u); got != want {
			t.Errorf("releaseAssetAllowed(%s) = %v, want %v", raw, got, want)
		}
	}
}

// releaseServer serves a GitHub-shaped latest release for version, with a BUILD
// asset holding build when build is non-empty.
func releaseServer(t *testing.T, version string, build *atomic.Value) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/BUILD" {
			if _, err := io.WriteString(w, build.Load().(string)+"\n"); err != nil {
				t.Error(err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body := fmt.Sprintf(`{"tag_name":"v%s","html_url":"https://example.invalid/r","body":"notes","assets":[{"name":"BUILD","browser_download_url":"%s/BUILD"}]}`,
			version, srv.URL)
		if _, err := io.WriteString(w, body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestASameVersionRebuildIsOfferedAndInstalledOnce(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true, AutoInstall: true})
	var build atomic.Value
	build.Store("202610220131")
	srv := releaseServer(t, "2026.09.01", &build)
	f.c.releaseURL = srv.URL
	f.c.assetAllowed = func(*url.URL) bool { return true }
	f.c.running = func() (string, string) { return "2026.09.01", "202609181858" }
	ctx := context.Background()

	state, err := f.c.CheckNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.UpdateAvailable || state.LatestBuild != "202610220131" || state.CurrentBuild != "202609181858" {
		t.Fatalf("a newer build of the running version must be offered: %+v", state)
	}
	settings, err := f.c.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !f.c.AutoInstall(ctx, settings, f.c.State()) || f.c.AutoInstall(ctx, settings, f.c.State()) {
		t.Fatalf("the rebuild must be installed exactly once: %d installs", f.installs.Load())
	}

	build.Store("202610230900")
	if _, err := f.c.CheckNow(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.c.AutoInstall(ctx, settings, f.c.State()) || f.installs.Load() != 2 {
		t.Fatalf("a second rebuild of the same version must be installed too: %d installs", f.installs.Load())
	}
}

func TestAFailedBuildDownloadKeepsTheKnownBuild(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true, AutoInstall: true})
	var build atomic.Value
	build.Store("202610220131")
	f.c.releaseURL = releaseServer(t, "2026.09.01", &build).URL
	f.c.assetAllowed = func(*url.URL) bool { return true }
	f.c.running = func() (string, string) { return "2026.09.01", "202609181858" }
	ctx := context.Background()

	if _, err := f.c.CheckNow(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := f.c.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !f.c.AutoInstall(ctx, settings, f.c.State()) {
		t.Fatal("the rebuild must be installed")
	}

	build.Store("not a stamp")
	state, err := f.c.CheckNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.LatestBuild != "202610220131" || state.NotifiedVersion != "2026.09.01+202610220131" {
		t.Fatalf("an unreadable BUILD must keep the known build and announcement: %+v", state)
	}
	if f.c.AutoInstall(ctx, settings, f.c.State()) || f.installs.Load() != 1 {
		t.Fatalf("an unreadable BUILD must not reschedule the install: %d installs", f.installs.Load())
	}
}

func TestTheRunningBuildIsNotOfferedAgain(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true})
	var build atomic.Value
	build.Store("202609181858")
	f.c.releaseURL = releaseServer(t, "2026.09.01", &build).URL
	f.c.assetAllowed = func(*url.URL) bool { return true }
	f.c.running = func() (string, string) { return "2026.09.01", "202609181858" }

	state, err := f.c.CheckNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.UpdateAvailable {
		t.Fatalf("the running build must not be offered as an update: %+v", state)
	}
}

func TestAnAssetOffGitHubIsIgnored(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true})
	var build atomic.Value
	build.Store("202610220131")
	f.c.releaseURL = releaseServer(t, "2026.09.01", &build).URL
	f.c.running = func() (string, string) { return "2026.09.01", "202609181858" }

	state, err := f.c.CheckNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.UpdateAvailable || state.LatestBuild != "" {
		t.Fatalf("a BUILD asset hosted off GitHub must not be fetched: %+v", state)
	}
}
