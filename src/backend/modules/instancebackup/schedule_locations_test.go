// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"testing"
)

// TestSetScheduleLocationsIsAtomic guards the replace: a link that fails midway
// must leave the schedule's previous destinations in place rather than empty.
func TestSetScheduleLocationsIsAtomic(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	seedLocation(t, svc, "loc-a", "A")
	seedLocation(t, svc, "loc-b", "B")
	if _, err := svc.db.Exec(`INSERT INTO instance_backup_schedules (id, name, cron, key_kind, key_secret_enc)
		VALUES ('sch-1', 'nightly', '0 3 * * *', 'passphrase', 'x')`); err != nil {
		t.Fatalf("seeding schedule: %v", err)
	}
	if err := svc.setScheduleLocations(ctx, "sch-1", []string{"loc-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`CREATE TRIGGER refuse_loc_b BEFORE INSERT ON instance_backup_schedule_storage_locations
		WHEN NEW.storage_location_id = 'loc-b' BEGIN SELECT RAISE(ABORT, 'refused'); END`); err != nil {
		t.Fatalf("creating trigger: %v", err)
	}

	if err := svc.setScheduleLocations(ctx, "sch-1", []string{"loc-b"}); err == nil {
		t.Fatal("expected the failing link to surface as an error")
	}

	var kept int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM instance_backup_schedule_storage_locations
		WHERE schedule_id = 'sch-1' AND storage_location_id = 'loc-a'`).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("the previous destination was lost after a failed replace: %d rows", kept)
	}
}
