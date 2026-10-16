// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// poolCapture records the pool every vm_disk_create carried.
type poolCapture struct {
	mu   sync.Mutex
	seen []agentDisk
}

func (c *poolCapture) answer(payload agentPayload) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if payload.Disk != nil {
		c.seen = append(c.seen, *payload.Disk)
	}
	return `{"success":true,"disk":{"volumePath":"/pool/vm1-disk-2.qcow2","sizeBytes":1073741824}}`, nil
}

func TestAddDiskWithOnlyAPoolIDUsesTheRecordedBackendAndTarget(t *testing.T) {
	svc, rpc, db := newHotplugTestService(t, StateStopped, true)
	capture := &poolCapture{}
	rpc.answer[msgVMDiskCreate] = capture.answer

	disk, err := svc.AddDisk(context.Background(), "vm1", DiskRequest{PoolID: "pool", SizeGB: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.seen) != 1 || capture.seen[0].PoolBackend != "dir" || capture.seen[0].PoolTarget != "/pool" {
		t.Fatalf("the agent was sent %+v, want backend dir at /pool", capture.seen)
	}
	if disk.PoolBackend != "dir" || disk.PoolTarget != "/pool" {
		t.Fatalf("the disk was recorded as %s at %s", disk.PoolBackend, disk.PoolTarget)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE id = ? AND pool_target = '/pool'`, disk.ID); n != 1 {
		t.Fatal("the stored row does not carry the recorded target")
	}
}

func TestAddDiskRefusesATargetThatDiffersFromTheRecord(t *testing.T) {
	cases := map[string]DiskRequest{
		"target":  {PoolID: "pool", PoolBackend: "dir", PoolTarget: "/srv/elsewhere", SizeGB: 1},
		"backend": {PoolID: "pool", PoolBackend: "zfs", PoolTarget: "/pool", SizeGB: 1},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			svc, rpc, db := newHotplugTestService(t, StateStopped, true)
			_, err := svc.AddDisk(context.Background(), "vm1", req)
			if !errors.Is(err, ErrPoolMismatch) {
				t.Fatalf("err = %v, want ErrPoolMismatch", err)
			}
			if calls := rpc.sent(); contains(calls, msgVMDiskCreate) {
				t.Fatalf("a mismatching pool reached the agent: %v", calls)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM plugin_addon_kvm_disks WHERE vm_id = 'vm1'`); n != 2 {
				t.Fatalf("disk rows = %d, want the original 2", n)
			}
		})
	}
}

func TestAMatchingClientPoolIsAccepted(t *testing.T) {
	svc, _, _ := newHotplugTestService(t, StateStopped, true)
	if _, err := svc.AddDisk(context.Background(), "vm1",
		DiskRequest{PoolID: "pool", PoolBackend: "dir", PoolTarget: "/pool", SizeGB: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestAPoolUnknownOnThisHostNeedsAWellFormedBackendAndTarget(t *testing.T) {
	svc, _, _ := newHotplugTestService(t, StateStopped, true)
	ctx := context.Background()
	for name, ref := range map[string]PoolRef{
		"no target":      {ID: "fresh", Backend: "dir"},
		"no backend":     {ID: "fresh", Target: "/srv/vms"},
		"unknown driver": {ID: "fresh", Backend: "nfs", Target: "/srv/vms"},
		"relative":       {ID: "fresh", Backend: "dir", Target: "srv/vms"},
		"dot-dot":        {ID: "fresh", Backend: "dir", Target: "/srv/../etc"},
		"root":           {ID: "fresh", Backend: "dir", Target: "/"},
	} {
		if _, err := svc.resolvePool(ctx, "env1", ref); err == nil {
			t.Errorf("%s: %+v was accepted", name, ref)
		}
	}
	got, err := svc.resolvePool(ctx, "env1", PoolRef{ID: "fresh", Backend: "zfs", Target: "tank/vms"})
	if err != nil || got.Target != "tank/vms" {
		t.Fatalf("a well-formed first use was refused: %+v %v", got, err)
	}
	// The record is per host: the same pool id elsewhere has no history.
	if _, err := svc.resolvePool(ctx, "env2", PoolRef{ID: "pool"}); err == nil {
		t.Fatal("a pool recorded on env1 was resolved for env2")
	}
}

func TestPoolMismatchReachesTheClientAsAConflict(t *testing.T) {
	svc, _, _ := newHotplugTestService(t, StateStopped, true)
	h := NewHandler(svc, nil, nil, quietLogger{})
	router := chi.NewRouter()
	router.Post("/api/vms/{id}/disks", h.HandleAddDisk)

	for _, tc := range []struct {
		body   string
		status int
		code   string
	}{
		{`{"poolId":"pool","poolTarget":"/srv/elsewhere","sizeGb":1}`, http.StatusConflict, codePoolMismatch},
		{`{"sizeGb":1}`, http.StatusBadRequest, codePoolRequired},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/vms/vm1/disks", strings.NewReader(tc.body))
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1"}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var envelope apiEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if rec.Code != tc.status || envelope.Code != tc.code {
			t.Fatalf("%s: got %d %q, want %d %q", tc.body, rec.Code, envelope.Code, tc.status, tc.code)
		}
	}
}
