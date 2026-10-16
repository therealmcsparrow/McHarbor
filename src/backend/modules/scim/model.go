// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

// EndpointInput is the request body for POST /api/scim/endpoints and
// PUT /api/scim/endpoints/{id}. Bearer tokens are accepted on create
// and on update (if non-empty). On subsequent GETs the token is
// replaced with `hasToken: true` so the client knows a token exists
// without seeing the plaintext.
type EndpointInput struct {
	Name         string `json:"name"`
	BaseURL      string `json:"baseUrl"`
	AuthType     string `json:"authType"`
	BearerToken  string `json:"bearerToken,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	PollSchedule string `json:"pollSchedule"`
	Enabled      bool   `json:"enabled"`
}

// Endpoint is the outward model returned to the admin UI. The token
// field is intentionally absent — admins only learn `hasToken` so we
// never echo back the secret. On creation, the response includes the
// newly generated token exactly once.
type Endpoint struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	BaseURL        string `json:"baseUrl"`
	AuthType       string `json:"authType"`
	PollSchedule   string `json:"pollSchedule"`
	Enabled        bool   `json:"enabled"`
	HasToken       bool   `json:"hasToken"`
	Username       string `json:"username,omitempty"`
	LastSyncAt     string `json:"lastSyncAt,omitempty"`
	LastSyncStatus string `json:"lastSyncStatus"`
	LastSyncError  string `json:"lastSyncError"`
	UsersCount     int    `json:"usersCount"`
	GroupsCount    int    `json:"groupsCount"`
	TokenRotatedAt string `json:"tokenRotatedAt,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// SyncLogEntry mirrors a row of scim_sync_log. UsersCreated counts new mapping
// rows (linked plus unmatched); Notes lists accounts a run deliberately skipped.
type SyncLogEntry struct {
	ID             string   `json:"id"`
	EndpointID     string   `json:"endpointId"`
	Direction      string   `json:"direction"`
	StartedAt      string   `json:"startedAt"`
	FinishedAt     string   `json:"finishedAt,omitempty"`
	Status         string   `json:"status"`
	UsersCreated   int      `json:"usersCreated"`
	UsersUpdated   int      `json:"usersUpdated"`
	UsersDeleted   int      `json:"usersDeleted"`
	GroupsCreated  int      `json:"groupsCreated"`
	GroupsUpdated  int      `json:"groupsUpdated"`
	GroupsDeleted  int      `json:"groupsDeleted"`
	Error          string   `json:"error"`
	UsersSeen      int      `json:"usersSeen"`
	UsersLinked    int      `json:"usersLinked"`
	UsersUnmatched int      `json:"usersUnmatched"`
	UsersDisabled  int      `json:"usersDisabled"`
	UsersSkipped   int      `json:"usersSkipped"`
	GroupsSeen     int      `json:"groupsSeen"`
	DryRun         bool     `json:"dryRun"`
	Trigger        string   `json:"trigger"`
	Notes          []string `json:"notes"`
}

// DryRunResult is the answer to a dry-run request: the plan, or the reason the
// directory could not be read.
type DryRunResult struct {
	Status string    `json:"status"`
	Error  string    `json:"error,omitempty"`
	Plan   *SyncPlan `json:"plan,omitempty"`
}

// RotatedToken carries a rotated bearer token, shown exactly once. The name
// avoids "…Response", which would collide with the generated client's wrapper
// type for the scimRotateToken operation and stop the CLI compiling.
type RotatedToken struct {
	Endpoint    *Endpoint `json:"endpoint"`
	BearerToken string    `json:"bearerToken"`
}

// SCIM protocol JSON shapes. Kept minimal — only the fields an IdP
// needs to interoperate are populated. Additional attributes can be
// added later without breaking existing clients.

// ListResponse is the wrapper SCIM 2.0 uses for collection GETs.
type ListResponse[T any] struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	ItemsPerPage int      `json:"itemsPerPage"`
	StartIndex   int      `json:"startIndex"`
	Resources    []T      `json:"Resources"`
}

// User is the SCIM 2.0 user representation.
type User struct {
	Schemas  []string `json:"schemas"`
	ID       string   `json:"id"`
	UserName string   `json:"userName"`
	Name     *Name    `json:"name,omitempty"`
	Emails   []Email  `json:"emails,omitempty"`
	Active   bool     `json:"active"`
	Meta     *Meta    `json:"meta,omitempty"`
}

// Name carries given/family name components.
type Name struct {
	GivenName  string `json:"givenName"`
	FamilyName string `json:"familyName"`
}

// Email is a single email entry.
type Email struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary"`
	Type    string `json:"type,omitempty"`
}

// Meta is the RFC 7643 resource metadata block.
type Meta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created"`
	LastModified string `json:"lastModified"`
	Location     string `json:"location"`
	Version      string `json:"version,omitempty"`
}

// ProviderConfig is the discovery document.
type ProviderConfig struct {
	Schemas               []string               `json:"schemas"`
	Patch                 Supported              `json:"patch"`
	Bulk                  BulkSupport            `json:"bulk"`
	Filter                Supported              `json:"filter"`
	ChangePassword        Supported              `json:"changePassword"`
	Sort                  Supported              `json:"sort"`
	ETag                  Supported              `json:"etag"`
	AuthenticationSchemes []AuthenticationScheme `json:"authenticationSchemes"`
}

// Supported is the supported boolean for optional features.
type Supported struct {
	Supported bool `json:"supported"`
}

// BulkSupport adds maxOperations and maxPayloadSize.
type BulkSupport struct {
	Supported
	MaxOperations  int `json:"maxOperations"`
	MaxPayloadSize int `json:"maxPayloadSize"`
}

// AuthenticationScheme is one entry in the auth scheme list.
type AuthenticationScheme struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	SpecURI          string `json:"specUri"`
	DocumentationURI string `json:"documentationUri"`
}
