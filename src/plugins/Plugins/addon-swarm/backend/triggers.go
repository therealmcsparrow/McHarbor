// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Swarm event trigger.
//
// The host's own docker-event trigger only carries container events.
// Swarm emits its orchestration on the same stream under other types
// — service, node, secret, config — and only on managers. This watcher
// subscribes to those types on every Docker environment the host
// knows, skipping the ones that are not a Swarm manager, and fires the
// `swarm-event` trigger node through HostContext.Workflows.
//
// One goroutine per environment with its own reconnect loop, mirroring
// the host watcher; the discovery loop re-lists environments every 30 s
// so a Swarm initialised after boot is picked up without a restart.

const (
	nodeSwarmEventTrigger = "swarm-event"

	swarmWatchRediscover = 30 * time.Second
	swarmWatchRetry      = 10 * time.Second
	swarmFireTimeout     = 15 * time.Second
)

// swarmEventTypes are the Swarm-level event types worth a trigger.
var swarmEventTypes = []string{"service", "node", "secret", "config"}

func (p *SwarmPlugin) swarmTriggerNodes() []struct {
	key  string
	desc plugin.NodeDescriptor
} {
	return []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{nodeSwarmEventTrigger, plugin.NodeDescriptor{
			Category:    "trigger",
			DisplayName: "Swarm: Event",
			Description: "Starts when a Swarm service, node, secret or config changes. msg.payload carries the event.",
			Icon:        "IconTopologyStarRing3",
			Inputs:      []string{},
			Outputs:     []string{"output"},
			Requires:    "swarm.view",
			Fields: []map[string]any{
				{"key": "environment", "label": "Environment (empty = any)", "type": "environment-select", "required": false},
				{"key": "type", "label": "Object type", "type": "select", "required": true, "default": "any",
					"options": []map[string]any{
						{"value": "any", "label": "Any"},
						{"value": "service", "label": "Service"},
						{"value": "node", "label": "Node"},
						{"value": "secret", "label": "Secret"},
						{"value": "config", "label": "Config"},
					}},
				{"key": "action", "label": "Action (empty = any; e.g. create, update, remove)", "type": "text", "required": false},
				{"key": "name", "label": "Object name (empty = any)", "type": "text", "required": false},
			},
			// No Execute: the host binds a pass-through; the watcher
			// below fires the node with the event as msg.payload.
		}},
	}
}

// swarmWatcher owns the per-environment event subscriptions.
type swarmWatcher struct {
	plugin *SwarmPlugin
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (p *SwarmPlugin) startWatcher() {
	if p.host.Workflows == nil {
		if p.host.Logger != nil {
			p.host.Logger.Info("swarm: host has no workflow bridge; swarm-event trigger inactive")
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &swarmWatcher{plugin: p, cancel: cancel}
	p.watcher = w
	w.wg.Add(1)
	go w.discover(ctx)
}

func (p *SwarmPlugin) stopWatcher() {
	if p.watcher == nil {
		return
	}
	p.watcher.cancel()
	p.watcher.wg.Wait()
	p.watcher = nil
}

func (w *swarmWatcher) discover(ctx context.Context) {
	defer w.wg.Done()
	watching := map[string]bool{}
	for {
		if ids, err := w.plugin.host.Workflows.DockerEnvironmentIDs(ctx); err == nil {
			for _, envID := range ids {
				if watching[envID] {
					continue
				}
				watching[envID] = true
				w.wg.Add(1)
				go w.watchEnv(ctx, envID)
			}
		} else if w.plugin.host.Logger != nil {
			w.plugin.host.Logger.Debug("swarm: event watcher cannot list environments", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(swarmWatchRediscover):
		}
	}
}

func (w *swarmWatcher) watchEnv(ctx context.Context, envID string) {
	defer w.wg.Done()
	for {
		w.listen(ctx, envID)
		select {
		case <-ctx.Done():
			return
		case <-time.After(swarmWatchRetry):
		}
	}
}

// listen subscribes once and returns when the stream ends. An
// environment that is not a Swarm manager returns immediately and is
// re-probed on the next retry, which is how a later `swarm init` on
// that host starts feeding events without a restart.
func (w *swarmWatcher) listen(ctx context.Context, envID string) {
	svc := w.plugin.handler.svc
	if svc == nil {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	capability, err := svc.Capability(probeCtx, envID)
	cancel()
	if err != nil || !capability.Capable || !capability.ControlAvailable {
		return
	}
	cli, err := svc.getClient(envID)
	if err != nil {
		return
	}
	args := make(client.Filters)
	for _, t := range swarmEventTypes {
		args.Add("type", t)
	}
	stream := cli.Events(ctx, client.EventsListOptions{Filters: args})
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-stream.Err:
			if ok && err != nil && ctx.Err() == nil && w.plugin.host.Logger != nil {
				w.plugin.host.Logger.Debug("swarm: event stream ended", "error", err, "env", envID)
			}
			return
		case evt, ok := <-stream.Messages:
			if !ok {
				return
			}
			w.fire(ctx, envID, evt)
		}
	}
}

func (w *swarmWatcher) fire(ctx context.Context, envID string, evt events.Message) {
	evtType := string(evt.Type)
	action := string(evt.Action)
	name := evt.Actor.Attributes["name"]
	payload := map[string]any{
		"trigger":       nodeSwarmEventTrigger,
		"environmentId": envID,
		"type":          evtType,
		"action":        action,
		"id":            evt.Actor.ID,
		"name":          name,
		"attributes":    evt.Actor.Attributes,
		"time":          time.Unix(evt.Time, 0).UTC().Format(time.RFC3339),
		"autoTriggered": true,
	}
	match := func(b plugin.WorkflowTriggerBinding) bool {
		if want := nodeString(b.Config, "environment", ""); want != "" && want != envID {
			return false
		}
		if want := nodeString(b.Config, "type", "any"); want != "any" && want != evtType {
			return false
		}
		if want := nodeString(b.Config, "action", ""); want != "" && !strings.EqualFold(want, action) {
			return false
		}
		if want := nodeString(b.Config, "name", ""); want != "" && want != name && want != evt.Actor.ID {
			return false
		}
		return true
	}
	fireCtx, cancel := context.WithTimeout(ctx, swarmFireTimeout)
	defer cancel()
	started, err := w.plugin.host.Workflows.Fire(fireCtx, nodeSwarmEventTrigger, payload, match)
	if err != nil {
		if w.plugin.host.Logger != nil {
			w.plugin.host.Logger.Warn("swarm: firing event trigger failed", "error", err, "env", envID, "type", evtType, "action", action)
		}
		return
	}
	if started > 0 && w.plugin.host.Logger != nil {
		w.plugin.host.Logger.Info("swarm: event trigger fired", "env", envID, "type", evtType, "action", action, "name", name, "runs", started)
	}
}
