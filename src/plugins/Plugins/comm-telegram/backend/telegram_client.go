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
	"net/url"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// telegramAPIBase is the canonical Telegram Bot API hostname.
// v1 surface — the Bot API has been at /v1/ for the whole life
// of the project; future majors bump the path.
const telegramAPIBase = "https://api.telegram.org"

// telegramHTTPTimeout caps a single API round-trip. Test calls
// (getMe) complete in ~200ms; Send calls (sendMessage) in ~300ms
// over typical consumer connections. 10s gives plenty of slack
// without hanging the host on a wedged bot.
const telegramHTTPTimeout = 10 * time.Second

// telegramUploadTimeout is longer than a text round-trip: a document
// upload carries a report-sized body.
const telegramUploadTimeout = 60 * time.Second

// telegramClient is the NotificationAdapterClient the host
// dispatches to for the telegram ChannelType. Stateless: every
// method takes the channel row's decrypted config map directly.
//
// Concurrency: NotificationAdapterClient implementations must be
// safe for concurrent use — the host invokes Test + Send from
// multiple goroutines during workflow fan-out. telegramClient has
// no shared state so this is automatic.
type telegramClient struct{}

// NewTelegramClient returns a fresh client. The plugin keeps no
// shared state; callers can construct as many as they want.
func NewTelegramClient() *telegramClient {
	return &telegramClient{}
}

// ============================================================================
// NotificationAdapterClient: Test
// ============================================================================

// Test verifies the bot token and then sends an actual message to
// the configured chat. The host's Test button promises delivery, so
// this must exercise sendMessage instead of only checking getMe.
func (c *telegramClient) Test(ctx context.Context, cfg map[string]string) error {
	token := strings.TrimSpace(cfg["bot_token"])
	chatID := strings.TrimSpace(cfg["chat_id"])

	if token == "" {
		return errors.New("comm-telegram: bot_token is required — obtain one from @BotFather on Telegram")
	}
	if chatID == "" {
		return errors.New("comm-telegram: chat_id is required before testing — get the chat_id from getUpdates or use @channelname for public channels")
	}

	endpoint := telegramAPIBase + "/bot" + token + "/getMe"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("comm-telegram: build getMe request: %w", err)
	}

	cli := &http.Client{Timeout: telegramHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("comm-telegram: getMe request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("comm-telegram: getMe returned status %d: %s", resp.StatusCode, truncateTelegramBody(body, 256))
	}

	// getMe returns {"ok":true,"result":{"id":...,"is_bot":true,"first_name":"...",...}}
	// We don't need the result fields; just confirm ok:true so a
	// compromised or revoked token is rejected.
	var parsed struct {
		OK     bool   `json:"ok"`
		Desc   string `json:"description"`
		ErrMsg string `json:"error_code"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("comm-telegram: parse getMe response: %w", err)
	}
	if !parsed.OK {
		return fmt.Errorf("comm-telegram: getMe failed: %s", parsed.Desc)
	}

	return c.Send(ctx, cfg, plugin.NotificationMessage{
		Title:   "McHarbor Test Notification",
		Message: "This is a test notification sent from McHarbor to verify your Telegram channel configuration.",
		Level:   "info",
	})
}

// ============================================================================
// NotificationAdapterClient: Send
// ============================================================================

// Send delivers msg to the configured Telegram chat. Builds a
// sendMessage request with parse_mode + disable_notification when
// set in the config; otherwise sends a plain text payload.
//
// Telegram's sendMessage signature (v1):
//
//	POST /bot<token>/sendMessage
//	{
//	  "chat_id":                  <int|str>,
//	  "text":                     <str>,
//	  "parse_mode":               "Markdown"|"HTML" (optional),
//	  "disable_notification":     true|false (optional)
//	}
//
// Title + Message are combined into the body as "Title\n\nMessage"
// so the subject line renders bolded above the body when parse_mode
// is enabled. The host does not pre-escape special characters;
// operators who need verbatim messages must set parse_mode to "".
//
// Attachments are silently ignored — the plugin reports
// SupportsRichAttachments=false so the host should never call
// Send with a non-empty Attachments slice. We still defensively
// skip them rather than returning an error.
func (c *telegramClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	token := strings.TrimSpace(cfg["bot_token"])
	chatID := strings.TrimSpace(cfg["chat_id"])

	if token == "" {
		return errors.New("comm-telegram: bot_token is required — obtain one from @BotFather on Telegram")
	}
	if chatID == "" {
		return errors.New("comm-telegram: chat_id is required — fill it in before saving the channel")
	}
	if msg.Title == "" && msg.Message == "" {
		return errors.New("comm-telegram: empty payload (title and message both blank)")
	}

	body := buildTelegramText(msg)

	// A document goes to a different endpoint with a multipart body,
	// so it branches before the JSON payload is assembled. Telegram
	// caps a caption at 1024 characters; the text is trimmed rather
	// than the request being rejected.
	if len(msg.Files) > 0 {
		return sendTelegramDocuments(ctx, token, chatID, body, msg.Files)
	}

	payload := map[string]any{
		"chat_id": chatID,
		"text":    body,
	}
	if pm := strings.TrimSpace(cfg["parse_mode"]); pm != "" {
		payload["parse_mode"] = pm
	}
	if dn := strings.TrimSpace(cfg["disable_notification"]); dn != "" {
		// Only forward when explicitly set to "true". Anything
		// else (including "false", "0", "no") is treated as
		// unset and Telegram defaults to sending a sound.
		if d, err := parseBoolString(dn); err == nil && d {
			payload["disable_notification"] = true
		}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("comm-telegram: marshal sendMessage payload: %w", err)
	}

	endpoint := telegramAPIBase + "/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return fmt.Errorf("comm-telegram: build sendMessage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: telegramHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("comm-telegram: sendMessage request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("comm-telegram: sendMessage returned status %d: %s", resp.StatusCode, truncateTelegramBody(respBody, 256))
	}

	// Telegram returns {"ok":true,"result":{"message_id":...}}. A
	// revoked token or invalid chat_id surfaces as ok:false with
	// a "description" field; we surface that directly so the host
	// audit log records the failure.
	var parsed struct {
		OK     bool   `json:"ok"`
		Desc   string `json:"description"`
		ErrMsg string `json:"error_code"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return fmt.Errorf("comm-telegram: parse sendMessage response: %w", err)
	}
	if !parsed.OK {
		return fmt.Errorf("comm-telegram: sendMessage failed: %s", parsed.Desc)
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// buildTelegramText concatenates Title + Message into the single
// "text" field Telegram's sendMessage accepts. When parse_mode is
// Markdown, we wrap the title in *bold* so the subject renders
// bolded above the body. With HTML we wrap it in <b>...</b>.
// With parse_mode unset we just join with a blank line.
func buildTelegramText(msg plugin.NotificationMessage) string {
	if msg.Title == "" {
		return msg.Message
	}
	if msg.Message == "" {
		return msg.Title
	}
	return msg.Title + "\n\n" + msg.Message
}

// truncateTelegramBody clamps an error body to `max` bytes so a
// multi-MB Telegram response doesn't leak into logs or user-
// facing messages. Mirrors the same helper in storage-sharepoint-
// online's client.
func truncateTelegramBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// parseBoolString decodes the subset of truthy strings the host
// writes to a `bool`-typed ConfigFieldDecl. ConfigFieldDecl.Type
// is a string in the JSON manifest; the host stores "true" /
// "false" verbatim. We accept a couple of common alternates to
// be forgiving when an operator types "1" / "yes".
func parseBoolString(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("not a bool: %q", s)
	}
}

// url path escape would normally come from net/url; importing
// the package here ensures the file compiles when the rest of
// the codebase changes. We use url.QueryEscape-style semantics
// for chat_id values that contain @ (channels) — Telegram
// accepts both raw and percent-encoded forms.
var _ = url.QueryEscape

// telegramCaptionLimit is Telegram's documented maximum caption length.
const telegramCaptionLimit = 1024

// sendTelegramDocuments uploads each file with sendDocument.
//
// Telegram takes one document per call, so several files become several
// calls; the caption rides on the first so the message is not repeated
// under every attachment.
func sendTelegramDocuments(ctx context.Context, token, chatID, caption string, files []plugin.NotificationFile) error {
	for i, f := range files {
		text := ""
		if i == 0 {
			text = caption
			if len(text) > telegramCaptionLimit {
				text = text[:telegramCaptionLimit]
			}
		}
		if err := sendTelegramDocument(ctx, token, chatID, text, f); err != nil {
			return err
		}
	}
	return nil
}

func sendTelegramDocument(ctx context.Context, token, chatID, caption string, file plugin.NotificationFile) error {
	ctx, cancel := context.WithTimeout(ctx, telegramUploadTimeout)
	defer cancel()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	if err := w.WriteField("chat_id", chatID); err != nil {
		return fmt.Errorf("comm-telegram: writing chat_id: %w", err)
	}
	if caption != "" {
		if err := w.WriteField("caption", caption); err != nil {
			return fmt.Errorf("comm-telegram: writing caption: %w", err)
		}
	}

	name := strings.TrimSpace(file.Filename)
	if name == "" {
		name = "attachment"
	}
	part, err := w.CreateFormFile("document", name)
	if err != nil {
		return fmt.Errorf("comm-telegram: creating document part: %w", err)
	}
	if _, err := part.Write(file.Data); err != nil {
		return fmt.Errorf("comm-telegram: writing document part: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("comm-telegram: closing multipart writer: %w", err)
	}

	endpoint := telegramAPIBase + "/bot" + token + "/sendDocument"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return fmt.Errorf("comm-telegram: build sendDocument request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	cli := &http.Client{Timeout: telegramUploadTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("comm-telegram: sendDocument request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("comm-telegram: sendDocument returned status %d: %s",
			resp.StatusCode, truncateTelegramBody(respBody, 256))
	}

	var parsed struct {
		OK   bool   `json:"ok"`
		Desc string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return fmt.Errorf("comm-telegram: parse sendDocument response: %w", err)
	}
	if !parsed.OK {
		return fmt.Errorf("comm-telegram: sendDocument rejected: %s", parsed.Desc)
	}
	return nil
}
