// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// TestRebootDrainListsOnlyLiveGuestsOfTheEnvironment: the running list is the agent's answer,
// limited to guests this plugin has on record, with only a running guest whose monitor
// answers counted as graceful.
func TestRebootDrainListsOnlyLiveGuestsOfTheEnvironment(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	insertIntentTestVM(t, db, "vm2", StatePaused)
	insertIntentTestVM(t, db, "vm3", StateStopped)
	rpc.reply = []byte(`{"states":[` +
		`{"vmId":"vm1","state":"running","qmpReachable":true},` +
		`{"vmId":"vm2","state":"paused","qmpReachable":true},` +
		`{"vmId":"vm3","state":"stopped"},` +
		`{"vmId":"ghost","state":"running","qmpReachable":true}]}`)

	got, err := svc.RebootDrainRunning(context.Background(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	want := []rebootDrainGuest{
		{ID: "vm1", Name: "guest-vm1", Graceful: true},
		{ID: "vm2", Name: "guest-vm2", Graceful: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("running = %+v, want %+v", got, want)
	}

	rpc.calls = nil
	none, err := svc.RebootDrainRunning(context.Background(), "env-empty")
	if err != nil || len(none) != 0 || len(rpc.calls) != 0 {
		t.Fatalf("an environment with no guests must answer [] without asking the agent: %v %v %v", none, err, rpc.calls)
	}
}

// TestRebootDrainStopRecordsTheHostRebootIntent: the stop is recorded as the requester's, from
// the host reboot, and an escalation is reported rather than hidden.
func TestRebootDrainStopRecordsTheHostRebootIntent(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	rpc.reply = []byte(`{"errorCode":"escalated_to_hard_stop"}`)
	ctx := withIntentOrigin(context.Background(), userActor("u1"), intentSourceHostReboot)

	escalated, err := svc.RebootDrainStop(ctx, "env1", "vm1")
	if err != nil || !escalated {
		t.Fatalf("stop = %v %v, want an escalation", escalated, err)
	}
	rows := intentRows(t, db, "vm1")
	want := intentRow{action: intentShutdown, actor: "user:u1", source: intentSourceHostReboot, outcome: intentOK}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("intent rows = %+v, want [%+v]", rows, want)
	}
	if rpc.calls[len(rpc.calls)-1] != msgVMShutdown {
		t.Fatalf("calls = %v, want a %s last", rpc.calls, msgVMShutdown)
	}
}

// TestRebootDrainStopRefusesAGuestElsewhere: a guest on another environment is never stopped
// for this environment's reboot.
func TestRebootDrainStopRefusesAGuestElsewhere(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	rpc.calls = nil
	if _, err := svc.RebootDrainStop(context.Background(), "env-other", "vm1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want a not-found refusal", err)
	}
	if len(rpc.calls) != 0 || len(intentRows(t, db, "vm1")) != 0 {
		t.Fatalf("a refused stop reached the agent (%v) or the ledger", rpc.calls)
	}
}

// TestRebootDrainStopNobodyWaitedForIsNotAFailure: when the host stops listening the shutdown
// is still running on the agent, so the ledger must not record a failed stop.
func TestRebootDrainStopNobodyWaitedForIsNotAFailure(t *testing.T) {
	svc, rpc, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateRunning)
	rpc.fail = context.DeadlineExceeded
	if _, err := svc.RebootDrainStop(context.Background(), "env1", "vm1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want the deadline passed through", err)
	}
	rows := intentRows(t, db, "vm1")
	if len(rows) != 1 || rows[0].outcome != intentOK {
		t.Fatalf("rows = %+v, want one intent recorded as asked for", rows)
	}
}

// TestRebootDrainStopHandlerNeedsAnEnvironmentAndAGuest holds the route's input checks.
func TestRebootDrainStopHandlerNeedsAnEnvironmentAndAGuest(t *testing.T) {
	svc, _, _ := newIntentTestService(t)
	h := &Handler{svc: svc, logger: quietLogger{}}
	call := func(target, body string, withUser bool) int {
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		if withUser {
			req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1"}))
		}
		rec := httptest.NewRecorder()
		h.HandleRebootDrainStop(rec, req)
		return rec.Code
	}
	if code := call("/api/vms/reboot-drain?env=env1", `{"id":"vm1"}`, false); code != http.StatusUnauthorized {
		t.Errorf("no user: %d", code)
	}
	if code := call("/api/vms/reboot-drain", `{"id":"vm1"}`, true); code != http.StatusBadRequest {
		t.Errorf("no environment: %d", code)
	}
	if code := call("/api/vms/reboot-drain?env=env1", `{}`, true); code != http.StatusBadRequest {
		t.Errorf("no guest: %d", code)
	}
}
