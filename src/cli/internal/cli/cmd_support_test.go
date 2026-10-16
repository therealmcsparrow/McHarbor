// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSupportDiagnosticsIsDataNotAnError: a failing check is a verdict, so the
// command must still exit 0. A CLI that exits non-zero here makes every
// monitoring wrapper treat "I found a problem" as "I could not look".
func TestSupportDiagnosticsIsDataNotAnError(t *testing.T) {
	report := map[string]any{
		"generatedAt": "2026-09-27T01:00:00Z", "version": "2026.09.01", "build": "202609270100",
		"checks": []any{
			map[string]any{"id": "db", "status": "pass", "reason": "reachable", "durationMs": 3},
			map[string]any{"id": "disk", "status": "fail", "reason": "low_disk", "durationMs": 1},
		},
	}
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/support/diagnostics": func(w http.ResponseWriter, _ string) { ok(w, report) },
	})
	code, out, stderr := run(t, f, goodKey, "support", "diagnostics")
	if code != ExitOK {
		t.Fatalf("a failing check must not fail the command: exit %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(out, "disk") || !strings.Contains(out, "low_disk") {
		t.Fatalf("the failing check must be visible: %q", out)
	}
	if !strings.Contains(stderr, "worst verdict fail") {
		t.Fatalf("the summary must name the worst verdict: %q", stderr)
	}
}

// TestSupportPreUpgradeExitsSevenWhenItBlocks is the reason the command exists:
// a CI job gates a rollout on the exit code, and a warning must not stop it.
func TestSupportPreUpgradeExitsSevenWhenItBlocks(t *testing.T) {
	var status string
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/support/pre-upgrade": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{
				"currentVersion": "2026.09.01", "targetVersion": "2026.09.02", "databaseDriver": "sqlite",
				"checks": []any{map[string]any{"id": "plugins", "status": status, "reason": "incompatible", "durationMs": 2}},
			})
		},
	})

	status = "fail"
	code, _, stderr := run(t, f, goodKey, "support", "pre-upgrade", "--target", "2026.09.02")
	if code != ExitRemote {
		t.Fatalf("a blocking report must exit %d, got %d (%q)", ExitRemote, code, stderr)
	}
	if !strings.Contains(stderr, "2026.09.02") {
		t.Fatalf("the refusal must name the target version: %q", stderr)
	}

	status = "warn"
	if code, _, stderr := run(t, f, goodKey, "support", "pre-upgrade"); code != ExitOK {
		t.Fatalf("a warning must not block the upgrade: exit %d, %q", code, stderr)
	}

	status = "pass"
	if code, _, _ := run(t, f, goodKey, "support", "pre-upgrade"); code != ExitOK {
		t.Fatalf("a passing report must exit 0, got %d", code)
	}
}

// TestSupportBundleDownloadWritesAFileAndNeverATerminal covers both halves of
// the download contract: the archive lands on disk rather than in the scrollback,
// and an existing file is not silently replaced.
func TestSupportBundleDownloadWritesAFileAndNeverATerminal(t *testing.T) {
	archive := "PK\x03\x04pretend-this-is-a-zip"
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/support/bundles/b1/download": func(w http.ResponseWriter, _ string) {
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", `attachment; filename="mcharbor-bundle-b1.zip"`)
			if _, err := w.Write([]byte(archive)); err != nil {
				t.Errorf("writing archive: %v", err)
			}
		},
	})

	dir := t.TempDir()
	dest := filepath.Join(dir, "bundle.zip")
	code, out, stderr := run(t, f, goodKey, "support", "bundle", "download", "b1", "-f", dest)
	if code != ExitOK {
		t.Fatalf("download failed: exit %d, %q %q", code, out, stderr)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != archive {
		t.Fatalf("the file must hold the archive bytes, got %q", string(body))
	}
	if strings.Contains(out, "PK") {
		t.Fatalf("the archive must never reach stdout: %q", out)
	}

	// Second run, same destination: refused rather than overwritten.
	code, _, stderr = run(t, f, goodKey, "support", "bundle", "download", "b1", "-f", dest)
	if code != ExitUsage || !strings.Contains(stderr, "--force") {
		t.Fatalf("an existing file must be refused and name --force: exit %d, %q", code, stderr)
	}
	if code, _, stderr := run(t, f, goodKey, "support", "bundle", "download", "b1", "-f", dest, "--force"); code != ExitOK {
		t.Fatalf("--force must overwrite: exit %d, %q", code, stderr)
	}

	// With no -o, the server's Content-Disposition name is used, base only.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { warnErr(os.Chdir(cwd)) })
	if code, _, stderr := run(t, f, goodKey, "support", "bundle", "download", "b1"); code != ExitOK {
		t.Fatalf("default filename: exit %d, %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "mcharbor-bundle-b1.zip")); err != nil {
		t.Fatalf("the server's filename must be used: %v", err)
	}

	// A server that tries to choose a path gets only the base name.
	if got := filenameFromResponse(&http.Response{Header: http.Header{
		"Content-Disposition": []string{`attachment; filename="../../etc/passwd"`}}}, "fallback.zip"); got != "passwd" {
		t.Fatalf("a path in the filename must be reduced to its base, got %q", got)
	}
}

// TestSupportBundleDeleteSendsNothingUnconfirmed: delete is irreversible, so
// --yes is not enough and a refused confirmation must reach no request at all.
func TestSupportBundleDeleteSendsNothingUnconfirmed(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"DELETE /api/support/bundles/b1": func(w http.ResponseWriter, _ string) { ok(w, nil) },
	})

	code, _, stderr := run(t, f, goodKey, "--yes", "support", "bundle", "delete", "b1")
	if code != ExitUsage || !strings.Contains(stderr, "--confirm b1") {
		t.Fatalf("--yes alone must be refused and name --confirm: exit %d, %q", code, stderr)
	}
	for _, r := range f.seen() {
		if r.Method == http.MethodDelete {
			t.Fatalf("a refused confirmation sent a request: %s %s", r.Method, r.URL.Path)
		}
	}

	if code, _, stderr := run(t, f, goodKey, "--confirm", "b1", "support", "bundle", "delete", "b1"); code != ExitOK {
		t.Fatalf("--confirm must allow the delete: exit %d, %q", code, stderr)
	}
	sawDelete := false
	for _, r := range f.seen() {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/support/bundles/b1" {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Fatal("the confirmed delete sent no request")
	}
}

// TestSupportBundleCreateFollowsToATerminalState checks the one follow pattern:
// progress on stderr, the final object on stdout, and a failed bundle exits 7.
func TestSupportBundleCreateFollowsToATerminalState(t *testing.T) {
	polls := 0
	status := "ready"
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/support/bundles": func(w http.ResponseWriter, body string) {
			if !strings.Contains(body, "logs") {
				t.Errorf("the request body must carry the categories, got %q", body)
			}
			ok(w, map[string]any{"id": "b9", "status": "running", "progress": 0, "step": "collecting"})
		},
		"GET /api/support/bundles/b9": func(w http.ResponseWriter, _ string) {
			polls++
			if polls < 2 {
				ok(w, map[string]any{"id": "b9", "status": "running", "progress": 40, "step": "redacting"})
				return
			}
			ok(w, map[string]any{"id": "b9", "status": status, "progress": 100, "sizeBytes": 4096, "error": "out of disk"})
		},
	})

	old := pollInterval
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = old })

	code, out, stderr := run(t, f, goodKey, "support", "bundle", "create", "--categories", "logs,config")
	if code != ExitOK {
		t.Fatalf("a ready bundle must exit 0: %d, %q %q", code, out, stderr)
	}
	if !strings.Contains(stderr, "redacting") {
		t.Fatalf("progress belongs on stderr: %q", stderr)
	}
	if !strings.Contains(out, "b9") {
		t.Fatalf("the final bundle belongs on stdout: %q", out)
	}

	polls, status = 0, "failed"
	code, _, stderr = run(t, f, goodKey, "support", "bundle", "create", "--categories", "logs")
	if code != ExitRemote || !strings.Contains(stderr, "out of disk") {
		t.Fatalf("a failed bundle must exit %d and carry the server's message: %d, %q", ExitRemote, code, stderr)
	}

	// --no-follow returns the id without polling.
	polls, status = 0, "ready"
	if code, out, _ := run(t, f, goodKey, "support", "bundle", "create", "--categories", "logs", "--no-follow"); code != ExitOK || !strings.Contains(out, "b9") {
		t.Fatalf("--no-follow: exit %d, %q", code, out)
	}
	if polls != 0 {
		t.Fatalf("--no-follow must not poll, polled %d times", polls)
	}
}
