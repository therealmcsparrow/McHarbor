// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import "testing"

func TestDropAllStatesMakesEveryRuleCold(t *testing.T) {
	sharedStateCache.put("rule-a", alertState{Key: "rule-a:env1:s"})
	sharedStateCache.put("rule-b", alertState{Key: "rule-b:env1:s"})
	DropAllStates()
	for _, id := range []string{"rule-a", "rule-b"} {
		if _, warm := sharedStateCache.snapshot(id); warm {
			t.Fatalf("%s is still warm after DropAllStates", id)
		}
	}
}
