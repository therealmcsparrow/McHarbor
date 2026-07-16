// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package cluster exposes the inter-node coordination state via
// a small read-only API. The endpoint is intentionally lightweight
// — it's surfaced on the /api/cluster/status route that load
// balancers and operator dashboards can poll, and on the public
// health route so an LB can demote a node that's lost leadership
// of every singleton.
package cluster

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// SingletonRole is a well-known job name. Exported so the
// status payload can use string constants rather than typos.
type SingletonRole string

const (
	// RoleScheduler is the container-backup scheduler tick.
	RoleScheduler SingletonRole = "container-backup-scheduler"
)

// Status is the JSON shape returned by /api/cluster/status.
type Status struct {
	NodeID   string                  `json:"nodeId"`
	Driver   string                  `json:"databaseDriver"`
	Roles    map[string]bool         `json:"roles"`
	Database dbStatus                `json:"database"`
	Time     time.Time               `json:"serverTime"`
}

// dbStatus is a minimal database liveness probe. We don't leak
// connection pool internals — just whether a ping round-trip
// succeeds within the timeout.
type dbStatus struct {
	Reachable bool `json:"reachable"`
}

// Service owns the coordinator handle and a snapshot of the
// singleton names this node is interested in. The same instance
// is shared with the HTTP handler so reads are always against the
// current leader state.
type Service struct {
	mu       sync.Mutex
	database *sql.DB
	coord    *coordinator.Coordinator
	roles    []SingletonRole
	logger   *slog.Logger
	probe    sync.Mutex
}

// Mount wires the cluster status route into the protected router
// group. The endpoint sits behind auth.RequireAuth so an LB probing
// the public /api/health route still works (it doesn't go through
// here). The Service is fetched from the AppDeps service locator
// at request time so main() can construct it after the database
// + coordinator are wired.
func Mount(app *router.AppDeps) {
	h := &handler{app: app}
	app.RegisterProtectedRoutes(func(r chi.Router) {
		r.Get("/cluster/status", h.handleStatus)
	})
}

type handler struct {
	app *router.AppDeps
}

func (h *handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	svc, ok := h.app.LookupService("cluster")
	if !ok {
		// Service wasn't attached yet (boot race). Return 503
		// rather than crash so the LB can demote this node
		// temporarily.
		response.InternalErrorCode(w, r, i18n.MsgCode("err.cluster.not_ready"))
		return
	}
	s := svc.(*Service)
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := Status{
		NodeID:   s.coord.NodeID(),
		Driver:   string(db.DriverOf()),
		Roles:    make(map[string]bool, len(s.roles)),
		Time:     time.Now().UTC(),
		Database: dbStatus{Reachable: s.pingOK(ctx)},
	}
	for _, role := range s.roles {
		status.Roles[string(role)] = s.coord.LeaderOf(ctx, string(role))
	}
	response.OK(w, status)
}

// NewService builds the cluster service. Pass all known
// SingletonRole values you want to expose in the status payload;
// the response includes a map keyed by role name.
func NewService(
	database *sql.DB,
	coord *coordinator.Coordinator,
	logger *slog.Logger,
	roles []SingletonRole,
) *Service {
	return &Service{database: database, coord: coord, logger: logger, roles: roles}
}

// pingOK does a single PING with a 2s budget. Returns true when the
// round-trip succeeds. The probe mutex prevents a slow query from
// running while a fast one is in flight (in practice the LB only
// hits this once per second, but a small lock keeps the worst
// case bounded).
func (s *Service) pingOK(ctx context.Context) bool {
	s.probe.Lock()
	defer s.probe.Unlock()
	if err := s.database.PingContext(ctx); err != nil {
		if s.logger != nil {
			s.logger.Warn("cluster: db ping failed", "error", err)
		}
		return false
	}
	return true
}
