// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package permcatalog

import "testing"

// TestCatalogScope pins the manifest-to-catalog scope translation every plugin
// key crosses on its way into the role editor, elevations and the OpenAPI spec.
func TestCatalogScope(t *testing.T) {
	for in, want := range map[string]string{
		"": "global", "global": "global", "env": "environment", "environment": "environment",
		"stack": "stack", "owner": "owner", "account": "account", "bogus": "global",
	} {
		if got := CatalogScope(in); got != want {
			t.Errorf("CatalogScope(%q) = %q, want %q", in, got, want)
		}
	}
}
