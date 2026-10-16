// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
)

// Settings keys for the per-address lockout, seeded by migration 110 and read by
// LoadLockoutPolicy on every login so an admin can tune them without a restart.
const (
	SettingLockoutIPMaxAttempts   = "security.lockout_ip_max_attempts"
	SettingLockoutIPWindowSeconds = "security.lockout_ip_window_seconds"
	SettingLockoutIPLockSeconds   = "security.lockout_ip_lock_seconds"
)

// loginReasonIPLocked marks a login_attempts row refused by the per-address
// lock. Those rows are excluded from the count so a lock does not feed itself.
const loginReasonIPLocked = "ip_locked"

// LoginClient is the attributed origin of a login attempt. IP must come from
// the trusted-proxy resolver, never a raw forwarding header, because per-address
// lockout keys on it; Source records how the resolver arrived at it.
type LoginClient struct {
	IP        string
	Source    string
	UserAgent string
}

// countsIPAddress reports whether failures from ip may be counted or locked.
// An empty or "unknown" address would otherwise pool every unattributable
// client into one bucket, and a trusted proxy only shows up here when
// attribution failed, so locking it would lock everyone behind it.
func countsIPAddress(ip string, policy LockoutPolicy) bool {
	ip = strings.TrimSpace(ip)
	if policy.MaxAttemptsPerIP <= 0 || ip == "" || strings.EqualFold(ip, "unknown") {
		return false
	}
	return !httpx.IsTrustedProxy(ip)
}

// CheckIPLockout reports whether ip is currently locked by the per-address
// threshold and for how long. It fails open on a database error, as CheckLockout
// does, because refusing every login is worse than a missed throttle.
func (s *Service) CheckIPLockout(ip string, policy LockoutPolicy) (bool, time.Duration) {
	return s.CheckIPLockoutContext(context.Background(), ip, policy)
}

// CheckIPLockoutContext is CheckIPLockout bound to the caller's context, so a cancelled request stops the address lock lookup.
func (s *Service) CheckIPLockoutContext(parent context.Context, ip string, policy LockoutPolicy) (bool, time.Duration) {
	if s == nil || s.db == nil || !countsIPAddress(ip, policy) {
		return false, 0
	}
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()

	lockedUntil, err := s.ipLockedUntil(ctx, ip)
	if err != nil || lockedUntil.IsZero() {
		return false, 0
	}
	now := time.Now().UTC()
	if now.Before(lockedUntil) {
		return true, lockedUntil.Sub(now)
	}
	return false, 0
}

// RecordIPFailure counts ip's failed logins inside the window and arms a lock
// when the count reaches the policy threshold, returning the lock duration (0
// when not locked). The failure itself must already be in login_attempts.
func (s *Service) RecordIPFailure(ip string, policy LockoutPolicy) time.Duration {
	return s.RecordIPFailureContext(context.Background(), ip, policy)
}

// RecordIPFailureContext is RecordIPFailure bound to the caller's context, so a cancelled request stops the address failure count.
func (s *Service) RecordIPFailureContext(parent context.Context, ip string, policy LockoutPolicy) time.Duration {
	if s == nil || s.db == nil || !countsIPAddress(ip, policy) {
		return 0
	}
	ctx, cancel := context.WithTimeout(parent, authSettingsQueryTimeout)
	defer cancel()

	now := time.Now().UTC()
	since := now.Add(-policy.IPWindow)

	lockedUntil, err := s.ipLockedUntil(ctx, ip)
	if err != nil {
		slog.Warn("auth: reading address lock failed", "ip", ip, "error", err)
		return 0
	}
	if now.Before(lockedUntil) {
		return lockedUntil.Sub(now)
	}
	// Failures that armed the previous lock were already paid for; counting
	// them again would re-lock the address on its first attempt afterwards.
	if lockedUntil.After(since) {
		since = lockedUntil
	}

	var failures int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM login_attempts
		 WHERE ip_address = ? AND created_at >= ? AND succeeded = 0
		   AND COALESCE(reason, '') <> ?
	`, ip, since.Format(time.RFC3339), loginReasonIPLocked).Scan(&failures); err != nil {
		slog.Warn("auth: counting address failures failed", "ip", ip, "error", err)
		return 0
	}
	if failures < policy.MaxAttemptsPerIP {
		return 0
	}

	nowStr := now.Format(time.RFC3339)
	until := now.Add(policy.IPLock).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO login_ip_locks (id, ip_address, locked_until, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (ip_address) DO UPDATE SET locked_until = excluded.locked_until, updated_at = excluded.updated_at
	`, xid.New().String(), ip, until, nowStr, nowStr); err != nil {
		slog.Warn("auth: recording address lock failed", "ip", ip, "error", err)
		return 0
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM login_ip_locks WHERE locked_until < ?`, now.Add(-policy.IPWindow).Format(time.RFC3339)); err != nil {
		slog.Warn("auth: pruning expired address locks failed", "error", err)
	}
	slog.Warn("auth: client address locked after repeated login failures",
		"ip", ip, "failures", failures, "lock", policy.IPLock)
	return policy.IPLock
}

// ipLockedUntil returns the stored lock expiry for ip, or the zero time when
// the address has never been locked.
func (s *Service) ipLockedUntil(ctx context.Context, ip string) (time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT locked_until FROM login_ip_locks WHERE ip_address = ?`, ip).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		slog.Warn("ignoring unparseable lockout timestamp", "error", err)
		return time.Time{}, nil
	}
	return t, nil
}
