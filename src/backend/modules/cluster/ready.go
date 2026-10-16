// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// ReadyCheck is one readiness condition. It carries a name and a verdict only: the
// probe is public, so a failure never explains itself to an anonymous caller.
type ReadyCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

// SetRegistry attaches the instance registry once main has built it.
func (s *Service) SetRegistry(r *Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = r
}

// Registry returns the attached instance registry, or nil before main wires it.
func (s *Service) Registry() *Registry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registry
}

// Ready answers what a load balancer needs: the database answers, the lock session is
// alive where leader election exists, and this node is neither draining nor a standby.
func (s *Service) Ready(ctx context.Context) (bool, []ReadyCheck) {
	return s.ready(ctx, s.Registry())
}

// readyLocked is Ready for callers already holding s.mu.
func (s *Service) readyLocked(ctx context.Context) (bool, []ReadyCheck) {
	return s.ready(ctx, s.registry)
}

func (s *Service) ready(ctx context.Context, reg *Registry) (bool, []ReadyCheck) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	checks := []ReadyCheck{{Name: "database", OK: s.pingOK(ctx)}}
	if db.DriverOf() != db.DriverSQLite {
		checks = append(checks, ReadyCheck{Name: "coordinator", OK: s.coord.Ping(ctx) == nil})
	}
	if reg != nil {
		checks = append(checks,
			ReadyCheck{Name: "accepting", OK: reg.State() == StateRunning},
			ReadyCheck{Name: "serving", OK: reg.Role() != RoleStandby})
	}
	ready := true
	for _, c := range checks {
		ready = ready && c.OK
	}
	return ready, checks
}
