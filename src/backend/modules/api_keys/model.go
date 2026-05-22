// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package api_keys

// APIKey represents an API key (without the secret).
type APIKey struct {
	ID         string   `json:"id"`
	UserID     string   `json:"userId"`
	Username   string   `json:"username"`
	Name       string   `json:"name"`
	KeyPrefix  string   `json:"keyPrefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  *string  `json:"expiresAt"`
	LastUsedAt *string  `json:"lastUsedAt"`
	IsActive   bool     `json:"isActive"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
}

// CreateAPIKeyResult includes the plaintext key (shown once).
type CreateAPIKeyResult struct {
	APIKey
	Key string `json:"key"`
}

// CreateAPIKeyInput is the request body for creating an API key.
type CreateAPIKeyInput struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expiresAt"`
}
