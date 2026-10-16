// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestVersionAgreementNamesTheDisagreeingPair covers each way the four copies of a
// plugin's version can disagree, because a status line that says only "load failed"
// is what made past version-bump incidents slow to diagnose.
func TestVersionAgreementNamesTheDisagreeingPair(t *testing.T) {
	cases := []struct {
		name                       string
		row, dir, manifest, symbol string
		want                       error
		wantText                   string
	}{
		{"all four agree", "2026.09.01", "2026.09.01", "2026.09.01", "2026.09.01", nil, ""},
		{"unknown values are skipped", "", "2026.09.01", "", "", nil, ""},
		{"row differs from directory", "2026.09.02", "2026.09.01", "2026.09.01", "2026.09.01", ErrRowVersionMismatch, `installed version "2026.09.02" does not match directory "2026.09.01"`},
		{"manifest differs from directory", "2026.09.01", "2026.09.01", "2026.09.02", "2026.09.02", ErrManifestVersionMismatch, `manifest version "2026.09.02" does not match directory "2026.09.01"`},
		{"symbol differs from manifest", "2026.09.01", "2026.09.01", "2026.09.01", "2026.09.02", ErrSymbolVersionMismatch, `manifest version "2026.09.01" does not match plugin symbol "2026.09.02"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkVersionAgreement("addon-demo", tc.row, tc.dir, tc.manifest, tc.symbol)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("expected agreement, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantText)
			}
			for _, other := range []error{ErrInstallDirMissing, ErrRowVersionMismatch, ErrManifestVersionMismatch, ErrSymbolVersionMismatch} {
				if !errors.Is(other, tc.want) && errors.Is(err, other) {
					t.Fatalf("error also matches %v; each mismatch must name exactly one pair", other)
				}
			}
		})
	}
}

// TestScanReportsInstalledVersionWithNoDirectory pins the case that used to be silent:
// a row whose version has no <slug>@<version> directory now leaves a failed status entry.
func TestScanReportsInstalledVersionWithNoDirectory(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "plugins", "addon-demo@2026.09.01"), 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "plugins.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Logf("closing db: %v", err)
		}
	})
	if _, err := db.Exec(`CREATE TABLE appstore_plugins (slug TEXT PRIMARY KEY, version TEXT NOT NULL);
		INSERT INTO appstore_plugins (slug, version) VALUES ('addon-demo', '2026.09.02');`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := &Runtime{
		dataDir: dataDir,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		host:    HostServices{DB: db},
		loaded:  make(map[string]*Loaded),
	}
	if err := r.scanAndLoad(context.Background()); err != nil {
		t.Fatalf("scanAndLoad: %v", err)
	}

	got, ok := r.Loaded()["addon-demo"]
	if !ok {
		t.Fatal("no status entry for an installed plugin whose directory is missing")
	}
	if got.Loaded {
		t.Fatal("status says loaded")
	}
	if got.Version != "2026.09.02" {
		t.Fatalf("status version = %q, want the row's 2026.09.02", got.Version)
	}
	for _, want := range []string{`installed version "2026.09.02" has no directory addon-demo@2026.09.02`, "2026.09.01"} {
		if !strings.Contains(got.Err, want) {
			t.Fatalf("status error %q does not contain %q", got.Err, want)
		}
	}
}
