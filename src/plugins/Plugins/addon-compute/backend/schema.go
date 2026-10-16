// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
)

// addedColumn is one column a later version of this plugin introduced.
type addedColumn struct {
	table  string
	column string
	// definition is the SQL after the column name. It must carry a default or
	// be nullable, because the table already has rows.
	definition string
}

// lateColumns are the columns added after this plugin's first release.
//
// They exist here as well as in the numbered migrations because a plugin's
// migrations run only when it is installed: an instance that already has the
// plugin never sees a new file, so a column added that way would be missing on
// exactly the installs that have data in them.
var lateColumns = []addedColumn{
	{"plugin_addon_compute_networks", "link_mode", "TEXT"},
	{"plugin_addon_compute_networks", "host_access", "INTEGER NOT NULL DEFAULT 0"},
	{"plugin_addon_compute_networks", "host_access_ip", "TEXT"},
	{"plugin_addon_compute_networks", "ipv6_enabled", "INTEGER NOT NULL DEFAULT 0"},
	{"plugin_addon_compute_networks", "host_access_ipv6", "TEXT"},
	{"plugin_addon_compute_networks", "host_link_name", "TEXT"},
	{"plugin_addon_compute_image_cache", "phase", "TEXT"},
}

// EnsureColumns adds any column this build expects and the database does not
// have, and is safe to call on every boot.
func (s *Service) EnsureColumns(ctx context.Context) error {
	for _, wanted := range lateColumns {
		has, err := s.hasColumn(ctx, wanted.table, wanted.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		// The table and column names come from the slice above, never from a
		// request, which is why they can be interpolated at all.
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			wanted.table, wanted.column, wanted.definition)
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("adding %s.%s: %w", wanted.table, wanted.column, err)
		}
	}
	return nil
}

// hasColumn reports whether a table already carries a column.
func (s *Service) hasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.Query(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("reading %s columns: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			dflt       any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &dflt, &primaryKey); err != nil {
			return false, fmt.Errorf("reading %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
