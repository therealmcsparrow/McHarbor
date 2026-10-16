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
	"unicode"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

const (
	callTimeout     = 30 * time.Second
	maxBodyBytes    = 4 << 20
	maxOperatorText = 300
	defaultRetry    = 30 * time.Second
)

// errRedirect stops the transport from following a redirect, so a compromised or misconfigured
// instance cannot bounce the token-bearing request to another host.
var errRedirect = errors.New("redirect refused")

// statusError keeps only the HTTP status of a failed call, so a vendor body that echoes a
// credential never reaches a log line through the error chain.
type statusError struct{ Code int }

// Error implements error.
func (e *statusError) Error() string { return "http status " + strconv.Itoa(e.Code) }

// zammadError is Zammad's error envelope; error_human is the sentence meant for people.
type zammadError struct {
	Error      string `json:"error"`
	ErrorHuman string `json:"error_human"`
}

// reqOpts carries the per-call extras that only some endpoints take.
type reqOpts struct {
	suppress bool
}

// do is the one HTTP path every Zammad call takes: it bounds time and body size, sets the
// token and From headers, and classifies the status for the host's outbox.
func (c *Client) do(ctx context.Context, cfg config, method, path string, body, out any) error {
	return c.doOpts(ctx, cfg, method, path, body, out, reqOpts{})
}

func (c *Client) doOpts(ctx context.Context, cfg config, method, path string, body, out any, opts reqOpts) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.base+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Token token="+cfg.token)
	if cfg.onBehalfOf != "" {
		req.Header.Set("From", cfg.onBehalfOf)
	}
	if opts.suppress && cfg.suppress {
		req.Header.Set("X-Zammad-Suppress-Notifications", "true")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if errors.Is(err, errRedirect) {
			return fmt.Errorf("%w: Zammad answered with a redirect; set the Zammad URL to the final https:// address", incident.ErrOperatorMessage)
		}
		return fmt.Errorf("%w: transport: %w", incident.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: read response: %w", incident.ErrRetryable, err)
	}
	if err := c.classify(resp, raw, cfg); err != nil {
		return err
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// classify maps a Zammad status to the outbox's vocabulary: retry, orphan, or park with a
// message the operator can act on.
func (c *Client) classify(resp *http.Response, raw []byte, cfg config) error {
	code := resp.StatusCode
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusTooManyRequests || code >= 500:
		if code == http.StatusTooManyRequests || resp.Header.Get("Retry-After") != "" {
			return &incident.RetryAfterError{After: retryAfter(resp.Header.Get("Retry-After"), c.now())}
		}
		return fmt.Errorf("%w: %w", incident.ErrRetryable, &statusError{Code: code})
	case code == http.StatusNotFound:
		return fmt.Errorf("%w: %w", incident.ErrNotFound, &statusError{Code: code})
	case code == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w: access token rejected (revoked or the token feature is disabled)", incident.ErrOperatorMessage, &statusError{Code: code})
	}
	var ze zammadError
	if json.Unmarshal(raw, &ze) != nil {
		ze = zammadError{}
	}
	text := ze.ErrorHuman
	if text == "" {
		text = ze.Error
	}
	notAuthorized := strings.Contains(strings.ToLower(ze.Error+" "+ze.ErrorHuman), "not authorized")
	switch {
	case code == http.StatusForbidden || (code == http.StatusUnprocessableEntity && notAuthorized):
		return fmt.Errorf("%w: %w: the token owner cannot create tickets in group %s", incident.ErrOperatorMessage, &statusError{Code: code}, sanitize(cfg.group, cfg.token))
	case code == http.StatusUnprocessableEntity || code == http.StatusBadRequest:
		if text == "" {
			text = "Zammad rejected the request"
		}
		return fmt.Errorf("%w: %w: %s", incident.ErrOperatorMessage, &statusError{Code: code}, sanitize(text, cfg.token, cfg.webhookSecret))
	}
	return &statusError{Code: code}
}

func retryAfter(v string, now time.Time) time.Duration {
	if v == "" {
		return defaultRetry
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now)
	}
	return defaultRetry
}

// sanitize makes vendor text safe for an operator: credentials redacted, control characters
// flattened, and the length capped so a stack trace cannot fill the tickets table.
func sanitize(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxOperatorText {
		s = string(r[:maxOperatorText]) + "…"
	}
	return s
}
