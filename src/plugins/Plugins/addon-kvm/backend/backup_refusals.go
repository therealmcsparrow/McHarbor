// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"
)

// Backup refusals. Each is a sentinel the service wraps with
// fmt.Errorf("%w: <sentence>"), so the client gets its own code and the
// sentence the service wrote, and never the text of an error beneath it.
var (
	// ErrBackupNoDestination means a plan or run has nowhere to write.
	ErrBackupNoDestination = errors.New("this backup has no destination")
	// ErrBackupTooManyDestinations means a plan names more destinations than one run can fan out to.
	ErrBackupTooManyDestinations = errors.New("too many backup destinations")
	// ErrBackupDestinationGone means a plan points at a storage location that was deleted.
	ErrBackupDestinationGone = errors.New("a backup destination no longer exists")
	// ErrBackupStorageUnavailable means the host has no storage destinations to resolve against.
	ErrBackupStorageUnavailable = errors.New("backup storage is unavailable")
	// ErrBackupNoDisks means the guest has no disk a backup covers.
	ErrBackupNoDisks = errors.New("this machine has no disks to back up")
	// ErrBackupKeepRange means a retention or full-copy cadence is out of bounds.
	ErrBackupKeepRange = errors.New("the backup retention is out of range")
	// ErrBackupPlanNeedsSchedule means an enabled plan was left without a cron.
	ErrBackupPlanNeedsSchedule = errors.New("this backup plan has no schedule")
	// ErrBackupNotRestorable means the chosen run is not a finished backup.
	ErrBackupNotRestorable = errors.New("this backup cannot be restored")
)

const (
	codeBackupTooManyDestinations = "err.vm.backup_too_many_destinations"
	codeBackupDestinationGone     = "err.vm.backup_destination_gone"
	codeBackupStorageUnavailable  = "err.vm.backup_storage_unavailable"
	codeBackupNoDisks             = "err.vm.backup_no_disks"
	codeBackupKeepRange           = "err.vm.backup_keep_range"
	codeBackupPlanNeedsSchedule   = "err.vm.backup_plan_needs_schedule"
	codeBackupNotRestorable       = "err.vm.backup_not_restorable"
)

// backupRefusals pairs each sentinel with the code its endpoint answers with.
var backupRefusals = []struct {
	sentinel error
	code     string
}{
	{ErrBackupNoDestination, codeNoDestination},
	{ErrBackupTooManyDestinations, codeBackupTooManyDestinations},
	{ErrBackupDestinationGone, codeBackupDestinationGone},
	{ErrBackupStorageUnavailable, codeBackupStorageUnavailable},
	{ErrBackupNoDisks, codeBackupNoDisks},
	{ErrBackupKeepRange, codeBackupKeepRange},
	{ErrBackupPlanNeedsSchedule, codeBackupPlanNeedsSchedule},
	{ErrBackupNotRestorable, codeBackupNotRestorable},
}

// backupRefusal resolves err to its code and the service's own sentence.
//
// Checked before validationCode in the backup handlers: a sentence such as "at
// least one destination" would otherwise match the text markers there and be
// flattened into the generic form error.
func backupRefusal(err error) (code, detail string, matched bool) {
	for _, refusal := range backupRefusals {
		if errors.Is(err, refusal.sentinel) {
			return refusal.code, sentinelDetail(err, refusal.sentinel), true
		}
	}
	return "", "", false
}

// respondBackupRefusal answers a backup refusal with its code and detail, and
// reports whether it did.
func (h *Handler) respondBackupRefusal(w http.ResponseWriter, r *http.Request, err error) bool {
	code, detail, matched := backupRefusal(err)
	if !matched {
		return false
	}
	h.logger.Warn("kvm: refused a backup request", "code", code, "reason", err)
	failDetailErr(w, r, http.StatusBadRequest, code, detail, err)
	return true
}
