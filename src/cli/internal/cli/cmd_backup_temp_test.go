// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

const (
	backupTempStatusRoute  = "GET /api/container-backups/temp"
	backupTempCleanupRoute = "POST /api/container-backups/temp/cleanup"
)

func backupTempServer(t *testing.T) *fakeServer {
	t.Helper()
	return groupServer(t, map[string]func(http.ResponseWriter, string){
		backupTempStatusRoute: func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"path": "/srv/scratch/bk", "source": "setting", "settingPath": "/srv/scratch/bk",
				"envOverride": false, "envInvalidCode": "", "defaultPath": "/data/backups/tmp",
				"sizeBytes": 5242880, "entries": 13, "activeRuns": 2, "retiredPaths": []string{},
			})
		},
		backupTempCleanupRoute: func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"removedEntries": 17, "removedBytes": 5242880, "keptOwned": 23, "keptRecent": 31, "activeRuns": 0,
			})
		},
	})
}

func requestsTo(f *fakeServer, method, path string) int {
	n := 0
	for _, r := range f.seen() {
		if r.Method == method && r.URL.Path == path {
			n++
		}
	}
	return n
}

func TestBackupTempStatusShowsTheColumns(t *testing.T) {
	f := backupTempServer(t)
	code, out, stderr := run(t, f, goodKey, "backup", "temp", "status")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"PATH", "SOURCE", "SIZE", "ENTRIES", "ACTIVE RUNS", "/srv/scratch/bk", "setting", "5.0 MiB", "13"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output lacks %q:\n%s", want, out)
		}
	}
	if n := requestsTo(f, http.MethodGet, "/api/container-backups/temp"); n != 1 {
		t.Fatalf("status made %d calls to the temp route, want 1", n)
	}
}

func TestBackupTempStatusSurfacesTheServerMessageOn403(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		backupTempStatusRoute: func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusForbidden, "err.rbac.permission_denied", "backups.manage must be granted globally")
		},
	})
	code, _, stderr := run(t, f, goodKey, "backup", "temp", "status")
	if code != ExitForbidden || !strings.Contains(stderr, "backups.manage") {
		t.Fatalf("exit %d, stderr %q; want exit %d naming backups.manage", code, stderr, ExitForbidden)
	}
}

func TestBackupTempCleanIsDestructive(t *testing.T) {
	f := backupTempServer(t)
	code, _, stderr := run(t, f, goodKey, "backup", "temp", "clean")
	if code != ExitUsage || !strings.Contains(stderr, "pass --") {
		t.Fatalf("an unconfirmed clean: exit %d, stderr %q; want %d with a confirmation refusal", code, stderr, ExitUsage)
	}
	if n := requestsTo(f, http.MethodPost, "/api/container-backups/temp/cleanup"); n != 0 {
		t.Fatalf("a refused clean still sent %d request(s)", n)
	}
}

func TestBackupTempCleanPrintsTheCounts(t *testing.T) {
	f := backupTempServer(t)
	code, out, stderr := run(t, f, goodKey, "--yes", "backup", "temp", "clean", "--confirm", "backup-temp")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"17", "5.0 MiB", "23", "31"} {
		if !strings.Contains(out, want) {
			t.Errorf("clean output lacks %q:\n%s", want, out)
		}
	}
	if n := requestsTo(f, http.MethodPost, "/api/container-backups/temp/cleanup"); n != 1 {
		t.Fatalf("clean sent %d cleanup request(s), want 1", n)
	}
}

func TestBackupTempCleanSurfacesTheServerMessageOn403(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		backupTempCleanupRoute: func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusForbidden, "err.rbac.permission_denied", "backups.manage must be granted globally")
		},
	})
	code, _, stderr := run(t, f, goodKey, "--yes", "backup", "temp", "clean", "--confirm", "backup-temp")
	if code != ExitForbidden || !strings.Contains(stderr, "backups.manage") {
		t.Fatalf("exit %d, stderr %q; want exit %d naming backups.manage", code, stderr, ExitForbidden)
	}
}
