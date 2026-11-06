// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

func completeAgainst(t *testing.T, status int, body string) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	_, err := newVllmClient().Complete(context.Background(),
		map[string]string{"api_key": "test-key", "base_url": server.URL, "default_model": "test-model"},
		plugin.LLMRequest{Model: "test-model", Messages: []plugin.LLMMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("status %d: Complete returned no error", status)
	}
	return err
}

// TestRefusedRequestSaysWhatTheProviderSaid covers every envelope the family's servers use:
// a log that reads only "status 400" once hid a tool name the API would not accept.
func TestRefusedRequestSaysWhatTheProviderSaid(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"the provider's reason"}}`,
		`{"error":"the provider's reason"}`,
		`{"message":"the provider's reason"}`,
		`{"detail":"the provider's reason"}`,
	} {
		if err := completeAgainst(t, http.StatusBadRequest, body); !strings.Contains(err.Error(), "the provider's reason") {
			t.Errorf("body %s: error %q does not carry the provider's message", body, err)
		}
	}
}

// TestRateLimitKeepsTheWordsTheHostMatchesOn: the host answers a rate limit with its own
// code only when the error says "rate limit".
func TestRateLimitKeepsTheWordsTheHostMatchesOn(t *testing.T) {
	err := completeAgainst(t, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`)
	for _, want := range []string{"rate limit", "slow down"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestAuthenticationFailureRepeatsNothing: a provider's 401 can echo part of the key.
func TestAuthenticationFailureRepeatsNothing(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		err := completeAgainst(t, status, `{"error":{"message":"Incorrect API key provided: sk-abc"}}`)
		if strings.Contains(err.Error(), "sk-abc") {
			t.Errorf("status %d: error %q repeats the provider's text", status, err)
		}
		if !strings.Contains(err.Error(), "authentication failed") {
			t.Errorf("status %d: error %q does not name the failure", status, err)
		}
	}
}

// TestOtherBodiesAreNotRepeated: a proxy's HTML page or an oversized message cannot flood
// the log and the response.
func TestOtherBodiesAreNotRepeated(t *testing.T) {
	err := completeAgainst(t, http.StatusBadGateway, `<html>secret proxy page</html>`)
	if strings.Contains(err.Error(), "secret proxy page") {
		t.Errorf("error %q repeats a body that is not an error envelope", err)
	}
	err = completeAgainst(t, http.StatusBadRequest, `{"error":{"message":"`+strings.Repeat("Z", 5000)+`"}}`)
	if got := strings.Count(err.Error(), "Z"); got != 500 {
		t.Errorf("an oversized message kept %d characters, want 500", got)
	}
}
