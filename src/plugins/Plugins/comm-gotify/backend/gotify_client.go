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
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const gotifyTimeout = 10 * time.Second

type gotifyClient struct{}

type gotifyConfig struct {
	ServerURL string
	Token     string
	Priority  string
}

// NewGotifyClient returns a stateless NotificationAdapterClient.
func NewGotifyClient() *gotifyClient { return &gotifyClient{} }

// Test sends a real test message to the configured Gotify application.
func (c *gotifyClient) Test(ctx context.Context, cfg map[string]string) error {
	parsed, err := parseGotifyConfig(cfg)
	if err != nil {
		return err
	}
	msg := plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification from the McHarbor Gotify communication channel.",
		Level:   "info",
	}
	return c.Send(ctx, configMapFromGotifyConfig(parsed), msg)
}

// Send delivers the notification through Gotify.
func (c *gotifyClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	parsed, err := parseGotifyConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comm-gotify: empty payload (title and message both blank)")
	}

	payload := map[string]any{
		"title":   firstNonEmpty(msg.Title, "McHarbor notification"),
		"message": strings.TrimSpace(msg.Message),
	}
	if parsed.Priority != "" {
		priority, err := strconv.Atoi(parsed.Priority)
		if err != nil {
			return fmt.Errorf("comm-gotify: invalid priority %q", parsed.Priority)
		}
		payload["priority"] = priority
	}
	return postGotify(ctx, parsed, payload)
}

func parseGotifyConfig(cfg map[string]string) (gotifyConfig, error) {
	serverURL := strings.TrimRight(strings.TrimSpace(firstNonEmpty(cfg["server_url"], cfg["serverUrl"])), "/")
	if serverURL == "" {
		return gotifyConfig{}, errors.New("comm-gotify: server_url is required")
	}
	if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
		return gotifyConfig{}, errors.New("comm-gotify: server_url must start with http:// or https://")
	}
	token := strings.TrimSpace(cfg["token"])
	if token == "" {
		return gotifyConfig{}, errors.New("comm-gotify: token is required")
	}
	priority := strings.TrimSpace(cfg["priority"])
	if priority != "" {
		if _, err := strconv.Atoi(priority); err != nil {
			return gotifyConfig{}, fmt.Errorf("comm-gotify: invalid priority %q", priority)
		}
	}
	return gotifyConfig{ServerURL: serverURL, Token: token, Priority: priority}, nil
}

func postGotify(ctx context.Context, cfg gotifyConfig, payload map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, gotifyTimeout)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("comm-gotify: marshaling request body: %w", err)
	}
	endpoint := cfg.ServerURL + "/message?token=" + cfg.Token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("comm-gotify: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-gotify: sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("comm-gotify: gotify returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("comm-gotify: gotify returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func configMapFromGotifyConfig(cfg gotifyConfig) map[string]string {
	return map[string]string{
		"server_url": cfg.ServerURL,
		"token":      cfg.Token,
		"priority":   cfg.Priority,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
