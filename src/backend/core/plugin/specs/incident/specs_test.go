// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incident

import "testing"

func TestEventValidate(t *testing.T) {
	cases := []struct {
		name string
		ev   Event
		ok   bool
	}{
		{"firing ok", Event{DedupKey: "a:e:s", State: StateFiring, Severity: SeverityCritical, Title: "x"}, true},
		{"missing key", Event{State: StateFiring, Severity: SeverityCritical, Title: "x"}, false},
		{"bad state", Event{DedupKey: "k", State: "open", Severity: SeverityInfo, Title: "x"}, false},
		{"bad severity", Event{DedupKey: "k", State: StateResolved, Severity: "p1", Title: "x"}, false},
		{"key too long", Event{DedupKey: string(make([]byte, 256)), State: StateFiring, Severity: SeverityInfo, Title: "x"}, false},
	}
	for _, c := range cases {
		if err := c.ev.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestRemoteStateTerminal(t *testing.T) {
	if !RemoteResolved.Terminal() || !RemoteClosed.Terminal() || !RemoteCancelled.Terminal() {
		t.Fatal("resolved/closed/cancelled must be terminal")
	}
	if RemoteOpen.Terminal() || RemoteAcknowledged.Terminal() {
		t.Fatal("open/acknowledged must not be terminal")
	}
}
