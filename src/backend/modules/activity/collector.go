// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/docker/docker/api/types/events"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// Collector subscribes to Docker events for all active environments
// and persists container events to the container_events table.
type Collector struct {
	db            *sql.DB
	dockerPool    *docker.ClientPool
	logger        *slog.Logger
	service       *Service
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	lastPollTimes sync.Map // envID -> time.Time
}

// NewCollector creates a new activity event collector.
func NewCollector(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger) *Collector {
	return &Collector{
		db:         db,
		dockerPool: pool,
		logger:     logger,
		service:    NewService(db),
	}
}

// Start launches the background collection goroutines.
func (c *Collector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	c.wg.Add(1)
	go c.run(ctx)
	c.logger.Info("activity collector started")
}

// Stop signals the collector to shut down and waits for completion.
func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

func (c *Collector) run(ctx context.Context) {
	defer c.wg.Done()

	// Short delay to let environments load
	select {
	case <-ctx.Done():
		return
	case <-time.After(3 * time.Second):
	}

	c.refresh(ctx)
	c.prune()

	// Re-scan environments every 60 seconds to pick up new ones
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refresh(ctx)
			c.prune()
		}
	}
}

// prune removes events older than the configured retention period.
func (c *Collector) prune() {
	retentionSettings := coreSettings.ReadRetentionSettings(c.db)
	days := retentionSettings.ActivityRetentionDays
	if days <= 0 {
		return // 0 = keep forever
	}

	_, err := c.db.Exec("DELETE FROM container_events WHERE timestamp < datetime('now', '-' || ? || ' days')", days)
	if err != nil {
		c.logger.Error("activity collector: failed to prune old events", "error", err)
	}
}

// activeStreams tracks which environments have an active event listener.
var activeStreams sync.Map

func (c *Collector) refresh(ctx context.Context) {
	envIDs := c.getActiveEnvIDs()

	// Also listen on the default/local environment (empty string) when enabled.
	if c.isEventTrackingEnabled("") {
		envIDs = append(envIDs, "")
	}

	agentSettings := coreSettings.ReadAgentSettings(c.db)

	for _, envID := range envIDs {
		if _, loaded := activeStreams.LoadOrStore(envID, true); loaded {
			continue // already active
		}

		// For agent environments, dispatch based on event mode setting
		if envID != "" && c.dockerPool.IsAgentEnv(envID) {
			if agentSettings.EventMode == "poll" {
				c.wg.Add(1)
				go c.pollAgentEvents(ctx, envID)
			} else {
				// stream mode - user opted in, allow streaming
				c.wg.Add(1)
				go c.streamEvents(ctx, envID)
			}
			continue
		}

		c.wg.Add(1)
		go c.streamEvents(ctx, envID)
	}
}

func (c *Collector) getActiveEnvIDs() []string {
	rows, err := c.db.Query(`
		SELECT id
		FROM environments
		WHERE is_active = 1
		  AND orchestrator_type = 'docker'
		  AND track_container_events_enabled = 1
	`)
	if err != nil {
		c.logger.Error("activity collector: failed to query environments", "error", err)
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (c *Collector) streamEvents(ctx context.Context, envID string) {
	defer c.wg.Done()
	defer activeStreams.Delete(envID)

	logEnv := envID
	if logEnv == "" {
		logEnv = "local"
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !c.isEventTrackingEnabled(envID) {
			return
		}

		if err := c.listenOnce(ctx, envID); err != nil {
			c.logger.Debug("activity collector: event stream ended", "env", logEnv, "error", err)
		}

		// Backoff before reconnecting
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func (c *Collector) listenOnce(ctx context.Context, envID string) error {
	// For agent environments in poll mode, skip streaming (poll handled separately)
	if envID != "" && c.dockerPool.IsAgentEnv(envID) {
		agentSettings := coreSettings.ReadAgentSettings(c.db)
		if agentSettings.EventMode == "poll" {
			return nil
		}
	}

	cli, err := c.dockerPool.Get(envID)
	if err != nil {
		return err
	}

	eventsCh, errCh := cli.Events(ctx, events.ListOptions{})
	settingsTicker := time.NewTicker(15 * time.Second)
	defer settingsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-settingsTicker.C:
			if !c.isEventTrackingEnabled(envID) {
				return nil
			}
		case err := <-errCh:
			return err
		case event := <-eventsCh:
			c.persistEvent(envID, event)
		}
	}
}

// pollAgentEvents periodically fetches bounded Docker event batches for an agent environment.
func (c *Collector) pollAgentEvents(ctx context.Context, envID string) {
	defer c.wg.Done()
	defer activeStreams.Delete(envID)

	c.logger.Info("activity collector: starting poll mode for agent", "env", envID)

	// Initialize last poll time to now minus one interval
	agentSettings := coreSettings.ReadAgentSettings(c.db)
	lastPoll := time.Now().Add(-time.Duration(agentSettings.EventPollInterval) * time.Second)
	c.lastPollTimes.Store(envID, lastPoll)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !c.isEventTrackingEnabled(envID) {
			return
		}

		// Re-read settings each cycle to pick up changes
		agentSettings = coreSettings.ReadAgentSettings(c.db)

		// If mode changed to stream, exit so refresh() can start a stream goroutine
		if agentSettings.EventMode != "poll" {
			c.logger.Info("activity collector: agent event mode changed to stream, exiting poll", "env", envID)
			return
		}

		now := time.Now()
		if val, ok := c.lastPollTimes.Load(envID); ok {
			lastPoll = val.(time.Time)
		}

		c.fetchEventsBatch(ctx, envID, lastPoll, now)
		c.lastPollTimes.Store(envID, now)

		// Sleep for the configured poll interval
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(agentSettings.EventPollInterval) * time.Second):
		}
	}
}

// fetchEventsBatch fetches Docker events in a bounded time window with a timeout.
func (c *Collector) fetchEventsBatch(ctx context.Context, envID string, since, until time.Time) {
	cli, err := c.dockerPool.Get(envID)
	if err != nil {
		c.logger.Debug("activity collector: poll get client failed", "env", envID, "error", err)
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	sinceStr := since.Format(time.RFC3339)
	untilStr := until.Format(time.RFC3339)

	eventsCh, errCh := cli.Events(fetchCtx, events.ListOptions{
		Since: sinceStr,
		Until: untilStr,
	})

	for {
		select {
		case <-fetchCtx.Done():
			return
		case err := <-errCh:
			if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
				c.logger.Debug("activity collector: poll batch error", "env", envID, "error", err)
			}
			return
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			c.persistEvent(envID, event)
		}
	}
}

func (c *Collector) persistEvent(envID string, event events.Message) {
	// Only persist container events
	if event.Type != events.ContainerEventType {
		return
	}

	containerName := event.Actor.Attributes["name"]
	var envPtr *string
	if envID != "" {
		envPtr = &envID
	} else {
		// Resolve default environment ID for DB foreign key
		defaultID := c.getDefaultEnvID()
		if defaultID != "" {
			envPtr = &defaultID
		}
	}
	var namePtr *string
	if containerName != "" {
		namePtr = &containerName
	}

	// Build metadata from Docker event attributes
	var metaPtr *string
	if attrs := event.Actor.Attributes; len(attrs) > 0 {
		meta := make(map[string]string, len(attrs))
		for k, v := range attrs {
			meta[k] = v
		}
		if b, err := json.Marshal(meta); err == nil {
			s := string(b)
			metaPtr = &s
		}
	}

	_, err := c.service.Create(CreateRequest{
		EnvironmentID: envPtr,
		ContainerID:   event.Actor.ID,
		ContainerName: namePtr,
		EventType:     string(event.Type),
		Action:        string(event.Action),
		Metadata:      metaPtr,
	})
	if err != nil {
		c.logger.Debug("activity collector: failed to persist event", "error", err)
	}
}

func (c *Collector) getDefaultEnvID() string {
	var id string
	if err := c.db.QueryRow("SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id); err != nil {
		return ""
	}
	return id
}

func (c *Collector) isEventTrackingEnabled(envID string) bool {
	if envID == "" {
		envID = c.getDefaultEnvID()
		if envID == "" {
			return true
		}
	}

	var enabled int
	if err := c.db.QueryRow(
		"SELECT track_container_events_enabled FROM environments WHERE id = ? LIMIT 1",
		envID,
	).Scan(&enabled); err != nil {
		return true
	}

	return enabled == 1
}
