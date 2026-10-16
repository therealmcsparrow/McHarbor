// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

var (
	testStart = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	testEnd   = time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
)

func at(day, hour int) time.Time {
	return time.Date(2026, 9, day, hour, 0, 0, 0, time.UTC)
}

func statFor(t *testing.T, a deliveryAnalysis, id string) forwarderStat {
	t.Helper()
	for _, s := range a.Forwarders {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no stats for forwarder %q", id)
	return forwarderStat{}
}

func findingsWith(a deliveryAnalysis, fragment string) []plugin.ReportFinding {
	var out []plugin.ReportFinding
	for _, f := range a.Findings {
		if strings.Contains(f.Title, fragment) {
			out = append(out, f)
		}
	}
	return out
}

func TestSuccessRateExcludesTestSends(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 23)}},
		Totals: []totalsRow{{ForwarderID: "a", Delivered: 90, Failed: 10, Tests: 40,
			FirstSuccess: at(1, 0), LastSuccess: at(7, 23)}},
	})
	s := statFor(t, a, "a")
	if !s.HasRate || math.Abs(s.Rate-90) > 1e-9 {
		t.Fatalf("rate = %v (has=%v), want 90", s.Rate, s.HasRate)
	}
	if !a.HasRate || math.Abs(a.Rate-90) > 1e-9 || a.Tests != 40 {
		t.Fatalf("overall rate = %v tests = %d, want 90 and 40", a.Rate, a.Tests)
	}
}

func TestNoAttemptsHasNoRate(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Idle", Enabled: true, EnabledSince: at(6, 0)}},
	})
	if s := statFor(t, a, "a"); s.HasRate {
		t.Fatalf("a forwarder with no attempts must not claim a rate, got %v", s.Rate)
	}
	if len(findingsWith(a, "delivered only")) != 0 {
		t.Fatal("no attempts must not raise a success-rate finding")
	}
}

func TestLongestGapPrefersInteriorWhenLongest(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 22)}},
		Totals:     []totalsRow{{ForwarderID: "a", Delivered: 50, FirstSuccess: at(1, 2), LastSuccess: at(7, 22)}},
		Gaps:       []gapRow{{ForwarderID: "a", From: at(3, 0), To: at(4, 12)}},
	})
	s := statFor(t, a, "a")
	if !s.HasGap || s.Gap != 36*time.Hour || !s.GapFrom.Equal(at(3, 0)) {
		t.Fatalf("gap = %v from %v, want 36h from %v", s.Gap, s.GapFrom, at(3, 0))
	}
	if a.LongestGap != 36*time.Hour || a.LongestGapName != "Prod" {
		t.Fatalf("overall longest gap = %v on %q", a.LongestGap, a.LongestGapName)
	}
}

func TestLongestGapCountsTrailingSilence(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(5, 0)}},
		Totals:     []totalsRow{{ForwarderID: "a", Delivered: 5, FirstSuccess: at(1, 1), LastSuccess: at(5, 0)}},
		Gaps:       []gapRow{{ForwarderID: "a", From: at(2, 0), To: at(2, 5)}},
	})
	s := statFor(t, a, "a")
	if s.Gap != 72*time.Hour || !s.GapTo.Equal(testEnd) {
		t.Fatalf("gap = %v to %v, want 72h to the period end", s.Gap, s.GapTo)
	}
}

func TestLongestGapStartsAtEnabledSince(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "New", Enabled: true, EnabledSince: at(7, 0)}},
	})
	s := statFor(t, a, "a")
	if s.Gap != 24*time.Hour || !s.GapFrom.Equal(at(7, 0)) {
		t.Fatalf("gap = %v from %v, want 24h from enabled_since", s.Gap, s.GapFrom)
	}
}

func TestDisabledForwarderHasNoEdgeGapOrSilence(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Old", Enabled: false, LastSuccess: at(1, 1)}},
		Totals:     []totalsRow{{ForwarderID: "a", Delivered: 3, FirstSuccess: at(1, 0), LastSuccess: at(1, 1)}},
	})
	s := statFor(t, a, "a")
	if s.HasGap || s.HasSilence {
		t.Fatalf("disabled forwarder: gap=%v silence=%v, want neither", s.HasGap, s.HasSilence)
	}
	if len(a.Findings) != 0 {
		t.Fatalf("disabled forwarder raised findings: %+v", a.Findings)
	}
}

func TestSuccessRateThresholds(t *testing.T) {
	cases := []struct {
		delivered, failed int
		want              string
	}{
		{delivered: 96, failed: 4, want: ""},
		{delivered: 90, failed: 10, want: "medium"},
		{delivered: 70, failed: 30, want: "high"},
	}
	for _, tc := range cases {
		a := analyseDelivery(deliveryInput{
			PeriodStart: testStart, PeriodEnd: testEnd,
			Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 23)}},
			Totals: []totalsRow{{ForwarderID: "a", Delivered: tc.delivered, Failed: tc.failed,
				FirstSuccess: at(1, 0), LastSuccess: at(7, 23), LastFailure: at(7, 1)}},
		})
		got := findingsWith(a, "delivered only")
		switch {
		case tc.want == "" && len(got) != 0:
			t.Errorf("%d/%d: unexpected finding %+v", tc.delivered, tc.failed, got)
		case tc.want != "" && (len(got) != 1 || got[0].Severity != tc.want):
			t.Errorf("%d/%d: findings %+v, want one %s", tc.delivered, tc.failed, got, tc.want)
		}
	}
}

func TestSilenceFindingSeverity(t *testing.T) {
	quiet := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(5, 0)}},
		Totals:     []totalsRow{{ForwarderID: "a", Delivered: 10, FirstSuccess: at(1, 0), LastSuccess: at(5, 0)}},
	})
	got := findingsWith(quiet, "has not delivered")
	if len(got) != 1 || got[0].Severity != "medium" {
		t.Fatalf("quiet forwarder: %+v, want one medium silence finding", got)
	}

	failing := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(5, 0)}},
		Totals: []totalsRow{{ForwarderID: "a", Delivered: 100, Failed: 2,
			FirstSuccess: at(1, 0), LastSuccess: at(5, 0), LastFailure: at(7, 0)}},
	})
	got = findingsWith(failing, "has not delivered")
	if len(got) != 1 || got[0].Severity != "high" {
		t.Fatalf("failing forwarder: %+v, want one high silence finding", got)
	}
	if failing.Findings[0].Severity != "high" {
		t.Fatalf("findings are not ordered by severity: %+v", failing.Findings)
	}

	recent := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 1)}},
	})
	if len(findingsWith(recent, "has not delivered")) != 0 {
		t.Fatal("a forwarder that delivered within the threshold must not be silent")
	}
}

func TestErrorGroupingFoldsAddressesKeepsStatusCodes(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{
			{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 23)},
			{ID: "b", Name: "Staging", Enabled: true, LastSuccess: at(7, 23)},
		},
		Errors: []errorRow{
			{ForwarderID: "a", Error: "dial tcp 10.0.0.5:8088: connect: connection refused", Count: 3, LastSeen: at(2, 0)},
			{ForwarderID: "b", Error: "dial tcp 10.0.0.9:8088:   connect: connection refused", Count: 4, LastSeen: at(3, 0)},
			{ForwarderID: "a", Error: "audit siem endpoint returned status 503", Count: 2, LastSeen: at(4, 0)},
			{ForwarderID: "a", Error: "audit siem endpoint returned status 401", Count: 1, LastSeen: at(4, 0)},
			{ForwarderID: "gone", Error: "", Count: 1, LastSeen: at(1, 0)},
		},
	})
	if len(a.Errors) != 4 {
		t.Fatalf("got %d groups, want 4: %+v", len(a.Errors), a.Errors)
	}
	top := a.Errors[0]
	if top.Count != 7 || top.Pattern != "dial tcp <address>: connect: connection refused" {
		t.Fatalf("top group = %+v", top)
	}
	if !top.LastSeen.Equal(at(3, 0)) || strings.Join(top.Forwarders, ",") != "Prod,Staging" {
		t.Fatalf("top group last seen %v forwarders %v", top.LastSeen, top.Forwarders)
	}
	var sawEmpty, sawRemoved bool
	for _, g := range a.Errors {
		if g.Pattern == "(no error text recorded)" {
			sawEmpty = true
			sawRemoved = len(g.Forwarders) == 1 && g.Forwarders[0] == "Removed forwarder gone"
		}
	}
	if !sawEmpty || !sawRemoved {
		t.Fatalf("empty error text or unknown forwarder not labelled: %+v", a.Errors)
	}
}

func TestRecurringErrorThresholdAndCap(t *testing.T) {
	rows := []errorRow{{ForwarderID: "a", Error: "rare", Count: 4}}
	for i := 0; i < 7; i++ {
		rows = append(rows, errorRow{ForwarderID: "a", Error: "status " + string(rune('A'+i)), Count: 5 + i})
	}
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 23)}},
		Errors:     rows,
	})
	got := findingsWith(a, "Recurring failure")
	if len(got) != maxErrorFindings {
		t.Fatalf("got %d recurring findings, want %d", len(got), maxErrorFindings)
	}
	for _, f := range got {
		if strings.Contains(f.Title, "rare") {
			t.Fatal("an error below the threshold was raised")
		}
	}
	if !strings.Contains(got[0].Title, "status G") {
		t.Fatalf("the most frequent error should come first, got %q", got[0].Title)
	}
}

func TestHistoryWithoutRegistryRowIsKept(t *testing.T) {
	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Totals: []totalsRow{{ForwarderID: "old", Delivered: 1, Failed: 1, FirstSuccess: at(1, 0), LastSuccess: at(1, 0)}},
	})
	s := statFor(t, a, "old")
	if s.Known || s.Enabled || s.Name != "Removed forwarder old" || s.HasSilence {
		t.Fatalf("removed forwarder stats = %+v", s)
	}
	if len(a.Findings) != 0 {
		t.Fatalf("a removed forwarder must not raise findings: %+v", a.Findings)
	}
}

func TestNormaliseErrorTruncates(t *testing.T) {
	got := normaliseError(strings.Repeat("x", 500))
	if n := len([]rune(got)); n != maxPatternRunes {
		t.Fatalf("normalised length = %d, want %d", n, maxPatternRunes)
	}
	if got := normaliseError("id 123456 at [::1]:514"); got != "id <n> at <address>" {
		t.Fatalf("normaliseError = %q", got)
	}
}

func TestSectionsForEmptyAndPopulatedPeriods(t *testing.T) {
	empty := buildDeliverySections(analyseDelivery(deliveryInput{PeriodStart: testStart, PeriodEnd: testEnd}),
		defaultThresholds(), testStart, testEnd)
	if len(empty) != 1 || empty[0].Level != "summary" || empty[0].Blocks[0].Kind != "callout" {
		t.Fatalf("empty report = %+v", empty)
	}

	a := analyseDelivery(deliveryInput{
		PeriodStart: testStart, PeriodEnd: testEnd,
		Forwarders: []forwarderRow{{ID: "a", Name: "Prod", Enabled: true, LastSuccess: at(7, 23)}},
		Totals:     []totalsRow{{ForwarderID: "a", Delivered: 70, Failed: 30, FirstSuccess: at(1, 0), LastSuccess: at(7, 23)}},
		Errors:     []errorRow{{ForwarderID: "a", Error: "boom", Count: 30}},
	})
	sections := buildDeliverySections(a, defaultThresholds(), testStart, testEnd)
	levels := make([]string, 0, len(sections))
	for _, s := range sections {
		levels = append(levels, s.Level)
	}
	if strings.Join(levels, ",") != "summary,strategic,tactical,operational" {
		t.Fatalf("levels = %v", levels)
	}
	if sections[1].Blocks[0].Kind != "findings" || len(sections[1].Blocks[0].Findings) != 2 {
		t.Fatalf("strategic section = %+v", sections[1].Blocks[0])
	}
	for _, k := range sections[0].Blocks[1].KPIs {
		if k.Label == "Success rate" && (k.Value != "70.0%" || k.Status != "bad") {
			t.Fatalf("success rate KPI = %+v", k)
		}
	}
}

func TestReportSpecIsAcceptedByTheHost(t *testing.T) {
	spec := siemReportSpec()
	if spec.Slug != "logging-siem" || spec.DisplayName == "" || len(spec.Types) != 1 {
		t.Fatalf("spec = %+v", spec)
	}
	decl := spec.Types[0]
	if !strings.HasPrefix(decl.Key, spec.Slug+"-") || decl.Category == "" || len(decl.Levels) == 0 {
		t.Fatalf("type declaration = %+v", decl)
	}
	for _, kind := range decl.Subjects {
		if !strings.Contains(kind, "-") {
			t.Fatalf("subject kind %q is not namespaced", kind)
		}
	}
	if decl.PointInTime || decl.DefaultDays <= 0 || decl.EnvironmentScoped {
		t.Fatalf("period/scope declaration = %+v", decl)
	}
}
