// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/quiesce"
)

// Poller periodically pulls users + groups from each enabled SCIM
// endpoint and reconciles them against the local users / roles
// tables. Sync cadence comes from the per-endpoint cron expression
// stored in scim_endpoints.poll_schedule.
//
// Each endpoint runs in its own goroutine so a slow / hung remote
// IdP cannot starve the others. We track an in-memory "running"
// flag per endpoint to avoid stacking duplicate runs when a pull
// runs longer than the cron interval.
type Poller struct {
	service     *Service
	logger      *slog.Logger
	puller      *Puller
	running     sync.Map // endpoint id -> struct{}
	stopOnce    sync.Once
	stop        chan struct{}
	coordinator pollerCoordinator
	nodeID      string
	hasCoord    bool
}

// pollerCoordinator is the subset of the cluster coordinator needed by the
// SCIM pull loop.
type pollerCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// NewPoller constructs a Poller that pulls through a default Puller.
func NewPoller(service *Service, logger *slog.Logger) *Poller {
	return &Poller{
		service: service,
		logger:  logger,
		puller:  NewPuller(),
		stop:    make(chan struct{}),
	}
}

// NewPollerWithCoordinator creates a poller that only pulls remote identity
// providers on the elected HA leader. SQLite coordinators always report
// leader=true.
func NewPollerWithCoordinator(service *Service, logger *slog.Logger, coord pollerCoordinator, nodeID string) *Poller {
	p := NewPoller(service, logger)
	p.coordinator = coord
	p.nodeID = nodeID
	p.hasCoord = coord != nil
	return p
}

// Start runs the scheduler loop until ctx is cancelled. Polls
// every minute so the per-endpoint cron expression resolves to
// minute granularity.
func (p *Poller) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	p.tick(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stop:
			return
		case now := <-ticker.C:
			quiesce.Run(func() { p.tick(ctx, now) })
		}
	}
}

// Stop signals the loop to exit. Safe to call multiple times.
func (p *Poller) Stop() {
	p.stopOnce.Do(func() { close(p.stop) })
}

// tick loads the due endpoints (cron match OR force_sync_requested)
// and launches a goroutine for each one. A sync.Map guards against
// stacking duplicate runs.
func (p *Poller) tick(ctx context.Context, now time.Time) {
	if !p.leaderFor(ctx) {
		p.logger.Debug("scim poller: not leader this tick; skipping", "node", p.nodeID)
		return
	}

	ids, err := p.service.DueEndpoints(ctx, now)
	if err != nil {
		p.logger.Error("scim poller: load due endpoints failed", "error", err)
		return
	}
	for _, id := range ids {
		if _, loaded := p.running.LoadOrStore(id, struct{}{}); loaded {
			continue
		}
		go func(endpointID string) {
			defer p.running.Delete(endpointID)
			if err := p.runEndpoint(ctx, endpointID); err != nil {
				p.logger.Error("scim poller: sync failed",
					"endpoint", endpointID, "error", err)
			}
		}(id)
	}
}

func (p *Poller) leaderFor(ctx context.Context) bool {
	if !p.hasCoord {
		return true
	}
	return p.coordinator.LeaderOf(ctx, "scim-poller")
}

// runEndpoint performs one sync for the endpoint: pull, plan, apply, record.
// The force-sync marker is consumed first so a manual request fires once, and
// every outcome, failures included, lands in the sync history.
func (p *Poller) runEndpoint(ctx context.Context, endpointID string) error {
	forced, err := p.service.ConsumeForceRequest(ctx, endpointID)
	if err != nil {
		return fmt.Errorf("consume force request: %w", err)
	}
	trigger := TriggerSchedule
	if forced {
		trigger = TriggerManual
	}
	plan, err := p.service.PlanSync(ctx, p.puller, endpointID)
	if err != nil {
		p.logger.Error("scim poller: plan failed", "endpoint", endpointID, "error", err)
		return p.service.RecordRun(ctx, endpointID, SyncRun{Status: "failed", Error: err.Error(), Trigger: trigger})
	}
	if err := p.service.ApplyPlan(ctx, endpointID, plan); err != nil {
		message := err.Error()
		if errors.Is(err, ErrPlanBlocked) {
			message = "sync blocked: " + plan.BlockedReason
		}
		p.logger.Error("scim poller: apply failed", "endpoint", endpointID, "error", err)
		return p.service.RecordRun(ctx, endpointID, SyncRun{
			Status: "failed", Error: message, Summary: plan.Summary, Trigger: trigger, Notes: PlanNotes(plan),
		})
	}
	return p.service.RecordRun(ctx, endpointID, SyncRun{
		Status: "success", Summary: plan.Summary, Trigger: trigger, Notes: PlanNotes(plan),
	})
}
