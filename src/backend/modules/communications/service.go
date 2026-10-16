// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

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
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// Service handles communication channel business logic and database operations.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
	// pluginRuntime is the host's loaded plugin runtime, set by
	// SetPluginRuntime after both modules finish wiring. Held as
	// `any` to avoid an import cycle between modules/communications
	// and the plugin runtime; test.go does the type assertion to
	// plugin.NotificationAdapterSource.
	pluginRuntime any
	// vault reads this module's adopted credential through the secrets vault
	// once Phase 5b's start-up backfill has linked it, and falls back to the
	// legacy column until then. Built here rather than injected: every
	// construction site already supplies the database and the encryption
	// service, and a service built without it would quietly read the legacy
	// column forever.
	vault *secrets.Service
}

// ErrChannelNotFound reports that no communication channel row matches the requested id.
var ErrChannelNotFound = errors.New("communication channel not found")

// NewService creates a new communication channel service.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	svc := &Service{db: database, enc: enc}
	if database != nil && enc != nil {
		svc.vault = secrets.NewService(database, enc, nil)
	}
	return svc
}

// SetPluginRuntime injects the loaded plugin runtime into the service.
// The runtime is held as `any` to avoid the import cycle; the lookup
// method does the type assertion to plugin.NotificationAdapterSource.
func (s *Service) SetPluginRuntime(runtime any) {
	s.pluginRuntime = runtime
}

// List returns all communication channels (secrets excluded).
func (s *Service) List(ctx context.Context) ([]CommunicationChannel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, channel_type, method, is_default, enabled, config_enc, created_at, updated_at
		 FROM communication_channels ORDER BY name ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing communication channels: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	var items []CommunicationChannel
	for rows.Next() {
		ch, err := s.scanChannel(ctx, rows, false)
		if err != nil {
			return nil, fmt.Errorf("scanning communication channel row: %w", err)
		}
		items = append(items, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating communication channel rows: %w", err)
	}

	if items == nil {
		items = []CommunicationChannel{}
	}

	return items, nil
}

// ByID returns a single communication channel by ID, or nil if not found.
func (s *Service) ByID(ctx context.Context, id string) (*CommunicationChannel, error) {
	return s.byID(ctx, id, false)
}

func (s *Service) byID(ctx context.Context, id string, includeSecrets bool) (*CommunicationChannel, error) {
	var ch CommunicationChannel
	var isDefault, enabled sql.NullBool
	var configEnc sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, channel_type, method, is_default, enabled, config_enc, created_at, updated_at
		 FROM communication_channels WHERE id = ?`, id,
	).Scan(&ch.ID, &ch.Name, &ch.ChannelType, &ch.Method, &isDefault, &enabled,
		&configEnc, &ch.CreatedAt, &ch.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting communication channel %s: %w", id, err)
	}

	ch.IsDefault = isDefault.Bool
	ch.Enabled = enabled.Bool
	cfg, err := s.decryptConfig(ctx, configEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting communication channel config: %w", err)
	}
	applyChannelConfig(&ch, cfg, includeSecrets, s.schemaSecretKeys(ch.ChannelType))

	return &ch, nil
}

// Create inserts a new communication channel, encrypting sensitive fields.
func (s *Service) Create(ctx context.Context, input CreateChannelInput) (*CommunicationChannel, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	config := channelConfigFromInput(input)
	configEnc, err := s.encryptConfig(config)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
			return nil, fmt.Errorf("clearing existing default: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO communication_channels (id, name, channel_type, method, is_default, enabled, config_enc, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		id, input.Name, input.ChannelType, input.Method, input.IsDefault, configEnc, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting communication channel: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &CommunicationChannel{
		ID:             id,
		Name:           input.Name,
		ChannelType:    input.ChannelType,
		Method:         input.Method,
		IsDefault:      input.IsDefault,
		Enabled:        true,
		ServerURL:      config["server_url"],
		Topic:          config["topic"],
		ChatID:         config["chat_id"],
		PhoneNumberID:  config["phone_number_id"],
		RecipientPhone: config["recipient_phone"],
		SenderNumber:   config["sender_number"],
		Recipients:     config["recipients"],
		Username:       config["username"],
		Priority:       config["priority"],
		Config:         redactConfig(config, s.schemaSecretKeys(input.ChannelType)),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Update applies partial updates to a communication channel.
func (s *Service) Update(ctx context.Context, id string, input UpdateChannelInput) (*CommunicationChannel, error) {
	ch, err := s.byID(ctx, id, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking communication channel existence: %w", err)
	}
	if ch == nil {
		return nil, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	config := make(map[string]string, len(ch.Config)+len(input.Config)+12)
	for k, v := range ch.Config {
		config[k] = v
	}
	mergeChannelConfigUpdate(config, input)
	configEnc, err := s.encryptConfig(config)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	if input.Name != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET name = ?, updated_at = ? WHERE id = ?", *input.Name, now, id); err != nil {
			return nil, fmt.Errorf("updating channel name: %w", err)
		}
	}
	if input.Method != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET method = ?, updated_at = ? WHERE id = ?", *input.Method, now, id); err != nil {
			return nil, fmt.Errorf("updating channel method: %w", err)
		}
	}
	if input.Enabled != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET enabled = ?, updated_at = ? WHERE id = ?", *input.Enabled, now, id); err != nil {
			return nil, fmt.Errorf("updating channel enabled: %w", err)
		}
	}
	if input.IsDefault != nil && *input.IsDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
			return nil, fmt.Errorf("clearing existing default: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 1, updated_at = ? WHERE id = ?", now, id); err != nil {
			return nil, fmt.Errorf("setting new default: %w", err)
		}
	} else if input.IsDefault != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
			return nil, fmt.Errorf("clearing default flag: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET config_enc = ?, config_secret_id = NULL, updated_at = ? WHERE id = ?", configEnc, now, id); err != nil {
		return nil, fmt.Errorf("updating channel config: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return s.ByID(ctx, id)
}

// Delete removes a communication channel by ID. Returns true if a row was deleted.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM communication_channels WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting communication channel %s: %w", id, err)
	}

	return db.RowsAffected(result) > 0, nil
}

// SetDefault sets the given channel as the default, clearing any existing default.
func (s *Service) SetDefault(ctx context.Context, id string) error {
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM communication_channels WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return fmt.Errorf("communication channel not found")
	} else if err != nil {
		return fmt.Errorf("checking communication channel existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer db.RollbackTx(tx)

	if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
		return fmt.Errorf("clearing existing default: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 1, updated_at = ? WHERE id = ?", now, id); err != nil {
		return fmt.Errorf("setting new default: %w", err)
	}

	return tx.Commit()
}

func (s *Service) scanChannel(ctx context.Context, rows *sql.Rows, includeSecrets bool) (CommunicationChannel, error) {
	var ch CommunicationChannel
	var isDefault, enabled sql.NullBool
	var configEnc sql.NullString

	if err := rows.Scan(&ch.ID, &ch.Name, &ch.ChannelType, &ch.Method, &isDefault, &enabled,
		&configEnc, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
		return CommunicationChannel{}, err
	}

	ch.IsDefault = isDefault.Bool
	ch.Enabled = enabled.Bool
	cfg, err := s.decryptConfig(ctx, configEnc)
	if err != nil {
		return CommunicationChannel{}, err
	}
	applyChannelConfig(&ch, cfg, includeSecrets, s.schemaSecretKeys(ch.ChannelType))

	return ch, nil
}

func (s *Service) encryptConfig(config map[string]string) (sql.NullString, error) {
	clean := compactConfig(config)
	if len(clean) == 0 {
		return sql.NullString{}, nil
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshalling communication channel config: %w", err)
	}
	enc, err := s.enc.Encrypt(string(raw))
	if err != nil {
		return sql.NullString{}, fmt.Errorf("encrypting communication channel config: %w", err)
	}
	return sql.NullString{String: enc, Valid: true}, nil
}

func (s *Service) decryptConfig(ctx context.Context, configEnc sql.NullString) (map[string]string, error) {
	// Passes no secret id, so it never reaches the vault — this is the plain
	// decrypt path for rows the backfill has not adopted yet.
	return s.decryptConfigThrough(ctx, sql.NullString{}, configEnc)
}

// decryptConfigThrough reads a channel's config blob through the vault when
// the row has been adopted, and falls back to the legacy config_enc column
// when it has not.
func (s *Service) decryptConfigThrough(ctx context.Context, secretID, configEnc sql.NullString) (map[string]string, error) {
	if !configEnc.Valid && !secretID.Valid {
		return map[string]string{}, nil
	}
	if configEnc.String == "" && secretID.String == "" {
		return map[string]string{}, nil
	}
	var (
		raw string
		err error
	)
	if s.vault != nil {
		raw, err = s.vault.ReadThrough(ctx, secretID, configEnc.String)
	} else {
		raw, err = s.enc.Decrypt(configEnc.String)
	}
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return map[string]string{}, nil
	}
	cfg := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling communication channel config: %w", err)
	}
	return cfg, nil
}

func channelConfigFromInput(input CreateChannelInput) map[string]string {
	cfg := make(map[string]string, len(input.Config)+12)
	for k, v := range input.Config {
		setConfigValue(cfg, normalizeConfigKey(k), v)
	}
	setLegacyValue(cfg, "webhook_url", input.WebhookURL)
	setLegacyValue(cfg, "server_url", input.ServerURL)
	setLegacyValue(cfg, "token", input.Token)
	setLegacyValue(cfg, "topic", input.Topic)
	setLegacyValue(cfg, "chat_id", input.ChatID)
	setLegacyValue(cfg, "phone_number_id", input.PhoneNumberID)
	setLegacyValue(cfg, "recipient_phone", input.RecipientPhone)
	setLegacyValue(cfg, "sender_number", input.SenderNumber)
	setLegacyValue(cfg, "recipients", input.Recipients)
	setLegacyValue(cfg, "username", input.Username)
	setLegacyValue(cfg, "password", input.Password)
	setLegacyValue(cfg, "priority", input.Priority)
	return cfg
}

// setLegacyValue applies a pre-plugin top-level field only when it carries a value, so an
// omitted one never erases the same key a plugin form sent inside config.
func setLegacyValue(cfg map[string]string, key, value string) {
	if strings.TrimSpace(value) != "" {
		setConfigValue(cfg, key, value)
	}
}

func mergeChannelConfigUpdate(cfg map[string]string, input UpdateChannelInput) {
	for k, v := range input.Config {
		setConfigValue(cfg, normalizeConfigKey(k), v)
	}
	setConfigValuePtr(cfg, "webhook_url", input.WebhookURL, true)
	setConfigValuePtr(cfg, "server_url", input.ServerURL, false)
	setConfigValuePtr(cfg, "token", input.Token, true)
	setConfigValuePtr(cfg, "topic", input.Topic, false)
	setConfigValuePtr(cfg, "chat_id", input.ChatID, false)
	setConfigValuePtr(cfg, "phone_number_id", input.PhoneNumberID, false)
	setConfigValuePtr(cfg, "recipient_phone", input.RecipientPhone, false)
	setConfigValuePtr(cfg, "sender_number", input.SenderNumber, false)
	setConfigValuePtr(cfg, "recipients", input.Recipients, false)
	setConfigValuePtr(cfg, "username", input.Username, false)
	setConfigValuePtr(cfg, "password", input.Password, true)
	setConfigValuePtr(cfg, "priority", input.Priority, false)
}

func applyChannelConfig(ch *CommunicationChannel, cfg map[string]string, includeSecrets bool, secretKeys map[string]bool) {
	ch.ServerURL = cfg["server_url"]
	ch.Topic = cfg["topic"]
	ch.ChatID = cfg["chat_id"]
	ch.PhoneNumberID = cfg["phone_number_id"]
	ch.RecipientPhone = cfg["recipient_phone"]
	ch.SenderNumber = cfg["sender_number"]
	ch.Recipients = cfg["recipients"]
	ch.Username = cfg["username"]
	ch.Priority = cfg["priority"]
	if includeSecrets {
		ch.Config = cfg
		return
	}
	ch.Config = redactConfig(cfg, secretKeys)
}

func applyPluginConfigAliases(channelType string, cfg map[string]string) {
	switch channelType {
	case "ntfy":
		if cfg["access_token"] == "" && cfg["token"] != "" {
			cfg["access_token"] = cfg["token"]
		}
	case "signal":
		if cfg["method"] == "rest_api" {
			cfg["method"] = "rest"
		}
		if cfg["auth_username"] == "" && cfg["username"] != "" {
			cfg["auth_username"] = cfg["username"]
		}
		if cfg["auth_password"] == "" && cfg["password"] != "" {
			cfg["auth_password"] = cfg["password"]
		}
		if cfg["bot_token"] == "" && cfg["token"] != "" {
			cfg["bot_token"] = cfg["token"]
		}
		if cfg["account_id"] == "" && cfg["sender_number"] != "" {
			cfg["account_id"] = cfg["sender_number"]
		}
	case "telegram":
		if cfg["bot_token"] == "" && cfg["token"] != "" {
			cfg["bot_token"] = cfg["token"]
		}
	case "whatsapp":
		switch cfg["method"] {
		case "cloud_api":
			cfg["method"] = "cloud"
		case "business":
			cfg["method"] = "bot"
		}
		if cfg["access_token"] == "" && cfg["token"] != "" {
			cfg["access_token"] = cfg["token"]
		}
		if cfg["gateway_url"] == "" && cfg["server_url"] != "" {
			cfg["gateway_url"] = cfg["server_url"]
		}
		if cfg["gateway_token"] == "" && cfg["token"] != "" {
			cfg["gateway_token"] = cfg["token"]
		}
		if cfg["business_api_url"] == "" && cfg["server_url"] != "" {
			cfg["business_api_url"] = cfg["server_url"]
		}
		if cfg["api_key"] == "" && cfg["token"] != "" {
			cfg["api_key"] = cfg["token"]
		}
		if cfg["provider_url"] == "" && cfg["server_url"] != "" {
			cfg["provider_url"] = cfg["server_url"]
		}
		if cfg["provider_api_key"] == "" && cfg["token"] != "" {
			cfg["provider_api_key"] = cfg["token"]
		}
		if cfg["provider_from"] == "" && cfg["phone_number_id"] != "" {
			cfg["provider_from"] = cfg["phone_number_id"]
		}
	}
}

func compactConfig(config map[string]string) map[string]string {
	clean := map[string]string{}
	for k, v := range config {
		setConfigValue(clean, normalizeConfigKey(k), v)
	}
	return clean
}

// schemaSecretKeys lists the config keys the channel's adapter declares secret, so a
// credential is redacted by what the plugin says it is, not only by what it is named.
func (s *Service) schemaSecretKeys(channelType string) map[string]bool {
	if s.pluginRuntime == nil {
		return nil
	}
	source, ok := s.pluginRuntime.(notificationAdapterSource)
	if !ok {
		return nil
	}
	_, spec, ok := source.NotificationAdapter(channelType)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for _, field := range spec.ConfigSchema {
		if field.Secret || field.Type == "secret" {
			keys[field.Key] = true
		}
	}
	return keys
}

func redactConfig(config map[string]string, secretKeys map[string]bool) map[string]string {
	safe := map[string]string{}
	for k, v := range config {
		if isSecretConfigKey(k) || secretKeys[k] {
			continue
		}
		safe[k] = v
	}
	if len(safe) == 0 {
		return nil
	}
	return safe
}

func isSecretConfigKey(key string) bool {
	switch normalizeConfigKey(key) {
	case "password", "token", "webhook_url", "secret", "client_secret", "access_token", "bot_token":
		return true
	default:
		return strings.Contains(strings.ToLower(key), "password") || strings.Contains(strings.ToLower(key), "secret") || strings.Contains(strings.ToLower(key), "token")
	}
}

func normalizeConfigKey(key string) string {
	switch key {
	case "webhookUrl":
		return "webhook_url"
	case "serverUrl":
		return "server_url"
	case "chatId":
		return "chat_id"
	case "phoneNumberId":
		return "phone_number_id"
	case "recipientPhone":
		return "recipient_phone"
	case "senderNumber":
		return "sender_number"
	default:
		return key
	}
}

func setConfigValue(cfg map[string]string, key, value string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		delete(cfg, key)
		return
	}
	cfg[key] = value
}

func setConfigValuePtr(cfg map[string]string, key string, value *string, preserveEmpty bool) {
	if value == nil {
		return
	}
	if preserveEmpty && strings.TrimSpace(*value) == "" {
		return
	}
	setConfigValue(cfg, key, *value)
}

// AdapterCapability is one notification adapter's declared capabilities,
// as recorded by the plugin loader into plugin_notification_adapters.
type AdapterCapability struct {
	ChannelType             string `json:"channelType"`
	DisplayName             string `json:"displayName"`
	SupportsFileAttachments bool   `json:"supportsFileAttachments"`
	SupportsRichAttachments bool   `json:"supportsRichAttachments"`
	// SupportsEmail reports whether this adapter delivers real email —
	// an addressed message with a sender and a subject — rather than a
	// chat post or a push notification.
	//
	// Declared by the plugin through an email-channel capability rather
	// than listed here, so a future comm-mailgun plugin is picked up
	// without a host change. Unlike the three flags above this one is
	// not mirrored into plugin_notification_adapters: it is read per
	// request from the in-process capability registry, which holds only
	// loaded plugins.
	SupportsEmail bool `json:"supportsEmail"`

	// Recipient describes the adapter's overridable destination, or
	// is zero when it has none. Mirrored into
	// plugin_notification_adapters at load, so a caller outside this
	// process - the reporting scheduler's target picker above all -
	// can offer an "send this one somewhere else" field only where
	// the provider genuinely allows one.
	Recipient AdapterRecipient `json:"recipient"`
}

// AdapterRecipient is one adapter's declared overridable destination.
//
// Key is deliberately not exported to the frontend: which config
// column a value lands in is the host's business, and publishing it
// would invite a caller to try setting a different one. Supported is
// the flag a UI branches on.
type AdapterRecipient struct {
	Supported   bool   `json:"supported"`
	Kind        string `json:"kind,omitempty"`
	Label       string `json:"label,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Multiple    bool   `json:"multiple,omitempty"`

	key string
}

// Key is the config field the override replaces. Empty when the
// adapter declared none.
func (r AdapterRecipient) Key() string { return r.key }

// AdapterCapabilities lists what each registered channel type can do.
//
// The flags also live in the in-memory adapter registry, but callers
// outside this process — a schedule's target picker deciding whether to
// offer "attach the document" — cannot reach that, which is why the
// loader mirrors them into a table.
func (s *Service) AdapterCapabilities(ctx context.Context) ([]AdapterCapability, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT channel_type, display_name,
		        supports_file_attachments, supports_rich_attachments,
		        recipient_key, recipient_kind, recipient_label,
		        recipient_placeholder, recipient_multiple
		   FROM plugin_notification_adapters
		  ORDER BY display_name, channel_type
		  LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing adapter capabilities: %w", err)
	}
	defer func() { db.CloseRows(rows) }()

	out := []AdapterCapability{}
	for rows.Next() {
		var c AdapterCapability
		if err := rows.Scan(&c.ChannelType, &c.DisplayName,
			&c.SupportsFileAttachments, &c.SupportsRichAttachments,
			&c.Recipient.key, &c.Recipient.Kind, &c.Recipient.Label,
			&c.Recipient.Placeholder, &c.Recipient.Multiple); err != nil {
			return nil, fmt.Errorf("scanning adapter capability: %w", err)
		}
		c.Recipient.Supported = strings.TrimSpace(c.Recipient.key) != ""
		c.SupportsEmail = s.adapterSupportsEmail(c.ChannelType)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading adapter capabilities: %w", err)
	}
	return out, nil
}

// adapterSupportsEmail reports whether a loaded plugin declares channelType an email
// transport. It reads the same email-channel capability the workflow send-email node
// lists channels by, so the security picker and the node cannot disagree.
func (s *Service) adapterSupportsEmail(channelType string) bool {
	return plugincaps.Default.IsEmailChannel(channelType)
}
