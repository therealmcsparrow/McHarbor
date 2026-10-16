// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// ErrNoSavedDeployment is returned when a redeploy is asked for on an
// environment that was never deployed with "remember these details".
var ErrNoSavedDeployment = errors.New("agent: no saved deployment for this environment")

// deployNameSanitiser strips everything a vault secret name may not contain.
var deployNameSanitiser = regexp.MustCompile(`[^A-Z0-9_]+`)

// SavedDeployment is the connection McHarbor remembers for one environment.
//
// It carries no credential — SecretID points at the vault row that does.
type SavedDeployment struct {
	Host        string       `json:"host"`
	Port        int          `json:"port"`
	User        string       `json:"user"`
	AuthType    string       `json:"authType"`
	SecretID    string       `json:"secretId"`
	Fingerprint string       `json:"hostKeyFingerprint"`
	Method      DeployMethod `json:"method"`
	LastAt      string       `json:"lastAt,omitempty"`
}

// deployCredential reads a saved SSH credential out of the vault.
//
// This is the only place the plaintext exists on the server, and it is never
// put on a DeployResult: a credential that reached the browser once is a
// credential in a browser's memory, a proxy log and possibly a screenshot.
func (s *Service) deployCredential(ctx context.Context, secretID string) (string, error) {
	if s.vault == nil {
		return "", errors.New("agent: secrets vault is unavailable")
	}
	value, err := s.vault.SystemValue(ctx, secretID)
	if err != nil {
		return "", fmt.Errorf("reading deploy credential: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("agent: saved deploy credential is empty")
	}
	return value, nil
}

// resolveSecretRefs substitutes every `${secret:NAME}` in the credential
// fields of a deploy request, in place.
//
// Scoped to the environment when there is one; a probe runs before the
// environment exists, so it resolves against global secrets only. Nothing is
// recorded in secret_references — a connection attempt is not a resource.
func (s *Service) resolveSecretRefs(ctx context.Context, envID string, req *DeployRequest) error {
	if s.vault == nil {
		return nil
	}
	scope := secrets.ResolveScope{EnvironmentID: envID}
	for _, field := range []*string{&req.SSHUser, &req.SSHKey, &req.SSHPassword, &req.RootPassword} {
		if !secrets.ContainsReference(*field) {
			continue
		}
		value, err := s.vault.ResolveValue(ctx, *field, scope, "")
		if err != nil {
			return fmt.Errorf("resolving deploy secret reference: %w", err)
		}
		*field = value
	}
	return nil
}

// SavedDeploymentFor returns what was remembered for an environment.
func (s *Service) SavedDeploymentFor(ctx context.Context, envID string) (*SavedDeployment, error) {
	var (
		host, user, authType, secretID, fingerprint, method, lastAt sql.NullString
		port                                                        sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT deploy_ssh_host, deploy_ssh_port, deploy_ssh_user, deploy_ssh_auth_type,
		       deploy_ssh_secret_id, deploy_host_key_fingerprint, deploy_method, deploy_last_at
		  FROM environments
		 WHERE id = ?`, envID).
		Scan(&host, &port, &user, &authType, &secretID, &fingerprint, &method, &lastAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoSavedDeployment
		}
		return nil, fmt.Errorf("reading saved deployment: %w", err)
	}
	if strings.TrimSpace(host.String) == "" || strings.TrimSpace(secretID.String) == "" {
		return nil, ErrNoSavedDeployment
	}

	return &SavedDeployment{
		Host:        host.String,
		Port:        int(port.Int64),
		User:        user.String,
		AuthType:    authType.String,
		SecretID:    secretID.String,
		Fingerprint: fingerprint.String,
		Method:      DeployMethod(method.String),
		LastAt:      lastAt.String,
	}, nil
}

// rememberDeployment stores the connection and, when the credential arrived as
// a literal, files it in the vault first.
//
// A request that already named a vault secret keeps that id rather than making
// a second copy: the operator pointed at a credential deliberately, and cloning
// it would leave two rows to rotate.
func (s *Service) rememberDeployment(
	ctx context.Context,
	envID string,
	req DeployRequest,
	fingerprint string,
) error {
	secretID := req.SSHSecretID
	if secretID == "" {
		var err error
		secretID, err = s.storeDeployCredential(ctx, envID, req)
		if err != nil {
			return err
		}
	}

	port := req.SSHPort
	if port == 0 {
		port = 22
	}
	if strings.TrimSpace(fingerprint) == "" {
		fingerprint = normalizeHostKeyFingerprint(req.HostKeyFingerprint)
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE environments
		   SET deploy_ssh_host = ?, deploy_ssh_port = ?, deploy_ssh_user = ?,
		       deploy_ssh_auth_type = ?, deploy_ssh_secret_id = ?,
		       deploy_host_key_fingerprint = ?, deploy_method = ?, deploy_last_at = ?
		 WHERE id = ?`,
		req.SSHHost, port, req.SSHUser, req.SSHAuthType, secretID,
		fingerprint, string(req.Method), time.Now().UTC().Format(time.RFC3339), envID)
	if err != nil {
		return fmt.Errorf("saving deployment: %w", err)
	}
	return nil
}

// storeDeployCredential files a typed-in key or password in the vault.
func (s *Service) storeDeployCredential(ctx context.Context, envID string, req DeployRequest) (string, error) {
	if s.vault == nil {
		return "", errors.New("agent: secrets vault is unavailable")
	}

	value := req.SSHKey
	kind := secrets.KindSSHKey
	if req.SSHAuthType == "password" {
		value = req.SSHPassword
		kind = secrets.KindPassword
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("agent: nothing to save")
	}

	created, _, err := s.vault.Create(ctx, "", envID, secrets.CreateRequest{
		Name:        deploySecretName(req.SSHUser, req.SSHHost),
		Kind:        kind,
		Value:       value,
		Description: fmt.Sprintf("Agent deployment credential for %s@%s", req.SSHUser, req.SSHHost),
	})
	if err != nil {
		return "", fmt.Errorf("storing deploy credential: %w", err)
	}
	return created.ID, nil
}

// deploySecretName builds a readable vault name from the connection.
//
// `SSH_DEPLOY_ROOT_10_35_36_254` reads as what it is in the vault list, which
// an opaque id does not.
func deploySecretName(user, host string) string {
	part := deployNameSanitiser.ReplaceAllString(
		strings.ToUpper(strings.TrimSpace(user+"_"+host)), "_")
	part = strings.Trim(part, "_")
	if part == "" {
		part = "HOST"
	}
	return "SSH_DEPLOY_" + part
}
