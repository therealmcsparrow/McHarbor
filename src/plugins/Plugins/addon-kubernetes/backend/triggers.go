// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Kubernetes event trigger.
//
// A poller rather than a watch: a watch per saved cluster would hold a
// long-lived connection per cluster for the life of the process, and
// most clusters here are reached through short-lived credentials. Every
// tick the poller lists events across all namespaces, fires the
// `kubernetes-event` trigger for each one newer than the previous
// tick, and remembers what it has seen so a repeated event (same uid,
// same count) is not fired twice. The first tick only records the
// baseline — a freshly installed plugin must not replay a day of
// history into every workflow.

const (
	nodeK8sEventTrigger = "kubernetes-event"

	k8sPollInterval  = 30 * time.Second
	k8sPollTimeout   = 45 * time.Second
	k8sFireTimeout   = 15 * time.Second
	k8sPollLimit     = 500
	k8sSeenRetention = 2 * time.Hour
)

func (p *KubernetesPlugin) kubernetesTriggerNodes() []struct {
	key  string
	desc plugin.NodeDescriptor
} {
	return []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{nodeK8sEventTrigger, plugin.NodeDescriptor{
			Category:    "trigger",
			DisplayName: "Kubernetes: Event",
			Description: "Starts when a cluster records an event (a Warning by default). msg.payload carries the event.",
			Icon:        "IconTopologyComplex",
			Inputs:      []string{},
			Outputs:     []string{"output"},
			Requires:    "kubernetes.view",
			Fields: []map[string]any{
				{"key": "cluster", "label": "Cluster ID (empty = any)", "type": "text", "required": false},
				{"key": "namespace", "label": "Namespace (empty = any)", "type": "text", "required": false},
				{"key": "type", "label": "Event type", "type": "select", "required": true, "default": "Warning",
					"options": []map[string]any{
						{"value": "Warning", "label": "Warning"},
						{"value": "Normal", "label": "Normal"},
						{"value": "any", "label": "Any"},
					}},
				{"key": "reason", "label": "Reason (empty = any; e.g. BackOff, Unhealthy, FailedScheduling)", "type": "text", "required": false},
				{"key": "kind", "label": "Object kind (empty = any; e.g. Pod, Deployment)", "type": "text", "required": false},
			},
			// No Execute: the host binds a pass-through; the poller
			// below fires the node with the event as msg.payload.
		}},
	}
}

type k8sEventKey struct {
	cluster string
	uid     string
	count   int32
}

type k8sWatcher struct {
	plugin *KubernetesPlugin
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	baseline map[string]bool // cluster id -> first tick done
	seen     map[k8sEventKey]time.Time
}

func (p *KubernetesPlugin) startWatcher() {
	if p.host.Workflows == nil {
		if p.host.Logger != nil {
			p.host.Logger.Info("kubernetes: host has no workflow bridge; kubernetes-event trigger inactive")
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &k8sWatcher{plugin: p, cancel: cancel, baseline: map[string]bool{}, seen: map[k8sEventKey]time.Time{}}
	p.watcher = w
	w.wg.Add(1)
	go w.run(ctx)
}

func (p *KubernetesPlugin) stopWatcher() {
	if p.watcher == nil {
		return
	}
	p.watcher.cancel()
	p.watcher.wg.Wait()
	p.watcher = nil
}

func (w *k8sWatcher) run(ctx context.Context) {
	defer w.wg.Done()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(k8sPollInterval):
		}
	}
}

// tick polls every saved cluster once. It skips the work entirely when
// no active workflow places the trigger, so an idle install costs
// nothing but the bindings query.
func (w *k8sWatcher) tick(ctx context.Context) {
	p := w.plugin
	if p.store == nil || p.pool == nil {
		return
	}
	bindCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	bindings, err := p.host.Workflows.TriggerBindings(bindCtx, nodeK8sEventTrigger)
	cancel()
	if err != nil || len(bindings) == 0 {
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	clusters, err := p.store.list(listCtx)
	cancel()
	if err != nil {
		if p.host.Logger != nil {
			p.host.Logger.Debug("kubernetes: event poller cannot list clusters", "error", err)
		}
		return
	}
	for _, c := range clusters {
		if !c.IsActive {
			continue
		}
		w.pollCluster(ctx, c.ID)
	}
	w.forget()
}

func (w *k8sWatcher) pollCluster(ctx context.Context, clusterID string) {
	p := w.plugin
	pollCtx, cancel := context.WithTimeout(ctx, k8sPollTimeout)
	defer cancel()
	cs, err := p.pool.Get(pollCtx, clusterID)
	if err != nil {
		return
	}
	list, err := cs.CoreV1().Events(metav1.NamespaceAll).List(pollCtx, metav1.ListOptions{Limit: k8sPollLimit})
	if err != nil {
		if p.host.Logger != nil {
			p.host.Logger.Debug("kubernetes: event poll failed", "error", err, "cluster", clusterID)
		}
		return
	}

	w.mu.Lock()
	first := !w.baseline[clusterID]
	w.baseline[clusterID] = true
	now := time.Now()
	fresh := make([]int, 0, 8)
	for i := range list.Items {
		x := &list.Items[i]
		key := k8sEventKey{cluster: clusterID, uid: string(x.UID), count: x.Count}
		if _, ok := w.seen[key]; ok {
			continue
		}
		w.seen[key] = now
		if !first {
			fresh = append(fresh, i)
		}
	}
	w.mu.Unlock()

	for _, i := range fresh {
		w.fire(ctx, clusterID, &list.Items[i])
	}
}

// forget drops seen-keys older than the retention window so the map
// cannot grow for the life of the process.
func (w *k8sWatcher) forget() {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := time.Now().Add(-k8sSeenRetention)
	for k, at := range w.seen {
		if at.Before(cutoff) {
			delete(w.seen, k)
		}
	}
}

func (w *k8sWatcher) fire(ctx context.Context, clusterID string, x *corev1.Event) {
	evtType := x.Type
	reason := x.Reason
	kind := x.InvolvedObject.Kind
	ns := x.Namespace
	when := x.LastTimestamp.Time
	if when.IsZero() {
		when = x.EventTime.Time
	}
	payload := map[string]any{
		"trigger":       nodeK8sEventTrigger,
		"cluster":       clusterID,
		"namespace":     ns,
		"type":          evtType,
		"reason":        reason,
		"kind":          kind,
		"name":          x.InvolvedObject.Name,
		"object":        kind + "/" + x.InvolvedObject.Name,
		"message":       x.Message,
		"count":         x.Count,
		"source":        x.Source.Component,
		"time":          when.UTC().Format(time.RFC3339),
		"autoTriggered": true,
	}
	match := func(b plugin.WorkflowTriggerBinding) bool {
		if want := nodeString(b.Config, "cluster", ""); want != "" && want != clusterID {
			return false
		}
		if want := nodeString(b.Config, "namespace", ""); want != "" && want != ns {
			return false
		}
		if want := nodeString(b.Config, "type", "Warning"); want != "any" && !strings.EqualFold(want, evtType) {
			return false
		}
		if want := nodeString(b.Config, "reason", ""); want != "" && !strings.EqualFold(want, reason) {
			return false
		}
		if want := nodeString(b.Config, "kind", ""); want != "" && !strings.EqualFold(want, kind) {
			return false
		}
		return true
	}
	fireCtx, cancel := context.WithTimeout(ctx, k8sFireTimeout)
	defer cancel()
	started, err := w.plugin.host.Workflows.Fire(fireCtx, nodeK8sEventTrigger, payload, match)
	if err != nil {
		if w.plugin.host.Logger != nil {
			w.plugin.host.Logger.Warn("kubernetes: firing event trigger failed", "error", err, "cluster", clusterID, "reason", reason)
		}
		return
	}
	if started > 0 && w.plugin.host.Logger != nil {
		w.plugin.host.Logger.Info("kubernetes: event trigger fired", "cluster", clusterID, "namespace", ns, "type", evtType, "reason", reason, "object", payload["object"], "runs", started)
	}
}
