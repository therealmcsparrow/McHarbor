// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

// Export statuses.
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Export is one row of the export history. The manifest is the plaintext one
// written into the artifact, so the list can say what an export holds without a key.
type Export struct {
	ID             string         `json:"id"`
	Status         string         `json:"status"`
	Trigger        string         `json:"trigger"`
	ScheduleID     *string        `json:"scheduleId,omitempty"`
	CreatedBy      string         `json:"createdBy"`
	FileName       string         `json:"fileName"`
	SizeBytes      int64          `json:"sizeBytes"`
	Manifest       *core.Manifest `json:"manifest,omitempty"`
	KeyKind        string         `json:"keyKind"`
	KeyID          string         `json:"keyId"`
	IncludeMetrics bool           `json:"includeMetrics"`
	Note           string         `json:"note"`
	Error          string         `json:"error"`
	// Destinations is where this export was actually sent, one row per place,
	// each with its own outcome.
	Destinations []ExportDestination `json:"destinations,omitempty"`
	// LocationID, LocationKey, LocationStatus and LocationError are the
	// single-destination summary kept for readers older than migration 123 — a
	// restore of a pre-migration backup and the reporting plugin's readiness
	// report both still select them. Destinations is the truth.
	LocationID     *string        `json:"locationId,omitempty"`
	LocationKey    string         `json:"locationKey,omitempty"`
	LocationStatus string         `json:"locationStatus"`
	LocationError  string         `json:"locationError"`
	VerifyStatus   string         `json:"verifyStatus"`
	VerifyError    string         `json:"verifyError"`
	VerifiedAt     *string        `json:"verifiedAt,omitempty"`
	CompletedAt    *string        `json:"completedAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	Progress       *core.Progress `json:"progress,omitempty"`
}

// CreateExportInput starts a manual export. Exactly one of Passphrase and
// KeyFile is set; the key is used once and never stored.
type CreateExportInput struct {
	Passphrase     string `json:"passphrase,omitempty"`
	KeyFile        string `json:"keyFile,omitempty"`
	IncludeMetrics bool   `json:"includeMetrics"`
	Note           string `json:"note,omitempty"`
	// LocationIDs copies the finished export to every storage location named,
	// the way a container backup plan does. Empty keeps it on this host only.
	//
	// The copies happen after the local file is complete and verified-by-rename,
	// never instead of it: an offsite copy that is the only copy is worse than
	// no offsite copy, because a failed upload would then lose the backup. Every
	// destination is attempted even after one fails, because holding two is
	// precisely a hedge against one being unreachable.
	LocationIDs []string `json:"locationIds,omitempty"`
}

// KeyFileResult is a freshly generated key file, shown once.
type KeyFileResult struct {
	KeyFile  string `json:"keyFile"`
	FileName string `json:"fileName"`
}

// InvalidReason is the machine reason attached to a refused request, which
// the interface turns into a sentence in the operator's language.
type InvalidReason struct {
	Reason string `json:"reason"`
}
