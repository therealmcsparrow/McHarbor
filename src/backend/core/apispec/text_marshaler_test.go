// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"net/netip"
	"testing"
)

type addressed struct {
	IP      netip.Addr   `json:"ip"`
	Subnet  netip.Prefix `json:"subnet,omitzero"`
	Gateway *netip.Addr  `json:"gateway"`
}

// TestTextMarshalersAreStrings guards the Docker API types, whose netip fields
// encode as JSON strings: an object schema would make generated clients reject
// every response, and an omitzero field is absent on the wire when unset.
func TestTextMarshalersAreStrings(t *testing.T) {
	b := newSchemaBuilder()
	ref := b.schemaFor(addressed{})
	key := ref["$ref"].(string)[len("#/components/schemas/"):]
	schema := b.components[key].(map[string]any)
	props := schema["properties"].(map[string]any)
	for _, name := range []string{"ip", "subnet", "gateway"} {
		got := props[name].(map[string]any)
		if got["type"] != "string" || got["$ref"] != nil {
			t.Errorf("%s: schema = %v, want a plain string", name, got)
		}
	}
	required, _ := schema["required"].([]string)
	for _, r := range required {
		if r == "subnet" {
			t.Errorf("omitzero field listed as required: %v", required)
		}
	}
	if len(required) != 1 || required[0] != "ip" {
		t.Errorf("required = %v, want [ip]", required)
	}
}
