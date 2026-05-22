// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package groups

// Group represents a named user group.
type Group struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	IsSystem    bool          `json:"isSystem"`
	MemberCount int           `json:"memberCount"`
	Members     []GroupMember `json:"members,omitempty"`
	Roles       []GroupRole   `json:"roles,omitempty"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
}

// GroupMember represents a user in a group.
type GroupMember struct {
	ID       string `json:"id"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// GroupRole represents a role assignment for a group.
type GroupRole struct {
	ID              string  `json:"id"`
	RoleID          string  `json:"roleId"`
	RoleName        string  `json:"roleName"`
	EnvironmentID   *string `json:"environmentId"`
	EnvironmentName *string `json:"environmentName"`
	StackName       *string `json:"stackName"`
}

// CreateGroupInput is the request body for creating a group.
type CreateGroupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateGroupInput is the request body for updating a group.
type UpdateGroupInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// AddMemberInput is the request body for adding a member.
type AddMemberInput struct {
	UserID string `json:"userId"`
}

// AssignRoleInput is the request body for assigning a role.
type AssignRoleInput struct {
	RoleID        string  `json:"roleId"`
	EnvironmentID *string `json:"environmentId"`
	StackName     *string `json:"stackName"`
}
