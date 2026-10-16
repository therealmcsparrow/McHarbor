// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

func TestRestartWithoutASpecIsRefusedRatherThanLeftStopped(t *testing.T) {
	var replies []WSMessage
	rt := &computeRuntime{
		cts:  lxc.NewManager(nil),
		send: func(msg WSMessage) error { replies = append(replies, msg); return nil },
	}

	rt.handleCT(context.Background(), WSMessage{
		Type: MsgCTRestart,
		ID:   "req-1",
		CT:   &CTPayload{ContainerID: "d0testct0000000000rs"},
	})

	if len(replies) != 1 || replies[0].CT == nil {
		t.Fatalf("replies = %+v, want one ct_result", replies)
	}
	reply := replies[0].CT
	if reply.Success {
		t.Fatal("a restart with no spec answered success; it stops the container and never starts it")
	}
	if !strings.Contains(reply.Error, "no spec") {
		t.Fatalf("error = %q, want it to name the missing spec", reply.Error)
	}
}

func TestAStatsQueryIsAnsweredWithASampleNotAReconcile(t *testing.T) {
	var replies []WSMessage
	rt := &computeRuntime{
		cts:  lxc.NewManager(nil),
		send: func(msg WSMessage) error { replies = append(replies, msg); return nil },
	}

	rt.handleCT(context.Background(), WSMessage{
		Type: MsgCTQuery,
		ID:   "req-2",
		CT:   &CTPayload{Operation: ctQueryStats, ContainerID: "d0testct0000000000st"},
	})

	if len(replies) != 1 || replies[0].CT == nil {
		t.Fatalf("replies = %+v, want one ct_result", replies)
	}
	reply := replies[0].CT
	// A container this host has never seen cannot be sampled. A reconcile would
	// have answered success with no stats, which the plugin read as "not running".
	if reply.Success || reply.Stats != nil || !strings.Contains(reply.Error, "not found") {
		t.Fatalf("reply = success %v, stats %+v, error %q; want the sample's not-found error",
			reply.Success, reply.Stats, reply.Error)
	}
}
