// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package elevations

// ElevationRequest represents one row of the elevation_requests table.
type ElevationRequest struct {
	ID               string  `json:"id"`
	RequesterID      string  `json:"requesterId"`
	PermissionKey    string  `json:"permissionKey"`
	PermissionLabel  string  `json:"permissionLabel"`
	EnvironmentID    *string `json:"environmentId,omitempty"`
	ResourceType     *string `json:"resourceType,omitempty"`
	ResourceID       *string `json:"resourceId,omitempty"`
	Justification    string  `json:"justification"`
	RequestedMinutes int     `json:"requestedMinutes"`
	Status           string  `json:"status"`
	ApproverID       *string `json:"approverId,omitempty"`
	ApproverNote     *string `json:"approverNote,omitempty"`
	ApprovedUntil    *string `json:"approvedUntil,omitempty"`
	DecidedAt        *string `json:"decidedAt,omitempty"`
	CreatedAt        string  `json:"createdAt"`
}

// CreateRequest is the body for POST /api/elevations.
type CreateRequest struct {
	PermissionKey    string  `json:"permissionKey"`
	EnvironmentID    *string `json:"environmentId,omitempty"`
	ResourceType     *string `json:"resourceType,omitempty"`
	ResourceID       *string `json:"resourceId,omitempty"`
	Justification    string  `json:"justification"`
	RequestedMinutes int     `json:"requestedMinutes"`
}

// DecideRequest is the body for POST /api/elevations/{id}/decide.
type DecideRequest struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note"`
}

// SetElevatableRequest is the body for PUT /api/elevations/elevatable.
type SetElevatableRequest struct {
	Key        string `json:"key"`
	Elevatable bool   `json:"elevatable"`
}

// ElevationStatus enumerates the lifecycle of an elevation request.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusDenied   = "denied"
	StatusExpired  = "expired"
)
