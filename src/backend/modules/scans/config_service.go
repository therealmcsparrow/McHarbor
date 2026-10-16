// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	plugin "github.com/therealmcsparrow/mcharbor/core/plugin"
	hostplugin "github.com/therealmcsparrow/mcharbor/core/plugin/host"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// maxScannerConfigRows bounds the unpaginated list the Scanners tab
// reads. A thousand configured scanners is already absurd; the cap
// exists so a corrupted table cannot turn a settings page into an OOM.
const maxScannerConfigRows = 1000

// ErrScannerAdapterUnknown is returned when no installed plugin has
// registered the requested scanner type. Callers map it to 404 rather
// than 500: an operator acting on a row whose plugin they uninstalled
// is a missing resource, not a server fault.
var ErrScannerAdapterUnknown = errors.New("scans: unknown scanner type")

// ErrScannerNotConfigured is returned when no `scanner_configs` row
// exists for the requested id.
var ErrScannerNotConfigured = errors.New("scans: scanner is not configured")

// ErrScannerNameRequired is returned when a scanner is saved with a
// blank name. The name is the only thing separating two instances of
// the same engine in the list, so it is not optional.
var ErrScannerNameRequired = errors.New("scans: scanner name is required")

// ErrScannerFieldRequired is returned when a save would leave a field
// the adapter declared Required empty.
//
// This is the difference between a scanner that does not work and a
// scanner that looks fine in the list: a Clair row with no server URL
// renders as a configured engine and scans nothing. Refusing the save
// keeps the list honest — everything in it has somewhere to scan from.
var ErrScannerFieldRequired = errors.New("scans: required scanner field is empty")

// ErrScannerUnavailable is returned when a scan is asked of an engine
// that resolved fine but cannot currently run — the CLI is missing
// from the container, or the server URL is blank.
var ErrScannerUnavailable = errors.New("scans: scanner is not available")

// ErrNoScannerConfigured is returned when a scan names no scanner and
// there is no enabled default to fall back on. Distinct from
// ErrScannerAdapterUnknown on purpose: "you asked for something that
// does not exist" and "you asked for anything and there is nothing"
// send an operator to different places.
var ErrNoScannerConfigured = errors.New("scans: no scanner is configured")

// ConfigService owns the host half of vulnerability scanning: the
// `scanner_configs` rows, secret encryption, and dispatch into
// whatever `scanner-` plugins are installed.
//
// It deliberately knows nothing about Trivy's JSON, Grype's exit codes
// or Clair's two-phase index/report dance. Those live in the plugins;
// this type's entire job is to turn a config row into a decrypted map
// and hand it to an adapter.
//
// A row is one *instance*: a named engine with its own settings.
// Several rows may share a scanner_type — a strict Trivy and a lenient
// one, a production Clair and a staging one — and each carries its own
// config and its own enable state.
type ConfigService struct {
	db      *sql.DB
	enc     *encryption.Service
	runtime any
}

// NewConfigService builds the service. runtime is the plugin runtime
// held as `any` (the same shape AppDeps.PluginAdapters carries) and
// may be nil — a host with no plugin runtime simply reports no
// adapters, which is the correct empty state, not an error.
func NewConfigService(database *sql.DB, enc *encryption.Service, runtime any) *ConfigService {
	return &ConfigService{db: database, enc: enc, runtime: runtime}
}

// SetPluginRuntime injects the loaded plugin runtime after construction.
func (s *ConfigService) SetPluginRuntime(runtime any) { s.runtime = runtime }

// source type-asserts the runtime to the image scanner source. Returns
// (nil, false) when no runtime is wired or it predates the scanner
// family — both mean "no adapters", never an error.
func (s *ConfigService) source() (hostplugin.ImageScannerAdapterSource, bool) {
	if s.runtime == nil {
		return nil, false
	}
	src, ok := s.runtime.(hostplugin.ImageScannerAdapterSource)
	return src, ok
}

// spec resolves an adapter by type.
func (s *ConfigService) spec(scannerType string) (plugin.ImageScannerAdapterSpec, plugin.ImageScannerAdapterClient, bool) {
	src, ok := s.source()
	if !ok {
		return plugin.ImageScannerAdapterSpec{}, nil, false
	}
	client, spec, ok := src.ImageScannerAdapter(scannerType)
	if !ok {
		return plugin.ImageScannerAdapterSpec{}, nil, false
	}
	return spec, client, true
}

// Adapters returns one entry per registered adapter type, in
// registration order so the picker tiles do not reshuffle between
// renders.
//
// Availability is probed here rather than left to the frontend: the
// tile has to be able to say "Trivy is installed but the binary is
// missing", and only the adapter knows that.
func (s *ConfigService) Adapters(ctx context.Context) []ScannerAdapterInfo {
	src, ok := s.source()
	if !ok {
		return []ScannerAdapterInfo{}
	}
	specs := src.ImageScannerAdapters()
	out := make([]ScannerAdapterInfo, 0, len(specs))
	for _, spec := range specs {
		info := ScannerAdapterInfo{
			ScannerType:          spec.ScannerType,
			Slug:                 spec.Slug,
			DisplayName:          spec.DisplayName,
			Icon:                 spec.Icon,
			Description:          spec.Description,
			RequiresLocalBinary:  spec.Capabilities.RequiresLocalBinary,
			SupportsTest:         spec.Capabilities.SupportsTest,
			EncryptedCredentials: spec.Capabilities.EncryptedCredentials,
			Fields:               buildScannerFields(spec, scannerRow{config: map[string]string{}}),
		}
		if _, client, ok := s.spec(spec.ScannerType); ok && client != nil {
			// Defaults only — an adapter tile is asked "could you run
			// at all", not "could you run with this operator's
			// config", which has not been written yet.
			info.Available = client.Available(ctx, defaultConfig(spec))
		}
		out = append(out, info)
	}
	return out
}

// Configs returns every configured instance, oldest first, with its
// stored config merged in.
func (s *ConfigService) Configs(ctx context.Context) ([]ScannerConfigInfo, error) {
	rows, err := s.readRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ScannerConfigInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.buildInfo(ctx, row))
	}
	return out, nil
}

// Config returns one configured instance by id. Used by handlers that
// need the row's name for an audit entry before acting on it.
func (s *ConfigService) Config(ctx context.Context, id string) (ScannerConfigInfo, error) {
	row, err := s.readRow(ctx, id)
	if err != nil {
		return ScannerConfigInfo{}, err
	}
	return s.buildInfo(ctx, row), nil
}

// buildInfo merges a row with its adapter spec. Secret values never
// leave this function — only the boolean that one exists.
func (s *ConfigService) buildInfo(ctx context.Context, row scannerRow) ScannerConfigInfo {
	info := ScannerConfigInfo{
		ID:          row.id,
		Name:        row.name,
		ScannerType: row.scannerType,
		DisplayName: row.name,
		Enabled:     row.enabled,
		IsDefault:   row.isDefault,
		Fields:      []ScannerField{},
	}
	spec, client, ok := s.spec(row.scannerType)
	if !ok {
		// The plugin was uninstalled with the scanner still
		// configured. The row stays in the list, greyed and
		// deletable, rather than vanishing and leaving an operator
		// wondering where their Clair config went.
		return info
	}
	info.Installed = true
	info.Slug = spec.Slug
	info.DisplayName = spec.DisplayName
	info.Icon = spec.Icon
	info.Description = spec.Description
	info.RequiresLocalBinary = spec.Capabilities.RequiresLocalBinary
	info.SupportsTest = spec.Capabilities.SupportsTest
	info.EncryptedCredentials = spec.Capabilities.EncryptedCredentials
	info.Fields = buildScannerFields(spec, row)

	if cfg, err := s.resolvedConfig(spec, row); err == nil && client != nil {
		info.Available = client.Available(ctx, cfg)
	}
	return info
}

// buildScannerFields renders one adapter's ConfigSchema against a
// stored row. Shared by the adapter list (empty row — schema and
// defaults only) and the instance list (stored values attached).
func buildScannerFields(spec plugin.ImageScannerAdapterSpec, row scannerRow) []ScannerField {
	fields := make([]ScannerField, 0, len(spec.ConfigSchema))
	for _, decl := range spec.ConfigSchema {
		f := ScannerField{
			Key:         decl.Key,
			Type:        decl.Type,
			Label:       decl.Label,
			Required:    decl.Required,
			Help:        decl.Help,
			Placeholder: decl.Placeholder,
			Default:     decl.Default,
		}
		stored, present := row.config[decl.Key]
		if decl.Type == "secret" {
			f.HasValue = present && stored != ""
		} else if present {
			f.Value = stored
		}
		fields = append(fields, f)
	}
	return fields
}

// defaultConfig materialises an adapter's declared defaults, for the
// availability probe on a type nobody has configured yet.
func defaultConfig(spec plugin.ImageScannerAdapterSpec) map[string]string {
	out := make(map[string]string, len(spec.ConfigSchema))
	for _, decl := range spec.ConfigSchema {
		if decl.Default == nil {
			continue
		}
		out[decl.Key] = fmt.Sprintf("%v", decl.Default)
	}
	return out
}

// scannerRow is the decoded `scanner_configs` row. config holds raw
// stored values: plaintext for ordinary fields, ciphertext for
// secrets.
type scannerRow struct {
	id          string
	name        string
	scannerType string
	enabled     bool
	isDefault   bool
	config      map[string]string
}

const scannerSelect = `
	SELECT id, name, scanner_type, enabled, is_default, config
	FROM scanner_configs
`

// scanScannerRow decodes one row's columns, including its JSON config
// blob.
func scanScannerRow(scan func(dest ...any) error) (scannerRow, error) {
	row := scannerRow{config: map[string]string{}}
	var enabled, isDefault int
	var configJSON string
	if err := scan(&row.id, &row.name, &row.scannerType, &enabled, &isDefault, &configJSON); err != nil {
		return row, err
	}
	row.enabled = enabled == 1
	row.isDefault = isDefault == 1
	if strings.TrimSpace(configJSON) != "" {
		if err := json.Unmarshal([]byte(configJSON), &row.config); err != nil {
			return row, fmt.Errorf("decoding scanner config %s: %w", row.id, err)
		}
	}
	return row, nil
}

// readRow loads one instance by id.
func (s *ConfigService) readRow(ctx context.Context, id string) (scannerRow, error) {
	row, err := scanScannerRow(s.db.QueryRowContext(ctx, scannerSelect+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrScannerNotConfigured
	}
	if err != nil {
		return row, fmt.Errorf("reading scanner config %s: %w", id, err)
	}
	return row, nil
}

// readRows loads every instance, oldest first so the list order is
// "the order they were added" rather than something that shifts when a
// scanner is renamed.
func (s *ConfigService) readRows(ctx context.Context) ([]scannerRow, error) {
	rows, err := s.db.QueryContext(ctx,
		scannerSelect+` ORDER BY created_at ASC, id ASC LIMIT ?`, maxScannerConfigRows)
	if err != nil {
		return nil, fmt.Errorf("listing scanner configs: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var out []scannerRow
	for rows.Next() {
		row, err := scanScannerRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning scanner config: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// mergeConfig folds a form submission onto what is already stored.
//
// Secret handling: an empty submitted secret means "keep what is
// stored", never "clear it". Clearing a secret is done by deleting the
// scanner, not by blanking a field the UI never shows the value of —
// otherwise every save from a form that cannot echo the secret would
// silently wipe it.
func (s *ConfigService) mergeConfig(spec plugin.ImageScannerAdapterSpec, submitted, existing map[string]string) (string, error) {
	merged := make(map[string]string, len(spec.ConfigSchema))
	for _, decl := range spec.ConfigSchema {
		value, present := submitted[decl.Key]
		value = strings.TrimSpace(value)
		if decl.Type == "secret" {
			if !present || value == "" {
				merged[decl.Key] = existing[decl.Key]
				continue
			}
			cipher, err := s.enc.Encrypt(value)
			if err != nil {
				return "", fmt.Errorf("encrypting scanner field %s: %w", decl.Key, err)
			}
			merged[decl.Key] = cipher
			continue
		}
		if !present {
			merged[decl.Key] = existing[decl.Key]
			continue
		}
		merged[decl.Key] = value
	}

	// Enforce the adapter's own Required flags. The frontend blocks
	// this too, but the check belongs here: a Clair row with no server
	// URL reads as a configured engine to every other code path — the
	// list, the scan-on-install hook, the operator — while scanning
	// nothing.
	for _, decl := range spec.ConfigSchema {
		if decl.Required && strings.TrimSpace(merged[decl.Key]) == "" {
			return "", fmt.Errorf("%w: %s", ErrScannerFieldRequired, decl.Key)
		}
	}

	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("encoding scanner config: %w", err)
	}
	return string(encoded), nil
}

// Create adds one scanner instance and returns its id.
func (s *ConfigService) Create(ctx context.Context, in SaveScannerConfigInput) (string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return "", ErrScannerNameRequired
	}
	spec, _, ok := s.spec(strings.TrimSpace(in.ScannerType))
	if !ok {
		return "", ErrScannerAdapterUnknown
	}
	encoded, err := s.mergeConfig(spec, in.Config, map[string]string{})
	if err != nil {
		return "", err
	}

	id := xid.New().String()
	err = db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO scanner_configs (id, name, scanner_type, enabled, is_default, config)
			VALUES (?, ?, ?, ?, 0, ?)
		`, id, name, spec.ScannerType, boolInt(in.Enabled), encoded); err != nil {
			return fmt.Errorf("inserting scanner config: %w", err)
		}
		return electDefault(ctx, tx, id, in.MakeDefault, in.Enabled)
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Update rewrites one scanner instance. The type is fixed at creation;
// everything else is editable.
//
// Rebinding a row to a different engine is deliberately impossible.
// The stored config keys belong to one adapter's ConfigSchema, so
// reinterpreting them under another engine's schema would silently
// feed a Clair server URL to a CLI flag parser.
func (s *ConfigService) Update(ctx context.Context, id string, in SaveScannerConfigInput) error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ErrScannerNameRequired
	}
	existing, err := s.readRow(ctx, id)
	if err != nil {
		return err
	}
	spec, _, ok := s.spec(existing.scannerType)
	if !ok {
		return ErrScannerAdapterUnknown
	}
	encoded, err := s.mergeConfig(spec, in.Config, existing.config)
	if err != nil {
		return err
	}

	return db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE scanner_configs
			SET name = ?, enabled = ?, config = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, name, boolInt(in.Enabled), encoded, id)
		if err != nil {
			return fmt.Errorf("updating scanner config %s: %w", id, err)
		}
		// The row was read a moment ago, so zero rows here means it
		// was deleted concurrently — report it as missing rather than
		// a silent no-op that leaves the operator thinking they saved.
		if db.RowsAffected(result) == 0 {
			return ErrScannerNotConfigured
		}
		return electDefault(ctx, tx, id, in.MakeDefault, in.Enabled)
	})
}

// Delete removes one scanner instance.
//
// Only the config row goes. Scans this engine already ran stay in
// `scans` with their findings intact: an operator removing a scanner
// must not silently erase the record of what it found, which is
// exactly the evidence a compliance review asks for.
func (s *ConfigService) Delete(ctx context.Context, id string) error {
	return db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM scanner_configs WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("deleting scanner config %s: %w", id, err)
		}
		if db.RowsAffected(result) == 0 {
			return ErrScannerNotConfigured
		}
		// Deleting the default leaves the estate with no scanner to
		// fall back on, so promote the next enabled row rather than
		// letting the next unqualified scan fail.
		return electDefault(ctx, tx, "", false, false)
	})
}

// electDefault keeps exactly one enabled row flagged as the default.
//
// Called after every write. The rules, in order:
//
//   - An explicit MakeDefault on an enabled row wins.
//   - A row that was the default and is now disabled loses the flag.
//   - If nothing holds the flag afterwards, the oldest enabled row
//     takes it.
//
// The last rule is what stops "disable the default" from quietly
// breaking every unqualified scan. It is done here rather than with a
// partial unique index because an estate with every scanner disabled
// is a legitimate state, and a constraint could not express "one, or
// none if there are none to have".
func electDefault(ctx context.Context, tx *sql.Tx, id string, makeDefault, enabled bool) error {
	if makeDefault && enabled && id != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE scanner_configs SET is_default = 0`); err != nil {
			return fmt.Errorf("clearing default scanner: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE scanner_configs SET is_default = 1 WHERE id = ?`, id); err != nil {
			return fmt.Errorf("setting default scanner %s: %w", id, err)
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE scanner_configs SET is_default = 0 WHERE enabled = 0`); err != nil {
		return fmt.Errorf("clearing default on disabled scanners: %w", err)
	}

	var defaults int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scanner_configs WHERE is_default = 1`).Scan(&defaults); err != nil {
		return fmt.Errorf("counting default scanners: %w", err)
	}
	if defaults > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE scanner_configs SET is_default = 1
		WHERE id = (
			SELECT id FROM scanner_configs
			WHERE enabled = 1
			ORDER BY created_at ASC, id ASC
			LIMIT 1
		)
	`); err != nil {
		return fmt.Errorf("electing default scanner: %w", err)
	}
	return nil
}

// Testing lives in test.go — see ConfigService.TestScanner, which
// returns a per-step report rather than a bare error so the Scanners
// tab can show an operator *which* phase failed.

// Resolve turns a scan request's `scanner` field into a runnable
// engine.
//
// The field is deliberately permissive, because three callers fill it
// three different ways and none of them should have to care about the
// others:
//
//   - a `scanner_configs` id — what the UI sends, and the only form
//     that can name one Clair out of two;
//   - a scanner type ("trivy") — what workflow nodes and the App
//     Store's scan-on-install hook have always sent, and what every
//     `scans.scanner` row already written contains;
//   - empty — "use whatever is configured", which resolves to the
//     default row.
//
// Type lookups take the oldest enabled row of that type. That keeps
// every pre-existing caller working unchanged after the upgrade, which
// is the whole point of accepting the form.
func (s *ConfigService) Resolve(ctx context.Context, ref string) (boundScanner, error) {
	ref = strings.TrimSpace(ref)

	if ref == "" {
		return s.resolveDefault(ctx)
	}
	if bound, err := s.resolveByID(ctx, ref, true); err == nil {
		return bound, nil
	} else if !errors.Is(err, ErrScannerNotConfigured) {
		return boundScanner{}, err
	}

	row, err := s.rowByType(ctx, ref)
	if err != nil {
		return boundScanner{}, err
	}
	return s.bind(ctx, row)
}

// resolveDefault returns the row flagged as default, or the oldest
// enabled row if the flag somehow went missing.
func (s *ConfigService) resolveDefault(ctx context.Context) (boundScanner, error) {
	row, err := scanScannerRow(s.db.QueryRowContext(ctx, scannerSelect+`
		WHERE enabled = 1
		ORDER BY is_default DESC, created_at ASC, id ASC
		LIMIT 1
	`).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return boundScanner{}, ErrNoScannerConfigured
	}
	if err != nil {
		return boundScanner{}, fmt.Errorf("reading default scanner: %w", err)
	}
	return s.bind(ctx, row)
}

// rowByType finds the oldest enabled row of one scanner type.
func (s *ConfigService) rowByType(ctx context.Context, scannerType string) (scannerRow, error) {
	row, err := scanScannerRow(s.db.QueryRowContext(ctx, scannerSelect+`
		WHERE scanner_type = ? AND enabled = 1
		ORDER BY is_default DESC, created_at ASC, id ASC
		LIMIT 1
	`, scannerType).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrScannerNotConfigured
	}
	if err != nil {
		return row, fmt.Errorf("reading scanner config for type %s: %w", scannerType, err)
	}
	return row, nil
}

// resolveByID binds one row by its own id.
//
// requireEnabled is false only for Test, which exists precisely so an
// operator can check a scanner *before* switching it on. Every scan
// path passes true: a disabled scanner that still runs when named
// directly would make the enable switch a suggestion rather than a
// control, and the Scanners tab would be telling the operator
// something untrue.
func (s *ConfigService) resolveByID(ctx context.Context, id string, requireEnabled bool) (boundScanner, error) {
	row, err := s.readRow(ctx, id)
	if err != nil {
		return boundScanner{}, err
	}
	if requireEnabled && !row.enabled {
		return boundScanner{}, fmt.Errorf("%w: %s is disabled", ErrScannerUnavailable, row.name)
	}
	return s.bind(ctx, row)
}

// bind pairs a row with its adapter and decrypts the config.
func (s *ConfigService) bind(ctx context.Context, row scannerRow) (boundScanner, error) {
	spec, client, ok := s.spec(row.scannerType)
	if !ok {
		return boundScanner{}, ErrScannerAdapterUnknown
	}
	cfg, err := s.resolvedConfig(spec, row)
	if err != nil {
		return boundScanner{}, err
	}
	return boundScanner{
		configID:    row.id,
		name:        row.name,
		scannerType: row.scannerType,
		client:      client,
		spec:        spec,
		config:      cfg,
		timeout:     s.scanTimeout(ctx),
	}, nil
}

// scanTimeout reads the operator's configured per-scan deadline.
//
// This is one of the three scanner settings that stayed in core, and
// the reason it did: it bounds how long the *host* holds a scan open,
// which is not an adapter's business and must not vary between them.
func (s *ConfigService) scanTimeout(ctx context.Context) time.Duration {
	settings := coreSettings.ReadScannerSettingsContext(ctx, s.db)
	if settings.ScanTimeout <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(settings.ScanTimeout) * time.Second
}

// resolvedConfig decrypts secret fields and adds the host-owned values
// every adapter may want but none of them store: the instance id and
// its type.
func (s *ConfigService) resolvedConfig(spec plugin.ImageScannerAdapterSpec, row scannerRow) (map[string]string, error) {
	out := make(map[string]string, len(row.config)+2)
	secret := make(map[string]bool, len(spec.ConfigSchema))
	for _, decl := range spec.ConfigSchema {
		if decl.Type == "secret" {
			secret[decl.Key] = true
		}
	}
	for k, v := range row.config {
		if secret[k] && v != "" {
			plain, err := s.enc.Decrypt(v)
			if err != nil {
				return nil, fmt.Errorf("decrypting scanner %s field %s: %w", row.id, k, err)
			}
			out[k] = plain
			continue
		}
		out[k] = v
	}
	// Fill declared defaults the operator never overrode, so an
	// adapter reads the same value whether the form was saved before
	// or after the field was added to its schema.
	for _, decl := range spec.ConfigSchema {
		if _, present := out[decl.Key]; present && out[decl.Key] != "" {
			continue
		}
		if decl.Default != nil {
			out[decl.Key] = fmt.Sprintf("%v", decl.Default)
		}
	}
	out["__scanner_id"] = row.id
	out["__scanner_type"] = spec.ScannerType
	return out, nil
}

// boolInt maps a Go bool onto the 0/1 SQLite stores.
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
