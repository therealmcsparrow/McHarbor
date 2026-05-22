// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"database/sql"
	"fmt"
	"os"
)

type stackStore struct {
	db *sql.DB
}

func newStackStore(db *sql.DB) *stackStore {
	return &stackStore{db: db}
}

func (s *stackStore) DeleteByName(name string) error {
	if name == "" {
		return fmt.Errorf("stack name is required")
	}

	var stackID string
	var projectPath sql.NullString
	err := s.db.QueryRow(
		`SELECT id, project_path
		 FROM stacks
		 WHERE name = ?`,
		name,
	).Scan(&stackID, &projectPath)
	if err == sql.ErrNoRows {
		return fmt.Errorf("stack not found")
	}
	if err != nil {
		return fmt.Errorf("querying stack: %w", err)
	}

	if _, err := s.db.Exec("DELETE FROM stack_environment_variables WHERE stack_id = ?", stackID); err != nil {
		return fmt.Errorf("deleting stack env vars: %w", err)
	}
	if _, err := s.db.Exec("DELETE FROM stacks WHERE id = ?", stackID); err != nil {
		return fmt.Errorf("deleting stack: %w", err)
	}
	if projectPath.Valid && projectPath.String != "" {
		if err := os.RemoveAll(projectPath.String); err != nil {
			return fmt.Errorf("removing stack directory: %w", err)
		}
	}

	return nil
}
