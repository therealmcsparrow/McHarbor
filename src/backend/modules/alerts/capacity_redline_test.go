// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"testing"
)

// A disk that is full and flat is refused as not_rising before already_over is
// checked, so the red-line observation must key on the used percentage.
func TestCapacityConditionReportsAFullButFlatFilesystem(t *testing.T) {
	db := openCapacityAlertDB(t)
	seedCapacity(t, db, "flat", 14, 98, 0)
	obs, err := newHostSource(EngineDeps{DB: db}).Evaluate(context.Background(), capacityRequest())
	if err != nil || len(obs) != 1 {
		t.Fatalf("obs=%+v err=%v, want one", obs, err)
	}
	if obs[0].Value != 0 || obs[0].ValueLabel != "past the red line" || obs[0].Context["refusal"] == "" {
		t.Fatalf("unexpected observation %+v", obs[0])
	}
}

func TestCapacityConditionStaysSilentForAFlatFilesystemBelowTheRedLine(t *testing.T) {
	db := openCapacityAlertDB(t)
	seedCapacity(t, db, "flat", 14, 50, 0)
	obs, err := newHostSource(EngineDeps{DB: db}).Evaluate(context.Background(), capacityRequest())
	if err != nil || len(obs) != 0 {
		t.Fatalf("obs=%+v err=%v, want none", obs, err)
	}
}
