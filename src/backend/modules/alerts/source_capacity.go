// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/capacity"
	"github.com/therealmcsparrow/mcharbor/core/forecast"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// conditionCapacityDays is the first trend condition McHarbor ships. Every other
// host condition compares an instantaneous reading; this one compares a
// projection, which is why its poll interval is minutes rather than a tick.
const conditionCapacityDays = "capacity_days"

// capacityPollInterval keeps a fit over the retained series off the fast path. The
// series is hourly, so evaluating it more often than this cannot change the
// answer.
const capacityPollInterval = 30 * time.Minute

// capacityConditionDecl declares "projected to run out of space within N days".
//
// The host compares, not the source: the operator's operator and threshold never
// reach here, so the observation carries days-to-exhaustion as its value and the
// engine decides what "< 14" means. Two sources cannot then disagree about it.
func capacityConditionDecl() alerting.AlertConditionDecl {
	return alerting.AlertConditionDecl{
		Key:              conditionCapacityDays,
		DisplayName:      "Days until storage is full",
		Description:      "How many days until a filesystem or storage destination reaches its red line, projected from the retained hourly capacity series. A subject already past the red line reports 0; one with too little history reports nothing rather than a large number.",
		Kind:             alerting.KindThreshold,
		Unit:             "days",
		Operators:        []string{"<", "<="},
		DefaultOperator:  "<",
		DefaultThreshold: 14,
		SupportsFor:      true,
		TargetKind:       alerting.TargetEnvironment,
		PollInterval:     capacityPollInterval,
	}
}

// evaluateCapacity reports days-to-exhaustion for every capacity subject in the
// environment.
//
// Two rules shape the result, and both are the difference between an alert that
// recovers and one that sticks:
//
//   - A healthy subject still gets an observation. Returning only the subjects
//     in trouble reads to the engine as "everything recovered" for the rest.
//   - A **refused** subject gets no observation at all. It is neither healthy nor
//     firing, and emitting a large value for it would make "not enough data" read
//     as a thousand days of headroom.
func (s *hostSource) evaluateCapacity(ctx context.Context, req alerting.EvaluateRequest) ([]alerting.AlertObservation, error) {
	if s.deps.DB == nil {
		return nil, fmt.Errorf("capacity condition: no database")
	}
	window := coreSettings.ReadRetentionSettingsContext(ctx, s.deps.DB).MetricsRollupDays
	forecasts, err := capacity.Projections(ctx, s.deps.DB, req.EnvironmentID, window, time.Now())
	if err != nil {
		return nil, fmt.Errorf("capacity projections: %w", err)
	}
	out := make([]alerting.AlertObservation, 0, len(forecasts))
	for _, f := range forecasts {
		pastRedLine := !f.HasDate() && f.UsedPercent >= forecast.RedPercent
		if !f.HasDate() && !pastRedLine {
			continue
		}
		name := f.SubjectName
		if name == "" {
			name = f.SubjectID
		}
		if pastRedLine {
			// A disk that is full and flat is refused before the projection runs,
			// so the refusal reason cannot be trusted to say it is over the line.
			out = append(out, alerting.AlertObservation{
				SubjectID:   f.SubjectKind + ":" + f.SubjectID,
				SubjectName: name,
				Value:       0,
				ValueLabel:  "past the red line",
				Context: map[string]string{
					"subject_kind": f.SubjectKind,
					"used_percent": formatFloat(f.UsedPercent),
					"ceiling":      formatFloat(forecast.RedPercent),
					"refusal":      f.Refusal,
				},
			})
			continue
		}
		out = append(out, alerting.AlertObservation{
			SubjectID:   f.SubjectKind + ":" + f.SubjectID,
			SubjectName: name,
			Value:       f.Red.Days,
			ValueLabel:  formatFloat(f.Red.Days) + " days",
			Context: map[string]string{
				"subject_kind":  f.SubjectKind,
				"used_percent":  formatFloat(f.UsedPercent),
				"ceiling":       formatFloat(f.Red.Percent),
				"r2":            formatFloat(f.R2),
				"observed_days": formatFloat(f.ObservedDays),
			},
		})
	}
	return out, nil
}
