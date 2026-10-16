// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package users

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"time"
)

// LifecycleUser is one account with every signal the lifecycle filters read.
// The flags are computed server-side against one clock, so a filter count and
// the rows it selects can never disagree.
type LifecycleUser struct {
	ID               string  `json:"id"`
	Username         string  `json:"username"`
	DisplayName      *string `json:"displayName"`
	AuthProvider     string  `json:"authProvider"`
	Disabled         bool    `json:"disabled"`
	LockedUntil      *string `json:"lockedUntil"`
	Locked           bool    `json:"locked"`
	MFAEnabled       bool    `json:"mfaEnabled"`
	LastLogin        *string `json:"lastLogin"`
	Inactive         bool    `json:"inactive"`
	LastElevatedAt   *string `json:"lastElevatedAt"`
	RecentlyElevated bool    `json:"recentlyElevated"`
	BreakGlass       bool    `json:"breakGlass"`
	CreatedAt        string  `json:"createdAt"`
}

// LifecycleCounts is how many users each filter selects.
type LifecycleCounts struct {
	Total            int `json:"total"`
	Disabled         int `json:"disabled"`
	Locked           int `json:"locked"`
	NoMFA            int `json:"noMfa"`
	Inactive         int `json:"inactive"`
	RecentlyElevated int `json:"recentlyElevated"`
}

// LifecycleReport is the lifecycle view's payload.
type LifecycleReport struct {
	InactiveDays int             `json:"inactiveDays"`
	ElevatedDays int             `json:"elevatedDays"`
	Counts       LifecycleCounts `json:"counts"`
	Users        []LifecycleUser `json:"users"`
}

// Lifecycle reports every account with its lifecycle flags. An account that has
// never signed in counts as inactive from its creation, not from the epoch, so a
// user created this morning is not flagged.
func (s *Service) Lifecycle(ctx context.Context, inactiveDays, elevatedDays int) (*LifecycleReport, error) {
	if inactiveDays <= 0 {
		inactiveDays = 90
	}
	if elevatedDays <= 0 {
		elevatedDays = 7
	}
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339)
	inactiveCutoff := now.AddDate(0, 0, -inactiveDays)
	elevatedCutoff := now.AddDate(0, 0, -elevatedDays).Format(time.RFC3339)

	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.username, u.display_name, COALESCE(u.auth_provider, 'local'),
		(u.disabled_at IS NOT NULL AND u.disabled_at != ''), u.password_locked_until,
		COALESCE((SELECT enabled FROM user_mfa m WHERE m.user_id = u.id), 0), u.last_login,
		(SELECT MAX(COALESCE(e.decided_at, e.created_at)) FROM elevation_requests e WHERE e.requester_id = u.id AND e.status IN ('approved', 'expired') AND e.approver_id IS NOT NULL),
		(SELECT COUNT(*) FROM break_glass_accounts b WHERE b.user_id = u.id),
		(SELECT MAX(started_at) FROM break_glass_sessions g WHERE g.user_id = u.id),
		u.created_at
		FROM users u WHERE u.id NOT IN ('system', '__legacy_owner__') ORDER BY u.username LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reading user lifecycle: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	report := &LifecycleReport{InactiveDays: inactiveDays, ElevatedDays: elevatedDays, Users: []LifecycleUser{}}
	for rows.Next() {
		var u LifecycleUser
		var display, locked, lastLogin, elevated, emergency sql.NullString
		var mfa, breakGlass int
		if err := rows.Scan(&u.ID, &u.Username, &display, &u.AuthProvider, &u.Disabled, &locked, &mfa, &lastLogin, &elevated, &breakGlass, &emergency, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning user lifecycle: %w", err)
		}
		if display.Valid {
			u.DisplayName = &display.String
		}
		if locked.Valid && locked.String != "" {
			u.LockedUntil = &locked.String
			u.Locked = locked.String > nowText
		}
		u.MFAEnabled = mfa == 1
		reference := u.CreatedAt
		if lastLogin.Valid && lastLogin.String != "" {
			u.LastLogin = &lastLogin.String
			reference = lastLogin.String
		}
		u.Inactive = olderThan(reference, inactiveCutoff)
		// A break-glass session is the widest elevation there is, so it counts.
		if emergency.Valid && emergency.String > elevated.String {
			elevated = emergency
		}
		if elevated.Valid && elevated.String != "" {
			u.LastElevatedAt = &elevated.String
			u.RecentlyElevated = elevated.String >= elevatedCutoff
		}
		u.BreakGlass = breakGlass > 0
		report.Users = append(report.Users, u)
		report.Counts.add(u)
	}
	return report, rows.Err()
}

func (c *LifecycleCounts) add(u LifecycleUser) {
	c.Total++
	if u.Disabled {
		c.Disabled++
	}
	if u.Locked {
		c.Locked++
	}
	if !u.MFAEnabled {
		c.NoMFA++
	}
	if u.Inactive {
		c.Inactive++
	}
	if u.RecentlyElevated {
		c.RecentlyElevated++
	}
}

// olderThan parses the RFC3339 and SQLite timestamp shapes the users table holds.
func olderThan(value string, cutoff time.Time) bool {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Before(cutoff)
		}
	}
	return false
}
