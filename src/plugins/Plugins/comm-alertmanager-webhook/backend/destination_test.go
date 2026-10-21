// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckDialAddress_RefusesEmbeddedAndMetadataForms(t *testing.T) {
	cases := map[string]string{
		"alibaba metadata":         "100.100.100.200:80",
		"aws metadata v6":          "[fd00:ec2::254]:80",
		"broadcast":                "255.255.255.255:80",
		"this network":             "0.1.2.3:80",
		"loopback high":            "127.255.255.254:80",
		"link-local v6 zoned":      "[fe80::1%eth0]:443",
		"nat64 metadata":           "[64:ff9b::a9fe:a9fe]:80",
		"nat64 loopback":           "[64:ff9b::7f00:1]:80",
		"nat64 local /96":          "[64:ff9b:1::a9fe:a9fe]:80",
		"nat64 local /48 position": "[64:ff9b:1:a9fe:a9:fe00::]:80",
		"6to4 metadata":            "[2002:a9fe:a9fe::1]:80",
		"ipv4-compatible metadata": "[::a9fe:a9fe]:80",
		"teredo server metadata":   "[2001:0:a9fe:a9fe::1]:80",
		"teredo client metadata":   "[2001:0:4136:e378:8000:63bf:5601:5601]:80",
		"isatap metadata":          "[2001:db8::5efe:a9fe:a9fe]:80",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDialAddress(addr, false); !errors.Is(err, errBlockedAddress) {
				t.Fatalf("checkDialAddress(%q) = %v, want errBlockedAddress", addr, err)
			}
		})
	}
	for _, addr := range []string{"[fd12:3456::1]:443", "[64:ff9b::808:808]:443", "100.100.100.201:443"} {
		if err := checkDialAddress(addr, false); err != nil {
			t.Errorf("checkDialAddress(%q) = %v, want nil", addr, err)
		}
	}
}

func TestPost_RedirectToMetadataIsNotFollowed(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "http://[64:ff9b::a9fe:a9fe]/latest/meta-data/", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := newClient(true).post(ctx, amConfig{URL: srv.URL + "/api/v2/alerts", Origin: "test"}, []byte("[]"))
	if err == nil {
		t.Fatal("a redirect to the metadata service was treated as delivered")
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("receiver hit %d times, want 1", n)
	}
}
