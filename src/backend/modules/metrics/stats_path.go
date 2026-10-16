// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// Stats path modes. Direct is a pull over the environment's own Docker
// connection, which is cheap; pull is the per-container sweep through an agent.
const (
	StatsModePush   = "push"
	StatsModePull   = "pull"
	StatsModeDirect = "direct"
	StatsModeOff    = "off"
)

// Reasons an agent environment is not on the push path.
const (
	StatsReasonAgentTooOld        = "agent_too_old"
	StatsReasonAwaitingFirstBatch = "awaiting_first_batch"
	StatsReasonAgentDisconnected  = "agent_disconnected"
	StatsReasonMetricsDisabled    = "agent_metrics_disabled"
)

// StatsPath is the GET /metrics/stats-path response.
type StatsPath struct {
	Mode            string `json:"mode"`
	Reason          string `json:"reason,omitempty"`
	IntervalSeconds int    `json:"intervalSeconds,omitempty"`
	LastBatchAt     string `json:"lastBatchAt,omitempty"`
	AgentVersion    string `json:"agentVersion,omitempty"`
	AgentBuild      string `json:"agentBuild,omitempty"`
	RequiredVersion string `json:"requiredVersion,omitempty"`
	RequiredBuild   string `json:"requiredBuild,omitempty"`
}

// ResolveStatsPath works out which stats path envID is on and, when it is not
// the push path, why. hub may be nil, in which case no environment pushes.
func ResolveStatsPath(ctx context.Context, db *sql.DB, pool *docker.ClientPool, hub *statshub.Hub, envID string) StatsPath {
	if !pool.IsAgentEnvContext(ctx, envID) {
		return StatsPath{Mode: StatsModeDirect}
	}
	if !coreSettings.ReadAgentSettingsContext(ctx, db).MetricsEnabled {
		return StatsPath{Mode: StatsModeOff, Reason: StatsReasonMetricsDisabled}
	}
	conn, ok := pool.AgentConnectionContext(ctx, envID)
	if !ok || conn.Transport == nil {
		return StatsPath{Mode: StatsModeOff, Reason: StatsReasonAgentDisconnected}
	}
	path := StatsPath{
		AgentVersion:    conn.Version,
		AgentBuild:      conn.Build,
		RequiredVersion: coreagent.StatsPushMinVersion,
		RequiredBuild:   coreagent.StatsPushMinBuild,
	}
	if !conn.Transport.StatsPushSupported() {
		path.Mode = StatsModePull
		path.Reason = StatsReasonAgentTooOld
		return path
	}
	if hub != nil {
		if st := hub.Status(envID); st.Active {
			path.Mode = StatsModePush
			path.IntervalSeconds = int(st.Interval / time.Second)
			path.LastBatchAt = st.LastBatchAt.UTC().Format(time.RFC3339)
			return path
		}
	}
	path.Mode = StatsModePull
	path.Reason = StatsReasonAwaitingFirstBatch
	return path
}
