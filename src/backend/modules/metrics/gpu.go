// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/gpu"
)

// gpuUtilizationSample returns the mean utilisation across all visible
// GPUs, and whether any reading was taken.
//
// The history rollup stores one number per bucket, so a multi-GPU host
// has to be reduced to a single series. The mean is the honest choice
// for "how busy is this machine's GPU capacity": a max would make one
// pinned card look like a saturated host, and a sum would exceed 100%
// and break every axis that assumes a percentage.
func gpuUtilizationSample(ctx context.Context, reader *gpu.Reader) (float64, bool) {
	if reader == nil || !reader.Available() {
		return 0, false
	}
	devices, err := reader.Devices(ctx)
	if err != nil || len(devices) == 0 {
		return 0, false
	}
	var total float64
	for _, d := range devices {
		total += d.Utilization
	}
	return total / float64(len(devices)), true
}
