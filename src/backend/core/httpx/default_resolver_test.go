// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package httpx

import (
	"net/http/httptest"
	"testing"
)

// TestClientIPIgnoresUntrustedForwarding guards login throttling and audit
// attribution: a direct client cannot choose its address with a header, while a
// configured proxy still can.
func TestClientIPIgnoresUntrustedForwarding(t *testing.T) {
	t.Cleanup(func() { SetDefaultResolver(nil) })
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.9:4444"
	req.Header.Set("X-Forwarded-For", "10.9.9.9")
	req.Header.Set("X-Real-IP", "10.8.8.8")

	SetDefaultResolver(nil)
	if got := ClientIP(req); got != "203.0.113.9" {
		t.Fatalf("no resolver: %s", got)
	}
	trusted, err := NewClientAddressResolver([]string{"203.0.113.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	SetDefaultResolver(trusted)
	if got := ClientIP(req); got != "10.9.9.9" {
		t.Fatalf("trusted proxy: %s", got)
	}
	other, err := NewClientAddressResolver([]string{"192.0.2.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	SetDefaultResolver(other)
	if got := ClientIP(req); got != "203.0.113.9" {
		t.Fatalf("untrusted peer must not move its address: %s", got)
	}
}
