// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package pluginslug holds the plugin slugs the host keeps for itself. It lives outside
// core/plugin/** so that reserving a slug does not change the ABI every plugin links against.
package pluginslug

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrReserved means a manifest names a slug the host uses as an identity of its own.
var ErrReserved = errors.New("plugin slug is reserved by the host")

// reserved is the closed set; src/contracts/pluginslugs/reserved.yaml mirrors it for the
// CLI. "host" is the workflow trigger's fire source for the host's own events, so a plugin
// slugged "host" would be allowed to fire host-only actions.
var reserved = map[string]bool{
	"host": true,
}

// Reserved reports whether slug is kept by the host, compared case- and space-insensitively
// so no spelling of it slips past a check that is stricter elsewhere.
func Reserved(slug string) bool {
	return reserved[strings.ToLower(strings.TrimSpace(slug))]
}

// Check refuses a reserved slug with ErrReserved; every install and every manifest load
// calls it.
func Check(slug string) error {
	if Reserved(slug) {
		return fmt.Errorf("%w: %q", ErrReserved, slug)
	}
	return nil
}

// List returns the reserved slugs, sorted.
func List() []string {
	out := make([]string, 0, len(reserved))
	for slug := range reserved {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
