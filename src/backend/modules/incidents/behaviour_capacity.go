// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/capacity"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// capacityForecasts records a finding for every filesystem or storage
// destination projected to cross the red line inside the forecast horizon.
//
// It reads the configured rollup retention unclamped: the capacity series is
// hourly and pruned at that horizon, so asking for less would discard history the
// operator is already paying to keep. The memory detectors read the same setting,
// clamped by memoryLookbackDays.
func (b *Behaviour) capacityForecasts(ctx context.Context, now time.Time, record recordFn) error {
	window := coreSettings.ReadRetentionSettingsContext(ctx, b.DB).MetricsRollupDays
	forecasts, err := capacity.Projections(ctx, b.DB, "", window, now)
	if err != nil {
		return err
	}
	for _, f := range forecasts {
		// A refused subject is not a finding. Three samples produce nothing,
		// which is the design: a wrong date is worse than no date because the
		// operator acts on it.
		if !f.HasDate() {
			continue
		}
		kind, subjectType := FindingHostDiskFull, "host"
		if f.SubjectKind == capacity.KindStorageLocation {
			kind, subjectType = FindingStorageLocationFull, "storage_location"
		}
		subject := f.SubjectName
		if subject == "" {
			subject = f.SubjectID
		}
		metrics := map[string]float64{
			"currentPercent": f.UsedPercent,
			"percentPerDay":  f.PercentPerDay,
			"daysUntilFull":  f.Red.Days,
			"ceilingPercent": f.Red.Percent,
			"r2":             f.R2,
			"days":           f.ObservedDays,
			"usedBytes":      float64(f.UsedBytes),
			"totalBytes":     float64(f.TotalBytes),
		}
		if f.Amber != nil {
			metrics["daysUntilAmber"] = f.Amber.Days
		}
		if err := record(f.EnvironmentID, subjectType, subject, kind, metrics); err != nil {
			return err
		}
	}
	return nil
}
