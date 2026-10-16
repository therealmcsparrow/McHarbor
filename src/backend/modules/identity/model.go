// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

// Provider represents an external identity provider configuration.
type Provider struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ProviderType  string  `json:"providerType"`
	Enabled       bool    `json:"enabled"`
	ClientID      string  `json:"clientId"`
	ClientSecret  string  `json:"-"`
	TenantID      *string `json:"tenantId,omitempty"`
	Domain        *string `json:"domain,omitempty"`
	IssuerURL     *string `json:"issuerUrl,omitempty"`
	MetadataURL   *string `json:"metadataUrl,omitempty"`
	EntityID      *string `json:"entityId,omitempty"`
	Scopes        string  `json:"scopes"`
	AutoProvision bool    `json:"autoProvision"`
	DefaultRoleID *string `json:"defaultRoleId,omitempty"`
	// DefaultRoleName is populated only by List, from a LEFT JOIN on
	// DefaultRoleID, so the table can show the role's name without a
	// second lookup per row.
	DefaultRoleName     *string        `json:"defaultRoleName,omitempty"`
	GroupMappingEnabled bool           `json:"groupMappingEnabled"`
	GroupMappings       []GroupMapping `json:"groupMappings"`
	AutoImportGroups    bool           `json:"autoImportGroups"`

	// Config is the plugin-owned field map persisted in the
	// identity_providers.config JSON column. It is what the edit
	// dialog seeds the plugin's ConfigForm from, so it is REDACTED
	// on the way out: every value the write path encrypted is
	// dropped here rather than decrypted. See redactConfigValues.
	// Never populate this from providerToConfigMap, which decrypts.
	Config map[string]string `json:"config,omitempty"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
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
//
// `Config` is the plugin-defined free-form map written by the
// identity-provider plugin's React ConfigForm. Hosts (the dialog,
// CLI, API) hand the map straight through; Service.Create unpacks
// known keys (client_id, client_secret, tenant_id, domain,
// issuer_url, metadata_url, entity_id, scopes, auto_provision,
// auto_import_groups, group_mapping_enabled, group_mappings) into
// typed columns and stores the rest as-is. Plugins read the map via
// Service.providerToConfigMap — keeping the wire shape uniform
// across host + plugin.
type CreateProviderInput struct {
	Name         string            `json:"name"`
	ProviderType string            `json:"providerType"`
	Config       map[string]string `json:"config,omitempty"`

	// Typed fields are kept for backwards compatibility with the
	// pre-port API. New callers should pass Config; Service.Create
	// prefers Config keys when present and falls back to these.
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
// Same wire shape as CreateProviderInput — typed fields retained
// for backwards compatibility; `Config` overrides when present.
type UpdateProviderInput struct {
	Name    *string           `json:"name,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
	Config  map[string]string `json:"config,omitempty"`

	ClientID            *string        `json:"clientId,omitempty"`
	ClientSecret        *string        `json:"clientSecret,omitempty"`
	TenantID            *string        `json:"tenantId,omitempty"`
	Domain              *string        `json:"domain,omitempty"`
	IssuerURL           *string        `json:"issuerUrl,omitempty"`
	MetadataURL         *string        `json:"metadataUrl,omitempty"`
	EntityID            *string        `json:"entityId,omitempty"`
	Scopes              *string        `json:"scopes,omitempty"`
	AutoProvision       *bool          `json:"autoProvision,omitempty"`
	DefaultRoleID       *string        `json:"defaultRoleID,omitempty"`
	GroupMappingEnabled *bool          `json:"groupMappingEnabled,omitempty"`
	GroupMappings       []GroupMapping `json:"groupMappings,omitempty"`
	AutoImportGroups    *bool          `json:"autoImportGroups,omitempty"`
}
