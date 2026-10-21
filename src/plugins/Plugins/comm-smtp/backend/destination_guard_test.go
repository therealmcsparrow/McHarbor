// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestCheckDestinationRefusesHostileAddresses(t *testing.T) {
	for _, addr := range []string{
		"169.254.169.254:25",
		"[fd00:ec2::254]:25",
		"100.100.100.200:25",
		"0.0.0.0:25",
		"[::]:25",
		"[fe80::1]:25",
		"[fe80::1%eth0]:25",
		"224.0.0.1:25",
		"[ff02::1]:25",
		"255.255.255.255:25",
		"[::ffff:169.254.169.254]:25",
		"[64:ff9b::a9fe:a9fe]:25",
		"[64:ff9b:1:a9fe:a9:fe00::]:25",
		"[2002:a9fe:a9fe::1]:25",
		"[2001:0:a9fe:a9fe::1]:25",
		"[2001:0:4136:e378:8000:63bf:5601:5601]:25",
		"[2001:db8::5efe:a9fe:a9fe]:25",
		"[::a9fe:a9fe]:25",
		"[::ffff:100.100.100.200]:25",
		"not-an-address",
		"relay.example.com:25",
	} {
		if err := checkDestination(addr); !errors.Is(err, errDestinationRefused) {
			t.Errorf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
		}
	}
}

func TestCheckDestinationAllowsLANLoopbackAndPublic(t *testing.T) {
	for _, addr := range []string{
		"127.0.0.1:25",
		"[::1]:25",
		"10.0.0.5:25",
		"172.16.4.2:25",
		"192.168.1.10:25",
		"[fd12:3456::1]:25",
		"[64:ff9b::a00:5]:25",
		"8.8.8.8:25",
		"[2606:4700::1111]:25",
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

func shortCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSendSMTPRefusesMetadataBeforeConnecting(t *testing.T) {
	for _, tc := range []struct{ host, encryption string }{
		{"169.254.169.254", "none"},
		{"169.254.169.254", "starttls"},
		{"169.254.169.254", "ssl_tls"},
		{"fd00:ec2::254", "none"},
		{"64:ff9b::a9fe:a9fe", "ssl_tls"},
	} {
		cfg := smtpConfig{Host: tc.host, Port: 25, Encryption: tc.encryption, AuthMethod: "none"}
		err := sendSMTP(shortCtx(t), cfg, []byte("x"))
		if !errors.Is(err, errDestinationRefused) {
			t.Errorf("sendSMTP(%s, %s) = %v, want errDestinationRefused", tc.host, tc.encryption, err)
		}
	}
}

func TestSendSMTPStillReachesLoopbackRelay(t *testing.T) {
	addr, hit := acceptOnce(t)
	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("tcp", port)
	err := sendSMTP(shortCtx(t), smtpConfig{Host: host, Port: p, Encryption: "none", AuthMethod: "none"}, []byte("x"))
	if errors.Is(err, errDestinationRefused) {
		t.Fatalf("loopback relay refused: %v", err)
	}
	waitHit(t, hit)
}
