// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "testing"

func TestProviderStatusAcceptsAliases(t *testing.T) {
	cases := map[string]Status{
		"good": StatusGood, "Success": StatusGood, "ok": StatusGood,
		"warn": StatusWarn, "warning": StatusWarn,
		"bad": StatusBad, "danger": StatusBad, "error": StatusBad, "critical": StatusBad,
		"info": StatusInfo, "muted": StatusMuted,
		"": StatusNeutral, "purple": StatusNeutral,
	}
	for raw, want := range cases {
		if got := providerStatus(raw); got != want {
			t.Errorf("providerStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}
