// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	_ "embed"
	"encoding/json"
)

//go:embed default_layouts.json
var defaultLayoutsJSON []byte

var defaultLayouts = loadDefaultLayouts()

// loadDefaultLayouts parses the shipped layouts once and drops any entry that fails
// validateLayout, so a bad edit to the JSON degrades to the code defaults instead of
// serving a payload the frontend would reject.
func loadDefaultLayouts() map[string]json.RawMessage {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(defaultLayoutsJSON, &raw); err != nil {
		return map[string]json.RawMessage{}
	}
	out := make(map[string]json.RawMessage, len(raw))
	for tableID, payload := range raw {
		if !ValidTableID(tableID) {
			continue
		}
		clean, err := validateLayout(payload)
		if err != nil {
			continue
		}
		out[tableID] = clean
	}
	return out
}

// defaultLayout returns the shipped layout for tableID, used until a user saves their
// own; its empty ID tells the page no row exists yet.
func defaultLayout(tableID string) *Layout {
	payload, ok := defaultLayouts[tableID]
	if !ok {
		return nil
	}
	return &Layout{ID: "", TableID: tableID, Payload: payload}
}
