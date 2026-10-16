// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

type stackStore struct {
	db *sql.DB
}

func newStackStore(db *sql.DB) *stackStore {
	return &stackStore{db: db}
}

func (s *stackStore) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("stack name is required")
	}

	var stackID string
	var projectPath sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_path
		 FROM stacks
		 WHERE name = ?`,
		name,
	).Scan(&stackID, &projectPath)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("stack not found")
	}
	if err != nil {
		return fmt.Errorf("querying stack: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, "DELETE FROM stack_environment_variables WHERE stack_id = ?", stackID); err != nil {
		return fmt.Errorf("deleting stack env vars: %w", err)
	}
	// The App Store installation record used to be cascaded away by the FK on
	// stacks(id); migration 104 dropped that constraint, so removing a stack by
	// hand from the container view has to clear the "Installed" flag itself.
	// Best-effort — the store reconciles leftovers on its next list.
	if _, err := s.db.ExecContext(ctx, "DELETE FROM appstore_installed WHERE stack_id = ?", stackID); err != nil {
		return fmt.Errorf("deleting app store installation record: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM stacks WHERE id = ?", stackID); err != nil {
		return fmt.Errorf("deleting stack: %w", err)
	}
	if projectPath.Valid && projectPath.String != "" {
		if err := os.RemoveAll(projectPath.String); err != nil {
			return fmt.Errorf("removing stack directory: %w", err)
		}
	}

	return nil
}
