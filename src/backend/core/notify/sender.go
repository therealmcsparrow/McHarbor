// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SlackConfig holds Slack webhook parameters.
type SlackConfig struct {
	WebhookURL string
}

// DiscordConfig holds Discord webhook parameters.
type DiscordConfig struct {
	WebhookURL string
}

// TeamsConfig holds Microsoft Teams webhook parameters.
type TeamsConfig struct {
	WebhookURL string
}

// GotifyConfig holds Gotify server parameters.
type GotifyConfig struct {
	ServerURL string
	AppToken  string
	Priority  string
}

// NtfyConfig holds ntfy server parameters.
type NtfyConfig struct {
	ServerURL   string
	Topic       string
	AccessToken string
	Username    string
	Password    string
	Priority    string
}

// TelegramConfig holds Telegram Bot API parameters.
type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// SignalConfig holds Signal REST API parameters.
type SignalConfig struct {
	ServerURL    string
	SenderNumber string
	Recipients   []string
	Username     string
	Password     string
}

// WhatsAppConfig holds WhatsApp Cloud API parameters.
type WhatsAppConfig struct {
	PhoneNumberID  string
	AccessToken    string
	RecipientPhone string
}

// SendSlack sends a message to a Slack webhook.
func SendSlack(ctx context.Context, cfg SlackConfig, title, message string) error {
	body := map[string]string{"text": fmt.Sprintf("*%s*\n%s", title, message)}
	return doPost(ctx, cfg.WebhookURL, body, nil)
}

// SendDiscord sends a message to a Discord webhook.
func SendDiscord(ctx context.Context, cfg DiscordConfig, title, message string) error {
	body := map[string]string{"content": fmt.Sprintf("**%s**\n%s", title, message)}
	return doPost(ctx, cfg.WebhookURL, body, nil)
}

// SendTeams sends an Adaptive Card message to a Microsoft Teams webhook.
func SendTeams(ctx context.Context, cfg TeamsConfig, title, message string) error {
	card := map[string]interface{}{
		"type":    "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]interface{}{
						{"type": "TextBlock", "text": title, "weight": "Bolder", "size": "Medium"},
						{"type": "TextBlock", "text": message, "wrap": true},
					},
				},
			},
		},
	}
	return doPost(ctx, cfg.WebhookURL, card, nil)
}

// SendGotify sends a message to a Gotify server.
func SendGotify(ctx context.Context, cfg GotifyConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/message?token=" + cfg.AppToken
	body := map[string]interface{}{
		"title":   title,
		"message": message,
	}
	if cfg.Priority != "" {
		var p int
		if _, err := fmt.Sscanf(cfg.Priority, "%d", &p); err == nil {
			body["priority"] = p
		}
	}
	return doPost(ctx, url, body, nil)
}

// SendNtfy sends a message to an ntfy server.
func SendNtfy(ctx context.Context, cfg NtfyConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/" + cfg.Topic

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("creating ntfy request: %w", err)
	}
	req.Header.Set("Title", title)
	if cfg.Priority != "" {
		req.Header.Set("Priority", cfg.Priority)
	}
	if cfg.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	} else if cfg.Username != "" && cfg.Password != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending ntfy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ntfy returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// SendTelegram sends a message via the Telegram Bot API.
func SendTelegram(ctx context.Context, cfg TelegramConfig, title, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.BotToken)
	body := map[string]interface{}{
		"chat_id":    cfg.ChatID,
		"text":       fmt.Sprintf("*%s*\n%s", title, message),
		"parse_mode": "Markdown",
	}
	return doPost(ctx, url, body, nil)
}

// SendSignal sends a message via the Signal REST API.
func SendSignal(ctx context.Context, cfg SignalConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/v2/send"
	body := map[string]interface{}{
		"message":    fmt.Sprintf("%s\n%s", title, message),
		"number":     cfg.SenderNumber,
		"recipients": cfg.Recipients,
	}
	var headers map[string]string
	if cfg.Username != "" && cfg.Password != "" {
		headers = map[string]string{}
	}
	req, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling signal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req))
	if err != nil {
		return fmt.Errorf("creating signal request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.Username != "" && cfg.Password != "" {
		httpReq.SetBasicAuth(cfg.Username, cfg.Password)
	}
	_ = headers

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending signal request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("signal returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// SendWhatsApp sends a message via the WhatsApp Cloud API.
func SendWhatsApp(ctx context.Context, cfg WhatsAppConfig, title, message string) error {
	url := fmt.Sprintf("https://graph.facebook.com/v23.0/%s/messages", cfg.PhoneNumberID)
	body := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                cfg.RecipientPhone,
		"type":              "text",
		"text": map[string]string{
			"body": fmt.Sprintf("*%s*\n%s", title, message),
		},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.AccessToken,
	}
	return doPost(ctx, url, body, headers)
}

// WhatsAppGatewayConfig holds parameters for self-hosted WhatsApp gateway or SaaS platforms.
type WhatsAppGatewayConfig struct {
	ServerURL      string
	Token          string
	RecipientPhone string
}

// SendWhatsAppGateway sends a message via a self-hosted WhatsApp gateway or SaaS endpoint.
func SendWhatsAppGateway(ctx context.Context, cfg WhatsAppGatewayConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/")
	body := map[string]string{
		"to":   cfg.RecipientPhone,
		"body": fmt.Sprintf("%s\n%s", title, message),
	}
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.Token,
	}
	return doPost(ctx, url, body, headers)
}

// WhatsAppBusinessConfig holds parameters for a WhatsApp Business bot platform.
type WhatsAppBusinessConfig struct {
	ServerURL      string
	PhoneNumberID  string
	Token          string
	RecipientPhone string
}

// SendWhatsAppBusiness sends a message via a WhatsApp Business platform endpoint.
func SendWhatsAppBusiness(ctx context.Context, cfg WhatsAppBusinessConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/v1/messages"
	body := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                cfg.RecipientPhone,
		"type":              "text",
		"text": map[string]string{
			"body": fmt.Sprintf("*%s*\n%s", title, message),
		},
	}
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.Token,
	}
	return doPost(ctx, url, body, headers)
}

// SignalBotConfig holds parameters for a Signal bot server.
type SignalBotConfig struct {
	ServerURL  string
	Token      string
	Recipients []string
}

// SendSignalBot sends a message via a Signal bot endpoint.
func SendSignalBot(ctx context.Context, cfg SignalBotConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/v1/send"
	body := map[string]interface{}{
		"message":    fmt.Sprintf("%s\n%s", title, message),
		"recipients": cfg.Recipients,
	}
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.Token,
	}
	return doPost(ctx, url, body, headers)
}

// SignalDConfig holds parameters for a signald server.
type SignalDConfig struct {
	ServerURL    string
	SenderNumber string
	Recipients   []string
}

// SendSignalD sends a message via a signald endpoint.
func SendSignalD(ctx context.Context, cfg SignalDConfig, title, message string) error {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/v1/send"

	recipientAddresses := make([]map[string]string, len(cfg.Recipients))
	for i, r := range cfg.Recipients {
		recipientAddresses[i] = map[string]string{"number": r}
	}

	body := map[string]interface{}{
		"username":         cfg.SenderNumber,
		"recipientAddress": recipientAddresses,
		"messageBody":      fmt.Sprintf("%s\n%s", title, message),
	}
	return doPost(ctx, url, body, nil)
}

// doPost sends a JSON POST request with optional extra headers.
func doPost(ctx context.Context, url string, body interface{}, headers map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("request returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
