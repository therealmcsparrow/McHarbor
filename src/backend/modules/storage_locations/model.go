// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

// StorageLocation represents a reusable external storage location.
// Credential fields are encrypted at rest and are never returned in API responses.
type StorageLocation struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	LocationType string `json:"locationType"`
	Enabled      bool   `json:"enabled"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	BasePath     string `json:"basePath,omitempty"`
	Region       string `json:"region,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	TenantID     string `json:"tenantId,omitempty"`
	SiteURL      string `json:"siteUrl,omitempty"`
	DriveID      string `json:"driveId,omitempty"`
	ShareName    string `json:"shareName,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Username     string `json:"username,omitempty"`
	AuthMethod   string `json:"authMethod,omitempty"`
	TLSMode      string `json:"tlsMode,omitempty"`
	PassiveMode  bool   `json:"passiveMode"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// CreateStorageLocationInput is the request body for creating a storage location.
type CreateStorageLocationInput struct {
	Name            string `json:"name"`
	LocationType    string `json:"locationType"`
	Enabled         *bool  `json:"enabled"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	BasePath        string `json:"basePath"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Endpoint        string `json:"endpoint"`
	TenantID        string `json:"tenantId"`
	SiteURL         string `json:"siteUrl"`
	DriveID         string `json:"driveId"`
	ShareName       string `json:"shareName"`
	Domain          string `json:"domain"`
	Username        string `json:"username"`
	AuthMethod      string `json:"authMethod"`
	TLSMode         string `json:"tlsMode"`
	PassiveMode     bool   `json:"passiveMode"`
	Password        string `json:"password"`
	PrivateKey      string `json:"privateKey"`
	Passphrase      string `json:"passphrase"`
	CACertificate   string `json:"caCertificate"`
	ClientCert      string `json:"clientCertificate"`
	ClientKey       string `json:"clientKey"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	ClientID        string `json:"clientId"`
	ClientSecret    string `json:"clientSecret"`
	RefreshToken    string `json:"refreshToken"`
	Token           string `json:"token"`
	// Config carries plugin-owned fields the plugin's ConfigForm
	// emits under its own (snake_case) keys. Persisted encrypted in
	// storage_locations.config; well-known keys are also mirrored
	// into the first-class columns above.
	Config map[string]string `json:"config,omitempty"`
}

// UpdateStorageLocationInput is the request body for updating a storage location.
type UpdateStorageLocationInput struct {
	Name            *string `json:"name"`
	LocationType    *string `json:"locationType"`
	Enabled         *bool   `json:"enabled"`
	Host            *string `json:"host"`
	Port            *int    `json:"port"`
	BasePath        *string `json:"basePath"`
	Region          *string `json:"region"`
	Bucket          *string `json:"bucket"`
	Endpoint        *string `json:"endpoint"`
	TenantID        *string `json:"tenantId"`
	SiteURL         *string `json:"siteUrl"`
	DriveID         *string `json:"driveId"`
	ShareName       *string `json:"shareName"`
	Domain          *string `json:"domain"`
	Username        *string `json:"username"`
	AuthMethod      *string `json:"authMethod"`
	TLSMode         *string `json:"tlsMode"`
	PassiveMode     *bool   `json:"passiveMode"`
	Password        *string `json:"password"`
	PrivateKey      *string `json:"privateKey"`
	Passphrase      *string `json:"passphrase"`
	CACertificate   *string `json:"caCertificate"`
	ClientCert      *string `json:"clientCertificate"`
	ClientKey       *string `json:"clientKey"`
	AccessKeyID     *string `json:"accessKeyId"`
	SecretAccessKey *string `json:"secretAccessKey"`
	ClientID        *string `json:"clientId"`
	ClientSecret    *string `json:"clientSecret"`
	RefreshToken    *string `json:"refreshToken"`
	Token           *string `json:"token"`
	// Config replaces the per-row plugin config map when non-nil.
	Config map[string]string `json:"config,omitempty"`
}

// OAuthAuthorizeResponse contains a provider consent URL for a storage location.
type OAuthAuthorizeResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	ExpiresAt        string `json:"expiresAt"`
}

// StorageLocationSecrets is the JSON body returned by
// GET /api/storage-locations/{id}/secrets. The endpoint is
// permission-gated to settings.manage and is only used by the
// editor (the dialog's form opens with the secret fields already
// populated so the operator can review or change them). Secrets
// stay encrypted at rest — only the AES-256-GCM layer decrypts
// at the moment of the API response.
//
// The shape matches the keys the dialog's typed save path
// recognises (clientId, clientSecret, refreshToken, token) plus
// the plugin-specific fields the plugin's ConfigForm writes
// (folderId, basePath, etc.). The frontend's formFromLocation()
// merges these into the in-memory form state so the plugin's
// ConfigForm reads populated values on first render.
type StorageLocationSecrets struct {
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	Token        string `json:"token,omitempty"`
	// Generic credential columns the plugin's ConfigForm may
	// surface. Always returned as empty strings for built-in
	// columns the operator hasn't configured (e.g. privateKey for
	// a Google Drive location is always "").
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	PrivateKey      string `json:"privateKey,omitempty"`
	Passphrase      string `json:"passphrase,omitempty"`
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	// Plugin-specific fields. The plugin's ConfigForm writes
	// arbitrary keys via the per-row Config map; the secrets
	// endpoint decrypts that JSON and returns it as a flat map so
	// the frontend can merge without a second pass.
	Extra map[string]string `json:"extra,omitempty"`
}
