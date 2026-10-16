// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import "errors"

// The reasons an artifact or key file is refused. Every refusal wraps one, so a
// handler answers with a reason key while the full message goes to the log: the
// message chain can carry raw library errors that are not for the operator.
var (
	// ErrNotExport means the file is not a McHarbor instance export at all.
	ErrNotExport = errors.New("not an instance export")
	// ErrUnsupportedFormat means the export uses a format this build cannot read.
	ErrUnsupportedFormat = errors.New("unsupported export format")
	// ErrIncomplete means the export ends before everything its manifest lists.
	ErrIncomplete = errors.New("incomplete export")
	// ErrDamaged means the export or its key parameters were altered or corrupted.
	ErrDamaged = errors.New("damaged export")
	// ErrKeyKind means the export was sealed with the other kind of secret.
	ErrKeyKind = errors.New("wrong kind of key")
	// ErrBadKeyFile means the key file given is not a readable instance key file.
	ErrBadKeyFile = errors.New("unreadable key file")
)
