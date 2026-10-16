// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

type fakeVault struct {
	value       string
	err         error
	gotValue    string
	gotScope    secrets.ResolveScope
	gotField    string
	cleared     [2]string
	resolveHits int
}

func (v *fakeVault) ResolveValue(_ context.Context, value string, scope secrets.ResolveScope, field string) (string, error) {
	v.resolveHits++
	v.gotValue, v.gotScope, v.gotField = value, scope, field
	return v.value, v.err
}

func (v *fakeVault) ClearReferences(_ context.Context, resourceType, resourceID string) error {
	v.cleared = [2]string{resourceType, resourceID}
	return nil
}

// fakeSecretDeps grants secrets.view only where allowEnv says.
type fakeSecretDeps struct {
	RouterDeps
	allowEnv string
	asked    []string
}

func (d *fakeSecretDeps) HasPermissionInContext(_ context.Context, userID, envID, perm string) (bool, error) {
	d.asked = append(d.asked, userID+"|"+envID+"|"+perm)
	return perm == "secrets.view" && envID == d.allowEnv, nil
}

func (d *fakeSecretDeps) MountProtectedRoutes(func(chi.Router)) {}

// pluginVault is the interface a plugin declares on its side. Asserting it
// against the handle is what proves no host type has to cross the boundary.
type pluginVault interface {
	Available() bool
	Resolve(ctx context.Context, userID, environmentID, name, resourceID, resourceName, field string) (string, error)
	Release(ctx context.Context, resourceID string) error
}

func newVaultView(t *testing.T, vault *fakeVault, deps *fakeSecretDeps) pluginVault {
	t.Helper()
	view, ok := secretsFor(vault, deps, "addon-kvm", []string{"agent.rpc", "secrets.read"}).(pluginVault)
	if !ok || !view.Available() {
		t.Fatal("a plugin declaring secrets.read did not get a vault view its own interface accepts")
	}
	return view
}

func TestSecretsViewNeedsTheManifestToken(t *testing.T) {
	if view := secretsFor(&fakeVault{}, &fakeSecretDeps{}, "addon-kvm", []string{"agent.rpc"}); view != nil {
		t.Fatal("a plugin without secrets.read was handed the vault")
	}
	if view := secretsFor(nil, &fakeSecretDeps{}, "addon-kvm", []string{"secrets.read"}); view != nil {
		t.Fatal("a host without a vault handed out a view")
	}
}

// TestSecretsResolveChecksTheUserNotThePlugin: the vault trusts its caller, so a
// user without secrets.view where the secret is read gets nothing.
func TestSecretsResolveChecksTheUserNotThePlugin(t *testing.T) {
	vault := &fakeVault{value: "s3cret"}
	deps := &fakeSecretDeps{allowEnv: "envA"}
	view := newVaultView(t, vault, deps)

	_, err := view.Resolve(t.Context(), "ops", "envB", "VM_PASSWORD", "vm1", "web-1", "cloudInit.password")
	var coded interface{ SecretCode() string }
	if !errors.As(err, &coded) || coded.SecretCode() != "denied" {
		t.Fatalf("resolve without secrets.view in envB: %v, want a denied SecretCode", err)
	}
	if vault.resolveHits != 0 {
		t.Fatal("the vault was read before the permission check refused")
	}

	got, err := view.Resolve(t.Context(), "ops", "envA", "VM_PASSWORD", "vm1", "web-1", "cloudInit.password")
	if err != nil || got != "s3cret" {
		t.Fatalf("resolve with secrets.view: %q, %v", got, err)
	}
	if vault.gotValue != "${secret:VM_PASSWORD}" || vault.gotField != "cloudInit.password" {
		t.Fatalf("vault asked for %q field %q", vault.gotValue, vault.gotField)
	}
	scope := vault.gotScope
	if scope.EnvironmentID != "envA" || scope.ResourceType != "plugin" || scope.ResourceID != "addon-kvm:vm1" || scope.ResourceName != "web-1" {
		t.Fatalf("reference recorded as %+v", scope)
	}
}

func TestSecretsResolveRefusesANameThatIsNotOne(t *testing.T) {
	vault := &fakeVault{}
	view := newVaultView(t, vault, &fakeSecretDeps{allowEnv: "envA"})
	_, err := view.Resolve(t.Context(), "ops", "envA", "X}${secret:OTHER", "vm1", "web-1", "")
	var coded interface{ SecretCode() string }
	if !errors.As(err, &coded) || coded.SecretCode() != "invalid_name" {
		t.Fatalf("crafted name: %v, want invalid_name", err)
	}
	if vault.resolveHits != 0 {
		t.Fatal("a crafted name reached the vault")
	}
}

func TestSecretsResolveMapsAMissingSecret(t *testing.T) {
	view := newVaultView(t, &fakeVault{err: secrets.ErrNotFound}, &fakeSecretDeps{allowEnv: "envA"})
	_, err := view.Resolve(t.Context(), "ops", "envA", "NOPE", "vm1", "web-1", "")
	var coded interface{ SecretCode() string }
	if !errors.As(err, &coded) || coded.SecretCode() != "not_found" {
		t.Fatalf("missing secret: %v, want not_found", err)
	}
}

func TestSecretsReleaseForgetsOnlyThisPluginsResource(t *testing.T) {
	vault := &fakeVault{}
	view := newVaultView(t, vault, &fakeSecretDeps{})
	if err := view.Release(t.Context(), "vm1"); err != nil {
		t.Fatal(err)
	}
	if vault.cleared != [2]string{"plugin", "addon-kvm:vm1"} {
		t.Fatalf("cleared %v", vault.cleared)
	}
}
