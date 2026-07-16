// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"errors"
)

// Runtime adapters in this file break the storage_locations →
// container_backups dependency direction. The concrete adapter
// lives in `internal/bootstrap/adapters.go` and is injected from
// `main.go`.

// BackupMigrator migrates completed backup runs into the local
// storage location. The handler calls this when a non-local
// storage location is deleted so users do not lose history.
type BackupMigrator interface {
	MigrateCompletedRunsToLocalStorage(ctx context.Context, locationID string) (any, error)
}

// ErrBackupMigrationStorageNotLocal is returned when the requested
// backup migration source is not a local storage location.
var ErrBackupMigrationStorageNotLocal = errors.New("backup migration source storage is not local")

// ErrBackupMigrationStorageDisabled is returned when the local
// backup destination is disabled and the migration cannot proceed.
var ErrBackupMigrationStorageDisabled = errors.New("backup migration destination storage is disabled")