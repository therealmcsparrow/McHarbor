// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/modules/workflows"
)

// PluginWorkflowBridge is the host side of plugin.WorkflowHandle. It
// is handed to the plugin runtime at boot, before the workflow trigger
// service exists, and attached to that service afterwards; until then
// every call reports the bridge as unavailable rather than panicking.
type PluginWorkflowBridge struct {
	trigger atomic.Pointer[workflows.TriggerService]
}

// NewPluginWorkflowBridge returns an unattached bridge.
func NewPluginWorkflowBridge() *PluginWorkflowBridge { return &PluginWorkflowBridge{} }

// Attach binds the bridge to the live trigger service.
func (b *PluginWorkflowBridge) Attach(ts *workflows.TriggerService) {
	if ts != nil {
		b.trigger.Store(ts)
	}
}

var errWorkflowBridgeUnavailable = errors.New("workflow trigger service not attached yet")

// TriggerBindings lists the workflow triggers bound to actionKey, and reports errWorkflowBridgeUnavailable until the trigger service is attached.
func (b *PluginWorkflowBridge) TriggerBindings(ctx context.Context, actionKey string) ([]pluginruntime.WorkflowTriggerBindingInfo, error) {
	ts := b.trigger.Load()
	if ts == nil {
		return nil, errWorkflowBridgeUnavailable
	}
	bindings, err := ts.PluginTriggerBindings(ctx, actionKey)
	if err != nil {
		return nil, err
	}
	return toBindingInfos(bindings), nil
}

// errUnscopedFire is what the unscoped bridge answers: a fire must name the plugin it is
// for, which only ForPlugin's scoped source does.
var errUnscopedFire = errors.New("workflows.fire: the bridge fires only through a plugin-scoped source")

// Fire refuses: an unscoped fire cannot say which plugin it comes from, so it could name
// any plugin's action key. The loader fires through ForPlugin instead.
func (b *PluginWorkflowBridge) Fire(context.Context, string, map[string]any, func(pluginruntime.WorkflowTriggerBindingInfo) bool) (int, error) {
	return 0, errUnscopedFire
}

// ForPlugin returns the bridge scoped to one plugin, whose Fire accepts only that
// plugin's own `plugin:<slug>:` action keys.
func (b *PluginWorkflowBridge) ForPlugin(slug string) pluginruntime.WorkflowTriggerSource {
	return &pluginWorkflowScope{bridge: b, slug: slug}
}

// pluginWorkflowScope is the per-plugin face of the bridge the loader hands one plugin.
type pluginWorkflowScope struct {
	bridge *PluginWorkflowBridge
	slug   string
}

// TriggerBindings lists the bindings of actionKey; reading them grants nothing to fire.
func (s *pluginWorkflowScope) TriggerBindings(ctx context.Context, actionKey string) ([]pluginruntime.WorkflowTriggerBindingInfo, error) {
	return s.bridge.TriggerBindings(ctx, actionKey)
}

// Fire runs every workflow bound to actionKey whose binding passes match, refusing any key
// that is not this plugin's own, and returns how many started.
func (s *pluginWorkflowScope) Fire(ctx context.Context, actionKey string, payload map[string]any, match func(pluginruntime.WorkflowTriggerBindingInfo) bool) (int, error) {
	ts := s.bridge.trigger.Load()
	if ts == nil {
		return 0, errWorkflowBridgeUnavailable
	}
	var m func(workflows.PluginTriggerBinding) bool
	if match != nil {
		m = func(pb workflows.PluginTriggerBinding) bool {
			return match(pluginruntime.WorkflowTriggerBindingInfo{WorkflowID: pb.WorkflowID, NodeID: pb.NodeID, Config: pb.Config})
		}
	}
	return ts.FirePluginTriggerFor(ctx, s.slug, actionKey, payload, m)
}

// DockerEnvironmentIDs returns the Docker environments workflows are currently active on.
func (s *pluginWorkflowScope) DockerEnvironmentIDs(ctx context.Context) ([]string, error) {
	return s.bridge.DockerEnvironmentIDs(ctx)
}

// DockerEnvironmentIDs returns the Docker environments workflows are currently active on.
func (b *PluginWorkflowBridge) DockerEnvironmentIDs(ctx context.Context) ([]string, error) {
	ts := b.trigger.Load()
	if ts == nil {
		return nil, errWorkflowBridgeUnavailable
	}
	return ts.ActiveDockerEnvIDs(ctx)
}

func toBindingInfos(bindings []workflows.PluginTriggerBinding) []pluginruntime.WorkflowTriggerBindingInfo {
	out := make([]pluginruntime.WorkflowTriggerBindingInfo, 0, len(bindings))
	for _, pb := range bindings {
		out = append(out, pluginruntime.WorkflowTriggerBindingInfo{WorkflowID: pb.WorkflowID, NodeID: pb.NodeID, Config: pb.Config})
	}
	return out
}
