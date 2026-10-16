// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
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

const exchangeTimeout = 30 * time.Second

// exchangeUploadTimeout is longer than exchangeTimeout: a sendMail
// request carrying an attachment goes over the same connection and 30s
// is not enough on a slow link.
const exchangeUploadTimeout = 120 * time.Second

// exchangeInlineAttachmentBytes is the largest payload that still fits
// in a single sendMail call. Graph rejects a sendMail request whose
// total size exceeds 4 MB, and contentBytes is base64 so the payload
// inflates 4/3 on the way out: 2.8 MB of raw data encodes to ~3.7 MB,
// leaving headroom for the headers and the HTML body. Above this the
// adapter switches to the draft + upload-session path, which costs four
// extra round trips — worth it for a 40 MB export, not for a 40 KB PDF.
const exchangeInlineAttachmentBytes = 2_800_000

// exchangeMaxAttachmentBytes is Graph's own ceiling for a message built
// through an upload session. The tenant's message-size limit may be
// lower (Exchange Online defaults well below this and is configurable
// per-mailbox), in which case Graph rejects the send and the operator
// sees that error rather than this one.
const exchangeMaxAttachmentBytes = 150 << 20

// exchangeUploadChunkSize is 320 KiB x 10. Graph requires every chunk
// but the last to be a multiple of 320 KiB, and each PUT to stay under
// 4 MB.
const exchangeUploadChunkSize = 320 * 1024 * 10

// exchangeMaxUploadDeadline bounds exchangeUploadDeadline so a stalled
// upload cannot pin the dispatcher indefinitely.
const exchangeMaxUploadDeadline = 30 * time.Minute

type exchangeClient struct{}

type exchangeConfig struct {
	TenantID      string
	ClientID      string
	ClientSecret  string
	FromAddress   string
	FromName      string
	Recipients    []string
	SubjectPrefix string
}

// NewExchangeClient returns a stateless NotificationAdapterClient.
func NewExchangeClient() *exchangeClient { return &exchangeClient{} }

// Test sends a real test email through the configured Microsoft Graph sender.
func (c *exchangeClient) Test(ctx context.Context, cfg map[string]string) error {
	parsed, err := parseExchangeConfig(cfg)
	if err != nil {
		return err
	}
	msg := plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification from the McHarbor Exchange communication channel.",
		Level:   "info",
	}
	return c.Send(ctx, configMapFromExchangeConfig(parsed), msg)
}

// Send delivers the notification as an HTML email through Microsoft Graph.
func (c *exchangeClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	parsed, err := parseExchangeConfig(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.Title) == "" && strings.TrimSpace(msg.Message) == "" {
		return errors.New("comm-exchange: empty payload (title and message both blank)")
	}

	// The host only populates Files for adapters that declared
	// SupportsFileAttachments, so a non-empty slice here is this
	// adapter's own capability coming back — no re-check needed.
	total := totalAttachmentBytes(msg.Files)
	if total > exchangeMaxAttachmentBytes {
		return fmt.Errorf("comm-exchange: attachments total %d bytes, over the %d byte limit", total, exchangeMaxAttachmentBytes)
	}

	timeout := exchangeTimeout
	if len(msg.Files) > 0 {
		timeout = exchangeUploadDeadline(total)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	token, err := fetchExchangeToken(ctx, parsed)
	if err != nil {
		return fmt.Errorf("comm-exchange: exchange token: %w", err)
	}

	subject := buildSubject(parsed.SubjectPrefix, msg.Title)
	body := buildHTMLBody(msg)

	// One message per recipient, so recipients never see each other.
	// The upload path keeps that, which means a large payload is
	// uploaded once per recipient: collapsing to a single draft with
	// several toRecipients would upload once but quietly change who can
	// see whom, and only for large sends.
	for _, recipient := range parsed.Recipients {
		if total > exchangeInlineAttachmentBytes {
			if err := sendExchangeLargeMessage(ctx, token, parsed.FromAddress, recipient, subject, body, msg.Files); err != nil {
				return fmt.Errorf("comm-exchange: send to %s: %w", recipient, err)
			}
			continue
		}
		if err := sendExchangeMessage(ctx, token, parsed.FromAddress, recipient, subject, body, msg.Files); err != nil {
			return fmt.Errorf("comm-exchange: send to %s: %w", recipient, err)
		}
	}
	return nil
}

// exchangeUploadDeadline scales the timeout with the payload: 2 MB and
// 150 MB are three orders of magnitude apart and no single constant
// suits both. A two-minute floor plus a minute per 5 MB tolerates a
// sustained ~85 KB/s, slow enough to survive a poor link, and the cap
// keeps a stalled upload from holding the dispatcher forever.
func exchangeUploadDeadline(total int) time.Duration {
	deadline := exchangeUploadTimeout + time.Duration(total/(5<<20))*time.Minute
	if deadline > exchangeMaxUploadDeadline {
		return exchangeMaxUploadDeadline
	}
	return deadline
}

func parseExchangeConfig(cfg map[string]string) (exchangeConfig, error) {
	tenantID := strings.TrimSpace(cfg["tenant_id"])
	if tenantID == "" {
		return exchangeConfig{}, errors.New("comm-exchange: tenant_id is required")
	}
	clientID := strings.TrimSpace(cfg["client_id"])
	if clientID == "" {
		return exchangeConfig{}, errors.New("comm-exchange: client_id is required")
	}
	clientSecret := strings.TrimSpace(cfg["client_secret"])
	if clientSecret == "" {
		return exchangeConfig{}, errors.New("comm-exchange: client_secret is required")
	}

	fromAddress := strings.TrimSpace(firstNonEmpty(cfg["from_address"], cfg["from"]))
	parsedFrom, err := mail.ParseAddress(fromAddress)
	if err != nil {
		return exchangeConfig{}, fmt.Errorf("comm-exchange: invalid from address: %w", err)
	}

	recipients, err := parseRecipients(cfg["to"])
	if err != nil {
		return exchangeConfig{}, err
	}

	return exchangeConfig{
		TenantID:      tenantID,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		FromAddress:   parsedFrom.Address,
		FromName:      strings.TrimSpace(cfg["from_name"]),
		Recipients:    recipients,
		SubjectPrefix: strings.TrimSpace(cfg["subject_prefix"]),
	}, nil
}

func fetchExchangeToken(ctx context.Context, cfg exchangeConfig) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("scope", "https://graph.microsoft.com/.default")

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(cfg.TenantID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
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

// buildExchangeMessage assembles the Graph message resource shared by
// the sendMail payload and the draft the upload path creates. Files are
// inlined as fileAttachment entries; the draft path passes nil and
// attaches its files afterwards through upload sessions.
func buildExchangeMessage(recipient, subject, bodyHTML string, files []plugin.NotificationFile) map[string]any {
	message := map[string]any{
		"subject": subject,
		"body": map[string]string{
			"contentType": "HTML",
			"content":     bodyHTML,
		},
		"toRecipients": []map[string]any{
			{
				"emailAddress": map[string]string{
					"address": recipient,
				},
			},
		},
	}

	// The key is omitted entirely when there is nothing to attach:
	// Graph accepts an empty array, but leaving it out keeps the
	// no-attachment request byte-identical to what it was before.
	if len(files) > 0 {
		attachments := make([]map[string]any, 0, len(files))
		for _, f := range files {
			attachments = append(attachments, map[string]any{
				"@odata.type":  "#microsoft.graph.fileAttachment",
				"name":         sanitizeFilename(f.Filename),
				"contentType":  attachmentContentType(f),
				"contentBytes": base64.StdEncoding.EncodeToString(f.Data),
			})
		}
		message["attachments"] = attachments
	}
	return message
}

func sendExchangeMessage(ctx context.Context, token, fromAddress, recipient, subject, bodyHTML string, files []plugin.NotificationFile) error {
	endpoint := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", url.PathEscape(fromAddress))
	payload := map[string]any{"message": buildExchangeMessage(recipient, subject, bodyHTML, files)}
	_, err := callExchangeAPI(ctx, token, http.MethodPost, endpoint, payload)
	return err
}

// sendExchangeLargeMessage delivers a message whose attachments exceed
// what sendMail accepts. Graph offers no single call for this: the
// message has to exist as a draft first, each file is streamed into it
// through its own upload session, and the draft is then sent.
func sendExchangeLargeMessage(ctx context.Context, token, fromAddress, recipient, subject, bodyHTML string, files []plugin.NotificationFile) error {
	draftID, err := createExchangeDraft(ctx, token, fromAddress, recipient, subject, bodyHTML)
	if err != nil {
		return err
	}

	for _, f := range files {
		if err := uploadExchangeAttachment(ctx, token, fromAddress, draftID, f); err != nil {
			return withDiscardedDraft(ctx, token, fromAddress, draftID, err)
		}
	}

	if err := sendExchangeDraft(ctx, token, fromAddress, draftID); err != nil {
		return withDiscardedDraft(ctx, token, fromAddress, draftID, err)
	}
	return nil
}

func createExchangeDraft(ctx context.Context, token, fromAddress, recipient, subject, bodyHTML string) (string, error) {
	endpoint := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/messages", url.PathEscape(fromAddress))
	body, err := callExchangeAPI(ctx, token, http.MethodPost, endpoint, buildExchangeMessage(recipient, subject, bodyHTML, nil))
	if err != nil {
		return "", fmt.Errorf("creating draft: %w", err)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return "", fmt.Errorf("decoding draft response: %w", err)
	}
	if created.ID == "" {
		return "", errors.New("draft response carried no message id")
	}
	return created.ID, nil
}

func uploadExchangeAttachment(ctx context.Context, token, fromAddress, draftID string, file plugin.NotificationFile) error {
	name := sanitizeFilename(file.Filename)
	// An upload session is defined by the size it will receive, so a
	// zero-byte file has no valid session to open. Graph would answer
	// 400 anyway; saying it here names the file that caused it.
	if len(file.Data) == 0 {
		return fmt.Errorf("attachment %s is empty", name)
	}
	endpoint := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/messages/%s/attachments/createUploadSession",
		url.PathEscape(fromAddress), url.PathEscape(draftID),
	)
	payload := map[string]any{
		"AttachmentItem": map[string]any{
			"attachmentType": "file",
			"name":           name,
			"size":           len(file.Data),
			"contentType":    attachmentContentType(file),
		},
	}

	body, err := callExchangeAPI(ctx, token, http.MethodPost, endpoint, payload)
	if err != nil {
		return fmt.Errorf("creating upload session for %s: %w", name, err)
	}

	var session struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		return fmt.Errorf("decoding upload session for %s: %w", name, err)
	}
	if session.UploadURL == "" {
		return fmt.Errorf("upload session for %s carried no upload URL", name)
	}

	// Chunks go out sequentially against ranges computed here rather
	// than following Graph's nextExpectedRanges hints: those matter only
	// for resuming an interrupted session, and a failed send is retried
	// from the top by the caller.
	total := len(file.Data)
	for start := 0; start < total; start += exchangeUploadChunkSize {
		end := start + exchangeUploadChunkSize
		if end > total {
			end = total
		}
		if err := putExchangeChunk(ctx, session.UploadURL, file.Data[start:end], start, total); err != nil {
			return fmt.Errorf("uploading %s bytes %d-%d: %w", name, start, end-1, err)
		}
	}
	return nil
}

func putExchangeChunk(ctx context.Context, uploadURL string, chunk []byte, start, total int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(chunk))
	if err != nil {
		return fmt.Errorf("creating chunk request: %w", err)
	}
	// The upload URL is pre-authenticated — Microsoft's documentation
	// omits the Authorization header on these PUTs, so this is the one
	// request in this file that does not carry the bearer token.
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+len(chunk)-1, total))
	req.ContentLength = int64(len(chunk))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("chunk request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("upload returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("upload returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func sendExchangeDraft(ctx context.Context, token, fromAddress, draftID string) error {
	endpoint := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/messages/%s/send",
		url.PathEscape(fromAddress), url.PathEscape(draftID),
	)
	if _, err := callExchangeAPI(ctx, token, http.MethodPost, endpoint, nil); err != nil {
		return fmt.Errorf("sending draft: %w", err)
	}
	return nil
}

func discardExchangeDraft(ctx context.Context, token, fromAddress, draftID string) error {
	endpoint := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/messages/%s",
		url.PathEscape(fromAddress), url.PathEscape(draftID),
	)
	_, err := callExchangeAPI(ctx, token, http.MethodDelete, endpoint, nil)
	return err
}

// withDiscardedDraft cleans up a half-built draft so a failed send does
// not accumulate in the sender's Drafts folder, and folds a failed
// cleanup into the returned error rather than dropping it — an operator
// who has to remove the draft by hand needs to be told.
func withDiscardedDraft(ctx context.Context, token, fromAddress, draftID string, cause error) error {
	// Detached from ctx on purpose: the most likely reason to be here is
	// that ctx just expired mid-upload, and reusing it would guarantee
	// the cleanup fails too. The deadline keeps the detached call bounded.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), exchangeTimeout)
	defer cancel()

	if err := discardExchangeDraft(cleanupCtx, token, fromAddress, draftID); err != nil {
		return fmt.Errorf("%w (draft %s left in the mailbox: %v)", cause, draftID, err)
	}
	return cause
}

// callExchangeAPI performs one authenticated Graph request and returns
// the response body. Every Graph call here has the same shape — bearer
// token, optional JSON in, JSON or nothing out, an error that has to
// carry the status and Graph's own message — so it lives in one place
// rather than five.
func callExchangeAPI(ctx context.Context, token, method, endpoint string, payload any) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshaling exchange payload: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("creating exchange request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange request: %w", err)
	}
	defer resp.Body.Close()

	// Bounded: a draft response is a full message resource, larger than
	// the 1 KB an error needs but never unbounded.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		if readErr != nil {
			return nil, fmt.Errorf("exchange API error status %d", resp.StatusCode)
		}
		detail := body
		if len(detail) > 1024 {
			detail = detail[:1024]
		}
		return nil, fmt.Errorf("exchange API error status %d: %s", resp.StatusCode, string(detail))
	}
	if readErr != nil {
		return nil, fmt.Errorf("reading exchange response: %w", readErr)
	}
	return body, nil
}

func attachmentContentType(file plugin.NotificationFile) string {
	if contentType := strings.TrimSpace(file.ContentType); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
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
			return nil, fmt.Errorf("comm-exchange: invalid recipient address %q: %w", addr, err)
		}
		recipients = append(recipients, parsed.Address)
	}
	if len(recipients) == 0 {
		return nil, errors.New("comm-exchange: at least one recipient is required")
	}
	return recipients, nil
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

// sanitizeFilename strips path separators and newlines so a filename
// can never carry structure into the Graph payload or the header the
// recipient's client derives from it.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	if name == "" {
		return "attachment"
	}
	return name
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

func htmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	return value
}

func configMapFromExchangeConfig(cfg exchangeConfig) map[string]string {
	return map[string]string{
		"tenant_id":      cfg.TenantID,
		"client_id":      cfg.ClientID,
		"client_secret":  cfg.ClientSecret,
		"from_address":   cfg.FromAddress,
		"from_name":      cfg.FromName,
		"to":             strings.Join(cfg.Recipients, ","),
		"subject_prefix": cfg.SubjectPrefix,
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
