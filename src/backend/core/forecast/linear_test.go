// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package forecast

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

var origin = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// series builds a daily series from start, rising by perDay, with noise added to
// every second point so a caller can make a fit as bad as it likes.
func series(days int, start, perDay, noise float64) []Sample {
	out := make([]Sample, 0, days)
	for i := 0; i < days; i++ {
		v := start + perDay*float64(i)
		if noise != 0 && i%2 == 1 {
			v += noise
		}
		out = append(out, Sample{At: origin.AddDate(0, 0, i), Value: v})
	}
	return out
}

func TestLinearFitsKnownInputs(t *testing.T) {
	// Exactly y = 10 + 2x over eleven days. Slope, intercept and R2 are all
	// determined, so this is arithmetic and not a tolerance test.
	fit, ok := Linear(series(11, 10, 2, 0))
	if !ok {
		t.Fatal("a clean eleven-day line must produce a fit")
	}
	if math.Abs(fit.SlopePerDay-2) > 1e-9 {
		t.Fatalf("slope = %v, want 2", fit.SlopePerDay)
	}
	if math.Abs(fit.Intercept-10) > 1e-9 {
		t.Fatalf("intercept = %v, want 10", fit.Intercept)
	}
	if math.Abs(fit.R2-1) > 1e-9 {
		t.Fatalf("r2 = %v, want 1", fit.R2)
	}
	if math.Abs(fit.Days-10) > 1e-9 {
		t.Fatalf("days = %v, want 10", fit.Days)
	}
}

func TestLinearRefusesFewerThanTwoPoints(t *testing.T) {
	if _, ok := Linear(nil); ok {
		t.Fatal("no points must not produce a fit")
	}
	if _, ok := Linear(series(1, 10, 2, 0)); ok {
		t.Fatal("one point must not produce a fit")
	}
}

func TestLinearRefusesPointsAllAtTheSameInstant(t *testing.T) {
	pts := []Sample{{At: origin, Value: 1}, {At: origin, Value: 2}, {At: origin, Value: 3}}
	if _, ok := Linear(pts); ok {
		t.Fatal("a zero time span has no slope and must be refused")
	}
}

// The five refusals are the point of the feature: each one is a case where a date
// could be produced and must not be.
func TestExhaustionRefusals(t *testing.T) {
	cases := []struct {
		name string
		pts  []Sample
		want Refusal
	}{
		{"three samples is too short", series(3, 10, 2, 0), RefusalShortSeries},
		{"a flat series is not rising", series(14, 50, 0, 0), RefusalNotRising},
		{"a falling series is not rising", series(14, 50, -1, 0), RefusalNotRising},
		{"a noisy series does not describe a trend", series(21, 50, 0.05, 18), RefusalNoisy},
		{"already past the ceiling", series(14, 91, 0.2, 0), RefusalAlreadyOver},
		{"a crawl reaches the ceiling past the horizon", series(14, 20, 0.01, 0), RefusalBeyondHorizon},
		{"fewer than two points is no fit at all", series(1, 10, 2, 0), RefusalNoFit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, refusal := Exhaustion(c.pts, RedPercent)
			if refusal != c.want {
				t.Fatalf("refusal = %q, want %q", refusal, c.want)
			}
			if p.DaysUntil != 0 {
				t.Fatalf("a refused projection must carry no days, got %v", p.DaysUntil)
			}
		})
	}
}

func TestExhaustionAnswersACleanClimb(t *testing.T) {
	// Eight days from 80% rising one point a day: at 87% on the last sample, the
	// red line at 90% is three days out.
	p, refusal := Exhaustion(series(8, 80, 1, 0), RedPercent)
	if refusal != RefusalNone {
		t.Fatalf("a clean eight-day climb was refused: %q", refusal)
	}
	if math.Abs(p.CurrentPercent-87) > 1e-9 {
		t.Fatalf("current = %v, want 87", p.CurrentPercent)
	}
	if math.Abs(p.DaysUntil-3) > 1e-9 {
		t.Fatalf("daysUntil = %v, want 3", p.DaysUntil)
	}
	if p.Ceiling != RedPercent {
		t.Fatalf("ceiling = %v, want %v", p.Ceiling, RedPercent)
	}
}

// Exactly MinDays of span is the boundary, and the gate is "fewer than", so a
// series spanning exactly seven days is answered and one spanning less is not.
func TestExhaustionBoundaryAtMinDays(t *testing.T) {
	if _, refusal := Exhaustion(series(MinDays+1, 80, 1, 0), RedPercent); refusal != RefusalNone {
		t.Fatalf("a span of exactly %d days must be answered, got %q", MinDays, refusal)
	}
	if _, refusal := Exhaustion(series(MinDays, 80, 1, 0), RedPercent); refusal != RefusalShortSeries {
		t.Fatalf("a span of %d days is short and must be refused, got %q", MinDays-1, refusal)
	}
}

// A property test earns its place here: the input space is every rising series,
// and the three invariants below are what an operator relies on without knowing it.
func TestExhaustionProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(20260927))
	for i := 0; i < 2000; i++ {
		days := 2 + rng.Intn(40)
		start := rng.Float64() * 95
		perDay := (rng.Float64() - 0.2) * 3
		noise := (rng.Float64() - 0.5) * 6
		pts := series(days, start, perDay, noise)

		p, refusal := Exhaustion(pts, RedPercent)
		if refusal != RefusalNone {
			continue
		}
		if p.DaysUntil < 0 {
			t.Fatalf("projected date is in the past: %v (start %v perDay %v days %v)", p.DaysUntil, start, perDay, days)
		}
		if p.DaysUntil > MaxDays {
			t.Fatalf("projected %v days, beyond the %d-day horizon", p.DaysUntil, MaxDays)
		}
		// Monotonic in the slope: the same series climbing faster can only reach
		// the ceiling sooner, never later.
		faster, fasterRefusal := Exhaustion(series(days, start, perDay*2, noise), RedPercent)
		if fasterRefusal == RefusalNone && faster.DaysUntil > p.DaysUntil+1e-9 {
			t.Fatalf("doubling the slope moved the date later: %v then %v", p.DaysUntil, faster.DaysUntil)
		}
	}
}

func TestRoundIsStable(t *testing.T) {
	if got := Round(1.23456, 3); got != 1.235 {
		t.Fatalf("Round(1.23456, 3) = %v, want 1.235", got)
	}
	if got := Round(2.5, 0); got != 3 {
		t.Fatalf("Round(2.5, 0) = %v, want 3", got)
	}
}
