// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// defaultTeamsThemeColor is the Teams brand purple used for the
// Adaptive Card's left accent stripe. Operators override per
// channel via cfg["theme_color"] (without the leading #). Mirrors
// the value Teams renders for native notifications.
const defaultTeamsThemeColor = "5059C9"

// teamsHTTPTimeout caps a single POST round-trip. Teams
// connectors ack in ~300ms in healthy conditions; 10s gives
// plenty of slack without hanging the host on a wedged connector
// service — matches the other communication plugins.
const teamsHTTPTimeout = 10 * time.Second

// teamsClient is the NotificationAdapterClient the host dispatches
// to for the Teams ChannelType. Stateless: every method takes the
// channel row's config map directly.
//
// Concurrency: NotificationAdapterClient implementations must be
// safe for concurrent use — the host invokes Test + Send from
// multiple goroutines during workflow fan-out. teamsClient has no
// shared state so this is automatic.
type teamsClient struct{}

// NewTeamsClient returns a fresh client. The plugin keeps no
// shared state; callers can construct as many as they want.
func NewTeamsClient() *teamsClient {
	return &teamsClient{}
}

// ============================================================================
// NotificationAdapterClient: Test
// ============================================================================

// Test verifies webhook_url is reachable and accepts an Adaptive
// Card payload. We POST a minimal ping card with a short
// Title + Message and let Teams' 200 OK (or 4xx with a plain-text
// error body) tell us whether the connector is live.
//
// We intentionally post a real card — Teams connectors have no
// "validate token" endpoint, so the cheapest liveness check is a
// publish. The card lands in the channel as a visible "McHarbor
// test" message, which is the operator's visual confirmation.
//
// The Message body is fixed ("This is a test message from
// McHarbor.") so subscribers recognise and ignore it.
func (c *teamsClient) Test(ctx context.Context, cfg map[string]string) error {
	endpoint, err := teamsWebhookURL(cfg)
	if err != nil {
		return err
	}

	payload := teamsPayload("McHarbor test notification", "This is a test message from McHarbor.", cfg)

	resp, err := teamsPost(ctx, endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("comm-teams: webhook returned %d: %s", resp.StatusCode, truncateTeamsBody(respBody, 256))
	}
	return nil
}

// ============================================================================
// NotificationAdapterClient: Send
// ============================================================================

// Send delivers msg to the configured Teams channel. Builds an
// Adaptive Card with Title as a bold TextBlock header and Message
// as a wrapping TextBlock body, then POSTs to the webhook URL.
//
// Teams' MessageCard shape (legacy but still supported by every
// connector in 2025+):
//
//	{
//	  "type": "message",
//	  "attachments": [{
//	    "contentType": "application/vnd.microsoft.card.adaptive",
//	    "content": {
//	      "$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
//	      "type":    "AdaptiveCard",
//	      "version": "1.4",
//	      "themeColor": "5059C9",
//	      "body": [
//	        { "type": "TextBlock", "text": title, "weight": "Bolder", "size": "Medium" },
//	        { "type": "TextBlock", "text": message, "wrap": true }
//	      ]
//	    }
//	  }]
//	}
//
// Title is sent as a TextBlock (Bolder/Medium) so it shows up
// prominently even when the host doesn't render the Teams
// preview pane. The legacy MessageCard wrapper is intentional —
// it works for every Teams connector in production today, and
// newer "raw" Adaptive Card POSTs require a different
// contentType. Mirrors the host's built-in SendTeams byte-for-byte.
//
// Attachments are silently ignored — the plugin reports
// SupportsRichAttachments=false so the host should never call
// Send with a non-empty Attachments slice. We still defensively
// skip them rather than returning an error.
func (c *teamsClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	endpoint, err := teamsWebhookURL(cfg)
	if err != nil {
		return err
	}
	if msg.Title == "" && msg.Message == "" {
		return errors.New("comm-teams: empty payload (title and message both blank)")
	}

	payload := teamsPayload(msg.Title, msg.Message, cfg)

	resp, err := teamsPost(ctx, endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("comm-teams: webhook returned %d: %s", resp.StatusCode, truncateTeamsBody(respBody, 256))
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// teamsWebhookURL validates + returns the configured Teams
// Incoming Webhook URL. Empty string fails fast with a
// user-facing error so the operator can fix the field rather
// than chasing a generic 4xx from Teams.
func teamsWebhookURL(cfg map[string]string) (string, error) {
	url := strings.TrimSpace(cfg["webhook_url"])
	if url == "" {
		return "", errors.New("comm-teams: webhook URL is required before testing — paste the Incoming Webhook connector URL from Teams")
	}
	return url, nil
}

// teamsPayload builds the Adaptive Card MessageCard that Teams
// renders inside the channel. Title becomes a bold TextBlock
// header; Message becomes a wrapping TextBlock body. When Title
// is blank we omit the header so the card shows just the message
// text (still wrapped).
//
// themeColor comes from cfg["theme_color"] when present, else
// falls back to Teams brand purple. The value must be a 6-char
// hex string without the leading # — Adaptive Cards don't
// accept the # prefix.
func teamsPayload(title, message string, cfg map[string]string) map[string]any {
	body := make([]map[string]any, 0, 2)

	if title = strings.TrimSpace(title); title != "" {
		body = append(body, map[string]any{
			"type":   "TextBlock",
			"text":   title,
			"weight": "Bolder",
			"size":   "Medium",
		})
	}

	if message = strings.TrimSpace(message); message != "" {
		body = append(body, map[string]any{
			"type": "TextBlock",
			"text": message,
			"wrap": true,
		})
	}

	themeColor := strings.TrimSpace(cfg["theme_color"])
	if themeColor == "" {
		themeColor = defaultTeamsThemeColor
	}
	// Adaptive Cards accept hex without the leading "#".
	themeColor = strings.TrimPrefix(themeColor, "#")

	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"$schema":    "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":       "AdaptiveCard",
					"version":    "1.4",
					"themeColor": themeColor,
					"body":       body,
				},
			},
		},
	}
}

// teamsPost marshals the Adaptive Card as JSON and POSTs it to
// the webhook URL with Content-Type: application/json. Centralises
// header construction so Test and Send emit identical requests —
// easier to debug from the operator's "what did McHarbor actually
// send" angle.
func teamsPost(ctx context.Context, endpoint string, payload map[string]any) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("comm-teams: marshal adaptive card: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("comm-teams: build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: teamsHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("comm-teams: post to webhook: %w", err)
	}
	return resp, nil
}

// truncateTeamsBody clamps an error body to `max` bytes so a
// multi-KB Teams error response doesn't leak into logs or user-
// facing messages. Mirrors the same helper in comm-ntfy and
// comm-telegram's clients.
func truncateTeamsBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}
