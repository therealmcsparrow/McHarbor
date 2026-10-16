// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package envredact

import (
	"strings"
	"sync"
)

// processPinned is the set of root-of-trust values this process registered at startup.
// Every Policy withholds them on top of its own Pinned list, so a redactor that was never
// handed the configuration — the AI assistant, a workflow node, a plugin sharing this
// package — still cannot print McHarbor's keys. There is deliberately no getter.
var processPinned struct {
	mu     sync.RWMutex
	values []string
}

// RegisterPinned adds values (the shared secret, the encryption key, the database DSN, the
// backup key) to the process-wide pinned set. Values shorter than the safe minimum are
// ignored, as PinnedValues does; registering one twice is harmless.
func RegisterPinned(values ...string) {
	add := PinnedValues(values...)
	if len(add) == 0 {
		return
	}
	processPinned.mu.Lock()
	defer processPinned.mu.Unlock()
	for _, v := range add {
		dup := false
		for _, have := range processPinned.values {
			if have == v {
				dup = true
				break
			}
		}
		if !dup {
			processPinned.values = append(processPinned.values, v)
		}
	}
}

// registeredPinned returns a copy of the process-wide pinned set.
func registeredPinned() []string {
	processPinned.mu.RLock()
	defer processPinned.mu.RUnlock()
	return append([]string(nil), processPinned.values...)
}

// ReplacePinned returns text with every registered pinned value replaced by with, for free
// text such as compose output that no Policy walks.
func ReplacePinned(text, with string) string {
	if text == "" {
		return text
	}
	for _, v := range registeredPinned() {
		text = strings.ReplaceAll(text, v, with)
	}
	return text
}

// allPinned is p.Pinned plus the registered set.
func (p Policy) allPinned() []string {
	reg := registeredPinned()
	if len(reg) == 0 {
		return p.Pinned
	}
	return append(append([]string(nil), p.Pinned...), reg...)
}
