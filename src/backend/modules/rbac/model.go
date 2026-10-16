// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

// PolicyInput is the request body for POST /api/rbac/policies and
// PUT /api/rbac/policies/{id}. The handler maps this onto the
// rbac.PolicyService inputs.
type PolicyInput struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Effect       string   `json:"effect"`
	ResourceType string   `json:"resourceType"`
	Expression   string   `json:"expression"`
	Priority     int      `json:"priority"`
	Enabled      bool     `json:"enabled"`
	Permissions  []string `json:"permissions"`
}

// TestInput is the body for POST /api/rbac/policies/{id}/test.
// Admins provide a sample attribute bundle to dry-run the expression.
type TestInput struct {
	Attributes *AttributesDTO `json:"attributes"`
}

// AttributesDTO mirrors core/rbac.Attributes with snake_case-friendly
// JSON so admins can paste it from curl. The handler maps it onto
// the rbac.Attributes struct.
type AttributesDTO struct {
	User     *UserDTO     `json:"user"`
	Env      *EnvDTO      `json:"env"`
	Resource *ResourceDTO `json:"resource"`
	Now      *TimeDTO     `json:"now"`
}

// UserDTO carries test-mode user attributes.
type UserDTO struct {
	ID       string            `json:"id"`
	Username string            `json:"username"`
	Email    string            `json:"email"`
	Roles    []string          `json:"roles"`
	Groups   []string          `json:"groups"`
	Labels   map[string]string `json:"labels"`
}

// EnvDTO carries test-mode env attributes.
type EnvDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResourceDTO carries test-mode resource attributes.
type ResourceDTO struct {
	Type   string            `json:"type"`
	ID     string            `json:"id"`
	Owner  string            `json:"owner"`
	Image  string            `json:"image"`
	State  string            `json:"state"`
	Labels map[string]string `json:"labels"`
}

// TimeDTO is the test-mode frozen clock.
type TimeDTO struct {
	Year    int `json:"year"`
	Month   int `json:"month"`
	Day     int `json:"day"`
	Hour    int `json:"hour"`
	Weekday int `json:"weekday"`
}

// TestResult is the response from POST /api/rbac/policies/{id}/test.
type TestResult struct {
	Matched bool   `json:"matched"`
	Reason  string `json:"reason"`
	Error   string `json:"error,omitempty"`
}

// UserLabelsInput is the body for PUT /api/rbac/users/{id}/labels.
// Admins use this to assign the per-user labels that the team-ownership
// seed policy matches against container labels.
type UserLabelsInput struct {
	Labels map[string]string `json:"labels"`
}
