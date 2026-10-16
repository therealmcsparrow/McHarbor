// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

func TestInitSessionHeaders(t *testing.T) {
	f := newFakeGLPI(t)
	c := NewClient(f.srv.Client())
	cfg, err := parseConfig(f.config("v1"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if f.lastInitAuth != "user_token UT" || f.lastInitApp != "AT" {
		t.Fatalf("init headers: %q %q", f.lastInitAuth, f.lastInitApp)
	}
	if err := b.close(); err != nil {
		t.Fatal(err)
	}
	if f.killed != 1 {
		t.Fatalf("killSession calls = %d", f.killed)
	}
}

func TestSessionKilledOnError(t *testing.T) {
	f := newFakeGLPI(t)
	f.failTicketPost = http.StatusInternalServerError
	_, err := NewClient(f.srv.Client()).Open(context.Background(), f.config("v1"),
		incident.Event{DedupKey: "k", State: incident.StateFiring, Severity: incident.SeverityInfo, Title: "t"})
	if !errors.Is(err, incident.ErrRetryable) {
		t.Fatalf("err = %v", err)
	}
	if f.killed != 1 {
		t.Fatalf("session leaked: killed=%d", f.killed)
	}
}

func TestSessionKilledWhenCallerContextCancelled(t *testing.T) {
	f := newFakeGLPI(t)
	c := NewClient(f.srv.Client())
	cfg, err := parseConfig(f.config("v1"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err = c.with(ctx, cfg, true, func(b glpiBackend) error {
		cancel()
		_, err := b.get(ctx, 1)
		return err
	})
	if err == nil {
		t.Fatal("call on a cancelled context succeeded")
	}
	if f.killed != 1 {
		t.Fatalf("killSession must use its own context: killed=%d", f.killed)
	}
}

func TestV1ReinitOnSessionExpiry(t *testing.T) {
	f := newFakeGLPI(t)
	id := f.seed(fakeTicket{Name: "x", Status: 2})
	f.expireSession = true
	err := NewClient(f.srv.Client()).Update(context.Background(), f.config("v1"),
		incident.Ref{ExternalID: itoa(id)}, incident.Event{Title: "t", Message: "again"})
	if err != nil {
		t.Fatal(err)
	}
	if f.inits != 2 || f.tickets[id].Followups != 1 || f.killed != 1 {
		t.Fatalf("inits=%d followups=%d killed=%d", f.inits, f.tickets[id].Followups, f.killed)
	}
}

func TestV1UserTokenRejectedIsOperatorMessage(t *testing.T) {
	f := newFakeGLPI(t)
	cfg := f.config("v1")
	cfg["user_token"] = "WRONG-TOKEN-VALUE"
	err := NewClient(f.srv.Client()).Test(context.Background(), cfg)
	if !errors.Is(err, incident.ErrOperatorMessage) || !strings.Contains(err.Error(), "user API token") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "WRONG-TOKEN-VALUE") {
		t.Fatalf("token leaked: %v", err)
	}
}
