// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"sort"
	"sync"
)

// Registry holds the capabilities of every loaded plugin, keyed by slug. An entry exists
// only while its plugin is loaded, so "declared here" also means "reachable now".
type Registry struct {
	mu     sync.RWMutex
	bySlug map[string][]Capability
}

// NewRegistry returns an empty registry; tests use their own, production uses Default.
func NewRegistry() *Registry {
	return &Registry{bySlug: map[string][]Capability{}}
}

// Default is the registry the plugin runtime writes and host consumers read.
var Default = NewRegistry()

// Set replaces slug's capabilities; an empty set removes the slug, which is what a
// reload of a plugin that dropped its declarations needs.
func (r *Registry) Set(slug string, caps []Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(caps) == 0 {
		delete(r.bySlug, slug)
		return
	}
	out := make([]Capability, len(caps))
	copy(out, caps)
	for i := range out {
		out[i].Plugin = slug
	}
	r.bySlug[slug] = out
}

// Remove drops slug's capabilities when the plugin unloads, is disabled or uninstalled.
func (r *Registry) Remove(slug string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.bySlug, slug)
}

// All returns every capability, sorted by plugin then declaration order, so the
// endpoint's output is stable.
func (r *Registry) All() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slugs := make([]string, 0, len(r.bySlug))
	for s := range r.bySlug {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	out := []Capability{}
	for _, s := range slugs {
		out = append(out, r.bySlug[s]...)
	}
	return out
}

// OfKind returns every capability of kind, in All's order.
func (r *Registry) OfKind(kind Kind) []Capability {
	out := []Capability{}
	for _, c := range r.All() {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// Provider returns the capability of kind keyed by key (a runbook domain, a source
// kind, a metric…). Two plugins claiming the same key resolve to the first slug in sort
// order, so the answer never depends on load order.
func (r *Registry) Provider(kind Kind, key string) (Capability, bool) {
	for _, c := range r.OfKind(kind) {
		if c.discriminator() == key {
			return c, true
		}
	}
	return Capability{}, false
}
