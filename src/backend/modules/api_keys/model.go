// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

// APIKeyRole is a role assigned to a key, as shown in the UI.
type APIKeyRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// APIKey represents an API key (without the secret).
type APIKey struct {
	ID             string       `json:"id"`
	UserID         string       `json:"userId"`
	Username       string       `json:"username"`
	Name           string       `json:"name"`
	KeyPrefix      string       `json:"keyPrefix"`
	Scopes         []string     `json:"scopes"`
	Roles          []APIKeyRole `json:"roles"`
	EnvironmentIDs []string     `json:"environmentIds"`
	// EnvironmentNames carries the names of EnvironmentIDs, so a table
	// column can show them without a client-side join. Not index-aligned
	// with EnvironmentIDs: a tombstone's archived snapshot skips a blank
	// name rather than emit an empty string at that position.
	EnvironmentNames []string `json:"environmentNames"`
	ExpiresAt        *string  `json:"expiresAt"`
	LastUsedAt       *string  `json:"lastUsedAt"`
	IsActive         bool     `json:"isActive"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`

	// Status collapses is_active, expiry and deletion into the one word
	// the table column shows: active, expired, revoked or deleted. The
	// raw fields stay because the form still needs them.
	Status string `json:"status"`
	// DeletedAt is set only for a tombstone — a key whose credential has
	// been destroyed but whose record is retained until retention expires.
	DeletedAt *string `json:"deletedAt"`

	// AgeDays, RotationDue and ExpiringSoon are the rotation reminders. They
	// are computed per list request against the thresholds it asked for, and
	// never stored, so changing a threshold re-flags every key at once.
	AgeDays      int  `json:"ageDays"`
	RotationDue  bool `json:"rotationDue"`
	ExpiringSoon bool `json:"expiringSoon"`
}

// CreateAPIKeyResult includes the plaintext key (shown once).
type CreateAPIKeyResult struct {
	APIKey
	Key string `json:"key"`
}

// CreateAPIKeyInput is the request body for creating an API key.
type CreateAPIKeyInput struct {
	Name           string   `json:"name"`
	RoleIDs        []string `json:"roleIds"`
	Scopes         []string `json:"scopes"`
	EnvironmentIDs []string `json:"environmentIds"`
	ExpiresAt      *string  `json:"expiresAt"`
}

// UpdateAPIKeyInput is the request body for PATCH. A nil slice leaves that
// grant list untouched; an empty slice clears it.
type UpdateAPIKeyInput struct {
	Name           *string  `json:"name"`
	RoleIDs        []string `json:"roleIds"`
	Scopes         []string `json:"scopes"`
	EnvironmentIDs []string `json:"environmentIds"`
}

// GrantOption is one role the caller may (or may not) put on a key.
type GrantOption struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Grantable   bool     `json:"grantable"`
}

// EnvironmentOption is one environment a key may be fenced to.
type EnvironmentOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GrantOptions is what the key form needs: every role with a grantable
// flag, every environment, and the permission keys the caller may grant
// explicitly. Computed server-side so the disabled state in the UI is
// exactly the rule the backend enforces.
type GrantOptions struct {
	Roles        []GrantOption       `json:"roles"`
	Environments []EnvironmentOption `json:"environments"`
	Permissions  []string            `json:"permissions"`
	Wildcard     bool                `json:"wildcard"`
}
