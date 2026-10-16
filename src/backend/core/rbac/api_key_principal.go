// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import "context"

// APIKeyPrincipal is the authorization identity of a request that was
// authenticated with a Bearer API key. It travels in the request context
// next to the owning auth.User. Every permission method on Service reads
// it and intersects the key's own permission set with the owner's, so a
// key can never do more than the person who holds it.
type APIKeyPrincipal struct {
	KeyID          string
	Name           string
	RoleIDs        []string
	Scopes         []Permission
	EnvironmentIDs []string
}

type apiKeyContextKey struct{}

// WithAPIKey stores the principal in ctx.
func WithAPIKey(ctx context.Context, p *APIKeyPrincipal) context.Context {
	return context.WithValue(ctx, apiKeyContextKey{}, p)
}

// APIKeyFromContext returns the principal, or nil for a session request.
func APIKeyFromContext(ctx context.Context) *APIKeyPrincipal {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(apiKeyContextKey{}).(*APIKeyPrincipal)
	return p
}

// allowsEnvironment reports whether the key may act in envID. An empty
// envID (global request) and an empty fence both allow.
func (p *APIKeyPrincipal) allowsEnvironment(envID string) bool {
	if p == nil || envID == "" || len(p.EnvironmentIDs) == 0 {
		return true
	}
	for _, id := range p.EnvironmentIDs {
		if id == envID {
			return true
		}
	}
	return false
}

// apiKeyCacheKey namespaces key principals away from "userID:env:stack".
func apiKeyCacheKey(keyID string) string {
	return "apikey:" + keyID
}

// keyPermissions resolves the union of the key's role permissions and its
// explicit scopes. The result is cached per key; role edits clear the
// whole cache and key edits call InvalidateAPIKeyCache.
func (s *Service) keyPermissions(ctx context.Context, p *APIKeyPrincipal) ([]Permission, error) {
	cacheKey := apiKeyCacheKey(p.KeyID)

	s.mu.RLock()
	if cached, ok := s.cache[cacheKey]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	perms, err := s.resolvePermissions(ctx, p.RoleIDs)
	if err != nil {
		return nil, err
	}
	if perms == nil {
		perms = []Permission{}
	}
	if !hasPermission(perms, PermWildcard) {
		seen := make(map[Permission]bool, len(perms))
		for _, perm := range perms {
			seen[perm] = true
		}
		for _, scope := range p.Scopes {
			if scope == PermWildcard {
				perms = []Permission{PermWildcard}
				break
			}
			if !seen[scope] {
				seen[scope] = true
				perms = append(perms, scope)
			}
		}
	}

	s.mu.Lock()
	s.cache[cacheKey] = perms
	s.mu.Unlock()

	return perms, nil
}

// intersectWithKey applies the spec's effective-set rule:
//
//	owner has "*"  -> the key's own set (which may itself be "*")
//	key has "*"    -> the owner's set
//	otherwise      -> keyPerms ∩ ownerPerms, in owner order
func intersectWithKey(keyPerms, ownerPerms []Permission) []Permission {
	if hasPermission(ownerPerms, PermWildcard) {
		return keyPerms
	}
	if hasPermission(keyPerms, PermWildcard) {
		return ownerPerms
	}
	allowed := make(map[Permission]bool, len(keyPerms))
	for _, perm := range keyPerms {
		allowed[perm] = true
	}
	result := make([]Permission, 0, len(ownerPerms))
	for _, perm := range ownerPerms {
		if allowed[perm] {
			result = append(result, perm)
		}
	}
	return result
}

// ConfiguredPermissionsForKey returns what a key asks for: the union of its
// roles' permissions and its explicit grants, before the owner is consulted.
//
// This is the "configured" half of the pair the detail page shows. On its own
// it can overstate what the key can do, which is exactly why it is shown
// beside the effective set rather than instead of it.
func (s *Service) ConfiguredPermissionsForKey(key *APIKeyPrincipal) ([]Permission, error) {
	return s.ConfiguredPermissionsForKeyContext(context.Background(), key)
}

// ConfiguredPermissionsForKeyContext is ConfiguredPermissionsForKey bound to ctx, so a cancelled request stops the role lookup.
func (s *Service) ConfiguredPermissionsForKeyContext(ctx context.Context, key *APIKeyPrincipal) ([]Permission, error) {
	if s == nil || key == nil {
		return []Permission{}, nil
	}
	perms, err := s.keyPermissions(ctx, key)
	if err != nil {
		return nil, err
	}
	return perms, nil
}

// EffectivePermissionsForKey returns what a key can actually do right now:
// its configured set intersected with its owner's current permissions.
//
// Computed rather than stored, because the owner's rights move underneath the
// key. Revoking a role from the person shrinks every key they hold, and the
// detail page has to show that the moment it happens.
func (s *Service) EffectivePermissionsForKey(ownerID string, key *APIKeyPrincipal) ([]Permission, error) {
	return s.EffectivePermissionsForKeyContext(context.Background(), ownerID, key)
}

// EffectivePermissionsForKeyContext is EffectivePermissionsForKey bound to ctx, so a cancelled request stops the owner and key lookups.
func (s *Service) EffectivePermissionsForKeyContext(ctx context.Context, ownerID string, key *APIKeyPrincipal) ([]Permission, error) {
	if s == nil || key == nil {
		return []Permission{}, nil
	}

	owner, err := s.effectivePermissions(ctx, ownerID, "", "")
	if err != nil {
		return nil, err
	}
	keyPerms, err := s.keyPermissions(ctx, key)
	if err != nil {
		return nil, err
	}
	return intersectWithKey(keyPerms, owner), nil
}

// InvalidateAPIKeyCache drops the cached permission set of one key.
func (s *Service) InvalidateAPIKeyCache(keyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, apiKeyCacheKey(keyID))
}
