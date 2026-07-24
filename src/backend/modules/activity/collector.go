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
	"github.com/rs/xid"

	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
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

// pruneBatchSize caps how many rows a single prune DELETE removes
// in one transaction. Bounded deletes let other writers (metrics
// collector, backup progress) interleave between batches so the
// SQLite write lock is held for milliseconds per batch instead of
// for the time it takes to scan a multi-million-row table on a
// stale install. The collector's main loop runs prune in a tight
// `for { pruneBatch(); ... }` cycle until the table is at or below
// the retention cutoff, so the operator-facing behaviour is
// identical (everything older than N days goes away), just
// delivered in slices.
const pruneBatchSize = 5000

// prune removes events older than the configured retention period.
//
// The timestamp column is stored as RFC3339 ("2026-05-25T00:00:02Z"),
// while datetime('now', '-N days') returns SQLite's native format
// ("2026-05-25 00:00:00"). A direct string comparison breaks at the
// date boundary because ' ' (0x20) sorts before 'T' (0x54), so the
// cutoff always sorts less than anything stored on the same date and
// nothing is ever deleted. Wrapping both sides in julianday() forces
// SQLite to convert the RFC3339 timestamp to a numeric day count so
// the comparison is chronological rather than lexicographic.
func (c *Collector) prune() {
	retentionSettings := coreSettings.ReadRetentionSettings(c.db)
	days := retentionSettings.ActivityRetentionDays
	if days <= 0 {
		return // 0 = keep forever
	}

	totalDeleted := int64(0)
	for {
		result, err := c.db.Exec(
			"DELETE FROM container_events WHERE id IN (SELECT id FROM container_events WHERE julianday(timestamp) < julianday('now', '-' || ? || ' days') LIMIT ?)",
			days, pruneBatchSize,
		)
		if err != nil {
			c.logger.Error("activity collector: failed to prune old events", "error", err)
			return
		}
		deleted := db.RowsAffected(result)
		totalDeleted += deleted
		if deleted < int64(pruneBatchSize) {
			break
		}
	}
	if totalDeleted > 0 {
		c.logger.Info("container events pruned", "days", days, "rows_deleted", totalDeleted)
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
			c.persistEvent(ctx, envID, event)
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
			c.persistEvent(ctx, envID, event)
		}
	}
}

func (c *Collector) persistEvent(ctx context.Context, envID string, event events.Message) {
	// Map the Docker event type to a lifecycle subject_type. The
	// 'docker' daemon events are skipped — they're noisy and add no
	// value to the lifecycle log.
	subjectType, ok := dockerEventToSubjectType(event.Type)
	if !ok {
		return
	}

	// Resolve the foreign-key to environments. The empty envID is
	// the implicit default (local) environment.
	var envPtr *string
	if envID != "" {
		envPtr = &envID
	} else {
		defaultID := c.getDefaultEnvID()
		if defaultID != "" {
			envPtr = &defaultID
		}
	}

	subjectName := event.Actor.Attributes["name"]
	var namePtr *string
	if subjectName != "" {
		namePtr = &subjectName
	}

	// Build metadata from the event attributes. This is what the
	// user expands to see "container failed because OOMKilled,
	// exit code 137" etc.
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

	severity, state := lifecycleSeverity(event.Type, string(event.Action))

	// Always mirror every event into lifecycle_events so the new
	// Logging tab in the Docker menu has a complete record
	// regardless of subject type. The legacy container_events
	// table is kept in sync for the /activity page so historical
	// reads continue to work.
	if _, err := c.db.ExecContext(ctx, `
		INSERT INTO lifecycle_events
			(id, environment_id, subject_type, subject_id, subject_name,
			 event_type, action, state, severity, metadata, source, timestamp,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'docker', ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`,
		xid.New().String(), envPtr, subjectType, event.Actor.ID, namePtr,
		string(event.Type), string(event.Action), nullIfEmpty(state), severity, metaPtr,
		event.TimeNano, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		c.logger.Debug("activity collector: failed to insert lifecycle event", "error", err)
	}

	// Backward compat: keep the existing /activity page working.
	if event.Type == events.ContainerEventType {
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
}

// dockerEventToSubjectType maps a Docker event.Type to the lifecycle
// log subject_type column. Returns ("", false) for events we don't
// care about (daemon, plugin, etc.).
func dockerEventToSubjectType(t events.Type) (string, bool) {
	switch t {
	case events.ContainerEventType:
		return "container", true
	case events.ImageEventType:
		return "image", true
	case events.VolumeEventType:
		return "volume", true
	case events.NetworkEventType:
		return "network", true
	default:
		return "", false
	}
}

// lifecycleSeverity returns the user-facing badge color for the
// event. The mapping is conservative: 'error' for destroy / die /
// failed states, 'warning' for restart / pause, 'success' for
// start / create / pull, 'info' for everything else.
func lifecycleSeverity(t events.Type, action string) (string, string) {
	var state string
	switch t {
	case events.ContainerEventType:
		switch action {
		case "start", "unpause":
			state = "running"
		case "stop", "kill":
			state = "stopped"
		case "die":
			state = "exited"
		case "pause":
			state = "paused"
		case "create":
			state = "created"
		case "restart":
			state = "restarting"
		case "destroy":
			state = "removed"
		}
	case events.ImageEventType:
		switch action {
		case "pull", "load", "import":
			state = "available"
		case "tag", "untag":
			state = "tagged"
		case "delete":
			state = "removed"
		}
	case events.VolumeEventType:
		switch action {
		case "create":
			state = "created"
		case "mount":
			state = "in_use"
		case "unmount":
			state = "available"
		case "destroy":
			state = "removed"
		}
	case events.NetworkEventType:
		switch action {
		case "create":
			state = "active"
		case "connect", "disconnect":
			state = "active"
		case "destroy":
			state = "removed"
		}
	}
	severity := "info"
	switch action {
	case "destroy", "kill", "die", "oom", "failed", "error":
		severity = "error"
	case "stop", "pause", "restart", "unmount", "delete":
		severity = "warning"
	case "start", "create", "pull", "load", "import", "mount", "connect", "tag", "unpause":
		severity = "success"
	}
	return severity, state
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
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
