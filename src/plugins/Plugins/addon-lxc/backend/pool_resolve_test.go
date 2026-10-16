// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"testing"
)

// TestCreateTakesThePoolOnRecordNotTheClients: once a pool has a container on a
// host, a create naming that pool id is held to the stored backend and target, and
// may leave them out; only a first use falls back to the request's shape.
func TestCreateTakesThePoolOnRecordNotTheClients(t *testing.T) {
	rpc := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, rpc)
	ctx := context.Background()

	if _, err := svc.Create(ctx, createRequest()); err != nil {
		t.Fatalf("first use: %v", err)
	}

	forged := createRequest()
	forged.Name = "forged"
	forged.PoolTarget = "/etc"
	if _, err := svc.Create(ctx, forged); !errors.Is(err, ErrPoolMismatch) {
		t.Fatalf("a different target for a recorded pool: err = %v, want ErrPoolMismatch", err)
	}

	omitted := createRequest()
	omitted.Name = "omitted"
	omitted.PoolBackend, omitted.PoolTarget = "", ""
	ct, err := svc.Create(ctx, omitted)
	if err != nil {
		t.Fatalf("omitting backend and target of a recorded pool: %v", err)
	}
	if ct.PoolBackend != "dir" || ct.PoolTarget != "/mcharbor/compute" {
		t.Fatalf("resolved pool = %s at %s, want dir at /mcharbor/compute", ct.PoolBackend, ct.PoolTarget)
	}
	if sent, ok := rpc.last(msgCTCreate); !ok || sent.Spec == nil || sent.Spec.PoolTarget != "/mcharbor/compute" {
		t.Fatalf("the agent was not sent the recorded target: %+v", sent.Spec)
	}
}

func TestFirstUseOfAPoolMustNameAUsableShape(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	ctx := context.Background()

	for name, mutate := range map[string]func(*CreateRequest){
		"no target":          func(r *CreateRequest) { r.PoolTarget = "" },
		"unknown backend":    func(r *CreateRequest) { r.PoolBackend = "nfs" },
		"relative path":      func(r *CreateRequest) { r.PoolTarget = "mcharbor/compute" },
		"unclean path":       func(r *CreateRequest) { r.PoolTarget = "/mcharbor/../etc" },
		"the root directory": func(r *CreateRequest) { r.PoolTarget = "/" },
	} {
		req := createRequest()
		mutate(&req)
		if _, err := svc.Create(ctx, req); !errors.Is(err, ErrPoolUnusable) {
			t.Errorf("%s: err = %v, want ErrPoolUnusable", name, err)
		}
	}
}

// TestCloneIntoAnotherPoolIsResolvedToo: a clone naming a pool is held to the same
// rule as a create, so it is not a side door around it.
func TestCloneIntoAnotherPoolIsResolvedToo(t *testing.T) {
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	ctx := context.Background()
	source, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Clone(ctx, source.ID, CloneRequest{
		Name: "copy", PoolID: "pool-a", PoolBackend: "dir", PoolTarget: "/etc",
	})
	if !errors.Is(err, ErrPoolMismatch) {
		t.Fatalf("clone naming a different target for the same pool: err = %v, want ErrPoolMismatch", err)
	}
}
