// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"testing"
	"time"
)

func TestAutomaticImagePruneDueBeforeRunHour(t *testing.T) {
	env := automationEnvironment{Timezone: "Europe/Amsterdam"}
	now := time.Date(2026, time.March, 13, 1, 30, 0, 0, time.UTC)

	if automaticImagePruneDue(env, now) {
		t.Fatal("expected prune to wait until the scheduled hour")
	}
}

func TestAutomaticImagePruneDueAfterRunHourWithoutPreviousRun(t *testing.T) {
	env := automationEnvironment{Timezone: "Europe/Amsterdam"}
	now := time.Date(2026, time.March, 13, 2, 30, 0, 0, time.UTC)

	if !automaticImagePruneDue(env, now) {
		t.Fatal("expected prune to run once the scheduled hour has passed")
	}
}

func TestAutomaticImagePruneDueSkipsSecondRunOnSameLocalDay(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatalf("loading location: %v", err)
	}

	lastRun := time.Date(2026, time.March, 13, 3, 15, 0, 0, loc).UTC().Format(time.RFC3339)
	env := automationEnvironment{
		Timezone:                     "Europe/Amsterdam",
		LastAutomaticImagePruneRunAt: lastRun,
	}
	now := time.Date(2026, time.March, 13, 8, 0, 0, 0, loc).UTC()

	if automaticImagePruneDue(env, now) {
		t.Fatal("expected prune to skip a second run on the same local day")
	}
}

func TestAutomaticImagePruneDueRunsAgainOnNextLocalDay(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatalf("loading location: %v", err)
	}

	lastRun := time.Date(2026, time.March, 13, 3, 15, 0, 0, loc).UTC().Format(time.RFC3339)
	env := automationEnvironment{
		Timezone:                     "Europe/Amsterdam",
		LastAutomaticImagePruneRunAt: lastRun,
	}
	now := time.Date(2026, time.March, 14, 4, 0, 0, 0, loc).UTC()

	if !automaticImagePruneDue(env, now) {
		t.Fatal("expected prune to run again on the next local day")
	}
}
