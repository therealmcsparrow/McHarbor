// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// loopbackSeen records the credential headers the stand-in host received.
func loopbackSeen(t *testing.T, build func(base string) *hostAPI) (cookie, auth string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, auth = r.Header.Get("Cookie"), r.Header.Get("Authorization")
		if _, err := fmt.Fprint(w, `{"success":true,"data":[]}`); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	var out []envRow
	if !build(srv.URL).get(context.Background(), "k", "K", "/api/environments", nil, &out) {
		t.Fatal("loopback call failed")
	}
	return cookie, auth
}

func TestHostAPIForwardsBearerWithoutCookie(t *testing.T) {
	cookie, auth := loopbackSeen(t, func(base string) *hostAPI {
		api := newHostAPI("", "en", "").withAuthorization("Bearer mch_key")
		api.baseURLs = []string{base}
		return api
	})
	if auth != "Bearer mch_key" || cookie != "" {
		t.Fatalf("cookie=%q authorization=%q", cookie, auth)
	}
}

func TestHostAPIForwardsCookieAndBearerTogether(t *testing.T) {
	cookie, auth := loopbackSeen(t, func(base string) *hostAPI {
		api := newHostAPI("sid=abc", "en", "").withAuthorization("Bearer mch_key")
		api.baseURLs = []string{base}
		return api
	})
	if auth != "Bearer mch_key" || cookie != "sid=abc" {
		t.Fatalf("cookie=%q authorization=%q", cookie, auth)
	}
}

func TestUnscopedSubClientKeepsAuthorization(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if _, err := fmt.Fprint(w, `{"success":true,"data":[]}`); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	api := newHostAPI("", "en", "env1").withAuthorization("Bearer mch_key")
	api.baseURLs = []string{srv.URL}
	var out []envRow
	if !resUnscopedGet(context.Background(), api, "k", "K", "/api/environments", nil, &out) {
		t.Fatal("unscoped call failed")
	}
	if auth != "Bearer mch_key" {
		t.Fatalf("authorization = %q", auth)
	}
}
