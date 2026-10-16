// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestClaimIsALease is the property that keeps a dead node from stranding an agent: a
// live claim cannot be stolen, an expired one can, and a released one is gone at once.
func TestClaimIsALease(t *testing.T) {
	database := migrated(t)
	ctx := context.Background()
	clock := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	a := NewOwners(database, "node-a", "http://node-a:8705")
	b := NewOwners(database, "node-b", "http://node-b:8705")
	for _, o := range []*Owners{a, b} {
		o.now = func() time.Time { return clock }
	}

	if err := a.Claim(ctx, "env-1"); err != nil {
		t.Fatalf("the first claim must succeed: %v", err)
	}
	owner, ok, err := b.Owner(ctx, "env-1")
	if err != nil || !ok || owner.NodeID != "node-a" || owner.AdvertiseURL != "http://node-a:8705" {
		t.Fatalf("every node must see who holds the agent, got %+v ok=%v err=%v", owner, ok, err)
	}

	if err := b.Claim(ctx, "env-1"); !errors.Is(err, ErrOwnedElsewhere) {
		t.Fatalf("a live claim must not be stolen, got %v", err)
	}

	// node-a renews; node-b still cannot take it.
	if err := a.Claim(ctx, "env-1"); err != nil {
		t.Fatalf("a renewal must succeed: %v", err)
	}
	if err := b.Claim(ctx, "env-1"); !errors.Is(err, ErrOwnedElsewhere) {
		t.Fatalf("a renewed claim must not be stolen, got %v", err)
	}

	// node-a dies: time moves past the lease and nobody renews.
	late := clock.Add(OwnerLease + time.Second)
	b.now = func() time.Time { return late }
	if _, ok, err := b.Owner(ctx, "env-1"); err != nil || ok {
		t.Fatal("an expired claim must not read as a live owner")
	}
	if err := b.Claim(ctx, "env-1"); err != nil {
		t.Fatalf("an expired claim must be takeable: %v", err)
	}
	owner, ok, err = b.Owner(ctx, "env-1")
	if err != nil || !ok || owner.NodeID != "node-b" {
		t.Fatalf("the taking node must now own it, got %+v ok=%v err=%v", owner, ok, err)
	}

	if err := a.Release(ctx, "env-1"); err != nil {
		t.Fatalf("releasing what this node does not own must not error: %v", err)
	}
	if _, ok, err := b.Owner(ctx, "env-1"); err != nil || !ok {
		t.Fatal("one node must not release another node's claim")
	}
	if err := b.Release(ctx, "env-1"); err != nil {
		t.Fatalf("release must succeed: %v", err)
	}
	if _, ok, err := b.Owner(ctx, "env-1"); err != nil || ok {
		t.Fatal("a released claim must be gone at once rather than after the lease")
	}
}

// TestListSkipsExpiredClaims keeps the diagnostic and the topology honest: a dead node's
// row is not a node to forward to.
func TestListSkipsExpiredClaims(t *testing.T) {
	database := migrated(t)
	ctx := context.Background()
	clock := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	a := NewOwners(database, "node-a", "http://node-a:8705")
	a.now = func() time.Time { return clock }
	if err := a.Claim(ctx, "env-1"); err != nil {
		t.Fatal(err)
	}
	if list, err := a.List(ctx); err != nil || len(list) != 1 {
		t.Fatalf("a live claim must be listed, got %d (%v)", len(list), err)
	}
	a.now = func() time.Time { return clock.Add(OwnerLease + time.Second) }
	if list, err := a.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("an expired claim must not be listed, got %d (%v)", len(list), err)
	}
}
