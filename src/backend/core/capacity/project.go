// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capacity

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/forecast"
)

// Crossing is the date a subject reaches one of the two lines, at the rate the
// retained series shows.
type Crossing struct {
	Percent float64   `json:"percent"`
	Days    float64   `json:"days"`
	At      time.Time `json:"at"`
}

// Forecast is one subject's answer, refusal included.
//
// Refusal is not an error field. A subject with three days of history has no
// date and that is the correct output; the reason and the two day counts are
// what let a surface say "needs 7 days, has 3" instead of drawing a blank cell
// or, worse, a number.
type Forecast struct {
	EnvironmentID string `json:"environmentId"`
	SubjectKind   string `json:"subjectKind"`
	SubjectID     string `json:"subjectId"`
	SubjectName   string `json:"subjectName"`

	UsedBytes   int64   `json:"usedBytes"`
	TotalBytes  int64   `json:"totalBytes"`
	UsedPercent float64 `json:"usedPercent"`

	ObservedDays float64 `json:"observedDays"`
	RequiredDays float64 `json:"requiredDays"`
	Samples      int     `json:"samples"`
	WindowDays   int     `json:"windowDays"`

	PercentPerDay float64 `json:"percentPerDay"`
	R2            float64 `json:"r2"`

	Amber   *Crossing `json:"amber,omitempty"`
	Red     *Crossing `json:"red,omitempty"`
	Refusal string    `json:"refusal,omitempty"`
}

// HasDate reports whether this forecast produced the sentence the feature exists
// for. A refused subject is neither healthy nor firing, and callers that report
// a value must check this rather than reading DaysUntil off a zero Crossing.
func (f Forecast) HasDate() bool { return f.Red != nil }

// Project turns one subject's retained series into a forecast, or into a named
// refusal. The gates are core/forecast's; nothing here invents a threshold.
func Project(s Subject, windowDays int, now time.Time) Forecast {
	pts := percentSeries(s.Points)
	out := Forecast{
		EnvironmentID: s.EnvironmentID,
		SubjectKind:   s.Kind,
		SubjectID:     s.ID,
		SubjectName:   s.Name,
		RequiredDays:  forecast.MinDays,
		Samples:       len(pts),
		WindowDays:    windowDays,
		Refusal:       string(forecast.RefusalNoFit),
	}
	if n := len(s.Points); n > 0 {
		last := s.Points[n-1]
		out.UsedBytes = last.UsedBytes
		out.TotalBytes = last.TotalBytes
		if last.TotalBytes > 0 {
			out.UsedPercent = forecast.Round(float64(last.UsedBytes)/float64(last.TotalBytes)*100, 1)
		}
	}
	if len(pts) >= 2 {
		out.ObservedDays = forecast.Round(pts[len(pts)-1].At.Sub(pts[0].At).Hours()/24, 1)
	}

	// The two lines are projected independently: a subject already past amber
	// still has a useful red date, and reporting the amber refusal for both
	// would hide it.
	red, refusal := forecast.Exhaustion(pts, forecast.RedPercent)
	out.Refusal = string(refusal)
	if refusal == forecast.RefusalNone {
		out.Refusal = ""
		out.PercentPerDay = forecast.Round(red.PercentPerDay, 3)
		out.R2 = forecast.Round(red.R2, 3)
		out.Red = crossing(forecast.RedPercent, red.DaysUntil, now)
		if amber, ok := forecast.Exhaustion(pts, forecast.AmberPercent); ok == forecast.RefusalNone {
			out.Amber = crossing(forecast.AmberPercent, amber.DaysUntil, now)
		}
	}
	return out
}

func crossing(percent, days float64, now time.Time) *Crossing {
	if days < 0 || math.IsNaN(days) || math.IsInf(days, 0) {
		return nil
	}
	return &Crossing{
		Percent: percent,
		Days:    forecast.Round(days, 1),
		At:      now.UTC().Add(time.Duration(days * 24 * float64(time.Hour))),
	}
}

// Projections reads the retained series for one environment (or every
// environment when envID is empty) and projects each subject.
//
// windowDays is the retained horizon, not a preference: the series is pruned at
// it, so asking for more than it holds would silently read less.
func Projections(ctx context.Context, db *sql.DB, envID string, windowDays int, now time.Time) ([]Forecast, error) {
	if windowDays < 1 {
		windowDays = 1
	}
	subjects, err := Series(ctx, db, envID, now.UTC().AddDate(0, 0, -windowDays))
	if err != nil {
		return nil, err
	}
	out := make([]Forecast, 0, len(subjects))
	for _, s := range subjects {
		out = append(out, Project(s, windowDays, now))
	}
	return out, nil
}
