// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// vaultView is the host's secrets view as this plugin declares it. The host
// hands it over as `any`, and only when plugin.yaml lists secrets.read, so a
// later host method never changes what this plugin was built against.
type vaultView interface {
	Available() bool
	Resolve(ctx context.Context, userID, environmentID, name, resourceID, resourceName, field string) (string, error)
	Release(ctx context.Context, resourceID string) error
}

const (
	codeVaultUnavailable  = "err.vm.vault_unavailable"
	codeVaultDenied       = "err.vm.vault_denied"
	codeVaultNotFound     = "err.vm.vault_secret_not_found"
	codePasswordAndSecret = "err.vm.password_and_secret"
)

// Sentinel errors the handler maps onto the vault codes above.
var (
	// ErrVaultUnavailable is a create that names a secret on a McHarbor that
	// gives this plugin no vault view.
	ErrVaultUnavailable = errors.New("this McHarbor gives the plugin no access to the secret vault")
	// ErrVaultDenied is a user without secrets.view where the secret is read.
	ErrVaultDenied = errors.New("the user may not read secrets in this environment")
	// ErrVaultSecretNotFound is a name no secret visible in the environment has.
	ErrVaultSecretNotFound = errors.New("no secret by that name is visible in this environment")
)

// newVault narrows the host handle to vaultView, nil when the host gave none.
func newVault(handle any) vaultView {
	view, ok := handle.(vaultView)
	if !ok || view == nil || !view.Available() {
		return nil
	}
	return view
}

// setVault wires the host's secrets view after construction, as
// setWorkflowTrigger does, so NewService's many callers stay unchanged.
func (s *Service) setVault(handle any) {
	s.vault = newVault(handle)
}

// resolveCloudInitSecrets replaces the vault names in a create request with
// their values, as the requesting user may read them, before anything exists.
//
// The password secret becomes Password and is hashed by writeCloudInit like a
// typed one; each SSH key secret may hold several keys, one per line. The copy
// is taken once: cloud-init applies it on the first boot only, so a later
// rotation in the vault does not reach the guest.
func (s *Service) resolveCloudInitSecrets(ctx context.Context, userID string, vm VM, ci *CloudInitRequest) error {
	passwordSecret := strings.TrimSpace(ci.PasswordSecret)
	if passwordSecret == "" && len(ci.SSHKeySecrets) == 0 {
		return nil
	}
	if passwordSecret != "" && ci.Password != "" {
		return invalid(codePasswordAndSecret, "both a password and a password secret were given")
	}
	if s.vault == nil {
		return ErrVaultUnavailable
	}

	if passwordSecret != "" {
		value, err := s.vault.Resolve(ctx, userID, vm.EnvironmentID, passwordSecret, vm.ID, vm.Name, "cloudInit.password")
		if err != nil {
			return vaultError(err)
		}
		ci.Password = value
	}
	for _, name := range ci.SSHKeySecrets {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		value, err := s.vault.Resolve(ctx, userID, vm.EnvironmentID, name, vm.ID, vm.Name, "cloudInit.sshKeys")
		if err != nil {
			return vaultError(err)
		}
		for _, line := range strings.Split(value, "\n") {
			if key := strings.TrimSpace(line); key != "" {
				ci.SSHKeys = append(ci.SSHKeys, key)
			}
		}
	}
	return nil
}

// vaultError maps the host's coded refusal onto this plugin's sentinels.
func vaultError(err error) error {
	var coded interface{ SecretCode() string }
	if errors.As(err, &coded) {
		switch coded.SecretCode() {
		case "denied":
			return ErrVaultDenied
		case "not_found":
			return ErrVaultSecretNotFound
		case "invalid_name":
			return invalid(codeVaultNotFound, "the secret name is not a valid vault name")
		}
	}
	return fmt.Errorf("reading a secret from the vault: %w", err)
}

// releaseVault forgets the vault references a guest recorded. Best effort: a
// stale reference only lingers in a secret's usage list.
func (s *Service) releaseVault(vmID string) {
	if s.vault == nil {
		return
	}
	if err := s.vault.Release(context.Background(), vmID); err != nil {
		s.logger.Warn("kvm: forgetting the guest's vault references", "vm", vmID, "error", err)
	}
}

// cloudInitPasswordSecret is the vault name a create took its password from,
// for the audit row; "" when none.
func cloudInitPasswordSecret(ci *CloudInitRequest) string {
	if ci == nil {
		return ""
	}
	return strings.TrimSpace(ci.PasswordSecret)
}

// cloudInitKeySecrets is the vault names a create took SSH keys from, for the
// audit row.
func cloudInitKeySecrets(ci *CloudInitRequest) []string {
	if ci == nil {
		return nil
	}
	return ci.SSHKeySecrets
}
