// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package notify delivers operator notifications over the enabled communication channels.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

var (
	// ErrChannelTargetNotFound indicates that no enabled communication channel could be resolved.
	ErrChannelTargetNotFound = errors.New("communication channel target not found")
)

// Dispatcher resolves canonical communication channels and sends messages through them.
type Dispatcher struct {
	db            *sql.DB
	enc           *encryption.Service
	pluginRuntime any
}

// ChannelRequest describes an outbound notification to be sent through a configured communication channel.
type ChannelRequest struct {
	ChannelID   string
	ChannelType string

	// ChannelTypes narrows an id-less, type-less send to the default channel of any of
	// these types, for callers that ask for a capability rather than a provider.
	ChannelTypes []string

	Title   string
	Message string

	// Files are optional attachments. They reach the provider only
	// when the resolved adapter declared SupportsFileAttachments;
	// otherwise the dispatcher drops them and sends Title + Message
	// alone. Callers that care are expected to put a link in Message
	// so the recipient is never left with nothing — the dispatcher
	// cannot write that fallback text for them, because only the
	// caller knows where the file can be fetched from.
	Files []plugin.NotificationFile

	// ConfigOverrides replaces individual keys of the resolved
	// channel config for this send only. The stored row is never
	// touched, so a channel configured to mail the ops list can also
	// carry a one-off message to a single person.
	//
	// The password-reset flow is the first caller: it overrides `to`
	// with the account's own address and `subject_prefix` with the
	// empty string so the operator-authored subject arrives verbatim.
	// An empty value is an intentional override, not "leave it alone" —
	// callers that mean "leave it alone" omit the key.
	//
	// Never put a credential here. Overrides are applied after the
	// alias pass and win over both the row and its aliases.
	ConfigOverrides map[string]string

	// Recipient retargets this one send to a different destination,
	// without touching the stored channel.
	//
	// Where ConfigOverrides is a trusted, host-internal escape hatch
	// that can set any key, this is the narrow form safe to expose to
	// a plugin: the dispatcher writes it into the single config key
	// the resolved adapter declared as its overridable destination
	// (`to`, `chat_id`, `topic`), and refuses the send when the
	// adapter declared none. A caller therefore cannot reach a
	// credential field through it.
	//
	// Empty means "the channel's own destination".
	Recipient string
}

// Delivery contains the resolved target metadata used for a send operation.
type Delivery struct {
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	TargetType string `json:"targetType"`
}

type channelTarget struct {
	ID             string
	Name           string
	ChannelType    string
	Method         string
	WebhookURL     string
	ServerURL      string
	Token          string
	Topic          string
	ChatID         string
	PhoneNumberID  string
	RecipientPhone string
	SenderNumber   string
	Recipients     string
	Username       string
	Password       string
	Priority       string
	Config         map[string]string
}

// NewDispatcher creates a notification dispatcher over the canonical transport tables.
func NewDispatcher(db *sql.DB, enc *encryption.Service) *Dispatcher {
	return &Dispatcher{db: db, enc: enc}
}

// SetPluginRuntime enables plugin-owned communication channel dispatch.
func (d *Dispatcher) SetPluginRuntime(runtime any) *Dispatcher {
	d.pluginRuntime = runtime
	return d
}

// Capabilities returns the configured notification capabilities available to workflows and other callers.
func (d *Dispatcher) Capabilities(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT DISTINCT channel_type
		 FROM communication_channels
		 WHERE enabled = 1
		 ORDER BY channel_type ASC
		 LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("listing communication channel capabilities: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close communication channel rows", "error", err)
		}
	}()

	capabilities := make(map[string]struct{})
	for rows.Next() {
		var capability string
		if err := rows.Scan(&capability); err != nil {
			return nil, fmt.Errorf("scanning communication capability: %w", err)
		}
		if capability == "" {
			continue
		}
		capabilities[capability] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating communication capabilities: %w", err)
	}

	capabilities["internal"] = struct{}{}

	items := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		items = append(items, capability)
	}
	sort.Strings(items)

	return items, nil
}

// SendChannel sends a notification through the requested or default configured communication channel.
func (d *Dispatcher) SendChannel(ctx context.Context, req ChannelRequest) (*Delivery, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, fmt.Errorf("notification message is required")
	}

	channelType := normalizeRequestedChannelType(req.ChannelType)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "McHarbor Notification"
	}

	if req.ChannelID == "" && channelType == "internal" {
		if err := inapp.CreateBroadcastContext(ctx, d.db, inapp.Record{
			Level:   "info",
			Title:   title,
			Message: message,
		}); err != nil {
			return nil, fmt.Errorf("creating in-app notification: %w", err)
		}

		return &Delivery{
			TargetID:   "internal",
			TargetName: "In-App Notifications",
			TargetType: "internal",
		}, nil
	}

	target, err := d.resolveChannelTarget(ctx, req.ChannelID, channelType, req.ChannelTypes...)
	if err != nil {
		return nil, err
	}
	client, spec, ok := d.lookupNotificationAdapter(target.ChannelType)
	if !ok {
		return nil, fmt.Errorf("notification adapter is not registered for channel type %s", target.ChannelType)
	}

	// The capability check lives here rather than in each adapter so
	// that eleven plugins do not each re-implement the same guard, and
	// so a plugin that forgot one cannot leak bytes to a provider that
	// cannot take them.
	files := req.Files
	if len(files) > 0 && !spec.Capabilities.SupportsFileAttachments {
		files = nil
	}

	cfg := target.Config
	if cfg == nil {
		cfg = map[string]string{}
	}
	applyPluginConfigAliases(target.ChannelType, cfg)
	// Overrides land last so they beat both the stored row and the
	// aliases derived from it. cfg is a fresh map per resolve, so this
	// cannot leak into the next send.
	for key, value := range req.ConfigOverrides {
		cfg[key] = value
	}
	// The recipient override lands after everything else and is the
	// only key a caller outside this process can move. A channel
	// whose adapter declared no overridable destination refuses the
	// send rather than delivering to the stored one: a report that
	// quietly went to the ops list instead of the board member who
	// was named is worse than a failure somebody can see.
	if recipient := strings.TrimSpace(req.Recipient); recipient != "" {
		key := strings.TrimSpace(spec.Capabilities.RecipientOverride.Key)
		if key == "" {
			return nil, fmt.Errorf(
				"channel type %s does not accept a recipient override", target.ChannelType)
		}
		if err := validateRecipientOverride(recipient); err != nil {
			return nil, err
		}
		cfg[key] = recipient
	}
	if err := client.Send(ctx, cfg, plugin.NotificationMessage{
		Title:   title,
		Message: message,
		Level:   "info",
		Files:   files,
	}); err != nil {
		return nil, err
	}

	return &Delivery{
		TargetID:   target.ID,
		TargetName: target.Name,
		TargetType: target.ChannelType,
	}, nil
}

// maxRecipientOverrideLen bounds an addressed override. Generous for a
// short comma-separated list of addresses or numbers, far below any
// provider's own limit.
const maxRecipientOverrideLen = 512

// validateRecipientOverride refuses what must never reach a provider's
// destination field.
//
// A newline is the whole attack: several of these adapters compose
// their request by writing the value into a header (an SMTP `To:`
// above all), and a value carrying CR or LF can add headers of its
// own - a second recipient, a different subject. Control characters
// are refused for the same reason one step earlier. The value is
// otherwise passed through verbatim: only the adapter knows whether
// its provider wants an address, a numeric chat id or a topic name,
// and a host-side format guess would reject valid destinations.
func validateRecipientOverride(v string) error {
	if len(v) > maxRecipientOverrideLen {
		return fmt.Errorf("recipient override is too long")
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("recipient override contains a control character")
		}
	}
	return nil
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

func (d *Dispatcher) resolveChannelTarget(ctx context.Context, channelID, channelType string, anyOf ...string) (*channelTarget, error) {
	channelType = normalizeRequestedChannelType(channelType)
	query := `SELECT id, name, channel_type, method, config_enc
	          FROM communication_channels
	          WHERE enabled = 1`
	args := []any{}
	switch {
	case channelID != "":
		query += ` AND id = ?`
		args = append(args, channelID)
	case channelType != "":
		query += ` AND channel_type = ? ORDER BY is_default DESC, name ASC LIMIT 1`
		args = append(args, channelType)
	case len(anyOf) > 0:
		query += ` AND channel_type IN (` + strings.TrimSuffix(strings.Repeat("?,", len(anyOf)), ",") + `) ORDER BY is_default DESC, name ASC LIMIT 1` // SQL identifier: only "?" markers are appended
		for _, t := range anyOf {
			args = append(args, normalizeRequestedChannelType(t))
		}
	default:
		query += ` ORDER BY is_default DESC, name ASC LIMIT 1`
	}

	var target channelTarget
	var configEnc sql.NullString

	err := d.db.QueryRowContext(ctx, query, args...).Scan(
		&target.ID,
		&target.Name,
		&target.ChannelType,
		&target.Method,
		&configEnc,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrChannelTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolving communication target: %w", err)
	}

	cfg, err := d.decryptConfig(configEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting communication config: %w", err)
	}
	target.WebhookURL = cfg["webhook_url"]
	target.ServerURL = cfg["server_url"]
	target.Token = cfg["token"]
	target.Topic = cfg["topic"]
	target.ChatID = cfg["chat_id"]
	target.PhoneNumberID = cfg["phone_number_id"]
	target.RecipientPhone = cfg["recipient_phone"]
	target.SenderNumber = cfg["sender_number"]
	target.Recipients = cfg["recipients"]
	target.Username = cfg["username"]
	target.Password = cfg["password"]
	target.Priority = cfg["priority"]
	target.Config = cfg

	return &target, nil
}

// lookupNotificationAdapter returns the adapter and its spec. The spec
// comes back because the caller needs the declared capabilities to
// decide what it may hand to Send.
func (d *Dispatcher) lookupNotificationAdapter(channelType string) (plugin.NotificationAdapterClient, plugin.NotificationAdapterSpec, bool) {
	if d.pluginRuntime == nil {
		return nil, plugin.NotificationAdapterSpec{}, false
	}
	source, ok := d.pluginRuntime.(plugin.NotificationAdapterSource)
	if !ok {
		return nil, plugin.NotificationAdapterSpec{}, false
	}
	client, spec, ok := source.NotificationAdapter(channelType)
	return client, spec, ok && client != nil
}

func normalizeRequestedChannelType(channelType string) string {
	normalized := strings.ToLower(strings.TrimSpace(channelType))
	switch normalized {
	case "":
		return ""
	case "alert", "in-app", "in_app", "inapp", "internal":
		return "internal"
	case "any", "default":
		return ""
	default:
		return normalized
	}
}

func (d *Dispatcher) decryptConfig(configEnc sql.NullString) (map[string]string, error) {
	if !configEnc.Valid || configEnc.String == "" {
		return map[string]string{}, nil
	}
	raw, err := d.enc.Decrypt(configEnc.String)
	if err != nil {
		return nil, err
	}
	cfg := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	return cfg, nil
}
