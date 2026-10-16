// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
)

// EnsureSchema adds the columns later versions of this plugin introduced.
//
// A plugin's migrations run once, at install, so a numbered file never reaches
// an installation that already has this plugin. SQLite has no ADD COLUMN IF NOT
// EXISTS either, so a second run reports a duplicate column, and that is the
// expected state rather than a failure.
func (s *Service) EnsureSchema(ctx context.Context) error {
	columns := []struct {
		definition string
		what       string
	}{
		{"attach_kind TEXT", "attachment kind"},
		{"parent_link TEXT", "parent interface"},
		{"link_mode TEXT", "link mode"},
		{"static_ipv6 TEXT", "static IPv6 address"},
		{"static_gw6 TEXT", "static IPv6 gateway"},
	}
	for _, wanted := range columns {
		if _, err := s.db.Exec(ctx,
			`ALTER TABLE plugin_addon_lxc_nics ADD COLUMN `+wanted.definition); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("adding the NIC %s column: %w", wanted.what, err)
		}
	}
	return s.ensureBackupSchema(ctx)
}

// ensureBackupSchema creates the backup tables when the migration has not run.
//
// Same reason as the columns above: 0003_ct_backups.sql reaches a fresh install
// and never an instance that already had this plugin, so without this the backup
// routes would answer "no such table" on every existing installation until
// somebody uninstalled and reinstalled. Every statement below is the migration's
// own, IF NOT EXISTS throughout, so running both is harmless.
func (s *Service) ensureBackupSchema(ctx context.Context) error {
	for _, statement := range backupSchemaStatements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("preparing the backup tables: %w", err)
		}
	}
	return nil
}
