// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// The run "item key" vocabulary is the one Service.Options mints:
// "config", "image", "filesystem", "logs" and "mount:<destination>".
// restoreFromRun reports restored entries with the same keys, so a
// backup run and the restore run derived from it are directly
// comparable.
const (
	runItemConfig     = "config"
	runItemImage      = "image"
	runItemFilesystem = "filesystem"
	runItemLogs       = "logs"
	runItemMountPfx   = "mount:"
)

// planIncludedItems flattens a plan's include flags and selected mounts
// into the item-key vocabulary. Order is stable (config, image,
// filesystem, logs, then mounts in plan order) so the persisted JSON
// does not churn between runs of the same plan.
func planIncludedItems(plan *BackupPlan) []string {
	if plan == nil {
		return nil
	}
	items := []string{}
	if plan.IncludeConfig {
		items = append(items, runItemConfig)
	}
	if plan.IncludeImage {
		items = append(items, runItemImage)
	}
	if plan.IncludeFilesystem {
		items = append(items, runItemFilesystem)
	}
	if plan.IncludeLogs {
		items = append(items, runItemLogs)
	}
	for _, mount := range plan.SelectedMounts {
		mount = strings.TrimSpace(mount)
		if mount == "" {
			continue
		}
		items = append(items, runItemMountPfx+mount)
	}
	return items
}

// runItemsForPlan resolves the item keys a run of this plan will capture
// and the keys the container had on offer at that moment.
//
// The available set comes from the same Options call the backup dialog
// uses, so "full" means exactly "everything the dialog would have let
// you tick". Inspecting the container can fail (daemon down, container
// gone, agent offline) — that is not worth failing the backup over, so
// on error the available set is left empty and the run simply carries no
// full/partial claim.
func (s *Service) runItemsForPlan(ctx context.Context, plan *BackupPlan) (included, available []string) {
	included = planIncludedItems(plan)
	if plan == nil {
		return included, nil
	}
	options, err := s.Options(ctx, plan.EnvironmentID, plan.ContainerID)
	if err != nil || options == nil {
		if err != nil && s.logger != nil {
			s.logger.Warn("container backup run item inventory failed",
				"env", plan.EnvironmentID, "container", plan.ContainerID, "error", err)
		}
		return included, nil
	}
	available = make([]string, 0, len(options.Options))
	for _, option := range options.Options {
		available = append(available, option.Key)
	}
	return included, available
}

// runScope classifies a run as "full" or "partial" by comparing what it
// captured against what was on offer. An empty available set means the
// inventory was never taken (an old row, or an inspect that failed), and
// the caller must not claim either — hence the empty string.
func runScope(included, available []string) string {
	if len(available) == 0 || len(included) == 0 {
		return ""
	}
	have := make(map[string]struct{}, len(included))
	for _, item := range included {
		have[item] = struct{}{}
	}
	for _, item := range available {
		if _, ok := have[item]; !ok {
			return "partial"
		}
	}
	return "full"
}

// encodeRunItems renders an item list for the TEXT column. An empty list
// stores the empty string rather than "[]" so "nothing recorded" and
// "recorded as empty" stay distinguishable on read.
func encodeRunItems(items []string) string {
	if len(items) == 0 {
		return ""
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// decodeRunItems parses a persisted item column. Malformed JSON is
// reported so the caller can fail the read loudly rather than silently
// showing a run as having captured nothing.
func decodeRunItems(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}, nil
	}
	items := []string{}
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return nil, fmt.Errorf("decoding backup run items: %w", err)
	}
	return items, nil
}

// restoreRunItems works out what a restore run will touch. The archive's
// contents are the source run's included items, and an empty selection
// means "restore everything in the archive" — the same contract
// restorePlan applies when filtering tar entries.
func restoreRunItems(sourceRun *BackupRun, requested []string) (included, available []string) {
	if sourceRun != nil {
		available = append(available, sourceRun.IncludedItems...)
	}
	cleaned := []string{}
	for _, item := range requested {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cleaned = append(cleaned, item)
	}
	if len(cleaned) == 0 {
		return append([]string{}, available...), available
	}
	// Keep the archive's own ordering so the restore run's item list
	// reads the same way the source backup's does.
	if len(available) > 0 {
		wanted := make(map[string]struct{}, len(cleaned))
		for _, item := range cleaned {
			wanted[item] = struct{}{}
		}
		ordered := []string{}
		for _, item := range available {
			if _, ok := wanted[item]; ok {
				ordered = append(ordered, item)
				delete(wanted, item)
			}
		}
		// Anything the caller asked for that the archive did not
		// advertise still belongs in the record.
		extra := make([]string, 0, len(wanted))
		for item := range wanted {
			extra = append(extra, item)
		}
		sort.Strings(extra)
		return append(ordered, extra...), available
	}
	return cleaned, available
}

// setRunItems overwrites the captured item list on a run, and the
// available list too when one is supplied. Used after a restore finishes
// so the row records what was actually written into the container rather
// than what was requested, and by the uploaded-archive path, which only
// learns an archive's contents by unpacking it.
func (s *Service) setRunItems(ctx context.Context, runID string, included, available []string) error {
	if strings.TrimSpace(runID) == "" || len(included) == 0 {
		return nil
	}
	query := `UPDATE container_backup_runs SET included_items = ? WHERE id = ?`
	args := []any{encodeRunItems(included), runID}
	if len(available) > 0 {
		query = `UPDATE container_backup_runs SET included_items = ?, available_items = ? WHERE id = ?`
		args = []any{encodeRunItems(included), encodeRunItems(available), runID}
	}
	var result sql.Result
	err := db.RetryOnBusy(ctx, 5, func() error {
		var execErr error
		result, execErr = s.db.ExecContext(ctx, query, args...)
		return execErr
	})
	if err != nil {
		return fmt.Errorf("updating backup run items: %w", err)
	}
	if db.RowsAffected(result) == 0 && s.logger != nil {
		s.logger.Warn("container backup run items update matched no row", "run", runID)
	}
	return nil
}
