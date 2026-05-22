// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"

	coreemail "github.com/therealmcsparrow/mcharbor/core/email"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
)

var (
	// ErrEmailTargetNotFound indicates that no enabled email server could be resolved.
	ErrEmailTargetNotFound = errors.New("email target not found")
	// ErrChannelTargetNotFound indicates that no enabled communication channel could be resolved.
	ErrChannelTargetNotFound = errors.New("communication channel target not found")
)

// Dispatcher resolves canonical email servers / communication channels and sends messages through them.
type Dispatcher struct {
	db  *sql.DB
	enc *encryption.Service
}

// EmailRequest describes an outbound email to be sent through a configured email server.
type EmailRequest struct {
	ServerID string
	To       string
	Subject  string
	Body     string
}

// ChannelRequest describes an outbound notification to be sent through a configured communication channel.
type ChannelRequest struct {
	ChannelID   string
	ChannelType string
	Title       string
	Message     string
}

// Delivery contains the resolved target metadata used for a send operation.
type Delivery struct {
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	TargetType string `json:"targetType"`
}

type emailTarget struct {
	ID           string
	Name         string
	ServerType   string
	Host         string
	Port         int
	Encryption   string
	AuthMethod   string
	Username     string
	Password     string
	ClientID     string
	ClientSecret string
	TenantID     string
	FromAddress  string
	FromName     string
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
}

// NewDispatcher creates a notification dispatcher over the canonical transport tables.
func NewDispatcher(db *sql.DB, enc *encryption.Service) *Dispatcher {
	return &Dispatcher{db: db, enc: enc}
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
	defer rows.Close()

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

	var emailCount int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM email_servers
		 WHERE enabled = 1`).Scan(&emailCount); err != nil {
		return nil, fmt.Errorf("counting email capabilities: %w", err)
	}
	if emailCount > 0 {
		capabilities["email"] = struct{}{}
	}
	capabilities["internal"] = struct{}{}

	items := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		items = append(items, capability)
	}
	sort.Strings(items)

	return items, nil
}

// SendEmail sends an email through the requested or default configured email server.
func (d *Dispatcher) SendEmail(ctx context.Context, req EmailRequest) (*Delivery, error) {
	if strings.TrimSpace(req.To) == "" {
		return nil, fmt.Errorf("email recipient is required")
	}

	target, err := d.resolveEmailTarget(ctx, req.ServerID)
	if err != nil {
		return nil, err
	}

	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "McHarbor Notification"
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = htmlParagraph(subject)
	}

	switch target.ServerType {
	case "smtp":
		err = coreemail.SendSMTP(ctx, coreemail.SMTPConfig{
			Host:        target.Host,
			Port:        target.Port,
			Encryption:  target.Encryption,
			AuthMethod:  target.AuthMethod,
			Username:    target.Username,
			Password:    target.Password,
			FromAddress: target.FromAddress,
			FromName:    target.FromName,
		}, req.To, subject, body)

	case "exchange":
		err = coreemail.SendExchange(ctx, coreemail.OAuthConfig{
			ClientID:     target.ClientID,
			ClientSecret: target.ClientSecret,
			TenantID:     target.TenantID,
			FromAddress:  target.FromAddress,
			FromName:     target.FromName,
		}, req.To, subject, body)

	case "gmail":
		err = coreemail.SendGmail(ctx, coreemail.OAuthConfig{
			ClientID:     target.ClientID,
			ClientSecret: target.ClientSecret,
			FromAddress:  target.FromAddress,
			FromName:     target.FromName,
		}, req.To, subject, body)

	default:
		return nil, fmt.Errorf("unsupported email server type: %s", target.ServerType)
	}
	if err != nil {
		return nil, err
	}

	return &Delivery{
		TargetID:   target.ID,
		TargetName: target.Name,
		TargetType: "email",
	}, nil
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
		if err := inapp.CreateBroadcast(d.db, inapp.Record{
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

	target, err := d.resolveChannelTarget(ctx, req.ChannelID, channelType)
	if err != nil {
		return nil, err
	}

	switch target.ChannelType {
	case "slack":
		err = SendSlack(ctx, SlackConfig{WebhookURL: target.WebhookURL}, title, message)
	case "discord":
		err = SendDiscord(ctx, DiscordConfig{WebhookURL: target.WebhookURL}, title, message)
	case "teams":
		err = SendTeams(ctx, TeamsConfig{WebhookURL: target.WebhookURL}, title, message)
	case "gotify":
		err = SendGotify(ctx, GotifyConfig{
			ServerURL: target.ServerURL,
			AppToken:  target.Token,
			Priority:  target.Priority,
		}, title, message)
	case "ntfy":
		err = SendNtfy(ctx, NtfyConfig{
			ServerURL:   target.ServerURL,
			Topic:       target.Topic,
			AccessToken: target.Token,
			Username:    target.Username,
			Password:    target.Password,
			Priority:    target.Priority,
		}, title, message)
	case "telegram":
		err = SendTelegram(ctx, TelegramConfig{
			BotToken: target.Token,
			ChatID:   target.ChatID,
		}, title, message)
	case "signal":
		recipients := splitRecipients(target.Recipients)
		switch target.Method {
		case "bot":
			err = SendSignalBot(ctx, SignalBotConfig{
				ServerURL:  target.ServerURL,
				Token:      target.Token,
				Recipients: recipients,
			}, title, message)
		case "signald":
			err = SendSignalD(ctx, SignalDConfig{
				ServerURL:    target.ServerURL,
				SenderNumber: target.SenderNumber,
				Recipients:   recipients,
			}, title, message)
		default:
			err = SendSignal(ctx, SignalConfig{
				ServerURL:    target.ServerURL,
				SenderNumber: target.SenderNumber,
				Recipients:   recipients,
				Username:     target.Username,
				Password:     target.Password,
			}, title, message)
		}
	case "whatsapp":
		switch target.Method {
		case "gateway", "saas":
			err = SendWhatsAppGateway(ctx, WhatsAppGatewayConfig{
				ServerURL:      target.ServerURL,
				Token:          target.Token,
				RecipientPhone: target.RecipientPhone,
			}, title, message)
		case "business":
			err = SendWhatsAppBusiness(ctx, WhatsAppBusinessConfig{
				ServerURL:      target.ServerURL,
				PhoneNumberID:  target.PhoneNumberID,
				Token:          target.Token,
				RecipientPhone: target.RecipientPhone,
			}, title, message)
		default:
			err = SendWhatsApp(ctx, WhatsAppConfig{
				PhoneNumberID:  target.PhoneNumberID,
				AccessToken:    target.Token,
				RecipientPhone: target.RecipientPhone,
			}, title, message)
		}
	default:
		return nil, fmt.Errorf("unsupported communication channel type: %s", target.ChannelType)
	}
	if err != nil {
		return nil, err
	}

	return &Delivery{
		TargetID:   target.ID,
		TargetName: target.Name,
		TargetType: target.ChannelType,
	}, nil
}

func (d *Dispatcher) resolveEmailTarget(ctx context.Context, serverID string) (*emailTarget, error) {
	query := `SELECT id, name, server_type, host, port, encryption, auth_method, username,
	                 password, client_id, client_secret, tenant_id, from_address, from_name
	          FROM email_servers
	          WHERE enabled = 1`
	args := []any{}
	if serverID != "" {
		query += ` AND id = ?`
		args = append(args, serverID)
	} else {
		query += ` ORDER BY is_default DESC, name ASC LIMIT 1`
	}

	var target emailTarget
	var host, encryptionMode, authMethod, username, password sql.NullString
	var clientID, clientSecret, tenantID, fromName sql.NullString
	var port sql.NullInt64

	err := d.db.QueryRowContext(ctx, query, args...).Scan(
		&target.ID,
		&target.Name,
		&target.ServerType,
		&host,
		&port,
		&encryptionMode,
		&authMethod,
		&username,
		&password,
		&clientID,
		&clientSecret,
		&tenantID,
		&target.FromAddress,
		&fromName,
	)
	if err == sql.ErrNoRows {
		return nil, ErrEmailTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolving email target: %w", err)
	}

	target.Host = host.String
	target.Port = int(port.Int64)
	target.Encryption = encryptionMode.String
	target.AuthMethod = authMethod.String
	target.Username = username.String
	target.ClientID = clientID.String
	target.TenantID = tenantID.String
	target.FromName = fromName.String

	target.Password, err = d.decryptField(password.String)
	if err != nil {
		return nil, fmt.Errorf("decrypting email password: %w", err)
	}
	target.ClientSecret, err = d.decryptField(clientSecret.String)
	if err != nil {
		return nil, fmt.Errorf("decrypting email client secret: %w", err)
	}

	return &target, nil
}

func (d *Dispatcher) resolveChannelTarget(ctx context.Context, channelID, channelType string) (*channelTarget, error) {
	channelType = normalizeRequestedChannelType(channelType)
	query := `SELECT id, name, channel_type, method, webhook_url, server_url, token, topic, chat_id,
	                 phone_number_id, recipient_phone, sender_number, recipients, username, password, priority
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
	default:
		query += ` ORDER BY is_default DESC, name ASC LIMIT 1`
	}

	var target channelTarget
	var webhookURL, serverURL, token, topic, chatID sql.NullString
	var phoneNumberID, recipientPhone, senderNumber sql.NullString
	var recipients, username, password, priority sql.NullString

	err := d.db.QueryRowContext(ctx, query, args...).Scan(
		&target.ID,
		&target.Name,
		&target.ChannelType,
		&target.Method,
		&webhookURL,
		&serverURL,
		&token,
		&topic,
		&chatID,
		&phoneNumberID,
		&recipientPhone,
		&senderNumber,
		&recipients,
		&username,
		&password,
		&priority,
	)
	if err == sql.ErrNoRows {
		return nil, ErrChannelTargetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolving communication target: %w", err)
	}

	target.ServerURL = serverURL.String
	target.Topic = topic.String
	target.ChatID = chatID.String
	target.PhoneNumberID = phoneNumberID.String
	target.RecipientPhone = recipientPhone.String
	target.SenderNumber = senderNumber.String
	target.Recipients = recipients.String
	target.Username = username.String
	target.Priority = priority.String

	target.WebhookURL, err = d.decryptField(webhookURL.String)
	if err != nil {
		return nil, fmt.Errorf("decrypting communication webhook url: %w", err)
	}
	target.Token, err = d.decryptField(token.String)
	if err != nil {
		return nil, fmt.Errorf("decrypting communication token: %w", err)
	}
	target.Password, err = d.decryptField(password.String)
	if err != nil {
		return nil, fmt.Errorf("decrypting communication password: %w", err)
	}

	return &target, nil
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

func (d *Dispatcher) decryptField(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	return d.enc.Decrypt(value)
}

func splitRecipients(value string) []string {
	parts := strings.Split(value, ",")
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		recipients = append(recipients, part)
	}
	return recipients
}

func htmlParagraph(message string) string {
	return "<p>" + html.EscapeString(message) + "</p>"
}
