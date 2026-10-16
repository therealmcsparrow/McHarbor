// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	maxOperatorText = 200
	defaultRetry    = 30 * time.Second
)

// statusError keeps only the HTTP status of a failed call, so a PagerDuty body that echoes a
// routing key never reaches a log line through the error chain.
type statusError struct{ Code int }

// Error implements error.
func (e *statusError) Error() string { return "comm-pagerduty: http status " + strconv.Itoa(e.Code) }

// doJSON is the one HTTP path every vendor call takes: it bounds time and body size, and
// classifies the status so the host's outbox knows whether to retry, orphan or park.
func (c *Client) doJSON(ctx context.Context, method, rawURL string, hdr http.Header, body, out any, secrets ...string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("comm-pagerduty: encode request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return fmt.Errorf("comm-pagerduty: build request: %w", err)
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: comm-pagerduty: transport: %w", incident.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: comm-pagerduty: read response: %w", incident.ErrRetryable, err)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return &incident.RetryAfterError{After: retryAfter(resp.Header.Get("Retry-After"), c.now())}
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: %w", incident.ErrRetryable, &statusError{Code: resp.StatusCode})
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %w", incident.ErrNotFound, &statusError{Code: resp.StatusCode})
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Errorf("%w: %w: %s", incident.ErrOperatorMessage, &statusError{Code: resp.StatusCode}, pdValidationText(raw, secrets))
	case resp.StatusCode >= 400:
		return &statusError{Code: resp.StatusCode}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("comm-pagerduty: decode response: %w", err)
		}
	}
	return nil
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

// pdValidationText reads both PagerDuty error shapes, the Events API's top-level
// message/errors and REST's nested error object, and never returns the raw body.
func pdValidationText(raw []byte, secrets []string) string {
	type pdErr struct {
		Message string   `json:"message"`
		Errors  []string `json:"errors"`
	}
	var body struct {
		pdErr
		Error *pdErr `json:"error"`
	}
	const fallback = "PagerDuty rejected the request"
	if err := json.Unmarshal(raw, &body); err != nil {
		return fallback
	}
	e := body.pdErr
	if body.Error != nil {
		e = *body.Error
	}
	parts := make([]string, 0, len(e.Errors)+1)
	for _, p := range append([]string{e.Message}, e.Errors...) {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return sanitize(strings.Join(parts, "; "), secrets...)
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

func restHeaders(cfg pdConfig) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Token token="+cfg.RESTKey)
	h.Set("Accept", "application/vnd.pagerduty+json;version=2")
	return h
}
