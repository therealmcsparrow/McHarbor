// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// MigrationDeadline is how long a conversion may wait for its agent.
//
// Generous on purpose: it has to cover an image pull on a slow link plus the
// agent's own reconnect backoff, which reaches about a minute. Anything still
// unproven after this is not slow, it is stuck.
const MigrationDeadline = 15 * time.Minute

// migrationSweepInterval is how often stalled conversions are looked for.
const migrationSweepInterval = 30 * time.Second

// StartMigrationFailsafe reverts conversions whose agent never authenticated.
//
// The failure it exists for is silent: an agent that cannot reach McHarbor, or
// was deployed onto a host that cannot run it, leaves the environment waiting
// forever with no error anywhere. Reverting restores a working environment
// without anyone having to notice.
// leader may be nil (single node); with it only the elected node reverts, so two
// nodes never race to roll back the same environment.
func (s *Service) StartMigrationFailsafe(ctx context.Context, logger *slog.Logger, leader func(context.Context) bool) {
	go func() {
		ticker := time.NewTicker(migrationSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if leader != nil && !leader(ctx) {
					continue
				}
				s.revertStalledMigrations(ctx, logger)
			}
		}
	}()
}

// revertStalledMigrations reverts every conversion past the deadline.
func (s *Service) revertStalledMigrations(ctx context.Context, logger *slog.Logger) {
	cutoff := time.Now().UTC().Add(-MigrationDeadline).Format(time.RFC3339)

	found, err := s.stalledMigrations(ctx, cutoff)
	if err != nil {
		logger.Warn("environments: scanning for stalled migrations failed", "error", err)
		return
	}
	for _, row := range found {
		// An agent that connected after the cutoff but before this statement
		// would have cleared the marker already; the WHERE guard makes the
		// revert lose that race rather than undo a migration that succeeded.
		if reverted, err := s.revertIfStillStalled(ctx, row.id, cutoff); err != nil {
			logger.Error("environments: reverting stalled migration failed",
				"env", row.id, "error", err)
			continue
		} else if !reverted {
			continue
		}

		logger.Warn("environments: migration reverted; the agent never connected",
			"env", row.id, "name", row.name, "restoredTo", restoredType(row.connType, row.revertTo),
			"deadline", MigrationDeadline.String())

		s.removeDeployedAgent(ctx, row.id, logger)
	}
}

// revertIfStillStalled reverts one environment, and reports whether it did.
func (s *Service) revertIfStillStalled(ctx context.Context, id, cutoff string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `
		UPDATE environments
		SET connection_type = COALESCE(migration_revert_to, connection_type),
		    agent_token = NULL, agent_token_hash = NULL, agent_token_secret_id = NULL,
		    agent_status = NULL, agent_version = NULL, agent_hostname = NULL,
		    agent_os = NULL, agent_arch = NULL, agent_last_seen = NULL,
		    pending_connection_type = NULL, pending_migration_started_at = NULL,
		    migration_revert_to = NULL,
		    updated_at = ?
		WHERE id = ?
		  AND pending_migration_started_at IS NOT NULL
		  AND pending_migration_started_at < ?`, now, id, cutoff)
	if err != nil {
		return false, fmt.Errorf("reverting stalled migration: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return false, nil
	}
	s.dockerPool.Remove(id)
	return true, nil
}

// removeDeployedAgent tears down an agent container that never connected.
//
// Best effort, and only after the revert: the environment is back on its old
// connection by now, so this reaches the host the same way the deploy did.
// Leaving the container would have it retry a revoked token indefinitely.
func (s *Service) removeDeployedAgent(ctx context.Context, envID string, logger *slog.Logger) {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		logger.Warn("environments: cannot reach host to remove the stalled agent",
			"env", envID, "error", err)
		return
	}
	removeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := cli.ContainerRemove(removeCtx, AgentContainerName, client.ContainerRemoveOptions{
		Force: true,
	}); err != nil && !cerrdefs.IsNotFound(err) {
		logger.Warn("environments: removing the stalled agent container failed",
			"env", envID, "error", err)
		return
	}
	logger.Info("environments: removed the agent container left by a reverted migration",
		"env", envID)
}

// restoredType names what an environment went back to, for the log line.
func restoredType(current, revertTo string) string {
	if revertTo != "" {
		return revertTo
	}
	return current
}

type stalledMigration struct {
	id, name, connType, revertTo string
}

// stalledMigrations lists conversions started before cutoff, capped at 100 per sweep so one pass stays short.
func (s *Service) stalledMigrations(ctx context.Context, cutoff string) ([]stalledMigration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, connection_type, COALESCE(migration_revert_to, '')
		  FROM environments
		 WHERE pending_migration_started_at IS NOT NULL
		   AND pending_migration_started_at < ?
		 LIMIT 100`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("querying stalled migrations: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	var found []stalledMigration
	for rows.Next() {
		var row stalledMigration
		if err := rows.Scan(&row.id, &row.name, &row.connType, &row.revertTo); err != nil {
			return nil, fmt.Errorf("reading stalled migration: %w", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stalled migrations: %w", err)
	}
	return found, nil
}
