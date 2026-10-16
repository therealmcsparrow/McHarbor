// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_test

import (
	"strings"
	"testing"
)

// knownUngatedScopedReads names scoped GET routes that read one caller-chosen
// environment or agent but carry neither a permission gate nor an exempt
// reason. Remediation plan P1 fixes these; an entry that stops matching fails
// the test below, so this list can only shrink.
var knownUngatedScopedReads = map[string]bool{}

// scopedReadPrefixes are the route families TestScopedReadRoutesAreGated
// checks: environment and agent reads that take an id in the path and so can
// leak another tenant's data to whoever guesses it.
var scopedReadPrefixes = []string{"/api/environments", "/api/agents"}

// scopedPathParams are the path parameter names that identify a specific
// environment or agent rather than a sub-resource.
var scopedPathParams = []string{"{envId}", "{environmentId}", "{id}"}

// TestScopedReadRoutesAreGated fails when a GET under /api/environments or
// /api/agents that names a specific environment/agent in its path carries
// neither x-mcharbor-permission nor x-mcharbor-exempt — the shape that let
// GET /api/agents/{envId}/status return any environment's status to any
// authenticated user.
func TestScopedReadRoutesAreGated(t *testing.T) {
	var missing []string
	stillMissing := map[string]bool{}
	for _, s := range loadOperations(t) {
		if s.Method != "GET" {
			continue
		}
		matchesPrefix := false
		for _, prefix := range scopedReadPrefixes {
			matchesPrefix = matchesPrefix || strings.HasPrefix(s.Path, prefix)
		}
		if !matchesPrefix {
			continue
		}
		matchesParam := false
		for _, param := range scopedPathParams {
			matchesParam = matchesParam || strings.Contains(s.Path, param)
		}
		if !matchesParam {
			continue
		}
		if len(s.Op.Permission) > 0 || strings.TrimSpace(s.Op.Exempt) != "" {
			continue
		}
		key := s.Method + " " + s.Path
		if knownUngatedScopedReads[key] {
			stillMissing[key] = true
			continue
		}
		missing = append(missing, key)
	}
	for key := range knownUngatedScopedReads {
		if !stillMissing[key] {
			t.Errorf("%s is now gated or exempt; remove it from knownUngatedScopedReads", key)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d scoped GET routes under /api/environments or /api/agents carry neither a "+
			"permission gate nor an exempt reason:\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
}
