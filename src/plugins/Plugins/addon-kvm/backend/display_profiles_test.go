// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func strPtr(value string) *string { return &value }

func storedProfile(t *testing.T, db *sql.DB, vmID string) sql.NullString {
	t.Helper()
	var stored sql.NullString
	if err := db.QueryRow(`SELECT display_profile_id FROM plugin_addon_kvm_vms WHERE id = ?`, vmID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestBuiltinDisplayProfilesAreImmutable(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	ctx := context.Background()

	for _, id := range []string{ProfileFastLAN, ProfileBalanced, ProfileLowBandwidth} {
		if _, err := svc.UpdateDisplayProfile(ctx, id, DisplayProfileRequest{ImageCompression: strPtr("glz")}); !errors.Is(err, ErrDisplayProfileBuiltin) {
			t.Errorf("updating %s: err = %v, want ErrDisplayProfileBuiltin", id, err)
		}
		if _, err := svc.DeleteDisplayProfile(ctx, id, true); !errors.Is(err, ErrDisplayProfileBuiltin) {
			t.Errorf("deleting %s: err = %v, want ErrDisplayProfileBuiltin", id, err)
		}
	}
	var image string
	if err := db.QueryRow(`SELECT image_compression FROM plugin_addon_kvm_display_profiles WHERE id = ?`,
		ProfileFastLAN).Scan(&image); err != nil || image != "off" {
		t.Fatalf("Fast LAN image compression = %q (%v), want it untouched at off", image, err)
	}

	// A hand-edited built-in is put back at the next Init.
	if _, err := db.Exec(`UPDATE plugin_addon_kvm_display_profiles SET image_compression = 'glz' WHERE id = ?`, ProfileFastLAN); err != nil {
		t.Fatal(err)
	}
	if err := svc.ensureDisplayProfileSchema(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := svc.DisplayProfile(ctx, ProfileFastLAN)
	if err != nil || profile.ImageCompression != "off" || !profile.Builtin {
		t.Fatalf("Fast LAN after Init = %+v (%v), want the built-in values back", profile, err)
	}
}

func TestDisplayProfileInUseDeleteIsRefusedWithItsCount(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	ctx := context.Background()
	custom, err := svc.CreateDisplayProfile(ctx, DisplayProfileRequest{Name: strPtr("office"), ImageCompression: strPtr("lz")})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"vm1", "vm2"} {
		insertIntentTestVM(t, db, id, StateStopped)
		if _, err := svc.Update(ctx, id, UpdateRequest{DisplayProfileID: strPtr(custom.ID)}); err != nil {
			t.Fatal(err)
		}
	}

	_, err = svc.DeleteDisplayProfile(ctx, custom.ID, false)
	var inUse *DisplayProfileInUseError
	if !errors.As(err, &inUse) || inUse.Count != 2 {
		t.Fatalf("delete in use: err = %v, want a DisplayProfileInUseError with 2", err)
	}
	if _, err := svc.DisplayProfile(ctx, custom.ID); err != nil {
		t.Fatalf("the refused delete removed the profile anyway: %v", err)
	}

	recorder := httptest.NewRecorder()
	(&Handler{logger: quietLogger{}}).respondDisplayProfileErr(recorder, httptest.NewRequest(http.MethodDelete, "/", nil), "delete", err)
	var body struct {
		Code string `json:"code"`
		Data struct {
			InUse int `json:"inUse"`
		} `json:"data"`
	}
	if jsonErr := json.Unmarshal(recorder.Body.Bytes(), &body); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if recorder.Code != http.StatusConflict || body.Code != codeDisplayProfileInUse || body.Data.InUse != 2 {
		t.Fatalf("response = %d %+v, want 409 %s with inUse 2", recorder.Code, body, codeDisplayProfileInUse)
	}

	moved, err := svc.DeleteDisplayProfile(ctx, custom.ID, true)
	if err != nil || moved != 2 {
		t.Fatalf("reassign and delete = %d, %v; want 2 moved", moved, err)
	}
	if _, err := svc.DisplayProfile(ctx, custom.ID); !errors.Is(err, ErrDisplayProfileNotFound) {
		t.Fatalf("profile still readable after delete: %v", err)
	}
	vm, err := svc.Get(ctx, "vm1")
	if err != nil || vm.DisplayProfileID != ProfileFastLAN || storedProfile(t, db, "vm1").Valid {
		t.Fatalf("reassigned guest = %q (%v), want Fast LAN stored as NULL", vm.DisplayProfileID, err)
	}
}

func TestDuplicateDisplayProfileCopiesIntoACustomProfile(t *testing.T) {
	svc, _, _ := newIntentTestService(t)
	ctx := context.Background()

	first, err := svc.DuplicateDisplayProfile(ctx, ProfileLowBandwidth)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "Low bandwidth (copy)" || first.Builtin || first.ID == ProfileLowBandwidth {
		t.Fatalf("duplicate = %+v, want a custom \"Low bandwidth (copy)\"", first)
	}
	if first.StreamingVideo != "all" || first.JPEGWANCompression != "always" || first.ImageCompression != "auto_lz" {
		t.Fatalf("duplicate options = %+v, want Low bandwidth's", first)
	}
	second, err := svc.DuplicateDisplayProfile(ctx, ProfileLowBandwidth)
	if err != nil || second.Name != "Low bandwidth (copy 2)" {
		t.Fatalf("second duplicate = %q (%v), want \"Low bandwidth (copy 2)\"", second.Name, err)
	}
	edited, err := svc.UpdateDisplayProfile(ctx, first.ID, DisplayProfileRequest{StreamingVideo: strPtr("off")})
	if err != nil || edited.StreamingVideo != "off" {
		t.Fatalf("editing the copy = %+v (%v), want it editable", edited, err)
	}
	if _, err := svc.UpdateDisplayProfile(ctx, first.ID, DisplayProfileRequest{AgentMouse: strPtr("maybe")}); err == nil {
		t.Fatal("an agent-mouse value outside on|off was accepted")
	}
}

func TestVMDisplayProfileDefaultsToFastLANAndReachesTheSpec(t *testing.T) {
	svc, _, db := newIntentTestService(t)
	ctx := context.Background()
	insertIntentTestVM(t, db, "vm1", StateRunning)

	vm, err := svc.Get(ctx, "vm1")
	if err != nil || vm.DisplayProfileID != ProfileFastLAN {
		t.Fatalf("new guest profile = %q (%v), want Fast LAN", vm.DisplayProfileID, err)
	}
	spec := svc.buildSpec(ctx, vm)
	if spec.SpiceOptions == nil || spec.SpiceOptions.ImageCompression != "off" || spec.SpiceOptions.PlaybackCompression != "on" {
		t.Fatalf("default spec options = %+v, want Fast LAN's", spec.SpiceOptions)
	}

	// Allowed on a running guest: it is read at the next start.
	if _, err := svc.Update(ctx, "vm1", UpdateRequest{DisplayProfileID: strPtr(ProfileLowBandwidth)}); err != nil {
		t.Fatalf("choosing a profile on a running guest: %v", err)
	}
	vm, _ = svc.Get(ctx, "vm1")
	if spec := svc.buildSpec(ctx, vm); spec.SpiceOptions == nil || spec.SpiceOptions.StreamingVideo != "all" {
		t.Fatalf("spec after choosing Low bandwidth = %+v", spec.SpiceOptions)
	}

	_, err = svc.Update(ctx, "vm1", UpdateRequest{DisplayProfileID: strPtr("no-such-profile")})
	if code, ok := validationCode(err); !ok || code != codeDisplayProfileUnknown {
		t.Fatalf("unknown profile: err = %v, want %s", err, codeDisplayProfileUnknown)
	}

	if _, err := svc.Update(ctx, "vm1", UpdateRequest{DisplayProfileID: strPtr("")}); err != nil {
		t.Fatal(err)
	}
	if storedProfile(t, db, "vm1").Valid {
		t.Fatal("choosing the default stored an id instead of NULL")
	}
}

// TestDisplayProfileMigrationReachesAnExistingInstall replays a repair: an
// install that predates 0009 gets it from the ledger, and Init's ensure runs
// before or after it without either failing.
func TestDisplayProfileMigrationReachesAnExistingInstall(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kvm.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var profiles string
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(file, "0009_display_profiles.sql") {
			profiles = string(body)
			continue
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	if profiles == "" {
		t.Fatal("0009_display_profiles.sql is missing")
	}
	svc := NewService(sqliteHandle{db}, &fakeGuestRPC{}, nil, nil, nil, nil, nil, t.TempDir(), quietLogger{})
	ctx := context.Background()
	for _, ensure := range []func(context.Context) error{
		svc.ensureScheduleSchema, svc.ensureImportSchema, svc.ensureNICSchema,
		svc.ensureSnapshotTreeSchema, svc.ensureIncidentSchema, svc.ensureGuestHealthSchema,
	} {
		if err := ensure(ctx); err != nil {
			t.Fatal(err)
		}
	}
	insertIntentTestVM(t, db, "old", StateStopped)

	// The new .so's Init can run before the repair applies the file, and the
	// repair must still succeed; then both run again, as a second restart would.
	if err := svc.ensureDisplayProfileSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		if _, err := db.Exec(profiles); err != nil {
			t.Fatalf("0009 on pass %d: %v", pass, err)
		}
		if err := svc.ensureDisplayProfileSchema(ctx); err != nil {
			t.Fatalf("ensure on pass %d: %v", pass, err)
		}
	}

	list, err := svc.ListDisplayProfiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].ID != ProfileFastLAN || list[1].ID != ProfileBalanced || list[2].ID != ProfileLowBandwidth {
		t.Fatalf("profiles after the repair = %+v, want the three built-ins in order", list)
	}
	if list[0].VMCount != 1 {
		t.Fatalf("Fast LAN counts %d guests, want the existing one", list[0].VMCount)
	}
	for _, profile := range list {
		seeded, _ := builtinProfile(profile.ID)
		if profile.ImageCompression != seeded.ImageCompression || profile.StreamingVideo != seeded.StreamingVideo ||
			profile.JPEGWANCompression != seeded.JPEGWANCompression || profile.PlaybackCompression != seeded.PlaybackCompression ||
			profile.ZlibGLZWANCompression != seeded.ZlibGLZWANCompression || profile.AgentMouse != seeded.AgentMouse {
			t.Errorf("%s: migration seed %+v disagrees with the Go built-in %+v", profile.ID, profile, seeded)
		}
	}
	vm, err := svc.Get(ctx, "old")
	if err != nil || vm.DisplayProfileID != ProfileFastLAN {
		t.Fatalf("existing guest = %q (%v), want Fast LAN", vm.DisplayProfileID, err)
	}
}

// TestSpiceOptionsMatchTheAgentProtocol decodes the host's vm slot fixture, which
// sets every protocol field, into the plugin's wire struct: a tag the agent does
// not send decodes to zero here instead of silently dropping an option.
func TestSpiceOptionsMatchTheAgentProtocol(t *testing.T) {
	var frame struct {
		Spec agentSpec `json:"spec"`
	}
	if err := json.Unmarshal(readFixture(t, "slots", "vm.json"), &frame); err != nil {
		t.Fatal(err)
	}
	if frame.Spec.SpiceOptions == nil {
		t.Fatal("agentSpec does not decode the protocol's spiceOptions")
	}
	var zero []string
	zeroFields(reflect.ValueOf(*frame.Spec.SpiceOptions), "agentSpiceOptions", &zero)
	if len(zero) > 0 {
		t.Fatalf("agentSpiceOptions decodes these fields to zero: %v", zero)
	}
}
