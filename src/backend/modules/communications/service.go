// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/notify"
)

// Service handles communication channel business logic and database operations.
type Service struct {
	db  *sql.DB
	enc *encryption.Service
}

// NewService creates a new communication channel service.
func NewService(database *sql.DB, enc *encryption.Service) *Service {
	return &Service{db: database, enc: enc}
}

// List returns all communication channels (secrets excluded).
func (s *Service) List(ctx context.Context) ([]CommunicationChannel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, channel_type, method, is_default, enabled, server_url, topic,
		        chat_id, phone_number_id, recipient_phone, sender_number,
		        recipients, username, priority, created_at, updated_at
		 FROM communication_channels ORDER BY name ASC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing communication channels: %w", err)
	}
	defer rows.Close()

	var items []CommunicationChannel
	for rows.Next() {
		ch, err := scanChannel(rows)
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
	var ch CommunicationChannel
	var serverURL, topic, chatID, phoneNumberID, recipientPhone sql.NullString
	var senderNumber, recipients, username, priority sql.NullString
	var isDefault, enabled sql.NullBool

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, channel_type, method, is_default, enabled, server_url, topic,
		        chat_id, phone_number_id, recipient_phone, sender_number,
		        recipients, username, priority, created_at, updated_at
		 FROM communication_channels WHERE id = ?`, id,
	).Scan(&ch.ID, &ch.Name, &ch.ChannelType, &ch.Method, &isDefault, &enabled,
		&serverURL, &topic, &chatID, &phoneNumberID, &recipientPhone,
		&senderNumber, &recipients, &username, &priority,
		&ch.CreatedAt, &ch.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting communication channel %s: %w", id, err)
	}

	ch.IsDefault = isDefault.Bool
	ch.Enabled = enabled.Bool
	ch.ServerURL = serverURL.String
	ch.Topic = topic.String
	ch.ChatID = chatID.String
	ch.PhoneNumberID = phoneNumberID.String
	ch.RecipientPhone = recipientPhone.String
	ch.SenderNumber = senderNumber.String
	ch.Recipients = recipients.String
	ch.Username = username.String
	ch.Priority = priority.String

	return &ch, nil
}

// Create inserts a new communication channel, encrypting sensitive fields.
func (s *Service) Create(ctx context.Context, input CreateChannelInput) (*CommunicationChannel, error) {
	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var encWebhookURL, encToken, encPassword string
	var err error

	if input.WebhookURL != "" {
		encWebhookURL, err = s.enc.Encrypt(input.WebhookURL)
		if err != nil {
			return nil, fmt.Errorf("encrypting webhook url: %w", err)
		}
	}

	if input.Token != "" {
		encToken, err = s.enc.Encrypt(input.Token)
		if err != nil {
			return nil, fmt.Errorf("encrypting token: %w", err)
		}
	}

	if input.Password != "" {
		encPassword, err = s.enc.Encrypt(input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting password: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
			return nil, fmt.Errorf("clearing existing default: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO communication_channels (id, name, channel_type, method, is_default, enabled,
		 webhook_url, server_url, token, topic, chat_id, phone_number_id, recipient_phone,
		 sender_number, recipients, username, password, priority, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.ChannelType, input.Method, input.IsDefault,
		nullStr(encWebhookURL), nullStr(input.ServerURL),
		nullStr(encToken), nullStr(input.Topic),
		nullStr(input.ChatID), nullStr(input.PhoneNumberID),
		nullStr(input.RecipientPhone), nullStr(input.SenderNumber),
		nullStr(input.Recipients), nullStr(input.Username),
		nullStr(encPassword), nullStr(input.Priority),
		now, now,
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
		ServerURL:      input.ServerURL,
		Topic:          input.Topic,
		ChatID:         input.ChatID,
		PhoneNumberID:  input.PhoneNumberID,
		RecipientPhone: input.RecipientPhone,
		SenderNumber:   input.SenderNumber,
		Recipients:     input.Recipients,
		Username:       input.Username,
		Priority:       input.Priority,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Update applies partial updates to a communication channel.
func (s *Service) Update(ctx context.Context, id string, input UpdateChannelInput) (*CommunicationChannel, error) {
	var existsID string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM communication_channels WHERE id = ?", id).Scan(&existsID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("checking communication channel existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

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
	if input.WebhookURL != nil && *input.WebhookURL != "" {
		enc, err := s.enc.Encrypt(*input.WebhookURL)
		if err != nil {
			return nil, fmt.Errorf("encrypting webhook url: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET webhook_url = ?, updated_at = ? WHERE id = ?", enc, now, id); err != nil {
			return nil, fmt.Errorf("updating channel webhook url: %w", err)
		}
	}
	if input.ServerURL != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET server_url = ?, updated_at = ? WHERE id = ?", *input.ServerURL, now, id); err != nil {
			return nil, fmt.Errorf("updating channel server url: %w", err)
		}
	}
	if input.Token != nil && *input.Token != "" {
		enc, err := s.enc.Encrypt(*input.Token)
		if err != nil {
			return nil, fmt.Errorf("encrypting token: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET token = ?, updated_at = ? WHERE id = ?", enc, now, id); err != nil {
			return nil, fmt.Errorf("updating channel token: %w", err)
		}
	}
	if input.Topic != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET topic = ?, updated_at = ? WHERE id = ?", *input.Topic, now, id); err != nil {
			return nil, fmt.Errorf("updating channel topic: %w", err)
		}
	}
	if input.ChatID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET chat_id = ?, updated_at = ? WHERE id = ?", *input.ChatID, now, id); err != nil {
			return nil, fmt.Errorf("updating channel chat id: %w", err)
		}
	}
	if input.PhoneNumberID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET phone_number_id = ?, updated_at = ? WHERE id = ?", *input.PhoneNumberID, now, id); err != nil {
			return nil, fmt.Errorf("updating channel phone number id: %w", err)
		}
	}
	if input.RecipientPhone != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET recipient_phone = ?, updated_at = ? WHERE id = ?", *input.RecipientPhone, now, id); err != nil {
			return nil, fmt.Errorf("updating channel recipient phone: %w", err)
		}
	}
	if input.SenderNumber != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET sender_number = ?, updated_at = ? WHERE id = ?", *input.SenderNumber, now, id); err != nil {
			return nil, fmt.Errorf("updating channel sender number: %w", err)
		}
	}
	if input.Recipients != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET recipients = ?, updated_at = ? WHERE id = ?", *input.Recipients, now, id); err != nil {
			return nil, fmt.Errorf("updating channel recipients: %w", err)
		}
	}
	if input.Username != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET username = ?, updated_at = ? WHERE id = ?", *input.Username, now, id); err != nil {
			return nil, fmt.Errorf("updating channel username: %w", err)
		}
	}
	if input.Password != nil && *input.Password != "" {
		enc, err := s.enc.Encrypt(*input.Password)
		if err != nil {
			return nil, fmt.Errorf("encrypting password: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET password = ?, updated_at = ? WHERE id = ?", enc, now, id); err != nil {
			return nil, fmt.Errorf("updating channel password: %w", err)
		}
	}
	if input.Priority != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET priority = ?, updated_at = ? WHERE id = ?", *input.Priority, now, id); err != nil {
			return nil, fmt.Errorf("updating channel priority: %w", err)
		}
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
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 0, updated_at = ? WHERE is_default = 1", now); err != nil {
		return fmt.Errorf("clearing existing default: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE communication_channels SET is_default = 1, updated_at = ? WHERE id = ?", now, id); err != nil {
		return fmt.Errorf("setting new default: %w", err)
	}

	return tx.Commit()
}

// Test sends a test notification using the specified channel.
func (s *Service) Test(ctx context.Context, id string) error {
	var channelType, method, webhookURL, serverURL, token, topic sql.NullString
	var chatID, phoneNumberID, recipientPhone, senderNumber sql.NullString
	var recipients, username, password, priority sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT channel_type, method, webhook_url, server_url, token, topic, chat_id,
		        phone_number_id, recipient_phone, sender_number, recipients,
		        username, password, priority
		 FROM communication_channels WHERE id = ?`, id,
	).Scan(&channelType, &method, &webhookURL, &serverURL, &token, &topic, &chatID,
		&phoneNumberID, &recipientPhone, &senderNumber, &recipients,
		&username, &password, &priority)

	if err == sql.ErrNoRows {
		return fmt.Errorf("communication channel not found")
	}
	if err != nil {
		return fmt.Errorf("looking up communication channel %s: %w", id, err)
	}

	title := "McHarbor Test Notification"
	message := "This is a test notification sent from McHarbor to verify your channel configuration."

	switch channelType.String {
	case "slack":
		decURL, err := s.decryptField(webhookURL.String)
		if err != nil {
			return fmt.Errorf("decrypting webhook url: %w", err)
		}
		return notify.SendSlack(ctx, notify.SlackConfig{WebhookURL: decURL}, title, message)

	case "discord":
		decURL, err := s.decryptField(webhookURL.String)
		if err != nil {
			return fmt.Errorf("decrypting webhook url: %w", err)
		}
		return notify.SendDiscord(ctx, notify.DiscordConfig{WebhookURL: decURL}, title, message)

	case "teams":
		decURL, err := s.decryptField(webhookURL.String)
		if err != nil {
			return fmt.Errorf("decrypting webhook url: %w", err)
		}
		return notify.SendTeams(ctx, notify.TeamsConfig{WebhookURL: decURL}, title, message)

	case "gotify":
		decToken, err := s.decryptField(token.String)
		if err != nil {
			return fmt.Errorf("decrypting token: %w", err)
		}
		return notify.SendGotify(ctx, notify.GotifyConfig{
			ServerURL: serverURL.String,
			AppToken:  decToken,
			Priority:  priority.String,
		}, title, message)

	case "ntfy":
		decToken, _ := s.decryptField(token.String)
		decPassword, _ := s.decryptField(password.String)
		return notify.SendNtfy(ctx, notify.NtfyConfig{
			ServerURL:   serverURL.String,
			Topic:       topic.String,
			AccessToken: decToken,
			Username:    username.String,
			Password:    decPassword,
			Priority:    priority.String,
		}, title, message)

	case "telegram":
		decToken, err := s.decryptField(token.String)
		if err != nil {
			return fmt.Errorf("decrypting token: %w", err)
		}
		return notify.SendTelegram(ctx, notify.TelegramConfig{
			BotToken: decToken,
			ChatID:   chatID.String,
		}, title, message)

	case "signal":
		recipientList := strings.Split(recipients.String, ",")
		for i := range recipientList {
			recipientList[i] = strings.TrimSpace(recipientList[i])
		}
		switch method.String {
		case "bot":
			decToken, err := s.decryptField(token.String)
			if err != nil {
				return fmt.Errorf("decrypting token: %w", err)
			}
			return notify.SendSignalBot(ctx, notify.SignalBotConfig{
				ServerURL:  serverURL.String,
				Token:      decToken,
				Recipients: recipientList,
			}, title, message)
		case "signald":
			return notify.SendSignalD(ctx, notify.SignalDConfig{
				ServerURL:    serverURL.String,
				SenderNumber: senderNumber.String,
				Recipients:   recipientList,
			}, title, message)
		default: // "", "rest_api", "simple"
			decPassword, _ := s.decryptField(password.String)
			return notify.SendSignal(ctx, notify.SignalConfig{
				ServerURL:    serverURL.String,
				SenderNumber: senderNumber.String,
				Recipients:   recipientList,
				Username:     username.String,
				Password:     decPassword,
			}, title, message)
		}

	case "whatsapp":
		decToken, err := s.decryptField(token.String)
		if err != nil {
			return fmt.Errorf("decrypting token: %w", err)
		}
		switch method.String {
		case "gateway", "saas":
			return notify.SendWhatsAppGateway(ctx, notify.WhatsAppGatewayConfig{
				ServerURL:      serverURL.String,
				Token:          decToken,
				RecipientPhone: recipientPhone.String,
			}, title, message)
		case "business":
			return notify.SendWhatsAppBusiness(ctx, notify.WhatsAppBusinessConfig{
				ServerURL:      serverURL.String,
				PhoneNumberID:  phoneNumberID.String,
				Token:          decToken,
				RecipientPhone: recipientPhone.String,
			}, title, message)
		default: // "", "cloud_api"
			return notify.SendWhatsApp(ctx, notify.WhatsAppConfig{
				PhoneNumberID:  phoneNumberID.String,
				AccessToken:    decToken,
				RecipientPhone: recipientPhone.String,
			}, title, message)
		}

	default:
		return fmt.Errorf("unsupported channel type: %s", channelType.String)
	}
}

func (s *Service) decryptField(val string) (string, error) {
	if val == "" {
		return "", nil
	}
	return s.enc.Decrypt(val)
}

func nullStr(str string) sql.NullString {
	if str == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: str, Valid: true}
}

func scanChannel(rows *sql.Rows) (CommunicationChannel, error) {
	var ch CommunicationChannel
	var serverURL, topic, chatID, phoneNumberID, recipientPhone sql.NullString
	var senderNumber, recipients, username, priority sql.NullString
	var isDefault, enabled sql.NullBool

	if err := rows.Scan(&ch.ID, &ch.Name, &ch.ChannelType, &ch.Method, &isDefault, &enabled,
		&serverURL, &topic, &chatID, &phoneNumberID, &recipientPhone,
		&senderNumber, &recipients, &username, &priority,
		&ch.CreatedAt, &ch.UpdatedAt); err != nil {
		return CommunicationChannel{}, err
	}

	ch.IsDefault = isDefault.Bool
	ch.Enabled = enabled.Bool
	ch.ServerURL = serverURL.String
	ch.Topic = topic.String
	ch.ChatID = chatID.String
	ch.PhoneNumberID = phoneNumberID.String
	ch.RecipientPhone = recipientPhone.String
	ch.SenderNumber = senderNumber.String
	ch.Recipients = recipients.String
	ch.Username = username.String
	ch.Priority = priority.String

	return ch, nil
}
