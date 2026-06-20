// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
)

const (
	enginePollInterval = 30 * time.Second
	engineStartupDelay = 10 * time.Second

	// CooldownBackoff steps the cooldown between successive restarts. The
	// first restart is allowed immediately; subsequent restarts must
	// wait progressively longer to break restart loops.
	cooldownStep1 = 30 * time.Second
	cooldownStep2 = 1 * time.Minute
	cooldownStep3 = 2 * time.Minute
	cooldownMax   = 5 * time.Minute
)

// Engine is the background auto-heal poller. It iterates running containers
// across active Docker environments, restarts the ones that opted in
// (via the com.mcharbor.autoheal=enabled label) and are failing their
// healthcheck, with backoff and a "was-ever-healthy" guard to prevent
// restart loops for misconfigured healthchecks.
type Engine struct {
	db       *sql.DB
	logger   *slog.Logger
	svc      *Service
	auditLog *audit.Logger

	sendInApp   func(inapp.Record) error
	sendChannel func(context.Context, corenotify.ChannelRequest) (*corenotify.Delivery, error)

	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewEngine creates a background autoheal engine.
func NewEngine(db *sql.DB, enc *encryption.Service, logger *slog.Logger, svc *Service, auditLog *audit.Logger) *Engine {
	dispatcher := corenotify.NewDispatcher(db, enc)
	return &Engine{
		db:          db,
		logger:      logger,
		svc:         svc,
		auditLog:    auditLog,
		sendInApp:   func(record inapp.Record) error { return inapp.CreateBroadcast(db, record) },
		sendChannel: dispatcher.SendChannel,
	}
}

// Start launches the background auto-heal loop.
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	e.wg.Add(1)
	go e.run(ctx)

	e.logger.Info("autoheal engine started", "interval", enginePollInterval.String())
}

// Stop shuts down the background auto-heal loop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

func (e *Engine) run(ctx context.Context) {
	defer e.wg.Done()

	startup := time.NewTimer(engineStartupDelay)
	ticker := time.NewTicker(enginePollInterval)
	defer startup.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-startup.C:
			e.tick(ctx)
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	envs, err := e.listDockerEnvironments(ctx)
	if err != nil {
		e.logger.Warn("autoheal: list environments failed", "error", err)
		return
	}
	if len(envs) == 0 {
		return
	}

	for _, env := range envs {
		e.evaluateEnvironment(ctx, env.ID, env.Name)
	}
}

type environmentRef struct {
	ID   string
	Name string
}

func (e *Engine) listDockerEnvironments(ctx context.Context) ([]environmentRef, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT id, name
		 FROM environments
		 WHERE is_active = 1
		   AND orchestrator_type = 'docker'
		 ORDER BY is_default DESC, name ASC
		 LIMIT 200`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing docker environments: %w", err)
	}
	defer rows.Close()

	envs := make([]environmentRef, 0, 16)
	for rows.Next() {
		var env environmentRef
		if err := rows.Scan(&env.ID, &env.Name); err != nil {
			return nil, fmt.Errorf("scanning docker environment: %w", err)
		}
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating docker environments: %w", err)
	}
	return envs, nil
}

func (e *Engine) evaluateEnvironment(ctx context.Context, envID, envName string) {
	containers, err := e.svc.ListRunning(ctx, envID)
	if err != nil {
		e.logger.Warn("autoheal: list containers failed", "environment", envName, "error", err)
		return
	}

	for _, c := range containers {
		if !isOptedIn(c.Labels) {
			// Clear the in-memory enabled flag in case the user
			// removed the label since the last cycle.
			e.svc.setEnabledFlag(envID, c.ID, false)
			continue
		}

		// Keep the in-memory enabled flag in sync with the label.
		e.svc.setEnabledFlag(envID, c.ID, true)

		inspect, err := e.svc.Inspect(ctx, envID, c.ID)
		if err != nil {
			e.logger.Warn("autoheal: inspect failed", "container", c.Name, "error", err)
			continue
		}
		if inspect == nil || inspect.State == nil {
			continue
		}
		if !inspect.State.Running {
			continue
		}
		if inspect.State.Health == nil {
			// No healthcheck — autoheal needs a healthcheck to know
			// the container is failing. Skip silently.
			continue
		}

		switch inspect.State.Health.Status {
		case "healthy":
			e.svc.MarkHealthy(envID, c.ID)
		case "unhealthy":
			e.maybeHeal(ctx, envID, envName, c.ID, c.Name)
		}
	}
}

func (e *Engine) maybeHeal(ctx context.Context, envID, envName, containerID, containerName string) {
	if !e.svc.IsEnabled(envID, containerID) {
		return
	}
	pref := e.svc.GetPreference(envID, containerID)
	if !pref.WasEverHealthy {
		// Don't restart a container that has never reached healthy.
		// This is the autoheal default behaviour: avoid loops for
		// containers whose healthcheck is misconfigured.
		e.logger.Debug("autoheal: skip (never healthy)", "container", containerName, "environment", envName)
		return
	}

	cooldown := cooldownFor(pref.RestartCount)
	if !e.svc.RecordHeal(envID, containerID, cooldown) {
		e.logger.Debug("autoheal: skip (cooldown)", "container", containerName, "cooldown", cooldown.String())
		return
	}

	if err := e.svc.Restart(ctx, envID, containerID); err != nil {
		e.logger.Warn("autoheal: restart failed", "container", containerName, "error", err)
		return
	}

	e.logger.Info("autoheal: restarted unhealthy container",
		"container", containerName,
		"environment", envName,
		"restartCount", pref.RestartCount+1,
	)
	e.deliver(ctx, envID, envName, containerID, containerName)
}

func cooldownFor(restartCount int) time.Duration {
	switch {
	case restartCount < 1:
		return cooldownStep1
	case restartCount < 2:
		return cooldownStep2
	case restartCount < 3:
		return cooldownStep3
	default:
		return cooldownMax
	}
}

func (e *Engine) deliver(ctx context.Context, envID, envName, containerID, containerName string) {
	title := fmt.Sprintf("Auto-heal: %s", containerName)
	message := fmt.Sprintf("Container %s in %s failed its healthcheck and was restarted.", containerName, envName)

	if e.auditLog != nil {
		// The audit logger accepts a *http.Request, but the engine runs
		// out of an HTTP context. Pass nil — the logger tolerates nil
		// and only uses the request to attribute the user.
		e.auditLog.Log(nil, audit.Entry{
			Action:        "autoheal",
			EntityType:    "container",
			EntityID:      containerID,
			EntityName:    containerName,
			EnvironmentID: envID,
			Details:       "health=unhealthy",
		})
	}

	if e.sendInApp != nil {
		if err := e.sendInApp(inapp.Record{
			Level:      "warning",
			Title:      title,
			Message:    message,
			Action:     "autoheal.container_restarted",
			EntityType: "container",
			EntityID:   containerID,
		}); err != nil {
			e.logger.Warn("autoheal: in-app delivery failed", "container", containerName, "error", err)
		}
	}
}

// isOptedIn reports whether a container has the autoheal=enabled label.
func isOptedIn(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	v, ok := labels[LabelKey]
	if !ok {
		return false
	}
	return v == LabelEnabled
}
