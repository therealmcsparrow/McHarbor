// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import "testing"

type embeddedBase struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type embeddedByValue struct {
	embeddedBase
	Key string `json:"key"`
}

type embeddedByPointer struct {
	*embeddedBase
	Key string `json:"key"`
}

// TestEmbeddedStructsAreFlattened matches encoding/json, which promotes an
// untagged embedded struct's fields; a nested property would make generated
// clients decode a shape the server never sends.
func TestEmbeddedStructsAreFlattened(t *testing.T) {
	for name, v := range map[string]any{"value": embeddedByValue{}, "pointer": embeddedByPointer{}} {
		b := newSchemaBuilder()
		ref := b.schemaFor(v)
		key := ref["$ref"].(string)[len("#/components/schemas/"):]
		props := b.components[key].(map[string]any)["properties"].(map[string]any)
		for _, want := range []string{"id", "name", "key"} {
			if _, ok := props[want]; !ok {
				t.Errorf("%s: property %q missing, got %v", name, want, props)
			}
		}
		if _, nested := props["embeddedBase"]; nested {
			t.Errorf("%s: embedded struct rendered as a nested property", name)
		}
	}
}
