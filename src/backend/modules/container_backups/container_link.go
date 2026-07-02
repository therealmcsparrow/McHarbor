// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
)

// Container-link reconciliation: plans and runs persist a
// `container_id` snapshot at creation time. Docker reassigns ids
// every time a container is recreated (`docker compose up`,
// `docker run`, agent restart), so the stored id can go stale.
// When that happens the link from the runs / plans table to the
// container detail page 404s, even though the rest of the row
// (container_name, schedule, archive) is still correct.
//
// The helpers in this file auto-resolve the live container id by
// name and persist the result. They are designed to be safe to
// call on read paths: failures fall back to the stored id and
// never error out the list call.

const (
	containerLinkResolveTimeout = 5 * time.Second
)

// resolveLiveContainerID returns the live container id for the
// given environment + stored id + container name, or the stored
// id if the live one can't be resolved. The boolean reports
// whether the id was refreshed (i.e. the stored value was stale
// and a new id was found).
//
// Strategy:
//  1. If the stored id is non-empty, inspect by id. If Docker
//     returns a container whose name matches, the stored id is
//     still valid — return it unchanged.
//  2. Otherwise (404, error, or name mismatch) list containers
//     in the env and find one whose name matches. Use the first
//     match.
//  3. If no live container can be found (env offline, name gone,
//     etc.) fall back to the stored id without marking it stale.
func (s *Service) resolveLiveContainerID(
	ctx context.Context,
	envID, storedID, containerName string,
) (string, bool) {
	if strings.TrimSpace(envID) == "" || strings.TrimSpace(containerName) == "" {
		return storedID, false
	}

	cli, err := s.client(envID)
	if err != nil {
		// Orchestrator unreachable — keep the stored id and don't
		// surface a stale badge. The user can retry later when the
		// env is back online.
		if s.logger != nil {
			s.logger.Warn("container link resolve: docker client unavailable",
				"env", envID, "error", err)
		}
		return storedID, false
	}

	targetName := strings.TrimSpace(strings.TrimPrefix(containerName, "/"))
	if targetName == "" {
		return storedID, false
	}

	// Step 1: try the stored id directly.
	if strings.TrimSpace(storedID) != "" {
		inspectCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
		info, err := cli.ContainerInspect(inspectCtx, storedID)
		cancel()
		if err == nil {
			liveName := strings.TrimPrefix(info.Name, "/")
			if liveName == targetName {
				// Stored id is still valid.
				return storedID, false
			}
			// Stored id resolves to a different container. Fall
			// through to the name-based lookup below.
		} else if !errors.Is(err, context.Canceled) {
			// Network / not-found — also fall through to the
			// name-based lookup, which catches the case where
			// the container was recreated.
			if s.logger != nil {
				s.logger.Debug(
					"container link resolve: inspect by stored id failed; falling back to name lookup",
					"env", envID,
					"storedId", storedID,
					"name", targetName,
					"error", err,
				)
			}
		}
	}

	// Step 2: name-based lookup.
	listCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel()
	containers, err := cli.ContainerList(listCtx, container.ListOptions{All: true})
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("container link resolve: list containers failed",
				"env", envID, "name", targetName, "error", err)
		}
		return storedID, false
	}
	for _, c := range containers {
		for _, rawName := range c.Names {
			liveName := strings.TrimPrefix(rawName, "/")
			if liveName == targetName {
				return c.ID, c.ID != storedID
			}
		}
	}
	// Step 3: nothing matched — keep the stored id silently.
	return storedID, false
}

// refreshPlanContainerID resolves the live id for a plan row and
// persists the change in-place. Returns the (possibly updated)
// plan and a `stale` flag the caller can surface in the API.
func (s *Service) refreshPlanContainerID(
	ctx context.Context,
	plan BackupPlan,
) (BackupPlan, bool) {
	if plan.EnvironmentID == "" || plan.ContainerName == "" {
		return plan, false
	}
	resolveCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel()
	liveID, refreshed := s.resolveLiveContainerID(
		resolveCtx,
		plan.EnvironmentID,
		plan.ContainerID,
		plan.ContainerName,
	)
	if !refreshed || liveID == plan.ContainerID {
		return plan, refreshed
	}
	// Persist the refreshed id so subsequent reads don't need to
	// re-resolve. Use a short context so a stuck DB doesn't slow
	// the API.
	dbCtx, dbCancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer dbCancel()
	if _, err := s.db.ExecContext(dbCtx,
		"UPDATE container_backup_plans SET container_id = ?, updated_at = ? WHERE id = ?",
		liveID, time.Now().UTC().Format(time.RFC3339), plan.ID,
	); err != nil {
		if s.logger != nil {
			s.logger.Warn(
				"container link refresh: persist plan container_id failed",
				"plan", plan.ID, "newId", liveID, "error", err,
			)
		}
		// Don't surface the stale flag — we couldn't persist the fix.
		return plan, false
	}
	if s.logger != nil {
		s.logger.Info(
			"container link refresh: plan container_id re-linked",
			"plan", plan.ID,
			"oldId", plan.ContainerID,
			"newId", liveID,
			"name", plan.ContainerName,
		)
	}
	plan.ContainerID = liveID
	return plan, true
}

// refreshRunContainerID resolves the live id for a single run row.
// Used by runByID and by the bulk endpoint. The bulk endpoint also
// fixes the parent plan so a future read sees the fresh id without
// needing the run-side resolution.
func (s *Service) refreshRunContainerID(
	ctx context.Context,
	run *BackupRun,
) (bool, error) {
	if run.EnvironmentID == "" || strings.TrimSpace(run.ContainerName) == "" {
		return false, nil
	}
	resolveCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel()
	liveID, refreshed := s.resolveLiveContainerID(
		resolveCtx,
		run.EnvironmentID,
		run.ContainerID,
		run.ContainerName,
	)
	if !refreshed || liveID == run.ContainerID {
		return false, nil
	}
	dbCtx, dbCancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer dbCancel()
	if _, err := s.db.ExecContext(dbCtx,
		"UPDATE container_backup_runs SET container_id = ?, updated_at = ? WHERE id = ?",
		liveID, time.Now().UTC().Format(time.RFC3339), run.ID,
	); err != nil {
		return true, fmt.Errorf("persist run container_id: %w", err)
	}
	run.ContainerID = liveID
	return true, nil
}

// refreshPlanContainerIDForRun updates the run row's container_id
// without touching the run. Container plan rows share the same
// container_id via plan_id, so when a run is re-linked we update
// the plan row's id too.
func (s *Service) refreshPlanContainerIDForRun(
	ctx context.Context,
	run *BackupRun,
) (bool, error) {
	if run.PlanID == "" {
		return false, nil
	}
	var storedID, containerName string
	dbCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel()
	if err := s.db.QueryRowContext(dbCtx,
		"SELECT container_id, container_name FROM container_backup_plans WHERE id = ?",
		run.PlanID,
	).Scan(&storedID, &containerName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if containerName == "" {
		return false, nil
	}
	resolveCtx, cancel2 := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel2()
	liveID, refreshed := s.resolveLiveContainerID(
		resolveCtx, run.EnvironmentID, storedID, containerName,
	)
	if !refreshed || liveID == storedID {
		return false, nil
	}
	if _, err := s.db.ExecContext(dbCtx,
		"UPDATE container_backup_plans SET container_id = ?, updated_at = ? WHERE id = ?",
		liveID, time.Now().UTC().Format(time.RFC3339), run.PlanID,
	); err != nil {
		return true, fmt.Errorf("persist plan container_id: %w", err)
	}
	run.ContainerID = liveID
	return true, nil
}

// RelinkAllResult summarizes a bulk container-link reconciliation pass.
type RelinkAllResult struct {
	PlansChecked    int      `json:"plansChecked"`
	PlansRefreshed  int      `json:"plansRefreshed"`
	RunsChecked     int      `json:"runsChecked"`
	RunsRefreshed   int      `json:"runsRefreshed"`
	RefreshedRunIDs []string `json:"refreshedRunIds,omitempty"`
	RefreshedPlanIDs []string `json:"refreshedPlanIds,omitempty"`
}

// RelinkAllStaleContainerLinks iterates every plan and run with a
// container_id, resolves the live id via the orchestrator, and
// persists the change when the stored id is stale. Runs in
// parallel with bounded concurrency so the pass completes quickly
// even on environments with many containers.
func (s *Service) RelinkAllStaleContainerLinks(ctx context.Context) (*RelinkAllResult, error) {
	result := &RelinkAllResult{}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, environment_id, container_id, container_name
		 FROM container_backup_plans
		 WHERE environment_id <> '' AND container_name <> ''`)
	if err != nil {
		return result, fmt.Errorf("listing plans for relink: %w", err)
	}
	type planRow struct {
		id, envID, containerID, containerName string
	}
	var plans []planRow
	for rows.Next() {
		var p planRow
		if err := rows.Scan(&p.id, &p.envID, &p.containerID, &p.containerName); err != nil {
			rows.Close()
			return result, fmt.Errorf("scanning plan row: %w", err)
		}
		plans = append(plans, p)
	}
	rows.Close()

	runRows, err := s.db.QueryContext(ctx,
		`SELECT id, environment_id, container_id, plan_id
		 FROM container_backup_runs
		 WHERE environment_id <> '' AND plan_id <> ''`)
	if err != nil {
		return result, fmt.Errorf("listing runs for relink: %w", err)
	}
	type runRow struct {
		id, envID, containerID, planID string
	}
	var runs []runRow
	for runRows.Next() {
		var r runRow
		if err := runRows.Scan(&r.id, &r.envID, &r.containerID, &r.planID); err != nil {
			runRows.Close()
			return result, fmt.Errorf("scanning run row: %w", err)
		}
		runs = append(runs, r)
	}
	runRows.Close()

	result.PlansChecked = len(plans)
	result.RunsChecked = len(runs)

	// Bound concurrency so a single slow env doesn't pin the
	// goroutine pool.
	const concurrency = 4
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	resolvePlan := func(p planRow) {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()

		resolveCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
		defer cancel()
		liveID, refreshed := s.resolveLiveContainerID(
			resolveCtx, p.envID, p.containerID, p.containerName,
		)
		if !refreshed || liveID == p.containerID {
			return
		}
		dbCtx, dbCancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
		defer dbCancel()
		if _, err := s.db.ExecContext(dbCtx,
			"UPDATE container_backup_plans SET container_id = ?, updated_at = ? WHERE id = ?",
			liveID, time.Now().UTC().Format(time.RFC3339), p.id,
		); err != nil {
			if s.logger != nil {
				s.logger.Warn("bulk relink: persist plan failed",
					"plan", p.id, "error", err)
			}
			return
		}
		mu.Lock()
		result.PlansRefreshed++
		result.RefreshedPlanIDs = append(result.RefreshedPlanIDs, p.id)
		mu.Unlock()
		if s.logger != nil {
			s.logger.Info(
				"bulk relink: plan container_id re-linked",
				"plan", p.id, "oldId", p.containerID, "newId", liveID,
			)
		}
	}

	resolveRun := func(r runRow) {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()

		resolveCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
		defer cancel()

		// Look up the run's container_name via its plan (the run
		// row doesn't store the name directly). For ad-hoc runs
		// without a plan we skip — they have no name to match on.
		var containerName string
		if r.planID != "" {
			dbCtx, dbCancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
			if err := s.db.QueryRowContext(dbCtx,
				"SELECT container_name FROM container_backup_plans WHERE id = ?",
				r.planID,
			).Scan(&containerName); err != nil {
				// Plan was deleted — fall through with empty name
				// and skip the resolve.
				containerName = ""
			}
			dbCancel()
		}
		if containerName == "" {
			return
		}

		liveID, refreshed := s.resolveLiveContainerID(
			resolveCtx, r.envID, r.containerID, containerName,
		)
		if !refreshed || liveID == r.containerID {
			return
		}
		dbCtx, dbCancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
		defer dbCancel()
		if _, err := s.db.ExecContext(dbCtx,
			"UPDATE container_backup_runs SET container_id = ?, updated_at = ? WHERE id = ?",
			liveID, time.Now().UTC().Format(time.RFC3339), r.id,
		); err != nil {
			if s.logger != nil {
				s.logger.Warn("bulk relink: persist run failed",
					"run", r.id, "error", err)
			}
			return
		}
		mu.Lock()
		result.RunsRefreshed++
		result.RefreshedRunIDs = append(result.RefreshedRunIDs, r.id)
		mu.Unlock()
		if s.logger != nil {
			s.logger.Info(
				"bulk relink: run container_id re-linked",
				"run", r.id, "oldId", r.containerID, "newId", liveID,
			)
		}
	}

	for _, p := range plans {
		wg.Add(1)
		go resolvePlan(p)
	}
	for _, r := range runs {
		wg.Add(1)
		go resolveRun(r)
	}
	wg.Wait()

	return result, nil
}