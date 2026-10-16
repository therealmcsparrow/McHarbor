// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"database/sql"
	"log/slog"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/statshub"
)

// subscribeStatsPush asks a freshly connected agent to push its container stats
// when it can, so it is on the push path from the first interval rather than
// from the collector's next tick. An agent that cannot stays on the pull path.
func subscribeStatsPush(ctx context.Context, db *sql.DB, hub *statshub.Hub, transport *coreagent.AgentTransport, logger *slog.Logger, envID string) {
	if hub == nil || !transport.StatsPushSupported() || !coreSettings.ReadAgentSettingsContext(ctx, db).MetricsEnabled {
		return
	}
	if err := transport.SubscribeStats(hub.Interval()); err != nil {
		logger.Warn("agent stats push subscribe failed", "env", envID, "error", err)
		return
	}
	hub.MarkSubscribed(envID)
}
