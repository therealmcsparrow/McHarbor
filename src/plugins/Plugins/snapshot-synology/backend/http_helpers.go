// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// reqWithCtx builds an *http.Request bound to ctx with a GET method.
// Used by every read-only DSM endpoint (login, apiVersion, listShares).
func reqWithCtx(ctx context.Context, method, rawURL string) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, method, rawURL, nil)
	return req
}

// reqWithCtxBody builds a POST/PATCH request with a form body. The
// body is consumed by the http client on send.
func reqWithCtxBody(ctx context.Context, method, rawURL string, body io.Reader) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, method, rawURL, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// urlQueryEscape returns url.QueryEscape(s). Extracted because the
// http package pulls it in but we want a one-liner at the call
// sites without importing net/url everywhere.
func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

// joinURL is unused right now but kept for future endpoints that
// need path-style joining. Kept exported for testability.
func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}