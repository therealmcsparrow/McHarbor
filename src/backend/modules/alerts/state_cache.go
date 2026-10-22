// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import "sync"

// stateCache holds each rule's firing state in memory so an alert can fire when
// SQLite cannot be written; see docs/adr/0002-alert-state-in-memory-first.md.
type stateCache struct {
	mu    sync.Mutex
	rules map[string]map[string]alertState
}

func newStateCache() *stateCache {
	return &stateCache{rules: make(map[string]map[string]alertState)}
}

// sharedStateCache is the cache the engine, the rule service and the ITSM
// acknowledgement hook all invalidate, because they run in different objects.
var sharedStateCache = newStateCache()

// snapshot returns a copy of one rule's states, and false on a cold rule.
func (c *stateCache) snapshot(alertID string) (map[string]alertState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	states, ok := c.rules[alertID]
	if !ok {
		return nil, false
	}
	out := make(map[string]alertState, len(states))
	for k, v := range states {
		out[k] = v
	}
	return out, true
}

// seed installs the states loaded from SQLite for a cold rule.
func (c *stateCache) seed(alertID string, states map[string]alertState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make(map[string]alertState, len(states))
	for k, v := range states {
		copied[k] = v
	}
	c.rules[alertID] = copied
}

// put records the newest state of one subject.
func (c *stateCache) put(alertID string, state alertState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	states, ok := c.rules[alertID]
	if !ok {
		states = make(map[string]alertState)
		c.rules[alertID] = states
	}
	states[state.Key] = state
}

// forget removes subjects that stopped being reported.
func (c *stateCache) forget(alertID string, keys []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.rules[alertID], key)
	}
}

// drop makes one rule cold again, so its next evaluation reloads from SQLite.
func (c *stateCache) drop(alertID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.rules, alertID)
}

// dropAll makes every rule cold, for a node that lost leadership.
func (c *stateCache) dropAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = make(map[string]map[string]alertState)
}

// acknowledge marks a cached state as acknowledged by an ITSM.
func (c *stateCache) acknowledge(stateKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, states := range c.rules {
		if s, ok := states[stateKey]; ok {
			s.Acknowledged = true
			states[stateKey] = s
			return
		}
	}
}

// NoteAcknowledged tells the running engine an ITSM acknowledged a firing state,
// since the acknowledgement is written to SQLite by a module the cache cannot see.
func NoteAcknowledged(stateKey string) { sharedStateCache.acknowledge(stateKey) }

// DropAllStates makes every rule cold so the next tick reloads SQLite, for the
// moment another module re-keys alert_states underneath the cache (an environment merge).
func DropAllStates() { sharedStateCache.dropAll() }
