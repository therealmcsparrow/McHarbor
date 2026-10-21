// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package egress

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

var hostileAddresses = map[string]string{
	"loopback v4":               "127.0.0.1:443",
	"loopback v4 high":          "127.255.255.254:443",
	"loopback v6":               "[::1]:443",
	"unspecified v4":            "0.0.0.0:443",
	"unspecified v6":            "[::]:443",
	"aws-gcp-azure metadata":    "169.254.169.254:80",
	"link-local":                "169.254.0.23:443",
	"alibaba metadata":          "100.100.100.200:80",
	"aws metadata v6":           "[fd00:ec2::254]:80",
	"link-local v6 zoned":       "[fe80::1%eth0]:443",
	"multicast":                 "239.1.1.1:443",
	"multicast v6":              "[ff02::1]:443",
	"broadcast":                 "255.255.255.255:443",
	"v4-mapped metadata":        "[::ffff:169.254.169.254]:80",
	"v4-mapped loopback":        "[::ffff:127.0.0.1]:443",
	"nat64 metadata":            "[64:ff9b::a9fe:a9fe]:80",
	"nat64 loopback":            "[64:ff9b::7f00:1]:443",
	"nat64 local /96":           "[64:ff9b:1::a9fe:a9fe]:80",
	"nat64 local /48 position":  "[64:ff9b:1:a9fe:a9:fe00::]:80",
	"6to4 metadata":             "[2002:a9fe:a9fe::1]:80",
	"ipv4-compatible metadata":  "[::a9fe:a9fe]:80",
	"teredo server metadata":    "[2001:0:a9fe:a9fe::1]:80",
	"teredo client metadata":    "[2001:0:4136:e378:8000:63bf:5601:5601]:80",
	"isatap metadata":           "[2001:db8::5efe:a9fe:a9fe]:80",
	"hostname instead of an IP": "metadata.google.internal:80",
}

func TestCheckAddress_RefusesHostileAddresses(t *testing.T) {
	for name, addr := range hostileAddresses {
		t.Run(name, func(t *testing.T) {
			if err := CheckAddress(addr, Policy{}); !errors.Is(err, ErrDestinationRefused) {
				t.Fatalf("CheckAddress(%q) = %v, want ErrDestinationRefused", addr, err)
			}
		})
	}
}

func TestCheckAddress_AllowsPrivateAndPublic(t *testing.T) {
	cases := map[string]string{
		"rfc1918 10":     "10.0.0.5:443",
		"rfc1918 172":    "172.16.1.1:443",
		"rfc1918 192":    "192.168.1.10:443",
		"ula":            "[fd12:3456::1]:443",
		"public":         "8.8.8.8:443",
		"public v6":      "[2606:4700::1111]:443",
		"nat64 public":   "[64:ff9b::808:808]:443",
		"near metadata":  "169.253.169.254:443",
		"cgnat neighbor": "100.100.100.201:443",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := CheckAddress(addr, Policy{}); err != nil {
				t.Fatalf("CheckAddress(%q) = %v, want nil", addr, err)
			}
		})
	}
}

func TestCheckAddress_AllowLoopbackOpensOnlyLoopback(t *testing.T) {
	open := Policy{AllowLoopback: true}
	for _, addr := range []string{"127.0.0.1:80", "[::1]:80", "[64:ff9b::7f00:1]:80"} {
		if err := CheckAddress(addr, open); err != nil {
			t.Errorf("CheckAddress(%q, AllowLoopback) = %v, want nil", addr, err)
		}
	}
	for _, addr := range []string{"169.254.169.254:80", "[64:ff9b::a9fe:a9fe]:80", "[fd00:ec2::254]:80", "0.0.0.0:80"} {
		if err := CheckAddress(addr, open); !errors.Is(err, ErrDestinationRefused) {
			t.Errorf("CheckAddress(%q, AllowLoopback) = %v, want ErrDestinationRefused", addr, err)
		}
	}
}

func fetchCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func get(t *testing.T, client *http.Client, target string) error {
	req, err := http.NewRequestWithContext(fetchCtx(t), http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("closing body: %v", cerr)
		}
	}
	return err
}

func TestNewClient_RefusesLoopbackByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	err := get(t, NewClient(Options{Timeout: 10 * time.Second}), srv.URL)
	if !errors.Is(err, ErrDestinationRefused) {
		t.Fatalf("GET loopback: err = %v, want ErrDestinationRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback server was reached")
	}
}

func TestNewClient_RedirectToMetadataIsRefused(t *testing.T) {
	cases := map[string]string{
		"to metadata":        "http://169.254.169.254/latest/meta-data/",
		"to nat64 metadata":  "http://[64:ff9b::a9fe:a9fe]/latest/meta-data/",
		"to alibaba":         "http://100.100.100.200/latest/meta-data/",
		"to aws metadata v6": "http://[fd00:ec2::254]/latest/meta-data/",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer srv.Close()
			// Loopback is opened only so the first hop reaches the test server.
			client := NewClient(Options{Timeout: 10 * time.Second, Policy: Policy{AllowLoopback: true}})
			if err := get(t, client, srv.URL); !errors.Is(err, ErrDestinationRefused) {
				t.Fatalf("redirect %s: err = %v, want ErrDestinationRefused", target, err)
			}
		})
	}
}

func TestCheckRedirect_RefusesHTTPSDowngrade(t *testing.T) {
	via := []*http.Request{{URL: mustURL(t, "https://catalog.example.org/index.json")}}
	err := CheckRedirect(&http.Request{URL: mustURL(t, "http://catalog.example.org/index.json")}, via)
	if !errors.Is(err, ErrDestinationRefused) {
		t.Fatalf("https to http redirect: err = %v, want ErrDestinationRefused", err)
	}
	via = []*http.Request{{URL: mustURL(t, "http://catalog.lan/index.json")}}
	if err := CheckRedirect(&http.Request{URL: mustURL(t, "http://mirror.lan/index.json")}, via); err != nil {
		t.Fatalf("http to http redirect: err = %v, want nil", err)
	}
}

type stubResolver map[string][]netip.Addr

func (s stubResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return s[host], nil
}

func TestCheckedProxy_ChecksTargetBeforeProxying(t *testing.T) {
	proxyURL := mustURL(t, "http://proxy.corp:3128")
	viaProxy := func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	resolver := stubResolver{
		"metadata.example": {netip.MustParseAddr("169.254.169.254")},
		"catalog.example":  {netip.MustParseAddr("93.184.216.34")},
	}
	pick := checkedProxy(Policy{}, viaProxy, resolver)
	for _, target := range []string{"http://metadata.example/", "http://169.254.169.254/", "http://[64:ff9b::a9fe:a9fe]/"} {
		req := &http.Request{Method: http.MethodGet, URL: mustURL(t, target)}
		if _, err := pick(req); !errors.Is(err, ErrDestinationRefused) {
			t.Errorf("proxy for %s: err = %v, want ErrDestinationRefused", target, err)
		}
	}
	req := &http.Request{Method: http.MethodGet, URL: mustURL(t, "https://catalog.example/index.json")}
	got, err := pick(req)
	if err != nil || got != proxyURL {
		t.Fatalf("proxy for a public catalog = %v, %v; want the proxy", got, err)
	}
	direct := checkedProxy(Policy{}, func(*http.Request) (*url.URL, error) { return nil, nil }, resolver)
	if got, err := direct(req); got != nil || err != nil {
		t.Fatalf("no proxy configured = %v, %v; want nil, nil (the dialer checks)", got, err)
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
