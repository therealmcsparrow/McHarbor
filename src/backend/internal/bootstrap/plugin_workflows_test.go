// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"errors"
	"testing"
)

// TestUnscopedWorkflowBridgeRefusesToFire: the bridge itself cannot say which plugin a
// fire comes from, so it refuses every fire; only a ForPlugin scope may fire.
func TestUnscopedWorkflowBridgeRefusesToFire(t *testing.T) {
	b := NewPluginWorkflowBridge()
	if _, err := b.Fire(context.Background(), "plugin:addon-demo:demo-trigger", nil, nil); !errors.Is(err, errUnscopedFire) {
		t.Fatalf("an unscoped fire must be refused, got %v", err)
	}
	scope, ok := b.ForPlugin("addon-demo").(*pluginWorkflowScope)
	if !ok || scope.slug != "addon-demo" {
		t.Fatalf("ForPlugin must return a scope bound to its slug, got %#v", b.ForPlugin("addon-demo"))
	}
	if _, err := scope.Fire(context.Background(), "plugin:addon-demo:demo-trigger", nil, nil); !errors.Is(err, errWorkflowBridgeUnavailable) {
		t.Fatalf("an unattached scope reports the bridge unavailable, got %v", err)
	}
}
