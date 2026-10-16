// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRestoreLocalHandsTheServerItsOwnFlags(t *testing.T) {
	server := filepath.Join(t.TempDir(), "mcharbor-server")
	if err := os.WriteFile(server, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var got []string
	old := runServer
	runServer = func(_ string, args []string, _ *globals) (int, error) {
		got = args
		return 3, nil
	}
	t.Cleanup(func() { runServer = old })

	code, _, stderr := run(t, nil, "", "--yes", "restore", "--local", "x.mchx", "--server-binary", server,
		"--key-file", "k.key", "--mode", "selective", "--categories", "access,settings", "--include-audit")
	if code != 3 {
		t.Fatalf("the server's exit status must be the CLI's: %d %s", code, stderr)
	}
	want := []string{"restore-local", "--file", "x.mchx", "--mode", "selective", "--key-file", "k.key",
		"--categories", "access,settings", "--include-audit", "--yes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args\n got %q\nwant %q", got, want)
	}

	got = nil
	if code, _, stderr := run(t, nil, "", "restore", "x.mchx", "--server-binary", server); code != ExitUsage || !strings.Contains(stderr, "--local") || got != nil {
		t.Fatalf("without --local: exit %d, %q", code, stderr)
	}
	if code, _, stderr := run(t, nil, "", "restore", "--local", "x.mchx", "--server-binary", filepath.Join(t.TempDir(), "absent")); code != ExitUsage || !strings.Contains(stderr, "not at") {
		t.Fatalf("no server binary: exit %d, %q", code, stderr)
	}
}

func TestAuditVerifyNamesRestoreAnchorsAndFailsOnABreak(t *testing.T) {
	result := map[string]any{"verified": true, "checkedRows": 12, "anchors": []any{
		map[string]any{"id": "an1", "timestamp": "2026-09-26T18:00:00Z", "actor": "local:root@harbor"},
	}}
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/audit/verify": func(w http.ResponseWriter, _ string) { ok(w, result) },
	})
	code, out, stderr := run(t, f, goodKey, "audit", "verify")
	if code != ExitOK || !strings.Contains(out, "Verified across 1 restore anchor") || !strings.Contains(out, "local:root@harbor") {
		t.Fatalf("a chain through a restore: exit %d, %q %q", code, out, stderr)
	}
	result = map[string]any{"verified": false, "firstBrokenId": "row9", "reason": "row hash mismatch", "anchors": []any{}}
	code, out, _ = run(t, f, goodKey, "audit", "verify")
	if code != ExitFailure || !strings.Contains(out, "Broken at row9: row hash mismatch") {
		t.Fatalf("a broken chain: exit %d, %q", code, out)
	}
}
