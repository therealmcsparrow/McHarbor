// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"strings"
	"testing"
	"time"
)

func resTestSnap(t *testing.T, pointInTime bool, sources ...string) *Snapshot {
	t.Helper()
	end := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	p := Period{Start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), End: end, Days: 29}
	if pointInTime {
		p = Period{Start: end, End: end, PointInTime: true}
	}
	snap := &Snapshot{Period: p}
	for _, s := range sources {
		snap.Sources = append(snap.Sources, SourceStatus{Key: s, Label: s, OK: true})
	}
	return snap
}

func resTestCtx(t *testing.T, key string, snap *Snapshot) *buildCtx {
	t.Helper()
	rt, ok := reportTypeByKey(key)
	if !ok {
		t.Fatalf("type %q is not in the catalog", key)
	}
	return &buildCtx{
		rt: rt, snap: snap,
		opts:   map[string]bool{OptIncludeDetail: true},
		levels: map[Level]bool{Strategic: true, Tactical: true, Operational: true},
	}
}

func findingTitled(fs []Finding, sev Severity, fragment string) bool {
	for _, f := range fs {
		if f.Severity == sev && strings.Contains(f.Title, fragment) {
			return true
		}
	}
	return false
}

func kpiValue(ks []KPI, label string) (string, bool) {
	for _, k := range ks {
		if k.Label == label {
			return k.Value, true
		}
	}
	return "", false
}

func assertLevels(t *testing.T, secs []Section, want ...Level) {
	t.Helper()
	have := map[Level]bool{}
	for _, s := range secs {
		if s.hasContent() {
			have[s.Level] = true
		}
	}
	for _, l := range want {
		if !have[l] {
			t.Errorf("no %s section with content", l)
		}
	}
}

func TestResilienceTypesAreRegistered(t *testing.T) {
	keys := []string{TypeIncidentNarrative, TypeDriftEnforcement, TypeInstanceDRReadiness, TypeHAPosture, TypeITSMIntegration}
	table := builders()
	jobs := collectJobs(t.Context(), nil, &Snapshot{}, Period{})
	for _, key := range keys {
		rt, ok := reportTypeByKey(key)
		if !ok {
			t.Errorf("%s: not in catalog()", key)
			continue
		}
		if _, ok := table[key]; !ok {
			t.Errorf("%s: no builder", key)
		}
		if reportTitle(rt) == titleCase(key) || reportSubtitle(rt) == "" {
			t.Errorf("%s: no cover title", key)
		}
		if len(rt.Options) == 0 || len(rt.Levels) == 0 {
			t.Errorf("%s: no options or levels", key)
		}
		for _, src := range rt.Sources {
			if _, ok := jobs[src]; !ok {
				t.Errorf("%s: source %s has no collector", key, src)
			}
		}
	}
	if rt, _ := reportTypeByKey(TypeHAPosture); rt.EnvironmentScoped {
		t.Error("ha-posture reads only platform-wide sources and must not ask for an environment")
	}
	if rt, _ := reportTypeByKey(TypeIncidentNarrative); !rt.EnvironmentScoped {
		t.Error("incident-narrative varies by environment")
	}
}

func TestIncidentNarrative(t *testing.T) {
	snap := resTestSnap(t, false, srcResIncidents, srcResIncidentFindings)
	snap.Resilience.Incidents = []resIncidentRow{
		{ID: "a", SubjectName: "api", Signal: "crash_loop", Status: "resolved",
			FirstSignalAt: "2026-09-02T10:00:00Z", ResolvedAt: "2026-09-02T11:00:00Z",
			NarrativeStatus: "ready", NarratedAt: "2026-09-02T10:30:00Z", Narrative: &resIncidentNarrative{Summary: "Bad image."}},
		{ID: "b", SubjectName: "api", Signal: "oom", Status: "resolved",
			FirstSignalAt: "2026-09-03T10:00:00Z", ResolvedAt: "2026-09-03T13:00:00Z"},
		{ID: "e", SubjectName: "api", Signal: "crash_loop", Status: "resolved",
			FirstSignalAt: "2026-09-10T10:00:00Z", ResolvedAt: "2026-09-10T12:00:00Z"},
		{ID: "c", SubjectName: "db", Signal: "unhealthy", Status: "open", FirstSignalAt: "2026-09-28T00:00:00Z"},
		{ID: "d", SubjectName: "queue", Signal: "alert", Status: "open", FirstSignalAt: "2026-08-20T00:00:00Z"},
	}
	snap.Resilience.IncidentFindings = []resIncidentFindingRow{{ID: "f1", Kind: "host_memory_forecast", Status: "open"}}

	f := analyzeIncidents(snap)
	if len(f.Opened) != 4 || len(f.OpenNow) != 2 || f.CarriedOver != 1 || f.StaleOpen != 2 {
		t.Fatalf("opened=%d open=%d carried=%d stale=%d", len(f.Opened), len(f.OpenNow), f.CarriedOver, f.StaleOpen)
	}
	if math.Abs(f.MTTR-120) > 0.001 || math.Abs(f.MedianTTR-120) > 0.001 || math.Abs(f.LongestTTR-180) > 0.001 {
		t.Fatalf("MTTR=%v median=%v longest=%v, want 120/120/180 minutes", f.MTTR, f.MedianTTR, f.LongestTTR)
	}
	if len(f.Recurring) != 1 || f.Recurring[0].Label != "api" || f.Recurring[0].Count != 3 {
		t.Fatalf("recurring = %+v", f.Recurring)
	}

	b := resTestCtx(t, TypeIncidentNarrative, snap)
	if v, _ := kpiValue(incidentKPIs(b), "Mean time to recover"); v != "2.0 h" {
		t.Errorf("MTTR KPI = %q", v)
	}
	fs := incidentFindings(b)
	if !findingTitled(fs, SevHigh, "open for more than a day") {
		t.Error("missing the stale-open finding")
	}
	if !findingTitled(fs, SevMedium, "three or more incidents") {
		t.Error("missing the recurring-subject finding")
	}
	if !findingTitled(fs, SevHigh, "Host memory") {
		t.Error("missing the memory forecast finding")
	}
	assertLevels(t, buildIncidentNarrative(b), Strategic, Tactical, Operational)
	if !strings.Contains(incidentLead(b), "mean time to recover of 2.0 h") {
		t.Errorf("lead = %q", incidentLead(b))
	}
}

func TestIncidentNarrativeNotAssessed(t *testing.T) {
	snap := resTestSnap(t, false)
	snap.Sources = []SourceStatus{{Key: srcResIncidents, OK: false, Note: "not permitted for this user"}}
	b := resTestCtx(t, TypeIncidentNarrative, snap)
	if v, _ := kpiValue(incidentKPIs(b), "Incidents opened"); v != "—" {
		t.Errorf("an unread source must not report a figure, got %q", v)
	}
	if fs := incidentFindings(b); len(fs) != 0 {
		t.Errorf("findings from an unread source: %+v", fs)
	}
	secs := buildIncidentNarrative(b)
	if len(secs) != 1 || !strings.Contains(secs[0].Blocks[1].Text, "not permitted") {
		t.Errorf("not-assessed section did not name the reason: %+v", secs)
	}
}

func TestDriftEnforcement(t *testing.T) {
	snap := resTestSnap(t, false, srcResDrift, srcResDesired, srcResProposals, srcResReconCfg)
	r := &snap.Resilience
	r.Drift = []resDriftRow{
		{DesiredStateID: "s1", Name: "web", Class: "configuration", Field: "env.DB_PASSWORD", Expected: "hunter2", Actual: "x", FirstSeenAt: "2026-09-10T00:00:00Z"},
		{DesiredStateID: "s1", Name: "web", Class: "state", Field: "status", Expected: "running", Actual: "exited", FirstSeenAt: "2026-09-10T00:00:00Z"},
		{DesiredStateID: "s2", Name: "db", Class: "image", Field: "digest", FirstSeenAt: "2026-09-29T00:00:00Z"},
	}
	r.Desired = []resDesiredRow{{ID: "s1", Mode: "enforce"}, {ID: "s2", Mode: "enforce"}, {ID: "s3", Mode: ""}}
	r.Proposals = []resProposalRow{
		{ID: "p1", Origin: "enforce", Status: "failed", CreatedAt: "2026-09-12T00:00:00Z", Error: "recreate refused"},
		{ID: "p2", Origin: "enforce", Status: "applied", CreatedAt: "2026-09-13T00:00:00Z"},
		{ID: "p3", Origin: "propose", Status: "applied", ResolvedBy: "alice", CreatedAt: "2026-09-14T00:00:00Z"},
		{ID: "p4", Origin: "propose", Status: "pending", CreatedAt: "2026-09-20T00:00:00Z"},
		{ID: "p5", Origin: "propose", Status: "applied", CreatedAt: "2026-08-01T00:00:00Z"},
	}
	r.ReconSetting = resReconcilerSettings{BreakerOpen: true, BreakerReason: "3 failures", ConsecutiveFailures: 3, FailureThreshold: 3}

	f := analyzeDrift(snap)
	if f.StatesDrifting != 2 || f.StaleDrift != 2 || f.ByClass["configuration"] != 1 {
		t.Fatalf("drifting=%d stale=%d classes=%v", f.StatesDrifting, f.StaleDrift, f.ByClass)
	}
	if math.Abs(f.OldestDays-20) > 0.001 {
		t.Fatalf("oldest drift = %v days, want 20", f.OldestDays)
	}
	if len(f.InPeriod) != 4 || f.EnforceActions != 2 || f.EnforceFailed != 1 || f.ApprovedByPerson != 1 || f.PendingStale != 1 {
		t.Fatalf("period=%d enforce=%d failed=%d approved=%d pendingStale=%d",
			len(f.InPeriod), f.EnforceActions, f.EnforceFailed, f.ApprovedByPerson, f.PendingStale)
	}
	if f.ByMode["observe"] != 1 || f.ByMode["enforce"] != 2 {
		t.Fatalf("modes = %v", f.ByMode)
	}

	b := resTestCtx(t, TypeDriftEnforcement, snap)
	fs := driftFindings(b)
	for _, want := range []struct {
		sev  Severity
		frag string
	}{
		{SevHigh, "circuit breaker is open"},
		{SevHigh, "enforcement action failed"},
		{SevMedium, "enforcement is off"},
		{SevMedium, "unresolved for over a week"},
		{SevMedium, "waited over three days"},
	} {
		if !findingTitled(fs, want.sev, want.frag) {
			t.Errorf("missing %s finding %q", want.sev, want.frag)
		}
	}
	secs := buildDriftEnforcement(b)
	assertLevels(t, secs, Strategic, Tactical, Operational)
	for _, s := range secs {
		for _, bl := range s.Blocks {
			for _, row := range bl.Rows {
				for _, c := range row {
					if strings.Contains(c.Text, "hunter2") {
						t.Fatal("an environment value reached the report")
					}
				}
			}
		}
	}
}

func TestInstanceDRReadiness(t *testing.T) {
	snap := resTestSnap(t, true, srcResExports, srcResDRSchedules, srcResRestore, srcResRestoreLedger)
	done3 := "2026-09-27T00:00:00Z"
	done10 := "2026-09-20T00:00:00Z"
	loc := "s3"
	r := &snap.Resilience
	r.Exports = []resExportRow{
		{ID: "x0", Status: "failed", CreatedAt: "2026-09-28T00:00:00Z", Error: "disk full"},
		{ID: "x1", Status: "completed", CreatedAt: "2026-09-26T23:00:00Z", CompletedAt: &done3, VerifyStatus: "passed", SizeBytes: 1 << 20},
		{ID: "x2", Status: "completed", CreatedAt: "2026-09-19T23:00:00Z", CompletedAt: &done10, LocationStatus: "uploaded", SizeBytes: 2 << 20},
	}
	r.DRSchedules = []resDRScheduleRow{{ID: "s1", Name: "nightly", Enabled: true, LastStatus: "failed", LastError: "key rejected", LocationID: &loc}}
	r.RestoreLedger = []resLedgerRow{
		{At: "2026-09-01T00:00:00Z", Phase: "intent", ID: "r1"},
		{At: "2026-09-01T00:05:00Z", Phase: "done", ID: "r1"},
		{At: "2026-09-05T00:00:00Z", Phase: "intent", ID: "r2"},
		{At: "2026-09-05T00:01:00Z", Phase: "failed", ID: "r2", Error: "manifest mismatch"},
	}

	f := analyzeDR(snap)
	if math.Abs(f.LastGoodAge-3) > 0.001 || math.Abs(f.LastVerified-3) > 0.001 || math.Abs(f.LastOffsite-10) > 0.001 {
		t.Fatalf("last good=%v verified=%v offsite=%v, want 3/3/10 days", f.LastGoodAge, f.LastVerified, f.LastOffsite)
	}
	if math.Abs(f.SuccessRate-200.0/3) > 0.01 {
		t.Fatalf("success rate = %v", f.SuccessRate)
	}
	if f.Enabled != 1 || len(f.Failing) != 1 || f.RestoresDone != 1 || f.RestoresFailed != 1 {
		t.Fatalf("enabled=%d failing=%d done=%d failed=%d", f.Enabled, len(f.Failing), f.RestoresDone, f.RestoresFailed)
	}

	b := resTestCtx(t, TypeInstanceDRReadiness, snap)
	if v, _ := kpiValue(drKPIs(b), "Last good backup"); v != "3 days ago" {
		t.Errorf("last good KPI = %q", v)
	}
	fs := drFindings(b)
	if !findingTitled(fs, SevMedium, "Last good instance backup was 3 days ago") {
		t.Errorf("missing the recovery-point finding: %+v", fs)
	}
	if !findingTitled(fs, SevHigh, "backup schedule is failing") {
		t.Error("missing the failing-schedule finding")
	}
	assertLevels(t, buildInstanceDRReadiness(b), Strategic, Tactical, Operational)

	r.Exports = []resExportRow{{ID: "only", Status: "failed", CreatedAt: "2026-09-28T00:00:00Z"}}
	if !findingTitled(drFindings(b), SevCritical, "no completed backup") {
		t.Error("an instance with no completed export must be a critical finding")
	}
}

func TestHAPosture(t *testing.T) {
	snap := resTestSnap(t, true, srcResCluster, srcResReadiness, srcResHAEvents)
	r := &snap.Resilience
	r.Cluster.Mode = "active-passive"
	r.Cluster.Database.Reachable = true
	r.Cluster.Instances = []resClusterInstance{
		{ID: "n1", Hostname: "a", Version: "2026.09.01", HARole: "active", State: "running", Alive: true, Self: true, HeartbeatAt: "2026-09-29T23:59:55Z"},
		{ID: "n2", Hostname: "b", Version: "2026.09.01", HARole: "standby", State: "running", Alive: false, HeartbeatAt: "2026-09-29T20:00:00Z"},
	}
	r.HAEvents = []resHAEventRow{{Action: "cluster.drained", Timestamp: "2026-09-10T00:00:00Z"}}

	f := analyzeHA(snap)
	if f.Alive != 1 || len(f.Dead) != 1 || f.StandbysReady != 0 || !f.StandbyMissing || f.Drains != 1 {
		t.Fatalf("alive=%d dead=%d standbys=%d missing=%v drains=%d", f.Alive, len(f.Dead), f.StandbysReady, f.StandbyMissing, f.Drains)
	}
	b := resTestCtx(t, TypeHAPosture, snap)
	fs := haFindings(b)
	if !findingTitled(fs, SevHigh, "No standby is ready") {
		t.Errorf("missing the standby-missing finding: %+v", fs)
	}
	if !findingTitled(fs, SevMedium, "stopped sending heartbeats") {
		t.Error("missing the silent-node finding")
	}
	if !findingTitled(fs, SevLow, "Failover has not been exercised") {
		t.Error("missing the unexercised-failover finding")
	}
	assertLevels(t, buildHAPosture(b), Strategic, Tactical, Operational)

	r.Cluster.Instances[1].Alive = true
	r.Cluster.Instances[1].HARole = "active"
	r.Cluster.ActiveConflict = []string{"n1", "n2"}
	if !findingTitled(haFindings(b), SevCritical, "Two nodes claim to be active") {
		t.Error("split brain must be critical")
	}

	single := resTestSnap(t, true, srcResCluster)
	single.Resilience.Cluster = resClusterStatus{Mode: "peer", Instances: []resClusterInstance{{ID: "n1", Alive: true, State: "running", HARole: "peer"}}}
	single.Resilience.Cluster.Database.Reachable = true
	if !findingTitled(haFindings(resTestCtx(t, TypeHAPosture, single)), SevMedium, "No standby is ready") {
		t.Error("a single peer node has no standby")
	}
}

func TestITSMIntegration(t *testing.T) {
	snap := resTestSnap(t, false, srcResITSMLinks, srcResITSMChannels)
	r := &snap.Resilience
	r.ITSMLinks = []resITSMLinkRow{
		{ID: "l1", ChannelName: "ServiceNow", LocalState: "resolved", OpenedAt: "2026-09-05T10:00:00Z", ResolvedAt: "2026-09-05T12:00:00Z", LastSyncedAt: "2026-09-05T12:00:00Z"},
		{ID: "l2", ChannelName: "ServiceNow", LocalState: "open", OpenedAt: "2026-09-06T10:00:00Z", Parked: true, LastError: "401 from instance", LastSyncedAt: "2026-09-29T12:00:00Z", RemoteAssignee: "ops"},
		{ID: "l3", ChannelName: "Jira", LocalState: "remote_closed", OpenedAt: "2026-09-07T10:00:00Z"},
		{ID: "l4", ChannelName: "Jira", LocalState: "orphaned", OpenedAt: "2026-09-08T10:00:00Z"},
		{ID: "l5", ChannelName: "Jira", LocalState: "open", OpenedAt: "2026-08-01T10:00:00Z", LastError: "timeout", LastSyncedAt: "2026-09-01T00:00:00Z"},
		{ID: "l6", ChannelName: "Jira", LocalState: "resolved", OpenedAt: "2026-08-01T10:00:00Z", ResolvedAt: "2026-08-02T10:00:00Z"},
	}
	r.ITSMChannels = []resITSMChannelRow{
		{ID: "c1", Name: "ServiceNow", WebhookSupported: true, TokenSet: true},
		{ID: "c2", Name: "Jira", WebhookSupported: true},
	}

	f := analyzeITSM(snap)
	if len(f.InPeriod) != 4 || f.Resolved != 1 || f.OpenNow != 2 || f.RemoteClosed != 1 || f.Orphaned != 1 {
		t.Fatalf("period=%d resolved=%d open=%d remote=%d orphaned=%d", len(f.InPeriod), f.Resolved, f.OpenNow, f.RemoteClosed, f.Orphaned)
	}
	if len(f.Parked) != 1 || len(f.Errored) != 1 || f.StaleSync != 1 || math.Abs(f.MTTResolve-120) > 0.001 {
		t.Fatalf("parked=%d errored=%d stale=%d mttr=%v", len(f.Parked), len(f.Errored), f.StaleSync, f.MTTResolve)
	}
	if len(f.NoInbound) != 1 || f.NoInbound[0].Name != "Jira" {
		t.Fatalf("no-inbound channels = %+v", f.NoInbound)
	}
	b := resTestCtx(t, TypeITSMIntegration, snap)
	if v, _ := kpiValue(itsmKPIs(b), "Sync failures"); v != "2" {
		t.Errorf("sync failures KPI = %q", v)
	}
	fs := itsmFindings(b)
	for _, want := range []struct {
		sev  Severity
		frag string
	}{
		{SevHigh, "deliveries parked"},
		{SevMedium, "failed its last sync"},
		{SevMedium, "orphaned"},
		{SevMedium, "cannot hear back"},
		{SevLow, "closed in the ITSM first"},
	} {
		if !findingTitled(fs, want.sev, want.frag) {
			t.Errorf("missing %s finding %q", want.sev, want.frag)
		}
	}
	assertLevels(t, buildITSMIntegration(b), Tactical, Operational)
}
