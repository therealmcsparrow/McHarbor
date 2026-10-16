// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Connection types an environment can hold.
const (
	ConnSocket = "socket"
	ConnTCP    = "tcp"
	ConnSSH    = "ssh"
	ConnAgent  = "agent"
)

// Errors the migration path returns. The handler maps each onto an i18n code;
// none of their text reaches a client.
var (
	ErrSameConnectionType = errors.New("environment already uses that connection type")
	ErrUnknownTargetType  = errors.New("unknown connection type")
	ErrMissingSocketPath  = errors.New("a socket path is required")
	ErrMissingTCPHost     = errors.New("a host and port are required")
	ErrMissingSSHHost     = errors.New("an ssh host and user are required")
	ErrNoPendingMigration = errors.New("no migration is pending")
)

// ConnectionMigrationRequest converts one environment from one connection type
// to another in place.
//
// In place is the whole point: the environment keeps its id, so every stack,
// metric, schedule, audit entry and compute pool keyed on it survives the
// change. Creating a second environment for the same host is what this exists
// to stop.
type ConnectionMigrationRequest struct {
	TargetType string  `json:"targetType"`
	SocketPath *string `json:"socketPath,omitempty"`
	Host       *string `json:"host,omitempty"`
	Port       *int    `json:"port,omitempty"`
	TLSCa      *string `json:"tlsCa,omitempty"`
	TLSCert    *string `json:"tlsCert,omitempty"`
	TLSKey     *string `json:"tlsKey,omitempty"`
	SSHHost    *string `json:"sshHost,omitempty"`
	SSHPort    *int    `json:"sshPort,omitempty"`
	SSHUser    *string `json:"sshUser,omitempty"`
	SSHKey     *string `json:"sshKey,omitempty"`
	// AgentToken adopts a token the operator already holds instead of minting
	// one, for an agent that is already deployed and pointed at this host.
	AgentToken *string `json:"agentToken,omitempty"`
	// Immediate flips the row now rather than waiting for the agent to
	// authenticate. It is the operator's choice to accept a dead environment
	// for the length of the install.
	Immediate bool `json:"immediate,omitempty"`
	// SkipAgentDeploy leaves the agent to the operator. The default is to
	// deploy it, because a migration that issues a token and then asks for
	// manual steps has not migrated anything.
	SkipAgentDeploy bool `json:"skipAgentDeploy,omitempty"`
	// AgentImage overrides the image the deployed agent runs.
	AgentImage string `json:"agentImage,omitempty"`
	// InsecureTLS tells the deployed agent not to verify the server
	// certificate, which a default install signs itself.
	InsecureTLS bool `json:"insecureTls,omitempty"`
}

// MigrationContext carries what only the HTTP layer knows: the address this
// McHarbor answers on, which is the address the deployed agent has to dial.
type MigrationContext struct {
	ServerURL string
	ForceTLS  bool
}

// ConnectionMigrationResult is what the dialog needs to finish the job.
//
// AgentToken is populated only when a token was minted here, and only once — it
// is never readable again, exactly like the create path.
type ConnectionMigrationResult struct {
	Environment *Environment     `json:"environment"`
	Pending     bool             `json:"pending"`
	AgentToken  string           `json:"agentToken,omitempty"`
	Agent       *AgentDeployment `json:"agent,omitempty"`
}

// MigrateConnection converts an environment to TargetType.
//
// Converting to an agent mints a token and, unless Immediate is set, leaves the
// row on its current connection until the agent authenticates. Converting away
// from an agent revokes the token and drops the live connection, because a
// valid token on a row nothing authenticates against is a credential with no
// owner.
func (s *Service) MigrateConnection(
	ctx context.Context,
	id string,
	req ConnectionMigrationRequest,
	mctx MigrationContext,
) (*ConnectionMigrationResult, error) {
	existing, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	target := strings.TrimSpace(req.TargetType)
	switch target {
	case ConnAgent:
		if existing.ConnectionType == ConnAgent {
			return nil, ErrSameConnectionType
		}
		return s.migrateToAgent(ctx, existing, req, mctx)
	case ConnSocket, ConnTCP, ConnSSH:
		if existing.ConnectionType == target && existing.PendingConnectionType == nil {
			return nil, ErrSameConnectionType
		}
		return s.migrateToDirect(ctx, existing, target, req)
	default:
		return nil, ErrUnknownTargetType
	}
}

// migrateToAgent mints or adopts a token, deploys the agent, and arms the
// cutover.
//
// The order is deliberate. The token is stored first so the agent can
// authenticate the instant it starts; the agent is then deployed through the
// connection the environment still has; only then is an immediate migration
// flipped, because flipping first would leave this method with no Docker
// connection to deploy through.
func (s *Service) migrateToAgent(
	ctx context.Context,
	existing *Environment,
	req ConnectionMigrationRequest,
	mctx MigrationContext,
) (*ConnectionMigrationResult, error) {
	resolved, minted, err := s.resolveAgentToken(req.AgentToken)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.enc.Encrypt(resolved)
	if err != nil {
		return nil, fmt.Errorf("encrypting agent token: %w", err)
	}
	hash := s.enc.StableHash(resolved)
	now := time.Now().UTC().Format(time.RFC3339)

	// The row keeps its current connection here even for an immediate
	// migration. Only the token and the pending marker are written, which is
	// what the agent auth path needs to recognise the environment when it
	// arrives — and what keeps the Docker connection alive for the deploy.
	if _, err := s.db.ExecContext(ctx, `
		UPDATE environments
		SET agent_token = ?, agent_token_hash = ?, agent_token_secret_id = NULL,
		    pending_connection_type = 'agent', pending_migration_started_at = ?,
		    updated_at = ?
		WHERE id = ?`, encrypted, hash, now, now, existing.ID); err != nil {
		return nil, fmt.Errorf("arming agent migration: %w", err)
	}

	var deployment *AgentDeployment
	if !req.SkipAgentDeploy {
		attempt := s.provisionAgentContainer(ctx, existing, agentProvisionOptions{
			ServerURL:   mctx.ServerURL,
			Token:       resolved,
			Image:       req.AgentImage,
			ForceTLS:    mctx.ForceTLS,
			InsecureTLS: req.InsecureTLS,
		})
		deployment = &attempt
	}

	// A failed deploy reverts the whole thing. The operator asked for a
	// migration, not for a half-issued token on an environment that still works
	// the old way and now carries a credential nothing will use.
	if deployment != nil && !deployment.Deployed {
		if err := s.revertMigration(ctx, existing.ID); err != nil {
			return nil, err
		}
		env, readErr := s.ByID(ctx, existing.ID)
		if readErr != nil {
			return nil, readErr
		}
		return &ConnectionMigrationResult{Environment: env, Agent: deployment}, nil
	}

	// An immediate migration converts only once the deploy has had its turn on
	// the old connection. The endpoint columns are deliberately kept: they are
	// what the failsafe reverts to if the agent never authenticates, and the
	// promotion clears them once it does.
	//
	// The guard is load-bearing. A locally deployed agent can authenticate
	// within milliseconds of starting — before this statement runs — and the
	// promotion it triggers has already finished the migration properly.
	// Without the guard this would re-arm `migration_revert_to` on a completed
	// migration and overwrite the status of an agent that is plainly connected.
	if req.Immediate {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE environments
			SET connection_type = 'agent',
			    migration_revert_to = ?,
			    agent_status = 'disconnected',
			    pending_connection_type = NULL,
			    updated_at = ?
			WHERE id = ? AND pending_migration_started_at IS NOT NULL`,
			existing.ConnectionType, time.Now().UTC().Format(time.RFC3339), existing.ID); err != nil {
			return nil, fmt.Errorf("migrating environment to agent: %w", err)
		}
	}

	s.dockerPool.Remove(existing.ID)

	env, err := s.ByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	// Read back rather than assumed: whether the migration is still waiting is
	// a property of the row now, and the agent may already have settled it.
	pending := env.PendingConnectionType != nil || env.MigrationRevertTo != nil
	result := &ConnectionMigrationResult{Environment: env, Pending: pending, Agent: deployment}
	if minted {
		result.AgentToken = resolved
	}
	return result, nil
}

// migrateToDirect converts an environment onto a socket, TCP or SSH connection.
func (s *Service) migrateToDirect(ctx context.Context, existing *Environment, target string, req ConnectionMigrationRequest) (*ConnectionMigrationResult, error) {
	if err := validateDirectTarget(target, req); err != nil {
		return nil, err
	}

	tlsCa, err := s.encryptOptional(req.TLSCa)
	if err != nil {
		return nil, fmt.Errorf("encrypting tls_ca: %w", err)
	}
	tlsCert, err := s.encryptOptional(req.TLSCert)
	if err != nil {
		return nil, fmt.Errorf("encrypting tls_cert: %w", err)
	}
	tlsKey, err := s.encryptOptional(req.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting tls_key: %w", err)
	}
	sshKey, err := s.encryptOptional(req.SSHKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting ssh_key: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE environments
		SET connection_type = ?,
		    socket_path = ?, host = ?, port = ?,
		    tls_ca = ?, tls_cert = ?, tls_key = ?,
		    tls_ca_secret_id = NULL, tls_cert_secret_id = NULL, tls_key_secret_id = NULL,
		    ssh_host = ?, ssh_port = ?, ssh_user = ?, ssh_key = ?,
		    ssh_key_secret_id = NULL,
		    agent_token = NULL, agent_token_hash = NULL, agent_token_secret_id = NULL,
		    agent_status = NULL, agent_version = NULL, agent_hostname = NULL,
		    agent_os = NULL, agent_arch = NULL, agent_last_seen = NULL,
		    pending_connection_type = NULL, pending_migration_started_at = NULL,
		    updated_at = ?
		WHERE id = ?`,
		target,
		req.SocketPath, req.Host, req.Port,
		tlsCa, tlsCert, tlsKey,
		req.SSHHost, req.SSHPort, req.SSHUser, sshKey,
		now, existing.ID); err != nil {
		return nil, fmt.Errorf("migrating environment to %s: %w", target, err)
	}

	s.disconnectAgent(existing.ID)
	s.dockerPool.Remove(existing.ID)

	env, err := s.ByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	return &ConnectionMigrationResult{Environment: env}, nil
}

// CancelPendingMigration disarms an armed agent cutover and revokes the token
// minted for it.
//
// The token is revoked rather than kept: it was issued for a cutover that is
// not happening, and an unrevoked one would silently convert the environment
// weeks later if that agent were ever started.
func (s *Service) CancelPendingMigration(ctx context.Context, id string) (*Environment, error) {
	existing, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	if existing.PendingMigrationStartedAt == nil {
		return nil, ErrNoPendingMigration
	}
	if err := s.revertMigration(ctx, id); err != nil {
		return nil, err
	}
	return s.ByID(ctx, id)
}

// revertMigration puts an environment back the way it was.
//
// One routine for all three ways a migration ends badly — the deploy failed,
// the operator cancelled, or the agent never turned up — because they have to
// leave the row in exactly the same state, and three near-identical UPDATEs
// would not stay that way.
func (s *Service) revertMigration(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE environments
		SET connection_type = COALESCE(migration_revert_to, connection_type),
		    agent_token = NULL, agent_token_hash = NULL, agent_token_secret_id = NULL,
		    agent_status = NULL, agent_version = NULL, agent_hostname = NULL,
		    agent_os = NULL, agent_arch = NULL, agent_last_seen = NULL,
		    pending_connection_type = NULL, pending_migration_started_at = NULL,
		    migration_revert_to = NULL,
		    updated_at = ?
		WHERE id = ?`, now, id); err != nil {
		return fmt.Errorf("reverting migration: %w", err)
	}
	// The endpoint columns were never cleared, so the environment is reachable
	// again the moment the cached client is dropped.
	s.dockerPool.Remove(id)
	return nil
}

// resolveAgentToken adopts the operator's token or mints a new one, reporting
// which happened so only a minted token is ever echoed back.
func (s *Service) resolveAgentToken(supplied *string) (string, bool, error) {
	if supplied != nil && strings.TrimSpace(*supplied) != "" {
		candidate := strings.TrimSpace(*supplied)
		if !agentTokenFormatRE.MatchString(candidate) {
			return "", false, fmt.Errorf("%w (must be agt_ followed by 64 hex chars)", ErrInvalidAgentToken)
		}
		return candidate, false, nil
	}
	token, err := s.mintAgentToken()
	if err != nil {
		return "", false, err
	}
	return token, true, nil
}

// disconnectAgent closes a live agent connection for an environment that is no
// longer an agent environment.
//
// The transport is closed before the pool entry is dropped, so the agent's read
// loop unwinds instead of parking on a socket nothing will read from again.
func (s *Service) disconnectAgent(envID string) {
	if s.agentPool == nil {
		return
	}
	if transport := s.agentPool.Transport(envID); transport != nil {
		transport.Close()
	}
	s.agentPool.Remove(envID)
}

// validateDirectTarget checks that a non-agent target carries what it needs to
// connect. Rejected here rather than at first use: an environment saved with no
// endpoint fails later, somewhere with no dialog to correct it in.
func validateDirectTarget(target string, req ConnectionMigrationRequest) error {
	switch target {
	case ConnSocket:
		if req.SocketPath == nil || strings.TrimSpace(*req.SocketPath) == "" {
			return ErrMissingSocketPath
		}
	case ConnTCP:
		if req.Host == nil || strings.TrimSpace(*req.Host) == "" || req.Port == nil || *req.Port <= 0 {
			return ErrMissingTCPHost
		}
	case ConnSSH:
		if req.SSHHost == nil || strings.TrimSpace(*req.SSHHost) == "" ||
			req.SSHUser == nil || strings.TrimSpace(*req.SSHUser) == "" {
			return ErrMissingSSHHost
		}
	}
	return nil
}
