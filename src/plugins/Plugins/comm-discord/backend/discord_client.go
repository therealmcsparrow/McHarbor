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
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const discordTimeout = 10 * time.Second

// discordUploadTimeout is longer than discordTimeout: an upload carries
// a report-sized body over the same connection and 10s is not enough on
// a slow link.
const discordUploadTimeout = 60 * time.Second

type discordClient struct{}

type discordConfig struct {
	WebhookURL string
}

// NewDiscordClient returns a stateless NotificationAdapterClient.
func NewDiscordClient() *discordClient { return &discordClient{} }

// Test sends a real test message to the configured Discord webhook.
func (c *discordClient) Test(ctx context.Context, cfg map[string]string) error {
	parsed, err := parseDiscordConfig(cfg)
	if err != nil {
		return err
	}
	msg := plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification from the McHarbor Discord communication channel.",
		Level:   "info",
	}
	return c.Send(ctx, configMapFromDiscordConfig(parsed), msg)
}

// Send delivers the notification through a Discord webhook.
func (c *discordClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	parsed, err := parseDiscordConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comm-discord: empty payload (title and message both blank)")
	}

	content := buildDiscordContent(msg)
	if len(msg.Files) > 0 {
		return postDiscordMultipart(ctx, parsed.WebhookURL, content, msg.Files)
	}
	return postDiscord(ctx, parsed.WebhookURL, map[string]string{"content": content})
}

// postDiscordMultipart uploads files alongside the message.
//
// Discord takes the JSON body as a form field named "payload_json" and
// each file as "files[n]"; the two cannot be combined any other way, so
// this is a separate path rather than an extra field on postDiscord.
func postDiscordMultipart(ctx context.Context, webhookURL, content string, files []plugin.NotificationFile) error {
	ctx, cancel := context.WithTimeout(ctx, discordUploadTimeout)
	defer cancel()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("comm-discord: marshaling payload_json: %w", err)
	}
	if err := w.WriteField("payload_json", string(payload)); err != nil {
		return fmt.Errorf("comm-discord: writing payload_json: %w", err)
	}

	for i, f := range files {
		part, err := w.CreateFormFile(fmt.Sprintf("files[%d]", i), sanitizeDiscordFilename(f.Filename))
		if err != nil {
			return fmt.Errorf("comm-discord: creating file part: %w", err)
		}
		if _, err := part.Write(f.Data); err != nil {
			return fmt.Errorf("comm-discord: writing file part: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("comm-discord: closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, &body)
	if err != nil {
		return fmt.Errorf("comm-discord: creating request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-discord: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("comm-discord: discord returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("comm-discord: discord returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// sanitizeDiscordFilename strips path separators so a filename can
// never escape into a path Discord would reject.
func sanitizeDiscordFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" {
		return "attachment"
	}
	return name
}

func parseDiscordConfig(cfg map[string]string) (discordConfig, error) {
	webhookURL := strings.TrimSpace(firstNonEmpty(cfg["webhook_url"], cfg["webhookUrl"]))
	if webhookURL == "" {
		return discordConfig{}, errors.New("comm-discord: webhook_url is required")
	}
	if !strings.HasPrefix(webhookURL, "https://discord.com/api/webhooks/") && !strings.HasPrefix(webhookURL, "https://discordapp.com/api/webhooks/") {
		return discordConfig{}, errors.New("comm-discord: webhook_url must be a Discord webhook URL")
	}
	return discordConfig{WebhookURL: webhookURL}, nil
}

func postDiscord(ctx context.Context, webhookURL string, payload map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, discordTimeout)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("comm-discord: marshaling request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("comm-discord: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-discord: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("comm-discord: discord returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("comm-discord: discord returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func buildDiscordContent(msg plugin.NotificationMessage) string {
	title := strings.TrimSpace(msg.Title)
	message := strings.TrimSpace(msg.Message)
	if title == "" {
		return message
	}
	if message == "" {
		return "**" + title + "**"
	}
	return fmt.Sprintf("**%s**\n%s", title, message)
}

func configMapFromDiscordConfig(cfg discordConfig) map[string]string {
	return map[string]string{"webhook_url": cfg.WebhookURL}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
