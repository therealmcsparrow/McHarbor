// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// secretsReadToken is the manifest scope token that grants a plugin its
// vault view. It was a known token long before anything implemented it.
const secretsReadToken = "secrets.read"

// pluginSecretResourceType is the secret_references resource type every plugin
// use is recorded under. It is generic on purpose: the host must not name a
// plugin's own object kinds, so the resource id carries the plugin slug and the
// plugin supplies the name an operator reads.
const pluginSecretResourceType = "plugin"

// SecretsSource is the host vault behind a plugin's secrets view; the secrets
// service satisfies it.
type SecretsSource interface {
	ResolveValue(ctx context.Context, value string, scope secrets.ResolveScope, field string) (string, error)
	ClearReferences(ctx context.Context, resourceType, resourceID string) error
}

// secretsPermissionChecker is the slice of RouterDeps the view needs; the
// context form honours an API key's own fence.
type secretsPermissionChecker interface {
	HasPermissionInContext(ctx context.Context, userID, envID, perm string) (bool, error)
}

// SecretError is what a plugin's Resolve call fails with. SecretCode is a
// stable reason a plugin maps to its own i18n code without importing the host:
// it asserts `interface{ SecretCode() string }` on the error.
type SecretError struct {
	Code string
	err  error
}

// Error keeps the underlying cause for the server log.
func (e *SecretError) Error() string {
	if e.err == nil {
		return "secret " + e.Code
	}
	return "secret " + e.Code + ": " + e.err.Error()
}

// SecretCode is one of "denied", "not_found" or "invalid_name".
func (e *SecretError) SecretCode() string { return e.Code }

// Unwrap exposes the cause to errors.Is on the host side.
func (e *SecretError) Unwrap() error { return e.err }

// pluginSecrets is one plugin's view of the vault. Every method has builtin
// types only, so a plugin asserts an interface it declares itself and the
// plugin contract never grows a vault type.
type pluginSecrets struct {
	source SecretsSource
	perms  secretsPermissionChecker
	slug   string
}

// secretsFor hands a plugin its vault view, or nil when the host has no vault
// or the manifest does not declare secrets.read.
func secretsFor(source SecretsSource, deps RouterDeps, slug string, perms []string) any {
	if source == nil || deps == nil || !slices.Contains(perms, secretsReadToken) {
		return nil
	}
	return &pluginSecrets{source: source, perms: deps, slug: slug}
}

// Available reports whether the view is usable; a nil view never reaches a
// plugin, so this is the assertion target that proves the interface matched.
func (p *pluginSecrets) Available() bool { return p != nil && p.source != nil }

// Resolve returns the value of the secret called name as userID may use it in
// environmentID, and records the plugin resource that used it.
//
// The vault's own resolver trusts its caller, so the permission check lives
// here: the user, not the plugin, has to hold secrets.view where the secret is
// read. The value is returned to the plugin's backend only, never to a browser.
func (p *pluginSecrets) Resolve(ctx context.Context, userID, environmentID, name, resourceID, resourceName, field string) (string, error) {
	name = strings.TrimSpace(name)
	if err := secrets.ValidateName(name); err != nil {
		return "", &SecretError{Code: "invalid_name", err: err}
	}
	if strings.TrimSpace(userID) == "" {
		return "", &SecretError{Code: "denied"}
	}
	ok, err := p.perms.HasPermissionInContext(ctx, userID, environmentID, string(rbac.PermSecretsView))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &SecretError{Code: "denied"}
	}

	scope := secrets.ResolveScope{EnvironmentID: environmentID}
	if strings.TrimSpace(resourceID) != "" {
		scope.ResourceType = pluginSecretResourceType
		scope.ResourceID = p.slug + ":" + resourceID
		scope.ResourceName = resourceName
	}
	value, err := p.source.ResolveValue(ctx, "${secret:"+name+"}", scope, field)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", &SecretError{Code: "not_found", err: err}
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// Release forgets every vault reference the plugin recorded for resourceID,
// so a deleted resource stops appearing in a secret's usage list.
func (p *pluginSecrets) Release(ctx context.Context, resourceID string) error {
	if strings.TrimSpace(resourceID) == "" {
		return nil
	}
	return p.source.ClearReferences(ctx, pluginSecretResourceType, p.slug+":"+resourceID)
}
