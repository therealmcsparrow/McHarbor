// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/agent"
)

// TestRelayDropsTheCeilingsProgressFrames holds the relay to the plugin route ceiling: every
// progress frame a ceiling route may stream is droppable for a slow reader, the four guest
// frames the relay listed by hand before are among them, and no request or reply type is.
func TestRelayDropsTheCeilingsProgressFrames(t *testing.T) {
	declared := map[string]bool{}
	for _, route := range agent.PluginRoutes() {
		for _, frame := range route.Progress {
			declared[frame] = true
		}
	}
	if len(declared) == 0 {
		t.Fatal("the ceiling declares no progress frames")
	}
	for frame := range declared {
		if !isOutputFrame(frame) {
			t.Errorf("%s is a declared progress frame but the relay would never drop it", frame)
		}
	}
	for _, frame := range []string{agent.MsgImageFetchProgress, agent.MsgVMCloneProgress,
		agent.MsgVMMigrateProgress, agent.MsgCTCloneProgress} {
		if !declared[frame] {
			t.Errorf("%s is no longer a ceiling progress frame", frame)
		}
	}
	for _, route := range agent.PluginRoutes() {
		if isOutputFrame(route.Type) {
			t.Errorf("the request %s must never be dropped", route.Type)
		}
	}
	if isOutputFrame(agent.MsgVMState) {
		t.Errorf("a guest reply must never be dropped")
	}
}
