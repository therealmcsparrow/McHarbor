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
	vendorTimeout = 30 * time.Second
	maxBody       = 4 << 20
	maxOperator   = 200
)

var errRouteMissing = errors.New("comm-otrs: web service route missing")

type giEnvelope struct {
	Error *struct {
		ErrorCode    string `json:"ErrorCode"`
		ErrorMessage string `json:"ErrorMessage"`
	} `json:"Error"`
}

func (c *Client) post(ctx context.Context, cfg config, method, route string, body map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, vendorTimeout)
	defer cancel()
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("comm-otrs: encode %s: %w", route, err)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.Endpoint+route, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("comm-otrs: build %s: %w", route, err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("comm-otrs: %s %s: %w: %w", method, route, incident.ErrRetryable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("comm-otrs: read %s: %w: %w", route, incident.ErrRetryable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("comm-otrs: %s %s: %w", method, route, statusError(resp.StatusCode, resp.Header.Get("Retry-After")))
	}
	var env giEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("%w: %s did not return JSON; is the web service imported with this route mapping?", incident.ErrOperatorMessage, route)
	}
	if env.Error != nil {
		sid, _ := body["SessionID"].(string)
		return giError(env.Error.ErrorCode, redact(env.Error.ErrorMessage, cfg.Password, sid))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("comm-otrs: decode %s: %w", route, err)
	}
	return nil
}

func statusError(code int, retryAfter string) error {
	switch {
	case code == http.StatusTooManyRequests || code >= 500:
		if d, ok := parseRetryAfter(retryAfter, time.Now()); ok {
			return &incident.RetryAfterError{After: d}
		}
		return fmt.Errorf("status %d: %w", code, incident.ErrRetryable)
	case code == http.StatusNotFound:
		return fmt.Errorf("status 404 (web service or route missing): %w: %w", errRouteMissing, incident.ErrNotFound)
	case code == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%w: status 413: the ticket body exceeds the web service MaxLength", incident.ErrOperatorMessage)
	default:
		return fmt.Errorf("status %d", code)
	}
}

func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if s, err := strconv.Atoi(v); err == nil {
		return time.Duration(s) * time.Second, s > 0
	}
	if t, err := http.ParseTime(v); err == nil && t.After(now) {
		return t.Sub(now), true
	}
	return 0, false
}

func giError(code, msg string) error {
	code = sanitize(code)
	switch {
	case strings.HasSuffix(code, ".AuthFail") || strings.HasSuffix(code, ".SessionInvalid"):
		return fmt.Errorf("%s: %w", code, errAuthFail)
	case strings.HasSuffix(code, ".NotValidTicketID"):
		return fmt.Errorf("%s: %w", code, incident.ErrNotFound)
	case strings.HasSuffix(code, ".AccessDenied"):
		return fmt.Errorf("%w: %s: the McHarbor agent lacks permission on this ticket or queue", incident.ErrOperatorMessage, code)
	case strings.HasSuffix(code, ".MissingParameter"), strings.HasSuffix(code, ".InvalidParameter"):
		return fmt.Errorf("%w: %s: %s", incident.ErrOperatorMessage, code, sanitize(msg))
	default:
		return fmt.Errorf("comm-otrs: web service error %s", code)
	}
}

func redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	return s
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > maxOperator {
		s = string(r[:maxOperator])
	}
	return s
}
