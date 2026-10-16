// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/xid"
)

// ErrStackNotOwned reports that a stack with the requested name exists but was
// not created by the caller. It is refused rather than overwritten, because a
// generated name colliding with an operator's own stack must never replace it.
var ErrStackNotOwned = errors.New("stacks: a stack with this name exists and is not managed by this caller")

// ownedStackRow is the slice of a stack row the owned-stack seam decides on.
type ownedStackRow struct {
	id            string
	environmentID string
	projectPath   string
	composeFile   string
	managedBy     string
}

// ownedStack reads the provenance of the stack called name, or nil when none exists.
func (s *Service) ownedStack(ctx context.Context, name string) (*ownedStackRow, error) {
	var row ownedStackRow
	var env, composeFile, managedBy sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, environment_id, project_path, compose_file, managed_by FROM stacks WHERE name = ?`, name,
	).Scan(&row.id, &env, &row.projectPath, &composeFile, &managedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stacks: reading stack %s: %w", name, err)
	}
	row.environmentID = env.String
	row.composeFile = composeFile.String
	if row.composeFile == "" {
		row.composeFile = "docker-compose.yml"
	}
	row.managedBy = managedBy.String
	return &row, nil
}

// DeployOwnedStack creates or updates the stack called name in envID from compose
// and runs `up -d`, on behalf of owner. It is the seam the GitOps module reaches
// through the service registry, so every signature is primitives.
//
// A stack of that name owned by anyone else is refused with ErrStackNotOwned; a
// failed `compose up` is returned as an error carrying compose's own scrubbed
// message, which is what the caller records for the operator.
func (s *Service) DeployOwnedStack(ctx context.Context, envID, name, owner, compose string) (string, error) {
	if strings.TrimSpace(owner) == "" {
		return "", errors.New("stacks: an owned stack needs an owner")
	}
	if strings.TrimSpace(compose) == "" {
		return "", errors.New("stacks: the compose file is empty")
	}
	safeName := sanitizeName(name)
	existing, err := s.ownedStack(ctx, safeName)
	if err != nil {
		return "", err
	}
	if existing != nil && existing.managedBy != owner {
		return "", fmt.Errorf("%w: %q", ErrStackNotOwned, safeName)
	}
	checkPath, checkVars := s.getProjectPath(safeName), map[string]string{}
	if existing != nil {
		checkPath = existing.projectPath
		if checkVars, err = s.loadEnvVars(ctx, existing.id); err != nil {
			return "", err
		}
	}
	if err := s.ownedStackRefusal(ctx, envID, safeName, existing, []byte(compose), checkPath, checkVars); err != nil {
		return "", err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var projectPath, composeFile, stackID string
	envVars := map[string]string{}
	if existing == nil {
		projectPath = s.getProjectPath(safeName)
		composeFile = "docker-compose.yml"
		if err := os.MkdirAll(projectPath, 0o755); err != nil {
			return "", fmt.Errorf("stacks: creating the project directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(projectPath, composeFile), []byte(compose), 0o644); err != nil {
			return "", fmt.Errorf("stacks: writing the compose file: %w", err)
		}
		stackID = xid.New().String()
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, managed_by, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, 'unknown', ?, ?, ?)`,
			stackID, safeName, nullIfEmpty(envID), projectPath, composeFile, owner, now, now,
		); err != nil {
			return "", fmt.Errorf("stacks: recording stack %s: %w", safeName, err)
		}
	} else {
		stackID = existing.id
		projectPath = existing.projectPath
		composeFile = existing.composeFile
		if existing.environmentID != envID {
			if res := s.runDockerCompose(ctx, []string{"down", "--remove-orphans"}, projectPath, nil, existing.environmentID); !res.Success {
				return "", fmt.Errorf("stacks: removing %s from its previous environment failed: %s", safeName, res.Error)
			}
		}
		if err := os.MkdirAll(projectPath, 0o755); err != nil {
			return "", fmt.Errorf("stacks: creating the project directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(projectPath, composeFile), []byte(compose), 0o644); err != nil {
			return "", fmt.Errorf("stacks: writing the compose file: %w", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE stacks SET environment_id = ?, updated_at = ? WHERE id = ?`,
			nullIfEmpty(envID), now, stackID,
		); err != nil {
			return "", fmt.Errorf("stacks: updating stack %s: %w", safeName, err)
		}
		stored, err := s.loadEnvVars(ctx, stackID)
		if err != nil {
			return "", err
		}
		envVars = stored
	}

	res := s.runDockerCompose(ctx, []string{"up", "-d", "--remove-orphans"}, projectPath, envVars, envID)
	if !res.Success {
		msg := strings.TrimSpace(res.Error)
		if msg == "" {
			msg = "docker compose up failed"
		}
		return safeName, fmt.Errorf("docker compose up failed: %s", msg)
	}
	return safeName, nil
}

// RemoveOwnedStack brings the stack called name down and deletes it, on behalf of
// owner. A stack that no longer exists is success, so a teardown interrupted by a
// restart can simply be run again; one owned by anyone else is refused.
func (s *Service) RemoveOwnedStack(ctx context.Context, envID, name, owner string, removeVolumes bool) error {
	safeName := sanitizeName(name)
	existing, err := s.ownedStack(ctx, safeName)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	if existing.managedBy != owner {
		return fmt.Errorf("%w: %q", ErrStackNotOwned, safeName)
	}
	target := existing.environmentID
	if target == "" {
		target = envID
	}
	if err := s.ownedStackRefusal(ctx, target, safeName, existing, nil, "", nil); err != nil {
		return err
	}
	args := []string{"down", "--remove-orphans"}
	if removeVolumes {
		args = append(args, "-v")
	}
	if res := s.runDockerCompose(ctx, args, existing.projectPath, nil, target); !res.Success {
		msg := strings.TrimSpace(res.Error)
		if msg == "" {
			msg = "docker compose down failed"
		}
		return fmt.Errorf("docker compose down failed: %s", msg)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM stack_environment_variables WHERE stack_id = ?`, existing.id); err != nil {
		return fmt.Errorf("stacks: deleting env vars of %s: %w", safeName, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM stacks WHERE id = ?`, existing.id); err != nil {
		return fmt.Errorf("stacks: deleting stack %s: %w", safeName, err)
	}
	if err := os.RemoveAll(existing.projectPath); err != nil {
		slog.Warn("stacks: removing an owned stack's project directory failed", "stack", safeName, "error", err)
	}
	return nil
}

// nullIfEmpty stores an empty environment id as NULL, which is how the rest of
// the module represents "the local environment".
func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}
