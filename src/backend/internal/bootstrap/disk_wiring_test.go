// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/modules/alerts"
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

func TestSweepLocalBackupOrphansRemovesStaleOnly(t *testing.T) {
	data := t.TempDir()
	dir := filepath.Join(data, "backups", "containers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, ".mcharbor-agent-backup-123.tar")
	fresh := filepath.Join(dir, ".mcharbor-agent-backup-456.tar")
	for _, p := range []string{stale, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	SweepLocalBackupOrphans(data, nil)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale orphan under <DATA_DIR>/backups/containers should be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh file must stay")
	}
}

func TestWireMergeInvalidationRegistersAHookThatSurvivesACall(t *testing.T) {
	var hook func(sourceID, targetID string)
	WireMergeInvalidation(func(fn func(sourceID, targetID string)) { hook = fn })
	if hook == nil {
		t.Fatal("no merge hook was registered")
	}
	hook("src", "tgt")
}
