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
	"net/url"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// signalClient is the runtime implementation of
// plugin.NotificationAdapterClient for the Signal channel. It is
// stateless — every call gets a freshly-allocated client through
// NewSignalClient. All HTTP work is bounded by a context timeout so
// a wedged Signal gateway can't pin a goroutine forever.
//
// Per-method dispatch mirrors the four buttons in the UMD
// ConfigForm:
//
//	rest    → POST <server>/v2/send             (signal-cli-rest-api)
//	bot     → POST <server>/message              (token-based bot)
//	signald → POST <server>/v1/accounts/<acc>/rpc (JSON-RPC 2.0 over HTTP)
//	simple  → POST <server>/send                 (no-auth relay)
type signalClient struct {
	// httpClient is shared across calls. Default transport is fine;
	// signal gateways live on a LAN and rarely need tuning.
	httpClient *http.Client
}

// NewSignalClient is the constructor the host calls from
// RegisterAdapters. Returns a fresh, stateless client.
func NewSignalClient() plugin.NotificationAdapterClient {
	return &signalClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --- Test ------------------------------------------------------------

// Test performs a cheap credential round-trip without sending an
// actual message. The signal-cli-rest-api exposes GET /v1/health
// which returns 204 when the daemon is reachable and authenticated;
// signald exposes GET /v1/accounts which lists configured accounts;
// bot servers vary but most respond 200 to a GET on the root.
//
// We don't hardcode a probe path because every gateway differs —
// the right probe is just "can we reach the server at all?" — so
// Test resolves <server_url> with HEAD and accepts 2xx or 3xx, then
// sends a real test message so the host's Test button confirms
// delivery rather than only connectivity.
func (c *signalClient) Test(ctx context.Context, cfg map[string]string) error {
	method, serverURL, _, _, err := signalRequireCommon(cfg)
	if err != nil {
		return err
	}

	// Method-specific extra checks so a misconfigured bot token /
	// signald account is caught at Test time, not at send time.
	switch method {
	case methodRest:
		// signal-cli-rest-api has /v1/health when auth is satisfied.
		err = c.signalHead(ctx, serverURL, "/v1/health", "", "")
	case methodBot:
		// Token-based servers usually expose GET / with no auth
		// required; the token is checked when actually posting.
		err = c.signalHead(ctx, serverURL, "/", "Bearer "+cfg["bot_token"], "")
	case methodSignald:
		acc := strings.TrimSpace(cfg["account_id"])
		if acc == "" {
			return fmt.Errorf("signald method requires account_id")
		}
		err = c.signalHead(ctx, serverURL, "/v1/accounts/"+url.PathEscape(acc), "", "")
	case methodSimple:
		// No credentials — just confirm the relay is up.
		err = c.signalHead(ctx, serverURL, "/", "", "")
	default:
		return fmt.Errorf("unknown signal connection method %q", method)
	}
	if err != nil {
		return err
	}

	return c.Send(ctx, cfg, plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification sent from McHarbor to verify your Signal channel configuration.",
		Level:   "info",
	})
}

// signalHead does an HTTP HEAD with the given auth header and
// returns nil for 2xx/3xx, an error otherwise. Shared by Test and
// as the connectivity-check side of Send.
func (c *signalClient) signalHead(ctx context.Context, base, path, authHeader, basicAuth string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	full := strings.TrimRight(base, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, full, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if basicAuth != "" {
		req.Header.Set("Authorization", basicAuth)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD %s: %w", full, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
}

// --- Send ------------------------------------------------------------

// Send delivers a single Signal message to every recipient listed
// in cfg["recipients"] (comma-separated). The plugin sends one
// request per recipient so a partial failure (one bad number)
// doesn't drop the whole batch.
//
// The Title is rendered as a Markdown-styled first line for
// signal-cli-rest-api and the bot method, since both accept text
// bodies. signald uses the same field but doesn't render Markdown;
// the receiver shows the asterisks literally. We accept that —
// signald users are sophisticated enough to strip them.
// Send delivers msg to every recipient listed in cfg["recipients"]
// (comma-separated). The plugin sends one request per recipient so
// a partial failure (one bad number) doesn't drop the whole batch.
//
// The Title is rendered as a bold first line via signal-cli-rest-api's
// Markdown style (`*title*`); the Bot and signald transports render
// the asterisks literally, but the title remains readable at the top
// of the body so alert subjects still pop in the conversation.
func (c *signalClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	if msg.Title == "" && msg.Message == "" {
		return errors.New("comm-signal: empty payload (title and message both blank)")
	}
	method, serverURL, sender, recipients, err := signalRequireCommon(cfg)
	if err != nil {
		return err
	}

	body := renderSignalBody(msg.Title, msg.Message)
	for _, recipient := range recipients {
		if err := c.sendOne(ctx, method, serverURL, cfg, sender, recipient, body, msg.Files); err != nil {
			return fmt.Errorf("send to %s: %w", recipient, err)
		}
	}
	return nil
}

// sendOne hits the per-method endpoint with a single recipient.
// Returns the transport-level error if any. The caller wraps the
// recipient into the message so the user can tell which number
// bounced.
func (c *signalClient) sendOne(ctx context.Context, method, serverURL string, cfg map[string]string, sender, recipient, body string, files []plugin.NotificationFile) error {
	if len(files) > 0 && method != methodRest {
		return errSignalAttachmentsUnsupported
	}
	switch method {
	case methodRest:
		return c.sendViaRest(ctx, serverURL, cfg, recipient, body, files)
	case methodBot:
		return c.sendViaBot(ctx, serverURL, cfg, recipient, body)
	case methodSignald:
		return c.sendViaSignald(ctx, serverURL, cfg, sender, recipient, body)
	case methodSimple:
		return c.sendViaSimple(ctx, serverURL, recipient, body)
	default:
		return fmt.Errorf("unknown signal connection method %q", method)
	}
}

// sendViaRest — POST <server>/v2/send with optional HTTP Basic auth.
//
// signal-cli-rest-api v2 body shape:
//
//	{
//	  "message":  "<body text>",
//	  "number":   "<sender E.164>",
//	  "recipients": ["<recipient E.164>"],
//	  "base64_attachments": ["data:<mime>;filename=<name>;base64,<data>"]  (only when files are sent)
//	}
func (c *signalClient) sendViaRest(ctx context.Context, serverURL string, cfg map[string]string, recipient, body string, files []plugin.NotificationFile) error {
	payload := map[string]any{
		"message":    body,
		"number":     strings.TrimSpace(cfg["sender_number"]),
		"recipients": []string{recipient},
	}
	if len(files) > 0 {
		payload["base64_attachments"] = signalAttachmentURIs(files)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/v2/send", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if user, pass := strings.TrimSpace(cfg["auth_username"]), cfg["auth_password"]; user != "" {
		req.Header.Set("Authorization", "Basic "+basicAuth(user, pass))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return signalCheckResponse(resp, "signal-cli-rest-api")
}

// sendViaBot — POST <server>/message with Bearer auth.
//
// Token-based bot servers (e.g. community Discord-bridge bots that
// also speak Signal) typically accept:
//
//	{ "channel": "<E.164>", "content": "<text>", "token": "<..." }
//
// We send the canonical shape plus the token in both header AND
// body for compatibility with bots that check either side.
func (c *signalClient) sendViaBot(ctx context.Context, serverURL string, cfg map[string]string, recipient, body string) error {
	payload := map[string]any{
		"channel": recipient,
		"content": body,
		"token":   cfg["bot_token"],
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/message", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg["bot_token"])

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return signalCheckResponse(resp, "signal-bot")
}

// sendViaSignald — POST <server>/v1/accounts/<account>/rpc as a
// JSON-RPC 2.0 envelope wrapping the `send` method.
//
// signald's send method body shape (per signald-protocol v1):
//
//	{ "method": "send",
//	  "params": { "account": "<account>", "recipientNumber": "<...>",
//	              "messageBody": "<text>" },
//	  "id":     "<uuid>",
//	  "jsonrpc":"2.0" }
func (c *signalClient) sendViaSignald(ctx context.Context, serverURL string, cfg map[string]string, sender, recipient, body string) error {
	account := strings.TrimSpace(cfg["account_id"])
	if account == "" {
		return fmt.Errorf("signald method requires account_id")
	}

	envelope := map[string]any{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("mcharbor-%d", time.Now().UnixNano()),
		"method":  "send",
		"params": map[string]any{
			"account":         account,
			"recipientNumber": recipient,
			"messageBody":     body,
		},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshalling envelope: %w", err)
	}

	urlPath := "/v1/accounts/" + url.PathEscape(account) + "/rpc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+urlPath, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return signalCheckResponse(resp, "signald")
}

// sendViaSimple — POST <server>/send with no auth. Community
// relays accept a minimal body:
//
//	{ "to": "<E.164>", "message": "<text>" }
func (c *signalClient) sendViaSimple(ctx context.Context, serverURL string, recipient, body string) error {
	payload := map[string]any{
		"to":      recipient,
		"message": body,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/send", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return signalCheckResponse(resp, "simple-relay")
}

// --- Helpers ---------------------------------------------------------

// signalRequireCommon validates the fields every method depends on
// and returns the parsed values. The cfg map is decrypted by the
// host before Test / Send — secret-typed fields are returned as
// plaintext, which is what the gateway needs.
func signalRequireCommon(cfg map[string]string) (method, serverURL, sender string, recipients []string, err error) {
	method = strings.TrimSpace(cfg["method"])
	if method == "" {
		return "", "", "", nil, fmt.Errorf("missing required field: method")
	}
	serverURL = strings.TrimSpace(cfg["server_url"])
	if serverURL == "" {
		return "", "", "", nil, fmt.Errorf("missing required field: server_url")
	}
	sender = strings.TrimSpace(cfg["sender_number"])
	if sender == "" {
		return "", "", "", nil, fmt.Errorf("missing required field: sender_number")
	}
	raw := strings.TrimSpace(cfg["recipients"])
	if raw == "" {
		return "", "", "", nil, fmt.Errorf("missing required field: recipients")
	}
	for _, r := range strings.Split(raw, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			recipients = append(recipients, r)
		}
	}
	if len(recipients) == 0 {
		return "", "", "", nil, fmt.Errorf("recipients list is empty after trimming")
	}
	return method, serverURL, sender, recipients, nil
}

// renderSignalBody concatenates Title + Message into the format
// the gateways expect. Title is bolded with signal-cli-rest-api's
// Markdown style so the alert header pops in the conversation.
func renderSignalBody(title, message string) string {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	if title == "" {
		return truncateSignalBody(message)
	}
	if message == "" {
		return truncateSignalBody(title)
	}
	return truncateSignalBody("*" + title + "*\n" + message)
}

// truncateSignalBody caps the body at a sensible size for Signal
// messages. Signal's text limit is generous, but a runaway
// workflow that inlines a 5 MB log will make the gateway sad.
func truncateSignalBody(s string) string {
	const max = 4000
	if len(s) <= max {
		return s
	}
	return s[:max-32] + "\n[…truncated by McHarbor]"
}

// basicAuth encodes user:pass into the Basic auth header value.
// Kept as a helper so we don't need an extra import for the
// one-liner.
func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

// signalCheckResponse validates the HTTP status and surfaces a
// readable error otherwise. Up to 2 KiB of the response body is
// included in the error so operators can see what the gateway
// actually complained about.
func signalCheckResponse(resp *http.Response, label string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("%s returned HTTP %d: %s", label, resp.StatusCode, strings.TrimSpace(string(body)))
}

// errSignalAttachmentsUnsupported is returned when a file is routed to a
// connection method other than the REST API, the only one whose send
// endpoint accepts attachments; failing loudly beats silently dropping a report.
var errSignalAttachmentsUnsupported = errors.New("comm-signal: file attachments require the Signal REST API connection method")

// signalAttachmentURIs encodes files in the data-URI form signal-cli-rest-api
// expects in base64_attachments. The filename is stripped of ';' and ','
// because either would end the URI's parameter list early.
func signalAttachmentURIs(files []plugin.NotificationFile) []string {
	uris := make([]string, 0, len(files))
	for _, f := range files {
		contentType := f.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		name := strings.NewReplacer(";", "_", ",", "_").Replace(f.Filename)
		uris = append(uris, "data:"+contentType+";filename="+name+";base64,"+base64.StdEncoding.EncodeToString(f.Data))
	}
	return uris
}
