// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"testing"
)

// scopingSource records which plugin it was scoped to; its unscoped face refuses to fire.
type scopingSource struct {
	scopedTo string
	face     string
}

func (s *scopingSource) TriggerBindings(context.Context, string) ([]WorkflowTriggerBindingInfo, error) {
	return nil, nil
}

func (s *scopingSource) Fire(_ context.Context, actionKey string, _ map[string]any, _ func(WorkflowTriggerBindingInfo) bool) (int, error) {
	if s.face == "" {
		return 0, errors.New("the unscoped face must never be fired")
	}
	if actionKey != "plugin:"+s.face+":demo-trigger" {
		return 0, errors.New("unexpected action key " + actionKey)
	}
	return 1, nil
}

func (s *scopingSource) DockerEnvironmentIDs(context.Context) ([]string, error) { return nil, nil }

func (s *scopingSource) ForPlugin(slug string) WorkflowTriggerSource {
	s.scopedTo = slug
	return &scopingSource{face: slug}
}

// TestWorkflowHandleFiresThroughThePluginScope: when the host source can scope itself, a
// plugin's handle is built on the scope for that plugin, never on the unscoped source.
func TestWorkflowHandleFiresThroughThePluginScope(t *testing.T) {
	src := &scopingSource{}
	handle := workflowHandleFor(src, "addon-demo")
	if src.scopedTo != "addon-demo" {
		t.Fatalf("the handle must be scoped to its plugin, got %q", src.scopedTo)
	}
	if _, err := handle.Fire(context.Background(), "demo-trigger", nil, nil); err != nil {
		t.Fatalf("fire through the scope: %v", err)
	}
}
