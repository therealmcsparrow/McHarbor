// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package agent serves the remote-agent WebSocket, install and deploy flows, and agent health for agent-type environments.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// AccessSettings is what the Access tab shows: where McHarbor reaches this
// host, and which vault row holds the credential — never the credential.
type AccessSettings struct {
	Configured  bool         `json:"configured"`
	Host        string       `json:"host,omitempty"`
	Port        int          `json:"port,omitempty"`
	User        string       `json:"user,omitempty"`
	AuthType    string       `json:"authType,omitempty"`
	Method      DeployMethod `json:"method,omitempty"`
	Fingerprint string       `json:"hostKeyFingerprint,omitempty"`
	LastAt      string       `json:"lastAt,omitempty"`
	SecretID    string       `json:"secretId,omitempty"`
	SecretName  string       `json:"secretName,omitempty"`
	SecretKind  string       `json:"secretKind,omitempty"`
}

// AccessUpdateRequest replaces the stored SSH access for an environment.
//
// SSHSecretID points at an existing vault row; SSHKey or SSHPassword are a
// literal to be filed in the vault. Sending neither keeps the credential that
// is already stored and updates only the connection details.
type AccessUpdateRequest struct {
	SSHHost            string       `json:"sshHost"`
	SSHPort            int          `json:"sshPort"`
	SSHUser            string       `json:"sshUser"`
	SSHAuthType        string       `json:"sshAuthType"`
	SSHKey             string       `json:"sshKey,omitempty"`
	SSHPassword        string       `json:"sshPassword,omitempty"`
	SSHSecretID        string       `json:"sshSecretId,omitempty"`
	HostKeyFingerprint string       `json:"hostKeyFingerprint"`
	Method             DeployMethod `json:"method"`
}

// Access returns the saved SSH access for an environment, decorated with the
// vault row's name so the tab can say which secret is in use.
func (s *Service) Access(ctx context.Context, envID string) (*AccessSettings, error) {
	saved, err := s.SavedDeploymentFor(ctx, envID)
	if err != nil {
		if errors.Is(err, ErrNoSavedDeployment) {
			return &AccessSettings{Configured: false}, nil
		}
		return nil, err
	}

	settings := &AccessSettings{
		Configured:  true,
		Host:        saved.Host,
		Port:        saved.Port,
		User:        saved.User,
		AuthType:    saved.AuthType,
		Method:      saved.Method,
		Fingerprint: saved.Fingerprint,
		LastAt:      saved.LastAt,
		SecretID:    saved.SecretID,
	}
	if s.vault != nil && saved.SecretID != "" {
		if secret, err := s.vault.ByID(ctx, saved.SecretID); err == nil && secret != nil {
			settings.SecretName = secret.Name
			settings.SecretKind = secret.Kind
		}
	}
	return settings, nil
}

// SaveAccess stores the SSH access an operator entered on the Access tab.
//
// A literal key or password is filed in the vault first, so the environments
// row still only ever holds an id.
func (s *Service) SaveAccess(ctx context.Context, envID string, req AccessUpdateRequest) error {
	if strings.TrimSpace(req.SSHHost) == "" || strings.TrimSpace(req.SSHUser) == "" {
		return errors.New("agent: a host and user are required")
	}
	if req.SSHAuthType != "key" && req.SSHAuthType != "password" {
		return errors.New("agent: auth type must be key or password")
	}
	if req.SSHPort <= 0 || req.SSHPort > 65535 {
		req.SSHPort = 22
	}
	if req.Method == "" {
		req.Method = DeployBinary
	}

	secretID := strings.TrimSpace(req.SSHSecretID)
	literal := req.SSHKey
	if req.SSHAuthType == "password" {
		literal = req.SSHPassword
	}

	switch {
	case strings.TrimSpace(literal) != "":
		stored, err := s.storeDeployCredential(ctx, envID, DeployRequest{
			SSHHost:     req.SSHHost,
			SSHUser:     req.SSHUser,
			SSHAuthType: req.SSHAuthType,
			SSHKey:      req.SSHKey,
			SSHPassword: req.SSHPassword,
		})
		if err != nil {
			return err
		}
		secretID = stored
	case secretID == "":
		// Neither a literal nor a chosen secret: keep whatever is filed, so
		// editing the port does not silently drop the credential.
		existing, err := s.SavedDeploymentFor(ctx, envID)
		if err != nil {
			return errors.New("agent: a credential is required the first time access is saved")
		}
		secretID = existing.SecretID
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE environments
		   SET deploy_ssh_host = ?, deploy_ssh_port = ?, deploy_ssh_user = ?,
		       deploy_ssh_auth_type = ?, deploy_ssh_secret_id = ?,
		       deploy_host_key_fingerprint = ?, deploy_method = ?
		 WHERE id = ?`,
		req.SSHHost, req.SSHPort, req.SSHUser, req.SSHAuthType, secretID,
		req.HostKeyFingerprint, string(req.Method), envID)
	if err != nil {
		return fmt.Errorf("saving access settings: %w", err)
	}
	return nil
}

// ForgetAccess clears the stored connection and unlinks the vault row.
//
// The secret itself is left in the vault: it may be shared with another
// environment, and deleting someone else's credential is not this tab's call.
func (s *Service) ForgetAccess(ctx context.Context, envID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE environments
		   SET deploy_ssh_host = NULL, deploy_ssh_port = NULL, deploy_ssh_user = NULL,
		       deploy_ssh_auth_type = NULL, deploy_ssh_secret_id = NULL,
		       deploy_host_key_fingerprint = NULL, deploy_method = NULL, deploy_last_at = NULL
		 WHERE id = ?`, envID)
	if err != nil {
		return fmt.Errorf("clearing access settings: %w", err)
	}
	return nil
}

// SSHSecrets lists the vault rows an operator can pick as an SSH credential.
func (s *Service) SSHSecrets(ctx context.Context, envID string) ([]AccessSecretOption, error) {
	if s.vault == nil {
		return []AccessSecretOption{}, nil
	}
	rows, _, err := s.vault.List(ctx, secrets.ListFilter{
		EnvironmentID: envID,
		IncludeSystem: true,
	}, 1, 200)
	if err != nil {
		return nil, fmt.Errorf("listing vault secrets: %w", err)
	}
	options := make([]AccessSecretOption, 0, len(rows))
	for _, row := range rows {
		if row.Kind != secrets.KindSSHKey && row.Kind != secrets.KindPassword {
			continue
		}
		options = append(options, AccessSecretOption{
			ID:   row.ID,
			Name: row.Name,
			Kind: row.Kind,
		})
	}
	return options, nil
}

// AccessSecretOption is one vault row offered as an SSH credential.
type AccessSecretOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// HandleAccess returns the saved SSH access for an environment.
func (h *Handler) HandleAccess(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	envID := chi.URLParam(r, "envId")
	settings, err := h.service.Access(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("agent: reading access settings failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, settings)
}

// HandleAccessSecrets lists vault rows usable as an SSH credential.
func (h *Handler) HandleAccessSecrets(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	envID := chi.URLParam(r, "envId")
	options, err := h.service.SSHSecrets(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("agent: listing ssh secrets failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, options)
}

// HandleSaveAccess stores the SSH access an operator entered.
func (h *Handler) HandleSaveAccess(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	var req AccessUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	envID := chi.URLParam(r, "envId")
	if err := h.service.SaveAccess(r.Context(), envID, req); err != nil {
		h.app.Logger.Error("agent: saving access settings failed", "env", envID, "error", err)
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "agent_access_updated",
		EntityType: "environment",
		EntityID:   envID,
		Details:    fmt.Sprintf("ssh access set to %s@%s:%d", req.SSHUser, req.SSHHost, req.SSHPort),
	})
	response.OKMsg(w, r, i18n.MsgAgentAccessSaved)
}

// HandleForgetAccess clears the stored SSH access for an environment.
func (h *Handler) HandleForgetAccess(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}

	envID := chi.URLParam(r, "envId")
	if err := h.service.ForgetAccess(r.Context(), envID); err != nil {
		h.app.Logger.Error("agent: clearing access settings failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "agent_access_cleared",
		EntityType: "environment",
		EntityID:   envID,
	})
	response.OKMsg(w, r, i18n.MsgAgentAccessSaved)
}
