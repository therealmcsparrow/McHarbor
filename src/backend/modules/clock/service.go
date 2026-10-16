// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package clock

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

const (
	driftInterval = 6 * time.Hour
)

// Coordinator lets the drift checker run on the cluster leader only, so a multi-node deployment does not probe the reference once per node.
type Coordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// Service runs the boot-time RTC check and the periodic drift check, and keeps the last Status in memory.
type Service struct {
	db          *sql.DB
	auditLog    *coreAudit.Logger
	logger      *slog.Logger
	coordinator Coordinator
	nodeID      string
	client      *http.Client
	cancel      context.CancelFunc
	mu          sync.RWMutex
	last        Status
	lastAlert   string
}

// NewService builds a Service; the HTTP client has a 10s timeout so a dead reference host cannot stall the checker.
func NewService(db *sql.DB, auditLog *coreAudit.Logger, logger *slog.Logger, coord Coordinator, nodeID string) *Service {
	return &Service{
		db:          db,
		auditLog:    auditLog,
		logger:      logger,
		coordinator: coord,
		nodeID:      nodeID,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

// Start launches the checker goroutine, which ends when ctx is cancelled or Stop is called; a nil receiver or missing audit log makes it a no-op.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.auditLog == nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go func() {
		select {
		case <-workerCtx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		s.bootCheck(workerCtx)
		s.driftCheck(workerCtx)
		ticker := time.NewTicker(driftInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				quiesce.Run(func() { s.driftCheck(workerCtx) })
			}
		}
	}()
}

// Stop cancels the checker goroutine started by Start and is safe on a nil or never-started Service.
func (s *Service) Stop() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

// Status returns the last recorded drift verdict, or an unknown placeholder before the first check has finished.
func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.last.CheckedAt != "" {
		return s.last
	}
	settings := coreSettings.ReadAuditSettings(s.db)
	now := time.Now().UTC()
	return Status{Status: "unknown", Source: "startup", SystemTimeUTC: now.Format(time.RFC3339), ThresholdSeconds: settings.ClockDriftThresholdSeconds, CheckedAt: now.Format(time.RFC3339)}
}

func (s *Service) bootCheck(ctx context.Context) {
	settings := coreSettings.ReadAuditSettingsContext(ctx, s.db)
	now := time.Now().UTC()
	status := Status{Status: "ok", Source: "rtc", SystemTimeUTC: now.Format(time.RFC3339), ThresholdSeconds: settings.ClockDriftThresholdSeconds, CheckedAt: now.Format(time.RFC3339), NextScheduledCheckAfter: now.Add(driftInterval).Format(time.RFC3339)}
	rtcDate, err := os.ReadFile("/sys/class/rtc/rtc0/date")
	if err != nil {
		status.Source = "system"
		status.LastError = "rtc_unavailable"
	} else {
		status.BootRTCDate = strings.TrimSpace(string(rtcDate))
	}
	s.store(status)
	s.auditLog.Log(nil, coreAudit.Entry{Action: "clock.boot_check", Severity: "info", EntityType: "clock", Details: fmt.Sprintf("source=%s rtc_date=%s system_time_utc=%s", status.Source, status.BootRTCDate, status.SystemTimeUTC)})
}

func (s *Service) driftCheck(ctx context.Context) {
	if s.coordinator != nil && !s.coordinator.LeaderOf(ctx, "clock-drift-checker") {
		if s.logger != nil {
			s.logger.Debug("clock drift checker: not leader this tick; skipping", "node", s.nodeID)
		}
		return
	}

	settings := coreSettings.ReadAuditSettingsContext(ctx, s.db)
	checkedAt := time.Now().UTC()
	status := Status{Status: "ok", Source: settings.ClockReferenceURL, SystemTimeUTC: checkedAt.Format(time.RFC3339), ThresholdSeconds: settings.ClockDriftThresholdSeconds, CheckedAt: checkedAt.Format(time.RFC3339), NextScheduledCheckAfter: checkedAt.Add(driftInterval).Format(time.RFC3339)}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	reference, err := s.referenceTime(checkCtx, settings.ClockReferenceURL)
	if err != nil {
		status.Status = "error"
		status.LastError = "reference_unavailable"
		s.store(status)
		if s.logger != nil {
			s.logger.Warn("clock drift check failed", "error", err)
		}
		s.auditLog.Log(nil, coreAudit.Entry{Action: "clock.drift_check", Severity: "warning", EntityType: "clock", Details: fmt.Sprintf("status=error reason=reference_unavailable source=%s", settings.ClockReferenceURL)})
		return
	}

	status.ReferenceTimeUTC = reference.Format(time.RFC3339)
	status.DeltaMilliseconds = checkedAt.Sub(reference).Milliseconds()
	severity := "success"
	if math.Abs(float64(status.DeltaMilliseconds)) > float64(settings.ClockDriftThresholdSeconds*1000) {
		status.Status = "drift"
		severity = "error"
		s.notifyDrift(ctx, status)
	} else {
		s.lastAlert = ""
	}
	s.store(status)
	s.auditLog.Log(nil, coreAudit.Entry{Action: "clock.drift_check", Severity: severity, EntityType: "clock", Details: fmt.Sprintf("status=%s delta_ms=%d threshold_seconds=%d reference=%s", status.Status, status.DeltaMilliseconds, status.ThresholdSeconds, status.ReferenceTimeUTC)})
}

func (s *Service) referenceTime(ctx context.Context, referenceURL string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, strings.TrimSpace(referenceURL), nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("creating clock reference request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("checking clock reference: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil && s.logger != nil {
			s.logger.Debug("closing clock reference response body failed", "error", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return time.Time{}, fmt.Errorf("clock reference returned status %d", resp.StatusCode)
	}
	dateHeader := resp.Header.Get("Date")
	if dateHeader == "" {
		return time.Time{}, fmt.Errorf("clock reference date header missing")
	}
	parsed, err := http.ParseTime(dateHeader)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing clock reference date: %w", err)
	}
	return parsed.UTC(), nil
}

func (s *Service) store(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = status
}

func (s *Service) notifyDrift(ctx context.Context, status Status) {
	alertKey := fmt.Sprintf("%d:%d", status.ThresholdSeconds, status.DeltaMilliseconds/1000)
	if alertKey == s.lastAlert {
		return
	}
	s.lastAlert = alertKey
	if err := inapp.CreateBroadcastContext(ctx, s.db, inapp.Record{Level: "error", Title: "Clock drift detected", Message: fmt.Sprintf("System clock drift is %d ms, above the %d second audit threshold", status.DeltaMilliseconds, status.ThresholdSeconds), Action: "clock.drift_check", EntityType: "clock"}); err != nil && s.logger != nil {
		s.logger.Error("clock drift notification failed", "error", err)
	}
}
