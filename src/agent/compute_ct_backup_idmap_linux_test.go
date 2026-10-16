// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"encoding/json"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
)

func TestRestoreSpecCarriesBothIDMapsToTheManager(t *testing.T) {
	var payload CTPayload
	body := `{"restore":{"runId":"r","targetPath":"/p/containers/b/rootfs",
		"sourceIdMap":{"uidStart":1000000,"uidCount":65536,"gidStart":1000000,"gidCount":65536},
		"targetIdMap":{"privileged":true}}}`
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	source := ctIDMap(payload.Restore.SourceIDMap)
	target := ctIDMap(payload.Restore.TargetIDMap)
	if source == nil || *source != (lxc.IDMap{UIDStart: 1000000, UIDCount: 65536, GIDStart: 1000000, GIDCount: 65536}) {
		t.Fatalf("source = %+v", source)
	}
	if target == nil || !target.Privileged {
		t.Fatalf("target = %+v", target)
	}
	if ctIDMap(nil) != nil {
		t.Fatal("an absent map became a zero range, which would re-own into ids 0-0")
	}
}

func TestRestoreReplyAcknowledgesTheShiftOnTheWire(t *testing.T) {
	var reply CTPayload
	acknowledgeRestore(&reply, lxc.RestoreResult{IDShift: lxc.IDShiftApplied, EntriesMoved: 525})
	if !reply.Success {
		t.Fatal("an acknowledged restore is not marked successful")
	}
	body, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		BackupResult struct {
			IDShift string `json:"idShift"`
		} `json:"backupResult"`
	}
	if err := json.Unmarshal(body, &back); err != nil || back.BackupResult.IDShift != "shifted" {
		t.Fatalf("the acknowledgement McHarbor reads is %q (%v) in %s", back.BackupResult.IDShift, err, body)
	}
}
