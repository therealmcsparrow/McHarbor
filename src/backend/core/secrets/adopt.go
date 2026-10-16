// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package secrets stores, adopts and resolves encrypted secret references that other modules keep instead of plaintext.
package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"
)

// LegacyColumn describes one module-owned encrypted column to bring into the
// vault.
//
// The shape is deliberately generic. Eleven modules each grew their own
// encrypt-into-my-own-column habit, and eleven bespoke backfills would be
// eleven places to get the idempotency wrong. One walker, eleven
// declarations.
type LegacyColumn struct {
	// Table and IDColumn address the owning row.
	Table    string
	IDColumn string
	// ValueColumn holds the existing `enc:v1:` ciphertext.
	ValueColumn string
	// SecretIDColumn is the new column added beside it. Both coexist until a
	// later release drops the legacy one.
	SecretIDColumn string
	// LabelColumn is a human-readable column (usually `name`) used to build a
	// readable secret name. Optional — the row id is used when it is empty.
	LabelColumn string
	// NamePrefix prefixes the generated secret name, e.g. "WEBHOOK".
	NamePrefix string
	// Module is written to `origin_module`, so a secret can always be traced
	// back to the feature that owns it.
	Module string
	// Kind is the vault kind: token, password, cert, private_key, kubeconfig…
	Kind string
	// Where optionally narrows the rows considered, e.g. only agent
	// environments. Appended to the WHERE clause with AND; it must not
	// reference user input.
	Where string
}

// nameSanitiser strips everything a secret name may not contain.
var nameSanitiser = regexp.MustCompile(`[^A-Z0-9_]+`)

// systemSecretName builds a readable, valid name for an adopted credential.
// `WEBHOOK_DEPLOY_NOTIFIER` beats `WEBHOOK_D1F2G3H4`, so the label is used
// when there is one and the id is the fallback and the collision-breaker.
func systemSecretName(prefix, label, id string) string {
	part := nameSanitiser.ReplaceAllString(strings.ToUpper(strings.TrimSpace(label)), "_")
	part = strings.Trim(part, "_")
	if part == "" {
		part = nameSanitiser.ReplaceAllString(strings.ToUpper(id), "_")
	}
	name := prefix + "_" + part
	if len(name) > 64 {
		name = name[:64]
	}
	name = strings.Trim(name, "_")
	if !nameRe.MatchString(name) {
		// Falls back to something that cannot fail validation. A name is a
		// convenience here; correctness is the id in SecretIDColumn.
		name = prefix + "_" + strings.ToUpper(xid.New().String())
		if len(name) > 64 {
			name = name[:64]
		}
	}
	return name
}

// AdoptLegacyColumns runs the backfill for every declared column.
//
// Idempotent by construction: it only looks at rows whose SecretIDColumn is
// still NULL, and the first thing it does with a row is write that column. A
// second boot finds nothing. A boot interrupted half way leaves the rows it
// finished migrated and the rest untouched, which is exactly the state a
// read-through fallback is built to serve.
//
// Failure is reported but never fatal. A module whose credential could not be
// adopted keeps reading its legacy column and keeps working; stopping the
// application because a bookkeeping copy failed would be a strictly worse
// outcome.
func (s *Service) AdoptLegacyColumns(ctx context.Context, specs ...LegacyColumn) error {
	if s == nil || s.db == nil || s.enc == nil {
		return nil
	}

	var failures []string
	for _, spec := range specs {
		adopted, err := s.adoptColumn(ctx, spec)
		if err != nil {
			slog.Error("adopting module credential into the secrets vault failed",
				"module", spec.Module, "table", spec.Table, "column", spec.ValueColumn, "error", err)
			failures = append(failures, spec.Module+"."+spec.ValueColumn)
			continue
		}
		if adopted > 0 {
			slog.Info("module credentials adopted into the secrets vault",
				"module", spec.Module, "column", spec.ValueColumn, "rows", adopted)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("adopting legacy credentials: %s", strings.Join(failures, ", "))
	}
	return nil
}

func (s *Service) adoptColumn(ctx context.Context, spec LegacyColumn) (int, error) {
	where := fmt.Sprintf(
		"%s IS NOT NULL AND %s != '' AND (%s IS NULL OR %s = '')",
		spec.ValueColumn, spec.ValueColumn, spec.SecretIDColumn, spec.SecretIDColumn,
	)
	if spec.Where != "" {
		where += " AND (" + spec.Where + ")"
	}

	label := "''"
	if spec.LabelColumn != "" {
		label = spec.LabelColumn
	}
	query := fmt.Sprintf( // SQL identifier: table and column names come from the compiled-in adoption catalog
		"SELECT %s, %s, %s FROM %s WHERE %s LIMIT 1000",
		spec.IDColumn, label, spec.ValueColumn, spec.Table, where,
	)

	type pending struct{ id, label, ciphertext string }
	batch, err := func() ([]pending, error) {
		rows, err := s.db.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("selecting rows to adopt: %w", err)
		}
		defer closeRows(rows)
		var out []pending
		for rows.Next() {
			var id string
			var rowLabel, ciphertext sql.NullString
			if err := rows.Scan(&id, &rowLabel, &ciphertext); err != nil {
				return nil, fmt.Errorf("scanning row to adopt: %w", err)
			}
			out = append(out, pending{id: id, label: rowLabel.String, ciphertext: ciphertext.String})
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterating rows to adopt: %w", err)
		}
		return out, nil
	}()
	if err != nil {
		return 0, err
	}

	adopted := 0
	// bounded by the adoption batch read for one catalog entry
	for _, row := range batch {
		// The legacy column may already hold plaintext on an install that
		// predates that module's own encryption. Decrypt passes plaintext
		// through unchanged, so this handles both.
		plaintext, err := s.decryptValue(row.ciphertext, row.id)
		if err != nil {
			// One undecryptable row must not stop the rest. It keeps its
			// legacy column and the read-through fallback keeps serving it.
			slog.Warn("skipping a credential that could not be decrypted during adoption",
				"module", spec.Module, "table", spec.Table, "row", row.id, "error", err)
			continue
		}

		secretID, err := s.insertSystemSecret(ctx, spec, row.label, row.id, plaintext)
		if err != nil {
			return adopted, err
		}

		if _, err := s.db.ExecContext(ctx, fmt.Sprintf( // SQL identifier: table and column names come from the compiled-in adoption catalog
			"UPDATE %s SET %s = ? WHERE %s = ?", spec.Table, spec.SecretIDColumn, spec.IDColumn,
		), secretID, row.id); err != nil {
			return adopted, fmt.Errorf("linking adopted secret: %w", err)
		}
		adopted++
	}

	if len(batch) == 1000 {
		more, err := s.adoptColumn(ctx, spec)
		return adopted + more, err
	}
	return adopted, nil
}

// insertSystemSecret writes the vault row directly rather than going through
// Create.
//
// Two reasons. Create refuses a `global` scope without secrets.manage_global,
// and the adoption pass runs at start-up with no user and no authorizer — it
// would fail closed, correctly, on every row. And a system secret needs
// `system = 1` and `origin_module` set, which Create deliberately does not
// expose: those are not things a user gets to claim about their own secret.
func (s *Service) insertSystemSecret(
	ctx context.Context, spec LegacyColumn, label, rowID, plaintext string,
) (string, error) {
	ciphertext, err := s.enc.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("re-encrypting adopted credential: %w", err)
	}

	kind := spec.Kind
	if kind == "" {
		kind = KindToken
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// One retry on a name collision, with the row id appended. Two rows can
	// legitimately share a label — two registries both called "internal" —
	// and the name is cosmetic, so a collision must not fail the adoption.
	for attempt := 0; attempt < 2; attempt++ {
		name := systemSecretName(spec.NamePrefix, label, rowID)
		if attempt == 1 {
			name = systemSecretName(spec.NamePrefix, label+"_"+rowID, rowID)
		}

		id := xid.New().String()
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO secrets (id, name, description, kind, scope, scope_key, value,
				version, generated, system, origin_module, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'global', ?, ?, 1, 0, 1, ?, ?, ?)
		`, id, name,
			fmt.Sprintf("Managed by McHarbor for %s (%s.%s)", spec.Module, spec.Table, spec.ValueColumn),
			kind, scopeKey(name, nil, nil), ciphertext, spec.Module, now, now)
		if err == nil {
			return id, nil
		}
		if !isUniqueViolation(err) {
			return "", fmt.Errorf("inserting adopted secret: %w", err)
		}
	}
	return "", fmt.Errorf("could not find a free name for the adopted %s credential", spec.Module)
}

// SystemValue reads one adopted credential back out of the vault.
//
// This is the module read-through path. It is not the reveal endpoint: there
// is no permission check here, because the caller is the owning module acting
// on its own credential — the same thing it did when the value lived in its
// own column. The access control that matters is on the module's own routes,
// exactly where it was before.
func (s *Service) SystemValue(ctx context.Context, secretID string) (string, error) {
	if secretID == "" {
		return "", ErrNotFound
	}
	var ciphertext string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM secrets WHERE id = ?", secretID).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("loading system secret: %w", err)
	}
	return s.decryptValue(ciphertext, secretID)
}

// ReadThrough is what every adopted module actually calls.
//
// It returns the vault's copy when the row has been adopted and the module's
// own legacy column when it has not. That is what makes a half-migrated
// database work, and it is why the legacy columns are not dropped in the same
// release as the backfill: if the backfill had a bug, this path is the way
// back.
//
// A vault miss falls back rather than failing. A secret row deleted out from
// under a module would otherwise take that module's feature down, and the
// legacy column still holds a perfectly good value.
func (s *Service) ReadThrough(ctx context.Context, secretID sql.NullString, legacyCiphertext string) (string, error) {
	if secretID.Valid && secretID.String != "" {
		value, err := s.SystemValue(ctx, secretID.String)
		if err == nil {
			return value, nil
		}
		slog.Warn("reading an adopted credential from the vault failed; falling back to the legacy column",
			"secret_id", secretID.String, "error", err)
	}
	if legacyCiphertext == "" {
		return "", nil
	}
	return s.decryptValue(legacyCiphertext, "")
}

// SetSystemValue rotates an adopted credential in place, so a module's own
// "change my password" flow keeps working after adoption without each module
// learning the vault's rotation rules.
func (s *Service) SetSystemValue(ctx context.Context, secretID, plaintext string) error {
	if secretID == "" {
		return ErrNotFound
	}
	ciphertext, err := s.enc.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("encrypting system secret: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		"UPDATE secrets SET value = ?, version = version + 1, updated_at = ? WHERE id = ? AND system = 1",
		ciphertext, now, secretID)
	if err != nil {
		return fmt.Errorf("updating system secret: %w", err)
	}
	if coreRowsAffected(result) == 0 {
		return ErrNotFound
	}
	return nil
}

// PruneOrphanedSystemSecrets deletes adopted secrets nothing points at any
// more.
//
// The write rule every adopted module follows is "write the legacy column and
// set `<thing>_secret_id = NULL`". That is deliberately the simplest correct
// thing: a module that rotates its own credential must not leave a stale copy
// in the vault, and clearing the pointer makes the read-through fall straight
// back to the column it just wrote, with no window in which the two disagree.
// The next start-up re-adopts the new value.
//
// What it leaves behind is an orphan, and this is the sweep for those. It
// only ever touches `system = 1` rows whose `origin_module` is one this
// catalogue owns — a user's own secret is never a candidate, whatever its
// reference count.
func (s *Service) PruneOrphanedSystemSecrets(ctx context.Context, specs []LegacyColumn) (int, error) {
	if s == nil || s.db == nil || len(specs) == 0 {
		return 0, nil
	}

	// Build "id NOT IN (every pointer column)" rather than deleting per spec,
	// so a secret referenced by any adopted column survives even if another
	// module's column was cleared.
	// The subqueries carry no LIMIT on purpose: they are the survivor set of a
	// DELETE, so truncating one would remove a secret that is still referenced.
	var subqueries []string
	modules := map[string]bool{}
	for _, spec := range specs {
		// No LIMIT: this is the survivor set of a DELETE, so truncating it
		// would drop a secret that is still referenced.
		subqueries = append(subqueries, fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s IS NOT NULL AND %s != ''",
			spec.SecretIDColumn, spec.Table, spec.SecretIDColumn, spec.SecretIDColumn,
		))
		modules[spec.Module] = true
	}

	placeholders := make([]string, 0, len(modules))
	args := make([]any, 0, len(modules))
	for module := range modules {
		placeholders = append(placeholders, "?")
		args = append(args, module)
	}

	query := fmt.Sprintf( // SQL identifier: the interpolated lists hold only "?" placeholders
		`DELETE FROM secrets
		  WHERE system = 1
		    AND origin_module IN (%s)
		    AND id NOT IN (%s)`,
		strings.Join(placeholders, ", "),
		strings.Join(subqueries, " UNION ALL "),
	)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("pruning orphaned system secrets: %w", err)
	}
	pruned := int(coreRowsAffected(result))
	if pruned > 0 {
		slog.Info("orphaned adopted credentials pruned from the secrets vault", "rows", pruned)
	}
	return pruned, nil
}
