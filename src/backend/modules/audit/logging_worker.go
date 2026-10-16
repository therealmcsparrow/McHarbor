// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// forwardTickInterval matches the cadence the two hard-coded
// forwarders used before this became a plugin family. Five seconds is
// short enough that an operator clicking around after enabling a
// forwarder sees delivery rows appear while they are still looking at
// the page, and long enough that an idle install does one trivial
// indexed query per tick.
const forwardTickInterval = 5 * time.Second

// forwardCycleTimeout bounds one ForwardPending call. A cycle that
// outruns the ticker blocks the next one, so a wedged sink degrades
// into "forwarding is slow" rather than "the goroutine is gone".
const forwardCycleTimeout = 60 * time.Second

// forwarderCoordinator is the subset of the cluster coordinator the
// forwarding loop needs. SQLite (single-node) coordinators always
// report leader=true.
type forwarderCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// LogForwarderWorker drives every installed `logging-` plugin on one
// ticker.
//
// It replaces the former SIEMForwarder and SyslogForwarder, which were
// two copies of this loop differing only in which service method they
// called. The transport-specific half of each moved into its plugin;
// what is left — a ticker, a rollout flag, and HA leader election — is
// host infrastructure no plugin should have to re-implement, and the
// single reason the host still drives forwarding at all.
type LogForwarderWorker struct {
	service     *LoggingService
	db          *sql.DB
	logger      *slog.Logger
	cancel      context.CancelFunc
	coordinator forwarderCoordinator
	nodeID      string
	hasCoord    bool
}

// NewLogForwarderWorker builds a worker with no cluster coordinator —
// every tick is a leader tick. Correct for single-node installs.
func NewLogForwarderWorker(service *LoggingService, database *sql.DB, logger *slog.Logger) *LogForwarderWorker {
	return &LogForwarderWorker{service: service, db: database, logger: logger}
}

// NewLogForwarderWorkerWithCoordinator builds a worker that forwards
// only on the elected HA leader, so an N-node cluster delivers each
// audit event once rather than N times.
func NewLogForwarderWorkerWithCoordinator(service *LoggingService, database *sql.DB, logger *slog.Logger, coord forwarderCoordinator, nodeID string) *LogForwarderWorker {
	w := NewLogForwarderWorker(service, database, logger)
	w.coordinator = coord
	w.nodeID = nodeID
	w.hasCoord = coord != nil
	return w
}

// Start launches the loop. Safe to call on a nil or serviceless worker
// — a host built without a plugin runtime simply never forwards.
func (w *LogForwarderWorker) Start(ctx context.Context) {
	if w == nil || w.service == nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go func() {
		ticker := time.NewTicker(forwardTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				quiesce.Run(func() { w.tick(workerCtx) })
			}
		}
	}()
}

// Flush runs one forwarding cycle now instead of waiting for the next tick.
// An instance restore calls it so its intent reaches the external sink before
// the table that holds it is replaced.
func (w *LogForwarderWorker) Flush(ctx context.Context) {
	w.tick(ctx)
}

// tick runs one cycle across every enabled forwarder.
func (w *LogForwarderWorker) tick(ctx context.Context) {
	forwarders, err := w.service.enabledForwarders(ctx)
	if err != nil {
		w.logger.Error("audit log forwarding: reading enabled forwarders failed", "error", err)
		return
	}
	if len(forwarders) == 0 {
		return
	}
	for _, f := range forwarders {
		// The rollout flag stays per forwarder *type*, not per
		// instance: it is the operator's kill switch for a whole
		// transport misbehaving, and an individual destination is
		// already switchable from its own Enabled toggle.
		flag := "audit." + f.spec.ForwarderType + "_forwarding.enabled"
		if !coreSettings.ReadAuditSettingBoolContext(ctx, w.db, flag, true) {
			w.logger.Debug("audit log forwarding: disabled by rollout flag",
				"forwarder", f.spec.ForwarderType, "instance", f.name)
			continue
		}
		if f.spec.Capabilities.RequiresLeaderElection && !w.leader(ctx) {
			w.logger.Debug("audit log forwarding: not leader this tick; skipping",
				"forwarder", f.spec.ForwarderType, "instance", f.name, "node", w.nodeID)
			continue
		}
		w.runOne(ctx, f)
	}
}

// runOne drives a single adapter's cycle under its own deadline, so
// one slow sink cannot starve the others on the same tick.
func (w *LogForwarderWorker) runOne(ctx context.Context, f enabledForwarder) {
	cycleCtx, cancel := context.WithTimeout(ctx, forwardCycleTimeout)
	defer cancel()

	result, err := f.client.ForwardPending(cycleCtx, f.config)
	if err != nil {
		// A failing sink is expected and recoverable — the collector
		// is down, the token expired, the firewall changed. Log it and
		// tick again; the adapter's own retry ceiling decides when to
		// give up on an individual event.
		w.logger.Error("audit log forwarding failed",
			"forwarder", f.spec.ForwarderType, "instance", f.name, "id", f.id, "error", err)
		return
	}
	if result.Delivered > 0 || result.Failed > 0 {
		w.logger.Info("audit log forwarding cycle",
			"forwarder", f.spec.ForwarderType,
			"instance", f.name,
			"delivered", result.Delivered,
			"failed", result.Failed,
			"skipped", result.Skipped,
		)
	}
}

// leader reports whether this node holds the forwarder lease.
func (w *LogForwarderWorker) leader(ctx context.Context) bool {
	if !w.hasCoord {
		return true
	}
	return w.coordinator.LeaderOf(ctx, "audit-log-forwarder")
}

// Stop cancels the loop.
func (w *LogForwarderWorker) Stop() {
	if w != nil && w.cancel != nil {
		w.cancel()
	}
}
