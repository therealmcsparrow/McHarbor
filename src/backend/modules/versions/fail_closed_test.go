// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"context"
	"errors"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/nativeupdate"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

// TestStableHostWithSwitchOffRefusesAnythingThatIsNotPlainStable: the check fails closed, so a
// prerelease written in a form the strict parser refuses, or hidden behind a digest, still stops.
func TestStableHostWithSwitchOffRefusesAnythingThatIsNotPlainStable(t *testing.T) {
	const img = "ghcr.io/x/mcharbor"
	cases := []struct {
		name, image, version string
		native               bool
		want                 error
	}{
		{"image rc", img + ":2026.11.02-rc.1", "", false, ErrPrereleaseOff},
		{"image rc with digest", img + ":2026.11.02-rc.1@sha256:abc", "", false, ErrPrereleaseOff},
		{"image rc hyphen", img + ":2026.11.02-rc-1", "", false, ErrPrereleaseOff},
		{"image rc underscore", img + ":2026.11.02-rc_1", "", false, ErrPrereleaseOff},
		{"image uppercase V rc", img + ":V2026.11.02-rc.1", "", false, ErrPrereleaseOff},
		{"image uppercase rc", img + ":2026.11.02-RC.1", "", false, ErrPrereleaseOff},
		{"image uppercase V stable", img + ":V2026.11.02", "", false, nativeupdate.ErrInvalidVersion},
		{"image digest only", img + "@sha256:abc", "", false, ErrDigestPinned},
		{"image latest with digest", img + ":latest@sha256:abc", "", false, ErrDigestPinned},
		{"image empty tag with digest", img + ":@sha256:abc", "", false, ErrDigestPinned},
		{"image stable tag with digest", img + ":2026.11.02@sha256:abc", "", false, ErrDigestPinned},
		{"image v stable tag with digest", img + ":v2026.11.02@sha256:abc", "", false, ErrDigestPinned},
		{"image junk tag", img + ":nightly", "", false, nativeupdate.ErrInvalidVersion},
		{"native rc", "", "2026.11.02-rc.1", true, ErrPrereleaseOff},
		{"native rc hyphen", "", "2026.11.02-rc-1", true, ErrPrereleaseOff},
		{"native rc underscore", "", "2026.11.02-rc_1", true, ErrPrereleaseOff},
		{"native uppercase V rc", "", "V2026.11.02-rc.1", true, ErrPrereleaseOff},
		{"native junk", "", "nightly", true, nativeupdate.ErrInvalidVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newUpdateFixture(t, "2026.11.01", tc.native, updatechannel.Patch{})
			if _, err := f.SelfUpdate(context.Background(), tc.image, tc.version); !errors.Is(err, tc.want) {
				t.Fatalf("SelfUpdate = %v, want %v", err, tc.want)
			}
			if len(f.nativeCalls) != 0 || len(f.imageCalls) != 0 {
				t.Fatalf("a refused update started native=%v image=%v", f.nativeCalls, f.imageCalls)
			}
		})
	}
}

// TestStableHostWithSwitchOffStillTakesStableTargets: the stricter check adds no refusal for a
// target that is a plain release or a reference that carries no version at all.
func TestStableHostWithSwitchOffStillTakesStableTargets(t *testing.T) {
	const img = "ghcr.io/x/mcharbor"
	for _, image := range []string{"", img, img + ":latest", img + ":2026.11.02", img + ":v2026.11.02",
		"localhost:5000/mcharbor", img + ":2026.11.02+build.5"} {
		f := newUpdateFixture(t, "2026.11.01", false, updatechannel.Patch{})
		if _, err := f.SelfUpdate(context.Background(), image, ""); err != nil {
			t.Errorf("image %q: %v", image, err)
		}
	}
	for _, v := range []string{"", "2026.11.02"} {
		f := newUpdateFixture(t, "2026.11.01", true, updatechannel.Patch{})
		if _, err := f.SelfUpdate(context.Background(), "", v); err != nil {
			t.Errorf("native %q: %v", v, err)
		}
	}
}

// TestSwitchOnStillAllowsAPrereleaseReference: the switch is what decides, so a tagged prerelease,
// with or without a digest, installs when it is on.
func TestSwitchOnStillAllowsAPrereleaseReference(t *testing.T) {
	for _, image := range []string{"ghcr.io/x/mcharbor:2026.11.02-rc.1", "ghcr.io/x/mcharbor:2026.11.02-rc.1@sha256:abc", "ghcr.io/x/mcharbor@sha256:abc", "ghcr.io/x/mcharbor:latest@sha256:abc", "ghcr.io/x/mcharbor:2026.11.02@sha256:abc"} {
		f := newUpdateFixture(t, "2026.11.01", false, updatechannel.Patch{McHarborPrereleases: on()})
		if _, err := f.SelfUpdate(context.Background(), image, ""); err != nil {
			t.Errorf("image %q with the switch on: %v", image, err)
		}
	}
}
