// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

// AuthProvider is the pluggable interface for authentication backends.
// Local auth is the default; OIDC, LDAP, MFA can be added later.
type AuthProvider interface {
	// Name returns the provider identifier (e.g., "local", "oidc", "ldap").
	Name() string

	// Authenticate validates credentials and returns a user.
	Authenticate(credentials map[string]string) (*AuthResult, error)

	// SupportsRegistration returns true if this provider allows user creation.
	SupportsRegistration() bool
}

// LocalProvider implements AuthProvider for username/password auth.
type LocalProvider struct {
	svc *Service
}

// NewLocalProvider creates a local auth provider.
func NewLocalProvider(svc *Service) *LocalProvider {
	return &LocalProvider{svc: svc}
}

func (p *LocalProvider) Name() string { return "local" }

func (p *LocalProvider) Authenticate(credentials map[string]string) (*AuthResult, error) {
	username := credentials["username"]
	password := credentials["password"]
	return p.svc.Login(username, password)
}

func (p *LocalProvider) SupportsRegistration() bool { return true }
