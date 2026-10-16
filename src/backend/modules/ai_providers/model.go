// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_providers

// AuthMethod values for AIProviderConnection.AuthMethod. A connection
// authenticates either with a pasted API key or with an OAuth device-flow
// login; both are first-class and selectable per connection.
const (
	AuthMethodAPIKey = "api_key"
	AuthMethodOAuth  = "oauth"
)

// AIProviderConnection represents one user-named AI provider instance.
// Mirrors the storage_locations / communication_channels pattern: the
// host owns a typed CRUD table for named connections, and the plugin
// runtime stays the source of truth for which provider_types are
// available.
//
// Secret fields are deliberately excluded from the JSON output shape
// (`apiKey` carries no JSON tag, and no OAuth token field exists here at
// all). The read path returns booleans and the account label; the
// service-layer test and chat dispatch decrypt on demand via
// Service.PlaintextConfig, which is the only path that ever produces
// a decrypted value.
type AIProviderConnection struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"providerType"`
	IsDefault    bool   `json:"isDefault"`
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"baseUrl,omitempty"`
	DefaultModel string `json:"defaultModel,omitempty"`
	HasAPIKey    bool   `json:"hasApiKey"`
	APIKey       string `json:"-"` // never serialized

	// AuthMethod is AuthMethodAPIKey or AuthMethodOAuth. Rows written
	// before the OAuth flow existed read back as AuthMethodAPIKey.
	AuthMethod string `json:"authMethod"`

	// OAuth carries the non-secret half of the device-flow config plus
	// the current connection state. Nil for API-key connections.
	OAuth *AIProviderOAuth `json:"oauth,omitempty"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// AIProviderOAuth is the client-visible view of a connection's OAuth
// device-flow configuration and state. The client ID and the endpoint
// URLs are not secrets (they travel in the user's own browser during a
// normal OAuth login), so they round-trip through the form; the client
// secret and both tokens never leave the backend.
type AIProviderOAuth struct {
	ClientID  string `json:"clientId,omitempty"`
	DeviceURL string `json:"deviceUrl,omitempty"`
	TokenURL  string `json:"tokenUrl,omitempty"`
	Scopes    string `json:"scopes,omitempty"`

	// HasClientSecret reports whether a secret is stored, without
	// revealing it — the same "leave blank to keep" convention the
	// API key field uses.
	HasClientSecret bool `json:"hasClientSecret"`

	// Connected is true once a device flow has produced an access token.
	Connected    bool   `json:"connected"`
	AccountLabel string `json:"accountLabel,omitempty"`
	ExpiresAt    string `json:"expiresAt,omitempty"`

	// Pending describes an in-flight device flow the operator has not
	// finished approving yet. The device code itself stays server-side;
	// only the user code and verification URI are shown.
	Pending          bool   `json:"pending"`
	UserCode         string `json:"userCode,omitempty"`
	VerificationURI  string `json:"verificationUri,omitempty"`
	PendingExpiresAt string `json:"pendingExpiresAt,omitempty"`
	IntervalSeconds  int    `json:"intervalSeconds,omitempty"`
}

// AIProviderOAuthDefault is one entry in the map returned by
// GET /api/ai-providers/oauth-defaults. It carries the non-secret
// device-flow config the create form prefills when the operator picks a
// provider type: the host's published endpoint URLs, overlaid with any
// client ID + endpoints a plugin declares via AIAdapterSpec.OAuthDefaults.
//
// PluginProvided is true when the merged config is complete on its own
// (client ID + both endpoints present) — the operator then only clicks
// Sign in, and the form hides the config fields. No client secret is
// ever included; a confidential client's secret lives in plugin_config.
type AIProviderOAuthDefault struct {
	ClientID       string `json:"clientId,omitempty"`
	DeviceURL      string `json:"deviceUrl,omitempty"`
	TokenURL       string `json:"tokenUrl,omitempty"`
	Scopes         string `json:"scopes,omitempty"`
	PluginProvided bool   `json:"pluginProvided"`
}

// AIProviderOAuthInput is the writable half of the device-flow config.
// Used by both create and update; an empty ClientSecret means "do not
// touch the stored value", matching the API key convention.
type AIProviderOAuthInput struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	DeviceURL    string `json:"deviceUrl"`
	TokenURL     string `json:"tokenUrl"`
	Scopes       string `json:"scopes"`
}

// CreateAIProviderInput is the request body for POST /api/ai-providers.
// Carries the plaintext API key on the way in; the service encrypts it
// before insert. An OAuth connection is created with no credential at
// all — the operator signs in afterwards through the oauth/begin +
// oauth/poll pair, which is what stores the token.
type CreateAIProviderInput struct {
	Name         string                `json:"name"`
	ProviderType string                `json:"providerType"`
	IsDefault    bool                  `json:"isDefault"`
	Enabled      *bool                 `json:"enabled"`
	BaseURL      string                `json:"baseUrl"`
	DefaultModel string                `json:"defaultModel"`
	APIKey       string                `json:"apiKey"`
	AuthMethod   string                `json:"authMethod"`
	OAuth        *AIProviderOAuthInput `json:"oauth"`
}

// UpdateAIProviderInput is the request body for PUT /api/ai-providers/{id}.
// Pointer fields enable partial updates. Empty-string `*APIKey` means
// "do not touch the stored value" (same convention as storage_locations).
type UpdateAIProviderInput struct {
	Name         *string               `json:"name"`
	IsDefault    *bool                 `json:"isDefault"`
	Enabled      *bool                 `json:"enabled"`
	BaseURL      *string               `json:"baseUrl"`
	DefaultModel *string               `json:"defaultModel"`
	APIKey       *string               `json:"apiKey"`
	AuthMethod   *string               `json:"authMethod"`
	OAuth        *AIProviderOAuthInput `json:"oauth"`
}
