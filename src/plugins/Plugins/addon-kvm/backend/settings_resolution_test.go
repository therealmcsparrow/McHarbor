// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"slices"
	"testing"
)

func TestValidateDisplayResolution(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		ok            bool
	}{
		{0, 0, true},
		{1920, 1080, true},
		{640, 480, true},
		{7680, 4320, true},
		{1920, 0, false},
		{0, 1080, false},
		{639, 480, false},
		{7681, 4320, false},
		{1920, 4321, false},
	} {
		err := validateDisplayResolution(tc.width, tc.height)
		if (err == nil) != tc.ok {
			t.Errorf("%dx%d: err = %v, want ok=%v", tc.width, tc.height, err, tc.ok)
		}
		if err != nil {
			if code, isValidation := validationCode(err); !isValidation || code != codeDisplayResolution {
				t.Errorf("%dx%d: code = %q, want %s", tc.width, tc.height, code, codeDisplayResolution)
			}
		}
	}
}

// TestBootResolutionIsColdAndReachesTheSpec covers the whole path: stored on a
// stopped guest, read back, sent to the agent, and listed as cold because QEMU
// only reads it at launch.
func TestBootResolutionIsColdAndReachesTheSpec(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	ctx := context.Background()
	insertIntentTestVM(t, db, "vm1", StateStopped)

	if _, err := svc.Update(ctx, "vm1", UpdateRequest{DisplayWidth: ptr(1920), DisplayHeight: ptr(1080)}); err != nil {
		t.Fatalf("setting a boot resolution: %v", err)
	}
	vm, err := svc.Get(ctx, "vm1")
	if err != nil || vm.DisplayWidth != 1920 || vm.DisplayHeight != 1080 {
		t.Fatalf("stored resolution = %dx%d (%v), want 1920x1080", vm.DisplayWidth, vm.DisplayHeight, err)
	}
	if spec := svc.buildSpec(ctx, vm); spec.DisplayWidth != 1920 || spec.DisplayHeight != 1080 {
		t.Fatalf("spec resolution = %dx%d, want 1920x1080", spec.DisplayWidth, spec.DisplayHeight)
	}

	_, err = svc.Update(ctx, "vm1", UpdateRequest{DisplayHeight: ptr(0)})
	if code, ok := validationCode(err); !ok || code != codeDisplayResolution {
		t.Fatalf("clearing one side: err = %v, want %s", err, codeDisplayResolution)
	}

	if !slices.Contains(coldSettings, "displayWidth") || !slices.Contains(coldSettings, "displayHeight") {
		t.Fatal("the boot resolution must be a cold setting: QEMU reads it only at launch")
	}
	if got := changedColdSettings(UpdateRequest{DisplayWidth: ptr(1280)}); !slices.Contains(got, "displayWidth") {
		t.Fatalf("changedColdSettings = %v, want displayWidth", got)
	}
}
