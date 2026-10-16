// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

// ContainerIdentityExpr groups history rows by the container they logically
// belong to, for queries that sweep every container rather than filtering one.
//
// Rows written since migration 083 carry a `container_key`; rows written before
// it carry NULL and fall back to their name, which is stable across a Compose
// recreate for the same reason the key is. `container_id` is the last resort so
// the expression can never evaluate to NULL and silently collapse every
// unnamed row into one group.
//
// NULLIF rather than a bare COALESCE because both text columns can be the empty
// string as well as NULL, and COALESCE would happily return "".
const ContainerIdentityExpr = `COALESCE(NULLIF(container_key, ''), NULLIF(container_name, ''), container_id)`

// ContainerScope is the resolved answer to "which history rows belong to this
// container". It is built once per request and shared by every tier the request
// reads, so the identity is resolved once rather than per query.
type ContainerScope struct {
	// Where is a WHERE fragment with `?` placeholders. It is a fixed string in
	// every branch — no caller input is ever interpolated into it.
	Where string

	filter []any
}

// NewContainerScope builds the filter that selects every history row belonging
// to one logical container.
//
// The three cases, in the order they are tried:
//
//   - key and name known — match the key, and also match legacy rows (written
//     before migration 083, so their key is NULL) on the name. This is what
//     stitches a series across the upgrade boundary as well as across a
//     recreate.
//   - key only — match the key alone.
//   - neither — fall back to the container id, which is what every one of these
//     queries did before the key existed. This is the path taken in the ≤30s
//     between a container being created and the collector's first tick writing
//     a keyed row for it.
func NewContainerScope(key, name, containerID string) ContainerScope {
	switch {
	case key != "" && name != "":
		return ContainerScope{
			Where:  "(container_key = ? OR (container_key IS NULL AND container_name = ?))",
			filter: []any{key, name},
		}
	case key != "":
		return ContainerScope{Where: "container_key = ?", filter: []any{key}}
	default:
		return ContainerScope{Where: "container_id = ?", filter: []any{containerID}}
	}
}

// Args splices the scope's own arguments in front of the rest of the query's.
// The scope fragment therefore has to lead the WHERE clause, which every caller
// writes it to do.
func (s ContainerScope) Args(rest ...any) []any {
	out := make([]any, 0, len(s.filter)+len(rest))
	out = append(out, s.filter...)
	return append(out, rest...)
}
