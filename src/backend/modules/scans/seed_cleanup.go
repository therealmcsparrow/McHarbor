// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// migrationSeededScannerIDs are the rows migration 079 inserts on every database to carry the
// pre-plugin built-in scanners forward. They are ids, not engine names.
var migrationSeededScannerIDs = []string{
	"scannercfgtrivy0000000000",
	"scannercfggrype0000000000",
}

// pruneUnusedSeedRows removes migration 079's seeded rows from a database that never ran the
// built-in scanners. That migration also runs on a fresh install, where neither engine is
// installed, and its rows then sit in the scanner list as "Plugin removed" with one of them the
// estate default. A row is kept when its engine is installed, when it was configured, or when
// legacy scanner settings show the install really used it.
func (s *ConfigService) pruneUnusedSeedRows(ctx context.Context) (int, error) {
	// The migration leaves its own scanner_configs_migrated marker in this category on every
	// database; only the built-in scanners' own settings say the install really used them.
	var legacy int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM settings WHERE category = 'scanners' AND key <> 'scanner_configs_migrated'`).Scan(&legacy); err != nil {
		return 0, fmt.Errorf("checking legacy scanner settings: %w", err)
	}
	if legacy > 0 {
		return 0, nil
	}

	removed := 0
	err := db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		// bounded by migrationSeededScannerIDs, the fixed list of rows the migration seeded.
		for _, id := range migrationSeededScannerIDs {
			var scannerType, config string
			err := tx.QueryRowContext(ctx,
				`SELECT scanner_type, config FROM scanner_configs WHERE id = ?`, id).Scan(&scannerType, &config)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return fmt.Errorf("reading seeded scanner %s: %w", id, err)
			}
			if strings.TrimSpace(config) != "{}" {
				continue
			}
			if _, _, installed := s.spec(scannerType); installed {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM scanner_configs WHERE id = ?`, id); err != nil {
				return fmt.Errorf("removing seeded scanner %s: %w", id, err)
			}
			removed++
		}
		if removed == 0 {
			return nil
		}
		return electDefault(ctx, tx, "", false, false)
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
