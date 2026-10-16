// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"slices"
	"testing"
)

// TestChannelAliasesResolveOnlyWhileTheirPluginIsLoaded: a channel-type claim runs as the
// host's send node bound to the claiming plugin's channel, never as a plugin node, and a
// plugin that unloads takes its claim with it.
func TestChannelAliasesResolveOnlyWhileTheirPluginIsLoaded(t *testing.T) {
	reg := NewRegistry()
	reg.Set("comm-pigeon", []Capability{
		{Kind: KindWorkflowNodeAlias, Alias: "send-pigeon", ChannelType: "pigeon"},
		{Kind: KindEmailChannel, ChannelType: "pigeon"},
	})
	reg.Set("addon-demo", []Capability{{Kind: KindWorkflowNodeAlias, Alias: "legacy-trigger", Node: "demo-trigger"}})

	if ct, ok := reg.ChannelNodeAlias("send-pigeon"); !ok || ct != "pigeon" {
		t.Fatalf("send-pigeon: want the pigeon channel, got %q (%v)", ct, ok)
	}
	if action, ok := reg.NodeAliasAction("send-pigeon"); ok {
		t.Fatalf("a channel claim must not resolve to a plugin node, got %q", action)
	}
	if _, ok := reg.ChannelNodeAlias("legacy-trigger"); ok {
		t.Fatal("a plugin-node claim must not resolve to a channel")
	}
	if _, ok := reg.ChannelNodeAlias("plugin:comm-pigeon:send-pigeon"); ok {
		t.Fatal("a composite action is never re-resolved")
	}
	if !reg.IsEmailChannel("pigeon") || !slices.Equal(reg.EmailChannelTypes(), []string{"pigeon"}) {
		t.Fatalf("want pigeon as the only email channel, got %v", reg.EmailChannelTypes())
	}

	reg.Remove("comm-pigeon")
	if _, ok := reg.ChannelNodeAlias("send-pigeon"); ok {
		t.Fatal("an unloaded plugin's alias must stay unresolved")
	}
	if reg.IsEmailChannel("pigeon") || len(reg.EmailChannelTypes()) != 0 {
		t.Fatalf("an unloaded plugin must leave no email channel, got %v", reg.EmailChannelTypes())
	}
}
