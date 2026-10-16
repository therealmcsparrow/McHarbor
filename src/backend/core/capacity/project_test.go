// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capacity

import (
	"context"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/forecast"
)

var now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// climbing builds a subject rising by perDayPercent a day, ending today.
func climbing(days int, startPercent, perDayPercent float64) Subject {
	const total int64 = 1_000_000
	pts := make([]Point, 0, days)
	for i := 0; i < days; i++ {
		pct := startPercent + perDayPercent*float64(i)
		at := now.AddDate(0, 0, -(days - 1 - i))
		pts = append(pts, Point{At: at, UsedBytes: int64(pct / 100 * float64(total)), TotalBytes: total})
	}
	return Subject{EnvironmentID: "env1", Kind: KindRootFS, ID: "env1", Name: "/", Points: pts}
}

// The whole point of the feature: a series too short produces no date, and says
// why, with both day counts so a surface can render "needs 7, has 3".
func TestProjectRefusesAShortSeriesRatherThanExtrapolating(t *testing.T) {
	f := Project(climbing(4, 80, 2), 30, now)
	if f.HasDate() {
		t.Fatal("four days of history must not produce a date")
	}
	if f.Refusal != string(forecast.RefusalShortSeries) {
		t.Fatalf("refusal = %q, want %q", f.Refusal, forecast.RefusalShortSeries)
	}
	if f.RequiredDays != forecast.MinDays {
		t.Fatalf("requiredDays = %v, want %v", f.RequiredDays, forecast.MinDays)
	}
	if f.ObservedDays != 3 {
		t.Fatalf("observedDays = %v, want 3", f.ObservedDays)
	}
	if f.Red != nil || f.Amber != nil {
		t.Fatal("a refused subject must carry neither crossing")
	}
	// The current reading is still reported: the operator can see how full it is
	// even when nobody can say when it fills.
	if f.UsedPercent != 86 {
		t.Fatalf("usedPercent = %v, want 86", f.UsedPercent)
	}
}

func TestProjectNamesEachRefusal(t *testing.T) {
	cases := []struct {
		name string
		subj Subject
		want forecast.Refusal
	}{
		{"flat", climbing(14, 50, 0), forecast.RefusalNotRising},
		{"already over the red line", climbing(14, 92, 0.1), forecast.RefusalAlreadyOver},
		{"a crawl beyond the horizon", climbing(14, 20, 0.01), forecast.RefusalBeyondHorizon},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Project(c.subj, 30, now)
			if f.Refusal != string(c.want) {
				t.Fatalf("refusal = %q, want %q", f.Refusal, c.want)
			}
			if f.HasDate() {
				t.Fatal("a refused subject must not carry a date")
			}
		})
	}
}

func TestProjectAnswersWithBothLines(t *testing.T) {
	// Ten days from 70% rising one point a day ends at 79%: amber at 75 is behind
	// it, red at 90 is eleven days out.
	f := Project(climbing(10, 70, 1), 30, now)
	if !f.HasDate() {
		t.Fatalf("a clean ten-day climb was refused: %q", f.Refusal)
	}
	if f.Refusal != "" {
		t.Fatalf("an answered subject must carry no refusal, got %q", f.Refusal)
	}
	if f.Red.Percent != forecast.RedPercent {
		t.Fatalf("red ceiling = %v, want %v", f.Red.Percent, forecast.RedPercent)
	}
	if f.Red.Days < 10 || f.Red.Days > 12 {
		t.Fatalf("red in %v days, want about 11", f.Red.Days)
	}
	if !f.Red.At.After(now) {
		t.Fatalf("the red date %v is not in the future", f.Red.At)
	}
	// Amber is already behind this subject, so it is projected independently and
	// simply absent — reporting amber's refusal for both would hide the red date.
	if f.Amber != nil {
		t.Fatalf("amber is already passed and must be absent, got %+v", f.Amber)
	}
	if f.WindowDays != 30 {
		t.Fatalf("windowDays = %d, want 30", f.WindowDays)
	}
}

func TestProjectReportsBothLinesWhenBothAreAhead(t *testing.T) {
	f := Project(climbing(12, 55, 1.2), 30, now)
	if !f.HasDate() {
		t.Fatalf("refused: %q", f.Refusal)
	}
	if f.Amber == nil {
		t.Fatal("amber is ahead of this subject and must be projected")
	}
	if f.Amber.Days >= f.Red.Days {
		t.Fatalf("amber (%v) must come before red (%v)", f.Amber.Days, f.Red.Days)
	}
}

func TestProjectionsSkipPointsWithNoCeiling(t *testing.T) {
	db := openCapacityTestDB(t)
	ctx := context.Background()
	// A single point is written; the series cannot be fitted and the refusal is
	// the no-fit one rather than a crash or a zero date.
	if err := Write(ctx, db, rootFS(10, 100), now); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	out, err := Projections(ctx, db, "env1", 30, now)
	if err != nil {
		t.Fatalf("projections: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d subjects, want 1", len(out))
	}
	if out[0].HasDate() {
		t.Fatal("one point must not produce a date")
	}
	if out[0].Refusal != string(forecast.RefusalNoFit) {
		t.Fatalf("refusal = %q, want %q", out[0].Refusal, forecast.RefusalNoFit)
	}
}
