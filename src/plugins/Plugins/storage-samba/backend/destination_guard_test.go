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
		"169.254.169.254:445",
		"[fd00:ec2::254]:445",
		"100.100.100.200:445",
		"0.0.0.0:445",
		"[::]:445",
		"[fe80::1]:445",
		"[fe80::1%eth0]:445",
		"224.0.0.1:445",
		"[ff02::1]:445",
		"255.255.255.255:445",
		"[::ffff:169.254.169.254]:445",
		"[64:ff9b::a9fe:a9fe]:445",
		"[64:ff9b:1:a9fe:a9:fe00::]:445",
		"[2002:a9fe:a9fe::1]:445",
		"[2001:0:a9fe:a9fe::1]:445",
		"[2001:0:4136:e378:8000:63bf:5601:5601]:445",
		"[2001:db8::5efe:a9fe:a9fe]:445",
		"[::a9fe:a9fe]:445",
		"[::ffff:100.100.100.200]:445",
		"not-an-address",
		"relay.example.com:445",
	} {
		if err := checkDestination(addr); !errors.Is(err, errDestinationRefused) {
			t.Errorf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
		}
	}
}

func TestCheckDestinationAllowsLANLoopbackAndPublic(t *testing.T) {
	for _, addr := range []string{
		"127.0.0.1:445",
		"[::1]:445",
		"10.0.0.5:445",
		"172.16.4.2:445",
		"192.168.1.10:445",
		"[fd12:3456::1]:445",
		"[64:ff9b::a00:5]:445",
		"8.8.8.8:445",
		"[2606:4700::1111]:445",
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

func TestSambaDialRefusesMetadataBeforeConnecting(t *testing.T) {
	c := NewSambaClient()
	for _, server := range []string{"169.254.169.254", "169.254.169.254:445", "[fd00:ec2::254]:445", "100.100.100.200"} {
		cfg := map[string]string{"server": server, "share": "backups", "username": "u", "password": "p"}
		session, share, conn, err := c.sambaDial(shortCtx(t), cfg)
		sambaClose(session, share, conn)
		if !errors.Is(err, errDestinationRefused) {
			t.Errorf("sambaDial(%s) = %v, want errDestinationRefused", server, err)
		}
	}
}

func TestSambaDialStillReachesLoopbackServer(t *testing.T) {
	addr, hit := acceptOnce(t)
	cfg := map[string]string{"server": addr, "share": "backups", "username": "u", "password": "p"}
	session, share, conn, err := NewSambaClient().sambaDial(shortCtx(t), cfg)
	sambaClose(session, share, conn)
	if errors.Is(err, errDestinationRefused) {
		t.Fatalf("loopback server refused: %v", err)
	}
	waitHit(t, hit)
}
