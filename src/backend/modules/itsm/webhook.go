// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package itsm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// ErrWebhookRejected is the single answer to every inbound failure, so the
// public route cannot be used to learn which tokens or channels exist.
var ErrWebhookRejected = errors.New("itsm: webhook rejected")

// maxWebhookBody bounds an inbound payload.
const maxWebhookBody = 1 << 20

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("itsm: generating token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// RotateResult is what a rotation returns, once: the inbound URL and the
// host-generated signing secret. Neither is shown again.
type RotateResult struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// RotateToken issues a new inbound token and signing secret for a channel,
// recording baseURL as how the ITSM reaches McHarbor.
func (s *Service) RotateToken(ctx context.Context, channelID, baseURL string) (*RotateResult, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	secret, err := randomToken()
	if err != nil {
		return nil, err
	}
	tokenEnc, err := s.enc.Encrypt(token)
	if err != nil {
		return nil, fmt.Errorf("itsm: encrypting token: %w", err)
	}
	secretEnc, err := s.enc.Encrypt(secret)
	if err != nil {
		return nil, fmt.Errorf("itsm: encrypting secret: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE communication_channels SET webhook_token_hash = ?, webhook_token_enc = ?,
		webhook_secret_enc = ?, webhook_base_url = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		tokenHash(token), tokenEnc, secretEnc, baseURL, channelID)
	if err != nil {
		return nil, fmt.Errorf("itsm: storing token: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return nil, sql.ErrNoRows
	}
	return &RotateResult{URL: strings.TrimRight(baseURL, "/") + "/api/incident-webhooks/" + token, Secret: secret}, nil
}

// SetVendorSecret stores a signing secret the vendor generated (PagerDuty,
// incident.io, Rootly), replacing the host-generated one.
func (s *Service) SetVendorSecret(ctx context.Context, channelID, secret string) error {
	enc, err := s.enc.Encrypt(secret)
	if err != nil {
		return fmt.Errorf("itsm: encrypting secret: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE communication_channels SET webhook_secret_enc = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		enc, channelID)
	if err != nil {
		return fmt.Errorf("itsm: storing secret: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Receive handles one inbound call: the token names the channel, the plugin
// authenticates the request, and each status it parses is applied. Every
// failure is ErrWebhookRejected. It returns the channel and how many statuses
// were applied, for the audit row.
func (s *Service) Receive(ctx context.Context, token string, header http.Header, body []byte) (string, int, error) {
	if token == "" || len(token) > 200 {
		return "", 0, ErrWebhookRejected
	}
	var channelID, channelType string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, channel_type FROM communication_channels WHERE webhook_token_hash = ? AND enabled = 1`, tokenHash(token),
	).Scan(&channelID, &channelType)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			s.logger.Error("itsm: webhook lookup failed", "error", err)
		}
		return "", 0, ErrWebhookRejected
	}
	parser, ok := s.disp.WebhookParser(channelType)
	if !ok {
		return "", 0, ErrWebhookRejected
	}
	_, cfg, err := s.disp.ChannelConfig(ctx, channelID)
	if err != nil {
		return "", 0, ErrWebhookRejected
	}
	cfg, err = s.withHostKeys(ctx, channelID, cfg)
	if err != nil {
		s.logger.Error("itsm: webhook config failed", "channel", channelID, "error", err)
		return "", 0, ErrWebhookRejected
	}
	if err := parser.VerifyWebhook(cfg, header, body); err != nil {
		s.logger.Warn("itsm: webhook verification failed", "channel", channelID, "error", err)
		return "", 0, ErrWebhookRejected
	}
	statuses, err := parser.ParseWebhook(ctx, cfg, header, body)
	if err != nil {
		s.logger.Warn("itsm: webhook parse failed", "channel", channelID, "error", err)
		return channelID, 0, nil
	}
	applied := 0
	for _, st := range statuses {
		ok, err := s.ApplyRemote(ctx, channelID, st)
		if err != nil {
			s.logger.Error("itsm: applying webhook status failed", "channel", channelID, "error", err)
			continue
		}
		if ok {
			applied++
		}
	}
	return channelID, applied, nil
}

// ChannelInfo is an incident-capable channel's webhook set-up, without secrets.
type ChannelInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ChannelType      string `json:"channelType"`
	WebhookSupported bool   `json:"webhookSupported"`
	PollSupported    bool   `json:"pollSupported"`
	TokenSet         bool   `json:"tokenSet"`
	SecretSet        bool   `json:"secretSet"`
	BaseURL          string `json:"baseUrl,omitempty"`
}

// Channels lists the channels whose plugin can open tickets.
func (s *Service) Channels(ctx context.Context) ([]ChannelInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, channel_type, webhook_token_hash <> '', webhook_secret_enc <> '', webhook_base_url
		FROM communication_channels ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("itsm: listing channels: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("closing rows failed", "error", cerr)
		}
	}()
	out := []ChannelInfo{}
	for rows.Next() {
		var c ChannelInfo
		if err := rows.Scan(&c.ID, &c.Name, &c.ChannelType, &c.TokenSet, &c.SecretSet, &c.BaseURL); err != nil {
			return nil, fmt.Errorf("itsm: scanning channel: %w", err)
		}
		if !s.Supports(c.ChannelType) {
			continue
		}
		_, c.WebhookSupported = s.disp.WebhookParser(c.ChannelType)
		_, c.PollSupported = s.disp.Poller(c.ChannelType)
		out = append(out, c)
	}
	return out, rows.Err()
}
