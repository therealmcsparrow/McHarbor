// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubProvisioner satisfies IdentityProvisionerSource; these tests never provision.
type stubProvisioner struct{}

func (stubProvisioner) ProvisionUser(context.Context, IdentityProvisionRequest) (*IdentityProvisionedUser, error) {
	return &IdentityProvisionedUser{}, nil
}

// recordingLogin counts what reached the host's external-login source.
type recordingLogin struct {
	calls int
}

func (l *recordingLogin) LDAPProviderConfig(context.Context, string) (map[string]string, error) {
	l.calls++
	return map[string]string{"url": "ldap://directory.test"}, nil
}

func (l *recordingLogin) BeginExternalLogin(context.Context, http.ResponseWriter, *http.Request, string, string) error {
	l.calls++
	return nil
}

func (l *recordingLogin) CheckExternalAccount(context.Context, http.ResponseWriter, *http.Request, string, string, string, string) error {
	l.calls++
	return nil
}

func (l *recordingLogin) FailExternalLogin(context.Context, http.ResponseWriter, *http.Request, string, string, string, string) {
	l.calls++
}

func (l *recordingLogin) CompleteExternalLogin(context.Context, http.ResponseWriter, *http.Request, string, string, string, string, string, []string) error {
	l.calls++
	return nil
}

func TestIdentityHandleWithoutExternalLoginLacksTheCapability(t *testing.T) {
	r := &Runtime{host: HostServices{Identity: stubProvisioner{}}, adapters: newAdapterRegistry()}
	handle := r.identityHandleFor("identity-ldap")
	if handle == nil {
		t.Fatal("no identity handle despite a provisioner")
	}
	if _, ok := handle.(ExternalLoginSource); ok {
		t.Fatal("a host without ExternalLogin handed out the capability; the plugin could not fail closed")
	}
}

func TestExternalLoginCapabilityIsOnlyForTheLDAPOwner(t *testing.T) {
	login := &recordingLogin{}
	r := &Runtime{host: HostServices{Identity: stubProvisioner{}, ExternalLogin: login}, adapters: newAdapterRegistry()}
	r.adapters.identityOrigin[ExternalLoginProviderType] = "identity-ldap"

	intruder, ok := r.identityHandleFor("identity-oidc").(ExternalLoginSource)
	if !ok {
		t.Fatal("wired host did not hand out the capability")
	}
	if _, err := intruder.LDAPProviderConfig(t.Context(), "prov-1"); !errors.Is(err, ErrExternalLoginNotOwner) {
		t.Fatalf("non-owner read the ldap config: err = %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/identity-ldap/auth", nil)
	if err := intruder.CompleteExternalLogin(t.Context(), rec, req, "prov-1", "uid=x", "x", "", "", nil); !errors.Is(err, ErrExternalLoginNotOwner) {
		t.Fatalf("non-owner completed a sign-in: err = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-owner status = %d; want 403", rec.Code)
	}
	if login.calls != 0 {
		t.Fatalf("host source was reached %d times by a plugin that does not own ldap", login.calls)
	}

	owner := r.identityHandleFor("identity-ldap").(ExternalLoginSource)
	if _, err := owner.LDAPProviderConfig(t.Context(), "prov-1"); err != nil {
		t.Fatalf("owner refused: %v", err)
	}
	if login.calls != 1 {
		t.Fatalf("owner call did not reach the host source")
	}
}
