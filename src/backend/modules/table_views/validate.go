// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxPayloadBytes bounds a stored payload so one row cannot bloat the database.
const MaxPayloadBytes = 65536

// MaxFiltersPerTable caps saved filters per user per table.
const MaxFiltersPerTable = 50

// MaxImportTables caps a single import request.
const MaxImportTables = 200

var (
	// ErrNotFound means the row does not exist or belongs to another user; the two are
	// deliberately indistinguishable.
	ErrNotFound = errors.New("table view not found")
	// ErrInvalid means a table id, name or payload failed validation.
	ErrInvalid = errors.New("table view invalid")
	// ErrLimit means the per-table filter cap was reached.
	ErrLimit = errors.New("table view limit reached")
	// ErrNameTaken means the user already has a filter with this name on this table.
	ErrNameTaken = errors.New("table view name taken")
)

var tableIDPattern = regexp.MustCompile(`^[a-z0-9_:.-]{1,128}$`)

var validSizes = map[string]bool{"auto": true, "xs": true, "sm": true, "md": true, "lg": true, "xl": true, "2xl": true, "3xl": true}

var validDensities = map[string]bool{"compact": true, "default": true, "comfortable": true}

var validAligns = map[string]bool{"": true, "auto": true, "left": true, "center": true, "right": true}

// ValidTableID reports whether id is a well-formed table identifier.
func ValidTableID(id string) bool {
	return tableIDPattern.MatchString(id)
}

func normalizeName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > 80 {
		return "", ErrInvalid
	}
	return trimmed, nil
}

func validateLayout(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > MaxPayloadBytes {
		return nil, ErrInvalid
	}
	var p LayoutPayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, ErrInvalid
	}
	if p.V != 1 || len(p.Columns) > 200 || !validDensities[p.Density] || p.PageSize < 1 || p.PageSize > 1000 || p.AutoRefreshMs < 0 {
		return nil, ErrInvalid
	}
	seen := make(map[string]bool, len(p.Columns))
	for _, c := range p.Columns {
		if c.ID == "" || len(c.ID) > 128 || seen[c.ID] || !validSizes[c.Size] || !validAligns[c.HeaderAlign] || !validAligns[c.CellAlign] {
			return nil, ErrInvalid
		}
		seen[c.ID] = true
	}
	return json.Marshal(p)
}

func validateState(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > MaxPayloadBytes {
		return nil, ErrInvalid
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, ErrInvalid
	}
	if obj == nil {
		return nil, ErrInvalid
	}
	return json.Marshal(obj)
}
