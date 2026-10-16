// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package roles

// Role represents a role with its permissions.
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	IsSystem    bool     `json:"isSystem"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	// UserCount, GroupCount and APIKeyCount are populated only by List, from
	// a COUNT(DISTINCT ...) join, so the table can show a role's blast
	// radius without a second round trip per row. Pointers so a genuinely
	// unused role's real zero still serializes; nil (omitted) elsewhere,
	// Get included.
	UserCount   *int `json:"userCount,omitempty"`
	GroupCount  *int `json:"groupCount,omitempty"`
	APIKeyCount *int `json:"apiKeyCount,omitempty"`
}

// CreateRoleInput is the request body for creating a role.
type CreateRoleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// UpdateRoleInput is the request body for updating a role.
type UpdateRoleInput struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
}
