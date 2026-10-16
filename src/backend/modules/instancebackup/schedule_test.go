// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

type fakeStore struct {
	mu      sync.Mutex
	objects map[string]int
	deleted []string
	fail    error
}

func (f *fakeStore) Upload(_ context.Context, loc, key string, body io.Reader, _ int64) (string, error) {
	if f.fail != nil {
		return "", f.fail
	}
	n, err := io.Copy(io.Discard, body)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[loc+"/"+key] = int(n)
	return loc + "/" + key, nil
}

func (f *fakeStore) Delete(_ context.Context, loc, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, loc+"/"+key)
	delete(f.objects, loc+"/"+key)
	return nil
}

func TestScheduleInputIsCheckedBeforeItIsStored(t *testing.T) {
	good := ScheduleInput{Name: "nightly", Cron: "0 3 * * *", Passphrase: "long enough passphrase", RetentionCount: 7}
	for name, c := range map[string]struct {
		in   ScheduleInput
		want string
	}{
		"good":         {good, ""},
		"bad cron":     {ScheduleInput{Name: "n", Cron: "every night", Passphrase: good.Passphrase, RetentionCount: 1}, "bad_cron"},
		"no retention": {ScheduleInput{Name: "n", Cron: good.Cron, Passphrase: good.Passphrase}, "no_retention"},
		"short key":    {ScheduleInput{Name: "n", Cron: good.Cron, Passphrase: "short", RetentionCount: 1}, "passphrase_too_short"},
		"no key":       {ScheduleInput{Name: "n", Cron: good.Cron, RetentionCount: 1}, "key_required"},
	} {
		if got := c.in.Check(true); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	if got := (ScheduleInput{Name: "n", Cron: good.Cron, RetentionCount: 1}).Check(false); got != "" {
		t.Errorf("an update that keeps the stored key: %q", got)
	}
}

func TestScheduledExportsUploadVerifyAndKeepOnlyTheirRetention(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	// A destination is only linked when the location exists: the join table has a
	// foreign key, and a schedule pointing at a location nobody created would fail
	// on insert rather than silently keep a dead reference.
	seedLocation(t, svc, "loc1", "Offsite NAS")
	sc, err := svc.CreateSchedule(ctx, "admin", ScheduleInput{
		Name: "nightly", Cron: "0 3 * * *", Enabled: true, Passphrase: "long enough passphrase",
		LocationIDs: []string{"loc1"}, RetentionCount: 2, VerifyAfter: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := svc.db.QueryRow(`SELECT key_secret_enc FROM instance_backup_schedules WHERE id = ?`, sc.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "long enough passphrase") {
		t.Fatal("the schedule key is stored in plaintext")
	}
	raw, err := json.Marshal(sc)
	if err != nil || strings.Contains(string(raw), "long enough") || strings.Contains(string(raw), "key_secret") {
		t.Fatalf("the schedule key leaks through the API: %s", raw)
	}

	store := &fakeStore{objects: map[string]int{}}
	for i := 0; i < 3; i++ {
		svc.RunSchedule(ctx, *sc, store)
	}
	items, total, err := svc.List(ctx, 1, 25)
	if err != nil || total != 2 {
		t.Fatalf("retention kept %d exports, want 2: %v", total, err)
	}
	for _, e := range items {
		if e.Trigger != "schedule" || e.VerifyStatus != VerifyPassed || e.LocationStatus != "uploaded" {
			t.Fatalf("a scheduled export: %+v", e)
		}
	}
	if len(store.objects) != 2 || len(store.deleted) != 1 {
		t.Fatalf("storage holds %d, deleted %d; want 2 and 1", len(store.objects), len(store.deleted))
	}
	after, err := svc.Schedule(ctx, sc.ID)
	if err != nil || after.LastStatus != StatusCompleted || after.NextRunAt == nil || after.LastRunAt == nil {
		t.Fatalf("schedule after runs: %+v %v", after, err)
	}

	store.fail = errors.New("sftp: permission denied")
	svc.RunSchedule(ctx, *sc, store)
	after, err = svc.Schedule(ctx, sc.ID)
	if err != nil || after.LastStatus != StatusFailed || !strings.Contains(after.LastError, "permission denied") {
		t.Fatalf("a failed upload must be recorded on the schedule: %+v %v", after, err)
	}
}
