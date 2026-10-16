// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	"encoding/json"
	"testing"
)

// TestConversionTableShipsItsDefaultLayout guards the Virtual Machines > Conversion default:
// a user who never saved a layout gets the agreed columns instead of the component's own.
func TestConversionTableShipsItsDefaultLayout(t *testing.T) {
	layout := defaultLayout("kvm-imports")
	if layout == nil {
		t.Fatal("kvm-imports has no shipped default layout")
	}
	var payload struct {
		Columns []struct {
			ID string `json:"id"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(layout.Payload, &payload); err != nil {
		t.Fatalf("the shipped layout is not valid JSON: %v", err)
	}
	want := []string{"fileName", "sourceKind", "sizeBytes", "state", "createdAt", "actions"}
	if len(payload.Columns) != len(want) {
		t.Fatalf("want %d columns, got %d", len(want), len(payload.Columns))
	}
	for i, id := range want {
		if payload.Columns[i].ID != id {
			t.Fatalf("column %d: want %q, got %q", i, id, payload.Columns[i].ID)
		}
	}
}
