// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/coordinator"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
)

// ModeActivePassive is the MCHARBOR_HA_MODE value that makes exactly one node serve.
const ModeActivePassive = "active-passive"

// ActiveLock is the coordinator lock the active node holds. It is a database session
// lock, so it doubles as fencing: an active whose session dies stops being active.
const ActiveLock = "ha-active"

const (
	haTick           = 5 * time.Second
	statePreferred   = "ha_preferred_active"
	stateDrainPrefix = "drain:"
)

// Errors the failover controls return; each maps to a refusal the operator can act on.
var (
	ErrNotActivePassive = errors.New("cluster: not in active/passive mode")
	ErrSplitBrain       = errors.New("cluster: more than one node claims to be active")
	ErrUnknownInstance  = errors.New("cluster: no such live node")
)

// Auditor records failover events; the audit logger satisfies it through a small adapter.
type Auditor func(action, nodeID, details string)

// HAController decides this node's role on every tick: in peer mode it does nothing,
// in active/passive mode it competes for ActiveLock and hands it over on request.
type HAController struct {
	mode     string
	coord    *coordinator.Coordinator
	reg      *Registry
	database *sql.DB
	audit    Auditor
	logger   *slog.Logger

	mu        sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	onPromote func(context.Context)
}

// SetOnPromote runs fn each time this node becomes active; the host uses it to fetch
// plugin packages installed while this node was a standby.
func (h *HAController) SetOnPromote(fn func(context.Context)) { h.onPromote = fn }

// NewHAController wires the controller and, in active/passive mode, gates the
// coordinator so a standby wins no singleton job.
func NewHAController(mode string, coord *coordinator.Coordinator, reg *Registry, database *sql.DB, audit Auditor, logger *slog.Logger) *HAController {
	h := &HAController{mode: strings.TrimSpace(mode), coord: coord, reg: reg, database: database, audit: audit, logger: logger}
	if h.ActivePassive() {
		reg.SetRole(RoleStandby)
		middleware.SetStandby(true)
		coord.SetEligible(func(name string) bool { return name == ActiveLock || reg.Role() == RoleActive })
	}
	return h
}

// ActivePassive reports whether this deployment runs one active node.
func (h *HAController) ActivePassive() bool { return h.mode == ModeActivePassive }

// Mode is the configured HA mode, "peer" for a multi-node deployment that chose neither.
func (h *HAController) Mode() string {
	switch h.mode {
	case ModeActivePassive:
		return ModeActivePassive
	case ModeActiveActive:
		return ModeActiveActive
	}
	return "peer"
}

// Start ticks until Stop. The first tick runs at once so a lone node becomes active
// without waiting a full interval.
func (h *HAController) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	h.Tick(ctx)
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		t := time.NewTicker(haTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.Tick(ctx)
			}
		}
	}()
}

// Stop ends the loop and, on an active node, releases the active lock at once so the
// standby takes over on its next tick rather than after the session times out.
func (h *HAController) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
	if h.ActivePassive() && h.reg.Role() == RoleActive {
		ctx, cancel := context.WithTimeout(context.Background(), clusterCallTimeout)
		defer cancel()
		h.demote(ctx, "shutdown")
	}
}

// Tick applies drain requests and settles this node's role once.
func (h *HAController) Tick(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.applyDrain(ctx)
	if !h.ActivePassive() {
		return
	}
	preferred := h.readState(ctx, statePreferred)
	otherPreferred := preferred != "" && preferred != h.reg.NodeID() && h.alive(ctx, preferred)
	draining := h.reg.State() == StateDraining

	if h.reg.Role() == RoleActive {
		switch {
		case otherPreferred:
			h.demote(ctx, "handover to "+preferred)
		case draining:
			h.demote(ctx, "draining")
		case !h.coord.LeaderOf(ctx, ActiveLock):
			h.setRole(ctx, RoleStandby)
			h.record("cluster.active_lost", "the active lock session was lost")
		}
		return
	}
	if draining || otherPreferred {
		h.setRole(ctx, RoleStandby)
		return
	}
	if h.coord.LeaderOf(ctx, ActiveLock) {
		h.setRole(ctx, RoleActive)
		h.record("cluster.promoted", "this node acquired the active lock")
		if h.onPromote != nil {
			h.onPromote(ctx)
		}
		// A completed handover consumes the request, so a later drain and resume of
		// this node never triggers a surprise failback.
		if preferred == h.reg.NodeID() {
			if err := h.deleteState(ctx, statePreferred); err != nil {
				h.logger.Warn("cluster: clearing the failover request failed", "error", err)
			}
		}
	} else {
		h.setRole(ctx, RoleStandby)
	}
}

func (h *HAController) demote(ctx context.Context, why string) {
	if err := h.coord.ReleaseAll(ctx); err != nil {
		h.logger.Error("cluster: releasing locks on demotion failed", "error", err)
	}
	h.setRole(ctx, RoleStandby)
	h.record("cluster.demoted", why)
}

func (h *HAController) setRole(ctx context.Context, role string) {
	if h.reg.Role() == role {
		return
	}
	h.reg.SetRole(role)
	middleware.SetStandby(role == RoleStandby)
	if err := h.reg.Beat(ctx); err != nil {
		h.logger.Warn("cluster: publishing role failed", "role", role, "error", err)
	}
	h.logger.Info("cluster: role changed", "node", h.reg.NodeID(), "role", role)
}

func (h *HAController) record(action, details string) {
	if h.audit != nil {
		h.audit(action, h.reg.NodeID(), details)
	}
}

func (h *HAController) applyDrain(ctx context.Context) {
	want := h.readState(ctx, stateDrainPrefix+h.reg.NodeID()) == "1"
	switch {
	case want && h.reg.State() == StateRunning:
		h.reg.SetState(ctx, StateDraining)
		h.record("cluster.drained", "drain requested")
	case !want && h.reg.State() == StateDraining:
		h.reg.SetState(ctx, StateRunning)
		h.record("cluster.resumed", "drain lifted")
	}
}

func (h *HAController) alive(ctx context.Context, nodeID string) bool {
	list, err := h.reg.List(ctx)
	if err != nil {
		return false
	}
	for _, in := range list {
		if in.ID == nodeID {
			return in.Alive && in.State == StateRunning
		}
	}
	return false
}

// RequestFailover makes target the preferred active; the current active hands over on
// its next tick. It refuses when target is not a live, running node or during a split brain.
func (h *HAController) RequestFailover(ctx context.Context, target string) error {
	if !h.ActivePassive() {
		return ErrNotActivePassive
	}
	list, err := h.reg.List(ctx)
	if err != nil {
		return err
	}
	if ActiveConflict(list) != nil {
		return ErrSplitBrain
	}
	if !h.alive(ctx, target) {
		return ErrUnknownInstance
	}
	if err := h.writeState(ctx, statePreferred, target); err != nil {
		return err
	}
	h.record("cluster.failover.requested", "preferred active set to "+target)
	return nil
}

// SetDrain asks node to drain (or resume); that node's controller applies it on its
// next tick, so any node can drain any other through the shared database.
func (h *HAController) SetDrain(ctx context.Context, node string, drain bool) error {
	list, err := h.reg.List(ctx)
	if err != nil {
		return err
	}
	known := false
	for _, in := range list {
		known = known || (in.ID == node && in.State != StateStopped)
	}
	if !known {
		return ErrUnknownInstance
	}
	if !drain {
		if err := h.deleteState(ctx, stateDrainPrefix+node); err != nil {
			return err
		}
		h.record("cluster.resume.requested", node)
		return nil
	}
	if err := h.writeState(ctx, stateDrainPrefix+node, "1"); err != nil {
		return err
	}
	h.record("cluster.drain.requested", node)
	return nil
}

func (h *HAController) readState(ctx context.Context, name string) string {
	var v string
	err := h.database.QueryRowContext(ctx, `SELECT value FROM cluster_state WHERE name = ?`, name).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		h.logger.Warn("cluster: reading state", "name", name, "error", err)
	}
	return v
}

func (h *HAController) writeState(ctx context.Context, name, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := h.database.ExecContext(ctx, `UPDATE cluster_state SET value = ?, updated_at = ? WHERE name = ?`, value, now, name)
	if err != nil {
		return fmt.Errorf("cluster: writing %s: %w", name, err)
	}
	if db.RowsAffected(res) > 0 {
		return nil
	}
	if _, err := h.database.ExecContext(ctx, `INSERT INTO cluster_state (id, name, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		xid.New().String(), name, value, now, now); err != nil {
		return fmt.Errorf("cluster: writing %s: %w", name, err)
	}
	return nil
}

func (h *HAController) deleteState(ctx context.Context, name string) error {
	if _, err := h.database.ExecContext(ctx, `DELETE FROM cluster_state WHERE name = ?`, name); err != nil {
		return fmt.Errorf("cluster: clearing %s: %w", name, err)
	}
	return nil
}
