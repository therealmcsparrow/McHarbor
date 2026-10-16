// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
	restPath        = "/webservices/rest.php?version=1.3"
)

// opError is an error whose text is safe for an operator; it reads cleanly in the Incidents
// table instead of carrying the sentinel's own prefix.
type opError struct{ msg string }

// Error implements error.
func (e *opError) Error() string { return e.msg }

// Unwrap makes errors.Is(err, incident.ErrOperatorMessage) true.
func (e *opError) Unwrap() error { return incident.ErrOperatorMessage }

func operatorError(format string, args ...any) error {
	return &opError{msg: fmt.Sprintf(format, args...)}
}

type restObject struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Class   string         `json:"class"`
	Key     json.Number    `json:"key"`
	Fields  map[string]any `json:"fields"`
}

// field returns a response attribute as text; iTop sends ids as strings or numbers by version.
func (o restObject) field(name string) string {
	switch v := o.Fields[name].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	}
	return ""
}

type restResponse struct {
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Objects map[string]restObject `json:"objects"`
}

// sorted returns the objects ordered by numeric key, because a JSON map has no order and the
// plugin must pick the same ticket every time.
func (r restResponse) sorted() []restObject {
	out := make([]restObject, 0, len(r.Objects))
	for _, o := range r.Objects {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].Key.String())
		b, _ := strconv.Atoi(out[j].Key.String())
		return a < b
	})
	return out
}

// oqlQuote turns any operator or ticket value into a single OQL string literal; backslash and
// quote are the only characters that can end the literal early.
func oqlQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	return "'" + r.Replace(s) + "'"
}

// rest posts one operation to rest.php; iTop reports every failure inside an HTTP 200 body,
// so both the top-level code and each object's code are checked.
func (c *Client) rest(ctx context.Context, cfg itopConfig, op map[string]any) (restResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	payload, err := json.Marshal(op)
	if err != nil {
		return restResponse{}, fmt.Errorf("comm-itop: encode: %w", err)
	}
	form := url.Values{"json_data": {string(payload)}}
	if cfg.AuthMode == "token" {
		form.Set("auth_token", cfg.Token)
	} else {
		form.Set("auth_user", cfg.User)
		form.Set("auth_pwd", cfg.Password)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL+restPath, strings.NewReader(form.Encode()))
	if err != nil {
		return restResponse{}, fmt.Errorf("comm-itop: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return restResponse{}, fmt.Errorf("comm-itop: post: %w: %w", incident.ErrRetryable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return restResponse{}, fmt.Errorf("comm-itop: read: %w: %w", incident.ErrRetryable, err)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return restResponse{}, &incident.RetryAfterError{After: retryAfter(resp.Header.Get("Retry-After"), c.now())}
	case resp.StatusCode >= 500:
		return restResponse{}, fmt.Errorf("comm-itop: status %d: %w", resp.StatusCode, incident.ErrRetryable)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return restResponse{}, operatorError("iTop refused the request (HTTP %d); check the credentials and the REST Services User profile", resp.StatusCode)
	case resp.StatusCode >= 300:
		return restResponse{}, operatorError("iTop answered HTTP %d; check that the iTop URL is the final address of the iTop root", resp.StatusCode)
	}
	var out restResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return restResponse{}, operatorError("iTop returned a non-JSON response (HTTP %d); check the iTop URL", resp.StatusCode)
	}
	secrets := cfg.secrets()
	switch out.Code {
	case 0:
	case 1:
		return out, operatorError("iTop rejected the credentials, or the user lacks the REST Services User profile")
	default:
		return out, operatorError("iTop error %d: %s", out.Code, sanitize(out.Message, secrets...))
	}
	for _, o := range out.sorted() {
		if o.Code != 0 {
			return out, operatorError("iTop refused %s: %s", sanitize(o.Class), sanitize(o.Message, secrets...))
		}
	}
	return out, nil
}

// get runs a core/get with an OQL key built by the caller from validated values only.
func (c *Client) get(ctx context.Context, cfg itopConfig, class, oql, fields string) ([]restObject, error) {
	resp, err := c.rest(ctx, cfg, map[string]any{
		"operation": "core/get", "class": class, "key": oql, "output_fields": fields,
	})
	if err != nil {
		return nil, err
	}
	return resp.sorted(), nil
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

// sanitize makes iTop text safe for an operator: credentials redacted, control characters
// flattened, and the length capped so a PHP stack trace cannot fill the tickets table.
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
