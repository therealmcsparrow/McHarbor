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

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/db"
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

	cli, err := s.client(ctx, envID)
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
		infoRes, err := cli.ContainerInspect(inspectCtx, storedID, client.ContainerInspectOptions{})
		info := infoRes.Container
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
	containersRes, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
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

// RelinkContainer points every plan and run in envID that names containerName at
// containerID, so a recreated container keeps its backups without anyone opening the page.
// Runs with no name of their own match through their plan, as in the bulk relink.
func (s *Service) RelinkContainer(ctx context.Context, envID, containerID, containerName string) (int64, error) {
	name := strings.TrimSpace(strings.TrimPrefix(containerName, "/"))
	if strings.TrimSpace(envID) == "" || strings.TrimSpace(containerID) == "" || name == "" {
		return 0, nil
	}
	dbCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel()
	now := time.Now().UTC().Format(time.RFC3339)
	slashed := "/" + name

	plans, err := s.db.ExecContext(dbCtx,
		`UPDATE container_backup_plans SET container_id = ?, updated_at = ?
		 WHERE environment_id = ? AND container_name IN (?, ?) AND container_id <> ?`,
		containerID, now, envID, name, slashed, containerID,
	)
	if err != nil {
		return 0, fmt.Errorf("relinking plans for %s: %w", name, err)
	}
	runs, err := s.db.ExecContext(dbCtx,
		`UPDATE container_backup_runs SET container_id = ?, updated_at = ?
		 WHERE environment_id = ? AND container_id <> ?
		   AND (COALESCE(container_name, '') IN (?, ?)
		        OR (COALESCE(container_name, '') = '' AND plan_id IN (
		            SELECT id FROM container_backup_plans
		            WHERE environment_id = ? AND container_name IN (?, ?))))`,
		containerID, now, envID, containerID, name, slashed, envID, name, slashed,
	)
	if err != nil {
		return db.RowsAffected(plans), fmt.Errorf("relinking runs for %s: %w", name, err)
	}
	return db.RowsAffected(plans) + db.RowsAffected(runs), nil
}

// RelinkAllResult summarizes a bulk container-link reconciliation pass.
type RelinkAllResult struct {
	PlansChecked     int      `json:"plansChecked"`
	PlansRefreshed   int      `json:"plansRefreshed"`
	RunsChecked      int      `json:"runsChecked"`
	RunsRefreshed    int      `json:"runsRefreshed"`
	RefreshedRunIDs  []string `json:"refreshedRunIds,omitempty"`
	RefreshedPlanIDs []string `json:"refreshedPlanIds,omitempty"`
}

// RelinkAllStaleContainerLinks iterates every plan and run with a
// container_id, resolves the live id via the orchestrator, and
// persists the change when the stored id is stale. Runs in
// parallel with bounded concurrency so the pass completes quickly
// even on environments with many containers.
func (s *Service) RelinkAllStaleContainerLinks(ctx context.Context) (*RelinkAllResult, error) {
	result := &RelinkAllResult{}

	plans, err := s.relinkPlanRows(ctx)
	if err != nil {
		return result, err
	}
	runs, err := s.relinkRunRows(ctx)
	if err != nil {
		return result, err
	}

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

		// The run stores its own container_name; fall back to the
		// plan's for rows written before migration 134 added that
		// column. A run with neither has no name to match on.
		containerName := strings.TrimSpace(r.containerName)
		if containerName == "" && r.planID != "" {
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

// backfillRunContainerNames fills the container_name of runs that
// still have none and persists the result.
//
// A run stores its container name from the moment it is created, but
// rows written before that column existed — and rows whose plan was
// deleted, taking the joined name with it — come back empty, and the
// UI then falls back to the bare container id. One container list per
// call is enough to name every such row, and because the fix is
// persisted the work happens at most once per run.
//
// Best-effort by contract: an unreachable orchestrator or a failed
// UPDATE leaves the run exactly as it was and never fails the list.
func (s *Service) backfillRunContainerNames(ctx context.Context, envID string, runs []BackupRun) {
	if strings.TrimSpace(envID) == "" {
		return
	}
	pending := 0
	for i := range runs {
		if strings.TrimSpace(runs[i].ContainerName) == "" && strings.TrimSpace(runs[i].ContainerID) != "" {
			pending++
		}
	}
	if pending == 0 {
		return
	}

	cli, err := s.client(ctx, envID)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("run name backfill: docker client unavailable", "env", envID, "error", err)
		}
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
	defer cancel()
	containersRes, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("run name backfill: list containers failed", "env", envID, "error", err)
		}
		return
	}
	nameByID := make(map[string]string, len(containers))
	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		nameByID[c.ID] = strings.TrimPrefix(c.Names[0], "/")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	// bounded by one page of backup runs; a failed name backfill repeats on the next listing
	for i := range runs {
		if strings.TrimSpace(runs[i].ContainerName) != "" {
			continue
		}
		name := nameByID[runs[i].ContainerID]
		if name == "" {
			continue
		}
		dbCtx, dbCancel := context.WithTimeout(ctx, containerLinkResolveTimeout)
		_, err := s.db.ExecContext(dbCtx,
			"UPDATE container_backup_runs SET container_name = ?, updated_at = ? WHERE id = ?",
			name, now, runs[i].ID,
		)
		dbCancel()
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("run name backfill: persist container_name failed",
					"run", runs[i].ID, "name", name, "error", err)
			}
			continue
		}
		runs[i].ContainerName = name
	}
}

type planRow struct {
	id, envID, containerID, containerName string
}

type runRow struct {
	id, envID, containerID, planID, containerName string
}

// relinkPlanRows reads every plan that names a container in an environment, for the bulk relink pass.
func (s *Service) relinkPlanRows(ctx context.Context) ([]planRow, error) {
	// No LIMIT: the relink pass has to see every plan. A cap would leave the
	// plans past it pointing at a container id that no longer exists.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, environment_id, container_id, container_name
		 FROM container_backup_plans
		 WHERE environment_id <> '' AND container_name <> ''`)
	if err != nil {
		return nil, fmt.Errorf("listing plans for relink: %w", err)
	}
	defer func() { db.CloseRows(rows) }()
	var plans []planRow
	for rows.Next() {
		var p planRow
		if err := rows.Scan(&p.id, &p.envID, &p.containerID, &p.containerName); err != nil {
			return nil, fmt.Errorf("scanning plan row: %w", err)
		}
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading plan rows for relink: %w", err)
	}
	return plans, nil
}

// relinkRunRows reads every run that can be matched to a container by plan or name, for the bulk relink pass.
func (s *Service) relinkRunRows(ctx context.Context) ([]runRow, error) {
	// No LIMIT: same sweep, over runs rather than plans.
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, environment_id, container_id, COALESCE(plan_id, ''), COALESCE(container_name, '')
		 FROM container_backup_runs
		 WHERE environment_id <> ''
		   AND (COALESCE(plan_id, '') <> '' OR COALESCE(container_name, '') <> '')`)
	if err != nil {
		return nil, fmt.Errorf("listing runs for relink: %w", err)
	}
	defer func() { db.CloseRows(rows) }()
	var runs []runRow
	for rows.Next() {
		var r runRow
		if err := rows.Scan(&r.id, &r.envID, &r.containerID, &r.planID, &r.containerName); err != nil {
			return nil, fmt.Errorf("scanning run row: %w", err)
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading run rows for relink: %w", err)
	}
	return runs, nil
}
