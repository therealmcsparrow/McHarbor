// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var govTestEnd = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// Sentinels planted in fields the report must never carry. Any of them in a
// rendered document means a value, credential or payload leaked.
var govLeakSentinels = []string{
	"SENTINEL-SECRET-VALUE", "SENTINEL-DESCRIPTION", "SENTINEL-HMAC", "hunter2",
	"SENTINELPATH", "token=abc", "SENTINEL-PAYLOAD", "SENTINEL-RESPONSE",
	"SENTINEL-BODY", "SENTINEL-CANVAS", "SENTINEL-RUN-ERROR", "SENTINEL-KEY-ID",
}

func govTS(daysAgo float64, plusMin float64) string {
	d := time.Duration(daysAgo*24*float64(time.Hour)) - time.Duration(plusMin*float64(time.Minute))
	return govTestEnd.Add(-d).Format(time.RFC3339)
}

func govDecode(t *testing.T, raw string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

// govFixture builds a snapshot from host-shaped JSON, so fields the row types
// deliberately drop are dropped exactly as they would be in production.
func govFixture(t *testing.T, pointInTime bool) *Snapshot {
	t.Helper()
	p := Period{Start: govTestEnd.AddDate(0, 0, -30), End: govTestEnd, Days: 30}
	if pointInTime {
		p = Period{Start: govTestEnd, End: govTestEnd, PointInTime: true}
	}
	snap := &Snapshot{Period: p}
	g := &snap.Governance

	govDecode(t, `[
	 {"id":"a1","category":"container.delete","requesterId":"u1","requesterName":"alice","status":"executed",
	  "requiredApprovers":1,"approvalsCount":1,"bodyRedacted":"SENTINEL-BODY","createdAt":"`+govTS(2, 0)+`",
	  "decidedAt":"`+govTS(2, 30)+`","executedAt":"`+govTS(2, 31)+`","resultStatus":200,
	  "decisions":[{"approverId":"u2","approverName":"bob","decision":"approve","note":"ok"}]},
	 {"id":"a2","category":"container.delete","requesterId":"u1","requesterName":"alice","status":"denied",
	  "createdAt":"`+govTS(3, 0)+`","decidedAt":"`+govTS(3, 90)+`"},
	 {"id":"a3","category":"stack.remove","requesterId":"u3","requesterName":"carol","status":"expired",
	  "createdAt":"`+govTS(4, 0)+`"},
	 {"id":"a4","category":"stack.remove","requesterId":"u3","requesterName":"carol","status":"expired",
	  "createdAt":"`+govTS(5, 0)+`","decidedAt":"`+govTS(5, 20)+`"},
	 {"id":"a5","category":"container.delete","requesterId":"u4","requesterName":"dave","status":"approved",
	  "createdAt":"`+govTS(6, 0)+`","decidedAt":"`+govTS(6, 10)+`",
	  "decisions":[{"approverId":"u4","approverName":"dave","decision":"approve"}]},
	 {"id":"a6","category":"container.delete","requesterId":"u1","status":"denied",
	  "createdAt":"`+govTS(60, 0)+`","decidedAt":"`+govTS(60, 5)+`"}
	]`, &g.Approvals)
	govDecode(t, `[{"id":"r1","category":"container.delete","enabled":true,"requiredApprovers":1,
	  "approverPermission":"approvals.decide","expiryMinutes":1440}]`, &g.ApprovalRules)
	govDecode(t, `[{"key":"container.delete","routes":[]},{"key":"stack.remove","routes":[]}]`, &g.ApprovalCategories)
	govDecode(t, `{"requests":[
	 {"id":"e1","requesterId":"u1","permissionKey":"secrets.reveal","justification":"x","requestedMinutes":600,
	  "status":"approved","approverId":"u2","approvedUntil":"`+govTS(-1, 0)+`","decidedAt":"`+govTS(1, 0)+`","createdAt":"`+govTS(1, 0)+`"},
	 {"id":"e2","requesterId":"u5","permissionKey":"users.manage","justification":"Rotate the break-glass admin after incident 42",
	  "requestedMinutes":60,"status":"expired","approverId":"u5","approvedUntil":"`+govTS(7, 60)+`","createdAt":"`+govTS(7, 0)+`"},
	 {"id":"e3","requesterId":"u6","permissionKey":"users.manage","justification":"please","requestedMinutes":30,
	  "status":"denied","approverId":"u2","createdAt":"`+govTS(8, 0)+`"}
	]}`, &struct {
		Requests *[]govElevationRow `json:"requests"`
	}{&g.Elevations})

	govDecode(t, `[
	 {"id":"s1","name":"legacy-db-password","kind":"password","scope":"global","version":1,
	  "value":"SENTINEL-SECRET-VALUE","description":"SENTINEL-DESCRIPTION",
	  "createdAt":"`+govTS(400, 0)+`","updatedAt":"`+govTS(400, 0)+`","referenceCount":0,"staleCount":0},
	 {"id":"s2","name":"registry-token","kind":"token","scope":"global","version":3,
	  "createdAt":"`+govTS(300, 0)+`","updatedAt":"`+govTS(10, 0)+`","referenceCount":12,"staleCount":2,
	  "lastRevealedAt":"`+govTS(3, 0)+`"},
	 {"id":"s3","name":"mcharbor-internal","kind":"token","scope":"application","version":1,"system":true,
	  "createdAt":"`+govTS(20, 0)+`","updatedAt":"`+govTS(20, 0)+`","referenceCount":0}
	]`, &g.Secrets)
	g.SecretVersions = map[string][]govSecretVersionRow{}
	var v1, v2, v3 []govSecretVersionRow
	govDecode(t, `[]`, &v1)
	govDecode(t, `[{"version":1,"createdAt":"`+govTS(100, 0)+`","value":"hunter2"},{"version":2,"createdAt":"`+govTS(10, 0)+`"}]`, &v2)
	govDecode(t, `[]`, &v3)
	g.SecretVersions["s1"], g.SecretVersions["s2"], g.SecretVersions["s3"] = v1, v2, v3
	var imp govSecretImpact
	govDecode(t, `{"resources":[
	 {"resourceType":"stack","resourceName":"web","environmentId":"prod","stale":true},
	 {"resourceType":"stack","resourceName":"api","environmentId":"staging"},
	 {"resourceType":"container","resourceName":"ci","environmentId":"dev"},
	 {"resourceType":"container","resourceName":"gone","environmentId":"dev","missing":true}],
	 "staleCount":2,"missingCount":1}`, &imp)
	g.SecretImpact = map[string]govSecretImpact{"s2": imp}

	govDecode(t, `[
	 {"id":"w1","name":"Nightly prune","status":"active","canvasData":"SENTINEL-CANVAS"},
	 {"id":"w2","name":"Idle watcher","status":"active"},
	 {"id":"w3","name":"Deploy notifier","status":"draft"}
	]`, &g.Workflows)
	govDecode(t, `[
	 {"id":"r1","workflowId":"w1","status":"failed","trigger":"auto","durationMs":1000,"startedAt":"`+govTS(1, 0)+`","error":"SENTINEL-RUN-ERROR"},
	 {"id":"r2","workflowId":"w1","status":"failed","trigger":"auto","durationMs":1000,"startedAt":"`+govTS(2, 0)+`"},
	 {"id":"r3","workflowId":"w1","status":"failed","trigger":"auto","durationMs":1000,"startedAt":"`+govTS(3, 0)+`"},
	 {"id":"r4","workflowId":"w1","status":"completed","trigger":"auto","durationMs":2000,"startedAt":"`+govTS(4, 0)+`"},
	 {"id":"r5","workflowId":"w1","status":"completed","trigger":"auto","durationMs":2000,"startedAt":"`+govTS(5, 0)+`"},
	 {"id":"r6","workflowId":"w3","status":"completed","trigger":"manual","durationMs":500,"startedAt":"`+govTS(6, 0)+`"},
	 {"id":"r7","workflowId":"w3","status":"completed","trigger":"webhook","durationMs":500,"startedAt":"`+govTS(7, 0)+`"},
	 {"id":"r8","workflowId":"w3","status":"failed","trigger":"manual","durationMs":500,"startedAt":"`+govTS(90, 0)+`"}
	]`, &g.WorkflowRuns)

	var wire []govWebhookWire
	govDecode(t, `[
	 {"id":"h1","name":"Chat alerts","url":"https://user:hunter2@hooks.example.com/services/T0/SENTINELPATH?token=abc",
	  "secret":"SENTINEL-HMAC","events":"[\"container.die\",\"scan.critical\"]","isActive":true,"createdAt":"`+govTS(100, 0)+`"},
	 {"id":"h2","name":"Legacy receiver","url":"http://plain.example.org/hook","events":"[]","isActive":true,"createdAt":"`+govTS(50, 0)+`"},
	 {"id":"h3","name":"Old pager","url":"https://pager.example.net/x","events":"[]","isActive":false}
	]`, &wire)
	g.Webhooks = govReduceWebhooks(wire)
	g.Deliveries = map[string][]govDeliveryRow{}
	var d1, d2, d3 []govDeliveryRow
	govDecode(t, `[
	 {"event":"container.die","responseStatus":500,"success":false,"duration":300,"createdAt":"`+govTS(1, 10)+`","payload":"SENTINEL-PAYLOAD","responseBody":"SENTINEL-RESPONSE"},
	 {"event":"container.die","responseStatus":500,"success":false,"duration":300,"createdAt":"`+govTS(1, 5)+`"},
	 {"event":"container.die","responseStatus":0,"success":false,"duration":0,"createdAt":"`+govTS(1, 0)+`"},
	 {"event":"scan.critical","responseStatus":200,"success":true,"duration":100,"createdAt":"`+govTS(2, 0)+`"}
	]`, &d1)
	govDecode(t, `[
	 {"event":"container.start","responseStatus":204,"success":true,"duration":50,"createdAt":"`+govTS(1, 0)+`"},
	 {"event":"container.start","responseStatus":204,"success":true,"duration":50,"createdAt":"`+govTS(2, 0)+`"}
	]`, &d2)
	govDecode(t, `[]`, &d3)
	g.Deliveries["h1"], g.Deliveries["h2"], g.Deliveries["h3"] = d1, d2, d3

	govDecode(t, `{"generatedAt":"`+govTS(0, 0)+`","version":"2026.09.01","redacted":true,
	 "summary":{"pass":1,"warn":1,"fail":1,"info":1,"unknown":1},
	 "sections":[
	  {"id":"audit","status":"pass","facts":[{"key":"retentionDays","kind":"days","value":"365"}],"reasons":[]},
	  {"id":"keys","status":"info","facts":[{"key":"backupKeyId","kind":"code","value":"SENTINEL-KEY-ID","sensitive":true}],"reasons":[]},
	  {"id":"tls","status":"fail","facts":[{"key":"daysRemaining","kind":"days","value":"0"}],"reasons":["certExpired"]},
	  {"id":"mfa","status":"warn","facts":[{"key":"mfaAdoption","kind":"percent","value":"50"}],"reasons":["mfaPartial"]},
	  {"id":"plugins","status":"unknown","facts":[],"reasons":["sourceUnavailable"]}
	 ]}`, &g.Compliance)

	for _, k := range []string{srcApprovals, srcApprovalRules, srcApprovalCategories, srcElevations,
		srcSecrets, srcSecretVersions, srcSecretImpact, srcWorkflows, srcWorkflowRuns,
		srcWebhooks, srcWebhookDeliveries, srcCompliance} {
		snap.Sources = append(snap.Sources, SourceStatus{Key: k, Label: k, OK: true})
	}
	return snap
}

func govBuildCtx(t *testing.T, key string, snap *Snapshot) *buildCtx {
	t.Helper()
	rt, ok := reportTypeByKey(key)
	if !ok {
		t.Fatalf("%s is not in the catalog", key)
	}
	facts := analyze(snap)
	levels := map[Level]bool{Strategic: true, Tactical: true, Operational: true}
	return &buildCtx{
		rt: rt, meta: Meta{Environment: "Test"}, snap: snap, facts: facts,
		score:  buildScorecard(facts, snap),
		opts:   map[string]bool{OptIncludeDetail: true, OptIncludeGlossary: true, OptIncludeNarrative: true},
		levels: levels, nar: newNarrator(context.Background(), nil),
	}
}

func govFindingTitles(fs []Finding) string {
	var sb strings.Builder
	for _, f := range fs {
		sb.WriteString(string(f.Severity) + ": " + f.Title + "\n")
	}
	return sb.String()
}

func govHasFinding(t *testing.T, fs []Finding, sev Severity, contains string) {
	t.Helper()
	for _, f := range fs {
		if f.Severity == sev && strings.Contains(f.Title, contains) {
			if f.Recommendation == "" || f.Owner == "" || f.Detail == "" {
				t.Errorf("finding %q lacks detail, recommendation or owner", f.Title)
			}
			return
		}
	}
	t.Errorf("no %s finding containing %q in:\n%s", sev, contains, govFindingTitles(fs))
}

func govNear(a, b float64) bool { return math.Abs(a-b) < 0.05 }

func TestGovernanceCatalogEntries(t *testing.T) {
	table := builders()
	want := map[string]bool{
		TypeChangeApprovals: false, TypeSecretsHygiene: true, TypeWorkflowAutomation: false,
		TypeWebhookDelivery: false, TypeCompliancePosture: false,
	}
	for key, envScoped := range want {
		rt, ok := reportTypeByKey(key)
		if !ok {
			t.Errorf("%s missing from catalog()", key)
			continue
		}
		if _, ok := table[key]; !ok {
			t.Errorf("%s has no builder", key)
		}
		if reportTitle(rt) == titleCase(key) || reportSubtitle(rt) == "" {
			t.Errorf("%s has no family title", key)
		}
		if rt.EnvironmentScoped != envScoped {
			t.Errorf("%s EnvironmentScoped = %v, want %v", key, rt.EnvironmentScoped, envScoped)
		}
		if len(rt.Levels) == 0 || rt.Icon == "" || len(rt.Options) == 0 || plannedSources(rt) == 0 {
			t.Errorf("%s catalog entry incomplete: %+v", key, rt)
		}
	}
}

func TestChangeApprovalsFigures(t *testing.T) {
	f := govApprovals(govFixture(t, false))
	if len(f.Requests) != 5 {
		t.Fatalf("requests in period = %d, want 5 (a6 is outside it)", len(f.Requests))
	}
	if f.Approved != 2 || f.Denied != 1 || f.ExpiredUndecided != 1 || f.ExpiredUnexecuted != 1 {
		t.Errorf("outcomes approved=%d denied=%d lapsedUndecided=%d lapsedUnexecuted=%d",
			f.Approved, f.Denied, f.ExpiredUndecided, f.ExpiredUnexecuted)
	}
	if len(f.Turnaround) != 3 || !govNear(f.MedianTurnaround, 30) {
		t.Errorf("turnaround %v median %.2f, want [30 90 10] median 30", f.Turnaround, f.MedianTurnaround)
	}
	if !govNear(f.ApprovalRate, 66.67) || !govNear(f.LapseRate, 25) {
		t.Errorf("approval rate %.2f lapse rate %.2f", f.ApprovalRate, f.LapseRate)
	}
	if len(f.SelfApproved) != 1 || f.SelfApproved[0].ID != "a5" {
		t.Errorf("self-approved = %+v", f.SelfApproved)
	}
	if len(f.Uncovered) != 1 || f.Uncovered[0] != "stack.remove" || f.SingleApprover != 1 {
		t.Errorf("uncovered %v single %d", f.Uncovered, f.SingleApprover)
	}
	if f.ElevGranted != 2 || f.ElevDenied != 1 || f.ElevGrantedMinutes != 660 {
		t.Errorf("elevations granted=%d denied=%d minutes=%d", f.ElevGranted, f.ElevDenied, f.ElevGrantedMinutes)
	}
	if len(f.ElevLong) != 1 || len(f.ElevNoJustification) != 1 || len(f.ElevSelfApproved) != 1 || len(f.ElevActive) != 1 {
		t.Errorf("long=%d noJust=%d self=%d active=%d", len(f.ElevLong), len(f.ElevNoJustification),
			len(f.ElevSelfApproved), len(f.ElevActive))
	}
	fs := approvalFindings(f)
	govHasFinding(t, fs, SevCritical, "own requester")
	govHasFinding(t, fs, SevMedium, "lapsed without a decision")
	govHasFinding(t, fs, SevMedium, "more than eight hours")
	govHasFinding(t, fs, SevLow, "no approval rule")
}

func TestSecretsHygieneFigures(t *testing.T) {
	f := govSecrets(govFixture(t, true))
	if f.Total != 3 || f.System != 1 {
		t.Fatalf("total %d system %d", f.Total, f.System)
	}
	if len(f.NeverRotated) != 1 || f.NeverRotated[0].Row.ID != "s1" || f.neverRotatedOverdue() != 1 {
		t.Errorf("never rotated = %+v", f.NeverRotated)
	}
	if len(f.Unreferenced) != 1 || f.Unreferenced[0].Row.ID != "s1" {
		t.Errorf("unreferenced should be s1 only (s3 is a system secret): %+v", f.Unreferenced)
	}
	var s2 secretState
	for _, s := range f.States {
		if s.Row.ID == "s2" {
			s2 = s
		}
	}
	if !s2.Rotated || !govNear(s2.AgeDays, 10) || s2.Envs != 3 || s2.Missing != 1 {
		t.Errorf("s2 rotated=%v age=%.2f envs=%d missing=%d", s2.Rotated, s2.AgeDays, s2.Envs, s2.Missing)
	}
	if len(f.Stale) != 1 || len(f.WideReach) != 1 || f.DanglingRefs != 1 || len(f.RevealedRecently) != 1 {
		t.Errorf("stale=%d wide=%d dangling=%d revealed=%d", len(f.Stale), len(f.WideReach), f.DanglingRefs, len(f.RevealedRecently))
	}
	if f.AgeBuckets != [5]int{2, 0, 0, 1, 0} {
		t.Errorf("age buckets %v", f.AgeBuckets)
	}
	fs := secretFindings(f)
	govHasFinding(t, fs, SevHigh, "never been rotated")
	govHasFinding(t, fs, SevLow, "referenced by nothing")
	govHasFinding(t, fs, SevMedium, "still run the old value")
	govHasFinding(t, fs, SevMedium, "blast radius")
}

func TestSecretsHygieneWithoutHistory(t *testing.T) {
	snap := govFixture(t, true)
	snap.Governance.SecretVersions = nil
	for i := range snap.Sources {
		if snap.Sources[i].Key == srcSecretVersions {
			snap.Sources[i].OK = false
		}
	}
	f := govSecrets(snap)
	if f.RotationUnknown != 1 || len(f.NeverRotated) != 1 {
		t.Errorf("without history: unknown=%d neverRotated=%d, want 1 and 1", f.RotationUnknown, len(f.NeverRotated))
	}
}

func TestWorkflowAutomationFigures(t *testing.T) {
	f := govWorkflowStats(govFixture(t, false))
	if len(f.Runs) != 7 || f.Completed != 4 || f.Failed != 3 {
		t.Fatalf("runs %d completed %d failed %d", len(f.Runs), f.Completed, f.Failed)
	}
	if !govNear(f.SuccessRate, 57.14) {
		t.Errorf("success rate %.2f, want 57.14", f.SuccessRate)
	}
	if len(f.Failing) != 1 || f.Failing[0].ID != "w1" {
		t.Errorf("failing = %+v", f.Failing)
	}
	if len(f.IdleActive) != 1 || f.IdleActive[0].ID != "w2" {
		t.Errorf("idle = %+v", f.IdleActive)
	}
	if f.ByTrigger["auto"] != 5 || f.ByTrigger["manual"] != 1 || f.ByTrigger["webhook"] != 1 {
		t.Errorf("by trigger %v", f.ByTrigger)
	}
	fs := workflowFindings(f)
	govHasFinding(t, fs, SevHigh, "half of")
	govHasFinding(t, fs, SevLow, "did not run")
}

func TestWebhookDeliveryFigures(t *testing.T) {
	f := govWebhookStats(govFixture(t, false))
	if f.Attempts != 6 || f.Failed != 3 || !govNear(f.FailureRate, 50) {
		t.Errorf("attempts %d failed %d rate %.2f", f.Attempts, f.Failed, f.FailureRate)
	}
	if f.Retries != 2 {
		t.Errorf("retries %d, want 2", f.Retries)
	}
	if len(f.Failing) != 1 || f.Failing[0].Hook.Host != "hooks.example.com" || f.Failing[0].Streak != 3 {
		t.Errorf("failing = %+v", f.Failing)
	}
	if len(f.PlainHTTP) != 1 || f.PlainHTTP[0].Hook.Host != "plain.example.org" {
		t.Errorf("plain http = %+v", f.PlainHTTP)
	}
	if f.CodeClasses["5xx"] != 2 || f.CodeClasses["No response"] != 1 || f.CodeClasses["2xx"] != 3 {
		t.Errorf("classes %v", f.CodeClasses)
	}
	fs := webhookFindings(f)
	govHasFinding(t, fs, SevHigh, "failing every delivery")
	govHasFinding(t, fs, SevMedium, "plain HTTP")
}

func TestCompliancePostureFigures(t *testing.T) {
	f := govCompliance(govFixture(t, true))
	if f.Pass != 1 || f.Fail != 1 || f.Warn != 1 || f.Unknown != 1 || !govNear(f.PassRate, 33.33) {
		t.Errorf("pass %d fail %d warn %d unknown %d rate %.2f", f.Pass, f.Fail, f.Warn, f.Unknown, f.PassRate)
	}
	fs := complianceFindings(f)
	govHasFinding(t, fs, SevCritical, "TLS: Fail")
	govHasFinding(t, fs, SevMedium, "Multi-factor authentication")
	govHasFinding(t, fs, SevLow, "Plugin signing")
}

// TestGovernanceDocumentsRender builds every family document end to end and
// checks that no planted value, credential or payload reaches any block.
func TestGovernanceDocumentsRender(t *testing.T) {
	keys := []string{TypeChangeApprovals, TypeSecretsHygiene, TypeWorkflowAutomation, TypeWebhookDelivery, TypeCompliancePosture}
	for _, key := range keys {
		rt, _ := reportTypeByKey(key)
		b := govBuildCtx(t, key, govFixture(t, rt.PointInTime))
		doc := buildDocument(b)
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		text := string(raw)
		for _, s := range govLeakSentinels {
			if strings.Contains(text, s) {
				t.Errorf("%s: rendered document contains %q", key, s)
			}
		}
		if len(doc.Sections) < 3 {
			t.Errorf("%s: only %d sections", key, len(doc.Sections))
		}
		if k := b.headlineKPIs(); len(k) == 0 || k[0].Label == "Workloads" {
			t.Errorf("%s: headline KPIs fell back to the generic set: %+v", key, k)
		}
		if lead := governanceLead(b); lead == "" {
			t.Errorf("%s: empty lead", key)
		}
	}
}

// TestGovernanceNotAssessed checks that a denied source reads as not assessed
// rather than as a clean zero.
func TestGovernanceNotAssessed(t *testing.T) {
	for _, key := range []string{TypeChangeApprovals, TypeSecretsHygiene, TypeWorkflowAutomation, TypeWebhookDelivery, TypeCompliancePosture} {
		rt, _ := reportTypeByKey(key)
		snap := govFixture(t, rt.PointInTime)
		for i := range snap.Sources {
			snap.Sources[i].OK = false
			snap.Sources[i].Note = "not permitted for this user"
		}
		b := govBuildCtx(t, key, snap)
		raw, err := json.Marshal(buildDocument(b))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "Not assessed") {
			t.Errorf("%s: denied sources did not produce a Not assessed callout", key)
		}
		if len(governanceFindings(b)) != 0 {
			t.Errorf("%s: findings produced from sources that did not answer", key)
		}
	}
}

// TestGovernanceCollection drives the real collectors against a fake host: it
// checks the endpoints called, that no value-returning endpoint is touched,
// that per-item reads stay within the concurrency bound, and that the webhook
// URL is reduced to its host before anything is stored.
func TestGovernanceCollection(t *testing.T) {
	var inFlight, peak int32
	var mu sync.Mutex
	paths := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()
		data := "[]"
		switch {
		case r.URL.Path == "/api/secrets":
			var items []string
			for i := 0; i < 12; i++ {
				items = append(items, `{"id":"s`+string(rune('a'+i))+`","name":"n","version":2,"referenceCount":1,"value":"SENTINEL-SECRET-VALUE"}`)
			}
			data = `{"items":[` + strings.Join(items, ",") + `],"total":12,"page":1,"per_page":1000,"total_pages":1}`
		case strings.HasSuffix(r.URL.Path, "/impact"):
			data = `{"resources":[],"staleCount":0,"missingCount":0}`
		case r.URL.Path == "/api/webhooks":
			data = `{"items":[{"id":"h1","name":"x","url":"https://u:hunter2@hooks.example.com/SENTINELPATH?token=abc","secret":"SENTINEL-HMAC","isActive":true}],"total":1,"page":1,"per_page":100,"total_pages":1}`
		case r.URL.Path == "/api/compliance/posture":
			if r.URL.Query().Get("redact") != "1" {
				t.Errorf("compliance posture requested without redact=1")
			}
			data = `{"sections":[]}`
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"success":true,"data":` + data + `}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()

	t.Setenv("MCHARBOR_INTERNAL_URL", srv.URL)
	resetBase := func() {
		baseOnce = sync.Once{}
		baseMu.Lock()
		baseWinner = ""
		baseMu.Unlock()
	}
	resetBase()
	defer resetBase()

	api := newHostAPI("", "en", "")
	snap := &Snapshot{Period: Period{Start: govTestEnd.AddDate(0, 0, -7), End: govTestEnd}}
	for _, job := range governanceJobs(context.Background(), api, snap, snap.Period) {
		job()
	}
	snap.Sources = api.sources()

	for _, want := range []string{"/api/approvals", "/api/approvals/rules", "/api/approvals/categories",
		"/api/elevations/all", "/api/secrets", "/api/secrets/sa/versions", "/api/secrets/sa/impact",
		"/api/workflows", "/api/workflows/runs", "/api/webhooks", "/api/webhooks/h1/deliveries", "/api/compliance/posture"} {
		if paths[want] == 0 {
			t.Errorf("collector never called %s", want)
		}
	}
	for p := range paths {
		if strings.Contains(p, "/reveal") || strings.Contains(p, "/generate") || strings.Contains(p, "/references/prune") {
			t.Errorf("collector called a value-returning or mutating endpoint: %s", p)
		}
	}
	if peak > govFanOutWorkers {
		t.Errorf("peak concurrency %d exceeds the fan-out bound", peak)
	}
	if got := snap.Governance.Webhooks; len(got) != 1 || got[0].Host != "hooks.example.com" {
		t.Errorf("webhook reduced to %+v", got)
	}
	raw, err := json.Marshal(snap.Governance)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range govLeakSentinels {
		if strings.Contains(string(raw), s) {
			t.Errorf("collected data holds %q", s)
		}
	}
	if !snap.available(srcSecretVersions) || !snap.available(srcWebhookDeliveries) {
		t.Errorf("fan-out statuses missing: %+v", snap.Sources)
	}
	versionRows := 0
	for _, st := range snap.Sources {
		if st.Key == srcSecretVersions {
			versionRows++
		}
	}
	if versionRows != 1 {
		t.Errorf("secret versions recorded %d annex rows, want one aggregate", versionRows)
	}
}
