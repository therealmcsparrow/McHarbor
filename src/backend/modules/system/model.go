// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import "time"

// OSLogResult contains a bounded host log snapshot.
type OSLogResult struct {
	Source    string    `json:"source"`
	Tail      int       `json:"tail"`
	Lines     []string  `json:"lines"`
	Notices   []string  `json:"notices"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// OSUpdateCheckResult describes available host OS package updates.
type OSUpdateCheckResult struct {
	Manager   string    `json:"manager"`
	Available bool      `json:"available"`
	Updates   []string  `json:"updates"`
	Output    string    `json:"output"`
	CheckedAt time.Time `json:"checkedAt"`
}

// OSUpdateApplyRequest confirms the user intentionally started an OS update.
type OSUpdateApplyRequest struct {
	Confirm bool `json:"confirm"`
}

// OSUpdateApplyResult contains the package-manager update run result.
type OSUpdateApplyResult struct {
	Manager  string    `json:"manager"`
	ExitCode int64     `json:"exitCode"`
	Success  bool      `json:"success"`
	Output   string    `json:"output"`
	RanAt    time.Time `json:"ranAt"`
}
