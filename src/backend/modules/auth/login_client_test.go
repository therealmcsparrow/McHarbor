// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func appWithTrustedProxies(t *testing.T, cidrs ...string) *router.AppDeps {
	t.Helper()
	resolver, err := httpx.NewClientAddressResolver(cidrs)
	if err != nil {
		t.Fatal(err)
	}
	return &router.AppDeps{ResolveClientAddress: func(r *http.Request) (string, string, string) {
		addr := resolver.Resolve(r)
		return addr.PeerIP, addr.ClientIP, addr.Source
	}}
}

func forgedLogin(peer string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req.RemoteAddr = peer
	req.Header.Set("X-Forwarded-For", "192.0.2.77")
	req.Header.Set("X-Real-IP", "192.0.2.78")
	req.Header.Set("User-Agent", "probe/1.0")
	return req
}

func TestLoginClientIgnoresForgedForwardingFromUntrustedPeer(t *testing.T) {
	app := appWithTrustedProxies(t, "10.0.0.0/8")
	got := resolveLoginClient(app, forgedLogin("198.51.100.7:4444"))
	if got.IP != "198.51.100.7" || got.Source != httpx.ClientAddressSourcePeer {
		t.Fatalf("forged header moved the recorded address: %+v", got)
	}
	if got.UserAgent != "probe/1.0" {
		t.Fatalf("user agent lost: %+v", got)
	}
}

func TestLoginClientHonoursTrustedProxy(t *testing.T) {
	app := appWithTrustedProxies(t, "10.0.0.0/8")
	got := resolveLoginClient(app, forgedLogin("10.0.0.2:4444"))
	if got.IP != "192.0.2.77" || got.Source != httpx.ClientAddressSourceXForwardedFor {
		t.Fatalf("trusted proxy's forwarded client not used: %+v", got)
	}
}

func TestLoginClientWithoutResolverUsesPeer(t *testing.T) {
	got := resolveLoginClient(&router.AppDeps{}, forgedLogin("198.51.100.8:4444"))
	if got.IP != "198.51.100.8" || got.Source != httpx.ClientAddressSourcePeer {
		t.Fatalf("nil resolver did not fall back to the peer: %+v", got)
	}
}
