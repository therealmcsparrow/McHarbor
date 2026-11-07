// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updatechannel

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

func on(v bool) *bool { return &v }

func TestDefaultsAreAllOff(t *testing.T) {
	s, err := Load(context.Background(), newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if s != (Settings{}) {
		t.Fatalf("fresh install must read all off, got %+v", s)
	}
	for _, p := range []Path{PathMcHarbor, PathAgents, PathPlugins} {
		if s.Allows(p, "2026.11.02-rc1") {
			t.Errorf("%s offered a prerelease by default", p)
		}
	}
}

// TestChannelMatrix walks every combination of switch, auto-install and option
// for the McHarbor and agent paths. A prerelease installs automatically only
// with all three on; a stable release follows auto-install alone; the other
// path's option never matters.
func TestChannelMatrix(t *testing.T) {
	for _, path := range []Path{PathMcHarbor, PathAgents} {
		for bits := 0; bits < 8; bits++ {
			sw, auto, opt := bits&1 != 0, bits&2 != 0, bits&4 != 0
			for _, otherOn := range []bool{false, true} {
				var s Settings
				if path == PathMcHarbor {
					s = Settings{McHarborPrereleases: sw, McHarborAutoPrereleases: opt, AgentPrereleases: otherOn, AgentAutoPrereleases: otherOn}
				} else {
					s = Settings{AgentPrereleases: sw, AgentAutoPrereleases: opt, McHarborPrereleases: otherOn, McHarborAutoPrereleases: otherOn}
				}
				if got, want := s.AutoInstalls(path, auto, "2026.11.02-rc1"), sw && auto && opt; got != want {
					t.Errorf("%s sw=%v auto=%v opt=%v other=%v: prerelease auto=%v want %v", path, sw, auto, opt, otherOn, got, want)
				}
				if got := s.AutoInstalls(path, auto, "v2026.11.02"); got != auto {
					t.Errorf("%s sw=%v auto=%v opt=%v: stable auto=%v want %v", path, sw, auto, opt, got, auto)
				}
				if got := s.Allows(path, "2026.11.02-rc1"); got != sw {
					t.Errorf("%s sw=%v: Allows prerelease=%v", path, sw, got)
				}
				if !s.Allows(path, "2026.11.02") {
					t.Errorf("%s: stable must always be allowed", path)
				}
			}
		}
	}
	for _, sw := range []bool{false, true} {
		if got := (Settings{PluginPrereleases: sw}).Allows(PathPlugins, "1.0.0-rc.1"); got != sw {
			t.Errorf("plugins sw=%v: Allows=%v", sw, got)
		}
	}
	if (Settings{PluginPrereleases: true}).AutoInstalls(PathPlugins, true, "1.0.0-rc.1") {
		t.Error("plugins have no auto-install option")
	}
}

func TestApplyOnlyChangesSentFields(t *testing.T) {
	ctx := context.Background()
	database := newDB(t)
	if _, _, err := Apply(ctx, database, Patch{McHarborPrereleases: on(true), AgentPrereleases: on(true)}); err != nil {
		t.Fatal(err)
	}
	before, after, err := Apply(ctx, database, Patch{PluginPrereleases: on(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !before.McHarborPrereleases || !after.McHarborPrereleases || !after.AgentPrereleases || !after.PluginPrereleases || before.PluginPrereleases {
		t.Fatalf("absent fields must stay as stored: before=%+v after=%+v", before, after)
	}
	ch := Diff(before, after)
	if len(ch) != 1 || ch[0].Key != "pluginPrereleases" || ch[0].Old || !ch[0].New {
		t.Fatalf("diff lists only what changed: %+v", ch)
	}
}

// TestStaleClientKeepsNewerSwitches saves one switch from a client that read
// the state before other switches changed and asserts those switches survive.
func TestStaleClientKeepsNewerSwitches(t *testing.T) {
	ctx := context.Background()
	database := newDB(t)
	if _, _, err := Apply(ctx, database, Patch{AgentPrereleases: on(true), McHarborAutoPrereleases: on(true)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Apply(ctx, database, Patch{McHarborPrereleases: on(true)}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{McHarborPrereleases: true, AgentPrereleases: true, McHarborAutoPrereleases: true}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestPrereleaseImage(t *testing.T) {
	cases := []struct {
		image, version, want string
		wantErr              bool
	}{
		{"ghcr.io/x/mcharbor:latest", "2026.11.02-rc1", "ghcr.io/x/mcharbor:2026.11.02-rc1", false},
		{"ghcr.io/x/mcharbor", "v2026.11.02-rc1", "ghcr.io/x/mcharbor:2026.11.02-rc1", false},
		{"ghcr.io/x/mcharbor:2026.11.01", "2026.11.02-rc1", "ghcr.io/x/mcharbor:2026.11.02-rc1", false},
		{"ghcr.io/x/mcharbor:v2026.11.01", "2026.11.02-rc1", "ghcr.io/x/mcharbor:v2026.11.02-rc1", false},
		{"localhost:5000/mcharbor:latest", "2026.11.02-rc1", "localhost:5000/mcharbor:2026.11.02-rc1", false},
		{"localhost:5000/mcharbor", "2026.11.02-rc1", "localhost:5000/mcharbor:2026.11.02-rc1", false},
		{"ghcr.io/x/mcharbor@sha256:abc", "2026.11.02-rc1", "", true},
		{"ghcr.io/x/mcharbor:latest", "latest", "", true},
		{"ghcr.io/x/mcharbor:latest", "2026.11.02", "", true},
		{"ghcr.io/x/mcharbor:latest", "../x-rc1", "", true},
		{"ghcr.io/x/mcharbor:latest", "1.0.0-rc1/../../y", "", true},
		{"ghcr.io/x/mcharbor:latest", "2026.11.02-rc.1+build.5", "", true},
		{"", "2026.11.02-rc1", "", true},
		{"ghcr.io/x/mcharbor:latest", "2026.11.02-" + strings.Repeat("a", 400), "", true},
	}
	for _, c := range cases {
		got, err := PrereleaseImage(c.image, c.version)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("PrereleaseImage(%q,%q) = %q, %v; want %q err=%v", c.image, c.version, got, err, c.want, c.wantErr)
		}
	}
}
