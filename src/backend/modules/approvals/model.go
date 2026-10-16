// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package approvals holds destructive requests until someone other than the
// requester approves them, then replays the stored request exactly once.
package approvals

// Request statuses. A request moves pending → approved|denied|expired|cancelled,
// and approved → executing → executed; nothing ever moves back.
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusDenied    = "denied"
	StatusExpired   = "expired"
	StatusCancelled = "cancelled"
	StatusExecuting = "executing"
	StatusExecuted  = "executed"
)

// ReasonApprovalRequired is the machine reason the gate puts in data.reason, so
// a client can tell "held for approval" from a refusal without a new message code.
const ReasonApprovalRequired = "approval_required"

// Rule says that one risk category needs approval, by how many people, holding
// which permission, and how long a request stays decidable.
type Rule struct {
	ID                 string  `json:"id"`
	Category           string  `json:"category"`
	EnvironmentID      *string `json:"environmentId"`
	Enabled            bool    `json:"enabled"`
	RequiredApprovers  int     `json:"requiredApprovers"`
	ApproverPermission string  `json:"approverPermission"`
	ExpiryMinutes      int     `json:"expiryMinutes"`
	Description        string  `json:"description"`
	CreatedBy          string  `json:"createdBy"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

// RuleInput is the body for creating or replacing a rule.
type RuleInput struct {
	Category           string  `json:"category"`
	EnvironmentID      *string `json:"environmentId"`
	Enabled            *bool   `json:"enabled"`
	RequiredApprovers  int     `json:"requiredApprovers"`
	ApproverPermission string  `json:"approverPermission"`
	ExpiryMinutes      int     `json:"expiryMinutes"`
	Description        string  `json:"description"`
}

// Decision is one approver's vote on a request.
type Decision struct {
	ID           string `json:"id"`
	ApproverID   string `json:"approverId"`
	ApproverName string `json:"approverName"`
	Decision     string `json:"decision"`
	Note         string `json:"note"`
	CreatedAt    string `json:"createdAt"`
}

// Request is a held destructive call. BodyRedacted is the canonical snapshot
// shown to approvers; the exact body is kept encrypted and never serialised.
type Request struct {
	ID                 string     `json:"id"`
	RuleID             string     `json:"ruleId"`
	Category           string     `json:"category"`
	RequesterID        string     `json:"requesterId"`
	RequesterName      string     `json:"requesterName"`
	EnvironmentID      *string    `json:"environmentId"`
	Method             string     `json:"method"`
	Path               string     `json:"path"`
	RoutePattern       string     `json:"routePattern"`
	ResourceName       string     `json:"resourceName"`
	BodyRedacted       string     `json:"bodyRedacted"`
	Status             string     `json:"status"`
	RequiredApprovers  int        `json:"requiredApprovers"`
	ApprovalsCount     int        `json:"approvalsCount"`
	ApproverPermission string     `json:"approverPermission"`
	ExpiresAt          string     `json:"expiresAt"`
	DecidedAt          *string    `json:"decidedAt"`
	ExecutedAt         *string    `json:"executedAt"`
	ResultStatus       *int       `json:"resultStatus"`
	ResultCode         string     `json:"resultCode"`
	CreatedAt          string     `json:"createdAt"`
	UpdatedAt          string     `json:"updatedAt"`
	Decisions          []Decision `json:"decisions"`

	bodyEncrypted string
	apiKeyID      string
}

// DecideInput is the body of POST /approvals/{id}/decide.
type DecideInput struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note"`
}

// HeldResponse is the data payload of the 202 the gate answers with.
type HeldResponse struct {
	Reason            string `json:"reason"`
	ApprovalID        string `json:"approvalId"`
	Category          string `json:"category"`
	RequiredApprovers int    `json:"requiredApprovers"`
	ExpiresAt         string `json:"expiresAt"`
}

// ExecuteResult is what POST /approvals/{id}/execute returns: the request as it
// now stands and the status and body the replayed route answered with.
type ExecuteResult struct {
	Approval     *Request `json:"approval"`
	ResultStatus int      `json:"resultStatus"`
	Response     any      `json:"response,omitempty"`
}

// RouteInfo is one method and path a category covers, as the builder previews it.
type RouteInfo struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
}

// CategoryInfo describes one risk category to the rules builder, including the
// routes it matches so an admin can see what a rule will hold before saving it.
type CategoryInfo struct {
	Key         string      `json:"key"`
	Routes      []RouteInfo `json:"routes"`
	Permissions []string    `json:"permissions"`
}
