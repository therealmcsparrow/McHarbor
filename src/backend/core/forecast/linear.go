// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package forecast holds the one linear fit McHarbor extrapolates from, and the
// gates that make it refuse rather than guess.
//
// The arithmetic lived in modules/incidents/behaviour.go, where the incident
// narrator's detectors are. Capacity forecasting needs the same fit from
// modules/metrics, modules/alerts and core/capacity, and a second copy of an
// ordinary least-squares line is how two surfaces start disagreeing about
// whether a host is filling up. So the fit moved here and behaviour.go delegates
// to it; there is still exactly one.
//
// Nothing in this package reads a database, calls a model or knows what a
// filesystem is. It takes points and returns a line, or nothing.
package forecast

import (
	"math"
	"time"
)

// The refusal gates. A projection an operator cannot trust is worse than no
// projection, because they act on it: below MinDays there is not enough history
// to see a trend, below MinR2 the line does not describe the points, and past
// MaxDays the extrapolation is longer than the window it came from.
const (
	MinDays = 7
	MinR2   = 0.7
	MaxDays = 30
)

// The amber and red lines a capacity projection aims at. A filesystem at 100% is
// already broken, so the useful date is the one it crosses these on.
//
// The frontend carries the same two numbers in
// src/frontend/resources/utils/capacity.ts, and
// core/capacity/thresholds_contract_test.go fails when the two drift apart.
const (
	AmberPercent = 75.0
	RedPercent   = 90.0
)

// Sample is one point of a series: a value at a time.
type Sample struct {
	At    time.Time
	Value float64
}

// Fit is an ordinary least-squares line through a series, expressed per day.
type Fit struct {
	SlopePerDay float64
	Intercept   float64
	R2          float64
	Days        float64
}

// Linear fits value against time in days. It needs two points spread over time;
// fewer is no fit, reported as ok=false rather than a zero slope.
func Linear(pts []Sample) (Fit, bool) {
	if len(pts) < 2 {
		return Fit{}, false
	}
	t0 := pts[0].At
	var sx, sy, sxx, sxy float64
	n := float64(len(pts))
	for _, p := range pts {
		x := p.At.Sub(t0).Hours() / 24
		sx += x
		sy += p.Value
		sxx += x * x
		sxy += x * p.Value
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return Fit{}, false
	}
	slope := (n*sxy - sx*sy) / den
	icpt := (sy - slope*sx) / n
	mean := sy / n
	var ssTot, ssRes float64
	for _, p := range pts {
		x := p.At.Sub(t0).Hours() / 24
		ssTot += (p.Value - mean) * (p.Value - mean)
		r := p.Value - (slope*x + icpt)
		ssRes += r * r
	}
	r2 := 1.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}
	return Fit{SlopePerDay: slope, Intercept: icpt, R2: r2, Days: pts[len(pts)-1].At.Sub(t0).Hours() / 24}, true
}

// Refusal names why a projection was not made, so a surface can say "needs 7
// days, has 3" instead of showing a blank cell or a very large number.
type Refusal string

// The refusals, in the order Exhaustion checks them.
const (
	RefusalNone          Refusal = ""
	RefusalNoFit         Refusal = "no_fit"
	RefusalShortSeries   Refusal = "short_series"
	RefusalNoisy         Refusal = "noisy"
	RefusalNotRising     Refusal = "not_rising"
	RefusalAlreadyOver   Refusal = "already_over"
	RefusalBeyondHorizon Refusal = "beyond_horizon"
)

// Projection is a fit that survived every gate: where the series is now, how
// fast it is moving, and how many days until it reaches Ceiling.
type Projection struct {
	CurrentPercent float64
	PercentPerDay  float64
	DaysUntil      float64
	R2             float64
	Days           float64
	Ceiling        float64
}

// Exhaustion projects when a percentage series reaches ceiling, or names the
// reason it will not answer. Every caller shares these gates, so two surfaces
// cannot disagree about whether a subject has a date.
func Exhaustion(pts []Sample, ceiling float64) (Projection, Refusal) {
	fit, ok := Linear(pts)
	if !ok {
		return Projection{}, RefusalNoFit
	}
	if fit.Days < MinDays {
		return Projection{}, RefusalShortSeries
	}
	if fit.R2 < MinR2 {
		return Projection{}, RefusalNoisy
	}
	if fit.SlopePerDay <= 0 {
		return Projection{}, RefusalNotRising
	}
	current := fit.Intercept + fit.SlopePerDay*fit.Days
	if current >= ceiling {
		return Projection{}, RefusalAlreadyOver
	}
	days := (ceiling - current) / fit.SlopePerDay
	if days > MaxDays {
		return Projection{}, RefusalBeyondHorizon
	}
	return Projection{
		CurrentPercent: current,
		PercentPerDay:  fit.SlopePerDay,
		DaysUntil:      days,
		R2:             fit.R2,
		Days:           fit.Days,
		Ceiling:        ceiling,
	}, RefusalNone
}

// Round returns v to places decimals. Every metric bag stored in a finding is
// rounded on the way in, so two runs of the same series produce the same row.
func Round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
