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
	"net/textproto"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// whatsappCloudBase is the canonical Meta Cloud API hostname
// (graph.facebook.com). Path versioning is appended per-request
// (/v21.0/<phone_number_id>/messages) because Meta revs the
// version on a separate cadence from the host. It is a variable only so
// tests can point the Cloud path at an httptest server.
var whatsappCloudBase = "https://graph.facebook.com"

// whatsappHTTPTimeout caps a single API round-trip. Test calls
// (a cheap GET or small POST) complete in ~300ms; Send calls in
// ~500ms over typical consumer connections. 10s gives plenty
// of slack without hanging the host on a wedged upstream.
const whatsappHTTPTimeout = 10 * time.Second

// whatsappClient is the NotificationAdapterClient the host
// dispatches to for the whatsapp ChannelType. Stateless: every
// method takes the channel row's decrypted config map directly.
//
// Concurrency: NotificationAdapterClient implementations must be
// safe for concurrent use — the host invokes Test + Send from
// multiple goroutines during workflow fan-out. whatsappClient has
// no shared state so this is automatic.
type whatsappClient struct{}

// NewWhatsappClient returns a fresh client. The plugin keeps no
// shared state; callers can construct as many as they want.
func NewWhatsappClient() *whatsappClient {
	return &whatsappClient{}
}

// ============================================================================
// NotificationAdapterClient: Test
// ============================================================================

// Test verifies the chosen connection method is reachable with
// the supplied credentials. Behavior depends on cfg["method"]:
//
//	cloud   — GET https://graph.facebook.com/v21.0/<phone_number_id>
//	          (returns the phone number resource if the access
//	          token is valid; 200 means the credential round-trips).
//	gateway — POST <gateway_url>/test with a minimal JSON body;
//	          the gateway is expected to return 2xx. We do not
//	          impose a body schema because every self-hosted
//	          gateway implements its own health route.
//	bot     — POST <business_api_url> with a probe body; the
//	          platform decides what counts as healthy. The D360
//	          family returns the contact list on GET <base>/contacts
//	          with D360-API-KEY; we use a HEAD with the API key
//	          header as the cheapest probe.
//	saas    — Same shape as bot: a small POST to <provider_url>
//	          with the provider_api_key. We don't send a real
//	          message; we only confirm the auth header is accepted.
//
// After the method-specific credential probe succeeds, Test sends a
// real message so the host's Test button confirms delivery rather
// than only connectivity.
func (c *whatsappClient) Test(ctx context.Context, cfg map[string]string) error {
	method := strings.ToLower(strings.TrimSpace(cfg["method"]))
	if method == "" {
		method = methodCloud
	}
	if err := requireRecipient(cfg); err != nil {
		return err
	}

	var err error
	switch method {
	case methodCloud:
		err = testCloud(ctx, cfg)
	case methodGateway:
		err = testGateway(ctx, cfg)
	case methodBot:
		err = testBot(ctx, cfg)
	case methodSaas:
		err = testSaaS(ctx, cfg)
	default:
		return fmt.Errorf("comm-whatsapp: unknown method %q (expected one of cloud, gateway, bot, saas)", method)
	}
	if err != nil {
		return err
	}

	return c.Send(ctx, cfg, plugin.NotificationMessage{
		Title:   "McHarbor test notification",
		Message: "This is a test notification sent from McHarbor to verify your WhatsApp channel configuration.",
		Level:   "info",
	})
}

// testCloud issues a GET against the Meta Cloud API for the
// configured phone_number_id. A 200 confirms the access token
// is live; a 401 / 400 with error_subcode 190 means the token
// was revoked.
func testCloud(ctx context.Context, cfg map[string]string) error {
	phoneID := strings.TrimSpace(cfg["phone_number_id"])
	token := strings.TrimSpace(cfg["access_token"])
	if phoneID == "" {
		return errors.New("comm-whatsapp: phone_number_id is required for the Cloud API method")
	}
	if token == "" {
		return errors.New("comm-whatsapp: access_token is required for the Cloud API method")
	}

	endpoint := fmt.Sprintf("%s/v21.0/%s?fields=id,display_phone_number,verified_name", whatsappCloudBase, phoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: build cloud probe: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: cloud probe request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("comm-whatsapp: cloud probe returned status %d: %s", resp.StatusCode, truncateWhatsappBody(body, 256))
	}
	return nil
}

// testGateway POSTs a minimal JSON body to <gateway_url>/test.
// The body is "{ \"ping\": true }" — the gateway returns 2xx if
// the bearer token is valid. Operators running a non-standard
// gateway that doesn't expose /test should map /test to their
// own health route.
func testGateway(ctx context.Context, cfg map[string]string) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["gateway_url"]), "/")
	token := strings.TrimSpace(cfg["gateway_token"])
	if base == "" {
		return errors.New("comm-whatsapp: gateway_url is required for the gateway method")
	}
	if token == "" {
		return errors.New("comm-whatsapp: gateway_token is required for the gateway method")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/test", bytes.NewReader([]byte(`{"ping":true}`)))
	if err != nil {
		return fmt.Errorf("comm-whatsapp: build gateway probe: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: gateway probe request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("comm-whatsapp: gateway probe returned status %d: %s", resp.StatusCode, truncateWhatsappBody(body, 256))
	}
	return nil
}

// testBot issues a HEAD against the business_api_url base path
// with the API key in the D360-API-KEY header. 360dialog and
// the Twilio-WhatsApp sandboxes both accept this; non-200 is
// treated as auth failure.
func testBot(ctx context.Context, cfg map[string]string) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["business_api_url"]), "/")
	key := strings.TrimSpace(cfg["api_key"])
	if base == "" {
		return errors.New("comm-whatsapp: business_api_url is required for the Business Bot method")
	}
	if key == "" {
		return errors.New("comm-whatsapp: api_key is required for the Business Bot method")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/contacts?limit=1", nil)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: build bot probe: %w", err)
	}
	req.Header.Set("D360-API-KEY", key)
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: bot probe request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("comm-whatsapp: bot probe returned status %d: %s", resp.StatusCode, truncateWhatsappBody(body, 256))
	}
	return nil
}

// testSaaS POSTs a minimal "dry-run" body to <provider_url> with
// the provider_api_key. Different SaaS providers authenticate
// differently; we use the most common shape (Authorization:
// Bearer <key>) which works for MessageBird, Gupshup, Plivo,
// and most other REST APIs. Twilio needs the Account SID as a
// basic-auth user — operators running Twilio should set
// provider_api_key to "<AccountSID>:<AuthToken>" and the SaaS
// path will switch to Basic auth.
func testSaaS(ctx context.Context, cfg map[string]string) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["provider_url"]), "/")
	key := strings.TrimSpace(cfg["provider_api_key"])
	from := strings.TrimSpace(cfg["provider_from"])
	if base == "" {
		return errors.New("comm-whatsapp: provider_url is required for the SaaS method")
	}
	if key == "" {
		return errors.New("comm-whatsapp: provider_api_key is required for the SaaS method")
	}
	if from == "" {
		return errors.New("comm-whatsapp: provider_from is required for the SaaS method")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/messages", bytes.NewReader([]byte(`{"dry_run":true}`)))
	if err != nil {
		return fmt.Errorf("comm-whatsapp: build saas probe: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	applySaaSAuth(req, key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: saas probe request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 400/422 ("dry_run not a valid field") still proves the
	// credential is accepted. 401/403 is the only true failure.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("comm-whatsapp: saas probe returned status %d: %s", resp.StatusCode, truncateWhatsappBody(body, 256))
	}
	return nil
}

// ============================================================================
// NotificationAdapterClient: Send
// ============================================================================

// Send delivers msg to the configured recipient via the method
// selected at channel-config time. The body shape differs per
// method, but every method treats the same Title/Message input:
//
//   - Title becomes the first line (bolded via WhatsApp *bold*
//     markdown).
//   - Message becomes the body.
//   - recipient_phone (E.164) becomes the to: number.
//
// Methods that need credentials (access_token, gateway_token,
// api_key, provider_api_key) MUST be set; Send returns a clear
// error before contacting the upstream.
func (c *whatsappClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	if msg.Title == "" && msg.Message == "" {
		return errors.New("comm-whatsapp: empty payload (title and message both blank)")
	}
	if err := requireRecipient(cfg); err != nil {
		return err
	}

	method := strings.ToLower(strings.TrimSpace(cfg["method"]))
	if method == "" {
		method = methodCloud
	}

	if len(msg.Files) > 0 && method != methodCloud {
		return errWhatsappAttachmentsUnsupported
	}

	switch method {
	case methodCloud:
		return sendCloud(ctx, cfg, msg)
	case methodGateway:
		return sendGateway(ctx, cfg, msg)
	case methodBot:
		return sendBot(ctx, cfg, msg)
	case methodSaas:
		return sendSaaS(ctx, cfg, msg)
	default:
		return fmt.Errorf("comm-whatsapp: unknown method %q (expected one of cloud, gateway, bot, saas)", method)
	}
}

// sendCloud posts to the Meta Cloud API
// (POST /v21.0/<phone_number_id>/messages). Payload uses the
// official "text" message shape (https://developers.facebook.com/
// docs/whatsapp/cloud-api/messages/text-messages).
func sendCloud(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	phoneID := strings.TrimSpace(cfg["phone_number_id"])
	token := strings.TrimSpace(cfg["access_token"])
	if phoneID == "" || token == "" {
		return errors.New("comm-whatsapp: cloud method requires phone_number_id and access_token")
	}

	body := map[string]any{
		"messaging_product": "whatsapp",
		"to":                strings.TrimSpace(cfg["recipient_phone"]),
		"type":              "text",
		"text": map[string]any{
			"body": renderWhatsappBody(msg),
		},
	}
	if err := postJSON(ctx,
		fmt.Sprintf("%s/v21.0/%s/messages", whatsappCloudBase, phoneID),
		map[string]string{"Authorization": "Bearer " + token},
		body, "cloud send"); err != nil {
		return err
	}
	for _, f := range msg.Files {
		if err := sendCloudFile(ctx, phoneID, token, strings.TrimSpace(cfg["recipient_phone"]), f); err != nil {
			return err
		}
	}
	return nil
}

// sendGateway POSTs {title, message, phone} to
// <gateway_url>/send. The body schema matches the most common
// Baileys / wa-gateway implementation; gateways that want a
// different shape should adapt on their side.
func sendGateway(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["gateway_url"]), "/")
	token := strings.TrimSpace(cfg["gateway_token"])
	if base == "" || token == "" {
		return errors.New("comm-whatsapp: gateway method requires gateway_url and gateway_token")
	}

	body := map[string]any{
		"phone":   strings.TrimSpace(cfg["recipient_phone"]),
		"title":   msg.Title,
		"message": msg.Message,
	}
	return postJSON(ctx, base+"/send",
		map[string]string{"Authorization": "Bearer " + token},
		body, "gateway send")
}

// sendBot POSTs a WhatsApp Business API text-message payload to
// the business_api_url. The D360-API-KEY header is set
// (360dialog convention) plus an Authorization Bearer fallback
// for platforms that prefer OAuth-style auth.
func sendBot(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["business_api_url"]), "/")
	key := strings.TrimSpace(cfg["api_key"])
	if base == "" || key == "" {
		return errors.New("comm-whatsapp: bot method requires business_api_url and api_key")
	}

	body := map[string]any{
		"to":   strings.TrimSpace(cfg["recipient_phone"]),
		"type": "text",
		"text": map[string]any{
			"body": renderWhatsappBody(msg),
		},
	}
	return postJSON(ctx, base+"/messages",
		map[string]string{
			"D360-API-KEY":  key,
			"Authorization": "Bearer " + key,
		},
		body, "bot send")
}

// sendSaaS POSTs to <provider_url>/messages. Different SaaS
// providers use different body shapes; we use a neutral
// {to, from, body} envelope that MessageBird, Gupshup, and Plivo
// all accept. Twilio operators must set provider_url to the
// Twilio API base and provider_api_key to "<SID>:<AuthToken>"
// (applySaaSAuth detects the colon and switches to Basic auth).
func sendSaaS(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	base := strings.TrimRight(strings.TrimSpace(cfg["provider_url"]), "/")
	key := strings.TrimSpace(cfg["provider_api_key"])
	from := strings.TrimSpace(cfg["provider_from"])
	if base == "" || key == "" || from == "" {
		return errors.New("comm-whatsapp: saas method requires provider_url, provider_api_key, and provider_from")
	}

	body := map[string]any{
		"to":   strings.TrimSpace(cfg["recipient_phone"]),
		"from": from,
		"body": renderWhatsappBody(msg),
	}

	req, err := buildJSONRequest(ctx, http.MethodPost, base+"/messages", body)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: build saas send: %w", err)
	}
	applySaaSAuth(req, key)

	return doWhatsappRequest(req, "saas send")
}

// ============================================================================
// Helpers
// ============================================================================

// requireRecipient returns an error when the channel row has no
// recipient_phone. Called by both Test and Send because the
// recipient is the one field every method needs.
func requireRecipient(cfg map[string]string) error {
	if strings.TrimSpace(cfg["recipient_phone"]) == "" {
		return errors.New("comm-whatsapp: recipient_phone is required (E.164 format, e.g. +14155551234)")
	}
	return nil
}

// renderWhatsappBody concatenates Title + Message into a single
// body string. WhatsApp's *bold* / _italic_ markdown is
// supported by every method, so we wrap the title in *…* so the
// subject renders bolded above the body.
func renderWhatsappBody(msg plugin.NotificationMessage) string {
	if msg.Title == "" {
		return msg.Message
	}
	if msg.Message == "" {
		return "*" + msg.Title + "*"
	}
	return "*" + msg.Title + "*\n\n" + msg.Message
}

// truncateWhatsappBody clamps an error body to `max` bytes so
// multi-MB upstream responses don't leak into logs or user-
// facing messages. Mirrors the same helper in comm-telegram.
func truncateWhatsappBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// buildJSONRequest serialises body as JSON and attaches the
// standard Content-Type header. Caller adds auth-specific
// headers before calling doWhatsappRequest.
func buildJSONRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// postJSON is a one-shot helper for the simple
// "build + send + check status" path used by cloud / gateway /
// bot methods. The saas path uses buildJSONRequest + doWhatsappRequest
// directly because it needs to swap auth schemes.
func postJSON(ctx context.Context, url string, headers map[string]string, body any, op string) error {
	req, err := buildJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: build %s: %w", op, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doWhatsappRequest(req, op)
}

// doWhatsappRequest executes req and validates the response. A
// 2xx is success; anything else is reported with the first
// 256 bytes of the response body so the operator can see the
// upstream's error description.
func doWhatsappRequest(req *http.Request, op string) error {
	cli := &http.Client{Timeout: whatsappHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("comm-whatsapp: %s request: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("comm-whatsapp: %s returned status %d: %s", op, resp.StatusCode, truncateWhatsappBody(respBody, 256))
	}
	return nil
}

// applySaaSAuth picks the auth scheme for the SaaS method.
// Bearer is the default; if the key contains a colon ("SID:Auth")
// we treat it as Twilio-style Basic auth credentials.
func applySaaSAuth(req *http.Request, key string) {
	if strings.Contains(key, ":") {
		req.SetBasicAuth(strings.SplitN(key, ":", 2)[0], strings.SplitN(key, ":", 2)[1])
		return
	}
	req.Header.Set("Authorization", "Bearer "+key)
}

// errWhatsappAttachmentsUnsupported is returned when a file is routed to a
// method other than the Cloud API, the only one with a documented media
// upload; failing loudly beats silently dropping a report.
var errWhatsappAttachmentsUnsupported = errors.New("comm-whatsapp: file attachments require the Cloud API connection method")

// sendCloudFile uploads f to /<phone_number_id>/media and then sends it to
// the recipient as a document by the returned media id, because the Cloud
// API cannot take file bytes inline on the messages endpoint.
func sendCloudFile(ctx context.Context, phoneID, token, to string, f plugin.NotificationFile) error {
	mediaID, err := uploadCloudMedia(ctx, phoneID, token, f)
	if err != nil {
		return err
	}
	return postJSON(ctx,
		fmt.Sprintf("%s/v21.0/%s/messages", whatsappCloudBase, phoneID),
		map[string]string{"Authorization": "Bearer " + token},
		map[string]any{
			"messaging_product": "whatsapp",
			"to":                to,
			"type":              "document",
			"document":          map[string]any{"id": mediaID, "filename": f.Filename},
		}, "cloud send file")
}

// uploadCloudMedia posts f as multipart/form-data to the media endpoint and
// returns the id WhatsApp assigns, which is valid for a message send for 30 days.
func uploadCloudMedia(ctx context.Context, phoneID, token string, f plugin.NotificationFile) (string, error) {
	contentType := f.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("messaging_product", "whatsapp"); err != nil {
		return "", fmt.Errorf("comm-whatsapp: build media upload: %w", err)
	}
	if err := mw.WriteField("type", contentType); err != nil {
		return "", fmt.Errorf("comm-whatsapp: build media upload: %w", err)
	}
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition", fmt.Sprintf("form-data; name=\"file\"; filename=%q", f.Filename))
	hdr.Set("Content-Type", contentType)
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return "", fmt.Errorf("comm-whatsapp: build media upload: %w", err)
	}
	if _, err := part.Write(f.Data); err != nil {
		return "", fmt.Errorf("comm-whatsapp: build media upload: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("comm-whatsapp: build media upload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v21.0/%s/media", whatsappCloudBase, phoneID), &buf)
	if err != nil {
		return "", fmt.Errorf("comm-whatsapp: build media upload: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: whatsappHTTPTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("comm-whatsapp: media upload request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("comm-whatsapp: media upload returned status %d: %s", resp.StatusCode, truncateWhatsappBody(raw, 256))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("comm-whatsapp: media upload returned no id: %s", truncateWhatsappBody(raw, 256))
	}
	return out.ID, nil
}
