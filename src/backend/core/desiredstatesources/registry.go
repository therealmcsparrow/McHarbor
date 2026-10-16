// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package desiredstatesources holds the plugins whose kinds the reconciler manages through the
// desiredstate contract, keyed by slug, so the reconciler never imports the runtime.
package desiredstatesources

import (
	"sort"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

var (
	mu        sync.RWMutex
	providers = map[string]desiredstate.Provider{}
)

// Register publishes a loaded plugin's provider; a second call for the same slug
// replaces the first, which is what a plugin repair or update needs.
func Register(slug string, p desiredstate.Provider) {
	if slug == "" || p == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	providers[slug] = p
}

// Unregister removes a plugin's provider when it unloads, so the reconciler stops
// asking a plugin that is no longer there.
func Unregister(slug string) {
	mu.Lock()
	defer mu.Unlock()
	delete(providers, slug)
}

// Get returns the provider for slug, if that plugin is loaded and opted in.
func Get(slug string) (desiredstate.Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := providers[slug]
	return p, ok
}

// Slugs lists the plugins currently providing desired-state kinds, sorted for stable output.
func Slugs() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(providers))
	for slug := range providers {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
