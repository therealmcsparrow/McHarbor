// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package stacks manages Compose stacks: their database records, project files, environment secrets and lifecycle over Docker.
package stacks

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// EnvMaskSentinel is what GET /stacks/{name}/env-vars returns in place of a
// value whose key looks like a credential. It is also the token the update
// path treats as "leave this one alone": a client that round-trips a masked
// payload back to the server must not overwrite the real value with the mask.
//
// The string is deliberately one no shell would accept as a value and no
// operator would type — a run of bullets. A collision would mean a user had
// literally set a variable to eight U+2022 characters, in which case the
// value is preserved unchanged, which is the safe direction to fail. It is
// envredact.Marker, so container env and stack env share one marker.
const EnvMaskSentinel = envredact.Marker

// secretKeyHints are matched case-insensitively as substrings of an
// environment variable's key to decide whether its value is masked in the
// ordinary editor. The heuristic drives presentation only — it never grants
// or withholds access, so a false positive costs a click on "reveal" and a
// false negative costs nothing that was not already visible.
var secretKeyHints = []string{
	"PASSWORD",
	"PASSWD",
	"SECRET",
	"TOKEN",
	"APIKEY",
	"API_KEY",
	"_KEY",
	"PRIVATE_KEY",
	"DSN",
	"CONNECTION_STRING",
	"CREDENTIAL",
	"AUTH",
}

// looksSecret reports whether an environment variable key suggests its value
// is a credential.
func looksSecret(key string) bool {
	upper := strings.ToUpper(key)
	for _, hint := range secretKeyHints {
		if strings.Contains(upper, hint) {
			return true
		}
	}
	return false
}

// envVarRow is one row of stack_environment_variables as the service sees it
// after decryption.
type envVarRow struct {
	Key      string
	Value    string
	IsSecret bool
}

// maskEnvVars replaces the value of every secret-looking row with the
// sentinel. Returned as a plain map because that is the wire shape the
// env-vars endpoint has always had and the frontend still expects.
func maskEnvVars(rows []envVarRow) map[string]string {
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.IsSecret {
			out[row.Key] = EnvMaskSentinel
			continue
		}
		out[row.Key] = row.Value
	}
	return out
}

// withholdRootOfTrust masks every value that carries one of McHarbor's own keys, whatever
// its row is called and whether or not the caller revealed: a stack that copied the secret
// under another name must not hand it to anyone.
func withholdRootOfTrust(vars map[string]string, pinned []string) map[string]string {
	policy := envredact.Policy{Reveal: true, Pinned: pinned}
	for key, value := range vars {
		if policy.Withhold(key, value) {
			vars[key] = EnvMaskSentinel
		}
	}
	return vars
}

// mergeMaskedEnvVars folds an incoming payload onto the stored values,
// treating the sentinel as "unchanged". This is the single highest-risk
// behaviour in Phase 1 of the secrets plan: get it wrong and the first save
// after this ships replaces every credential in the stack with a row of
// bullets. Covered directly by TestEnvVarMaskRoundTrip.
//
// A key absent from `incoming` is a deletion, and a key present with a real
// value is an update — only the exact sentinel is special.
func mergeMaskedEnvVars(incoming, stored map[string]string) map[string]string {
	merged := make(map[string]string, len(incoming))
	for key, value := range incoming {
		if value == EnvMaskSentinel {
			if previous, ok := stored[key]; ok {
				merged[key] = previous
				continue
			}
			// The sentinel arrived for a key with nothing behind it —
			// a client bug or a hand-rolled request. Storing the
			// bullets would be worse than storing nothing.
			merged[key] = ""
			continue
		}
		merged[key] = value
	}
	return merged
}

// envVarBackfillRow is one legacy stack env var awaiting encryption.
type envVarBackfillRow struct {
	id       string
	key      string
	value    string
	isSecret bool
}

// pendingEnvVarBackfill reads the unencrypted rows and releases the cursor before returning, so the caller can open a write transaction on a single-connection database.
func (s *Service) pendingEnvVarBackfill(ctx context.Context) ([]envVarBackfillRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, key, value
		FROM stack_environment_variables
		WHERE value_encrypted = 0
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("selecting unencrypted stack env vars: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var batch []envVarBackfillRow
	for rows.Next() {
		var id, key string
		var value sql.NullString
		if err := rows.Scan(&id, &key, &value); err != nil {
			return nil, fmt.Errorf("scanning stack env var: %w", err)
		}
		batch = append(batch, envVarBackfillRow{id: id, key: key, value: value.String, isSecret: looksSecret(key)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stack env vars: %w", err)
	}
	return batch, nil
}

// BackfillEnvVarEncryption re-encrypts every stack environment variable that
// predates migration 112 and stamps the is_secret heuristic on it. It runs at
// start-up, is idempotent, and is a no-op on the second boot because every
// row it touches comes back with value_encrypted = 1.
//
// Failure here is logged and returned but must not be fatal: a single
// undecryptable row should not stop McHarbor from booting, and the rows it
// did convert stay converted.
func (s *Service) BackfillEnvVarEncryption(ctx context.Context) error {
	if s.enc == nil {
		return nil
	}

	batch, err := s.pendingEnvVarBackfill(ctx)
	if err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning env var backfill transaction: %w", err)
	}
	defer func() { closeutil.Rollback(tx) }()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE stack_environment_variables
		SET value = ?, is_secret = ?, value_encrypted = 1, updated_at = ?
		WHERE id = ?
	`)
	if err != nil {
		return fmt.Errorf("preparing env var backfill update: %w", err)
	}
	defer func() { closeutil.Report(stmt.Close(), "stmt") }()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range batch {
		ciphertext, err := s.enc.Encrypt(row.value)
		if err != nil {
			return fmt.Errorf("encrypting stack env var %s: %w", row.id, err)
		}
		secret := 0
		if row.isSecret {
			secret = 1
		}
		if _, err := stmt.ExecContext(ctx, ciphertext, secret, now, row.id); err != nil {
			return fmt.Errorf("updating stack env var %s: %w", row.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing env var backfill: %w", err)
	}

	slog.Info("stack environment variables encrypted at rest", "rows", len(batch))

	// A boot that found a full page almost certainly has more waiting.
	// Recurse rather than loop so the 1000-row cap stays the only bound.
	if len(batch) == 1000 {
		return s.BackfillEnvVarEncryption(ctx)
	}
	return nil
}

// TightenEnvFilePermissions chmods every existing stack .env to 0600.
//
// writeEnvFile handles this for any file it touches, but a stack that is not
// redeployed is never touched, so without this sweep an install that has been
// running since before the secrets work keeps world-readable credentials on
// disk indefinitely. Runs once at start-up alongside the encryption backfill
// and is cheap: one stat and at most one chmod per managed stack.
//
// Best-effort per file — a stack directory that has been removed by hand, or
// a .env owned by another user, must not stop the sweep or the boot.
func (s *Service) TightenEnvFilePermissions() {
	stacksDir := stacksRoot(s.dataDir)
	entries, err := os.ReadDir(stacksDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("listing stack directories for .env permission sweep failed", "error", err)
		}
		return
	}

	tightened := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(stacksDir, entry.Name(), ".env")
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode().Perm() == 0o600 {
			continue
		}
		if err := os.Chmod(path, 0o600); err != nil {
			slog.Warn("tightening stack .env permissions failed", "stack", entry.Name(), "error", err)
			continue
		}
		tightened++
	}
	if tightened > 0 {
		slog.Info("stack .env files tightened to 0600", "files", tightened)
	}
}
