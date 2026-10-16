// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"testing"
)

func TestClusterGroupIsNotDuplicated(t *testing.T) {
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_PLUGIN_CLI", "1")
	root, _ := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	count := 0
	var cluster = map[string]bool{}
	for _, c := range root.Commands() {
		if c.Name() == "cluster" {
			count++
			for _, sub := range c.Commands() {
				cluster[sub.Name()] = true
			}
		}
	}
	if count != 1 {
		t.Fatalf("there must be exactly one cluster group, found %d", count)
	}
	for _, want := range []string{"status", "failover", "drain", "resume", "readiness", "agent-routing"} {
		if !cluster[want] {
			t.Errorf("cluster %s is missing; the HA commands and the status command must share one group", want)
		}
	}
}

// TestGitopsGroupIsNotDuplicated: `gitops preview` attaches to the group
// addGitopsCommands created, so registration order matters. Registered too
// early it creates a second `gitops`, one of which is unreachable.
func TestGitopsGroupIsNotDuplicated(t *testing.T) {
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_PLUGIN_CLI", "1")
	root, _ := NewRoot(&bytes.Buffer{}, &bytes.Buffer{})
	count := 0
	subs := map[string]bool{}
	for _, c := range root.Commands() {
		if c.Name() == "gitops" {
			count++
			for _, sub := range c.Commands() {
				subs[sub.Name()] = true
			}
		}
	}
	if count != 1 {
		t.Fatalf("there must be exactly one gitops group, found %d", count)
	}
	for _, want := range []string{"pipeline", "approval", "preview"} {
		if !subs[want] {
			t.Errorf("gitops %s is missing; preview must join the existing group", want)
		}
	}
}
