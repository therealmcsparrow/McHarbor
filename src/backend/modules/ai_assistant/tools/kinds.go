// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

// Allowed focal kinds for the entity graph. The set is closed; unknown
// values are rejected so we don't pollute the cursor or the graph
// walker with garbage from a runaway model.
var validFocalKinds = map[string]struct{}{
	"container": {},
	"image":     {},
	"stack":     {},
	"env":       {},
	"workflow":  {},
	"scan":      {},
	"user":      {},
}

// IsValidFocalKind reports whether k is one of the supported entity
// kinds. Empty kind is not valid.
func IsValidFocalKind(k string) bool {
	_, ok := validFocalKinds[k]
	return ok
}
