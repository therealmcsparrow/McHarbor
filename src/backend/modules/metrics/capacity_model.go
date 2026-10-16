// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import "github.com/therealmcsparrow/mcharbor/core/capacity"

// CapacityReport is the answer GET /api/metrics/capacity gives: one forecast per
// subject, refusals included, plus the two lines the projections aim at.
//
// Thresholds travel with the payload so a client never hard-codes them a second
// time, and WindowDays is stated rather than implied — a projection whose window
// is not on screen is a guess wearing a date.
type CapacityReport struct {
	Subjects     []capacity.Forecast `json:"subjects"`
	WindowDays   int                 `json:"windowDays"`
	RequiredDays float64             `json:"requiredDays"`
	AmberPercent float64             `json:"amberPercent"`
	RedPercent   float64             `json:"redPercent"`
	BucketHours  float64             `json:"bucketHours"`
}

// IdleResource is one reclaim candidate: what it is, how long it has been idle,
// and what it holds.
//
// SizeKnown is not a convenience flag. Writable-layer sizes are not collected for
// agent environments and are NULL on samples an agent pushed, so a size of zero
// would be a number this feature is trusted for and silently wrong. A false here
// means the UI must say "size unknown" and name the reason.
type IdleResource struct {
	EnvironmentID string `json:"environmentId"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`

	// Reason is "stopped" or "quiet". They are kept apart deliberately: a
	// container stopped for 60 days is a fact, a running container with no
	// traffic is a judgement call, and one list showing both as the same thing
	// would make the safe half untrustworthy.
	Reason string `json:"reason"`

	IdleDays float64 `json:"idleDays"`

	SizeKnown     bool   `json:"sizeKnown"`
	SizeReason    string `json:"sizeReason,omitempty"`
	SizeRwBytes   int64  `json:"sizeRwBytes"`
	SizeRootBytes int64  `json:"sizeRootBytes"`
}

// IdleReport is the answer GET /api/metrics/capacity/idle gives.
//
// ReclaimableBytes is Docker's own figure from `system df` — what a prune would
// return today — and is reported beside the candidates rather than summed from
// them, because the two count different things and adding them would double.
type IdleReport struct {
	Containers       []IdleResource `json:"containers"`
	ThresholdDays    int            `json:"thresholdDays"`
	ReclaimableBytes int64          `json:"reclaimableBytes"`
	// SizesUnavailable is true when this environment collects no writable-layer
	// sizes at all, so the table can explain it once instead of on every row.
	SizesUnavailable bool `json:"sizesUnavailable"`
}
