// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"fmt"
	"os"
	"path/filepath"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// validateAgentRPC holds an agent-rpc declaration inside the host's ceiling. The agent runs
// as root, so a declaration can only narrow what core/agent already allows plugins to send
// and observe; it can never name a type the host has not reviewed.
func validateAgentRPC(c Capability) error {
	seen := map[string]bool{}
	for _, msgType := range c.Send {
		if _, ok := coreagent.PluginRouteFor(msgType); !ok {
			return fmt.Errorf("send: %q is outside the agent-rpc ceiling; a plugin may send only the agent messages the host lists", msgType)
		}
		if seen[msgType] {
			return fmt.Errorf("send: %q is listed twice", msgType)
		}
		seen[msgType] = true
	}
	seen = map[string]bool{}
	for _, msgType := range c.Observe {
		if _, ok := coreagent.PluginObservedFor(msgType); !ok {
			return fmt.Errorf("observe: %q is not an observable agent frame", msgType)
		}
		if seen[msgType] {
			return fmt.Errorf("observe: %q is listed twice", msgType)
		}
		seen[msgType] = true
	}
	return nil
}

// AgentRPCDeclaration reads the agent-rpc capability out of a plugin's installed manifest.
// It returns false when the plugin declares none, and also when any of its capabilities is
// invalid, because the loader drops a bad declaration set whole and the agent view must
// never outlive the declaration that justified it.
func AgentRPCDeclaration(slug, dir string) (Capability, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return Capability{}, false
	}
	caps, err := Parse(slug, raw)
	if err != nil {
		return Capability{}, false
	}
	for _, c := range caps {
		if c.Kind == KindAgentRPC {
			return c, true
		}
	}
	return Capability{}, false
}
