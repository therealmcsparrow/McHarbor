// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import "strings"

// Search modes accepted by SearchClause. They mirror the frontend's
// `SearchMode` union minus `regex`: no portable regex predicate exists
// across SQLite, Postgres and MySQL, so regex searches stay client-side.
const (
	SearchModeContains = "contains"
	SearchModeExact    = "exact"
)

// ValidSearchMode reports whether mode is one this package can push into
// SQL. The empty string means "unset" and is treated as `contains`.
func ValidSearchMode(mode string) bool {
	return mode == "" || mode == SearchModeContains || mode == SearchModeExact
}

// SearchClause builds a case-insensitive OR predicate over cols for a
// free-text query, matching the frontend matcher: `contains` is a
// substring test, `exact` compares the whole trimmed value. Both sides
// are lowered so the result does not depend on the driver's LIKE
// collation — SQLite folds ASCII case, Postgres does not.
//
// Returns an empty clause (and no args) when there is nothing to match,
// so callers can append unconditionally. The caller passes column
// expressions, never user input, and every value is bound.
func SearchClause(search, mode string, cols []string) (string, []any) {
	search = strings.TrimSpace(search)
	if search == "" || len(cols) == 0 {
		return "", nil
	}

	value := strings.ToLower(search)
	op := "LIKE"
	if mode == SearchModeExact {
		op = "="
	} else {
		value = "%" + value + "%"
	}

	parts := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	for _, col := range cols {
		parts = append(parts, "LOWER("+col+") "+op+" ?")
		args = append(args, value)
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}
