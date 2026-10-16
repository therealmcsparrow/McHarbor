// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

func TestParseGuestUnitsNamesEachGuest(t *testing.T) {
	out := "mcharbor-vm-cs1abc.service loaded active running McHarbor VM Home Assistant\n" +
		"mcharbor-ct-cs2def.service loaded inactive dead McHarbor CT pihole\n" +
		"mcharbor-tpm-cs1abc.service loaded active running McHarbor vTPM Home Assistant\n" +
		"mcharbor-vm-bad;id.service loaded active running McHarbor VM evil\n"
	got := parseGuestUnits(out)
	want := []HostLogTarget{{ID: "vm-cs1abc", Label: "Home Assistant"}, {ID: "ct-cs2def", Label: "pihole"}}
	if len(got) != len(want) {
		t.Fatalf("targets = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("target %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestQEMUTargetsReadTheGuestUnitJournal(t *testing.T) {
	for _, target := range []string{"vm-cs1abc", "ct-cs2def"} {
		if !isGuestUnitTarget(target) {
			t.Errorf("%s should read its unit journal", target)
		}
	}
	for _, target := range []string{"homeassistant", "vm-", "tpm-cs1abc"} {
		if isGuestUnitTarget(target) {
			t.Errorf("%s is not a guest unit", target)
		}
	}
}
