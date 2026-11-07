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

type nullableHolder struct {
	Flag  *bool `json:"flag" openapi:"nullable"`
	Maybe *bool `json:"maybe,omitempty" openapi:"nullable"`
	Plain *bool `json:"plain"`
}

// TestNullableFieldIsRequiredAndNullable pins the tag that lets a body say
// "key required, null allowed", which a bare pointer cannot express.
func TestNullableFieldIsRequiredAndNullable(t *testing.T) {
	b := newSchemaBuilder()
	ref := b.schemaFor(nullableHolder{})
	key := ref["$ref"].(string)[len("#/components/schemas/"):]
	schema := b.components[key].(map[string]any)
	props := schema["properties"].(map[string]any)
	if got := props["flag"].(map[string]any)["nullable"]; got != true {
		t.Errorf("flag nullable = %v, want true", got)
	}
	if _, ok := props["plain"].(map[string]any)["nullable"]; ok {
		t.Error("an untagged pointer must not become nullable")
	}
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "flag" {
		t.Errorf("required = %v, want [flag]", required)
	}
}
