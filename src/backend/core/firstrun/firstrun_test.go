// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package firstrun

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func request(token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", nil)
	if token != "" {
		req.Header.Set(Header, token)
	}
	return req
}

func TestGateAcceptsOnlyItsOwnToken(t *testing.T) {
	g := New(func() bool { return true }, nil)
	if g.token == "" || len(g.token) != 32 {
		t.Fatalf("a fresh install must mint a 32-hex-char token, got %q", g.token)
	}
	if !g.Valid(request(g.token)) {
		t.Fatal("the minted token must be accepted")
	}
	for _, bad := range []string{"", "0000", g.token + "0"} {
		if g.Valid(request(bad)) {
			t.Fatalf("token %q must be refused", bad)
		}
	}
}

func TestGateIsClosedOnceSetUpOrUnwired(t *testing.T) {
	if g := New(func() bool { return false }, nil); g.token != "" || g.Valid(request("")) {
		t.Fatal("an instance that is already set up mints no token and accepts nothing")
	}
	var nilGate *Gate
	if nilGate.Valid(request("anything")) {
		t.Fatal("a nil gate must refuse")
	}
}
