// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exemptPrefixes is the closed, reviewed vocabulary a x-mcharbor-exempt reason
// must start with. It replaces the out-of-date prose list in
// .instructions/sanity/permissions.md with something a test can enforce.
var exemptPrefixes = []string{
	"pre-auth:", "first-run:", "self-service:", "gated in handler:", "IdP callback:",
	"inbound webhook:", "public ITSM callback:", "agent transfer:", "node-to-node:",
	"device sign-in", "every user", "probe:",
}

// unknownExemptReasons returns every operation whose x-mcharbor-exempt reason
// does not start with one of exemptPrefixes.
func unknownExemptReasons(t *testing.T) []string {
	t.Helper()
	var bad []string
	for _, s := range loadOperations(t) {
		reason := strings.TrimSpace(s.Op.Exempt)
		if reason == "" {
			continue
		}
		known := false
		for _, prefix := range exemptPrefixes {
			if strings.HasPrefix(reason, prefix) {
				known = true
				break
			}
		}
		if !known {
			bad = append(bad, s.String()+" -> "+reason)
		}
	}
	return bad
}

// TestExemptReasonsUseAKnownPrefix turns the exempt reason list into a closed
// vocabulary: a route that skips permission gating must say why using one of
// the reviewed prefixes, not an ad-hoc sentence nobody reviewed.
func TestExemptReasonsUseAKnownPrefix(t *testing.T) {
	bad := unknownExemptReasons(t)
	if len(bad) > 0 {
		t.Errorf("%d exempt reasons do not start with a known prefix (%s):\n  %s",
			len(bad), strings.Join(exemptPrefixes, ", "), strings.Join(bad, "\n  "))
	}
}

// corruptedExemptReasonCopy writes a temp copy of the real openapi.json with the
// first x-mcharbor-exempt reason it finds replaced by "because", and returns the
// copy's path. It never touches the committed spec.
func corruptedExemptReasonCopy(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("openapi.json")
	if err != nil {
		t.Fatalf("read openapi.json: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse openapi.json: %v", err)
	}
	// path -> method -> raw operation body, mirroring loadOperations; "parameters"
	// is skipped below because its value is an array, not an operation object.
	var paths map[string]map[string]json.RawMessage
	if err := json.Unmarshal(doc["paths"], &paths); err != nil {
		t.Fatalf("parse openapi.json paths: %v", err)
	}
	corrupted := false
	for _, methods := range paths {
		for method, rawOp := range methods {
			if method == "parameters" {
				continue
			}
			var op map[string]json.RawMessage
			if err := json.Unmarshal(rawOp, &op); err != nil {
				continue
			}
			var reason string
			if err := json.Unmarshal(op["x-mcharbor-exempt"], &reason); err != nil || reason == "" {
				continue
			}
			corruptedReason, err := json.Marshal("because")
			if err != nil {
				t.Fatalf("marshal corrupted reason: %v", err)
			}
			op["x-mcharbor-exempt"] = corruptedReason
			newRawOp, err := json.Marshal(op)
			if err != nil {
				t.Fatalf("re-marshal operation: %v", err)
			}
			methods[method] = newRawOp
			corrupted = true
			break
		}
		if corrupted {
			break
		}
	}
	if !corrupted {
		t.Fatal("no x-mcharbor-exempt reason found in openapi.json to corrupt")
	}
	rawPaths, err := json.Marshal(paths)
	if err != nil {
		t.Fatalf("re-marshal paths: %v", err)
	}
	doc["paths"] = rawPaths
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-marshal openapi.json: %v", err)
	}
	path := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("write temp copy: %v", err)
	}
	return path
}

// TestExemptReasonsUseAKnownPrefixDetectsABadReason proves the check above can
// actually fail: it points specFile at a temp copy with one reason corrupted to
// "because", runs the same detection, and requires it to be reported.
func TestExemptReasonsUseAKnownPrefixDetectsABadReason(t *testing.T) {
	path := corruptedExemptReasonCopy(t)
	original := specFile
	specFile = path
	defer func() { specFile = original }()

	if bad := unknownExemptReasons(t); len(bad) == 0 {
		t.Fatal("expected the corrupted \"because\" reason to be reported, but nothing was")
	}
}
