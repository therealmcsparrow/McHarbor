// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sync"
	"time"
)

// reconcileInterval is how often each host is asked what is actually running.
//
// Agent events carry ordinary lifecycle changes within a second, so this is the
// backstop rather than the mechanism: it catches what no event could, such as a
// guest that stopped while the agent was restarting. Frequent enough that a
// wrong state is measured in a minute, rare enough that it is one cheap query
// per host per minute.
const reconcileInterval = time.Minute

// reconcileTimeout bounds one sweep across every host.
const reconcileTimeout = 2 * time.Minute

// reconciler periodically corrects stored guest state against the hosts.
//
// Before this, nothing called Service.Reconcile at all. State moved only when a
// user acted or an agent event arrived, so a guest whose transition was missed
// stayed wrong until somebody clicked something — which is how a completed
// shutdown could read "shutting down" indefinitely.
type reconciler struct {
	svc    *Service
	logger pluginLogger
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func newReconciler(svc *Service, logger pluginLogger) *reconciler {
	return &reconciler{
		svc:    svc,
		logger: logger,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start runs the loop until Stop is called.
func (r *reconciler) Start() {
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(reconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
				r.sweep()
			}
		}
	}()
}

// Stop ends the loop and waits for an in-flight sweep to finish.
func (r *reconciler) Stop() {
	r.once.Do(func() {
		close(r.stop)
		<-r.done
	})
}

// sweep reconciles every host that has guests.
//
// A host that is unreachable is skipped quietly. An agent being offline is an
// ordinary state, already visible elsewhere, and logging it once a minute per
// host would bury everything else.
func (r *reconciler) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	envIDs, err := r.svc.guestEnvironmentIDs(ctx)
	if err != nil {
		r.logger.Warn("kvm: reconcile could not list hosts", "error", err)
		return
	}

	for _, envID := range envIDs {
		orphans, err := r.svc.Reconcile(ctx, envID)
		if err != nil {
			continue
		}
		if len(orphans) > 0 {
			r.logger.Info("kvm: host reports guests McHarbor has no row for",
				"env", envID, "units", orphans)
		}
		r.refreshAddresses(ctx, envID)
	}
}

// refreshAddresses asks each running guest's agent for its addresses.
//
// Nothing else ever called RefreshGuestInfo, so guest_reported_ips was written
// by no code path and every guest showed a blank address forever. A guest with
// no agent installed simply fails here, which is ordinary and not logged: the
// answer to "why is there no address" is the agent state the UI already shows,
// not a line per guest per minute in the host's log.
func (r *reconciler) refreshAddresses(ctx context.Context, envID string) {
	ids, err := r.svc.runningGuestIDs(ctx, envID)
	if err != nil {
		r.logger.Warn("kvm: reconcile could not list running guests", "env", envID, "error", err)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		_ = r.svc.RefreshGuestInfo(ctx, id)
	}
}
