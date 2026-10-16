// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// hostGuestEvent is a guest event as the host's golden records it.
type hostGuestEvent struct {
	EnvID     string            `json:"envId"`
	GuestKind string            `json:"guestKind"`
	GuestID   string            `json:"guestId"`
	Kind      string            `json:"kind"`
	Device    string            `json:"device"`
	Detail    map[string]string `json:"detail"`
	At        time.Time         `json:"at"`
}

// TestGuestEventsMatchTheHostGoldens replays every vm_event frame the host recorded in
// converted/vm_events.json through this plugin's conversion. The host's own sink produced
// those events before the conversion moved here, so a divergence is a behaviour change.
func TestGuestEventsMatchTheHostGoldens(t *testing.T) {
	var cases []struct {
		Name        string          `json:"name"`
		EnvID       string          `json:"envId"`
		Frame       json.RawMessage `json:"frame"`
		Event       *hostGuestEvent `json:"event"`
		ReceiveTime bool            `json:"receiveTime"`
	}
	if err := json.Unmarshal(readFixture(t, "converted", "vm_events.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 4 {
		t.Fatalf("the golden holds %d cases, want at least 4", len(cases))
	}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, c := range cases {
		got, ok := guestEvent(c.EnvID, c.Frame, now)
		if ok != (c.Event != nil) {
			t.Errorf("%s: produced an event = %v, the host = %v", c.Name, ok, c.Event != nil)
			continue
		}
		if !ok {
			continue
		}
		want := GuestEvent{
			EnvID: c.Event.EnvID, GuestKind: c.Event.GuestKind, GuestID: c.Event.GuestID,
			Kind: c.Event.Kind, Device: c.Event.Device, Detail: c.Event.Detail, At: c.Event.At,
		}
		if c.ReceiveTime {
			want.At = now
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got  %+v\n want %+v", c.Name, got, want)
		}
	}
}

// TestImportProgressMatchesTheHostGoldens replays every vm_import_progress frame the host
// recorded in converted/import_progress.json and asks for the same import the host was asked.
func TestImportProgressMatchesTheHostGoldens(t *testing.T) {
	var cases []struct {
		Name     string          `json:"name"`
		EnvID    string          `json:"envId"`
		ImportID string          `json:"importId"`
		Frame    json.RawMessage `json:"frame"`
		Phase    string          `json:"phase"`
		Disk     int             `json:"disk"`
		Percent  int             `json:"percent"`
		Known    bool            `json:"known"`
	}
	if err := json.Unmarshal(readFixture(t, "converted", "import_progress.json"), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 8 {
		t.Fatalf("the golden holds %d cases, want at least 8", len(cases))
	}
	for _, c := range cases {
		bridge := newAgentBridge(&agentFake{})
		bridge.recordImportProgress(c.EnvID, c.Frame, time.Now())
		phase, disk, percent, known := bridge.ImportProgress(c.EnvID, c.ImportID)
		if phase != c.Phase || disk != c.Disk || percent != c.Percent || known != c.Known {
			t.Errorf("%s: got %q disk %d %d%% known=%v, the host %q disk %d %d%% known=%v",
				c.Name, phase, disk, percent, known, c.Phase, c.Disk, c.Percent, c.Known)
		}
	}
}
