// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Service handles agent-related business logic.
type Service struct {
	db        *sql.DB
	enc       *encryption.Service
	agentPool *coreagent.AgentPool
}

// NewService creates a new agent service.
func NewService(db *sql.DB, enc *encryption.Service, agentPool *coreagent.AgentPool) *Service {
	return &Service{db: db, enc: enc, agentPool: agentPool}
}

// GenerateAgentToken creates a new agent token with "agt_" prefix.
func (s *Service) GenerateAgentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating agent token: %w", err)
	}
	return "agt_" + hex.EncodeToString(b), nil
}

// ValidateAgentToken looks up an environment by matching the decrypted agent_token.
// Returns the environment ID on success.
func (s *Service) ValidateAgentToken(token string) (string, error) {
	tokenHash := s.enc.StableHash(token)

	var envID string
	var encToken string
	err := s.db.QueryRow(
		`SELECT id, agent_token
		 FROM environments
		 WHERE connection_type = 'agent' AND agent_token_hash = ?
		 LIMIT 1`,
		tokenHash,
	).Scan(&envID, &encToken)
	if err == nil {
		decrypted, decryptErr := s.enc.Decrypt(encToken)
		if decryptErr != nil {
			return "", fmt.Errorf("decrypting agent token: %w", decryptErr)
		}
		if subtle.ConstantTimeCompare([]byte(decrypted), []byte(token)) == 1 {
			return envID, nil
		}
		return "", fmt.Errorf("invalid agent token")
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("querying agent token: %w", err)
	}

	return s.validateLegacyAgentToken(token, tokenHash)
}

// UpdateAgentStatus updates the agent connection metadata in the DB.
func (s *Service) UpdateAgentStatus(envID, status string, auth *coreagent.AuthPayload) error {
	now := time.Now().UTC().Format(time.RFC3339)

	if auth != nil {
		_, err := s.db.Exec(`
			UPDATE environments
			SET agent_status = ?, agent_version = ?, agent_hostname = ?,
			    agent_os = ?, agent_arch = ?, agent_last_seen = ?,
			    docker_version = ?, last_connected = ?, updated_at = ?
			WHERE id = ?`,
			status, auth.AgentVersion, auth.Hostname,
			auth.OS, auth.Arch, now,
			auth.DockerVersion, now, now,
			envID,
		)
		return err
	}

	_, err := s.db.Exec(`
		UPDATE environments SET agent_status = ?, agent_last_seen = ?, updated_at = ? WHERE id = ?`,
		status, now, now, envID,
	)
	return err
}

// ListAgents returns agent info for all agent-type environments.
func (s *Service) ListAgents() ([]AgentInfo, error) {
	rows, err := s.db.Query(`
		SELECT id, name, agent_status, agent_hostname, agent_os, agent_arch,
		       agent_version, docker_version, agent_last_seen
		FROM environments
		WHERE connection_type = 'agent'
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying agents: %w", err)
	}
	defer rows.Close()

	var agents []AgentInfo
	for rows.Next() {
		var a AgentInfo
		var status, hostname, os, arch, version, dockerVer, lastSeen sql.NullString
		if err := rows.Scan(&a.EnvID, &a.EnvName, &status, &hostname, &os, &arch, &version, &dockerVer, &lastSeen); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		a.Status = "disconnected"
		if status.Valid {
			a.Status = status.String
		}
		// Override with live status from pool
		if s.agentPool.IsConnected(a.EnvID) {
			a.Status = "connected"
		}
		if hostname.Valid {
			a.Hostname = hostname.String
		}
		if os.Valid {
			a.OS = os.String
		}
		if arch.Valid {
			a.Arch = arch.String
		}
		if version.Valid {
			a.AgentVersion = version.String
		}
		if dockerVer.Valid {
			a.DockerVersion = dockerVer.String
		}
		if lastSeen.Valid {
			a.LastSeen = lastSeen.String
		}
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []AgentInfo{}
	}
	return agents, rows.Err()
}

// AgentStatus returns agent info for a single environment.
func (s *Service) AgentStatus(envID string) (*AgentInfo, error) {
	var a AgentInfo
	var status, hostname, os, arch, version, dockerVer, lastSeen sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, agent_status, agent_hostname, agent_os, agent_arch,
		       agent_version, docker_version, agent_last_seen
		FROM environments
		WHERE id = ? AND connection_type = 'agent'
	`, envID).Scan(&a.EnvID, &a.EnvName, &status, &hostname, &os, &arch, &version, &dockerVer, &lastSeen)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying agent status: %w", err)
	}

	a.Status = "disconnected"
	if status.Valid {
		a.Status = status.String
	}
	if s.agentPool.IsConnected(a.EnvID) {
		a.Status = "connected"
	}
	if hostname.Valid {
		a.Hostname = hostname.String
	}
	if os.Valid {
		a.OS = os.String
	}
	if arch.Valid {
		a.Arch = arch.String
	}
	if version.Valid {
		a.AgentVersion = version.String
	}
	if dockerVer.Valid {
		a.DockerVersion = dockerVer.String
	}
	if lastSeen.Valid {
		a.LastSeen = lastSeen.String
	}
	return &a, nil
}

// RegenerateToken creates a new token for an agent environment.
// Returns the plaintext token.
func (s *Service) RegenerateToken(envID string) (string, error) {
	token, err := s.GenerateAgentToken()
	if err != nil {
		return "", err
	}

	encrypted, err := s.enc.Encrypt(token)
	if err != nil {
		return "", fmt.Errorf("encrypting agent token: %w", err)
	}
	tokenHash := s.enc.StableHash(token)

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(
		`UPDATE environments
		 SET agent_token = ?, agent_token_hash = ?, updated_at = ?
		 WHERE id = ? AND connection_type = 'agent'`,
		encrypted, tokenHash, now, envID,
	)
	if err != nil {
		return "", fmt.Errorf("updating agent token: %w", err)
	}

	return token, nil
}

func (s *Service) validateLegacyAgentToken(token, tokenHash string) (string, error) {
	rows, err := s.db.Query(
		`SELECT id, agent_token
		 FROM environments
		 WHERE connection_type = 'agent' AND agent_token IS NOT NULL AND (agent_token_hash IS NULL OR agent_token_hash = '')
		 LIMIT 1000`,
	)
	if err != nil {
		return "", fmt.Errorf("querying legacy agent environments: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var envID, encToken string
		if err := rows.Scan(&envID, &encToken); err != nil {
			continue
		}
		decrypted, err := s.enc.Decrypt(encToken)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(decrypted), []byte(token)) != 1 {
			continue
		}
		if _, updateErr := s.db.Exec(
			"UPDATE environments SET agent_token_hash = ? WHERE id = ?",
			tokenHash,
			envID,
		); updateErr != nil {
			return "", fmt.Errorf("backfilling agent token hash: %w", updateErr)
		}
		return envID, nil
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterating legacy agent environments: %w", err)
	}

	return "", fmt.Errorf("invalid agent token")
}
