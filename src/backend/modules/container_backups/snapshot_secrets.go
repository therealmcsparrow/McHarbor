// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// ErrSnapshotSecretEncryptionUnavailable refuses to store a snapshot driver's secret
// field (a NAS API key or password) when the service was built without encryption.
var ErrSnapshotSecretEncryptionUnavailable = errors.New("snapshot config secret cannot be stored without encryption")

// snapshotSecretKeys returns the config keys the driver's ConfigSchema declares as
// secret. The bool is false when no driver answers to the name, in which case the
// caller cannot know what to encrypt and must not write a config map.
func (s *Service) snapshotSecretKeys(driver string) (map[string]bool, bool) {
	_, spec, ok := s.resolveSnapshotAdapter(driver)
	if !ok {
		return nil, false
	}
	out := map[string]bool{}
	for _, decl := range spec.ConfigSchema {
		if decl.Secret || strings.EqualFold(decl.Type, "secret") {
			out[decl.Key] = true
		}
	}
	return out, true
}

// sealSnapshotConfig folds a submitted config map onto the stored one, encrypting
// every secret field. A blank or omitted secret keeps the stored value, because the
// API never echoes a secret back and a form that saves without it must not wipe it.
func (s *Service) sealSnapshotConfig(driver string, submitted, existing map[string]string) (map[string]string, error) {
	secrets, ok := s.snapshotSecretKeys(driver)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotDriverUnavailable, driver)
	}
	out := make(map[string]string, len(submitted)+len(secrets))
	for key, value := range submitted {
		if !secrets[key] {
			out[key] = value
			continue
		}
		if strings.TrimSpace(value) == "" || (encryption.IsEncrypted(value) && value == existing[key]) {
			if previous := existing[key]; previous != "" {
				out[key] = previous
			}
			continue
		}
		if s.enc == nil {
			return nil, ErrSnapshotSecretEncryptionUnavailable
		}
		cipher, err := s.enc.Encrypt(value)
		if err != nil {
			return nil, fmt.Errorf("encrypting snapshot config field %s: %w", key, err)
		}
		out[key] = cipher
	}
	for key := range secrets {
		if _, present := submitted[key]; present {
			continue
		}
		if previous := existing[key]; previous != "" {
			out[key] = previous
		}
	}
	return out, nil
}

// publicSnapshotConfig is the shape a snapshot config leaves the API in: secret
// values are dropped and only their key is listed in SecretsSet. A value carrying the
// ciphertext prefix is withheld even when the driver is not loaded to name it.
func (s *Service) publicSnapshotConfig(cfg SnapshotConfig) SnapshotConfig {
	secrets, _ := s.snapshotSecretKeys(cfg.Driver)
	visible := make(map[string]string, len(cfg.Config))
	var set []string
	for key, value := range cfg.Config {
		if secrets[key] || encryption.IsEncrypted(value) {
			if value != "" {
				set = append(set, key)
			}
			continue
		}
		visible[key] = value
	}
	sort.Strings(set)
	cfg.Config = visible
	cfg.SecretsSet = set
	return cfg
}

// driverConfigMap is snapshotConfigToMap with every encrypted value decrypted. It is
// the only place a snapshot secret is turned back into plaintext, and only on the way
// into the driver.
func (s *Service) driverConfigMap(cfg SnapshotConfig) (map[string]string, error) {
	out := snapshotConfigToMap(cfg)
	for key, value := range out {
		if !encryption.IsEncrypted(value) {
			continue
		}
		if s.enc == nil {
			return nil, ErrSnapshotSecretEncryptionUnavailable
		}
		plain, err := s.enc.Decrypt(value)
		if err != nil {
			return nil, fmt.Errorf("decrypting snapshot config %s field %s: %w", cfg.ID, key, err)
		}
		out[key] = plain
	}
	return out, nil
}

// snapshotBackfillRow is one stored snapshot config the backfill inspects.
type snapshotBackfillRow struct {
	cfg SnapshotConfig
	raw string
}

// pendingSnapshotBackfill reads every snapshot config and releases the cursor before
// returning, so the caller can write on a single-connection database.
func (s *Service) pendingSnapshotBackfill(ctx context.Context) ([]snapshotBackfillRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, environment_id, name, driver, plugin_slug, enabled,
		       COALESCE(pool_name, ''), COALESCE(dataset_prefix, ''),
		       COALESCE(ssh_target, ''), use_lxd, config,
		       COALESCE(last_probed_at, ''), COALESCE(last_probe, ''),
		       created_at, updated_at
		FROM container_backup_snapshot_configs
		ORDER BY id LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing snapshot configs for backfill: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []snapshotBackfillRow
	for rows.Next() {
		var raw string
		cfg, err := scanSnapshotConfigWithRaw(rows, &raw)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshotBackfillRow{cfg: cfg, raw: raw})
	}
	return out, rows.Err()
}

// BackfillSnapshotSecretEncryption encrypts every secret field a snapshot config
// stored before secrets were encrypted. It is idempotent: a value that already
// carries the ciphertext prefix is left alone, and a row whose driver is not loaded
// is skipped until a boot that can name its secret fields.
func (s *Service) BackfillSnapshotSecretEncryption(ctx context.Context) (int, error) {
	if s.enc == nil {
		return 0, ErrSnapshotSecretEncryptionUnavailable
	}
	batch, err := s.pendingSnapshotBackfill(ctx)
	if err != nil {
		return 0, err
	}
	encrypted := 0
	// bounded by pendingSnapshotBackfill, which reads at most 1000 rows per run
	for _, row := range batch {
		secrets, ok := s.snapshotSecretKeys(row.cfg.Driver)
		if !ok {
			continue
		}
		changed := false
		next := make(map[string]string, len(row.cfg.Config))
		for key, value := range row.cfg.Config {
			if secrets[key] && value != "" && !encryption.IsEncrypted(value) {
				cipher, err := s.enc.Encrypt(value)
				if err != nil {
					return encrypted, fmt.Errorf("encrypting snapshot config %s field %s: %w", row.cfg.ID, key, err)
				}
				value, changed = cipher, true
			}
			next[key] = value
		}
		if !changed {
			continue
		}
		encoded, err := marshalSnapshotConfigMap(next)
		if err != nil {
			return encrypted, err
		}
		// Conditional on the value read, so a config saved between the read and this
		// write is never overwritten with the older plaintext-derived map.
		result, err := s.db.ExecContext(ctx,
			`UPDATE container_backup_snapshot_configs SET config = ?, updated_at = ? WHERE id = ? AND config = ?`,
			encoded, time.Now().UTC().Format(time.RFC3339), row.cfg.ID, row.raw)
		if err != nil {
			return encrypted, fmt.Errorf("encrypting snapshot config %s: %w", row.cfg.ID, err)
		}
		encrypted += int(db.RowsAffected(result))
	}
	return encrypted, nil
}
