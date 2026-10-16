// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package demo switches McHarbor into and out of demo mode: a separate SQLite
// database of simulated environments, containers, VMs and clusters, served by
// in-process fakes so nothing real is read or touched.
package demo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
)

// Restarter puts a new database into service by restarting the instance; the
// database handle is read once at boot, so there is no hot swap.
type Restarter interface {
	RestartSelf(ctx context.Context) error
}

// RestarterFunc adapts a plain function to Restarter.
type RestarterFunc func(ctx context.Context) error

// RestartSelf calls f.
func (f RestarterFunc) RestartSelf(ctx context.Context) error { return f(ctx) }

// ErrAlreadyActive is returned when entering demo mode while in it.
var ErrAlreadyActive = errors.New("demo mode is already active")

// ErrNotActive is returned when exiting demo mode outside it.
var ErrNotActive = errors.New("demo mode is not active")

// ErrClustered is returned when the instance runs as part of a cluster, where
// one node restarting onto a different database would split it.
var ErrClustered = errors.New("demo mode is unavailable on a clustered instance")

// ErrBusy is returned while another enter, exit or reset is in progress.
var ErrBusy = errors.New("a demo mode change is already in progress")

// Service owns the demo marker, the demo database file and the restart.
type Service struct {
	db        *sql.DB
	dataDir   string
	logger    *slog.Logger
	restarter Restarter
	busy      sync.Mutex
	runtime   *Runtime
}

// NewService returns a Service over the running database and data directory.
func NewService(database *sql.DB, dataDir string, logger *slog.Logger, restarter Restarter) *Service {
	return &Service{db: database, dataDir: dataDir, logger: logger, restarter: restarter}
}

// SetRuntime attaches the running simulation so Status can report its size.
func (s *Service) SetRuntime(rt *Runtime) { s.runtime = rt }

func clustered() bool { return os.Getenv("MCHARBOR_HA_MODE") != "" }

// Status describes demo mode for the banner and the settings card.
func (s *Service) Status() Status {
	st := Status{Available: demoModeAvailable(), Active: coredemo.Active(), Clustered: clustered()}
	if _, err := os.Stat(coredemo.DatabasePath(s.dataDir)); err == nil {
		st.DatabaseExists = true
	}
	st.Defaults = coredemo.DefaultConfig()
	st.Limits = coredemo.DefaultLimits()
	if m := coredemo.Current(); m != nil {
		st.EnteredAt = m.CreatedAt
		cfg := m.Effective()
		st.Config = &cfg
	} else if parked, err := coredemo.ParkedMarker(s.dataDir); err == nil && parked != nil && st.DatabaseExists {
		cfg := parked.Effective()
		st.Config = &cfg
	}
	if s.runtime != nil {
		st.Counts = s.runtime.Counts()
	}
	st.CanEnter = !st.Active && !st.Clustered
	return st
}

func (s *Service) restart(ctx context.Context) Outcome {
	if s.restarter == nil {
		return Outcome{RestartRequired: true}
	}
	if err := s.restarter.RestartSelf(ctx); err != nil {
		s.logger.Warn("demo: automatic restart failed, a manual restart is required", "error", err)
		return Outcome{RestartRequired: true}
	}
	return Outcome{RestartScheduled: true}
}

// Enter prepares demo.db for cfg, writes the marker and restarts onto it. A
// demo.db from an earlier visit is reused, with its secret and refreshed
// accounts, only when it was built for the same sizes; otherwise it is rebuilt.
// A nil cfg means the earlier visit's sizes, or the defaults.
func (s *Service) Enter(ctx context.Context, userID string, cfg *coredemo.Config) (Outcome, error) {
	if !s.busy.TryLock() {
		return Outcome{}, ErrBusy
	}
	defer s.busy.Unlock()
	if coredemo.Active() {
		return Outcome{}, ErrAlreadyActive
	}
	if clustered() {
		return Outcome{}, ErrClustered
	}
	path := coredemo.DatabasePath(s.dataDir)
	parked, err := coredemo.ParkedMarker(s.dataDir)
	if err != nil {
		return Outcome{}, err
	}
	want := coredemo.DefaultConfig()
	if parked != nil {
		want = parked.Effective()
	}
	if cfg != nil {
		want = *cfg
	}
	if err := want.Validate(); err != nil {
		return Outcome{}, err
	}
	_, statErr := os.Stat(path)
	reuse := parked != nil && statErr == nil && parked.Effective() == want
	marker := parked
	if reuse {
		target, err := openDemoDB(path)
		if err != nil {
			return Outcome{}, err
		}
		copyErr := s.copyAccounts(ctx, s.db, target)
		if cerr := target.Close(); cerr != nil {
			s.logger.Warn("demo: closing demo database", "error", cerr)
		}
		if copyErr != nil {
			return Outcome{}, fmt.Errorf("refreshing demo accounts: %w", copyErr)
		}
	} else {
		marker, err = coredemo.NewMarker(userID)
		if err != nil {
			return Outcome{}, err
		}
		if err := s.buildDatabase(ctx, s.db, path); err != nil {
			return Outcome{}, err
		}
	}
	marker.Config = &want
	if err := coredemo.WriteMarker(s.dataDir, marker); err != nil {
		return Outcome{}, err
	}
	if err := coredemo.RemoveParkedMarker(s.dataDir); err != nil {
		return Outcome{}, err
	}
	return s.restart(ctx), nil
}

// Exit parks the marker and restarts onto the real database. demo.db stays so
// the next visit starts where this one ended.
func (s *Service) Exit(ctx context.Context) (Outcome, error) {
	if !s.busy.TryLock() {
		return Outcome{}, ErrBusy
	}
	defer s.busy.Unlock()
	if !coredemo.Active() {
		return Outcome{}, ErrNotActive
	}
	if err := coredemo.ParkMarker(s.dataDir); err != nil {
		return Outcome{}, err
	}
	return s.restart(ctx), nil
}

// Reset throws the demo estate away. Inside demo mode it stages a fresh
// demo.db for cfg (nil keeps the current sizes), installed at the next boot
// because the open file cannot be replaced; outside it deletes demo.db and the
// parked secret.
func (s *Service) Reset(ctx context.Context, userID string, cfg *coredemo.Config) (Outcome, error) {
	if !s.busy.TryLock() {
		return Outcome{}, ErrBusy
	}
	defer s.busy.Unlock()
	if !coredemo.Active() {
		if err := coredemo.RemoveDatabase(s.dataDir); err != nil {
			return Outcome{}, err
		}
		if err := coredemo.RemoveParkedMarker(s.dataDir); err != nil {
			return Outcome{}, err
		}
		return Outcome{}, nil
	}
	want := coredemo.Current().Effective()
	if cfg != nil {
		want = *cfg
	}
	if err := want.Validate(); err != nil {
		return Outcome{}, err
	}
	marker, err := coredemo.NewMarker(userID)
	if err != nil {
		return Outcome{}, err
	}
	marker.Config = &want
	next := coredemo.NextDatabasePath(s.dataDir)
	if err := s.buildDatabase(ctx, s.db, next); err != nil {
		return Outcome{}, err
	}
	if err := coredemo.WriteMarker(s.dataDir, marker); err != nil {
		return Outcome{}, err
	}
	return s.restart(ctx), nil
}
