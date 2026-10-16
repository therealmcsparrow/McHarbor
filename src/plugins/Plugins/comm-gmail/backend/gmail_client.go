// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const gmailTimeout = 30 * time.Second

// gmailUploadTimeout is longer than gmailTimeout: a message carrying a
// report-sized attachment goes over the same connection and 30s is not
// enough on a slow link.
const gmailUploadTimeout = 120 * time.Second

// gmailMaxAttachmentBytes caps the raw bytes of a single send. Gmail's
// own ceiling is 35 MB for the finished message, but the bytes are
// base64-encoded twice on the way there — once into the MIME part, once
// into the JSON "raw" field — which inflates them by roughly 1.8x.
// Checking here turns an opaque provider 413 into a readable error.
const gmailMaxAttachmentBytes = 20 << 20

type gmailClient struct{}

type gmailConfig struct {
	ClientID      string
	ClientSecret  string
	FromAddress   string
	FromName      string
	Recipients    []string
	SubjectPrefix string
}

// NewGmailClient returns a stateless NotificationAdapterClient.
func NewGmailClient() *gmailClient { return &gmailClient{} }

// Test sends a real test email through the configured Gmail API sender.
func (c *gmailClient) Test(ctx context.Context, cfg map[string]string) error {
	parsed, err := parseGmailConfig(cfg)
	if err != nil {
		return err
	}
	msg := plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification from the McHarbor Gmail communication channel.",
		Level:   "info",
	}
	return c.Send(ctx, configMapFromGmailConfig(parsed), msg)
}

// Send delivers the notification as an HTML email through the Gmail API.
func (c *gmailClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	parsed, err := parseGmailConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comm-gmail: empty payload (title and message both blank)")
	}

	// The host only populates Files for adapters that declared
	// SupportsFileAttachments, so a non-empty slice here is this
	// adapter's own capability coming back — no re-check needed.
	if total := totalAttachmentBytes(msg.Files); total > gmailMaxAttachmentBytes {
		return fmt.Errorf("comm-gmail: attachments total %d bytes, over the %d byte limit", total, gmailMaxAttachmentBytes)
	}

	timeout := gmailTimeout
	if len(msg.Files) > 0 {
		timeout = gmailUploadTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	token, err := fetchGmailToken(ctx, parsed)
	if err != nil {
		return fmt.Errorf("comm-gmail: gmail token: %w", err)
	}

	subject := buildSubject(parsed.SubjectPrefix, msg.Title)
	body := buildHTMLBody(msg)
	for _, recipient := range parsed.Recipients {
		raw := buildMIMEMessage(parsed.FromHeader(), recipient, subject, body, msg.Files)
		if err := sendGmailMessage(ctx, token, parsed.FromAddress, raw); err != nil {
			return fmt.Errorf("comm-gmail: send to %s: %w", recipient, err)
		}
	}
	return nil
}

func parseGmailConfig(cfg map[string]string) (gmailConfig, error) {
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return gmailConfig{}, errors.New("comm-gmail: client_id is required")
	}
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientSecret == "" {
		return gmailConfig{}, errors.New("comm-gmail: client_secret is required")
	}

	fromAddress := strings.TrimSpace(firstNonEmpty(cfg["from_address"], cfg["from"]))
	parsedFrom, err := mail.ParseAddress(fromAddress)
	if err != nil {
		return gmailConfig{}, fmt.Errorf("comm-gmail: invalid from address: %w", err)
	}

	recipients, err := parseRecipients(cfg["to"])
	if err != nil {
		return gmailConfig{}, err
	}

	return gmailConfig{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		FromAddress:   parsedFrom.Address,
		FromName:      strings.TrimSpace(cfg["from_name"]),
		Recipients:    recipients,
		SubjectPrefix: strings.TrimSpace(cfg["subject_prefix"]),
	}, nil
}

func fetchGmailToken(ctx context.Context, cfg gmailConfig) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("scope", "https://www.googleapis.com/auth/gmail.send")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("oauth2 error: %s - %s", result.Error, result.ErrorDesc)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}
	if result.AccessToken == "" {
		return "", errors.New("empty access token in response")
	}
	return result.AccessToken, nil
}

func sendGmailMessage(ctx context.Context, token, fromAddress string, msg []byte) error {
	payload, err := json.Marshal(map[string]string{"raw": base64.URLEncoding.EncodeToString(msg)})
	if err != nil {
		return fmt.Errorf("marshaling gmail payload: %w", err)
	}

	endpoint := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/%s/messages/send", url.PathEscape(fromAddress))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating gmail request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("gmail send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("gmail API error status %d", resp.StatusCode)
		}
		return fmt.Errorf("gmail API error status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func parseRecipients(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	recipients := make([]string, 0, len(parts))
	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr == "" {
			continue
		}
		parsed, err := mail.ParseAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("comm-gmail: invalid recipient address %q: %w", addr, err)
		}
		recipients = append(recipients, parsed.Address)
	}
	if len(recipients) == 0 {
		return nil, errors.New("comm-gmail: at least one recipient is required")
	}
	return recipients, nil
}

func buildSubject(prefix, title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "McHarbor notification"
	}
	if prefix == "" {
		return title
	}
	return strings.TrimSpace(prefix + " " + title)
}

func buildHTMLBody(msg plugin.NotificationMessage) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html><body>")
	if level := strings.TrimSpace(msg.Level); level != "" {
		b.WriteString("<p><strong>Level:</strong> ")
		b.WriteString(htmlEscape(level))
		b.WriteString("</p>")
	}
	if title := strings.TrimSpace(msg.Title); title != "" {
		b.WriteString("<h2>")
		b.WriteString(htmlEscape(title))
		b.WriteString("</h2>")
	}
	b.WriteString("<p>")
	b.WriteString(strings.ReplaceAll(htmlEscape(strings.TrimSpace(msg.Message)), "\n", "<br>"))
	b.WriteString("</p></body></html>")
	return b.String()
}

func buildMIMEMessage(from, to, subject, bodyHTML string, files []plugin.NotificationFile) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	buf.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	buf.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")

	// Without attachments the message stays a single text/html part —
	// the shape every mail client renders most predictably. Wrapping a
	// plain notification in multipart just because the code can would
	// make every existing send slightly worse.
	if len(files) == 0 {
		buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		buf.WriteString("\r\n")
		buf.WriteString(bodyHTML)
		return buf.Bytes()
	}

	boundary := mimeBoundary()
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	buf.WriteString("\r\n")

	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(bodyHTML)
	buf.WriteString("\r\n")

	for _, f := range files {
		contentType := strings.TrimSpace(f.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		name := sanitizeFilename(f.Filename)

		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		fmt.Fprintf(&buf, "Content-Type: %s; name=%q\r\n", sanitizeHeader(contentType), name)
		buf.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&buf, "Content-Disposition: attachment; filename=%q\r\n", name)
		buf.WriteString("\r\n")
		writeBase64Lines(&buf, f.Data)
		buf.WriteString("\r\n")
	}
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	return buf.Bytes()
}

// totalAttachmentBytes sums the raw payload so the size ceiling is
// checked once, before a token is fetched or a byte is encoded.
func totalAttachmentBytes(files []plugin.NotificationFile) int {
	total := 0
	for _, f := range files {
		total += len(f.Data)
	}
	return total
}

// sanitizeFilename strips path separators and header-breaking
// characters so a filename can never escape its quoted MIME parameter.
func sanitizeFilename(name string) string {
	name = sanitizeHeader(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	if name == "" {
		return "attachment"
	}
	return name
}

// mimeBoundary returns a boundary that cannot collide with message
// content. crypto/rand rather than math/rand: a predictable boundary
// appearing inside an attachment would truncate the message.
func mimeBoundary() string {
	var buf [18]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// A boundary must exist even if the entropy source fails; the
		// timestamp is unique enough to keep the message well-formed.
		return fmt.Sprintf("mcharbor-%d", time.Now().UnixNano())
	}
	return "mcharbor-" + base64.RawURLEncoding.EncodeToString(buf[:])
}

// writeBase64Lines emits base64 wrapped at 76 characters, as RFC 2045
// requires. Mail servers may reject or mangle longer lines.
func writeBase64Lines(w *bytes.Buffer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	for len(encoded) > lineLen {
		fmt.Fprint(w, encoded[:lineLen], "\r\n")
		encoded = encoded[lineLen:]
	}
	if encoded != "" {
		fmt.Fprint(w, encoded, "\r\n")
	}
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	return value
}

func configMapFromGmailConfig(cfg gmailConfig) map[string]string {
	return map[string]string{
		"client_id":      cfg.ClientID,
		"client_secret":  cfg.ClientSecret,
		"from_address":   cfg.FromAddress,
		"from_name":      cfg.FromName,
		"to":             strings.Join(cfg.Recipients, ","),
		"subject_prefix": cfg.SubjectPrefix,
	}
}

func (cfg gmailConfig) FromHeader() string {
	if cfg.FromName == "" {
		return cfg.FromAddress
	}
	return (&mail.Address{Name: cfg.FromName, Address: cfg.FromAddress}).String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
