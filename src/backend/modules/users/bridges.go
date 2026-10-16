// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"database/sql"
)

// Deleting a user destroys their API keys through an ON DELETE CASCADE, so
// the keys have to be archived first — the tombstone is the only record that
// the credentials ever existed. The api_keys module owns that archive, and a
// module must not import another module, so users declares the one call it
// needs and main.go injects the bridge from internal/bootstrap.

// APIKeyArchiver writes tombstones for the keys a user owns, inside the
// caller's transaction so the archive and the delete stand or fall together.
type APIKeyArchiver interface {
	ArchiveOwnedKeysTx(ctx context.Context, tx *sql.Tx, userID string) error
}

// SetAPIKeyArchiver injects the archive collaborator used when deleting a
// user. Without it Delete refuses to run rather than destroying keys that
// were never archived (migration 085's trigger is the backstop, not a
// substitute — it cannot resolve the owner's effective permissions).
func (s *Service) SetAPIKeyArchiver(a APIKeyArchiver) { s.apiKeyArchiver = a }
