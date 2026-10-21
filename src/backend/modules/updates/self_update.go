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
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/notify"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// SelfUpdateState is the cached result of the most recent GitHub
// release check. It is persisted in the `settings` table under
// `category = 'self_update'` so the API can return it instantly
// without hitting GitHub on every request.
type SelfUpdateState struct {
	CurrentVersion  string `json:"currentVersion"`
	CurrentBuild    string `json:"currentBuild,omitempty"`
	LatestVersion   string `json:"latestVersion"`
	LatestBuild     string `json:"latestBuild,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	PublishedAt     string `json:"publishedAt,omitempty"`
	ReleaseNotes    string `json:"releaseNotes,omitempty"`
	LastCheckedAt   string `json:"lastCheckedAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	NextCheckAt     string `json:"nextCheckAt,omitempty"`
	IntervalHours   int    `json:"intervalHours"`
	// NotifiedVersion is the releaseLabel last announced, so a rebuild of
	// an announced version is announced again.
	NotifiedVersion string `json:"notifiedVersion,omitempty"`
	// AutoInstallVersion is the releaseLabel the automatic installer last
	// scheduled, so a failed or pending install is not re-scheduled on
	// every tick.
	AutoInstallVersion string `json:"autoInstallVersion,omitempty"`
	AutoInstallAt      string `json:"autoInstallAt,omitempty"`
	AutoInstallError   string `json:"autoInstallError,omitempty"`
	// LatestAgentVersion is the agent version the updater targets.
	LatestAgentVersion string `json:"latestAgentVersion,omitempty"`
	AgentsUpdatedAt    string `json:"agentsUpdatedAt,omitempty"`
	// NextInstallAt is when the next install window opens.
	NextInstallAt string `json:"nextInstallAt,omitempty"`
	// InstallDeferred is why an open window has not installed yet, one of
	// the Busy codes; empty when nothing is waiting.
	InstallDeferred string `json:"installDeferred,omitempty"`
	// InstallSkipped is true when the last window closed while still
	// deferred, so the install waits for the next window.
	InstallSkipped bool `json:"installSkipped,omitempty"`
}

// SelfUpdateSettings is the operator-tunable configuration that
// drives the periodic update checker.
type SelfUpdateSettings struct {
	IntervalHours   int      `json:"intervalHours"`
	ChannelIDs      []string `json:"channelIds"`
	LastSeenVersion string   `json:"lastSeenVersion"`
	Enabled         bool     `json:"enabled"`
	// AutoInstall schedules the McHarbor self-update helper as soon as a
	// periodic check finds a newer release. Only the periodic tick does
	// this; a manual "Check now" never restarts the server.
	AutoInstall bool `json:"autoInstall"`
	// AutoUpdateAgents tells connected remote agents that lag the bundled
	// agent version to replace themselves on every periodic tick.
	AutoUpdateAgents bool `json:"autoUpdateAgents"`
	// AgentExcludedEnvIDs lists agent environments that must never be
	// updated by a sweep, automatic or manual.
	AgentExcludedEnvIDs []string `json:"agentExcludedEnvIds"`
	// CheckCron is when to look for a new release.
	CheckCron string `json:"checkCron"`
	// InstallCron opens the window in which an automatic install and the
	// automatic agent sweep may start, so neither lands in working hours.
	InstallCron string `json:"installCron"`
	// InstallWindowMinutes is how long the window stays open while work is
	// still in flight; an install that never finds a quiet moment waits for
	// the next window.
	InstallWindowMinutes int `json:"installWindowMinutes"`
}

// DefaultSelfUpdateSettings returns the operator-facing defaults:
// check every 24 hours, fire on every configured channel, never
// mark a version as seen, run the checker.
func DefaultSelfUpdateSettings() SelfUpdateSettings {
	return SelfUpdateSettings{
		IntervalHours:        24,
		ChannelIDs:           []string{},
		LastSeenVersion:      "",
		Enabled:              true,
		AutoInstall:          false,
		AutoUpdateAgents:     false,
		AgentExcludedEnvIDs:  []string{},
		CheckCron:            defaultCheckCron,
		InstallCron:          defaultInstallCron,
		InstallWindowMinutes: defaultInstallWindowMin,
	}
}

const (
	selfUpdateCategory       = "self_update"
	selfUpdateIntervalKey    = "self_update_interval_hours"
	selfUpdateChannelsKey    = "self_update_channel_ids"
	selfUpdateLastSeenKey    = "self_update_last_seen_version"
	selfUpdateEnabledKey     = "self_update_enabled"
	selfUpdateAutoInstallKey = "self_update_auto_install"
	selfUpdateAutoAgentsKey  = "self_update_auto_update_agents"
	selfUpdateAgentExclKey   = "self_update_agent_excluded_env_ids"
	selfUpdateStateKey       = "self_update_state"
	selfUpdateCheckCronKey   = "self_update_check_cron"
	selfUpdateInstallCronKey = "self_update_install_cron"
	selfUpdateWindowKey      = "self_update_install_window_minutes"
	defaultCheckCron         = "0 4 * * *"
	defaultInstallCron       = "30 4 * * *"
	defaultInstallWindowMin  = 120
	maxInstallWindowMin      = 720
	minUpdateCheckIntervalHr = 1
	maxUpdateCheckIntervalHr = 168 // one week
	defaultUpdateCheckURL    = "https://api.github.com/repos/therealmcsparrow/mcharbor/releases/latest"
	selfUpdateHTTPTimeout    = 10 * time.Second
)

// ErrSelfUpdateDisabled indicates the operator has disabled the
// periodic checker.
var ErrSelfUpdateDisabled = errors.New("self-update checker is disabled")

// SelfUpdateApplier schedules the local McHarbor self-update helper for
// the given image. Wired to core/docker.ApplyLocalSelfUpdate in Mount().
type SelfUpdateApplier func(ctx context.Context, targetImage string) error

// SelfUpdateChecker runs the periodic GitHub release check, caches
// the result, emits notifications on new releases and, when the
// operator opted in, installs the release and updates remote agents.
type SelfUpdateChecker struct {
	db          *sql.DB
	dispatcher  *notify.Dispatcher
	logger      *slog.Logger
	httpClient  *http.Client
	releaseURL  string
	coordinator checkerCoordinator
	nodeID      string
	hasCoord    bool
	applier     SelfUpdateApplier
	agentUpdate *AgentUpdater
	currentImg  func(ctx context.Context) string
	// running and assetAllowed are seams for tests; nil means the real
	// VERSION/BUILD and the GitHub-only allow-list.
	running      func() (version, build string)
	assetAllowed func(*url.URL) bool

	mu    sync.RWMutex
	state SelfUpdateState

	stopCh chan struct{}

	busy BusyChecker
	plan schedulePlan
}

// checkerCoordinator is the subset of the cluster coordinator needed by the
// background self-update checker.
type checkerCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
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

// NewSelfUpdateCheckerWithCoordinator creates a checker that only runs periodic
// release checks on the elected HA leader. Manual checks still use CheckNow
// directly and are not blocked by this guard.
func NewSelfUpdateCheckerWithCoordinator(db *sql.DB, dispatcher *notify.Dispatcher, logger *slog.Logger, coord checkerCoordinator, nodeID string) *SelfUpdateChecker {
	c := NewSelfUpdateChecker(db, dispatcher, logger)
	c.coordinator = coord
	c.nodeID = nodeID
	c.hasCoord = coord != nil
	return c
}

// SetApplier wires the function that schedules the McHarbor self-update
// helper. currentImage resolves the running container's image reference so
// a pinned semver tag can be swapped for the new release.
func (c *SelfUpdateChecker) SetApplier(applier SelfUpdateApplier, currentImage func(ctx context.Context) string) {
	c.applier = applier
	c.currentImg = currentImage
}

// SetAgentUpdater wires the remote agent updater.
func (c *SelfUpdateChecker) SetAgentUpdater(u *AgentUpdater) {
	c.agentUpdate = u
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
	defer func() { closeutil.Report(rows.Close(), "rows") }()
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
		case selfUpdateAutoInstallKey:
			s.AutoInstall = value == "true"
		case selfUpdateAutoAgentsKey:
			s.AutoUpdateAgents = value == "true"
		case selfUpdateAgentExclKey:
			s.AgentExcludedEnvIDs = decodeStringList(value)
		case selfUpdateCheckCronKey:
			if strings.TrimSpace(value) != "" {
				s.CheckCron = value
			}
		case selfUpdateInstallCronKey:
			if strings.TrimSpace(value) != "" {
				s.InstallCron = value
			}
		case selfUpdateWindowKey:
			if v, err := strconv.Atoi(value); err == nil && v > 0 {
				s.InstallWindowMinutes = v
			}
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
	if s.AgentExcludedEnvIDs == nil {
		s.AgentExcludedEnvIDs = []string{}
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
	if err := upsertSetting(ctx, c.db, selfUpdateAutoInstallKey, strconv.FormatBool(in.AutoInstall)); err != nil {
		return in, err
	}
	if err := upsertSetting(ctx, c.db, selfUpdateAutoAgentsKey, strconv.FormatBool(in.AutoUpdateAgents)); err != nil {
		return in, err
	}
	excluded := normalizeIDList(in.AgentExcludedEnvIDs)
	if err := upsertSetting(ctx, c.db, selfUpdateAgentExclKey, encodeStringList(excluded)); err != nil {
		return in, err
	}
	checkCron, installCron, window := normalizeSchedule(in)
	if err := upsertSetting(ctx, c.db, selfUpdateCheckCronKey, checkCron); err != nil {
		return in, err
	}
	if err := upsertSetting(ctx, c.db, selfUpdateInstallCronKey, installCron); err != nil {
		return in, err
	}
	if err := upsertSetting(ctx, c.db, selfUpdateWindowKey, strconv.Itoa(window)); err != nil {
		return in, err
	}
	return SelfUpdateSettings{
		IntervalHours:        interval,
		ChannelIDs:           channels,
		LastSeenVersion:      lastSeen,
		Enabled:              in.Enabled,
		AutoInstall:          in.AutoInstall,
		AutoUpdateAgents:     in.AutoUpdateAgents,
		AgentExcludedEnvIDs:  excluded,
		CheckCron:            checkCron,
		InstallCron:          installCron,
		InstallWindowMinutes: window,
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
	latestLabel := releaseLabel(state.LatestVersion, state.LatestBuild)
	announceable := state.UpdateAvailable &&
		state.LatestVersion != "" &&
		latestLabel != settings.LastSeenVersion &&
		latestLabel != c.State().NotifiedVersion

	if announceable {
		if err := c.notify(ctx, settings, state); err != nil {
			c.logger.Warn("self-update: notification dispatch failed",
				"version", latestLabel, "error", err)
		}
		state.NotifiedVersion = latestLabel
	}

	// Carry the installer / agent bookkeeping over from the previous
	// state: fetchRelease only knows about the GitHub payload.
	prev := c.State()
	state.AutoInstallVersion = prev.AutoInstallVersion
	state.AutoInstallAt = prev.AutoInstallAt
	state.AutoInstallError = prev.AutoInstallError
	state.LatestAgentVersion = prev.LatestAgentVersion
	state.AgentsUpdatedAt = prev.AgentsUpdatedAt
	state.NextCheckAt, state.NextInstallAt = prev.NextCheckAt, prev.NextInstallAt
	state.InstallDeferred, state.InstallSkipped = prev.InstallDeferred, prev.InstallSkipped
	if state.NotifiedVersion == "" {
		state.NotifiedVersion = prev.NotifiedVersion
	}

	c.persistAndCache(ctx, state)
	return state, nil
}

// AutoInstall schedules the self-update helper for the latest release
// when the operator enabled automatic installs, the cached state reports
// a newer version, and that version was not already scheduled. Returns
// true when an install was scheduled; the process exits shortly after.
func (c *SelfUpdateChecker) AutoInstall(ctx context.Context, settings SelfUpdateSettings, state SelfUpdateState) bool {
	if !settings.AutoInstall || c.applier == nil {
		return false
	}
	if !state.UpdateAvailable || state.LatestVersion == "" {
		return false
	}
	label := releaseLabel(state.LatestVersion, state.LatestBuild)
	if state.AutoInstallVersion == label {
		// Already scheduled (or failed) for this release; do not loop.
		return false
	}

	target := ""
	if c.currentImg != nil {
		target = imageForVersion(c.currentImg(ctx), state.LatestVersion)
	}

	state.AutoInstallVersion = label
	state.AutoInstallAt = time.Now().UTC().Format(time.RFC3339)
	state.AutoInstallError = ""

	from := releaseLabel(state.CurrentVersion, state.CurrentBuild)
	c.logger.Info("self-update: automatic install scheduled",
		"from", from, "to", label, "image", target)
	if err := c.applier(ctx, target); err != nil {
		state.AutoInstallError = err.Error()
		c.persistAndCache(ctx, state)
		c.logger.Warn("self-update: automatic install failed", "version", label, "error", err)
		c.notifyText(ctx, settings,
			fmt.Sprintf("McHarbor automatic update to %s failed", label),
			fmt.Sprintf("The automatic update from %s to %s could not be scheduled. Open Settings > About to update manually.",
				from, label))
		return false
	}
	c.persistAndCache(ctx, state)
	c.notifyText(ctx, settings,
		fmt.Sprintf("McHarbor is updating to %s", label),
		fmt.Sprintf("Automatic update from %s to %s has been scheduled. McHarbor restarts in a moment.",
			from, label))
	return true
}

// UpdateAgents runs one agent update sweep and records the outcome in
// the cached state. force bypasses the per-agent retry backoff. Agents on
// the exclusion list are never touched, forced or not.
func (c *SelfUpdateChecker) UpdateAgents(ctx context.Context, force bool, excludedEnvIDs []string) AgentUpdateSummary {
	if c.agentUpdate == nil {
		return AgentUpdateSummary{Results: []AgentUpdateResult{}}
	}
	summary := c.agentUpdate.UpdateOutdated(ctx, force, excludedEnvIDs)
	state := c.State()
	state.LatestAgentVersion = summary.TargetVersion
	state.AgentsUpdatedAt = time.Now().UTC().Format(time.RFC3339)
	c.persistAndCache(ctx, state)
	if summary.Started > 0 || summary.Failed > 0 {
		c.logger.Info("self-update: agent update sweep finished",
			"target", summary.TargetVersion, "started", summary.Started,
			"failed", summary.Failed, "unsupported", summary.Unsupported)
	}
	return summary
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
	// Stop closed the channel; the new loop needs an open one or it exits
	// on its first select.
	c.stopCh = make(chan struct{})
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
	c.mu.Lock()
	c.plan = schedulePlan{}
	c.mu.Unlock()
	stop := c.stopCh
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			c.tick(parent, time.Now())
			select {
			case <-parent.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (c *SelfUpdateChecker) leaderFor(ctx context.Context) bool {
	if !c.hasCoord {
		return true
	}
	return c.coordinator.LeaderOf(ctx, "self-update-checker")
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
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()

	if resp.StatusCode != http.StatusOK {
		return SelfUpdateState{}, fmt.Errorf("github returned status %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return SelfUpdateState{}, fmt.Errorf("decode github response: %w", err)
	}

	current, currentBuild := c.runningRelease()
	latest := strings.TrimPrefix(rel.TagName, "v")
	latestBuild := c.releaseBuild(ctx, rel.Assets)
	if prev := c.State(); latestBuild == "" && prev.LatestVersion == latest {
		// A failed BUILD download must not turn "v+build" back into "v": the
		// label keys notification, install and dismiss, so a flip would redo all three.
		latestBuild = prev.LatestBuild
	}
	notes := rel.Body
	if len(notes) > 2000 {
		notes = notes[:2000]
	}

	return SelfUpdateState{
		CurrentVersion:  current,
		CurrentBuild:    currentBuild,
		LatestVersion:   latest,
		LatestBuild:     latestBuild,
		UpdateAvailable: releaseNewer(current, currentBuild, latest, latestBuild),
		ReleaseURL:      rel.HTMLURL,
		PublishedAt:     rel.PublishedAt,
		ReleaseNotes:    notes,
		LastCheckedAt:   time.Now().UTC().Format(time.RFC3339),
		LastError:       "",
		IntervalHours:   0,
		NotifiedVersion: "",
	}, nil
}

// runningRelease is the version and build of this process.
func (c *SelfUpdateChecker) runningRelease() (version, build string) {
	if c.running != nil {
		return c.running()
	}
	return appversion.Current(), appversion.Build()
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
			Title:       fmt.Sprintf("McHarbor %s is available", releaseLabel(state.LatestVersion, state.LatestBuild)),
			Message: fmt.Sprintf(
				"A new McHarbor release is available. Running %s, latest is %s.\n\n%s",
				releaseLabel(state.CurrentVersion, state.CurrentBuild), releaseLabel(state.LatestVersion, state.LatestBuild),
				truncateNotes(state.ReleaseNotes, 500),
			),
		})
		return err
	}
	var firstErr error
	for _, id := range settings.ChannelIDs {
		_, err := c.dispatcher.SendChannel(ctx, notify.ChannelRequest{
			ChannelID: id,
			Title:     fmt.Sprintf("McHarbor %s is available", releaseLabel(state.LatestVersion, state.LatestBuild)),
			Message: fmt.Sprintf(
				"A new McHarbor release is available. Running %s, latest is %s.\n\n%s",
				releaseLabel(state.CurrentVersion, state.CurrentBuild), releaseLabel(state.LatestVersion, state.LatestBuild),
				truncateNotes(state.ReleaseNotes, 500),
			),
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// notifyText sends a free-form announcement to the configured channels,
// falling back to the in-app channel when none are selected.
func (c *SelfUpdateChecker) notifyText(ctx context.Context, settings SelfUpdateSettings, title, message string) {
	if c.dispatcher == nil {
		return
	}
	if len(settings.ChannelIDs) == 0 {
		if _, err := c.dispatcher.SendChannel(ctx, notify.ChannelRequest{
			ChannelType: "internal", Title: title, Message: message,
		}); err != nil {
			c.logger.Warn("self-update: in-app notification failed", "error", err)
		}
		return
	}
	for _, id := range settings.ChannelIDs {
		if _, err := c.dispatcher.SendChannel(ctx, notify.ChannelRequest{
			ChannelID: id, Title: title, Message: message,
		}); err != nil {
			c.logger.Warn("self-update: channel notification failed", "channel", id, "error", err)
		}
	}
}

// imageForVersion is a seam over core/docker.ImageForVersion so the
// checker stays testable without a Docker client. Set in Mount().
var imageForVersion = func(currentImage, version string) string { return currentImage }

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
		`INSERT INTO settings (id, category, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
		xid.New().String(), selfUpdateCategory, key, value, now, now)
	if err != nil {
		return fmt.Errorf("upserting self-update setting %s: %w", key, err)
	}
	return nil
}

// normalizeIDList trims, de-duplicates and drops empty ids, keeping order.
func normalizeIDList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, id := range items {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
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

func truncateNotes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
