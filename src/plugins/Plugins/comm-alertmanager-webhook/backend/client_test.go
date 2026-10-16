// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func testClient(t *testing.T, h http.HandlerFunc) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := newClient(true)
	c.now = func() time.Time { return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC) }
	return c, srv.URL
}

func mustCfg(t *testing.T, m map[string]string) amConfig {
	t.Helper()
	cfg, err := parseConfig(m)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestDialerBlocksLinkLocal(t *testing.T) {
	c := newClient(false)
	cfg := map[string]string{"webhook_url": "http://169.254.169.254/latest/meta-data/", "allow_http": "true"}
	err := c.post(context.Background(), mustCfg(t, cfg), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "blocked address") || errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("metadata address not blocked: %v", err)
	}
	if strings.Contains(err.Error(), "meta-data") {
		t.Fatalf("path leaked: %v", err)
	}
}

func TestDialerBlocksLoopbackBehindDNSName(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	c := newClient(false)
	err := c.post(context.Background(), mustCfg(t, map[string]string{"webhook_url": "http://localhost:" + port + "/hook", "allow_http": "true"}), []byte(`{}`))
	if err == nil || !errors.Is(err, errBlockedAddress) || hits.Load() != 0 {
		t.Fatalf("loopback behind a DNS name was not blocked: %v (hits %d)", err, hits.Load())
	}
}

func TestCheckDialAddress(t *testing.T) {
	blocked := []string{"169.254.169.254:80", "[fe80::1]:443", "127.0.0.1:80", "[::1]:80", "0.0.0.0:80", "[::]:80", "224.0.0.1:80", "[ff02::1]:80", "[::ffff:169.254.169.254]:80", "nonsense"}
	for _, a := range blocked {
		if err := checkDialAddress(a, false); !errors.Is(err, errBlockedAddress) {
			t.Errorf("%s allowed", a)
		}
	}
	allowed := []string{"10.0.0.5:443", "192.168.1.10:8080", "172.16.0.1:443", "93.184.216.34:443", "[2606:4700::1111]:443"}
	for _, a := range allowed {
		if err := checkDialAddress(a, false); err != nil {
			t.Errorf("%s blocked: %v", a, err)
		}
	}
	if err := checkDialAddress("127.0.0.1:80", true); err != nil {
		t.Errorf("test loopback opt-in ignored: %v", err)
	}
}

func TestRedirectNotFollowed(t *testing.T) {
	var followed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)
	c, base := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/steal", http.StatusFound)
	})
	cfg := mustCfg(t, map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "auth_mode": "bearer", "bearer_token": "tok"})
	err := c.post(context.Background(), cfg, []byte(`{}`))
	if err == nil || errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("redirect must be a permanent error: %v", err)
	}
	if followed.Load() != 0 {
		t.Fatal("redirect was followed to another host")
	}
}

func TestErrorsHideURLPath(t *testing.T) {
	const token = "tok-SECRET-123"
	for _, code := range []int{http.StatusForbidden, http.StatusBadGateway, http.StatusNotFound} {
		c, base := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
			if _, err := w.Write([]byte(`token ` + token + ` invalid`)); err != nil {
				t.Error(err)
			}
		})
		err := c.post(context.Background(), mustCfg(t, map[string]string{"webhook_url": base + "/integrations/v1/alertmanager/" + token + "/?key=" + token, "allow_http": "true"}), []byte(`{}`))
		if err == nil || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "integrations") {
			t.Fatalf("status %d: %v", code, err)
		}
		if code < 500 && !errors.Is(err, incident.ErrOperatorMessage) {
			t.Fatalf("status %d must be an operator message: %v", code, err)
		}
	}
	c := newClient(true)
	err := c.post(context.Background(), mustCfg(t, map[string]string{"webhook_url": "http://127.0.0.1:1/x/" + token + "/", "allow_http": "true"}), []byte(`{}`))
	if err == nil || !errors.Is(err, incident.ErrRetryable) || strings.Contains(err.Error(), token) {
		t.Fatalf("transport error leaked or misclassified: %v", err)
	}
}

func TestStatusClassification(t *testing.T) {
	cases := []struct {
		code   int
		header string
		check  func(error) bool
	}{
		{200, "", func(err error) bool { return err == nil }},
		{202, "", func(err error) bool { return err == nil }},
		{429, "7", func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && ra.After == 7*time.Second && errors.Is(err, incident.ErrRetryable)
		}},
		{429, "", func(err error) bool {
			var ra *incident.RetryAfterError
			return errors.As(err, &ra) && ra.After == defaultRetry
		}},
		{503, "", func(err error) bool { return errors.Is(err, incident.ErrRetryable) }},
		{400, "", func(err error) bool {
			return errors.Is(err, incident.ErrOperatorMessage) && !errors.Is(err, incident.ErrRetryable)
		}},
		{301, "", func(err error) bool { return err != nil && !errors.Is(err, incident.ErrRetryable) }},
	}
	for _, tc := range cases {
		c, base := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
			if tc.header != "" {
				w.Header().Set("Retry-After", tc.header)
			}
			if tc.code >= 300 && tc.code < 400 {
				w.Header().Set("Location", "https://elsewhere.example/")
			}
			w.WriteHeader(tc.code)
		})
		err := c.post(context.Background(), mustCfg(t, map[string]string{"webhook_url": base + "/hook", "allow_http": "true"}), []byte(`{}`))
		if !tc.check(err) {
			t.Errorf("status %d: unexpected classification %v", tc.code, err)
		}
	}
}

func TestAuthHeadersAndSignature(t *testing.T) {
	var got http.Header
	var body []byte
	c, base := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		body = buf
		w.WriteHeader(http.StatusOK)
	})
	payload := []byte(`{"version":"4"}`)
	cfg := mustCfg(t, map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "auth_mode": "bearer", "bearer_token": "b-tok", "hmac_secret": "s3cret"})
	if err := c.post(context.Background(), cfg, payload); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer b-tok" || got.Get("Content-Type") != "application/json" {
		t.Fatalf("headers %v", got)
	}
	mac := hmac.New(sha256.New, []byte("s3cret"))
	ts := strconv.FormatInt(c.now().Unix(), 10)
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	if got.Get("X-McHarbor-Signature") != want {
		t.Fatalf("signature %q, want %q", got.Get("X-McHarbor-Signature"), want)
	}

	cfg = mustCfg(t, map[string]string{"webhook_url": base + "/hook", "allow_http": "true", "profile": "keep", "basic_password": "k-key"})
	if err := c.post(context.Background(), cfg, payload); err != nil {
		t.Fatal(err)
	}
	req := &http.Request{Header: got}
	if u, p, ok := req.BasicAuth(); !ok || u != "api_key" || p != "k-key" || got.Get("X-McHarbor-Signature") != "" {
		t.Fatalf("keep basic auth: %v %q %q", ok, u, p)
	}
}
