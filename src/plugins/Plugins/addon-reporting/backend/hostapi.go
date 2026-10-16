// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// hostAPI is a client for McHarbor's own REST API over loopback.
//
// Why go out through HTTP rather than read the database:
//
//  1. RBAC. Every request carries the calling user's session cookie, so
//     a report can only ever contain data that user could already see in
//     the UI. Reading tables directly would quietly bypass roles, ABAC
//     rules and environment scoping — in a product where "who can see
//     production" is a real question, that is not a trade worth making.
//  2. Scope. The plugin DB handle is deliberately fenced to
//     plugin_reporting_* tables, and that fence is right.
//  3. Shape. The API is the host's supported contract; its table layout
//     is not. A schema migration should not silently change a board
//     report.
//
// The cost is a few dozen loopback requests per report, which run
// concurrently and are individually deadlined.
type hostAPI struct {
	client *http.Client

	// cookie and lang are lifted from the request that asked for the
	// report, so the collector acts as that user.
	cookie string
	// authorization is the caller's Authorization header (an API key's
	// Bearer token), forwarded so a request that carries no session cookie
	// still collects as that user. It is only ever sent to the loopback
	// bases from bases(), never to another host.
	authorization string
	lang          string
	envID         string

	// baseURLs overrides the resolved loopback bases; only tests set it.
	baseURLs []string

	mu    sync.Mutex
	stats []SourceStatus
}

// resolveBaseURL works out where the host is listening.
//
// MCHARBOR_INTERNAL_URL is the escape hatch for unusual deployments.
// Otherwise the host binds PORT (default 8705) and may or may not have
// TLS enabled, so both schemes are tried once and the winner cached.
func resolveBaseURL() []string {
	if v := strings.TrimSpace(os.Getenv("MCHARBOR_INTERNAL_URL")); v != "" {
		return []string{strings.TrimRight(v, "/")}
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8705"
	}
	host := net.JoinHostPort("127.0.0.1", port)
	return []string{"https://" + host, "http://" + host}
}

func newHostAPI(cookie, lang, envID string) *hostAPI {
	return &hostAPI{
		cookie: cookie,
		lang:   lang,
		envID:  envID,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				// The only peer here is the host process on loopback, so
				// certificate verification is checking the process
				// against itself. A self-signed cert is the normal case
				// for a self-hosted install and must not break reporting.
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // loopback to this same process
				MaxIdleConnsPerHost: 8,
				DisableCompression:  false,
			},
		},
	}
}

// withAuthorization sets the caller's Authorization header on the client and
// returns it; an empty value leaves the client cookie-only.
func (h *hostAPI) withAuthorization(authorization string) *hostAPI {
	h.authorization = strings.TrimSpace(authorization)
	return h
}

// envelope is the host's standard response shape.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Code    string          `json:"code"`
}

// paged is the shape list endpoints return inside `data`.
type paged struct {
	Items      json.RawMessage `json:"items"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PerPage    int             `json:"per_page"`
	TotalPages int             `json:"total_pages"`
}

// get fetches one endpoint and unmarshals `data` into out.
//
// A 401/403/404 is not an error: it means this user cannot see this
// source, or the module is not present in this build. The report says so
// in its Methodology annex and carries on. Failing the whole report
// because one optional module is disabled would make it useless on most
// installs.
func (h *hostAPI) get(ctx context.Context, key, label, path string, params map[string]string, out any) bool {
	started := time.Now()
	ok, note := h.doGet(ctx, path, params, out)
	h.mu.Lock()
	h.stats = append(h.stats, SourceStatus{
		Key: key, Label: label, OK: ok, Note: note,
		DurationMS: time.Since(started).Milliseconds(),
	})
	h.mu.Unlock()
	return ok
}

func (h *hostAPI) doGet(ctx context.Context, path string, params map[string]string, out any) (bool, string) {
	q := url.Values{}
	if h.envID != "" {
		q.Set("env", h.envID)
	}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	suffix := ""
	if len(q) > 0 {
		suffix = "?" + q.Encode()
	}

	var lastNote string
	for _, base := range h.bases() {
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+path+suffix, nil)
		if err != nil {
			cancel()
			return false, "request build failed"
		}
		if h.cookie != "" {
			req.Header.Set("Cookie", h.cookie)
		}
		if h.authorization != "" {
			req.Header.Set("Authorization", h.authorization)
		}
		if h.lang != "" {
			req.Header.Set("Accept-Language", h.lang)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := h.client.Do(req)
		if err != nil {
			cancel()
			lastNote = "unreachable"
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		cancel()
		if readErr != nil {
			return false, "response truncated"
		}

		h.lockBase(base)
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return false, "not permitted for this user"
		case resp.StatusCode == http.StatusForbidden:
			return false, "not permitted for this user"
		case resp.StatusCode == http.StatusNotFound:
			return false, "not available on this installation"
		case resp.StatusCode >= 400:
			return false, fmt.Sprintf("host returned %d", resp.StatusCode)
		}

		var env envelope
		if err := json.Unmarshal(body, &env); err != nil {
			return false, "unreadable response"
		}
		if !env.Success {
			if env.Code != "" {
				return false, env.Code
			}
			return false, "request declined"
		}
		if out == nil || len(env.Data) == 0 {
			return true, ""
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			// Several list endpoints wrap their rows in a pagination
			// envelope; unwrap and retry before giving up.
			var p paged
			if json.Unmarshal(env.Data, &p) == nil && len(p.Items) > 0 {
				if err2 := json.Unmarshal(p.Items, out); err2 == nil {
					return true, ""
				}
			}
			return false, "unexpected response shape"
		}
		return true, ""
	}
	if lastNote == "" {
		lastNote = "unreachable"
	}
	return false, lastNote
}

// bases returns the candidate base URLs, preferring one that has already
// answered so only the first request pays for the scheme probe.
var (
	baseOnce   sync.Once
	baseList   []string
	baseWinner string
	baseMu     sync.RWMutex
)

func (h *hostAPI) bases() []string {
	if len(h.baseURLs) > 0 {
		return h.baseURLs
	}
	baseOnce.Do(func() { baseList = resolveBaseURL() })
	baseMu.RLock()
	w := baseWinner
	baseMu.RUnlock()
	if w != "" {
		return []string{w}
	}
	return baseList
}

func (h *hostAPI) lockBase(base string) {
	baseMu.Lock()
	if baseWinner == "" {
		baseWinner = base
	}
	baseMu.Unlock()
}

// getList fetches a paginated endpoint and unmarshals the items.
func (h *hostAPI) getList(ctx context.Context, key, label, path string, params map[string]string, out any) bool {
	p := map[string]string{"per_page": "100"}
	for k, v := range params {
		p[k] = v
	}
	return h.get(ctx, key, label, path, p, out)
}

// sources returns the collection outcomes, sorted for a stable annex.
func (h *hostAPI) sources() []SourceStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]SourceStatus, len(h.stats))
	copy(out, h.stats)
	return out
}
