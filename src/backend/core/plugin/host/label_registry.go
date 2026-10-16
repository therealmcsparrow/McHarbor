// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

import "sync"

// PermissionLabel maps a permission key to its i18n coordinates.
// The host's /api/plugins/loader/permission-labels endpoint publishes
// the full slice on every request; the frontend merges each entry
// into the `permissions` i18next namespace so the existing
// `t('permissions.' + key)` lookups pick up the plugin-provided label.
//
// Concurrency: LabelRegistry stores entries behind a mutex. Plugins
// call Register/Unregister at Init/Shutdown (low frequency) and the
// loader endpoint reads on every admin UI render — the read path
// returns a snapshot copy so callers never observe a torn state.
//
// See dependency-host-plugin.md §6.4 — the registry lets plugins own
// their permission labels without host locale changes.
type PermissionLabel struct {
	Key      string `json:"key"`
	NS       string `json:"ns"`        // plugin slug or i18n namespace
	LabelKey string `json:"label_key"` // dotted path inside the namespace
}

// LabelRegistry is the shared label store. The host hands a
// LabelRegistry to its RBACHandle implementation; plugins reach it
// through the HostContext.RBAC handle. The plugin_runtime service
// also holds a reference so its /loader/permission-labels endpoint
// can read the current set without an additional round-trip through
// the RBAC subsystem.
type LabelRegistry struct {
	mu      sync.RWMutex
	entries map[string]PermissionLabel
}

// NewLabelRegistry builds an empty registry. The zero value is not
// usable because the map is nil — callers must use this constructor.
func NewLabelRegistry() *LabelRegistry {
	return &LabelRegistry{entries: make(map[string]PermissionLabel)}
}

// Register associates a permission key with its i18n coordinates.
// Calling Register twice with the same key replaces the prior entry;
// the plugin runtime should treat that as a no-op (the loader logs
// it at debug level). Empty key, ns, or labelKey is a no-op so a
// plugin with no AddsPerms can safely skip the call entirely.
func (r *LabelRegistry) Register(key, ns, labelKey string) {
	if key == "" || ns == "" || labelKey == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = PermissionLabel{Key: key, NS: ns, LabelKey: labelKey}
}

// Unregister removes a label. Missing keys are silently ignored so
// the runtime can call this unconditionally during Shutdown.
func (r *LabelRegistry) Unregister(key string) {
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
}

// Snapshot returns a copy of the current label set. Callers (the
// loader endpoint) iterate the slice without holding the lock so
// JSON encoding never blocks a concurrent Register/Unregister.
func (r *LabelRegistry) Snapshot() []PermissionLabel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PermissionLabel, 0, len(r.entries))
	for _, v := range r.entries {
		out = append(out, v)
	}
	return out
}
