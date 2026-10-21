// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestCheckDestinationRefusesHostileAddresses(t *testing.T) {
	for _, addr := range []string{
		"169.254.169.254:389",
		"[fd00:ec2::254]:389",
		"100.100.100.200:389",
		"0.0.0.0:389",
		"[::]:389",
		"[fe80::1]:389",
		"[fe80::1%eth0]:389",
		"224.0.0.1:389",
		"[ff02::1]:389",
		"255.255.255.255:389",
		"[::ffff:169.254.169.254]:389",
		"[64:ff9b::a9fe:a9fe]:389",
		"[64:ff9b:1:a9fe:a9:fe00::]:389",
		"[2002:a9fe:a9fe::1]:389",
		"[2001:0:a9fe:a9fe::1]:389",
		"[2001:0:4136:e378:8000:63bf:5601:5601]:389",
		"[2001:db8::5efe:a9fe:a9fe]:389",
		"[::a9fe:a9fe]:389",
		"[::ffff:100.100.100.200]:389",
		"not-an-address",
		"relay.example.com:389",
	} {
		if err := checkDestination(addr); !errors.Is(err, errDestinationRefused) {
			t.Errorf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
		}
	}
}

func TestCheckDestinationAllowsLANLoopbackAndPublic(t *testing.T) {
	for _, addr := range []string{
		"127.0.0.1:389",
		"[::1]:389",
		"10.0.0.5:389",
		"172.16.4.2:389",
		"192.168.1.10:389",
		"[fd12:3456::1]:389",
		"[64:ff9b::a00:5]:389",
		"8.8.8.8:389",
		"[2606:4700::1111]:389",
	} {
		if err := checkDestination(addr); err != nil {
			t.Errorf("checkDestination(%q) = %v, want nil", addr, err)
		}
	}
}

func TestGuardedDialerRefusesUnixSockets(t *testing.T) {
	d := guardedDialer(time.Second)
	if err := d.Control("unix", "/var/run/docker.sock", nil); !errors.Is(err, errDestinationRefused) {
		t.Fatalf("unix control = %v, want errDestinationRefused", err)
	}
}

// acceptOnce listens on loopback and reports whether anything connected, proving loopback is
// deliberately reachable for this plugin.
func acceptOnce(t *testing.T) (string, <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Logf("close listener: %v", err)
		}
	})
	hit := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Closing before signalling keeps any close error inside the test lifetime.
		if err := conn.Close(); err != nil {
			t.Logf("close accepted conn: %v", err)
		}
		hit <- struct{}{}
	}()
	return ln.Addr().String(), hit
}

func waitHit(t *testing.T, hit <-chan struct{}) {
	t.Helper()
	select {
	case <-hit:
	case <-time.After(5 * time.Second):
		t.Fatal("loopback listener was never dialled")
	}
}

func TestLDAPDialRefusesMetadataBeforeConnecting(t *testing.T) {
	for _, tc := range []ldapProviderConfig{
		{URL: "ldap://169.254.169.254:389"},
		{URL: "ldap://169.254.169.254:389", StartTLS: true},
		{URL: "ldaps://169.254.169.254:636"},
		{URL: "ldaps://[fd00:ec2::254]:636"},
		{URL: "ldap://[64:ff9b::a9fe:a9fe]:389"},
		{URL: "cldap://169.254.169.254"},
		{URL: "ldapi://%2Ftmp%2Fmcharbor-guard-test.sock"},
	} {
		conn, err := tc.dial()
		if conn != nil {
			if cerr := conn.Close(); cerr != nil {
				t.Logf("close: %v", cerr)
			}
		}
		if !errors.Is(err, errDestinationRefused) {
			t.Errorf("dial(%s) = %v, want errDestinationRefused", tc.URL, err)
		}
	}
}

func TestLDAPDialStillReachesLoopbackDirectory(t *testing.T) {
	addr, hit := acceptOnce(t)
	conn, err := ldapProviderConfig{URL: "ldap://" + addr}.dial()
	if err != nil {
		t.Fatalf("loopback directory refused: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	})
	waitHit(t, hit)
}
