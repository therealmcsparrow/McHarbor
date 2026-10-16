// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/capacity"
)

// CapacityReadings reports used-of-total for every enabled destination, for the
// hourly capacity series.
//
// A destination whose provider cannot answer is **absent** from the result
// rather than present with zeroes: Usage returns Supported=false for a plugin
// that never opted into Quota, and a forecast reading "0 of 0" would show a full
// disk for a destination nobody can measure.
func (s *Service) CapacityReadings(ctx context.Context, environmentID string) ([]capacity.Reading, error) {
	locations, err := s.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing storage locations for capacity: %w", err)
	}
	out := make([]capacity.Reading, 0, len(locations))
	for _, loc := range locations {
		if !loc.Enabled {
			continue
		}
		usage, err := s.Usage(ctx, loc.ID)
		// One unreachable destination must not cost the sweep the others. The
		// error is the caller's to log; the subject is simply absent this hour,
		// which the fit tolerates because it reads a series, not a tick.
		if err != nil || usage == nil || !usage.Supported || usage.TotalBytes <= 0 {
			continue
		}
		out = append(out, capacity.Reading{
			EnvironmentID: environmentID,
			SubjectKind:   capacity.KindStorageLocation,
			SubjectID:     loc.ID,
			SubjectName:   loc.Name,
			UsedBytes:     usage.UsedBytes,
			TotalBytes:    usage.TotalBytes,
		})
	}
	return out, nil
}
