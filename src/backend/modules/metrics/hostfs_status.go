// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Host filesystem states, levels and defaults.
const (
	// HostFSStateOK means the newest read succeeded and is fresh.
	HostFSStateOK = "ok"
	// HostFSStateStale means the last read failed or is older than hostFSStaleAfter; the last good sample is still returned.
	HostFSStateStale = "stale"
	// HostFSStateUnknown means no sample was ever seen for the environment.
	HostFSStateUnknown = "unknown"
	// HostFSStateUnsupported means the environment type cannot be measured, so silence is not a finding.
	HostFSStateUnsupported = "unsupported"

	// HostFSLevelNormal is below the warning threshold.
	HostFSLevelNormal = "normal"
	// HostFSLevelWarning is at or above the warning threshold.
	HostFSLevelWarning = "warning"
	// HostFSLevelCritical is at or above the critical threshold.
	HostFSLevelCritical = "critical"

	// DefaultHostFSWarningPercent is the warning threshold when no alert rule sets one.
	DefaultHostFSWarningPercent = 85.0
	// DefaultHostFSCriticalPercent is the critical threshold when no alert rule sets one.
	DefaultHostFSCriticalPercent = 95.0

	hostFSStaleAfter   = 5 * time.Minute
	hostFSRefreshAfter = 60 * time.Second
)

// HostFSThresholds are the percentages the level is computed against.
type HostFSThresholds struct {
	Warning  float64 `json:"warning"`
	Critical float64 `json:"critical"`
}

// HostFSStatus is the GET /metrics/host/fs payload: the last good reading plus
// how much it can be trusted. Level is null unless State is ok, and LastSeen is
// null when no sample was ever seen.
type HostFSStatus struct {
	EnvironmentID string           `json:"environmentId"`
	State         string           `json:"state"`
	Level         *string          `json:"level"`
	Path          string           `json:"path"`
	TotalBytes    int64            `json:"totalBytes"`
	UsedBytes     int64            `json:"usedBytes"`
	Percent       float64          `json:"percent"`
	LastSeen      *time.Time       `json:"lastSeen"`
	Reason        string           `json:"reason,omitempty"`
	Thresholds    HostFSThresholds `json:"thresholds"`
}

// HostFSThresholdSource lets the alert module tell metrics which percentages
// its rules use, so the dashboard colour follows an edited rule.
// A zero return value means "no rule sets this one".
type HostFSThresholdSource interface {
	HostFSThresholds(ctx context.Context, envID string) (warning, critical float64)
}

var (
	thresholdMu     sync.RWMutex
	thresholdSource HostFSThresholdSource
)

// SetHostFSThresholds installs the process-wide threshold source; nil restores the 85/95 defaults.
func SetHostFSThresholds(src HostFSThresholdSource) {
	thresholdMu.Lock()
	thresholdSource = src
	thresholdMu.Unlock()
}

func thresholdsFor(ctx context.Context, envID string) HostFSThresholds {
	thresholdMu.RLock()
	src := thresholdSource
	thresholdMu.RUnlock()
	th := HostFSThresholds{Warning: DefaultHostFSWarningPercent, Critical: DefaultHostFSCriticalPercent}
	if src == nil {
		return th
	}
	w, c := src.HostFSThresholds(ctx, envID)
	if w > 0 {
		th.Warning = w
	}
	if c > 0 {
		th.Critical = c
	}
	return th
}

type hostFSEntry struct {
	last        HostFSUsage
	hasLast     bool
	lastSeen    time.Time
	lastAttempt time.Time
	lastOK      bool
	reason      string
}

// hostFSTrack is package-level because metrics.Service is built per caller; a
// per-Service cache would forget the last good sample between requests.
var hostFSTrack = struct {
	mu      sync.Mutex
	entries map[string]*hostFSEntry
	locks   map[string]*sync.Mutex
}{entries: map[string]*hostFSEntry{}, locks: map[string]*sync.Mutex{}}

var hostFSNow = time.Now

func resetHostFSTracker() {
	hostFSTrack.mu.Lock()
	hostFSTrack.entries = map[string]*hostFSEntry{}
	hostFSTrack.locks = map[string]*sync.Mutex{}
	hostFSTrack.mu.Unlock()
}

// ForgetHostFS drops the tracked reading of envID, for an environment that was deleted or merged away.
func ForgetHostFS(envID string) {
	hostFSTrack.mu.Lock()
	delete(hostFSTrack.entries, envID)
	delete(hostFSTrack.locks, envID)
	hostFSTrack.mu.Unlock()
}

func hostFSTrackedCount() int {
	hostFSTrack.mu.Lock()
	defer hostFSTrack.mu.Unlock()
	return len(hostFSTrack.entries) + len(hostFSTrack.locks)
}

func hostFSLock(key string) *sync.Mutex {
	hostFSTrack.mu.Lock()
	defer hostFSTrack.mu.Unlock()
	l, ok := hostFSTrack.locks[key]
	if !ok {
		l = &sync.Mutex{}
		hostFSTrack.locks[key] = l
	}
	return l
}

// trackKey is the canonical environment id; an unresolvable one is used as given.
func (s *Service) trackKey(ctx context.Context, envID string) string {
	if key, err := s.backend().resolve(ctx, envID); err == nil && key != "" {
		return key
	}
	return envID
}

func (s *Service) recordHostFS(ctx context.Context, envID string, usage HostFSUsage) {
	recordHostFSKey(s.trackKey(ctx, envID), usage, nil)
}

func (s *Service) recordHostFSFailure(ctx context.Context, envID string, err error) {
	recordHostFSKey(s.trackKey(ctx, envID), HostFSUsage{}, err)
}

func recordHostFSKey(key string, usage HostFSUsage, err error) {
	now := hostFSNow()
	hostFSTrack.mu.Lock()
	defer hostFSTrack.mu.Unlock()
	e, ok := hostFSTrack.entries[key]
	if !ok {
		e = &hostFSEntry{}
		hostFSTrack.entries[key] = e
	}
	e.lastAttempt = now
	if err != nil {
		e.lastOK = false
		e.reason = hostFSReason(err)
		return
	}
	e.last, e.hasLast, e.lastSeen, e.lastOK, e.reason = usage, true, now, true, ""
}

func snapshotHostFS(key string) (hostFSEntry, bool) {
	hostFSTrack.mu.Lock()
	defer hostFSTrack.mu.Unlock()
	if e, ok := hostFSTrack.entries[key]; ok {
		return *e, true
	}
	return hostFSEntry{}, false
}

// HostFSStatus returns the tracked reading for envID, re-reading the host first
// when the last attempt is older than a minute. It never calls Docker. The only
// errors are an environment that cannot be resolved or is not stored, checked
// before the tracker so a hostile ?env= value cannot grow it.
func (s *Service) HostFSStatus(ctx context.Context, envID string) (HostFSStatus, error) {
	key, err := s.backend().resolve(ctx, envID)
	if err != nil {
		return HostFSStatus{}, fmt.Errorf("%w: %w", ErrHostFSEnvNotFound, err)
	}
	found, err := s.backend().exists(ctx, key)
	if err != nil {
		return HostFSStatus{}, err
	}
	if !found {
		ForgetHostFS(key)
		return HostFSStatus{}, ErrHostFSEnvNotFound
	}
	status := HostFSStatus{EnvironmentID: key, Path: "/", Thresholds: thresholdsFor(ctx, key)}

	lock := hostFSLock(key)
	lock.Lock()
	e, seen := snapshotHostFS(key)
	if !seen || hostFSNow().Sub(e.lastAttempt) >= hostFSRefreshAfter {
		usage, rerr := s.readHostFS(ctx, key)
		if errors.Is(rerr, ErrHostFSUnsupported) {
			lock.Unlock()
			status.State = HostFSStateUnsupported
			return status, nil
		}
		if rerr != nil {
			recordHostFSKey(key, HostFSUsage{}, rerr)
		} else {
			recordHostFSKey(key, usage, nil)
		}
		e, _ = snapshotHostFS(key)
	}
	lock.Unlock()

	if !e.hasLast {
		if used, total, at, ok := s.lastCapacitySample(ctx, key); ok {
			e.last, e.hasLast, e.lastSeen = newHostFSUsage(total, used), true, at
		}
	}
	return finishHostFSStatus(status, e), nil
}

func finishHostFSStatus(status HostFSStatus, e hostFSEntry) HostFSStatus {
	status.Reason = e.reason
	if !e.hasLast {
		status.State = HostFSStateUnknown
		return status
	}
	seen := e.lastSeen
	status.LastSeen = &seen
	status.Path, status.TotalBytes, status.UsedBytes, status.Percent = e.last.Path, e.last.Total, e.last.Used, e.last.Percent
	if !e.lastOK || hostFSNow().Sub(e.lastSeen) > hostFSStaleAfter {
		status.State = HostFSStateStale
		return status
	}
	status.State = HostFSStateOK
	level := HostFSLevelNormal
	switch {
	case e.last.Percent >= status.Thresholds.Critical:
		level = HostFSLevelCritical
	case e.last.Percent >= status.Thresholds.Warning:
		level = HostFSLevelWarning
	}
	status.Level = &level
	return status
}

// lastCapacitySample is the restart fallback: the capacity collector's newest
// rootfs row survives a restart and is readable on a full disk. It returns
// updated_at, not sampled_at, because the latter is truncated to the hour bucket
// and would make "last seen" up to an hour older than the real reading. A missing row
// and a failed read both mean "nothing to fall back to".
func (s *Service) lastCapacitySample(ctx context.Context, envID string) (used, total int64, at time.Time, ok bool) {
	if s.db == nil {
		return 0, 0, time.Time{}, false
	}
	var stamp any
	err := s.db.QueryRowContext(ctx,
		`SELECT used_bytes, total_bytes, updated_at FROM capacity_samples
		 WHERE environment_id = ? AND subject_kind = 'rootfs' AND total_bytes > 0
		 ORDER BY sampled_at DESC LIMIT 1`, envID).Scan(&used, &total, &stamp)
	if err != nil {
		return 0, 0, time.Time{}, false
	}
	at, perr := parseStoredTime(stamp)
	if perr != nil {
		return 0, 0, time.Time{}, false
	}
	return used, total, at.UTC(), true
}

// parseStoredTime reads an updated_at value in any shape the supported drivers return.
func parseStoredTime(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case []byte:
		return parseStoredTimeString(string(t))
	case string:
		return parseStoredTimeString(t)
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp type %T", v)
}

// parseStoredTimeString tries the SQLite/MySQL shapes and the Postgres timestamptz
// text form (`2026-10-22 10:47:30.123456+00`, offsets `+00`, `+02` or `+02:00`),
// because capacity_samples.updated_at is TEXT on Postgres and defaults to CURRENT_TIMESTAMP.
func parseStoredTimeString(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05", time.RFC3339Nano, "2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999999-07", "2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-07:00:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", s)
}
