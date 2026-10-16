// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newCreateTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	database, err := coredb.OpenSQLite(filepath.Join(dir, "storage_locations_test.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	enc, err := encryption.New(dir, "")
	if err != nil {
		t.Fatalf("encryption: %v", err)
	}
	return NewService(database, enc)
}

// TestCreateHonoursEnabled covers a location created disabled, which the API
// accepted and then stored as enabled; omitting the field still means enabled.
func TestCreateHonoursEnabled(t *testing.T) {
	svc := newCreateTestService(t)
	disabled := false
	cases := []struct {
		name         string
		locationType string
		enabled      *bool
		want         bool
	}{
		{"explicitly disabled", "sftp", &disabled, false},
		{"omitted", "sftp", nil, true},
		{"extra local honours it too", "local", &disabled, false},
	}
	for _, tc := range cases {
		loc, err := svc.Create(context.Background(), CreateStorageLocationInput{
			Name: "t-" + tc.name, LocationType: tc.locationType, Host: "sftp.invalid", BasePath: "/tmp/x", Enabled: tc.enabled,
		})
		if err != nil {
			t.Fatalf("%s: Create: %v", tc.name, err)
		}
		if loc.Enabled != tc.want {
			t.Errorf("%s: enabled = %v, want %v", tc.name, loc.Enabled, tc.want)
		}
	}
}

// TestOnlyTheBuiltInLocalLocationIsProtected covers the extra local location
// that could be created but never disabled or deleted; the seeded one stays locked.
func TestOnlyTheBuiltInLocalLocationIsProtected(t *testing.T) {
	svc := newCreateTestService(t)
	ctx := context.Background()
	extra, err := svc.Create(ctx, CreateStorageLocationInput{Name: "extra", LocationType: "local", BasePath: "/mnt/extra"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	off := false
	if _, err := svc.Update(ctx, extra.ID, UpdateStorageLocationInput{Enabled: &off}); err != nil {
		t.Errorf("disabling an extra local location: %v", err)
	}
	if deleted, err := svc.Delete(ctx, extra.ID); err != nil || !deleted {
		t.Errorf("deleting an extra local location: deleted=%v err=%v", deleted, err)
	}
	if _, err := svc.Update(ctx, DefaultLocalLocationID, UpdateStorageLocationInput{Enabled: &off}); !errors.Is(err, ErrLocalStorageProtected) {
		t.Errorf("disabling the built-in location: err = %v, want ErrLocalStorageProtected", err)
	}
	if _, err := svc.Delete(ctx, DefaultLocalLocationID); !errors.Is(err, ErrLocalStorageProtected) {
		t.Errorf("deleting the built-in location: err = %v, want ErrLocalStorageProtected", err)
	}
}
