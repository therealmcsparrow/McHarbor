// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	reSyncComment = regexp.MustCompile(`[ \t]*//.*$`)
	reSyncSpaces  = regexp.MustCompile(` +`)
)

// normalizeProtocol applies the Makefile's check-protocol normalisation: no
// package line, no comments, no blank lines, and runs of whitespace squeezed.
func normalizeProtocol(raw string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "package ") {
			continue
		}
		line = reSyncComment.ReplaceAllString(line, "")
		if strings.TrimSpace(line) == "" {
			continue
		}
		line = reSyncSpaces.ReplaceAllString(strings.ReplaceAll(line, "\t", " "), " ")
		out = append(out, line)
	}
	return out
}

// TestProtocolMatchesTheAgentCopy fails when this file and the agent's copy
// drift: the agent silently drops a field or message it does not know.
func TestProtocolMatchesTheAgentCopy(t *testing.T) {
	agentCopy := filepath.Join("..", "..", "..", "agent", "protocol.go")
	theirs, err := os.ReadFile(agentCopy)
	if err != nil {
		t.Skipf("the agent source is not in this checkout: %v", err)
	}
	ours, err := os.ReadFile("protocol.go")
	if err != nil {
		t.Fatal(err)
	}
	a, b := normalizeProtocol(string(ours)), normalizeProtocol(string(theirs))
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			t.Fatalf("protocol.go and %s differ at normalised line %d:\n  core:  %s\n  agent: %s", agentCopy, i+1, a[i], b[i])
		}
	}
	if len(a) != len(b) {
		t.Fatalf("protocol.go has %d normalised lines, %s has %d", len(a), agentCopy, len(b))
	}
}
