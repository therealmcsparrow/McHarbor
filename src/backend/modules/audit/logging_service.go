// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

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
)

// maxLoggingHistoryRows caps what the Logging tab's history panel asks
// an adapter for. The panel shows the last handful of attempts to
// answer "is this sink actually receiving events" — it is not a log
// browser, and an unbounded limit would let a plugin return its entire
// delivery table into a settings page.
const maxLoggingHistoryRows = 50

// maxForwarderRows bounds the unpaginated list the Logging tab reads.
// A thousand configured sinks is already absurd; the cap exists so a
// corrupted table cannot turn a settings page into an OOM.
const maxForwarderRows = 1000

// ErrForwarderUnknown is returned when no installed plugin has
// registered the requested forwarder type. Callers map it to 404
// rather than 500: an operator hitting a stale bookmark for a
// forwarder whose plugin they uninstalled is a missing resource, not
// a server fault.
var ErrForwarderUnknown = errors.New("audit: unknown log forwarder type")

// ErrForwarderNotConfigured is returned when no forwarder row exists
// for the requested id.
var ErrForwarderNotConfigured = errors.New("audit: log forwarder is not configured")

// ErrForwarderFieldRequired is returned when a save would leave a
// field the adapter declared Required empty.
//
// This is the difference between a forwarder that does not work and a
// forwarder that looks fine in the list: a row with an empty endpoint
// renders as a configured sink and forwards nothing. Refusing the save
// keeps the list honest — everything in it has somewhere to send to.
var ErrForwarderFieldRequired = errors.New("audit: required log forwarder field is empty")

// ErrForwarderNameRequired is returned when a forwarder is saved with
// a blank name. The name is the only thing separating two instances of
// the same type in the list, so it is not optional.
var ErrForwarderNameRequired = errors.New("audit: log forwarder name is required")

// LoggingService owns the host half of audit log forwarding: the
// generic `audit_log_forwarders` config rows, secret encryption, and
// dispatch into whatever `logging-` plugins are installed.
//
// It deliberately knows nothing about HTTP collectors or syslog
// framing. Those live in the plugins; this type's entire job is to
// turn a config row into a decrypted map and hand it to an adapter.
//
// A row is one *instance*: a named destination of some type. Several
// rows may share a forwarder_type — two SIEM collectors, a local and
// an off-site syslog — and each carries its own config, its own
// enable state, and its own watermark.
type LoggingService struct {
	db      *sql.DB
	enc     *encryption.Service
	runtime any
}

// NewLoggingService builds the service. runtime is the plugin runtime
// held as `any` (the same shape AppDeps.PluginAdapters carries) and
// may be nil — a host with no plugin runtime simply reports no
// adapters, which is the correct empty state, not an error.
func NewLoggingService(database *sql.DB, enc *encryption.Service, runtime any) *LoggingService {
	return &LoggingService{db: database, enc: enc, runtime: runtime}
}

// SetPluginRuntime injects the loaded plugin runtime after construction.
func (s *LoggingService) SetPluginRuntime(runtime any) { s.runtime = runtime }

// source type-asserts the runtime to the log forwarder source. Returns
// (nil, false) when no runtime is wired or it predates the logging
// family — both mean "no adapters", never an error.
func (s *LoggingService) source() (hostplugin.LogForwarderAdapterSource, bool) {
	if s.runtime == nil {
		return nil, false
	}
	src, ok := s.runtime.(hostplugin.LogForwarderAdapterSource)
	return src, ok
}

// ForwarderField is one config field as the Logging tab renders it.
// Mirrors the adapter's declared ConfigFieldDecl, with the stored
// value attached — redacted for secrets.
type ForwarderField struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Default     any    `json:"default,omitempty"`
	// Value is the stored value for non-secret fields. Always empty
	// for secrets — see HasValue.
	Value string `json:"value"`
	// HasValue reports whether a secret field currently holds a
	// stored value, so the form can render "leave blank to keep"
	// instead of an empty box that looks unconfigured.
	HasValue bool `json:"hasValue"`
}

// ForwarderAdapterInfo is one installed adapter type — a tile in the
// Add picker, and the schema the create form renders.
//
// Adapters are listed separately from configured forwarders because
// the two answer different questions: "what can I add" is a property
// of what is installed, "what am I shipping to" is a property of what
// is configured. Folding them into one list is what forced the old
// one-instance-per-type model.
type ForwarderAdapterInfo struct {
	ForwarderType           string           `json:"forwarderType"`
	Slug                    string           `json:"slug"`
	DisplayName             string           `json:"displayName"`
	Icon                    string           `json:"icon,omitempty"`
	Description             string           `json:"description,omitempty"`
	SupportsDeliveryHistory bool             `json:"supportsDeliveryHistory"`
	EncryptedCredentials    bool             `json:"encryptedCredentials"`
	Fields                  []ForwarderField `json:"fields"`
}

// ForwarderInfo is one configured forwarder instance.
type ForwarderInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ForwarderType string `json:"forwarderType"`
	Slug          string `json:"slug,omitempty"`
	DisplayName   string `json:"displayName"`
	Icon          string `json:"icon,omitempty"`
	Description   string `json:"description,omitempty"`

	// Available reports whether an installed plugin still provides
	// this row's type. False when the plugin was uninstalled with the
	// forwarder still configured: the row is shown, greyed, and
	// deletable, rather than vanishing and leaving an operator
	// wondering where their SIEM config went.
	Available bool `json:"available"`

	Enabled      bool   `json:"enabled"`
	EnabledSince string `json:"enabledSince,omitempty"`

	SupportsDeliveryHistory bool `json:"supportsDeliveryHistory"`
	EncryptedCredentials    bool `json:"encryptedCredentials"`

	Fields []ForwarderField `json:"fields"`
}

// SaveForwarderInput is the Logging tab's form submission. Config
// carries the adapter's declared ConfigSchema keys; the host does not
// know or validate their meaning beyond the declared type.
type SaveForwarderInput struct {
	Name string
	// ForwarderType is read on create and ignored on update — the type
	// is the adapter binding, and rebinding a row to a different
	// transport would silently reinterpret every stored config key.
	ForwarderType string
	Enabled       bool
	Config        map[string]string
}

// Adapters returns one entry per registered adapter type, in
// registration order so the picker tiles do not reshuffle between
// renders.
func (s *LoggingService) Adapters() []ForwarderAdapterInfo {
	src, ok := s.source()
	if !ok {
		return []ForwarderAdapterInfo{}
	}
	specs := src.LogForwarderAdapters()
	out := make([]ForwarderAdapterInfo, 0, len(specs))
	for _, spec := range specs {
		out = append(out, ForwarderAdapterInfo{
			ForwarderType:           spec.ForwarderType,
			Slug:                    spec.Slug,
			DisplayName:             spec.DisplayName,
			Icon:                    spec.Icon,
			Description:             spec.Description,
			SupportsDeliveryHistory: spec.Capabilities.SupportsDeliveryHistory,
			EncryptedCredentials:    spec.Capabilities.EncryptedCredentials,
			Fields:                  buildFields(spec, forwarderRow{config: map[string]string{}}),
		})
	}
	return out
}

// Forwarders returns every configured instance, oldest first, with its
// stored config merged in.
func (s *LoggingService) Forwarders(ctx context.Context) ([]ForwarderInfo, error) {
	rows, err := s.readRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ForwarderInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.buildInfo(row))
	}
	return out, nil
}

// Forwarder returns one configured instance by id. Used by handlers
// that need the row's name for an audit entry before acting on it.
func (s *LoggingService) Forwarder(ctx context.Context, id string) (ForwarderInfo, error) {
	row, err := s.readRow(ctx, id)
	if err != nil {
		return ForwarderInfo{}, err
	}
	return s.buildInfo(row), nil
}

// buildInfo merges a row with its adapter spec. Secret values never
// leave this function — only the boolean that one exists.
func (s *LoggingService) buildInfo(row forwarderRow) ForwarderInfo {
	info := ForwarderInfo{
		ID:            row.id,
		Name:          row.name,
		ForwarderType: row.forwarderType,
		DisplayName:   row.name,
		Enabled:       row.enabled,
		EnabledSince:  row.enabledSince,
		Fields:        []ForwarderField{},
	}
	spec, _, ok := s.spec(row.forwarderType)
	if !ok {
		return info
	}
	info.Available = true
	info.Slug = spec.Slug
	info.DisplayName = spec.DisplayName
	info.Icon = spec.Icon
	info.Description = spec.Description
	info.SupportsDeliveryHistory = spec.Capabilities.SupportsDeliveryHistory
	info.EncryptedCredentials = spec.Capabilities.EncryptedCredentials
	info.Fields = buildFields(spec, row)
	return info
}

// buildFields renders one adapter's ConfigSchema against a stored row.
// Shared by the adapter list (empty row — schema and defaults only)
// and the instance list (stored values attached).
func buildFields(spec plugin.LogForwarderAdapterSpec, row forwarderRow) []ForwarderField {
	fields := make([]ForwarderField, 0, len(spec.ConfigSchema))
	for _, decl := range spec.ConfigSchema {
		f := ForwarderField{
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

// forwarderRow is the decoded `audit_log_forwarders` row. config holds
// raw stored values: plaintext for ordinary fields, ciphertext for
// secrets.
type forwarderRow struct {
	id            string
	name          string
	forwarderType string
	enabled       bool
	enabledSince  string
	config        map[string]string
}

const forwarderSelect = `
	SELECT id, name, forwarder_type, enabled, config, enabled_since
	FROM audit_log_forwarders
`

// scanRow decodes one row's columns, including its JSON config blob.
func scanRow(scan func(dest ...any) error) (forwarderRow, error) {
	row := forwarderRow{config: map[string]string{}}
	var enabled int
	var configJSON string
	var enabledSince sql.NullString
	if err := scan(&row.id, &row.name, &row.forwarderType, &enabled, &configJSON, &enabledSince); err != nil {
		return row, err
	}
	row.enabled = enabled == 1
	row.enabledSince = enabledSince.String
	if strings.TrimSpace(configJSON) != "" {
		if err := json.Unmarshal([]byte(configJSON), &row.config); err != nil {
			return row, fmt.Errorf("decoding audit log forwarder %s config: %w", row.id, err)
		}
	}
	return row, nil
}

// readRow loads one instance by id.
func (s *LoggingService) readRow(ctx context.Context, id string) (forwarderRow, error) {
	row, err := scanRow(s.db.QueryRowContext(ctx, forwarderSelect+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrForwarderNotConfigured
	}
	if err != nil {
		return row, fmt.Errorf("reading audit log forwarder %s: %w", id, err)
	}
	return row, nil
}

// readRows loads every instance, oldest first so the list order is
// "the order they were added" rather than something that shifts when a
// forwarder is renamed.
func (s *LoggingService) readRows(ctx context.Context) ([]forwarderRow, error) {
	rows, err := s.db.QueryContext(ctx,
		forwarderSelect+` ORDER BY created_at ASC, id ASC LIMIT ?`, maxForwarderRows)
	if err != nil {
		return nil, fmt.Errorf("listing audit log forwarders: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()

	var out []forwarderRow
	for rows.Next() {
		row, err := scanRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning audit log forwarder: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// enabledRows loads every instance the worker should drive.
func (s *LoggingService) enabledRows(ctx context.Context) ([]forwarderRow, error) {
	rows, err := s.db.QueryContext(ctx,
		forwarderSelect+` WHERE enabled = 1 ORDER BY created_at ASC, id ASC LIMIT ?`, maxForwarderRows)
	if err != nil {
		return nil, fmt.Errorf("listing enabled audit log forwarders: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()

	var out []forwarderRow
	for rows.Next() {
		row, err := scanRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning enabled audit log forwarder: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// mergeConfig folds a form submission onto what is already stored.
//
// Secret handling follows the rule the old SIEM form used: an empty
// submitted secret means "keep what is stored", never "clear it".
// Clearing a secret is done by deleting the forwarder, not by blanking
// a field the UI never shows the value of — otherwise every save from
// a form that cannot echo the secret would silently wipe it.
func (s *LoggingService) mergeConfig(spec plugin.LogForwarderAdapterSpec, submitted, existing map[string]string) (string, error) {
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
				return "", fmt.Errorf("encrypting audit log forwarder field %s: %w", decl.Key, err)
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
	// this too, but the check belongs here: a row with an empty
	// endpoint reads as a configured sink to every other code path —
	// the worker, the list, the operator — while forwarding nothing.
	for _, decl := range spec.ConfigSchema {
		if decl.Required && strings.TrimSpace(merged[decl.Key]) == "" {
			return "", fmt.Errorf("%w: %s", ErrForwarderFieldRequired, decl.Key)
		}
	}

	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("encoding audit log forwarder config: %w", err)
	}
	return string(encoded), nil
}

// Create adds one forwarder instance and returns its id.
func (s *LoggingService) Create(ctx context.Context, in SaveForwarderInput) (string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return "", ErrForwarderNameRequired
	}
	spec, _, ok := s.spec(strings.TrimSpace(in.ForwarderType))
	if !ok {
		return "", ErrForwarderUnknown
	}
	encoded, err := s.mergeConfig(spec, in.Config, map[string]string{})
	if err != nil {
		return "", err
	}

	// enabled_since is the replay watermark. A forwarder created
	// already enabled starts from now, so adding a second collector to
	// a long-running install ships what happens next rather than
	// replaying the whole trail into it.
	var enabledSince any
	if in.Enabled {
		enabledSince = time.Now().UTC().Format(time.RFC3339)
	}

	id := xid.New().String()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_log_forwarders (id, name, forwarder_type, enabled, config, enabled_since)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, name, spec.ForwarderType, boolInt(in.Enabled), encoded, enabledSince)
	if err != nil {
		return "", fmt.Errorf("inserting audit log forwarder: %w", err)
	}
	return id, nil
}

// Update rewrites one forwarder instance. The type is fixed at
// creation; everything else is editable.
func (s *LoggingService) Update(ctx context.Context, id string, in SaveForwarderInput) error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return ErrForwarderNameRequired
	}
	existing, err := s.readRow(ctx, id)
	if err != nil {
		return err
	}
	spec, _, ok := s.spec(existing.forwarderType)
	if !ok {
		return ErrForwarderUnknown
	}
	encoded, err := s.mergeConfig(spec, in.Config, existing.config)
	if err != nil {
		return err
	}

	// Stamp the watermark on the transition into enabled and leave it
	// alone otherwise, so toggling off and on again does not re-send
	// everything since the first time it was ever switched on.
	enabledSince := existing.enabledSince
	if in.Enabled && !existing.enabled {
		enabledSince = time.Now().UTC().Format(time.RFC3339)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE audit_log_forwarders
		SET name = ?, enabled = ?, config = ?, enabled_since = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, boolInt(in.Enabled), encoded, nullableString(enabledSince), id)
	if err != nil {
		return fmt.Errorf("updating audit log forwarder %s: %w", id, err)
	}
	// The row was read a moment ago, so zero rows here means it was
	// deleted concurrently — report it as missing rather than a silent
	// no-op that leaves the operator thinking they saved.
	if db.RowsAffected(result) == 0 {
		return ErrForwarderNotConfigured
	}
	return nil
}

// Delete removes one forwarder instance.
//
// Only the host's config row goes. The adapter's own delivery history
// lives in `plugin_<slug>_delivery_history` and is the plugin's to
// keep or prune: an operator removing a sink must not silently erase
// the record of what was already shipped to it, which is exactly the
// evidence a compliance audit asks for.
func (s *LoggingService) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM audit_log_forwarders WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("deleting audit log forwarder %s: %w", id, err)
	}
	if db.RowsAffected(result) == 0 {
		return ErrForwarderNotConfigured
	}
	return nil
}

// Test hands the adapter a decrypted config and asks it to hit the
// wire.
func (s *LoggingService) Test(ctx context.Context, id string) error {
	row, err := s.readRow(ctx, id)
	if err != nil {
		return err
	}
	spec, client, ok := s.spec(row.forwarderType)
	if !ok {
		return ErrForwarderUnknown
	}
	cfg, err := s.resolvedConfig(spec, row)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return client.Test(ctx, cfg)
}

// History returns this instance's recent delivery attempts. Adapters
// that declared SupportsDeliveryHistory=false return an empty slice
// rather than a fabricated one.
func (s *LoggingService) History(ctx context.Context, id string) ([]plugin.LogDelivery, error) {
	row, err := s.readRow(ctx, id)
	if err != nil {
		return nil, err
	}
	spec, client, ok := s.spec(row.forwarderType)
	if !ok {
		return nil, ErrForwarderUnknown
	}
	if !spec.Capabilities.SupportsDeliveryHistory {
		return []plugin.LogDelivery{}, nil
	}
	cfg, err := s.resolvedConfig(spec, row)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	items, err := client.DeliveryHistory(ctx, cfg, maxLoggingHistoryRows)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []plugin.LogDelivery{}, nil
	}
	return items, nil
}

// spec resolves an adapter by type.
func (s *LoggingService) spec(forwarderType string) (plugin.LogForwarderAdapterSpec, plugin.LogForwarderAdapterClient, bool) {
	src, ok := s.source()
	if !ok {
		return plugin.LogForwarderAdapterSpec{}, nil, false
	}
	client, spec, ok := src.LogForwarderAdapter(forwarderType)
	if !ok {
		return plugin.LogForwarderAdapterSpec{}, nil, false
	}
	return spec, client, true
}

// resolvedConfig decrypts secret fields and adds the host-owned values
// every adapter needs but none of them store: the instance id, the
// enabled_since watermark, and the forwarder's own type.
//
// `__forwarder_id` is what makes several instances of one type
// possible. Each adapter dedups deliveries against its own history
// table; scoping that table by the instance id is the difference
// between two SIEM collectors each receiving every event and the two
// of them splitting the trail between them at random.
func (s *LoggingService) resolvedConfig(spec plugin.LogForwarderAdapterSpec, row forwarderRow) (map[string]string, error) {
	out := make(map[string]string, len(row.config)+3)
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
				return nil, fmt.Errorf("decrypting audit log forwarder %s field %s: %w", row.id, k, err)
			}
			out[k] = plain
			continue
		}
		out[k] = v
	}
	out["__forwarder_id"] = row.id
	out["__forwarder_type"] = spec.ForwarderType
	out["__enabled_since"] = row.enabledSince
	return out, nil
}

// enabledForwarders returns every enabled instance paired with its
// decrypted config. The forwarding worker calls this once per tick.
func (s *LoggingService) enabledForwarders(ctx context.Context) ([]enabledForwarder, error) {
	if _, ok := s.source(); !ok {
		return nil, nil
	}
	rows, err := s.enabledRows(ctx)
	if err != nil {
		return nil, err
	}
	var out []enabledForwarder
	for _, row := range rows {
		// A watermark-less row was never switched on; nothing to send.
		if row.enabledSince == "" {
			continue
		}
		spec, client, ok := s.spec(row.forwarderType)
		if !ok {
			// The plugin was uninstalled with the forwarder still
			// enabled. Skipping is right: the row stays visible in the
			// tab so an operator can see what stopped.
			continue
		}
		cfg, err := s.resolvedConfig(spec, row)
		if err != nil {
			return nil, err
		}
		out = append(out, enabledForwarder{
			id:     row.id,
			name:   row.name,
			spec:   spec,
			client: client,
			config: cfg,
		})
	}
	return out, nil
}

type enabledForwarder struct {
	id     string
	name   string
	spec   plugin.LogForwarderAdapterSpec
	client plugin.LogForwarderAdapterClient
	config map[string]string
}

// nullableString maps "" to a SQL NULL so enabled_since stays NULL
// until forwarding is first switched on, rather than holding an empty
// string that reads as a valid-but-zero timestamp.
func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
