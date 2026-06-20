// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service manages the per-container auto-heal preference (a Docker label
// round-trip) and the in-memory per-container state the engine uses for
// backoff/cooldown decisions.
type Service struct {
	pool *docker.ClientPool
	db   *sql.DB

	mu        sync.Mutex
	prefs     map[string]prefState // key = envID + "|" + containerID
	lastHeal  map[string]time.Time
	healCount map[string]int
}

type prefState struct {
	enabled         bool
	wasEverHealthy  bool
	restartCount    int
	lastHealAt      time.Time
}

// NewService creates a new autoheal service.
func NewService(pool *docker.ClientPool, db *sql.DB) *Service {
	return &Service{
		pool:      pool,
		db:        db,
		prefs:     make(map[string]prefState),
		lastHeal:  make(map[string]time.Time),
		healCount: make(map[string]int),
	}
}

// GetPreference returns the in-memory preference for a container. The
// source of truth for whether a container is opted in is the Docker label
// on the container itself; this method just surfaces the runtime state.
func (s *Service) GetPreference(envID, containerID string) Preference {
	key := prefKey(envID, containerID)
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.prefs[key]
	last := ""
	if !st.lastHealAt.IsZero() {
		last = st.lastHealAt.UTC().Format(time.RFC3339)
	}
	return Preference{
		ContainerID:    containerID,
		Enabled:        st.enabled,
		LastHealAt:     last,
		RestartCount:   st.restartCount,
		WasEverHealthy: st.wasEverHealthy,
	}
}

// SetPreference toggles the auto-heal loop for a container. The preference
// is stored in memory only — the Docker SDK's ContainerUpdate does not
// expose label mutation, so we don't write a label on the container itself.
// When the container is recreated the preference is reset and the user
// has to re-enable auto-heal for the new container.
func (s *Service) SetPreference(ctx context.Context, envID, containerID string, enabled bool) error {
	key := prefKey(envID, containerID)
	s.mu.Lock()
	st := s.prefs[key]
	st.enabled = enabled
	if !enabled {
		// Reset heal state on disable so re-enabling starts fresh.
		st.lastHealAt = time.Time{}
		st.restartCount = 0
		st.wasEverHealthy = false
	}
	s.prefs[key] = st
	s.mu.Unlock()
	return nil
}

// MarkHealthy records that a container reached the "healthy" state at least
// once. Until a container is healthy at least once, the engine will not
// auto-restart it (this prevents restart loops for misconfigured
// healthchecks).
func (s *Service) MarkHealthy(envID, containerID string) {
	key := prefKey(envID, containerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.prefs[key]
	st.wasEverHealthy = true
	s.prefs[key] = st
}

// IsEnabled reports whether the in-memory state for a container has it
// opted in. The engine consults this together with the Docker label.
func (s *Service) IsEnabled(envID, containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prefs[prefKey(envID, containerID)].enabled
}

// setEnabledFlag updates the in-memory enabled cache without touching
// Docker. Used by the engine to keep its cache in sync with the labels it
// observes during evaluation.
func (s *Service) setEnabledFlag(envID, containerID string, enabled bool) {
	key := prefKey(envID, containerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.prefs[key]
	st.enabled = enabled
	s.prefs[key] = st
}

// LastHealAt returns the timestamp of the last auto-heal restart for the
// container, or the zero time if none.
func (s *Service) LastHealAt(envID, containerID string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prefs[prefKey(envID, containerID)].lastHealAt
}

// RecordHeal marks a heal action and returns true when the engine should
// proceed (within backoff). Returns false to skip the heal.
func (s *Service) RecordHeal(envID, containerID string, cooldown time.Duration) bool {
	key := prefKey(envID, containerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.prefs[key]
	if !st.lastHealAt.IsZero() && time.Since(st.lastHealAt) < cooldown {
		return false
	}
	st.lastHealAt = time.Now().UTC()
	st.restartCount++
	s.prefs[key] = st
	return true
}

// ResetForContainer clears the in-memory state for a container. Call this
// when the container is recreated so the engine treats the new container
// as fresh.
func (s *Service) ResetForContainer(envID, containerID string) {
	key := prefKey(envID, containerID)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prefs, key)
	delete(s.lastHeal, key)
	delete(s.healCount, key)
}

// Restart restarts a container via the Docker SDK.
func (s *Service) Restart(ctx context.Context, envID, containerID string) error {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return fmt.Errorf("getting docker client: %w", err)
	}
	timeout, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return cli.ContainerRestart(timeout, containerID, container.StopOptions{})
}

// Inspect returns the container's running state and health.
func (s *Service) Inspect(ctx context.Context, envID, containerID string) (*Inspect, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}
	info, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}
	out := &Inspect{}
	if info.State != nil {
		st := &InspectState{
			Running:    info.State.Running,
			StartedAt:  info.State.StartedAt,
			FinishedAt: info.State.FinishedAt,
		}
		if info.State.Health != nil {
			st.Health = &Health{Status: string(info.State.Health.Status)}
		}
		out.State = st
	}
	return out, nil
}

// ListRunning returns all running containers in the environment.
func (s *Service) ListRunning(ctx context.Context, envID string) ([]Container, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client: %w", err)
	}
	items, err := cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	out := make([]Container, 0, len(items))
	for _, c := range items {
		// Strip leading slash from container name.
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out = append(out, Container{
			ID:     c.ID,
			Name:   name,
			State:  c.State,
			Labels: c.Labels,
		})
	}
	return out, nil
}

// setLabel and unsetLabel are not yet implemented because the Docker Go
// SDK's ContainerUpdate does not expose label mutation. The current
// implementation keeps the preference in memory only. When the Docker SDK
// gains label support we can call into it here to make the preference
// visible via `docker inspect`.

func prefKey(envID, containerID string) string {
	return envID + "|" + containerID
}

// ErrNoHealthcheck is returned when the engine tries to evaluate a
// container that has no Docker healthcheck configured.
var ErrNoHealthcheck = errors.New("container has no healthcheck")
