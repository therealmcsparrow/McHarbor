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
	_, err := newAnthropicClient().Complete(context.Background(),
		map[string]string{"api_key": "test-key", "base_url": server.URL},
		plugin.LLMRequest{Messages: []plugin.LLMMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("status %d: Complete returned no error", status)
	}
	return err
}

// TestRefusedRequestSaysWhatAnthropicSaid guards the diagnosis of a refused request: the
// log once read only "status 400", which hid a tool name the API would not accept.
func TestRefusedRequestSaysWhatAnthropicSaid(t *testing.T) {
	err := completeAgainst(t, http.StatusBadRequest,
		`{"type":"error","error":{"type":"invalid_request_error","message":"tools.0.custom.name: String should match pattern"}}`)
	for _, want := range []string{"status 400", "tools.0.custom.name: String should match pattern"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestRateLimitKeepsTheWordsTheHostMatchesOn: the host answers a rate limit with its own
// code only when the error says "rate limit".
func TestRateLimitKeepsTheWordsTheHostMatchesOn(t *testing.T) {
	err := completeAgainst(t, http.StatusTooManyRequests,
		`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	for _, want := range []string{"rate limit", "slow down"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestAuthenticationFailureRepeatsNothing keeps Anthropic in line with the provider family.
func TestAuthenticationFailureRepeatsNothing(t *testing.T) {
	err := completeAgainst(t, http.StatusUnauthorized, `{"error":{"message":"invalid x-api-key sk-abc"}}`)
	if got, want := err.Error(), "anthropic authentication failed"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

// TestOtherBodiesAreNotRepeated: only the envelope's message leaves the plugin, so a
// proxy's HTML page or an oversized message cannot flood the log and the response.
func TestOtherBodiesAreNotRepeated(t *testing.T) {
	err := completeAgainst(t, http.StatusBadGateway, `<html>secret proxy page</html>`)
	if got, want := err.Error(), "anthropic request failed with status 502"; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	long := strings.Repeat("x", 5000)
	err = completeAgainst(t, http.StatusBadRequest, `{"error":{"message":"`+long+`"}}`)
	if strings.Count(err.Error(), "x") > maxAnthropicErrorRunes {
		t.Errorf("an oversized message was not capped: %d characters", len(err.Error()))
	}
}
