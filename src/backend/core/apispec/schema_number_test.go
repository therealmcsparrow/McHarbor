// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import "testing"

type numberHolder struct {
	Ratio float64 `json:"ratio"`
	Small float32 `json:"small"`
}

// TestFloatFormatsSurviveCodegen pins the formats that make generated clients
// keep float64 precision; a bare "number" becomes float32 in oapi-codegen.
func TestFloatFormatsSurviveCodegen(t *testing.T) {
	b := newSchemaBuilder()
	ref := b.schemaFor(numberHolder{})
	key := ref["$ref"].(string)[len("#/components/schemas/"):]
	props := b.components[key].(map[string]any)["properties"].(map[string]any)
	if got := props["ratio"].(map[string]any)["format"]; got != "double" {
		t.Errorf("float64 format = %v, want double", got)
	}
	if got := props["small"].(map[string]any)["format"]; got != "float" {
		t.Errorf("float32 format = %v, want float", got)
	}
}
