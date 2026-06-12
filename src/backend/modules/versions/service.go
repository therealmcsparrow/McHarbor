// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"database/sql"
	"fmt"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// Service collects version information for the local instance and agents.
type Service struct {
	db        *sql.DB
	agentPool *coreagent.AgentPool
}

// NewService creates a version service.
func NewService(db *sql.DB, agentPool *coreagent.AgentPool) *Service {
	return &Service{db: db, agentPool: agentPool}
}

// Info returns the local McHarbor version plus all agent environment versions.
func (s *Service) Info() (*VersionInfo, error) {
	agents, err := s.ListAgents()
	if err != nil {
		return nil, err
	}

	return &VersionInfo{
		McHarbor: ComponentVersion{
			Name:    "mcharbor",
			Version: appversion.Current(),
		},
		Agents: agents,
	}, nil
}

// ListAgents returns version information for all agent-type environments.
func (s *Service) ListAgents() ([]AgentVersion, error) {
	rows, err := s.db.Query(`
		SELECT id, name, agent_status, agent_hostname, agent_os, agent_arch,
		       agent_version, docker_version, agent_last_seen
		FROM environments
		WHERE connection_type = 'agent'
		ORDER BY name ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying agent versions: %w", err)
	}
	defer rows.Close()

	var agents []AgentVersion
	for rows.Next() {
		var a AgentVersion
		var status, hostname, os, arch, version, dockerVer, lastSeen sql.NullString
		if err := rows.Scan(&a.EnvID, &a.EnvName, &status, &hostname, &os, &arch, &version, &dockerVer, &lastSeen); err != nil {
			return nil, fmt.Errorf("scanning agent version: %w", err)
		}
		a.Status = "disconnected"
		if status.Valid {
			a.Status = status.String
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
		s.applyLiveAgentMetadata(&a)
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []AgentVersion{}
	}
	return agents, rows.Err()
}

func (s *Service) applyLiveAgentMetadata(a *AgentVersion) {
	if a == nil || s.agentPool == nil {
		return
	}
	conn, ok := s.agentPool.Get(a.EnvID)
	if !ok {
		return
	}
	a.Status = "connected"
	a.Hostname = conn.Hostname
	a.OS = conn.OS
	a.Arch = conn.Arch
	a.AgentVersion = conn.Version
	a.DockerVersion = conn.DockerVer
}
