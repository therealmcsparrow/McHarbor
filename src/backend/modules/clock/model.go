// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package clock watches the UTC system clock against a reference URL and raises an audit event and notification when drift exceeds the configured threshold.
package clock

// Status is the latest clock drift verdict served to the settings UI and mirrored in audit entries.
type Status struct {
	Status                  string `json:"status"`
	Source                  string `json:"source"`
	SystemTimeUTC           string `json:"systemTimeUtc"`
	ReferenceTimeUTC        string `json:"referenceTimeUtc,omitempty"`
	DeltaMilliseconds       int64  `json:"deltaMilliseconds"`
	ThresholdSeconds        int    `json:"thresholdSeconds"`
	CheckedAt               string `json:"checkedAt"`
	LastError               string `json:"lastError,omitempty"`
	BootRTCDate             string `json:"bootRtcDate,omitempty"`
	NextScheduledCheckAfter string `json:"nextScheduledCheckAfter,omitempty"`
}
