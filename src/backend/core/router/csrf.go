// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// samlACSPath matches the one route a browser legitimately POSTs cross-site: the IdP's form post to the SAML assertion consumer.
var samlACSPath = regexp.MustCompile(`^/api/identity-providers/[^/]+/acs/?$`)

// crossSiteGuard refuses unsafe requests a browser marks as cross-site or sends with a foreign Origin.
// Clients that send neither header (CLI, bearer tokens, agents, webhooks) pass, because a CSRF attack needs a browser and a browser always sends one.
func crossSiteGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if k := originKey(o); k != "" {
			allowed[k] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if crossSiteAllowed(r, allowed) {
				next.ServeHTTP(w, r)
				return
			}
			response.ForbiddenCode(w, r, i18n.ErrForbidden)
		})
	}
}

func crossSiteAllowed(r *http.Request, allowed map[string]struct{}) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if r.Method == http.MethodPost && samlACSPath.MatchString(r.URL.Path) {
		return true
	}

	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if site == "same-origin" || site == "none" {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if site == "" && origin == "" {
		return true
	}
	if origin == "" {
		return false
	}

	key := originKey(origin)
	if key == "" {
		return false
	}
	if _, ok := allowed[key]; ok {
		return true
	}
	// Without fetch metadata (plain-http non-localhost pages) the Origin must match the host the request was addressed to.
	return site == "" && key == originKey(httpx.RequestScheme(r)+"://"+r.Host)
}

// originKey normalises an origin to lowercase scheme://host[:port] with default ports dropped, or "" when it is not an origin.
func originKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" || (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + port
}
