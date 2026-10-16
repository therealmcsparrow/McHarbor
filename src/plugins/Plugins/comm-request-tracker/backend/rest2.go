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
	restPrefix      = "/REST/2.0"
)

// statusError keeps only the HTTP status of a failed call, so an RT body that echoes a
// credential never reaches a log line through the error chain.
type statusError struct{ Code int }

// Error implements error.
func (e *statusError) Error() string { return "http status " + strconv.Itoa(e.Code) }

// errNotJSON marks a 2xx answer that is not REST2 JSON, typically an SSO login page in front
// of RT, which retrying cannot fix.
var errNotJSON = errors.New("request_tracker: response is not REST2 JSON")

// do is the one HTTP path every RT call takes: it bounds time and body size, authenticates with
// the token, and classifies the status so the host's outbox knows whether to retry or park.
func (c *Client) do(ctx context.Context, cfg *config, method, path string, in, out any) (http.Header, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var rdr io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, cfg.base+restPrefix+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "token "+cfg.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: transport: %w", incident.ErrRetryable, redactErr(err, cfg.token))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", incident.ErrRetryable, err)
	}
	if err := c.classify(cfg, method, path, resp, raw); err != nil {
		return resp.Header, err
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.Header, fmt.Errorf("%w: %w: check that instance_url is RT's web root", incident.ErrOperatorMessage, errNotJSON)
		}
	}
	return resp.Header, nil
}

// classify maps an RT status to the host's outbox semantics; the operator texts name the field
// or the RT right to fix, because RT's own 403 does not say which right is missing.
func (c *Client) classify(cfg *config, method, path string, resp *http.Response, raw []byte) error {
	code := resp.StatusCode
	se := &statusError{Code: code}
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusTooManyRequests:
		return &incident.RetryAfterError{After: retryAfter(resp.Header.Get("Retry-After"), c.now())}
	case code >= 500:
		return fmt.Errorf("%w: %w", incident.ErrRetryable, se)
	case code >= 300 && code < 400:
		return fmt.Errorf("%w: %w: RT answered with a redirect; set instance_url to RT's exact https:// web root", incident.ErrOperatorMessage, se)
	case code == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w: auth token rejected", incident.ErrOperatorMessage, se)
	case code == http.StatusForbidden:
		return fmt.Errorf("%w: %w: the token's user lacks a right on queue %s (likely %s)", incident.ErrOperatorMessage, se, cfg.queue, likelyRight(method, path))
	case code == http.StatusNotFound:
		return fmt.Errorf("%w: %w", incident.ErrNotFound, se)
	case code == http.StatusBadRequest || code == http.StatusUnprocessableEntity:
		msg := rtMessage(raw, cfg.token)
		if strings.Contains(strings.ToLower(msg), "valid status") {
			return invalidStatus(cfg, msg)
		}
		return fmt.Errorf("%w: %w: %s", incident.ErrOperatorMessage, se, msg)
	}
	return se
}

func invalidStatus(cfg *config, msg string) error {
	return fmt.Errorf("%w: RT refused status %q on queue %s; set resolve_status to a status of that queue's lifecycle (RT said: %s)",
		incident.ErrOperatorMessage, cfg.resolveStatus, cfg.queue, msg)
}

// likelyRight names the RT right a 403 most probably lacks, from the call that got it.
func likelyRight(method, path string) string {
	p := path
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	switch {
	case strings.HasSuffix(p, "/comment"):
		return "CommentOnTicket"
	case method == http.MethodPost && p == "/ticket":
		return "CreateTicket"
	case method == http.MethodPut:
		return "ModifyTicket"
	case strings.HasPrefix(p, "/queue"):
		return "SeeQueue"
	case strings.Contains(p, "customfield"):
		return "SeeCustomField"
	}
	return "ShowTicket"
}

// rtMessage extracts REST2's {"message": "…"} and makes it safe for an operator.
func rtMessage(raw []byte, secrets ...string) string {
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &body); err == nil && body.Message != "" {
		return sanitize(body.Message, secrets...)
	}
	return sanitize(string(raw), secrets...)
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

// sanitize makes RT text safe for an operator: credentials redacted, control characters
// flattened, and the length capped so a Perl stack trace cannot fill the tickets table.
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

// redactedError carries a transport error's text with the token removed, while still
// unwrapping to the original so context deadlines stay detectable.
type redactedError struct {
	msg string
	err error
}

// Error implements error.
func (e *redactedError) Error() string { return e.msg }

// Unwrap exposes the original transport error to errors.Is.
func (e *redactedError) Unwrap() error { return e.err }

func redactErr(err error, token string) error {
	if token == "" || !strings.Contains(err.Error(), token) {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(err.Error(), token, "[redacted]"), err: err}
}

// rtID reads an RT id that REST2 renders as a number on create and read but as a string in
// collection items, so both shapes decode to the same decimal text.
type rtID string

// UnmarshalJSON accepts 77 and "77".
func (r *rtID) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*r = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return fmt.Errorf("decode id: %w", err)
		}
		*r = rtID(v)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("decode id: %w", err)
	}
	*r = rtID(n.String())
	return nil
}

// rtUser reads Owner, which REST2 renders as {"id":"jdoe",…} but older builds as a bare name.
type rtUser string

// UnmarshalJSON accepts {"id":"jdoe"} and "jdoe".
func (u *rtUser) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	switch {
	case s == "null":
		*u = ""
	case strings.HasPrefix(s, `"`):
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return fmt.Errorf("decode owner: %w", err)
		}
		*u = rtUser(v)
	default:
		var v struct {
			ID   rtID   `json:"id"`
			Name string `json:"Name"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return fmt.Errorf("decode owner: %w", err)
		}
		*u = rtUser(v.ID)
		if v.Name != "" {
			*u = rtUser(v.Name)
		}
	}
	return nil
}

// assignee hides RT's placeholder owner, which means "unowned" rather than a person.
func (u rtUser) assignee() string {
	if strings.EqualFold(string(u), "Nobody") {
		return ""
	}
	return string(u)
}

// collection is REST2's paged envelope.
type collection struct {
	Total int          `json:"total"`
	Count int          `json:"count"`
	Page  int          `json:"page"`
	Pages int          `json:"pages"`
	Items []ticketItem `json:"items"`
}

// ticketItem is a collection row with the projected fields the plugin asks for.
type ticketItem struct {
	ID          rtID   `json:"id"`
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	Owner       rtUser `json:"Owner"`
	LastUpdated string `json:"LastUpdated"`
	Resolved    string `json:"Resolved"`
}
