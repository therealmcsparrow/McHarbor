// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

func rel(tag string) githubRelease { return githubRelease{TagName: tag} }

func TestSelectReleasePicksTheNewestAllowedTag(t *testing.T) {
	on := updatechannel.Settings{McHarborPrereleases: true}
	off := updatechannel.Settings{}
	list := []githubRelease{
		rel("v2026.11.01"), rel("v2026.11.02-rc1"), rel("v2026.11.02-rc2"),
		{TagName: "v2026.12.01-draft", Draft: true},
		rel("v../../etc"), rel("not-a-version"), rel(""),
		{TagName: "v2026.11.09", Prerelease: true},
	}
	got, ok := selectRelease(list, on)
	if !ok || got.TagName != "v2026.11.02-rc2" {
		t.Fatalf("switch on must pick the newest prerelease, got %q ok=%v", got.TagName, ok)
	}
	got, ok = selectRelease(list, off)
	if !ok || got.TagName != "v2026.11.01" {
		t.Fatalf("switch off must pick the newest stable, got %q ok=%v", got.TagName, ok)
	}
	if _, ok := selectRelease([]githubRelease{rel("v1.0.0-x"), rel("junk")}, off); ok {
		t.Fatal("only prereleases and junk must select nothing with the switch off")
	}
}

// channelServer serves the latest-stable object on /latest and a release list on /list.
func channelServer(t *testing.T, latestTag string, listTags ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body string
		if r.URL.Path == "/list" {
			items := make([]string, 0, len(listTags))
			for _, tag := range listTags {
				items = append(items, fmt.Sprintf(`{"tag_name":%q,"html_url":"https://example.invalid/r","body":"n"}`, tag))
			}
			body = "[" + strings.Join(items, ",") + "]"
		} else {
			body = fmt.Sprintf(`{"tag_name":%q,"html_url":"https://example.invalid/r","body":"n"}`, latestTag)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestReleaseSelection(t *testing.T) {
	cases := []struct {
		name       string
		running    string
		switchOn   bool
		latestTag  string
		list       []string
		wantLatest string
		wantOffer  bool
		wantPre    bool
	}{
		{"on offers a newer prerelease, labelled", "2026.11.01", true, "v2026.11.01", []string{"v2026.11.02-rc1", "v2026.11.01"}, "2026.11.02-rc1", true, true},
		{"off never offers that prerelease", "2026.11.01", false, "v2026.11.01", []string{"v2026.11.02-rc1", "v2026.11.01"}, "2026.11.01", false, false},
		{"off on a prerelease offers the next newer stable", "2026.11.02-rc1", false, "v2026.11.02", nil, "2026.11.02", true, false},
		{"off on a prerelease offers nothing lower", "2026.11.02-rc1", false, "v2026.11.01", nil, "2026.11.01", false, false},
		{"on, running prerelease, same prerelease is not an update", "2026.11.02-rc1", true, "v2026.11.01", []string{"v2026.11.02-rc1"}, "2026.11.02-rc1", false, true},
		{"on, a newer rc replaces an older rc", "2026.11.02-rc1", true, "v2026.11.01", []string{"v2026.11.02-rc2", "v2026.11.02-rc1"}, "2026.11.02-rc2", true, true},
		{"on, the stable beats its own rc", "2026.11.02-rc2", true, "v2026.11.02", []string{"v2026.11.02", "v2026.11.02-rc2"}, "2026.11.02", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSchedFixture(t, SelfUpdateSettings{Enabled: true})
			srv := channelServer(t, tc.latestTag, tc.list...)
			f.c.releaseURL = srv.URL + "/latest"
			f.c.releasesURL = srv.URL + "/list"
			f.c.running = func() (string, string) { return tc.running, "dev" }
			ctx := context.Background()
			if _, _, err := updatechannel.Apply(ctx, f.c.db, updatechannel.Patch{McHarborPrereleases: &tc.switchOn}); err != nil {
				t.Fatal(err)
			}
			state, err := f.c.CheckNow(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if state.LatestVersion != tc.wantLatest || state.UpdateAvailable != tc.wantOffer || state.LatestIsPrerelease != tc.wantPre {
				t.Fatalf("got latest=%q offer=%v pre=%v, want %q %v %v", state.LatestVersion, state.UpdateAvailable, state.LatestIsPrerelease, tc.wantLatest, tc.wantOffer, tc.wantPre)
			}
		})
	}
}

func TestAHostileTagIsNeverOffered(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true})
	f.c.releaseURL = channelServer(t, "v../../x").URL + "/latest"
	f.c.running = func() (string, string) { return "2026.11.01", "dev" }
	state, err := f.c.CheckNow(context.Background())
	if err == nil || state.UpdateAvailable {
		t.Fatalf("an unparsable tag must fail the check, got %+v err=%v", state, err)
	}
}

type installFixture struct {
	*schedFixture
	targets []string
}

func newInstallFixture(t *testing.T, image string, patch updatechannel.Patch) *installFixture {
	t.Helper()
	f := &installFixture{schedFixture: newSchedFixture(t, SelfUpdateSettings{Enabled: true, AutoInstall: true})}
	f.c.SetApplier(func(_ context.Context, target string) error { f.targets = append(f.targets, target); return nil },
		func(context.Context) string { return image })
	if image == "" {
		f.c.SetApplier(func(_ context.Context, target string) error { f.targets = append(f.targets, target); return nil }, nil)
	}
	f.c.running = func() (string, string) { return "2026.11.01", "dev" }
	old := imageForVersion
	imageForVersion = func(img, v string) string { return "stable-image:" + v }
	t.Cleanup(func() { imageForVersion = old })
	if _, _, err := updatechannel.Apply(context.Background(), f.c.db, patch); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *installFixture) run(t *testing.T, latest string) (bool, SelfUpdateState) {
	t.Helper()
	ctx := context.Background()
	settings, err := f.c.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := SelfUpdateState{CurrentVersion: "2026.11.01", LatestVersion: latest, UpdateAvailable: true}
	return f.c.AutoInstall(ctx, settings, st), f.c.State()
}

func tr() *bool { v := true; return &v }

func TestAutoInstallTargets(t *testing.T) {
	both := updatechannel.Patch{McHarborPrereleases: tr(), McHarborAutoPrereleases: tr()}

	t.Run("native prerelease targets the version", func(t *testing.T) {
		f := newInstallFixture(t, "", both)
		ok, _ := f.run(t, "2026.11.02-rc1")
		if !ok || len(f.targets) != 1 || f.targets[0] != "2026.11.02-rc1" {
			t.Fatalf("ok=%v targets=%v", ok, f.targets)
		}
	})
	t.Run("floating image moves to the pinned prerelease tag", func(t *testing.T) {
		f := newInstallFixture(t, "ghcr.io/x/mcharbor:latest", both)
		ok, _ := f.run(t, "2026.11.02-rc1")
		if !ok || len(f.targets) != 1 || f.targets[0] != "ghcr.io/x/mcharbor:2026.11.02-rc1" {
			t.Fatalf("ok=%v targets=%v", ok, f.targets)
		}
	})
	t.Run("digest-pinned image refuses and records it", func(t *testing.T) {
		f := newInstallFixture(t, "ghcr.io/x/mcharbor@sha256:abc", both)
		ok, st := f.run(t, "2026.11.02-rc1")
		if ok || len(f.targets) != 0 {
			t.Fatalf("must not install: ok=%v targets=%v", ok, f.targets)
		}
		if st.AutoInstallError == "" || strings.Contains(st.AutoInstallError, "sha256") {
			t.Fatalf("refusal must be recorded and redacted: %q", st.AutoInstallError)
		}
		if again, _ := f.run(t, "2026.11.02-rc1"); again || len(f.targets) != 0 {
			t.Fatal("a refused release must not be retried every tick")
		}
	})
	t.Run("stable keeps ImageForVersion", func(t *testing.T) {
		f := newInstallFixture(t, "ghcr.io/x/mcharbor:latest", updatechannel.Patch{})
		ok, _ := f.run(t, "2026.11.02")
		if !ok || len(f.targets) != 1 || f.targets[0] != "stable-image:2026.11.02" {
			t.Fatalf("ok=%v targets=%v", ok, f.targets)
		}
	})
}

func TestAutoInstallNeedsEveryOptionAndNeverMovesLower(t *testing.T) {
	cases := []struct {
		name   string
		patch  updatechannel.Patch
		latest string
	}{
		{"switch off", updatechannel.Patch{McHarborAutoPrereleases: tr()}, "2026.11.02-rc1"},
		{"auto option off", updatechannel.Patch{McHarborPrereleases: tr()}, "2026.11.02-rc1"},
		{"lower than running", updatechannel.Patch{}, "2026.10.05"},
		{"lower prerelease than running", updatechannel.Patch{McHarborPrereleases: tr(), McHarborAutoPrereleases: tr()}, "2026.11.01-rc1"},
		{"unparsable target", updatechannel.Patch{}, "latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInstallFixture(t, "", tc.patch)
			if ok, _ := f.run(t, tc.latest); ok || len(f.targets) != 0 {
				t.Fatalf("must not install %q: ok=%v targets=%v", tc.latest, ok, f.targets)
			}
		})
	}
}
