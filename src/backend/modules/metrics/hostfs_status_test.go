// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"

	_ "modernc.org/sqlite"
)

type fixedThresholds struct{ warning, critical float64 }

func (f fixedThresholds) HostFSThresholds(context.Context, string) (float64, float64) {
	return f.warning, f.critical
}

// statusFixture isolates the package-level tracker, clock and threshold source.
func statusFixture(t *testing.T) *time.Time {
	t.Helper()
	resetHostFSTracker()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	prevNow := hostFSNow
	hostFSNow = func() time.Time { return now }
	t.Cleanup(func() {
		hostFSNow = prevNow
		SetHostFSThresholds(nil)
		resetHostFSTracker()
	})
	SetHostFSThresholds(nil)
	return &now
}

func agentBackend(used int64) *fakeFSBackend {
	return &fakeFSBackend{kindOf: hostKindAgent,
		stats: coreagent.HostStatsPayload{Supported: true, FSTotal: 100, FSUsed: used}}
}

func mustStatus(t *testing.T, s *Service, env string) HostFSStatus {
	t.Helper()
	st, err := s.HostFSStatus(context.Background(), env)
	if err != nil {
		t.Fatalf("HostFSStatus: %v", err)
	}
	return st
}

func TestHostFSStatusLevelsFollowDefaultThresholds(t *testing.T) {
	statusFixture(t)
	for _, c := range []struct {
		used  int64
		level string
	}{{50, HostFSLevelNormal}, {85, HostFSLevelWarning}, {94, HostFSLevelWarning}, {95, HostFSLevelCritical}, {100, HostFSLevelCritical}} {
		resetHostFSTracker()
		st := mustStatus(t, fsService(agentBackend(c.used)), "env-levels")
		if st.State != HostFSStateOK || st.Level == nil || *st.Level != c.level {
			t.Errorf("used %d: state %q level %v, want ok/%s", c.used, st.State, st.Level, c.level)
		}
		if st.Thresholds.Warning != 85 || st.Thresholds.Critical != 95 {
			t.Errorf("default thresholds = %+v", st.Thresholds)
		}
	}
}

func TestHostFSStatusUsesTheThresholdSource(t *testing.T) {
	statusFixture(t)
	SetHostFSThresholds(fixedThresholds{warning: 70, critical: 80})
	st := mustStatus(t, fsService(agentBackend(75)), "env-thr")
	if st.Level == nil || *st.Level != HostFSLevelWarning || st.Thresholds.Warning != 70 || st.Thresholds.Critical != 80 {
		t.Fatalf("got level %v thresholds %+v", st.Level, st.Thresholds)
	}
	SetHostFSThresholds(fixedThresholds{})
	st = mustStatus(t, fsService(agentBackend(75)), "env-thr")
	if st.Thresholds.Warning != 85 || st.Thresholds.Critical != 95 {
		t.Fatalf("zero source values must fall back to 85/95, got %+v", st.Thresholds)
	}
}

func TestHostFSStatusKeepsLastGoodSampleWhenTheReadFails(t *testing.T) {
	now := statusFixture(t)
	b := agentBackend(97)
	svc := fsService(b)
	first := mustStatus(t, svc, "env-stale")
	seen := *now

	*now = now.Add(2 * time.Minute)
	b.statsErr = ErrHostFSAgentOffline
	st := mustStatus(t, svc, "env-stale")
	if st.State != HostFSStateStale || st.Reason != HostFSReasonAgentOffline {
		t.Fatalf("state %q reason %q, want stale/agent_offline", st.State, st.Reason)
	}
	if st.Level != nil {
		t.Fatalf("a stale reading must carry no level, got %q", *st.Level)
	}
	if st.UsedBytes != first.UsedBytes || st.Percent != 97 {
		t.Fatalf("last good sample lost: %+v", st)
	}
	if st.LastSeen == nil || !st.LastSeen.Equal(seen) {
		t.Fatalf("lastSeen = %v, want %v", st.LastSeen, seen)
	}

	*now = now.Add(2 * time.Minute)
	b.statsErr = nil
	if st = mustStatus(t, svc, "env-stale"); st.State != HostFSStateOK || st.Reason != "" {
		t.Fatalf("recovery: state %q reason %q", st.State, st.Reason)
	}
}

func TestHostFSStatusIsUnknownForAHostNeverSeen(t *testing.T) {
	statusFixture(t)
	b := &fakeFSBackend{kindOf: hostKindAgent, statsErr: ErrHostFSAgentTooOld}
	st := mustStatus(t, fsService(b), "env-new")
	if st.State != HostFSStateUnknown || st.Reason != HostFSReasonAgentTooOld || st.LastSeen != nil || st.Level != nil {
		t.Fatalf("got %+v", st)
	}
}

func TestHostFSStatusIsUnsupportedForARemoteHostWithoutAnAgent(t *testing.T) {
	statusFixture(t)
	b := &fakeFSBackend{kindOf: hostKindOther}
	st := mustStatus(t, fsService(b), "env-tcp")
	if st.State != HostFSStateUnsupported || st.Level != nil || st.LastSeen != nil {
		t.Fatalf("got %+v", st)
	}
}

func TestHostFSStatusRefreshesOnlyAfterSixtySeconds(t *testing.T) {
	now := statusFixture(t)
	b := agentBackend(10)
	svc := fsService(b)
	mustStatus(t, svc, "env-ttl")
	*now = now.Add(59 * time.Second)
	mustStatus(t, svc, "env-ttl")
	if b.agentCalls != 1 {
		t.Fatalf("read %d times inside the 60s window, want 1", b.agentCalls)
	}
	*now = now.Add(2 * time.Second)
	mustStatus(t, svc, "env-ttl")
	if b.agentCalls != 2 {
		t.Fatalf("read %d times after the window, want 2", b.agentCalls)
	}
}

func TestHostFSStatusFallsBackToTheNewestCapacitySampleAfterARestart(t *testing.T) {
	statusFixture(t)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing db: %v", err)
		}
	})
	if _, err := db.Exec(`CREATE TABLE capacity_samples (id TEXT PRIMARY KEY, environment_id TEXT, subject_kind TEXT,
		subject_id TEXT, subject_name TEXT, sampled_at TEXT, used_bytes INTEGER, total_bytes INTEGER, updated_at TEXT);
		INSERT INTO capacity_samples VALUES
		('a','env-db','rootfs','env-db','/','2026-10-21 08:00:00',40,100,'2026-10-21 08:59:00'),
		('b','env-db','rootfs','env-db','/','2026-10-22 10:00:00',92,100,'2026-10-22 10:47:30'),
		('c','env-db','volume','v','v','2026-10-22 11:00:00',1,100,'2026-10-22 11:30:00');`); err != nil {
		t.Fatal(err)
	}
	b := &fakeFSBackend{kindOf: hostKindAgent, statsErr: ErrHostFSAgentOffline}
	svc := &Service{fsb: b, db: db}
	st := mustStatus(t, svc, "env-db")
	want := time.Date(2026, 10, 22, 10, 47, 30, 0, time.UTC)
	if st.State != HostFSStateStale || st.LastSeen == nil || !st.LastSeen.Equal(want) || st.UsedBytes != 92 {
		t.Fatalf("got %+v lastSeen %v", st, st.LastSeen)
	}

	empty := &Service{fsb: b, db: db}
	if st = mustStatus(t, empty, "env-without-rows"); st.State != HostFSStateUnknown {
		t.Fatalf("no rows: state %q, want unknown", st.State)
	}
}

func TestHostFSStatusRejectsAnUnknownEnvironmentBeforeTouchingTheTracker(t *testing.T) {
	statusFixture(t)
	svc := fsService(&fakeFSBackend{kindOf: hostKindAgent, statsErr: ErrHostFSAgentOffline, unknownEnv: true})
	for i := 0; i < 50; i++ {
		_, err := svc.HostFSStatus(context.Background(), fmt.Sprintf("random-%d", i))
		if !errors.Is(err, ErrHostFSEnvNotFound) {
			t.Fatalf("env %d: err = %v, want ErrHostFSEnvNotFound", i, err)
		}
	}
	if n := hostFSTrackedCount(); n != 0 {
		t.Fatalf("tracker holds %d entries after 50 unknown ids, want 0", n)
	}
}

func TestForgetHostFSDropsTheTrackedEnvironment(t *testing.T) {
	statusFixture(t)
	mustStatus(t, fsService(agentBackend(10)), "env-gone")
	if hostFSTrackedCount() == 0 {
		t.Fatal("expected a tracked entry")
	}
	ForgetHostFS("env-gone")
	if n := hostFSTrackedCount(); n != 0 {
		t.Fatalf("tracker holds %d after ForgetHostFS", n)
	}
}
