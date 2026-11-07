// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/nativeupdate"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

type updateFixture struct {
	*Service
	nativeCalls []string
	imageCalls  []string
}

// newUpdateFixture builds a Service on a migrated database whose two install paths only record
// what they were asked for, so a refusal can be shown to change nothing.
func newUpdateFixture(t *testing.T, current string, native bool, patch updatechannel.Patch) *updateFixture {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	if _, _, err := updatechannel.Apply(context.Background(), database, patch); err != nil {
		t.Fatal(err)
	}
	f := &updateFixture{Service: NewService(database, nil)}
	f.current = func() string { return current }
	if native {
		f.applyNative = func(_ context.Context, version string) (nativeupdate.Result, error) {
			f.nativeCalls = append(f.nativeCalls, version)
			return nativeupdate.Result{Version: version}, nil
		}
	}
	f.applyImage = func(_ context.Context, image string) (SelfUpdateResult, error) {
		f.imageCalls = append(f.imageCalls, image)
		return SelfUpdateResult{TargetImage: image}, nil
	}
	return f
}

func on() *bool { v := true; return &v }

// TestLowerVersionRefused: a prerelease host never installs an older, floating or unparsable
// target, image or native, and nothing is started when it refuses.
func TestLowerVersionRefused(t *testing.T) {
	const cur = "2026.11.02-beta.2"
	cases := []struct {
		name, image, version string
		native               bool
	}{
		{"native lower prerelease", "", "2026.11.02-beta.1", true},
		{"native lower stable", "", "2026.11.01", true},
		{"native empty means latest", "", "", true},
		{"native unparsable", "", "latest", true},
		{"native traversal", "", "../2026.11.03", true},
		{"native encoded traversal", "", "%2e%2e", true},
		{"image lower prerelease", "ghcr.io/x/mcharbor:2026.11.02-beta.1", "", false},
		{"image lower v-tag", "ghcr.io/x/mcharbor:v2026.11.01", "", false},
		{"image floating latest", "ghcr.io/x/mcharbor:latest", "", false},
		{"image untagged", "ghcr.io/x/mcharbor", "", false},
		{"image empty reuses the current tag", "", "", false},
		{"image digest", "ghcr.io/x/mcharbor@sha256:abc", "", false},
		{"image ignores a higher version field", "ghcr.io/x/mcharbor:2026.11.01", "2026.11.09", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newUpdateFixture(t, cur, tc.native, updatechannel.Patch{McHarborPrereleases: on()})
			_, err := f.SelfUpdate(context.Background(), tc.image, tc.version)
			if !errors.Is(err, ErrDowngradeRefused) {
				t.Fatalf("SelfUpdate = %v, want ErrDowngradeRefused", err)
			}
			if len(f.nativeCalls) != 0 || len(f.imageCalls) != 0 {
				t.Fatalf("a refused update started native=%v image=%v", f.nativeCalls, f.imageCalls)
			}
		})
	}
}

// TestHigherAndEqualVersionsInstallOnAPrereleaseHost: the refusal is only for a move down.
func TestHigherAndEqualVersionsInstallOnAPrereleaseHost(t *testing.T) {
	const cur = "2026.11.02-beta.2"
	for _, v := range []string{"2026.11.02-beta.2", "2026.11.02-beta.10", "2026.11.02", "2026.12.01"} {
		f := newUpdateFixture(t, cur, true, updatechannel.Patch{McHarborPrereleases: on()})
		if _, err := f.SelfUpdate(context.Background(), "", v); err != nil {
			t.Fatalf("native %s: %v", v, err)
		}
		if len(f.nativeCalls) != 1 || f.nativeCalls[0] != v {
			t.Fatalf("native %s: asked for %v", v, f.nativeCalls)
		}
		g := newUpdateFixture(t, cur, false, updatechannel.Patch{McHarborPrereleases: on()})
		img := "ghcr.io/x/mcharbor:" + v
		if _, err := g.SelfUpdate(context.Background(), img, ""); err != nil {
			t.Fatalf("image %s: %v", v, err)
		}
		if len(g.imageCalls) != 1 || g.imageCalls[0] != img {
			t.Fatalf("image %s: asked for %v", v, g.imageCalls)
		}
	}
}

// TestPrereleaseInstallsByHandOnlyWhenTheSwitchIsOn: AC5, on a stable host and on a prerelease one.
func TestPrereleaseInstallsByHandOnlyWhenTheSwitchIsOn(t *testing.T) {
	for _, cur := range []string{"2026.11.01", "2026.11.02-beta.1"} {
		off := newUpdateFixture(t, cur, true, updatechannel.Patch{})
		if _, err := off.SelfUpdate(context.Background(), "", "2026.11.02-beta.3"); !errors.Is(err, ErrPrereleaseOff) {
			t.Fatalf("%s switch off: %v, want ErrPrereleaseOff", cur, err)
		}
		if len(off.nativeCalls) != 0 {
			t.Fatalf("%s: a refused prerelease started %v", cur, off.nativeCalls)
		}
		onF := newUpdateFixture(t, cur, true, updatechannel.Patch{McHarborPrereleases: on()})
		if _, err := onF.SelfUpdate(context.Background(), "", "2026.11.02-beta.3"); err != nil {
			t.Fatalf("%s switch on: %v", cur, err)
		}
	}
}

// TestStableHostKeepsItsBehaviour: no new refusal for a stable host moving to a stable target.
func TestStableHostKeepsItsBehaviour(t *testing.T) {
	f := newUpdateFixture(t, "2026.11.01", true, updatechannel.Patch{})
	if _, err := f.SelfUpdate(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	if len(f.nativeCalls) != 1 || f.nativeCalls[0] != "" {
		t.Fatalf("a stable host with no version must read latest, got %v", f.nativeCalls)
	}
	g := newUpdateFixture(t, "2026.11.01", false, updatechannel.Patch{})
	if _, err := g.SelfUpdate(context.Background(), "", ""); err != nil {
		t.Fatal(err)
	}
	if len(g.imageCalls) != 1 {
		t.Fatalf("image reuse must still run, got %v", g.imageCalls)
	}
}
