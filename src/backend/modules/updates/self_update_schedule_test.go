// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

type schedFixture struct {
	c        *SelfUpdateChecker
	checks   atomic.Int32
	installs atomic.Int32
	busy     string
}

func newSchedFixture(t *testing.T, s SelfUpdateSettings) *schedFixture {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	f := &schedFixture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.checks.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `{"tag_name":"v9999.01.01","html_url":"https://example.invalid/r","published_at":"2026-09-26T00:00:00Z","body":"notes"}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewSelfUpdateChecker(database, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.releaseURL = srv.URL
	c.SetApplier(func(context.Context, string) error { f.installs.Add(1); return nil }, func(context.Context) string { return "mcharbor:latest" })
	c.SetBusyChecker(func(context.Context) string { return f.busy })
	if _, err := c.SaveSettings(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	f.c = c
	return f
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 9, 26, hour, minute, 0, 0, time.Local)
}

func TestTheCheckAndTheInstallRunOnlyOnTheirSchedules(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true, AutoInstall: true, CheckCron: "0 4 * * *", InstallCron: "30 4 * * *", InstallWindowMinutes: 60})
	ctx := context.Background()

	f.c.tick(ctx, at(3, 59))
	f.c.tick(ctx, at(3, 59))
	if f.checks.Load() != 0 || f.installs.Load() != 0 {
		t.Fatalf("before 04:00: %d checks, %d installs", f.checks.Load(), f.installs.Load())
	}
	f.c.tick(ctx, at(4, 0))
	if f.checks.Load() != 1 || !f.c.State().UpdateAvailable {
		t.Fatalf("at 04:00 the check must run once and find the release: %d checks", f.checks.Load())
	}
	f.c.tick(ctx, at(4, 15))
	if f.installs.Load() != 0 {
		t.Fatal("an update was installed outside the install window")
	}

	f.busy = BusyBackupRunning
	f.c.tick(ctx, at(4, 30))
	if f.installs.Load() != 0 || f.c.State().InstallDeferred != BusyBackupRunning {
		t.Fatalf("a running backup must defer the install: %d installs, deferred %q", f.installs.Load(), f.c.State().InstallDeferred)
	}
	f.busy = ""
	f.c.tick(ctx, at(4, 45))
	if f.installs.Load() != 1 || f.c.State().InstallDeferred != "" {
		t.Fatalf("once quiet, the install must run once: %d installs, deferred %q", f.installs.Load(), f.c.State().InstallDeferred)
	}
	f.c.tick(ctx, at(4, 46))
	if f.installs.Load() != 1 {
		t.Fatal("the same release was installed twice")
	}
	if f.checks.Load() != 1 {
		t.Fatalf("the check ran %d times; it runs only at 04:00", f.checks.Load())
	}
}

func TestAWindowThatNeverGoesQuietIsSkippedNotOverrun(t *testing.T) {
	f := newSchedFixture(t, SelfUpdateSettings{Enabled: true, AutoInstall: true, CheckCron: "0 4 * * *", InstallCron: "30 4 * * *", InstallWindowMinutes: 60})
	ctx := context.Background()
	f.c.tick(ctx, at(3, 59))
	f.c.tick(ctx, at(4, 0))
	f.busy = BusyExportRunning
	f.c.tick(ctx, at(4, 30))
	f.c.tick(ctx, at(5, 29))
	f.busy = ""
	f.c.tick(ctx, at(5, 31))
	if f.installs.Load() != 0 {
		t.Fatal("an install started after its window closed")
	}
	if st := f.c.State(); !st.InstallSkipped || st.InstallDeferred != BusyExportRunning {
		t.Fatalf("the skipped window must say why: %+v", st)
	}
}

func TestSchedulesAreValidatedAndDefaulted(t *testing.T) {
	if ValidSchedule(SelfUpdateSettings{CheckCron: "every night"}) || ValidSchedule(SelfUpdateSettings{InstallCron: "61 * * * *"}) {
		t.Fatal("an invalid cron was accepted")
	}
	c, i, w := normalizeSchedule(SelfUpdateSettings{InstallWindowMinutes: 100000})
	if c != defaultCheckCron || i != defaultInstallCron || w != maxInstallWindowMin {
		t.Fatalf("defaults: %q %q %d", c, i, w)
	}
}
