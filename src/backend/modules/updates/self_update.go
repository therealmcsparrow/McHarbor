// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/notify"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// SelfUpdateState is the cached result of the most recent GitHub
// release check. It is persisted in the `settings` table under
// `category = 'self_update'` so the API can return it instantly
// without hitting GitHub on every request.
type SelfUpdateState struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	PublishedAt     string `json:"publishedAt,omitempty"`
	ReleaseNotes    string `json:"releaseNotes,omitempty"`
	LastCheckedAt   string `json:"lastCheckedAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	NextCheckAt     string `json:"nextCheckAt,omitempty"`
	IntervalHours   int    `json:"intervalHours"`
	NotifiedVersion string `json:"notifiedVersion,omitempty"`
}

// SelfUpdateSettings is the operator-tunable configuration that
// drives the periodic update checker.
type SelfUpdateSettings struct {
	IntervalHours   int      `json:"intervalHours"`
	ChannelIDs      []string `json:"channelIds"`
	LastSeenVersion string   `json:"lastSeenVersion"`
	Enabled         bool     `json:"enabled"`
}

// DefaultSelfUpdateSettings returns the operator-facing defaults:
// check every 24 hours, fire on every configured channel, never
// mark a version as seen, run the checker.
func DefaultSelfUpdateSettings() SelfUpdateSettings {
	return SelfUpdateSettings{
		IntervalHours:   24,
		ChannelIDs:      []string{},
		LastSeenVersion: "",
		Enabled:         true,
	}
}

const (
	selfUpdateCategory        = "self_update"
	selfUpdateIntervalKey     = "self_update_interval_hours"
	selfUpdateChannelsKey     = "self_update_channel_ids"
	selfUpdateLastSeenKey      = "self_update_last_seen_version"
	selfUpdateEnabledKey       = "self_update_enabled"
	selfUpdateStateKey         = "self_update_state"
	minUpdateCheckIntervalHr  = 1
	maxUpdateCheckIntervalHr  = 168 // one week
	defaultUpdateCheckURL     = "https://api.github.com/repos/therealmcsparrow/mcharbor/releases/latest"
	selfUpdateHTTPTimeout     = 10 * time.Second
)

// ErrSelfUpdateDisabled indicates the operator has disabled the
// periodic checker.
var ErrSelfUpdateDisabled = errors.New("self-update checker is disabled")

// SelfUpdateChecker runs the periodic GitHub release check, caches
// the result, and emits notifications on new releases.
type SelfUpdateChecker struct {
	db          *sql.DB
	dispatcher  *notify.Dispatcher
	logger      *slog.Logger
	httpClient  *http.Client
	releaseURL  string

	mu    sync.RWMutex
	state SelfUpdateState

	stopCh chan struct{}
}

// NewSelfUpdateChecker creates a checker bound to the dispatcher so
// it can fire notifications on new releases.
func NewSelfUpdateChecker(db *sql.DB, dispatcher *notify.Dispatcher, logger *slog.Logger) *SelfUpdateChecker {
	return &SelfUpdateChecker{
		db:         db,
		dispatcher: dispatcher,
		logger:     logger,
		httpClient: &http.Client{Timeout: selfUpdateHTTPTimeout},
		releaseURL: defaultUpdateCheckURL,
		stopCh:     make(chan struct{}),
	}
}

// Load reads any persisted state from the settings table. Call this
// once during startup so the in-memory cache reflects the most
// recent prior run.
func (c *SelfUpdateChecker) Load(ctx context.Context) error {
	row := c.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE category = ? AND key = ?`,
		selfUpdateCategory, selfUpdateStateKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("loading self-update state: %w", err)
	}
	var state SelfUpdateState
	if err := json.Unmarshal([]byte(value), &state); err != nil {
		return fmt.Errorf("decoding self-update state: %w", err)
	}
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
	return nil
}

// State returns a copy of the cached self-update state.
func (c *SelfUpdateChecker) State() SelfUpdateState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Settings returns the operator-facing self-update configuration.
func (c *SelfUpdateChecker) Settings(ctx context.Context) (SelfUpdateSettings, error) {
	s := DefaultSelfUpdateSettings()

	rows, err := c.db.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE category = ?`, selfUpdateCategory)
	if err != nil {
		return s, fmt.Errorf("loading self-update settings: %w", err)
	}
	defer rows.Close()
	foundInterval := false
	foundEnabled := false
	foundLastSeen := false
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return s, fmt.Errorf("scanning self-update setting: %w", err)
		}
		switch key {
		case selfUpdateIntervalKey:
			if v, err := strconv.Atoi(value); err == nil {
				s.IntervalHours = v
			}
			foundInterval = true
		case selfUpdateChannelsKey:
			s.ChannelIDs = decodeStringList(value)
		case selfUpdateLastSeenKey:
			s.LastSeenVersion = value
			foundLastSeen = true
		case selfUpdateEnabledKey:
			s.Enabled = value == "true"
			foundEnabled = true
		}
	}
	if err := rows.Err(); err != nil {
		return s, fmt.Errorf("iterating self-update settings: %w", err)
	}
	if !foundInterval {
		s.IntervalHours = 24
	}
	if !foundEnabled {
		s.Enabled = true
	}
	if !foundLastSeen {
		s.LastSeenVersion = ""
	}
	return s, nil
}

// SaveSettings persists the operator's self-update configuration.
func (c *SelfUpdateChecker) SaveSettings(ctx context.Context, in SelfUpdateSettings) (SelfUpdateSettings, error) {
	interval := in.IntervalHours
	if interval == 0 {
		interval = 24
	}
	if interval < minUpdateCheckIntervalHr {
		interval = minUpdateCheckIntervalHr
	}
	if interval > maxUpdateCheckIntervalHr {
		interval = maxUpdateCheckIntervalHr
	}
	channels := in.ChannelIDs
	if channels == nil {
		channels = []string{}
	}
	enabled := "false"
	if in.Enabled {
		enabled = "true"
	}
	lastSeen := strings.TrimSpace(in.LastSeenVersion)

	if err := upsertSetting(ctx, c.db, selfUpdateIntervalKey, strconv.Itoa(interval)); err != nil {
		return in, err
	}
	if err := upsertSetting(ctx, c.db, selfUpdateChannelsKey, encodeStringList(channels)); err != nil {
		return in, err
	}
	if err := upsertSetting(ctx, c.db, selfUpdateEnabledKey, enabled); err != nil {
		return in, err
	}
	if err := upsertSetting(ctx, c.db, selfUpdateLastSeenKey, lastSeen); err != nil {
		return in, err
	}
	return SelfUpdateSettings{
		IntervalHours:   interval,
		ChannelIDs:      channels,
		LastSeenVersion: lastSeen,
		Enabled:         in.Enabled,
	}, nil
}

// Dismiss records that the operator has seen the current latest
// version so the in-app banner can stop highlighting it.
func (c *SelfUpdateChecker) Dismiss(ctx context.Context, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("version is required")
	}
	return upsertSetting(ctx, c.db, selfUpdateLastSeenKey, version)
}

// CheckNow performs a single GitHub release check, updates the
// cached state, persists it, and emits notifications on a new
// release. Returns the updated state.
func (c *SelfUpdateChecker) CheckNow(ctx context.Context) (SelfUpdateState, error) {
	settings, err := c.Settings(ctx)
	if err != nil {
		return c.State(), fmt.Errorf("loading settings before check: %w", err)
	}
	if !settings.Enabled {
		return c.State(), ErrSelfUpdateDisabled
	}

	state, err := c.fetchRelease(ctx)
	if err != nil {
		// Persist a sentinel error on the cached state so the UI can
		// surface "could not check for updates" instead of pretending
		// the previous cached state is still authoritative.
		failed := c.State()
		failed.LastError = err.Error()
		failed.LastCheckedAt = time.Now().UTC().Format(time.RFC3339)
		c.persistAndCache(ctx, failed)
		return failed, err
	}

	// Detect "new" version: the latest differs from the previously
	// announced one AND from the version the operator dismissed.
	announceable := state.UpdateAvailable &&
		state.LatestVersion != "" &&
		state.LatestVersion != settings.LastSeenVersion &&
		state.LatestVersion != c.State().NotifiedVersion

	if announceable {
		if err := c.notify(ctx, settings, state); err != nil {
			c.logger.Warn("self-update: notification dispatch failed",
				"version", state.LatestVersion, "error", err)
		}
		state.NotifiedVersion = state.LatestVersion
	}

	c.persistAndCache(ctx, state)
	return state, nil
}

// RunUntilCancelled starts the periodic checker and blocks until
// the context is cancelled or Stop() is called.
func (c *SelfUpdateChecker) RunUntilCancelled(ctx context.Context) {
	c.start(ctx)
	<-ctx.Done()
	c.Stop()
}

// Start kicks off the periodic checker on a background goroutine.
func (c *SelfUpdateChecker) Start(parent context.Context) {
	c.start(parent)
}

// Restart stops the current ticker (if any) and starts a fresh
// one. Used when the operator changes the check interval so the
// new cadence takes effect immediately rather than at the next
// scheduled tick.
func (c *SelfUpdateChecker) Restart(parent context.Context) {
	c.Stop()
	c.start(parent)
}

// Stop signals the periodic checker to exit.
func (c *SelfUpdateChecker) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

func (c *SelfUpdateChecker) start(parent context.Context) {
	go func() {
		for {
			settings, err := c.Settings(parent)
			if err != nil {
				c.logger.Warn("self-update: failed to read settings; sleeping", "error", err)
			} else if !settings.Enabled {
				c.logger.Debug("self-update: disabled; sleeping")
			} else {
				interval := time.Duration(settings.IntervalHours) * time.Hour
				if interval <= 0 {
					interval = 24 * time.Hour
				}
				checkCtx, cancel := context.WithTimeout(parent, 30*time.Second)
				if _, err := c.CheckNow(checkCtx); err != nil &&
					!errors.Is(err, ErrSelfUpdateDisabled) {
					c.logger.Warn("self-update: periodic check failed", "error", err)
				}
				cancel()
				next := c.scheduleNext(parent, interval)
				c.mu.Lock()
				c.state.NextCheckAt = next.UTC().Format(time.RFC3339)
				c.mu.Unlock()
			}
			select {
			case <-parent.Done():
				return
			case <-c.stopCh:
				return
			case <-time.After(time.Hour):
				// Hard cap so a misconfigured interval doesn't pin the
				// goroutine for a week without yielding to the runtime.
			}
		}
	}()
}

func (c *SelfUpdateChecker) scheduleNext(parent context.Context, interval time.Duration) time.Time {
	interval = clampInterval(interval)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return time.Now()
	case <-parent.Done():
		return time.Now()
	case <-c.stopCh:
		return time.Now()
	}
}

func clampInterval(d time.Duration) time.Duration {
	min := time.Duration(minUpdateCheckIntervalHr) * time.Hour
	max := time.Duration(maxUpdateCheckIntervalHr) * time.Hour
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

// fetchRelease hits the GitHub release API and returns the
// resulting SelfUpdateState. Errors at the HTTP / JSON level are
// returned to the caller; the caller's CheckNow handles the
// persistence and notification logic.
func (c *SelfUpdateChecker) fetchRelease(ctx context.Context) (SelfUpdateState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releaseURL, nil)
	if err != nil {
		return SelfUpdateState{}, fmt.Errorf("build github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "McHarbor/"+appversion.Current())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SelfUpdateState{}, fmt.Errorf("github unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SelfUpdateState{}, fmt.Errorf("github returned status %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return SelfUpdateState{}, fmt.Errorf("decode github response: %w", err)
	}

	current := appversion.Current()
	latest := strings.TrimPrefix(rel.TagName, "v")
	notes := rel.Body
	if len(notes) > 2000 {
		notes = notes[:2000]
	}

	return SelfUpdateState{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: latest != "" && latest != current && compareVersions(latest, current) > 0,
		ReleaseURL:      rel.HTMLURL,
		PublishedAt:     rel.PublishedAt,
		ReleaseNotes:    notes,
		LastCheckedAt:   time.Now().UTC().Format(time.RFC3339),
		LastError:       "",
		IntervalHours:   0,
		NotifiedVersion: "",
	}, nil
}

// notify fires the configured channels with a release announcement.
// We intentionally send through the canonical Dispatcher so the
// same transport layer (email / Slack / Discord / Teams / Telegram
// / internal) that the workflows module uses is exercised.
func (c *SelfUpdateChecker) notify(ctx context.Context, settings SelfUpdateSettings, state SelfUpdateState) error {
	if c.dispatcher == nil {
		return errors.New("dispatcher not configured")
	}
	if len(settings.ChannelIDs) == 0 {
		// No channels selected — still write an in-app notification so
		// the user sees the update inside McHarbor.
		_, err := c.dispatcher.SendChannel(ctx, notify.ChannelRequest{
			ChannelType: "internal",
			Title:       fmt.Sprintf("McHarbor %s is available", state.LatestVersion),
			Message: fmt.Sprintf(
				"A new McHarbor release is available. Running %s, latest is %s.\n\n%s",
				state.CurrentVersion, state.LatestVersion, truncateNotes(state.ReleaseNotes, 500),
			),
		})
		return err
	}
	var firstErr error
	for _, id := range settings.ChannelIDs {
		_, err := c.dispatcher.SendChannel(ctx, notify.ChannelRequest{
			ChannelID: id,
			Title:      fmt.Sprintf("McHarbor %s is available", state.LatestVersion),
			Message: fmt.Sprintf(
				"A new McHarbor release is available. Running %s, latest is %s.\n\n%s",
				state.CurrentVersion, state.LatestVersion, truncateNotes(state.ReleaseNotes, 500),
			),
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// persistAndCache stores the new state in the settings table and
// updates the in-memory cache under the same lock.
func (c *SelfUpdateChecker) persistAndCache(ctx context.Context, state SelfUpdateState) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()

	payload, err := json.Marshal(state)
	if err != nil {
		c.logger.Warn("self-update: failed to marshal state", "error", err)
		return
	}
	if err := upsertSetting(ctx, c.db, selfUpdateStateKey, string(payload)); err != nil {
		c.logger.Warn("self-update: failed to persist state", "error", err)
	}
}

func upsertSetting(ctx context.Context, db *sql.DB, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings (category, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
		selfUpdateCategory, key, value, now, now)
	if err != nil {
		return fmt.Errorf("upserting self-update setting %s: %w", key, err)
	}
	return nil
}

func decodeStringList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeStringList(items []string) string {
	if items == nil {
		items = []string{}
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(payload)
}

func truncateNotes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
