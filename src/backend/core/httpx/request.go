// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package httpx

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/config"
)

func RequestScheme(r *http.Request) string {
	if proto := forwardedProto(r); proto != "" {
		return proto
	}
	if r != nil && r.TLS != nil {
		return "https"
	}
	return "http"
}

func WebSocketScheme(r *http.Request) string {
	if RequestScheme(r) == "https" {
		return "wss"
	}
	return "ws"
}

func BaseURL(r *http.Request) string {
	return fmt.Sprintf("%s://%s", RequestScheme(r), r.Host)
}

func WebSocketBaseURL(r *http.Request) string {
	return fmt.Sprintf("%s://%s", WebSocketScheme(r), r.Host)
}

func ShouldSetSecureCookie(r *http.Request, cfg *config.Config) bool {
	if cfg != nil && cfg.ForceSecureCookies {
		return true
	}
	return RequestScheme(r) == "https"
}

func IsAllowedWebSocketOrigin(r *http.Request, cfg *config.Config, allowMissingOrigin bool) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return allowMissingOrigin
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}

	requestScheme := RequestScheme(r)
	if strings.EqualFold(originURL.Scheme, requestScheme) &&
		normalizeHostPort(originURL.Scheme, originURL.Host) == normalizeHostPort(requestScheme, r.Host) {
		return true
	}

	if cfg == nil {
		return false
	}

	for _, allowedOrigin := range cfg.AllowedOriginList() {
		allowedURL, err := url.Parse(allowedOrigin)
		if err != nil || allowedURL.Scheme == "" || allowedURL.Host == "" {
			continue
		}

		if strings.EqualFold(originURL.Scheme, allowedURL.Scheme) &&
			normalizeHostPort(originURL.Scheme, originURL.Host) == normalizeHostPort(allowedURL.Scheme, allowedURL.Host) {
			return true
		}
	}

	return false
}

func forwardedProto(r *http.Request) string {
	if r == nil {
		return ""
	}

	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if idx := strings.IndexByte(proto, ','); idx >= 0 {
		proto = strings.TrimSpace(proto[:idx])
	}

	switch strings.ToLower(proto) {
	case "https", "wss":
		return "https"
	case "http", "ws":
		return "http"
	default:
		return ""
	}
}

func normalizeHostPort(scheme, host string) string {
	u := &url.URL{
		Scheme: strings.ToLower(strings.TrimSpace(scheme)),
		Host:   strings.TrimSpace(host),
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return ""
	}

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http", "ws":
			port = "80"
		case "https", "wss":
			port = "443"
		}
	}

	if port == "" {
		return hostname
	}
	return hostname + ":" + port
}
