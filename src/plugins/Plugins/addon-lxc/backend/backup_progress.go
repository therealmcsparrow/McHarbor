// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// backupEventTimeout bounds one progress write.
const backupEventTimeout = 10 * time.Second

// SubscribeBackupProgress routes the agent's container events into the run rows.
//
// Registered rather than polled because the copy is the phase that takes the
// time: without it a multi-gigabyte backup shows "archiving" and zero bytes for
// its whole life, which is indistinguishable from one that has wedged — and the
// operator's only way to tell was to watch the destination stay empty, which it
// legitimately does until the archive is finished and the upload starts.
func (s *Service) SubscribeBackupProgress(event guestEvent) {
	if event.GuestKind != guestKindCT || event.Kind != "backup_progress" {
		return
	}
	s.recordBackupProgress(event.Detail)
}

// recordBackupProgress writes what the agent reports about a running copy.
func (s *Service) recordBackupProgress(detail map[string]string) {
	runID := detail["runId"]
	if runID == "" {
		return
	}
	stage := detail["stage"]
	if stage == "" {
		stage = "archiving"
	}

	copied, _ := strconv.ParseInt(detail["bytes"], 10, 64)
	total, _ := strconv.ParseInt(detail["total"], 10, 64)

	// A phase frame carries a sentence instead of a byte count: everything
	// before the archive starts moves no bytes, and "0 B" for two minutes is the
	// symptom this whole path exists to remove.
	message := detail["detail"]
	if message == "" {
		switch {
		case total > 0 && copied > 0:
			message = fmt.Sprintf("%s of %s (%d%%)",
				humanBytes(copied), humanBytes(total), copied*100/total)
		case copied > 0:
			message = humanBytes(copied)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), backupEventTimeout)
	defer cancel()
	now := time.Now().UTC().Format(time.RFC3339)

	// bytes_total is written only by a frame that carries a count. A phase frame
	// has none, and letting it write zero would reset a figure the copy had
	// already moved forward.
	query := `
		UPDATE plugin_addon_lxc_backup_runs
		   SET progress_stage = ?, progress_message = ?, progress_updated_at = ?,
		       bytes_total = ?, updated_at = ?
		 WHERE id = ? AND status = 'running'`
	args := []any{stage, message, now, copied, now, runID}
	if _, carries := detail["bytes"]; !carries {
		query = `
		UPDATE plugin_addon_lxc_backup_runs
		   SET progress_stage = ?, progress_message = ?, progress_updated_at = ?,
		       updated_at = ?
		 WHERE id = ? AND status = 'running'`
		args = []any{stage, message, now, now, runID}
	}
	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		s.logger.Warn("system containers: recording backup progress",
			"run", runID, "error", err)
	}
}

// environmentName asks the host what an environment is called.
//
// Falls back to the id: a backup must not fail because the host could not name
// the environment it belongs to, and an id is still a stable folder.
func (s *Service) environmentName(ctx context.Context, envID string) string {
	if s == nil || s.environments == nil {
		return envID
	}
	if name := s.environments.Name(ctx, envID); name != "" {
		return name
	}
	return envID
}
