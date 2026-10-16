// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// External-login failure reasons a plugin may pass to FailExternalLogin. Every
// reason except ExternalReasonUnavailable counts toward lockout, exactly as a
// wrong password does.
const (
	// ExternalReasonInvalidCredentials is a rejected bind or an unknown user.
	ExternalReasonInvalidCredentials = "invalid_credentials" //nolint:gosec // a failure-reason code, not a credential
	// ExternalReasonNotProvisioned is a valid credential with no McHarbor account
	// and auto-provisioning off; it still counts, so it cannot probe the directory.
	ExternalReasonNotProvisioned = "not_provisioned"
	// ExternalReasonUnavailable is a directory or provider the host could not use.
	// It does not count: an outage must not lock every user out behind it.
	ExternalReasonUnavailable = "directory_unavailable"
)

// ErrExternalLoginRefused tells the plugin the host refused the attempt and has
// already written the response.
var ErrExternalLoginRefused = errors.New("external login refused")

// ExternalProvisioner is the identity module's half of an external sign-in:
// provider configuration and account linking. It is declared here and satisfied
// structurally, so this module never imports the identity module.
type ExternalProvisioner interface {
	// ExternalProviderConfig returns an enabled provider of providerType with its
	// secrets decrypted.
	ExternalProviderConfig(ctx context.Context, providerID, providerType string) (map[string]string, error)
	// LinkedUserID returns the account a sign-in by externalID would land on, or ""
	// when there is none yet.
	LinkedUserID(ctx context.Context, providerID, externalID, email string) (string, error)
	// ProvisionExternalUser links or creates the account per the provider's
	// settings; "" with a nil error means auto-provisioning is off and none matched.
	ProvisionExternalUser(ctx context.Context, providerID, externalID, username, email, displayName string, groups []string) (string, error)
}

// ExternalLogin is the host half of a sign-in whose credential a plugin checks
// against a directory. It owns everything a password login owns — lockout,
// login_attempts, audit, MFA, the enrolment gate and the session cookie — so a
// directory user cannot get a weaker sign-in than a local one.
type ExternalLogin struct {
	h           *Handler
	provisioner ExternalProvisioner
}

// NewExternalLogin builds the external-login host with the same second-factor
// verifier the password login uses.
func NewExternalLogin(app *router.AppDeps, verifier MFAVerifier, provisioner ExternalProvisioner) *ExternalLogin {
	return &ExternalLogin{h: NewHandlerWithMFA(app, verifier), provisioner: provisioner}
}

// externalProviderType is the one directory type this host serves; OIDC and SAML
// never check a credential in a plugin.
const externalProviderType = "ldap"

// auditExtra tags every audit entry of an external sign-in with its provider.
func auditExtra(providerID string) map[string]string {
	return map[string]string{"identityProviderId": providerID, "method": externalProviderType}
}

// LDAPProviderConfig returns an enabled ldap provider's configuration, secrets
// decrypted, for the plugin's bind; it is never written to any response.
func (e *ExternalLogin) LDAPProviderConfig(ctx context.Context, providerID string) (map[string]string, error) {
	return e.provisioner.ExternalProviderConfig(ctx, providerID, externalProviderType)
}

// BeginExternalLogin refuses a locked client address before the directory is
// contacted, answering with the same 423 a password login from it would get.
func (e *ExternalLogin) BeginExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username string) error {
	if res := e.h.app.AuthService.ExternalAddressGateContext(ctx, username, resolveLoginClient(e.h.app, r)); res != nil {
		e.h.refuseLogin(w, r, username, res, auditExtra(providerID))
		return ErrExternalLoginRefused
	}
	return nil
}

// CheckExternalAccount refuses a locked or disabled linked account before the
// user's bind, so the directory never sees a guess against a locked account. A
// failed lookup proceeds, matching CheckLockout's fail-open.
func (e *ExternalLogin) CheckExternalAccount(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, email string) error {
	userID, err := e.provisioner.LinkedUserID(ctx, providerID, externalID, email)
	if err != nil {
		e.h.app.Logger.Warn("external login: linked account lookup failed", "provider_id", providerID, "error", err)
		return nil
	}
	if res := e.h.app.AuthService.ExternalAccountGateContext(ctx, username, userID, resolveLoginClient(e.h.app, r)); res != nil {
		e.h.refuseLogin(w, r, username, res, auditExtra(providerID))
		return ErrExternalLoginRefused
	}
	return nil
}

// FailExternalLogin records a failed attempt and writes the refusal. An outage is
// answered 503 and not counted; every other reason counts against the address and
// the linked account, and answers exactly as a wrong password.
func (e *ExternalLogin) FailExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, username, externalID, reason string) {
	if reason == ExternalReasonUnavailable {
		e.h.auditUnavailable(r, username, providerID)
		response.ServiceUnavailableCode(w, r, i18n.ErrIdentityDirectoryUnavailable)
		return
	}
	userID := ""
	if externalID != "" {
		linked, err := e.provisioner.LinkedUserID(ctx, providerID, externalID, "")
		if err != nil {
			e.h.app.Logger.Warn("external login: linked account lookup failed", "provider_id", providerID, "error", err)
		}
		userID = linked
	}
	res := e.h.app.AuthService.ExternalLoginFailedContext(ctx, username, userID, reason, resolveLoginClient(e.h.app, r))
	e.h.refuseLogin(w, r, username, res, auditExtra(providerID))
}

// CompleteExternalLogin links or provisions the account, re-checks it (an email
// match can land on an account the pre-bind check could not see), and finishes
// through completeLogin — the same MFA challenge, enrolment gate, cookie and
// audit a password login gets.
func (e *ExternalLogin) CompleteExternalLogin(ctx context.Context, w http.ResponseWriter, r *http.Request, providerID, externalID, username, email, displayName string, groups []string) error {
	extra := auditExtra(providerID)
	userID, err := e.provisioner.ProvisionExternalUser(ctx, providerID, externalID, username, email, displayName, groups)
	if err != nil {
		e.h.app.Logger.Error("external login: provisioning failed", "provider_id", providerID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrOIDCProvisionFailed, err)
		return err
	}
	if userID == "" {
		e.FailExternalLogin(ctx, w, r, providerID, username, "", ExternalReasonNotProvisioned)
		return ErrExternalLoginRefused
	}
	client := resolveLoginClient(e.h.app, r)
	if res := e.h.app.AuthService.ExternalAccountGateContext(ctx, username, userID, client); res != nil {
		e.h.refuseLogin(w, r, username, res, extra)
		return ErrExternalLoginRefused
	}
	result, err := e.h.app.AuthService.ExternalLoginSucceededContext(ctx, userID)
	if err != nil {
		e.h.app.Logger.Error("external login: completing sign-in failed", "provider_id", providerID, "user_id", userID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return err
	}
	if !result.Success {
		e.h.refuseLogin(w, r, username, result, extra)
		return ErrExternalLoginRefused
	}
	e.h.completeLogin(w, r, result, extra)
	return nil
}

// auditUnavailable records a directory sign-in the directory could not serve, so
// an outage is visible in the audit log beside the failures it did not count.
func (h *Handler) auditUnavailable(r *http.Request, username, providerID string) {
	fields := auditExtra(providerID)
	fields["reason"] = ExternalReasonUnavailable
	fields["attemptedUsername"] = auth.NormalizeUsername(username)
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "login_failed",
		Severity:   "error",
		EntityType: "user",
		EntityName: fields["attemptedUsername"],
		Details:    h.auditDetails(fields),
	})
}
