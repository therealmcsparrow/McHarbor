// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestBackupProgressMatchesTheHostGoldens replays every ct_backup_progress frame the host
// recorded in converted/ct_events.json through this plugin's conversion. The host's sink
// produced those events before the conversion moved here; the receive time it stamped is
// not part of the conversion and this plugin does not carry one.
func TestBackupProgressMatchesTheHostGoldens(t *testing.T) {
	var cases []struct {
		Name  string          `json:"name"`
		EnvID string          `json:"envId"`
		Frame json.RawMessage `json:"frame"`
		Event *struct {
			EnvID     string            `json:"envId"`
			GuestKind string            `json:"guestKind"`
			GuestID   string            `json:"guestId"`
			Kind      string            `json:"kind"`
			Detail    map[string]string `json:"detail"`
		} `json:"event"`
	}
	if err := json.Unmarshal(readFixture(t, "converted", "ct_events.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 2 {
		t.Fatalf("the golden holds %d cases, want at least 2", len(cases))
	}
	for _, c := range cases {
		got, ok := backupProgressEvent(c.EnvID, c.Frame)
		if ok != (c.Event != nil) {
			t.Errorf("%s: produced an event = %v, the host = %v", c.Name, ok, c.Event != nil)
			continue
		}
		if !ok {
			continue
		}
		want := guestEvent{EnvID: c.Event.EnvID, GuestKind: c.Event.GuestKind, GuestID: c.Event.GuestID,
			Kind: c.Event.Kind, Detail: c.Event.Detail}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got  %+v\n want %+v", c.Name, got, want)
		}
	}
}
