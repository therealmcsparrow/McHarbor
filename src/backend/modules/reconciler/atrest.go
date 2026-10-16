// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// ErrReconcilerEncryptionUnavailable refuses to store a desired state's config or spec
// without the encryption service: both can pin a container's environment, credentials
// included, and a plaintext copy at rest is worse than a failed save.
var ErrReconcilerEncryptionUnavailable = errors.New("reconciler: desired state cannot be stored without encryption")

// atRestBackfillTimeout bounds the boot-time sealing of legacy rows, so a slow database
// delays startup by at most this long instead of holding the server back indefinitely.
const atRestBackfillTimeout = 30 * time.Second

// seal encrypts a config or spec column value for storage. An empty value stays empty
// so "no config" never needs a decrypt on the way back.
func (s *Service) seal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", ErrReconcilerEncryptionUnavailable
	}
	cipher, err := s.enc.Encrypt(value)
	if err != nil {
		return "", fmt.Errorf("encrypting desired state: %w", err)
	}
	return cipher, nil
}

// open reverses seal. A value without the ciphertext prefix is a row written before
// encryption and is returned as stored until the startup backfill reaches it.
func (s *Service) open(value string) (string, error) {
	if !encryption.IsEncrypted(value) {
		return value, nil
	}
	if s.enc == nil {
		return "", ErrReconcilerEncryptionUnavailable
	}
	plain, err := s.enc.Decrypt(value)
	if err != nil {
		return "", fmt.Errorf("decrypting desired state: %w", err)
	}
	return plain, nil
}

// envPolicy decides which desired values a response withholds, the same classifier
// drift reports and stack env use, so a value is masked identically everywhere.
var envPolicy = envredact.Policy{}

// Redacted returns ds as the API shows it: every credential-looking env value in the
// config and spec value is replaced by envredact.Marker, which an update sends back to
// mean "unchanged".
func Redacted(ds DesiredState) DesiredState {
	ds.Config = maskConfigEnv(ds.Config)
	if len(ds.Spec) > 0 {
		spec := make(map[string]string, len(ds.Spec))
		for k, v := range ds.Spec {
			if envPolicy.Withhold(k, v) {
				v = envredact.Marker
			}
			spec[k] = v
		}
		ds.Spec = spec
	}
	return ds
}

// RedactedList is Redacted over a page of desired states.
func RedactedList(items []DesiredState) []DesiredState {
	out := make([]DesiredState, len(items))
	for i, ds := range items {
		out[i] = Redacted(ds)
	}
	return out
}

// configEnv decodes the env object of a config blob, keeping every other key verbatim.
// ok is false when the blob is not a JSON object, which also means it pins no env.
func configEnv(raw string) (doc map[string]json.RawMessage, env map[string]string, ok bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil, false
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil || doc == nil {
		return nil, nil, false
	}
	if rawEnv, present := doc["env"]; present {
		if err := json.Unmarshal(rawEnv, &env); err != nil {
			return nil, nil, false
		}
	}
	return doc, env, true
}

// withEnv writes env back into doc and re-encodes the blob.
func withEnv(doc map[string]json.RawMessage, env map[string]string) (string, error) {
	encoded, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("encoding desired env: %w", err)
	}
	doc["env"] = encoded
	out, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("encoding desired config: %w", err)
	}
	return string(out), nil
}

// maskConfigEnv withholds credential-looking env values inside a config blob. A blob
// that is not an object, or pins no env, has nothing to withhold and is returned as is.
func maskConfigEnv(raw string) string {
	doc, env, ok := configEnv(raw)
	if !ok || len(env) == 0 {
		return raw
	}
	changed := false
	for k, v := range env {
		if envPolicy.Withhold(k, v) {
			env[k], changed = envredact.Marker, true
		}
	}
	if !changed {
		return raw
	}
	out, err := withEnv(doc, env)
	if err != nil {
		// Unreachable for a map that just decoded, but a blob that cannot be
		// re-encoded must not go out with its credentials.
		return "{}"
	}
	return out
}

// restoreConfigEnv puts the stored value back wherever a submitted config carries the
// mask, so a client that round-trips a masked config never pins the marker as the
// desired value. A masked key with nothing stored behind it is dropped.
func restoreConfigEnv(submitted, stored string) (string, error) {
	doc, env, ok := configEnv(submitted)
	if !ok || len(env) == 0 {
		return submitted, nil
	}
	_, storedEnv, _ := configEnv(stored)
	changed := false
	for k, v := range env {
		if v != envredact.Marker {
			continue
		}
		changed = true
		if previous, has := storedEnv[k]; has {
			env[k] = previous
			continue
		}
		delete(env, k)
	}
	if !changed {
		return submitted, nil
	}
	return withEnv(doc, env)
}

// restoreSpec is restoreConfigEnv for a plugin kind's pinned properties.
func restoreSpec(submitted, stored map[string]string) map[string]string {
	if submitted == nil {
		return nil
	}
	out := make(map[string]string, len(submitted))
	for k, v := range submitted {
		if v != envredact.Marker {
			out[k] = v
			continue
		}
		if previous, has := stored[k]; has {
			out[k] = previous
		}
	}
	return out
}

// desiredBackfillRow is one row whose config or spec is still plaintext.
type desiredBackfillRow struct {
	id, config, spec string
}

// pendingDesiredBackfill reads the rows still holding plaintext and releases the
// cursor before returning, so the caller can write on a single-connection database.
func (s *Service) pendingDesiredBackfill(ctx context.Context) ([]desiredBackfillRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, ''), COALESCE(spec, '')
		FROM desired_states
		WHERE (COALESCE(config, '') <> '' AND config NOT LIKE 'enc:v1:%')
		   OR (COALESCE(spec, '') <> '' AND spec NOT LIKE 'enc:v1:%')
		ORDER BY id LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("selecting plaintext desired states: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("reconciler: closing rows failed", "error", cerr)
		}
	}()
	var out []desiredBackfillRow
	for rows.Next() {
		var row desiredBackfillRow
		if err := rows.Scan(&row.id, &row.config, &row.spec); err != nil {
			return nil, fmt.Errorf("scanning plaintext desired state: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// BackfillEncryption encrypts the config and spec of every desired state written
// before they were encrypted at rest. It is idempotent, and each write is conditional
// on the values read so a row saved in between is never overwritten.
func (s *Service) BackfillEncryption(ctx context.Context) (int, error) {
	if s.enc == nil {
		return 0, ErrReconcilerEncryptionUnavailable
	}
	batch, err := s.pendingDesiredBackfill(ctx)
	if err != nil {
		return 0, err
	}
	encrypted := 0
	// bounded by pendingDesiredBackfill, which reads at most 1000 rows per run
	for _, row := range batch {
		config, spec := row.config, row.spec
		if !encryption.IsEncrypted(config) {
			if config, err = s.seal(config); err != nil {
				return encrypted, err
			}
		}
		if !encryption.IsEncrypted(spec) {
			if spec, err = s.seal(spec); err != nil {
				return encrypted, err
			}
		}
		result, err := s.db.ExecContext(ctx,
			`UPDATE desired_states SET config = ?, spec = ?
			 WHERE id = ? AND COALESCE(config, '') = ? AND COALESCE(spec, '') = ?`,
			config, spec, row.id, row.config, row.spec)
		if err != nil {
			return encrypted, fmt.Errorf("encrypting desired state %s: %w", row.id, err)
		}
		encrypted += int(db.RowsAffected(result))
	}
	// A full page almost certainly has more behind it; the cap stays the only bound.
	if len(batch) == 1000 && encrypted > 0 {
		more, err := s.BackfillEncryption(ctx)
		return encrypted + more, err
	}
	return encrypted, nil
}
