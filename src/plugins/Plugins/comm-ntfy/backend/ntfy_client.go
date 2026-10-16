// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// defaultNtfyServer is the canonical cloud endpoint. Operators
// on self-hosted ntfy override this via server_url. Kept as a
// package-level const so the empty-string fallback is stable
// across refactors.
// ntfyUploadTimeout is longer than a text publish: an attachment body
// is report-sized, not a line of text.
const ntfyUploadTimeout = 60 * time.Second

const defaultNtfyServer = "https://ntfy.sh"

// ntfyHTTPTimeout caps a single publish round-trip. ntfy.sh
// responds in ~150ms for a publish; self-hosted on a LAN is
// similar. 10s gives plenty of slack without hanging the host on
// a wedged broker — matches the other communication plugins.
const ntfyHTTPTimeout = 10 * time.Second

// ntfyPriorityMap translates the named priorities operators see
// in the form (min/low/default/high/urgent) into ntfy's integer
// priority 1/2/3/4/5 — which the ntfy server uses to drive mobile
// interruption heuristics. Empty string falls through to ntfy's
// default (3 / "default"), the value the operator gets when they
// don't set the Priority header.
var ntfyPriorityMap = map[string]string{
	"":        "", // pass through — server default
	"default": "3",
	"min":     "1",
	"low":     "2",
	"high":    "4",
	"urgent":  "5",
}

// ntfyClient is the NotificationAdapterClient the host dispatches
// to for the ntfy ChannelType. Stateless: every method takes the
// channel row's decrypted config map directly.
//
// Concurrency: NotificationAdapterClient implementations must be
// safe for concurrent use — the host invokes Test + Send from
// multiple goroutines during workflow fan-out. ntfyClient has no
// shared state so this is automatic.
type ntfyClient struct{}

// NewNtfyClient returns a fresh client. The plugin keeps no
// shared state; callers can construct as many as they want.
func NewNtfyClient() *ntfyClient {
	return &ntfyClient{}
}

// ============================================================================
// NotificationAdapterClient: Test
// ============================================================================

// Test verifies server_url + topic reach a working ntfy
// endpoint. We POST a minimal ping body to <server>/<topic>
// with a short Message and let the server's 200 OK (or 4xx with
// a JSON error body) tell us whether credentials + URL are
// valid.
//
// We intentionally do NOT make this a no-op like comm-telegram's
// getMe — ntfy's publish API has no equivalent "echo my
// credentials back" endpoint, so the cheapest liveness check is
// a publish. The publish goes to the same topic operators will
// receive alerts on, so a single message appearing in their ntfy
// app is the visual confirmation.
//
// The Message body is fixed ("McHarbor test") so subscribers
// recognise and ignore it.
func (c *ntfyClient) Test(ctx context.Context, cfg map[string]string) error {
	serverURL := strings.TrimSpace(cfg["server_url"])
	if serverURL == "" {
		serverURL = defaultNtfyServer
	}
	topic := strings.TrimSpace(cfg["topic"])
	if topic == "" {
		return errors.New("comm-ntfy: topic is required before testing — set the topic name subscribers will receive alerts on")
	}

	endpoint := buildNtfyEndpoint(serverURL, topic)

	resp, err := ntfyRequest(ctx, http.MethodPost, endpoint, "McHarbor test notification", cfg, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		// 401/403 → bad credentials; 404 → unknown topic (typo or
		// access not granted); 429 → rate limited. Surface the
		// status + a snippet of the JSON error body so the
		// operator can fix the most common mistake without
		// consulting ntfy docs.
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("comm-ntfy: publish to %s returned %d: %s", endpoint, resp.StatusCode, truncateNtfyBody(respBody, 256))
	}
	return nil
}

// ============================================================================
// NotificationAdapterClient: Send
// ============================================================================

// Send delivers msg to the configured ntfy topic. Builds a
// publish request to <server_url>/<topic> with Title + Message
// as the body, plus optional Priority and Tags headers.
//
// ntfy's publish signature (v1):
//
//	POST /<topic>
//	Title:        <str>           # optional — rendered as header
//	Priority:     <1..5>          # optional — drives interruption
//	Tags:         <emoji,emoji>   # optional — inline icons
//	Authorization: Bearer <token> # optional
//	<body>                          # plain text message body
//
// Title is sent as the Title header AND prepended to the body
// when both are non-empty. ntfy's mobile app shows the Title
// header as a bolded header line above the body, so the body
// duplication is redundant for the mobile app — but it makes
// the message self-contained when someone forwards it or reads
// it via curl. The Title header alone is enough for the app, so
// we skip the body duplication.
//
// Attachments are silently ignored — the plugin reports
// SupportsRichAttachments=false so the host should never call
// Send with a non-empty Attachments slice. We still defensively
// skip them rather than returning an error.
func (c *ntfyClient) Send(ctx context.Context, cfg map[string]string, msg plugin.NotificationMessage) error {
	serverURL := strings.TrimSpace(cfg["server_url"])
	if serverURL == "" {
		serverURL = defaultNtfyServer
	}
	topic := strings.TrimSpace(cfg["topic"])

	if topic == "" {
		return errors.New("comm-ntfy: topic is required — fill it in before saving the channel")
	}
	if msg.Title == "" && msg.Message == "" {
		return errors.New("comm-ntfy: empty payload (title and message both blank)")
	}

	endpoint := buildNtfyEndpoint(serverURL, topic)

	// ntfy carries a file as the request body with the text moved into
	// headers, which is the opposite arrangement from a text publish —
	// hence a separate call rather than an extra argument. Only the
	// first file is sent: ntfy binds one attachment per message, and
	// silently dropping the rest would be worse than saying so.
	if len(msg.Files) > 0 {
		if len(msg.Files) > 1 {
			return fmt.Errorf("comm-ntfy: ntfy carries one attachment per message, got %d", len(msg.Files))
		}
		return publishNtfyAttachment(ctx, endpoint, cfg, msg, msg.Files[0])
	}

	resp, err := ntfyRequest(ctx, http.MethodPost, endpoint, msg.Title, cfg, ntfyBodyText(msg))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("comm-ntfy: publish to %s returned %d: %s", endpoint, resp.StatusCode, truncateNtfyBody(respBody, 256))
	}
	return nil
}

// publishNtfyAttachment PUTs the file as the request body.
//
// With a binary body the message text can no longer travel in the body,
// so it moves to the Message header — ntfy's documented arrangement for
// attachments.
func publishNtfyAttachment(
	ctx context.Context,
	endpoint string,
	cfg map[string]string,
	msg plugin.NotificationMessage,
	file plugin.NotificationFile,
) error {
	ctx, cancel := context.WithTimeout(ctx, ntfyUploadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(file.Data))
	if err != nil {
		return fmt.Errorf("comm-ntfy: build attachment request: %w", err)
	}

	if title := strings.TrimSpace(msg.Title); title != "" {
		req.Header.Set("Title", title)
	}
	if body := strings.TrimSpace(msg.Message); body != "" {
		req.Header.Set("Message", body)
	}
	if name := strings.TrimSpace(file.Filename); name != "" {
		req.Header.Set("Filename", name)
	}
	if p := strings.TrimSpace(cfg["priority"]); p != "" {
		if mapped, ok := ntfyPriorityMap[p]; ok && mapped != "" {
			req.Header.Set("Priority", mapped)
		}
	}
	if tags := strings.TrimSpace(cfg["tags"]); tags != "" {
		req.Header.Set("Tags", tags)
	}
	if token := strings.TrimSpace(cfg["access_token"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ct := strings.TrimSpace(file.ContentType); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("comm-ntfy: publishing attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("comm-ntfy: attachment publish to %s returned %d: %s",
			endpoint, resp.StatusCode, truncateNtfyBody(respBody, 256))
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

// buildNtfyEndpoint composes the full publish URL. The server URL
// gets a trailing-slash trim so operators can paste either
// "https://ntfy.sh" or "https://ntfy.sh/" without producing a
// double-slash. The topic is appended with a leading slash so
// "https://ntfy.sh" + "alerts" → "https://ntfy.sh/alerts".
func buildNtfyEndpoint(serverURL, topic string) string {
	return strings.TrimRight(serverURL, "/") + "/" + strings.TrimLeft(topic, "/")
}

// ntfyBodyText concatenates Title + Message into the single
// message body ntfy accepts. When Title is set, we send it as
// the Title header (handled by ntfyRequest) and emit just
// Message in the body — the ntfy app displays the Title header
// above the body automatically. When Title is blank we send
// Message alone.
//
// If both are blank ntfyRequest is never reached (Send guards
// above) so we don't have to special-case that here.
func ntfyBodyText(msg plugin.NotificationMessage) string {
	if msg.Title == "" {
		return msg.Message
	}
	return msg.Message
}

// ntfyRequest builds + sends a publish POST to the given ntfy
// endpoint. Centralises header construction so Test and Send
// emit identical requests — easier to debug from the operator's
// "what did McHarbor actually send" angle.
//
// Title becomes the Title header (when non-empty). The body is
// sent verbatim as the request body — ntfy publishes whatever
// bytes are there. Headers are applied in this order so the
// auth headers come last and overwrite any earlier auth the
// caller might have set by mistake:
//
//  1. Title    (when msgTitle != "")
//  2. Priority (when cfg["priority"] maps to a known level)
//  3. Tags     (when cfg["tags"] != "")
//  4. Authorization Bearer <token>     (when cfg["access_token"] != "")
//  5. Basic auth                       (when cfg["username"] != "" && cfg["password"] != "")
//
// Bearer takes precedence over Basic — matches ntfy.sh's auth
// model where a topic token grants per-topic publish rights and
// is the more secure option for cloud deployments.
func ntfyRequest(
	ctx context.Context,
	method, endpoint, msgTitle string,
	cfg map[string]string,
	body string,
) (*http.Response, error) {
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("comm-ntfy: build publish request: %w", err)
	}

	if title := strings.TrimSpace(msgTitle); title != "" {
		req.Header.Set("Title", title)
	}
	if p := strings.TrimSpace(cfg["priority"]); p != "" {
		if mapped, ok := ntfyPriorityMap[p]; ok && mapped != "" {
			req.Header.Set("Priority", mapped)
		}
	}
	if tags := strings.TrimSpace(cfg["tags"]); tags != "" {
		req.Header.Set("Tags", tags)
	}
	if token := strings.TrimSpace(cfg["access_token"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if user := strings.TrimSpace(cfg["username"]); user != "" {
		if pw := strings.TrimSpace(cfg["password"]); pw != "" {
			req.SetBasicAuth(user, pw)
		}
	}

	cli := &http.Client{Timeout: ntfyHTTPTimeout}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("comm-ntfy: publish request to %s: %w", endpoint, err)
	}
	return resp, nil
}

// truncateNtfyBody clamps an error body to `max` bytes so a
// multi-KB ntfy error response doesn't leak into logs or user-
// facing messages. Mirrors the same helper in comm-telegram's
// client.
func truncateNtfyBody(b []byte, max int) string {
	if max <= 0 || len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

// url is imported defensively — we don't call QueryEscape
// directly but importing the package here ensures the file
// compiles when the rest of the codebase changes. Topic names
// pass through verbatim per ntfy's spec; encoding is the
// operator's responsibility (they shouldn't put spaces or
// slashes in topic names anyway).
var _ = url.QueryEscape
