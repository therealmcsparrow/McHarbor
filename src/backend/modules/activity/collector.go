// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package activity records Docker container events and serves them as the activity feed.
package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
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

	// recordExec caches the "record Docker exec events" setting.
	//
	// persistEvent runs on every Docker event — measured at roughly two per
	// second on a modest host, almost all of them healthcheck probes — so it
	// must not query the settings table per event. The flag is refreshed on
	// the same 60s loop that rescans environments, which is far tighter than
	// an operator needs a settings change to take effect.
	recordExec atomic.Bool

	// leader, when set, gates the streams any node could open (socket and TCP
	// environments) so a multi-node install records each Docker event once. Agent
	// environments are not gated: only the node holding the agent can stream them.
	leader func(ctx context.Context) bool

	// containerListener sees container lifecycle events as they arrive. It is
	// an atomic pointer because it is wired after Start, by services built later.
	containerListener atomic.Pointer[ContainerListener]
}

// ContainerListener receives a container lifecycle event: envID is the real
// environment id (the local one resolved), never empty.
type ContainerListener func(ctx context.Context, envID, containerID, containerName, action string)

// SetContainerListener lets another module react to container create, rename
// and start events without opening a second Docker event stream per environment.
func (c *Collector) SetContainerListener(fn ContainerListener) {
	if fn == nil {
		c.containerListener.Store(nil)
		return
	}
	c.containerListener.Store(&fn)
}

// SetLeader installs the leader check; nil keeps the single-node behaviour.
func (c *Collector) SetLeader(fn func(ctx context.Context) bool) { c.leader = fn }

func (c *Collector) mayStream(ctx context.Context, envID string) bool {
	if c.leader == nil || (envID != "" && c.dockerPool.IsAgentEnvContext(ctx, envID)) {
		return true
	}
	return c.leader(ctx)
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

	c.reloadSettings(ctx)
	c.refresh(ctx)
	c.prune(ctx)

	// Re-scan environments every 60 seconds to pick up new ones
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			quiesce.Run(func() {
				c.reloadSettings(ctx)
				c.refresh(ctx)
				c.prune(ctx)
			})
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
// reloadSettings refreshes the cached copies of settings the hot event path
// reads, so persistEvent never touches the database to make a decision.
func (c *Collector) reloadSettings(ctx context.Context) {
	c.recordExec.Store(coreSettings.ReadRetentionSettingsContext(ctx, c.db).RecordExecEvents)
}

// isExecEvent reports whether an action is Docker exec churn.
//
// A healthcheck fires three of these per probe — exec_create, exec_start,
// exec_die — and the action string carries the whole command, so each row is
// large as well as numerous. On the install where this was measured they were
// 95.4% of all stored events. The container's actual health outcome is
// recorded separately and properly, with exit codes, in container_health_probes.
//
// Matching on the prefix rather than the three exact strings is deliberate:
// the action arrives as "exec_start: /bin/sh -c pg_isready …", command and all.
func isExecEvent(action string) bool {
	return strings.HasPrefix(action, "exec_")
}

func (c *Collector) prune(ctx context.Context) {
	retentionSettings := coreSettings.ReadRetentionSettingsContext(ctx, c.db)
	c.pruneLifecycle(ctx, retentionSettings.LifecycleRetentionDays)
	c.pruneAgentLogs(ctx, retentionSettings.ActivityRetentionDays)

	days := retentionSettings.ActivityRetentionDays
	if days <= 0 {
		return // 0 = keep forever
	}

	totalDeleted := int64(0)
	for {
		result, err := c.db.ExecContext(ctx,
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

// pruneLifecycle enforces retention on lifecycle_events.
//
// The table previously had no scheduled prune at all: lifecycle.PurgeByRetention
// existed but was reachable only from a manual HTTP handler, so the table grew
// without bound while container_events beside it was pruned every 60 seconds.
//
// julianday() rather than a string comparison, for the reason documented above
// prune(), and for a second reason specific to this column: rows written before
// migration 130 held int64 nanoseconds, which sort BEFORE any RFC3339 cutoff
// lexicographically and would therefore all have been deleted regardless of
// age. julianday() returns NULL for such a value, so an unconverted row is
// skipped rather than destroyed.
func (c *Collector) pruneLifecycle(ctx context.Context, days int) {
	if days <= 0 {
		return // 0 = keep forever
	}

	totalDeleted := int64(0)
	for {
		result, err := c.db.ExecContext(ctx,
			"DELETE FROM lifecycle_events WHERE id IN (SELECT id FROM lifecycle_events WHERE julianday(timestamp) < julianday('now', '-' || ? || ' days') LIMIT ?)",
			days, pruneBatchSize,
		)
		if err != nil {
			c.logger.Error("activity collector: failed to prune old lifecycle events", "error", err)
			return
		}
		deleted := db.RowsAffected(result)
		totalDeleted += deleted
		if deleted < int64(pruneBatchSize) {
			break
		}
	}
	if totalDeleted > 0 {
		c.logger.Info("lifecycle events pruned", "days", days, "rows_deleted", totalDeleted)
	}
}

// pruneAgentLogs enforces retention on agent_logs (the remote agent's own log,
// replayed from its offline buffer). It rides the activity retention setting
// rather than a new one: the log is operational chatter of the same kind and
// volume as the activity feed, and an operator who shortens one wants the
// other shortened too.
func (c *Collector) pruneAgentLogs(ctx context.Context, days int) {
	if days <= 0 {
		return // 0 = keep forever
	}

	totalDeleted := int64(0)
	for {
		result, err := c.db.ExecContext(ctx,
			"DELETE FROM agent_logs WHERE id IN (SELECT id FROM agent_logs WHERE julianday(logged_at) < julianday('now', '-' || ? || ' days') LIMIT ?)",
			days, pruneBatchSize,
		)
		if err != nil {
			c.logger.Error("activity collector: failed to prune old agent logs", "error", err)
			return
		}
		deleted := db.RowsAffected(result)
		totalDeleted += deleted
		if deleted < int64(pruneBatchSize) {
			break
		}
	}
	if totalDeleted > 0 {
		c.logger.Info("agent logs pruned", "days", days, "rows_deleted", totalDeleted)
	}
}

// activeStreams tracks which environments have an active event listener.
var activeStreams sync.Map

func (c *Collector) refresh(ctx context.Context) {
	envIDs := c.getActiveEnvIDs(ctx)

	// Also listen on the default/local environment (empty string) when enabled.
	if c.isEventTrackingEnabled(ctx, "") {
		envIDs = append(envIDs, "")
	}

	agentSettings := coreSettings.ReadAgentSettingsContext(ctx, c.db)

	for _, envID := range envIDs {
		if !c.mayStream(ctx, envID) {
			continue
		}
		if _, loaded := activeStreams.LoadOrStore(envID, true); loaded {
			continue // already active
		}

		// For agent environments, dispatch based on event mode setting
		if envID != "" && c.dockerPool.IsAgentEnvContext(ctx, envID) {
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

func (c *Collector) getActiveEnvIDs(ctx context.Context) []string {
	// No LIMIT: this is the set of environments to stream from, not a page
	// of them. A cap would silently stop collecting for whatever fell past it.
	rows, err := c.db.QueryContext(ctx, `
		SELECT id
		FROM environments
		WHERE is_active = 1
		  AND track_container_events_enabled = 1
	`)
	if err != nil {
		c.logger.Error("activity collector: failed to query environments", "error", err)
		return nil
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing rows failed", "error", err)
		}
	}()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		c.logger.Error("activity collector: failed to read environments", "error", err)
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

		if !c.isEventTrackingEnabled(ctx, envID) || !c.mayStream(ctx, envID) {
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
	if envID != "" && c.dockerPool.IsAgentEnvContext(ctx, envID) {
		agentSettings := coreSettings.ReadAgentSettingsContext(ctx, c.db)
		if agentSettings.EventMode == "poll" {
			return nil
		}
	}

	cli, err := c.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return err
	}

	stream := cli.Events(ctx, client.EventsListOptions{})
	eventsCh, errCh := stream.Messages, stream.Err
	settingsTicker := time.NewTicker(15 * time.Second)
	defer settingsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-settingsTicker.C:
			if !c.isEventTrackingEnabled(ctx, envID) {
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
	agentSettings := coreSettings.ReadAgentSettingsContext(ctx, c.db)
	lastPoll := time.Now().Add(-time.Duration(agentSettings.EventPollInterval) * time.Second)
	c.lastPollTimes.Store(envID, lastPoll)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !c.isEventTrackingEnabled(ctx, envID) {
			return
		}

		// Re-read settings each cycle to pick up changes
		agentSettings = coreSettings.ReadAgentSettingsContext(ctx, c.db)

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
	cli, err := c.dockerPool.GetContext(ctx, envID)
	if err != nil {
		c.logger.Debug("activity collector: poll get client failed", "env", envID, "error", err)
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	sinceStr := since.Format(time.RFC3339)
	untilStr := until.Format(time.RFC3339)

	stream := cli.Events(fetchCtx, client.EventsListOptions{
		Since: sinceStr,
		Until: untilStr,
	})
	eventsCh, errCh := stream.Messages, stream.Err

	for {
		select {
		case <-fetchCtx.Done():
			return
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
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

	// Drop healthcheck exec churn unless the operator has opted in. This is
	// checked before any work is done, because it discards the large majority
	// of events on a host that uses healthchecks.
	if isExecEvent(string(event.Action)) && !c.recordExec.Load() {
		return
	}

	// Resolve the foreign-key to environments. The empty envID is
	// the implicit default (local) environment.
	var envPtr *string
	if envID != "" {
		envPtr = &envID
	} else {
		defaultID := c.getDefaultEnvID(ctx)
		if defaultID != "" {
			envPtr = &defaultID
		}
	}
	c.notifyContainerListener(ctx, envPtr, event)

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
		// RFC3339, NOT event.TimeNano. Docker reports the event time as an
		// int64 nanosecond count; writing that straight into a column every
		// other reader treats as RFC3339 made the value uncomparable —
		// julianday() returned NULL on it, and a retention DELETE comparing
		// TEXT against a "2026-…" cutoff matched every such row, because '1'
		// sorts before '2'. On the install where this was found that meant the
		// lifecycle "Clear now" button would have deleted all 1.45M rows
		// including ones written seconds earlier. Migration 130 repairs the
		// rows already stored; this is the write that stopped creating them.
		time.Unix(0, event.TimeNano).UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		c.logger.Debug("activity collector: failed to insert lifecycle event", "error", err)
	}

	// Backward compat: keep the existing /activity page working.
	if event.Type == events.ContainerEventType {
		_, err := c.service.Create(ctx, CreateRequest{
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

// notifyContainerListener forwards the events that can give a container a new
// id under an existing name. Compose recreates under a temporary name and
// renames afterwards, so create alone would miss it.
func (c *Collector) notifyContainerListener(ctx context.Context, envPtr *string, event events.Message) {
	if event.Type != events.ContainerEventType || envPtr == nil {
		return
	}
	switch event.Action {
	case events.ActionCreate, events.ActionRename, events.ActionStart:
	default:
		return
	}
	fn := c.containerListener.Load()
	name := strings.TrimPrefix(event.Actor.Attributes["name"], "/")
	if fn == nil || name == "" || event.Actor.ID == "" {
		return
	}
	(*fn)(ctx, *envPtr, event.Actor.ID, name, string(event.Action))
}

// SubjectTypeForDockerEvent is the exported form of dockerEventToSubjectType
// for callers that ingest Docker events from outside the live stream — the
// agent's offline replay — so both paths classify identically.
func SubjectTypeForDockerEvent(eventType string) (string, bool) {
	return dockerEventToSubjectType(events.Type(eventType))
}

// LifecycleSeverity is the exported form of lifecycleSeverity. Returns
// (severity, state).
func LifecycleSeverity(eventType, action string) (string, string) {
	return lifecycleSeverity(events.Type(eventType), action)
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

func (c *Collector) getDefaultEnvID(ctx context.Context) string {
	var id string
	if err := c.db.QueryRowContext(ctx, "SELECT id FROM environments WHERE is_default = 1 LIMIT 1").Scan(&id); err != nil {
		return ""
	}
	return id
}

func (c *Collector) isEventTrackingEnabled(ctx context.Context, envID string) bool {
	if envID == "" {
		envID = c.getDefaultEnvID(ctx)
		if envID == "" {
			return true
		}
	}

	var enabled int
	if err := c.db.QueryRowContext(ctx,
		"SELECT track_container_events_enabled FROM environments WHERE id = ? LIMIT 1",
		envID,
	).Scan(&enabled); err != nil {
		return true
	}

	return enabled == 1
}
