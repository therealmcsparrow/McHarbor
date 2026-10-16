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

const slackTimeout = 10 * time.Second

type slackClient struct{}

type slackConfig struct {
	WebhookURL string
}

// NewSlackClient returns a stateless NotificationAdapterClient.
func NewSlackClient() *slackClient { return &slackClient{} }

// Test sends a real test message to the configured Slack webhook.
func (c *slackClient) Test(ctx context.Context, cfg map[string]string) error {
	parsed, err := parseSlackConfig(cfg)
	if err != nil {
		return err
	}
	msg := plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification from the McHarbor Slack communication channel.",
		Level:   "info",
	}
	return c.Send(ctx, configMapFromSlackConfig(parsed), msg)
}

// Send delivers the notification through a Slack incoming webhook.
func (c *slackClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	parsed, err := parseSlackConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comm-slack: empty payload (title and message both blank)")
	}

	payload := map[string]string{"text": buildSlackText(msg)}
	return postSlack(ctx, parsed.WebhookURL, payload)
}

func parseSlackConfig(cfg map[string]string) (slackConfig, error) {
	webhookURL := strings.TrimSpace(firstNonEmpty(cfg["webhook_url"], cfg["webhookUrl"]))
	if webhookURL == "" {
		return slackConfig{}, errors.New("comm-slack: webhook_url is required")
	}
	if !strings.HasPrefix(webhookURL, "https://hooks.slack.com/services/") {
		return slackConfig{}, errors.New("comm-slack: webhook_url must be a Slack incoming webhook URL")
	}
	return slackConfig{WebhookURL: webhookURL}, nil
}

func postSlack(ctx context.Context, webhookURL string, payload map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, slackTimeout)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("comm-slack: marshaling request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("comm-slack: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-slack: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("comm-slack: slack returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("comm-slack: slack returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func buildSlackText(msg plugin.NotificationMessage) string {
	title := strings.TrimSpace(msg.Title)
	message := strings.TrimSpace(msg.Message)
	if title == "" {
		return message
	}
	if message == "" {
		return "*" + title + "*"
	}
	return fmt.Sprintf("*%s*\n%s", title, message)
}

func configMapFromSlackConfig(cfg slackConfig) map[string]string {
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
