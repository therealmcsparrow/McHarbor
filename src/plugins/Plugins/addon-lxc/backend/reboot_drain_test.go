// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func insertDrainContainer(t *testing.T, svc *Service, id, env, state, initSystem string) {
	t.Helper()
	if err := svc.insertContainer(context.Background(), Container{
		ID: id, EnvironmentID: env, Name: "ct-" + id, State: state, InitSystem: initSystem,
		PoolID: "p1", PoolBackend: "dir", PoolTarget: "/pool", RootfsPath: "/pool/" + id,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

// TestRebootDrainListsOnlyLiveContainersOfTheEnvironment: the running list is the agent's
// answer for the containers on record, with only a running one counted as graceful.
func TestRebootDrainListsOnlyLiveContainersOfTheEnvironment(t *testing.T) {
	rpc := &fakeRPC{reply: func(messageType string, _ agentPayload) ([]byte, error) {
		return []byte(`{"success":true,"states":[` +
			`{"containerId":"c1","state":"running"},` +
			`{"containerId":"c2","state":"frozen"},` +
			`{"containerId":"c3","state":"stopped"},` +
			`{"containerId":"ghost","state":"running"}]}`), nil
	}}
	svc, _ := newTestService(t, rpc)
	insertDrainContainer(t, svc, "c1", "env1", StateRunning, "")
	insertDrainContainer(t, svc, "c2", "env1", StateFrozen, "")
	insertDrainContainer(t, svc, "c3", "env1", "stopped", "")

	got, err := svc.RebootDrainRunning(context.Background(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	want := []rebootDrainGuest{{ID: "c1", Name: "ct-c1", Graceful: true}, {ID: "c2", Name: "ct-c2", Graceful: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("running = %+v, want %+v", got, want)
	}
	query, ok := rpc.last(msgCTQuery)
	if !ok || len(query.States) != 3 || query.EnvID != "env1" {
		t.Fatalf("the query must name the three containers on record: %+v", query)
	}

	none, err := svc.RebootDrainRunning(context.Background(), "env-empty")
	if err != nil || len(none) != 0 {
		t.Fatalf("an environment with no containers must answer []: %v %v", none, err)
	}
}

// TestRebootDrainStopSendsTheContainersOwnInitSystem: the halt goes out with the container's
// own init system (systemd when it has none on record) and the long grace, and an escalation
// is reported rather than hidden.
func TestRebootDrainStopSendsTheContainersOwnInitSystem(t *testing.T) {
	escalate := false
	rpc := &fakeRPC{reply: func(string, agentPayload) ([]byte, error) {
		if escalate {
			return []byte(`{"success":true,"errorCode":"escalated_to_hard_stop"}`), nil
		}
		return []byte(`{"success":true}`), nil
	}}
	svc, _ := newTestService(t, rpc)
	insertDrainContainer(t, svc, "c1", "env1", StateRunning, InitOpenRC)
	insertDrainContainer(t, svc, "c2", "env1", StateRunning, "")

	escalated, err := svc.RebootDrainStop(context.Background(), "env1", "c1")
	if err != nil || escalated {
		t.Fatalf("stop = %v %v", escalated, err)
	}
	sent, _ := rpc.last(msgCTShutdown)
	if sent.ContainerID != "c1" || sent.GraceSeconds != rebootGraceSeconds || sent.Spec == nil || sent.Spec.InitSystem != InitOpenRC {
		t.Fatalf("sent %+v", sent)
	}
	escalate = true
	escalated, err = svc.RebootDrainStop(context.Background(), "env1", "c2")
	if err != nil || !escalated {
		t.Fatalf("an escalation must be reported: %v %v", escalated, err)
	}
	if sent, _ := rpc.last(msgCTShutdown); sent.Spec == nil || sent.Spec.InitSystem != InitSystemd {
		t.Fatalf("a container with no init system on record halts as systemd: %+v", sent.Spec)
	}
}

// TestRebootDrainStopRefusesAContainerElsewhere: a container on another environment is never
// stopped for this environment's reboot.
func TestRebootDrainStopRefusesAContainerElsewhere(t *testing.T) {
	rpc := &fakeRPC{}
	svc, _ := newTestService(t, rpc)
	insertDrainContainer(t, svc, "c1", "env1", StateRunning, "")
	if _, err := svc.RebootDrainStop(context.Background(), "env-other", "c1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want a not-found refusal", err)
	}
	if _, sent := rpc.last(msgCTShutdown); sent {
		t.Fatal("a refused stop reached the agent")
	}
}
