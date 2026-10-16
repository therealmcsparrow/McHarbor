// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// recordBackupProgress writes what the agent reports about a running copy.
//
// Without it a multi-gigabyte backup shows "copying" and zero bytes for its
// whole life, which is indistinguishable from one that has wedged — and the
// operator's only way to tell was to watch the destination stay empty, which
// it legitimately does until the copy finishes and the upload starts.
func (s *Service) recordBackupProgress(detail map[string]string) {
	runID := detail["runId"]
	if runID == "" {
		return
	}
	stage := detail["stage"]
	if stage == "" {
		stage = "copying"
	}

	copied, _ := strconv.ParseInt(detail["bytes"], 10, 64)
	total, _ := strconv.ParseInt(detail["total"], 10, 64)

	// A phase event carries a sentence instead of a byte count: everything
	// before the copy starts moves no bytes, and "0 B" for two minutes is the
	// symptom this whole path exists to remove.
	message := detail["detail"]
	if message == "" {
		if total > 0 {
			message = fmt.Sprintf("%s of %s (%d%%)",
				humanBytes(copied), humanBytes(total), copied*100/total)
		} else if copied > 0 {
			message = humanBytes(copied)
		}
	}
	if disk := detail["diskId"]; disk != "" && message != "" {
		message = disk + ": " + message
	}

	ctx, cancel := context.WithTimeout(context.Background(), eventApplyTimeout)
	defer cancel()
	now := time.Now().UTC().Format(time.RFC3339)
	// bytes_total tracks the copy so the run's size is honest while it runs;
	// the upload overwrites it with what actually left the host.
	// bytes_total is only written by a copy report. A phase event carries no
	// byte count, and letting it write zero would reset a figure the copy had
	// already moved forward.
	query := `
		UPDATE plugin_addon_kvm_backup_runs
		   SET progress_stage = ?, progress_message = ?, progress_updated_at = ?,
		       bytes_total = ?, updated_at = ?
		 WHERE id = ? AND status = 'running'`
	args := []any{stage, message, now, copied, now, runID}
	if _, ok := detail["bytes"]; !ok {
		query = `
		UPDATE plugin_addon_kvm_backup_runs
		   SET progress_stage = ?, progress_message = ?, progress_updated_at = ?,
		       updated_at = ?
		 WHERE id = ? AND status = 'running'`
		args = []any{stage, message, now, now, runID}
	}
	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		s.logger.Warn("kvm: recording backup progress", "run", runID, "error", err)
	}
}

// environmentName asks the host what an environment is called.
//
// Falls back to the id: a backup must not fail because the host could not
// name the environment it belongs to, and an id is still a stable folder.
func (s *Service) environmentName(ctx context.Context, envID string) string {
	if s == nil || s.environments == nil {
		return envID
	}
	if name := s.environments.Name(ctx, envID); name != "" {
		return name
	}
	return envID
}
