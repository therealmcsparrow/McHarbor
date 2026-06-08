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
	Enabled         bool   `json:"enabled"`
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
}

// OAuthAuthorizeResponse contains a provider consent URL for a storage location.
type OAuthAuthorizeResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	ExpiresAt        string `json:"expiresAt"`
}
