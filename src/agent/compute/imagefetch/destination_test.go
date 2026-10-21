// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMirrorDestination_RefusesHostileAddresses(t *testing.T) {
	cases := map[string]string{
		"loopback v4":               "127.0.0.1:443",
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
			if err := checkDestination(addr, false); !errors.Is(err, ErrDestinationRefused) {
				t.Fatalf("checkDestination(%q) = %v, want ErrDestinationRefused", addr, err)
			}
		})
	}
}

func TestMirrorDestination_AllowsLANAndPublicMirrors(t *testing.T) {
	cases := map[string]string{
		"rfc1918 10":   "10.0.0.5:443",
		"rfc1918 172":  "172.16.1.1:443",
		"rfc1918 192":  "192.168.1.10:443",
		"ula":          "[fd12:3456::1]:443",
		"public":       "151.101.2.132:443",
		"public v6":    "[2a04:4e42::644]:443",
		"nat64 public": "[64:ff9b::808:808]:443",
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkDestination(addr, false); err != nil {
				t.Fatalf("checkDestination(%q) = %v, want nil", addr, err)
			}
		})
	}
}

func mirrorCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestFetch_ProductionClientRefusesLoopbackMirror(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	f, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Fetch(mirrorCtx(t), Request{
		ImageID:     "img-loop",
		Kind:        KindCloudImage,
		SourceURL:   srv.URL + "/img.qcow2",
		ChecksumURL: srv.URL + "/SHA256SUMS",
	}, nil)
	if !errors.Is(err, ErrDestinationRefused) {
		t.Fatalf("fetch from a loopback mirror: err = %v, want ErrDestinationRefused", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback mirror was reached")
	}
}

func TestFetch_RedirectIsRechecked(t *testing.T) {
	cases := map[string]string{
		"image to metadata":       "https://169.254.169.254/latest/meta-data/",
		"image to nat64 metadata": "https://[64:ff9b::a9fe:a9fe]/latest/meta-data/",
		"image to plain http":     "http://10.0.0.5/img.qcow2",
		"metadata http downgrade": "http://169.254.169.254/latest/meta-data/",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer srv.Close()
			f, err := New(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			f.client = newMirrorClient(true)
			f.client.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			_, err = f.Fetch(mirrorCtx(t), Request{
				ImageID:     "img-redirect",
				Kind:        KindCloudImage,
				SourceURL:   srv.URL + "/img.qcow2",
				ChecksumURL: srv.URL + "/SHA256SUMS",
			}, nil)
			if !errors.Is(err, ErrDestinationRefused) {
				t.Fatalf("redirect to %s: err = %v, want ErrDestinationRefused", target, err)
			}
		})
	}
}
