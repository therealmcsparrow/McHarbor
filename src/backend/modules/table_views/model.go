// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package table_views stores each user's table layouts and named saved filters so a
// table looks the same on every browser the user signs in from.
package table_views

import "encoding/json"

// Layout is a user's column order, visibility and sizing for one table; Payload is
// returned verbatim because only the page knows its column ids.
type Layout struct {
	ID        string          `json:"id"`
	TableID   string          `json:"tableId"`
	Payload   json.RawMessage `json:"payload"`
	UpdatedAt string          `json:"updatedAt"`
}

// Filter is one named saved filter; State is opaque to the server because each page
// defines its own filter model.
type Filter struct {
	ID        string          `json:"id"`
	TableID   string          `json:"tableId"`
	Name      string          `json:"name"`
	State     json.RawMessage `json:"state"`
	IsDefault bool            `json:"isDefault"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

// View is everything a table needs on load, fetched in one request so the layout and
// the filter menu never disagree.
type View struct {
	Layout  *Layout  `json:"layout"`
	Filters []Filter `json:"filters"`
}

// LayoutPayload is the validated shape of a layout; V lets a future shape change be
// detected rather than misread.
type LayoutPayload struct {
	V             int            `json:"v"`
	Columns       []LayoutColumn `json:"columns"`
	Density       string         `json:"density"`
	PageSize      int            `json:"pageSize"`
	AutoRefreshMs int            `json:"autoRefreshMs"`
}

// LayoutColumn is one column's placement; array order is display order.
type LayoutColumn struct {
	ID          string `json:"id"`
	Visible     bool   `json:"visible"`
	Size        string `json:"size"`
	HeaderAlign string `json:"headerAlign,omitempty"`
	CellAlign   string `json:"cellAlign,omitempty"`
	// Sortable overrides the column's own sorting default; nil keeps the default.
	Sortable *bool `json:"sortable,omitempty"`
}

// FilterInput is the body of a filter create and each filter in an import.
type FilterInput struct {
	Name      string          `json:"name"`
	State     json.RawMessage `json:"state"`
	IsDefault bool            `json:"isDefault"`
}

// FilterUpdate is a partial filter update; nil fields are left unchanged.
type FilterUpdate struct {
	Name      *string         `json:"name"`
	State     json.RawMessage `json:"state"`
	IsDefault *bool           `json:"isDefault"`
}

// ImportTable carries one table's localStorage state during the one-time import.
type ImportTable struct {
	TableID string          `json:"tableId"`
	Layout  json.RawMessage `json:"layout"`
	Filters []FilterInput   `json:"filters"`
}

// ImportInput is the body of POST /table-views/import.
type ImportInput struct {
	Tables []ImportTable `json:"tables"`
}
