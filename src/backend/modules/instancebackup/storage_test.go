// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeUploader records what was sent and what was removed, and can be told to
// refuse, so a failed upload can be asserted as a recorded state rather than a
// lost backup.
type fakeUploader struct {
	mu       sync.Mutex
	uploaded []string
	deleted  []string
	bytes    int64
	failWith error
	// failFor refuses exactly one location, leaving the others working.
	failFor string
}

func (f *fakeUploader) Upload(_ context.Context, locationID, key string, body io.Reader, size int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return "", f.failWith
	}
	if f.failFor != "" && f.failFor == locationID {
		return "", errors.New("the destination refused the upload")
	}
	// Read it, so a test would notice an upload handed an unreadable file.
	n, err := io.Copy(io.Discard, body)
	if err != nil {
		return "", err
	}
	f.bytes = n
	f.uploaded = append(f.uploaded, locationID+"/"+key)
	_ = size
	return key, nil
}

func (f *fakeUploader) Delete(_ context.Context, locationID, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, locationID+"/"+key)
	return nil
}

func (f *fakeUploader) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.uploaded...)
}

func (f *fakeUploader) removed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

// TestManualExportCopiesToItsStorageLocation: a schedule could already copy
// offsite; a manual "back up now" could not, even though the handler was handed
// an uploader. An operator taking a backup before an upgrade is exactly who wants
// it off the box.
func TestManualExportCopiesToItsStorageLocation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	up := &fakeUploader{}
	seedLocation(t, svc, "loc-offsite", "Offsite NAS")

	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase", LocationIDs: []string{"loc-offsite"},
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	if e.Status != StatusCompleted {
		t.Fatalf("the export did not complete: %+v", e)
	}

	// The upload happens after finish(), so the row has to be read again.
	stored := waitForLocation(t, svc, e.ID)
	if stored.LocationStatus != "uploaded" {
		t.Fatalf("locationStatus = %q, want uploaded (error: %q)", stored.LocationStatus, stored.LocationError)
	}
	if stored.LocationID == nil || *stored.LocationID != "loc-offsite" {
		t.Fatalf("locationId = %v", stored.LocationID)
	}
	if stored.LocationKey == "" {
		t.Fatal("no object key was recorded, so the remote copy could never be deleted")
	}
	if sent := up.sent(); len(sent) != 1 {
		t.Fatalf("expected one upload, got %v", sent)
	}
	if up.bytes == 0 {
		t.Fatal("the uploader was handed an empty body")
	}
}

// TestManualExportWithNoLocationUploadsNothing: the default must stay local.
func TestManualExportWithNoLocationUploadsNothing(t *testing.T) {
	svc, _ := newTestService(t)
	up := &fakeUploader{}

	started, err := svc.StartExport(context.Background(), "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase",
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	if e.Status != StatusCompleted {
		t.Fatalf("the export did not complete: %+v", e)
	}
	if sent := up.sent(); len(sent) != 0 {
		t.Fatalf("an export with no location uploaded anyway: %v", sent)
	}
}

// TestAFailedUploadKeepsTheBackup: the offsite copy is an addition, never a
// replacement. An upload failure must leave a completed, restorable local export
// and say so on the row, because an operator who reads "stored here, not offsite"
// can act on it — one who reads "failed" deletes it and loses the backup.
func TestAFailedUploadKeepsTheBackup(t *testing.T) {
	svc, _ := newTestService(t)
	up := &fakeUploader{failWith: errors.New("no space left on device")}
	seedLocation(t, svc, "loc-full", "Offsite NAS")

	started, err := svc.StartExport(context.Background(), "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase", LocationIDs: []string{"loc-full"},
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	if e.Status != StatusCompleted {
		t.Fatalf("a failed upload must not fail the export: status = %q", e.Status)
	}

	stored := waitForLocation(t, svc, e.ID)
	if stored.LocationStatus != "failed" {
		t.Fatalf("locationStatus = %q, want failed", stored.LocationStatus)
	}
	if stored.LocationError == "" {
		t.Fatal("a failed upload recorded no reason, so the operator cannot act on it")
	}
	// The local artifact still has to be there and be restorable.
	if _, _, err := svc.FilePath(context.Background(), e.ID); err != nil {
		t.Fatalf("the local export is gone after a failed upload: %v", err)
	}
}

// TestDeletingAnExportRemovesTheRemoteCopy: deleting the row first would orphan
// the object permanently, because nothing would be left that knows its key.
func TestDeletingAnExportRemovesTheRemoteCopy(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	up := &fakeUploader{}
	seedLocation(t, svc, "loc-offsite", "Offsite NAS")

	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase", LocationIDs: []string{"loc-offsite"},
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	stored := waitForLocation(t, svc, e.ID)

	if _, err := svc.Delete(ctx, e.ID, up); err != nil {
		t.Fatal(err)
	}
	removed := up.removed()
	if len(removed) != 1 {
		t.Fatalf("expected the remote copy to be removed once, got %v", removed)
	}
	if removed[0] != "loc-offsite/"+stored.LocationKey {
		t.Fatalf("removed %q, want %q", removed[0], "loc-offsite/"+stored.LocationKey)
	}
}

// TestDeletingALocalOnlyExportTouchesNoStorage keeps the nil-location path quiet.
func TestDeletingALocalOnlyExportTouchesNoStorage(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	up := &fakeUploader{}
	seedLocation(t, svc, "loc-offsite", "Offsite NAS")

	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase",
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)

	if _, err := svc.Delete(ctx, e.ID, up); err != nil {
		t.Fatal(err)
	}
	if removed := up.removed(); len(removed) != 0 {
		t.Fatalf("a local-only export asked storage to delete something: %v", removed)
	}
}

// waitForLocation waits for the post-finish upload to record its outcome. The
// upload runs after the export's status is already final, so waitFor alone
// returns too early.
func waitForLocation(t *testing.T, svc *Service, id string) *Export {
	t.Helper()
	for i := 0; i < 600; i++ {
		e, err := svc.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if e.LocationStatus != "" {
			return e
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the upload outcome was never recorded")
	return nil
}

// seedLocation inserts a storage location, because the destination tables carry
// a foreign key to it and a schedule or export can only be linked to one that
// exists.
func seedLocation(t *testing.T, svc *Service, id, name string) {
	t.Helper()
	if _, err := svc.db.Exec(`INSERT INTO storage_locations (id, name, location_type, enabled, created_at, updated_at)
		VALUES (?, ?, 'local', 1, ?, ?)`, id, name, now(), now()); err != nil {
		t.Fatalf("seeding storage location %s: %v", id, err)
	}
}

// TestAnExportReachesEveryDestination is the capability itself: more than one
// place, each recorded separately. A single location_id could only ever answer
// "offsite or not"; the reason to hold two is that one may be unreachable.
func TestAnExportReachesEveryDestination(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	up := &fakeUploader{}
	seedLocation(t, svc, "loc-nas", "Onsite NAS")
	seedLocation(t, svc, "loc-s3", "Offsite object store")

	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase", LocationIDs: []string{"loc-nas", "loc-s3"},
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	if e.Status != StatusCompleted {
		t.Fatalf("the export did not complete: %+v", e)
	}
	stored := waitForDestinations(t, svc, e.ID, 2)

	if len(up.sent()) != 2 {
		t.Fatalf("expected two uploads, got %v", up.sent())
	}
	byName := map[string]ExportDestination{}
	for _, d := range stored.Destinations {
		byName[d.LocationName] = d
	}
	for _, name := range []string{"Onsite NAS", "Offsite object store"} {
		d, ok := byName[name]
		if !ok {
			t.Fatalf("no destination row for %q: %+v", name, stored.Destinations)
		}
		if d.Status != DestSuccess {
			t.Errorf("%s: status %q, error %q", name, d.Status, d.Error)
		}
		if d.RemoteKey == "" {
			t.Errorf("%s recorded no object key, so its copy could never be deleted", name)
		}
		// The name is copied onto the row, not joined, so deleting the location
		// later cannot erase where a surviving backup was sent.
		if d.LocationType != "local" {
			t.Errorf("%s: locationType %q", name, d.LocationType)
		}
	}
}

// TestOneUnreachableDestinationDoesNotStopTheOther: holding two is a hedge, and
// a hedge that gives up on the first refusal is not one.
func TestOneUnreachableDestinationDoesNotStopTheOther(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	up := &fakeUploader{failFor: "loc-broken"}
	seedLocation(t, svc, "loc-broken", "Unreachable NAS")
	seedLocation(t, svc, "loc-good", "Offsite object store")

	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase", LocationIDs: []string{"loc-broken", "loc-good"},
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	if e.Status != StatusCompleted {
		t.Fatalf("one bad destination must not fail the export: %+v", e)
	}
	stored := waitForDestinations(t, svc, e.ID, 2)

	var ok, bad int
	for _, d := range stored.Destinations {
		switch d.Status {
		case DestSuccess:
			ok++
		case DestFailure:
			bad++
			if d.Error == "" {
				t.Error("a failed destination recorded no reason")
			}
		}
	}
	if ok != 1 || bad != 1 {
		t.Fatalf("want one success and one failure, got %d/%d: %+v", ok, bad, stored.Destinations)
	}
	// The good copy really was sent, not merely recorded.
	if sent := up.sent(); len(sent) != 1 || sent[0] != "loc-good/mcharbor-instance/"+e.FileName {
		t.Fatalf("the reachable destination did not receive the export: %v", sent)
	}
}

// TestDeletingAnExportRemovesEveryRemoteCopy: one unreachable location must not
// leave the others orphaned.
func TestDeletingAnExportRemovesEveryRemoteCopy(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	up := &fakeUploader{}
	seedLocation(t, svc, "loc-nas", "Onsite NAS")
	seedLocation(t, svc, "loc-s3", "Offsite object store")

	started, err := svc.StartExport(ctx, "admin", "manual", nil, CreateExportInput{
		Passphrase: "long enough passphrase", LocationIDs: []string{"loc-nas", "loc-s3"},
	}, up)
	if err != nil {
		t.Fatal(err)
	}
	e := waitFor(t, svc, started.ID)
	waitForDestinations(t, svc, e.ID, 2)

	if _, err := svc.Delete(ctx, e.ID, up); err != nil {
		t.Fatal(err)
	}
	if removed := up.removed(); len(removed) != 2 {
		t.Fatalf("expected both remote copies removed, got %v", removed)
	}
}

// TestSavingAScheduleReplacesItsDestinations: the editor sends the whole set, so
// a location the operator removed has to disappear rather than linger.
func TestSavingAScheduleReplacesItsDestinations(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	seedLocation(t, svc, "loc-a", "A")
	seedLocation(t, svc, "loc-b", "B")
	seedLocation(t, svc, "loc-c", "C")

	sc, err := svc.CreateSchedule(ctx, "admin", ScheduleInput{
		Name: "nightly", Cron: "0 3 * * *", Enabled: true, Passphrase: "long enough passphrase",
		LocationIDs: []string{"loc-a", "loc-b"}, RetentionCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.LocationIDs) != 2 {
		t.Fatalf("created with %v", sc.LocationIDs)
	}

	updated, err := svc.UpdateSchedule(ctx, sc.ID, ScheduleInput{
		Name: "nightly", Cron: "0 3 * * *", Enabled: true,
		LocationIDs: []string{"loc-c"}, RetentionCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.LocationIDs) != 1 || updated.LocationIDs[0] != "loc-c" {
		t.Fatalf("after update: %v — a removed destination survived", updated.LocationIDs)
	}
}

// waitForDestinations waits until the post-finish uploads have recorded n rows.
func waitForDestinations(t *testing.T, svc *Service, id string, n int) *Export {
	t.Helper()
	for i := 0; i < 600; i++ {
		e, err := svc.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(e.Destinations) >= n {
			return e
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("only some destinations were recorded for %s", id)
	return nil
}
