// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestV2PasswordGrant(t *testing.T) {
	f := newFakeGLPI(t)
	cfg, err := parseConfig(f.config("v2"))
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(f.srv.Client())
	for i := 0; i < 3; i++ {
		b, err := c.open(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.get(context.Background(), f.seed(fakeTicket{Name: "x", Status: 1})); err != nil {
			t.Fatal(err)
		}
	}
	if f.tokenCalls != 1 || f.lastGrant != "password" || f.lastScope != "api" {
		t.Fatalf("token calls=%d grant=%q scope=%q", f.tokenCalls, f.lastGrant, f.lastScope)
	}
}

func TestV2TokenExpiresBeforeExpiresIn(t *testing.T) {
	f := newFakeGLPI(t)
	cfg, err := parseConfig(f.config("v2"))
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(f.srv.Client())
	now := time.Unix(1_790_000_000, 0)
	c.now = func() time.Time { return now }
	if _, err := c.bearer(context.Background(), cfg, false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3600*time.Second - 59*time.Second)
	if _, err := c.bearer(context.Background(), cfg, false); err != nil {
		t.Fatal(err)
	}
	if f.tokenCalls != 2 {
		t.Fatalf("token reused inside the 60s margin: calls=%d", f.tokenCalls)
	}
}

func TestV2TokenRenewedOn401(t *testing.T) {
	f := newFakeGLPI(t)
	id := f.seed(fakeTicket{Name: "x", Status: 2})
	c := NewClient(f.srv.Client())
	cfg := f.config("v2")
	if err := c.Test(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	f.revokeBearers = true
	if err := c.Update(context.Background(), cfg, incident.Ref{ExternalID: itoa(id)}, incident.Event{Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if f.tokenCalls != 2 || f.tickets[id].Followups != 1 {
		t.Fatalf("calls=%d followups=%d", f.tokenCalls, f.tickets[id].Followups)
	}
}

func TestV2BadGrantIsOperatorMessage(t *testing.T) {
	f := newFakeGLPI(t)
	cfg := f.config("v2")
	cfg["oauth_password"] = "nope-secret-pw"
	err := NewClient(f.srv.Client()).Test(context.Background(), cfg)
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "credentials were incorrect") {
		t.Fatalf("err = %v", err)
	}
}

func TestV2TokenCacheConcurrent(t *testing.T) {
	f := newFakeGLPI(t)
	c := NewClient(f.srv.Client())
	cfgA := f.config("v2")
	cfgB := f.config("v2")
	cfgB["oauth_username"] = "other"
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := cfgA
			if i%2 == 1 {
				cfg = cfgB
			}
			if err := c.Test(context.Background(), cfg); err != nil && i%2 == 0 {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}
