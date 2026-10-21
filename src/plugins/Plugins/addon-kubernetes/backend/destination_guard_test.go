// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDestinationPolicy_RefusesHostileRanges(t *testing.T) {
	cases := map[string]string{
		"aws-gcp-azure metadata v4": "169.254.169.254:80",
		"link-local low":            "169.254.0.1:443",
		"link-local high":           "169.254.255.254:443",
		"aws metadata v6":           "[fd00:ec2::254]:80",
		"aws v6 prefix":             "[fd00:ec2::1]:80",
		"link-local v6":             "[fe80::1]:6443",
		"link-local v6 top of /10":  "[febf::1]:6443",
		"link-local v6 with zone":   "[fe80::1%eth0]:6443",
		"alibaba metadata":          "100.100.100.200:80",
		"v4-mapped metadata":        "[::ffff:169.254.169.254]:80",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDestination(addr); !errors.Is(err, errDestinationRefused) {
				t.Fatalf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
			}
		})
	}
}

func TestDestinationPolicy_AllowsLoopbackAndPrivate(t *testing.T) {
	cases := map[string]string{
		"loopback v4":   "127.0.0.1:6443",
		"loopback v6":   "[::1]:6443",
		"rfc1918 10":    "10.0.0.5:6443",
		"rfc1918 172":   "172.16.1.1:6443",
		"rfc1918 192":   "192.168.1.10:6443",
		"ula":           "[fd12:3456::1]:6443",
		"public":        "8.8.8.8:443",
		"near metadata": "169.253.169.254:80",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDestination(addr); err != nil {
				t.Fatalf("checkDestination(%q) = %v, want nil", addr, err)
			}
		})
	}
}

func TestGuardedDial_RefusesBeforeConnecting(t *testing.T) {
	for _, addr := range []string{"169.254.169.254:80", "[fe80::1]:80", "[fd00:ec2::254]:80", "100.100.100.200:80"} {
		t.Run(addr, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := guardedDial(ctx, "tcp", addr)
			if conn != nil {
				t.Cleanup(func() { _ = conn.Close() })
				t.Fatalf("guardedDial(%q) returned a connection", addr)
			}
			if !errors.Is(err, errDestinationRefused) {
				t.Fatalf("guardedDial(%q) err = %v, want errDestinationRefused", addr, err)
			}
		})
	}
}

func TestGuardedDial_AllowsLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := guardedDial(ctx, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("loopback dial refused: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("closing the loopback connection: %v", err)
	}
}

func TestRestConfig_BothBranchesCarryTheDialGuard(t *testing.T) {
	fx := newGuardFixtures(t)
	conns := map[string]*Connection{
		"kubeconfig": {Kubeconfig: guardKubeconfig(t, fx, "current", nil)},
		"server_url": {ServerURL: "https://10.0.0.1:6443", BearerToken: "tok"},
	}
	for name, conn := range conns {
		t.Run(name, func(t *testing.T) {
			cfg, err := conn.restConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Dial == nil {
				t.Fatal("restConfig left Dial unset; the destination policy is bypassed")
			}
			if _, err := cfg.Dial(context.Background(), "tcp", "169.254.169.254:80"); !errors.Is(err, errDestinationRefused) {
				t.Fatalf("cfg.Dial to metadata = %v, want errDestinationRefused", err)
			}
		})
	}
}

// loopbackConnections builds both connection shapes against one loopback test server.
func loopbackConnections(srv *httptest.Server) map[string]*Connection {
	kc := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: c1
  cluster:
    server: %s
users:
- name: u1
  user:
    token: loopback-token
contexts:
- name: ctx
  context:
    cluster: c1
    user: u1
current-context: ctx
`, srv.URL)
	return map[string]*Connection{
		"kubeconfig": {Kubeconfig: kc},
		"server_url": {ServerURL: srv.URL, BearerToken: "loopback-token"},
	}
}

func listNamespaces(conn *Connection) error {
	cs, err := conn.createClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	return err
}

func TestDestination_LoopbackClusterStillWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`)); err != nil {
			t.Errorf("writing the namespace list: %v", err)
		}
	}))
	defer srv.Close()
	for name, conn := range loopbackConnections(srv) {
		t.Run(name, func(t *testing.T) {
			if err := listNamespaces(conn); err != nil {
				t.Fatalf("loopback cluster refused: %v", err)
			}
		})
	}
}

func TestDestination_RedirectToMetadataIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()
	for name, conn := range loopbackConnections(srv) {
		t.Run(name, func(t *testing.T) {
			err := listNamespaces(conn)
			if !errors.Is(err, errDestinationRefused) {
				t.Fatalf("redirect to metadata: err = %v, want errDestinationRefused", err)
			}
		})
	}
}

func TestDestination_UnconfirmedUpstreamBodyIsNotEchoed(t *testing.T) {
	const marker = "INTERNAL-SERVICE-SECRET-BODY"
	cases := map[string]struct {
		status int
		ctype  string
		body   string
		check  func(error) bool
	}{
		"500 plain text": {http.StatusInternalServerError, "text/plain", marker, apierrors.IsInternalError},
		"404 html":       {http.StatusNotFound, "text/html", "<html>" + marker + "</html>", apierrors.IsNotFound},
		"403 status":     {http.StatusForbidden, "application/json", `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"` + marker + `","reason":"Forbidden","code":403}`, apierrors.IsForbidden},
		"401 json":       {http.StatusUnauthorized, "application/json", `{"leak":"` + marker + `"}`, apierrors.IsUnauthorized},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.ctype)
				w.WriteHeader(tc.status)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Errorf("writing the upstream body: %v", err)
				}
			}))
			defer srv.Close()
			for shape, conn := range loopbackConnections(srv) {
				err := listNamespaces(conn)
				if err == nil {
					t.Fatalf("%s: expected an error", shape)
				}
				if strings.Contains(err.Error(), marker) {
					t.Errorf("%s: error echoes the upstream body: %v", shape, err)
				}
				if !tc.check(err) {
					t.Errorf("%s: status class lost, got %v", shape, err)
				}
			}
		})
	}
}
