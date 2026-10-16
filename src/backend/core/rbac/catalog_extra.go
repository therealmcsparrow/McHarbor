// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import "sync"

// The append seam for permissions declared in their own file.
//
// The catalog's first three layers — a const, a CatalogSeed entry and an
// AllPermissions member — all live in two shared lists, so every feature that
// adds a key edits the same two places. That is why `AllPermissions` has drifted
// from `CatalogSeed()` before: an entry added to one and forgotten in the other
// leaves a key with no scope, invisible to the role editor, failing the wrong way.
//
// RegisterPermissions takes all three in one call, from one file, so the three
// cannot disagree. Both forms are supported deliberately — the shared lists are
// the historical record of what shipped, and rewriting 200 entries to prove a
// point would be a worse change than letting new areas use the seam.

var (
	extraMu          sync.RWMutex
	extraPermissions []PermissionEntry
)

// RegisterPermissions adds entries to the catalog from an init() in the file
// that declares their constants. Registering the same key twice panics at
// startup rather than letting two areas disagree about its scope.
func RegisterPermissions(entries ...PermissionEntry) {
	extraMu.Lock()
	defer extraMu.Unlock()
	seen := make(map[Permission]bool, len(extraPermissions))
	for _, e := range extraPermissions {
		seen[e.Key] = true
	}
	for _, e := range entries {
		if seen[e.Key] {
			panic("rbac: permission " + string(e.Key) + " is registered twice") // panic-ok: init-time registration conflict must abort startup
		}
		seen[e.Key] = true
		extraPermissions = append(extraPermissions, e)
	}
}

// registeredPermissions returns the entries added through the seam.
func registeredPermissions() []PermissionEntry {
	extraMu.RLock()
	defer extraMu.RUnlock()
	out := make([]PermissionEntry, len(extraPermissions))
	copy(out, extraPermissions)
	return out
}
