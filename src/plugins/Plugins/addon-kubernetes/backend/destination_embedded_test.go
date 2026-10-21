// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDestinationPolicy_RefusesEmbeddedMetadata(t *testing.T) {
	cases := map[string]string{
		"nat64 well-known":            "[64:ff9b::a9fe:a9fe]:80",
		"nat64 well-known dotted":     "[64:ff9b::169.254.169.254]:80",
		"nat64 alibaba":               "[64:ff9b::6464:64c8]:80",
		"nat64 local /96":             "[64:ff9b:1::a9fe:a9fe]:80",
		"nat64 local /48 position":    "[64:ff9b:1:a9fe:a9:fe00::]:80",
		"nat64 local /64 position":    "[64:ff9b:1::a9:fea9:fe00:0]:80",
		"6to4":                        "[2002:a9fe:a9fe::1]:80",
		"6to4 alibaba":                "[2002:6464:64c8::]:80",
		"ipv4-compatible":             "[::a9fe:a9fe]:80",
		"ipv4-compatible dotted":      "[::169.254.169.254]:80",
		"ipv4-mapped hex":             "[::ffff:a9fe:a9fe]:80",
		"teredo server":               "[2001:0:a9fe:a9fe::1]:80",
		"teredo client (obfuscated)":  "[2001:0:4136:e378:8000:63bf:5601:5601]:80",
		"isatap":                      "[2001:db8::5efe:a9fe:a9fe]:80",
		"isatap universal bit":        "[2001:db8::200:5efe:a9fe:a9fe]:80",
		"nat64 link-local low corner": "[64:ff9b::a9fe:1]:80",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDestination(addr); !errors.Is(err, errDestinationRefused) {
				t.Fatalf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
			}
		})
	}
}

func TestDestinationPolicy_AllowsEmbeddedOrdinaryAddresses(t *testing.T) {
	cases := map[string]string{
		"nat64 public":      "[64:ff9b::808:808]:443",
		"nat64 rfc1918":     "[64:ff9b::a00:5]:6443",
		"6to4 public":       "[2002:808:808::1]:443",
		"teredo public":     "[2001:0:808:808:8000:63bf:f7f7:f7f7]:443",
		"global unicast":    "[2606:4700::1111]:443",
		"ipv4-compatible 1": "[::1]:6443",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDestination(addr); err != nil {
				t.Fatalf("checkDestination(%q) = %v, want nil", addr, err)
			}
		})
	}
}

// proxyChildEnv marks the child process TestDestination_EnvironmentProxyIsIgnored spawns.
// net/http reads HTTP(S)_PROXY once per process, so the proxied case needs a fresh one.
const proxyChildEnv = "MCHARBOR_K8S_PROXY_CHILD"

func TestDestination_EnvironmentProxyIsIgnored(t *testing.T) {
	if os.Getenv(proxyChildEnv) == "1" {
		for name, conn := range map[string]*Connection{
			"kubeconfig": loopbackConnections(&httptest.Server{URL: "http://169.254.169.254"})["kubeconfig"],
			"server_url": {ServerURL: "http://169.254.169.254", BearerToken: "t"},
		} {
			if err := listNamespaces(conn); !errors.Is(err, errDestinationRefused) {
				t.Fatalf("%s through an environment proxy: err = %v, want errDestinationRefused", name, err)
			}
		}
		return
	}

	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`)); err != nil {
			t.Errorf("writing the proxied reply: %v", err)
		}
	}))
	defer proxy.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestDestination_EnvironmentProxyIsIgnored$", "-test.v")
	cmd.Env = append(os.Environ(), proxyChildEnv+"=1",
		"HTTP_PROXY="+proxy.URL, "HTTPS_PROXY="+proxy.URL, "http_proxy="+proxy.URL, "https_proxy="+proxy.URL,
		"NO_PROXY=", "no_proxy=")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PASS") {
		t.Fatalf("child run failed: %v\n%s", err, out)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the environment proxy received %d cluster request(s); the dial guard was bypassed", n)
	}
}
