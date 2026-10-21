// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogDestination_RefusesHostileAddresses(t *testing.T) {
	cases := map[string]string{
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
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDestination(addr, false); !errors.Is(err, errDestinationRefused) {
				t.Fatalf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
			}
		})
	}
}

func TestCatalogDestination_AllowsPrivateAndPublic(t *testing.T) {
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
			if err := checkDestination(addr, false); err != nil {
				t.Fatalf("checkDestination(%q) = %v, want nil", addr, err)
			}
		})
	}
}

const testCatalogIndex = `[{"id":"img-test","kind":"cloud_image","distro":"debian","release":"13","arch":"x86_64",` +
	`"displayName":"Debian 13","sourceUrl":"https://cloud.example.org/debian.qcow2","checksum":"` +
	`0000000000000000000000000000000000000000000000000000000000000000","checksumAlgo":"sha256"}]`

// loopbackCatalogClient is the guarded client with loopback opened and the test server's
// certificate trusted, so a test can drive a real TLS fetch and redirect.
func loopbackCatalogClient(srv *httptest.Server) *http.Client {
	client := newCatalogClient(true)
	client.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	return client
}

func fetchCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestCatalogFetch_LoopbackTestClientLoadsIndex(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(testCatalogIndex)); err != nil {
			t.Errorf("writing the index: %v", err)
		}
	}))
	defer srv.Close()
	entries, _, err := fetchCatalogIndexWith(fetchCtx(t), loopbackCatalogClient(srv), srv.URL)
	if err != nil || len(entries) != 1 {
		t.Fatalf("fetch = %d entries, %v; want 1 entry", len(entries), err)
	}
}

func TestCatalogFetch_ProductionClientRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	_, _, err := fetchCatalogIndex(fetchCtx(t), srv.URL)
	if !errors.Is(err, errDestinationRefused) {
		t.Fatalf("fetch of a loopback catalog: err = %v, want errDestinationRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback server was reached")
	}
}

func TestCatalogFetch_RedirectIsRechecked(t *testing.T) {
	cases := map[string]string{
		"to metadata":       "https://169.254.169.254/latest/meta-data/",
		"to nat64 metadata": "https://[64:ff9b::a9fe:a9fe]/latest/meta-data/",
		"to alibaba":        "https://100.100.100.200/latest/meta-data/",
		"to plain http":     "http://10.0.0.5/index.json",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer srv.Close()
			_, _, err := fetchCatalogIndexWith(fetchCtx(t), loopbackCatalogClient(srv), srv.URL)
			if !errors.Is(err, errDestinationRefused) {
				t.Fatalf("redirect %s: err = %v, want errDestinationRefused", target, err)
			}
		})
	}
}

// catalogProxyChildEnv marks the child process TestCatalogFetch_EnvironmentProxyIsIgnored
// spawns. net/http reads HTTP(S)_PROXY once per process, so the proxied case needs a fresh one.
const catalogProxyChildEnv = "MCHARBOR_COMPUTE_PROXY_CHILD"

func TestCatalogFetch_EnvironmentProxyIsIgnored(t *testing.T) {
	if os.Getenv(catalogProxyChildEnv) == "1" {
		// Loopback is opened so the proxy itself would be reachable; only a direct dial to
		// the metadata address is refused.
		_, _, err := fetchCatalogIndexWith(fetchCtx(t), newCatalogClient(true), "https://169.254.169.254/index.json")
		if !errors.Is(err, errDestinationRefused) {
			t.Fatalf("fetch through an environment proxy: err = %v, want errDestinationRefused", err)
		}
		return
	}

	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCatalogFetch_EnvironmentProxyIsIgnored$", "-test.v")
	cmd.Env = append(os.Environ(), catalogProxyChildEnv+"=1",
		"HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL, "http_proxy="+proxy.URL, "https_proxy="+proxy.URL,
		"NO_PROXY=", "no_proxy=")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PASS") {
		t.Fatalf("child run failed: %v\n%s", err, out)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the environment proxy received %d catalog request(s); the dial guard was bypassed", n)
	}
}
