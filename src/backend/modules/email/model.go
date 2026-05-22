// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package email

// EmailServer represents an email server configuration.
type EmailServer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ServerType   string `json:"serverType"`   // smtp, exchange, gmail
	IsDefault    bool   `json:"isDefault"`
	Enabled      bool   `json:"enabled"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Encryption   string `json:"encryption,omitempty"`   // none, starttls, ssl_tls
	AuthMethod   string `json:"authMethod,omitempty"`   // none, plain, login, cram_md5
	Username     string `json:"username,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	TenantID     string `json:"tenantId,omitempty"`
	FromAddress  string `json:"fromAddress"`
	FromName     string `json:"fromName,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// CreateEmailServerInput is the request body for creating an email server.
type CreateEmailServerInput struct {
	Name         string `json:"name"`
	ServerType   string `json:"serverType"`
	IsDefault    bool   `json:"isDefault"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Encryption   string `json:"encryption"`
	AuthMethod   string `json:"authMethod"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	TenantID     string `json:"tenantId"`
	FromAddress  string `json:"fromAddress"`
	FromName     string `json:"fromName"`
}

// UpdateEmailServerInput is the request body for updating an email server.
// Pointer fields allow partial updates.
type UpdateEmailServerInput struct {
	Name         *string `json:"name"`
	IsDefault    *bool   `json:"isDefault"`
	Enabled      *bool   `json:"enabled"`
	Host         *string `json:"host"`
	Port         *int    `json:"port"`
	Encryption   *string `json:"encryption"`
	AuthMethod   *string `json:"authMethod"`
	Username     *string `json:"username"`
	Password     *string `json:"password"`
	ClientID     *string `json:"clientId"`
	ClientSecret *string `json:"clientSecret"`
	TenantID     *string `json:"tenantId"`
	FromAddress  *string `json:"fromAddress"`
	FromName     *string `json:"fromName"`
}

// TestEmailInput is the request body for sending a test email.
type TestEmailInput struct {
	To string `json:"to"`
}
