// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	containerbackups "github.com/therealmcsparrow/mcharbor/modules/container_backups"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
)

type fakeHostFS struct {
	usage  metrics.HostFSUsage
	status metrics.HostFSStatus
	err    error
}

func (f fakeHostFS) HostFS(context.Context, string) (metrics.HostFSUsage, error) {
	return f.usage, f.err
}

func (f fakeHostFS) HostFSStatus(context.Context, string) (metrics.HostFSStatus, error) {
	return f.status, f.err
}

func TestAlertHostFSMapsUsageAndKeepsErrors(t *testing.T) {
	a := alertHostFS{src: fakeHostFS{usage: metrics.HostFSUsage{Path: "/", Total: 100, Used: 97}}}
	got, err := a.HostFS(context.Background(), "e1")
	if err != nil || got.Path != "/" || got.Total != 100 || got.Used != 97 {
		t.Fatalf("got %+v, %v", got, err)
	}
	boom := errors.New("agent offline")
	if _, err := (alertHostFS{src: fakeHostFS{err: boom}}).HostFS(context.Background(), "e1"); !errors.Is(err, boom) {
		t.Fatalf("error must pass through, got %v", err)
	}
}

func TestAlertHostFSHealthMapsStateReasonAndLastSeen(t *testing.T) {
	seen := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	a := alertHostFSHealth{src: fakeHostFS{status: metrics.HostFSStatus{
		State: metrics.HostFSStateStale, Reason: metrics.HostFSReasonAgentOffline, LastSeen: &seen,
	}}}
	got, err := a.HostFSHealth(context.Background(), "e1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != alerts.HostFSStateStale || got.Reason != "agent_offline" || !got.LastSeen.Equal(seen) {
		t.Fatalf("got %+v", got)
	}
}

func TestAlertHostFSHealthNeverSeenHasZeroLastSeen(t *testing.T) {
	a := alertHostFSHealth{src: fakeHostFS{status: metrics.HostFSStatus{State: metrics.HostFSStateUnknown}}}
	got, err := a.HostFSHealth(context.Background(), "e1")
	if err != nil || got.State != alerts.HostFSStateUnknown || !got.LastSeen.IsZero() {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestHostFSStateConstantsAgree(t *testing.T) {
	pairs := [][2]string{
		{metrics.HostFSStateOK, alerts.HostFSStateOK},
		{metrics.HostFSStateStale, alerts.HostFSStateStale},
		{metrics.HostFSStateUnknown, alerts.HostFSStateUnknown},
		{metrics.HostFSStateUnsupported, alerts.HostFSStateUnsupported},
	}
	for _, p := range pairs {
		if p[0] != p[1] {
			t.Fatalf("metrics %q and alerts %q drifted", p[0], p[1])
		}
	}
}

type fakeThresholds struct {
	w, c float64
	err  error
}

func (f fakeThresholds) HostFSThresholds(context.Context, string) (float64, float64, error) {
	return f.w, f.c, f.err
}

func TestThresholdAdapterPassesValuesAndZeroesOnError(t *testing.T) {
	w, c := (hostFSThresholds{src: fakeThresholds{w: 70, c: 90}}).HostFSThresholds(context.Background(), "e1")
	if w != 70 || c != 90 {
		t.Fatalf("got %v/%v", w, c)
	}
	w, c = (hostFSThresholds{src: fakeThresholds{w: 70, c: 90, err: errors.New("db")}}).HostFSThresholds(context.Background(), "e1")
	if w != 0 || c != 0 {
		t.Fatalf("a failed lookup must fall back to metrics defaults (0/0), got %v/%v", w, c)
	}
}

// syncBuffer lets the sweeper goroutine write its log line while the test polls it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// StartBackupTempSweeper(ctx, db, dataDir, logger) replaces SweepLocalBackupOrphans: it sweeps once at start
// (first tick immediately), then hourly until ctx ends, and logs "backup temp sweep".
func TestStartBackupTempSweeperSweepsAtStartAndLogs(t *testing.T) {
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sweep.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	data := t.TempDir()
	spool := filepath.Join(t.TempDir(), "spool")
	t.Setenv("MCHARBOR_BACKUP_TEMP_DIR", spool)
	dir := containerbackups.LocalBackupDir(data)
	staleRun := filepath.Join(spool, "mcharbor-run-stale")
	freshRun := filepath.Join(spool, "mcharbor-run-fresh")
	foreign := filepath.Join(spool, "their-data")
	for _, d := range []string{staleRun, freshRun, foreign, dir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	stalePartial := filepath.Join(dir, ".mcharbor-agent-backup-123.tar")
	freshPartial := filepath.Join(dir, ".mcharbor-backup-456.tar")
	for _, p := range []string{stalePartial, freshPartial} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-90 * time.Minute)
	for _, p := range []string{staleRun, stalePartial, foreign} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	logs := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	StartBackupTempSweeper(ctx, database, data, slog.New(slog.NewTextHandler(logs, nil)))

	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(logs.String(), "backup temp sweep") {
		if time.Now().After(deadline) {
			t.Fatalf("no first sweep at start; log: %q", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	line := logs.String()
	for _, want := range []string{"removed=2", "bytes=1", "kept_owned=0", "older_than_2h=0"} {
		if !strings.Contains(line, want) {
			t.Errorf("sweep log line lacks %s: %q", want, line)
		}
	}
	for _, p := range []string{staleRun, stalePartial} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("stale %s should be removed by the first sweep", p)
		}
	}
	for _, p := range []string{freshRun, freshPartial, foreign} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s must stay: %v", p, err)
		}
	}
	cancel()
}

func TestWireMergeInvalidationRegistersAHookThatSurvivesACall(t *testing.T) {
	var hook func(sourceID, targetID string)
	WireMergeInvalidation(func(fn func(sourceID, targetID string)) { hook = fn })
	if hook == nil {
		t.Fatal("no merge hook was registered")
	}
	hook("src", "tgt")
}
