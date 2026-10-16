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

// errSessionInvalid is GLPI's ERROR_SESSION_TOKEN_INVALID; the v1 backend answers it with one
// re-init and replay, because a PHP session can expire between two calls of one chain.
var errSessionInvalid = errors.New("comm-glpi: session token invalid")

// errUnauthorized marks any other 401, so the v2 backend can drop a cached bearer token and
// retry once before the operator sees the rejection.
var errUnauthorized = errors.New("GLPI rejected the credentials (HTTP 401)")

// statusError keeps only the HTTP status of a failed call, so a vendor body that echoes a
// credential never reaches a log line through the error chain.
type statusError struct{ Code int }

// Error implements error.
func (e *statusError) Error() string { return "http status " + strconv.Itoa(e.Code) }

// result is what a successful call reports beyond its decoded body.
type result struct {
	status int
	header http.Header
}

// do is the one HTTP path every GLPI call takes: it bounds time and body size, refuses
// redirects, and classifies the status so the host's outbox knows whether to retry, orphan or park.
func (c *Client) do(ctx context.Context, method, rawURL string, hdr http.Header, body, out any, secrets ...string) (result, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return result{}, fmt.Errorf("comm-glpi: encode request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return result{}, fmt.Errorf("comm-glpi: build request: %w", err)
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return result{}, fmt.Errorf("%w: comm-glpi: transport: %w", incident.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return result{}, fmt.Errorf("%w: comm-glpi: read response: %w", incident.ErrRetryable, err)
	}
	res := result{status: resp.StatusCode, header: resp.Header}
	if err := c.classify(resp, raw, secrets); err != nil {
		return res, err
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return res, fmt.Errorf("comm-glpi: decode response: %w", err)
		}
	}
	return res, nil
}

func (c *Client) classify(resp *http.Response, raw []byte, secrets []string) error {
	code := resp.StatusCode
	se := &statusError{Code: code}
	switch {
	case code >= 200 && code < 300:
		return nil
	case code >= 300 && code < 400:
		return operatorError("GLPI answered with a redirect (HTTP %d); set the GLPI URL to the final https address", code)
	case code == http.StatusTooManyRequests:
		return &incident.RetryAfterError{After: retryAfter(resp.Header.Get("Retry-After"), c.now())}
	case code >= 500:
		return fmt.Errorf("%w: %w", incident.ErrRetryable, se)
	case code == http.StatusNotFound:
		return fmt.Errorf("%w: %w", incident.ErrNotFound, se)
	}
	vcode, msg := vendorMessage(raw)
	msg = sanitize(msg, secrets...)
	switch {
	case code == http.StatusUnauthorized && vcode == "ERROR_SESSION_TOKEN_INVALID":
		return fmt.Errorf("%w: %w", errSessionInvalid, se)
	case code == http.StatusUnauthorized && vcode == "ERROR_GLPI_LOGIN_USER_TOKEN":
		return operatorError("GLPI rejected the user API token")
	case code == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", incident.ErrOperatorMessage, errUnauthorized)
	case msg != "":
		return operatorError("GLPI refused the request (HTTP %d): %s", code, msg)
	}
	return operatorError("GLPI refused the request (HTTP %d)", code)
}

// vendorMessage reads GLPI's error shapes: v1's ["ERROR_CODE","message"], v1 bulk
// [{"id":false,"message"}], the HL API's {status,title,detail} and OAuth's {error,error_description}.
func vendorMessage(raw []byte) (string, string) {
	raw = bytes.TrimSpace(raw)
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		var code, msg string
		if json.Unmarshal(arr[0], &code) == nil {
			if len(arr) > 1 && json.Unmarshal(arr[1], &msg) != nil {
				msg = ""
			}
			return code, msg
		}
		return "", firstMessage(raw)
	}
	var obj struct {
		Status           string          `json:"status"`
		Title            string          `json:"title"`
		Detail           json.RawMessage `json:"detail"`
		Error            string          `json:"error"`
		ErrorDescription string          `json:"error_description"`
		Message          string          `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		var detail string
		if json.Unmarshal(obj.Detail, &detail) != nil {
			detail = ""
		}
		parts := []string{}
		for _, p := range []string{obj.Title, detail, obj.ErrorDescription, obj.Message} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		code := obj.Status
		if code == "" {
			code = obj.Error
		}
		return code, strings.Join(parts, ": ")
	}
	if len(raw) > 0 && raw[0] != '<' {
		return "", string(raw)
	}
	return "", ""
}

// firstMessage returns the message of the first element of a v1 bulk answer.
func firstMessage(raw []byte) string {
	var rows []struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &rows) != nil {
		return ""
	}
	for _, r := range rows {
		if r.Message != "" {
			return r.Message
		}
	}
	return ""
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
