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
		"169.254.169.254:514",
		"[fd00:ec2::254]:514",
		"100.100.100.200:514",
		"0.0.0.0:514",
		"[::]:514",
		"[fe80::1]:514",
		"[fe80::1%eth0]:514",
		"224.0.0.1:514",
		"[ff02::1]:514",
		"255.255.255.255:514",
		"[::ffff:169.254.169.254]:514",
		"[64:ff9b::a9fe:a9fe]:514",
		"[64:ff9b:1:a9fe:a9:fe00::]:514",
		"[2002:a9fe:a9fe::1]:514",
		"[2001:0:a9fe:a9fe::1]:514",
		"[2001:0:4136:e378:8000:63bf:5601:5601]:514",
		"[2001:db8::5efe:a9fe:a9fe]:514",
		"[::a9fe:a9fe]:514",
		"[::ffff:100.100.100.200]:514",
		"not-an-address",
		"relay.example.com:514",
	} {
		if err := checkDestination(addr); !errors.Is(err, errDestinationRefused) {
			t.Errorf("checkDestination(%q) = %v, want errDestinationRefused", addr, err)
		}
	}
}

func TestCheckDestinationAllowsLANLoopbackAndPublic(t *testing.T) {
	for _, addr := range []string{
		"127.0.0.1:514",
		"[::1]:514",
		"10.0.0.5:514",
		"172.16.4.2:514",
		"192.168.1.10:514",
		"[fd12:3456::1]:514",
		"[64:ff9b::a00:5]:514",
		"8.8.8.8:514",
		"[2606:4700::1111]:514",
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

func TestDeliverRefusesMetadataBeforeConnecting(t *testing.T) {
	c := newSyslogClient()
	for _, protocol := range []string{"udp", "tcp", "tls"} {
		for _, host := range []string{"169.254.169.254", "fd00:ec2::254", "64:ff9b::a9fe:a9fe"} {
			cfg := resolved{host: host, port: 514, protocol: protocol, timeoutSeconds: 2}
			err := c.deliver(shortCtx(t), cfg, "x")
			if !errors.Is(err, errDestinationRefused) {
				t.Errorf("deliver(%s, %s) = %v, want errDestinationRefused", protocol, host, err)
				continue
			}
			if got := safeError(err); got != errDestinationRefused.Error() {
				t.Errorf("delivery history shows %q, want %q", got, errDestinationRefused.Error())
			}
		}
	}
}

func TestDeliverStillReachesLoopbackCollector(t *testing.T) {
	addr, hit := acceptOnce(t)
	host, port, _ := net.SplitHostPort(addr)
	p, _ := net.LookupPort("tcp", port)
	err := newSyslogClient().deliver(shortCtx(t), resolved{host: host, port: p, protocol: "tcp", timeoutSeconds: 2}, "x")
	if errors.Is(err, errDestinationRefused) {
		t.Fatalf("loopback collector refused: %v", err)
	}
	waitHit(t, hit)
}
