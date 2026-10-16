// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type codedErr string

func (c codedErr) Error() string      { return "secret " + string(c) }
func (c codedErr) SecretCode() string { return string(c) }

// fakeHostVault has the host view's method set, typed only in builtins, the
// way the real handle arrives through HostContext.Secrets.
type fakeHostVault struct {
	values   map[string]string
	denied   bool
	asked    []string
	released []string
}

func (v *fakeHostVault) Available() bool { return true }

func (v *fakeHostVault) Resolve(_ context.Context, userID, envID, name, resourceID, resourceName, field string) (string, error) {
	v.asked = append(v.asked, userID+"|"+envID+"|"+name+"|"+resourceID+"|"+resourceName+"|"+field)
	if v.denied {
		return "", codedErr("denied")
	}
	value, ok := v.values[name]
	if !ok {
		return "", codedErr("not_found")
	}
	return value, nil
}

func (v *fakeHostVault) Release(_ context.Context, resourceID string) error {
	v.released = append(v.released, resourceID)
	return nil
}

func vaultService(handle any) *Service {
	s := &Service{logger: quietLogger{}}
	s.setVault(handle)
	return s
}

var vaultGuest = VM{ID: "vm1", EnvironmentID: "envA", Name: "mcawesome-dev"}

// TestCreateTakesThePasswordAndKeysFromTheVault: the wizard names secrets, the
// values are read as the requesting user and land where typed ones would.
func TestCreateTakesThePasswordAndKeysFromTheVault(t *testing.T) {
	host := &fakeHostVault{values: map[string]string{
		"VM_PASSWORD": "s3cret",
		"ADMIN_KEYS":  "ssh-ed25519 AAAA1 one\n\nssh-ed25519 AAAA2 two\n",
	}}
	s := vaultService(host)
	ci := &CloudInitRequest{PasswordSecret: "VM_PASSWORD", SSHKeySecrets: []string{"ADMIN_KEYS"}, SSHKeys: []string{"ssh-rsa typed"}}

	if err := s.resolveCloudInitSecrets(context.Background(), "ops", vaultGuest, ci); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ci.Password != "s3cret" {
		t.Fatalf("password = %q, want the secret's value for writeCloudInit to hash", ci.Password)
	}
	if !slices.Equal(ci.SSHKeys, []string{"ssh-rsa typed", "ssh-ed25519 AAAA1 one", "ssh-ed25519 AAAA2 two"}) {
		t.Fatalf("ssh keys = %v", ci.SSHKeys)
	}
	if !slices.Equal(host.asked, []string{
		"ops|envA|VM_PASSWORD|vm1|mcawesome-dev|cloudInit.password",
		"ops|envA|ADMIN_KEYS|vm1|mcawesome-dev|cloudInit.sshKeys",
	}) {
		t.Fatalf("vault asked %v; it must read as the user, in the guest's environment, for the guest", host.asked)
	}
}

func TestCreateRefusesATypedPasswordAndASecretTogether(t *testing.T) {
	s := vaultService(&fakeHostVault{values: map[string]string{"VM_PASSWORD": "x"}})
	err := s.resolveCloudInitSecrets(context.Background(), "ops", vaultGuest,
		&CloudInitRequest{Password: "typed", PasswordSecret: "VM_PASSWORD"})
	if code, ok := validationCode(err); !ok || code != codePasswordAndSecret {
		t.Fatalf("both given: %v, want %s", err, codePasswordAndSecret)
	}
}

func TestCreateMapsTheVaultsRefusals(t *testing.T) {
	ci := func() *CloudInitRequest { return &CloudInitRequest{PasswordSecret: "VM_PASSWORD"} }

	if err := vaultService(nil).resolveCloudInitSecrets(context.Background(), "ops", vaultGuest, ci()); !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("no vault view: %v", err)
	}
	if err := vaultService(&fakeHostVault{denied: true}).resolveCloudInitSecrets(context.Background(), "ops", vaultGuest, ci()); !errors.Is(err, ErrVaultDenied) {
		t.Fatalf("denied: %v", err)
	}
	if err := vaultService(&fakeHostVault{}).resolveCloudInitSecrets(context.Background(), "ops", vaultGuest, ci()); !errors.Is(err, ErrVaultSecretNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestNoVaultSecretsTouchesNothing(t *testing.T) {
	host := &fakeHostVault{}
	ci := &CloudInitRequest{Password: "typed"}
	if err := vaultService(host).resolveCloudInitSecrets(context.Background(), "ops", vaultGuest, ci); err != nil || ci.Password != "typed" || len(host.asked) != 0 {
		t.Fatalf("a create without vault names consulted the vault or changed the password: %v %+v %v", err, ci, host.asked)
	}
}

func TestReleaseForgetsTheGuestsReferences(t *testing.T) {
	host := &fakeHostVault{}
	vaultService(host).releaseVault("vm1")
	if !slices.Equal(host.released, []string{"vm1"}) {
		t.Fatalf("released %v", host.released)
	}
}
