// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

// IdentityProvider represents an external identity provider configuration.
type IdentityProvider struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	ProviderType        string         `json:"providerType"`
	Enabled             bool           `json:"enabled"`
	ClientID            string         `json:"clientId"`
	ClientSecret        string         `json:"-"`
	TenantID            *string        `json:"tenantId,omitempty"`
	Domain              *string        `json:"domain,omitempty"`
	IssuerURL           *string        `json:"issuerUrl,omitempty"`
	MetadataURL         *string        `json:"metadataUrl,omitempty"`
	EntityID            *string        `json:"entityId,omitempty"`
	Scopes              string         `json:"scopes"`
	AutoProvision       bool           `json:"autoProvision"`
	DefaultRoleID       *string        `json:"defaultRoleId,omitempty"`
	GroupMappingEnabled bool           `json:"groupMappingEnabled"`
	GroupMappings       []GroupMapping `json:"groupMappings"`
	AutoImportGroups    bool           `json:"autoImportGroups"`
	CreatedAt           string         `json:"createdAt"`
	UpdatedAt           string         `json:"updatedAt"`
}

// EnabledProvider is the minimal info returned on the public endpoint.
type EnabledProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"providerType"`
}

// GroupMapping maps a provider group to a McHarbor group.
type GroupMapping struct {
	ProviderGroup   string `json:"providerGroup"`
	McharborGroupID string `json:"mcharborGroupId"`
}

// ProviderGroupInfo represents a group fetched from the identity provider API.
type ProviderGroupInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateProviderInput is the request body for creating a provider.
type CreateProviderInput struct {
	Name                string         `json:"name"`
	ProviderType        string         `json:"providerType"`
	ClientID            string         `json:"clientId"`
	ClientSecret        string         `json:"clientSecret"`
	TenantID            *string        `json:"tenantId,omitempty"`
	Domain              *string        `json:"domain,omitempty"`
	IssuerURL           *string        `json:"issuerUrl,omitempty"`
	MetadataURL         *string        `json:"metadataUrl,omitempty"`
	EntityID            *string        `json:"entityId,omitempty"`
	Scopes              *string        `json:"scopes,omitempty"`
	AutoProvision       *bool          `json:"autoProvision,omitempty"`
	DefaultRoleID       *string        `json:"defaultRoleId,omitempty"`
	GroupMappingEnabled *bool          `json:"groupMappingEnabled,omitempty"`
	GroupMappings       []GroupMapping `json:"groupMappings,omitempty"`
	AutoImportGroups    *bool          `json:"autoImportGroups,omitempty"`
}

// UpdateProviderInput is the request body for updating a provider.
type UpdateProviderInput struct {
	Name                *string        `json:"name,omitempty"`
	Enabled             *bool          `json:"enabled,omitempty"`
	ClientID            *string        `json:"clientId,omitempty"`
	ClientSecret        *string        `json:"clientSecret,omitempty"`
	TenantID            *string        `json:"tenantId,omitempty"`
	Domain              *string        `json:"domain,omitempty"`
	IssuerURL           *string        `json:"issuerUrl,omitempty"`
	MetadataURL         *string        `json:"metadataUrl,omitempty"`
	EntityID            *string        `json:"entityId,omitempty"`
	Scopes              *string        `json:"scopes,omitempty"`
	AutoProvision       *bool          `json:"autoProvision,omitempty"`
	DefaultRoleID       *string        `json:"defaultRoleId,omitempty"`
	GroupMappingEnabled *bool          `json:"groupMappingEnabled,omitempty"`
	GroupMappings       []GroupMapping `json:"groupMappings,omitempty"`
	AutoImportGroups    *bool          `json:"autoImportGroups,omitempty"`
}
