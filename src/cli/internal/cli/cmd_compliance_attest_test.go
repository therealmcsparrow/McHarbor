// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestComplianceAttestRefusesWithoutANote(t *testing.T) {
	f := groupServer(t, nil)
	code, _, stderr := run(t, f, goodKey, "--yes", "compliance", "attest", "access-review")
	if code != ExitUsage || !strings.Contains(stderr, "--note is required") {
		t.Fatalf("exit %d, stderr %q; want a usage error naming --note", code, stderr)
	}
	if n := len(f.seen()); n != 0 {
		t.Fatalf("a refused sign-off still sent %d requests", n)
	}
}

func TestComplianceAttestSendsTheRunAndTheNote(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/compliance/attestations": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"id": "att-1", "framework": "cis-docker-benchmark"})
		},
	})
	code, _, stderr := run(t, f, goodKey, "--yes", "compliance", "attest", "cis-docker-benchmark",
		"--run", "run-7", "--note", "Accepted; 2.13 tracked in CHG-42")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	f.mu.Lock()
	raw := f.bodies["POST /api/compliance/attestations"]
	f.mu.Unlock()
	var body map[string]string
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("request body %q: %v", raw, err)
	}
	if body["framework"] != "cis-docker-benchmark" || body["runId"] != "run-7" || body["note"] != "Accepted; 2.13 tracked in CHG-42" {
		t.Fatalf("request body %v", body)
	}
}

func TestComplianceAttestationsListsByFramework(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"GET /api/compliance/attestations": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"items": []map[string]any{{
				"id": "att-1", "framework": "access-review", "runId": "", "attestedByName": "ann",
				"attestedAt": "2026-09-27T10:00:00Z", "note": "Q3 review",
			}}, "total": 1, "page": 1, "per_page": 25, "total_pages": 1})
		},
	})
	code, out, stderr := run(t, f, goodKey, "compliance", "attestations", "--framework", "access-review")
	if code != ExitOK || !strings.Contains(out, "Q3 review") || !strings.Contains(out, "ann") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, stderr)
	}
	reqs := f.seen()
	if len(reqs) != 1 || reqs[0].URL.Query().Get("framework") != "access-review" {
		t.Fatalf("requests %v", reqs)
	}
}
