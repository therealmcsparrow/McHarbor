// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types/filters"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

const automaticImagePruneHour = 3

type automationEnvironment struct {
	ID                           string
	Name                         string
	Timezone                     string
	LastAutomaticImagePruneRunAt string
}

// AutomationService runs background environment automation tasks.
type AutomationService struct {
	db         *sql.DB
	dockerPool *docker.ClientPool
	logger     *slog.Logger
	now        func() time.Time
}

// NewAutomationService creates an environment automation service.
func NewAutomationService(db *sql.DB, dockerPool *docker.ClientPool, logger *slog.Logger) *AutomationService {
	return &AutomationService{
		db:         db,
		dockerPool: dockerPool,
		logger:     logger,
		now:        time.Now,
	}
}

// Start runs the environment automation loop until the context is cancelled.
func (s *AutomationService) Start(ctx context.Context) {
	s.runDueTasks(ctx)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueTasks(ctx)
		}
	}
}

func (s *AutomationService) runDueTasks(ctx context.Context) {
	envs, err := s.listAutomaticImagePruneEnvironments(ctx)
	if err != nil {
		s.logger.Error("environments: failed to list automatic image pruning environments", "error", err)
		return
	}

	now := s.now().UTC()
	for _, env := range envs {
		if !automaticImagePruneDue(env, now) {
			continue
		}
		s.runAutomaticImagePrune(ctx, env, now)
	}
}

func (s *AutomationService) listAutomaticImagePruneEnvironments(ctx context.Context) ([]automationEnvironment, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(queryCtx, `
		SELECT id, name, timezone, last_automatic_image_prune_run_at
		FROM environments
		WHERE orchestrator_type = 'docker'
		  AND is_active = 1
		  AND automatic_image_pruning_enabled = 1
		ORDER BY name ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying automatic image pruning environments: %w", err)
	}
	defer rows.Close()

	envs := make([]automationEnvironment, 0)
	for rows.Next() {
		var env automationEnvironment
		if err := rows.Scan(&env.ID, &env.Name, &env.Timezone, &env.LastAutomaticImagePruneRunAt); err != nil {
			return nil, fmt.Errorf("scanning automation environment: %w", err)
		}
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating automation environments: %w", err)
	}

	return envs, nil
}

func automaticImagePruneDue(env automationEnvironment, now time.Time) bool {
	loc := automationLocation(env.Timezone)
	localNow := now.In(loc)
	if localNow.Hour() < automaticImagePruneHour {
		return false
	}
	if env.LastAutomaticImagePruneRunAt == "" {
		return true
	}

	lastRunAt, err := time.Parse(time.RFC3339, env.LastAutomaticImagePruneRunAt)
	if err != nil {
		return true
	}

	localLastRun := lastRunAt.In(loc)
	return localLastRun.Year() != localNow.Year() || localLastRun.YearDay() != localNow.YearDay()
}

func automationLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}

	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}

	return loc
}

func (s *AutomationService) runAutomaticImagePrune(ctx context.Context, env automationEnvironment, runAt time.Time) {
	defer func() {
		if err := s.recordAutomaticImagePruneRun(ctx, env.ID, runAt); err != nil {
			s.logger.Error("environments: failed to record automatic image prune run", "envID", env.ID, "error", err)
		}
	}()

	cli, err := s.dockerPool.Get(env.ID)
	if err != nil {
		s.logger.Error("environments: automatic image pruning failed to get docker client", "envID", env.ID, "envName", env.Name, "error", err)
		return
	}

	pruneCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	report, err := cli.ImagesPrune(pruneCtx, unusedImagePruneFilters())
	if err != nil {
		s.logger.Error("environments: automatic image pruning failed", "envID", env.ID, "envName", env.Name, "error", err)
		return
	}

	s.logger.Info(
		"environments: automatic image pruning completed",
		"envID", env.ID,
		"envName", env.Name,
		"deletedCount", len(report.ImagesDeleted),
		"spaceReclaimed", report.SpaceReclaimed,
	)
}

func (s *AutomationService) recordAutomaticImagePruneRun(ctx context.Context, envID string, runAt time.Time) error {
	updateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := s.db.ExecContext(
		updateCtx,
		"UPDATE environments SET last_automatic_image_prune_run_at = ? WHERE id = ?",
		runAt.UTC().Format(time.RFC3339),
		envID,
	); err != nil {
		return fmt.Errorf("updating last automatic image prune run: %w", err)
	}

	return nil
}

func unusedImagePruneFilters() filters.Args {
	return filters.NewArgs(filters.Arg("dangling", "false"))
}
