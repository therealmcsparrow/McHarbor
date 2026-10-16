// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"net/url"
)

// reqWithCtxAuth builds a GET request with the TrueNAS bearer
// token attached. Reuses the helper from snapshot-synology's
// pattern but adds the Authorization header at construction so
// callers don't repeat it.
func reqWithCtxAuth(ctx context.Context, method, rawURL, apiKey string) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return req
}

// urlQueryEscape is duplicated here so this plugin compiles
// without a shared internal package. Cheap (single function).
func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}