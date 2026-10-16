// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SettingMFARequired is the settings key holding who must enrol a second factor.
// Seeded as "off" by migration 111 so an upgrade never locks anyone out.
const SettingMFARequired = "security.mfa_required"

// MFARequirement is the value of SettingMFARequired.
type MFARequirement string

const (
	// MFARequiredOff leaves MFA optional for everyone.
	MFARequiredOff MFARequirement = "off"
	// MFARequiredAdmins requires MFA of every holder of settings.manage, the key that can
	// turn the policy off again and so the one whose password alone must not suffice.
	MFARequiredAdmins MFARequirement = "admins"
	// MFARequiredAll requires MFA of every interactive user.
	MFARequiredAll MFARequirement = "all"
)

// mfaPolicyCacheTTL bounds how stale the cached requirement may be; it is read on every
// authenticated request, and a write through the settings API invalidates it at once.
const mfaPolicyCacheTTL = 15 * time.Second

// mfaPolicyGeneration is bumped by InvalidateMFAPolicy so every MFAPolicy drops its cache
// without the settings module needing a handle on the instance the mfa module built.
var mfaPolicyGeneration atomic.Uint64

// ParseMFARequirement validates a raw setting value; ok is false for anything but the
// three documented values, which is what the settings API rejects.
func ParseMFARequirement(raw string) (MFARequirement, bool) {
	switch MFARequirement(strings.ToLower(strings.TrimSpace(raw))) {
	case MFARequiredOff:
		return MFARequiredOff, true
	case MFARequiredAdmins:
		return MFARequiredAdmins, true
	case MFARequiredAll:
		return MFARequiredAll, true
	}
	return MFARequiredOff, false
}

// InvalidateMFAPolicy makes the next MFAPolicy read go to the database.
func InvalidateMFAPolicy() {
	mfaPolicyGeneration.Add(1)
}

// IsSettingsAdministrator reports whether userID holds settings.manage globally, which is
// what the "admins" MFA requirement means. The synthetic auth-disabled user is excluded.
func (s *Service) IsSettingsAdministrator(ctx context.Context, userID string) (bool, error) {
	if userID == "" || userID == "system" {
		return false, nil
	}
	return s.HasPermission(ctx, userID, "", PermSettingsManage)
}

// MFAPolicy resolves security.mfa_required for a user. It lives beside the RBAC service
// because "admins" is an RBAC question, and the auth package must not query roles.
type MFAPolicy struct {
	db   *sql.DB
	rbac *Service

	mu         sync.Mutex
	value      MFARequirement
	loadedAt   time.Time
	generation uint64
}

// NewMFAPolicy returns a policy reading db and resolving "admins" through svc.
func NewMFAPolicy(db *sql.DB, svc *Service) *MFAPolicy {
	return &MFAPolicy{db: db, rbac: svc}
}

// Requirement returns the configured requirement. A missing row, an empty value or an
// unrecognised value reads as off: clearing the row is the documented break-glass path.
func (p *MFAPolicy) Requirement(ctx context.Context) (MFARequirement, error) {
	if p == nil || p.db == nil {
		return MFARequiredOff, nil
	}
	generation := mfaPolicyGeneration.Load()
	p.mu.Lock()
	if !p.loadedAt.IsZero() && p.generation == generation && time.Since(p.loadedAt) < mfaPolicyCacheTTL {
		value := p.value
		p.mu.Unlock()
		return value, nil
	}
	p.mu.Unlock()

	var raw sql.NullString
	err := p.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, SettingMFARequired).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MFARequiredOff, fmt.Errorf("reading %s: %w", SettingMFARequired, err)
	}
	value, ok := ParseMFARequirement(raw.String)
	if !ok && strings.TrimSpace(raw.String) != "" {
		slog.Warn("unrecognised MFA requirement treated as off", "key", SettingMFARequired, "value", raw.String)
	}

	p.mu.Lock()
	p.value, p.loadedAt, p.generation = value, time.Now(), generation
	p.mu.Unlock()
	return value, nil
}

// AppliesTo reports whether the requirement covers userID, whether or not that user has
// already enrolled. The synthetic auth-disabled user is never covered.
func (p *MFAPolicy) AppliesTo(ctx context.Context, userID string) (bool, error) {
	if userID == "" || userID == "system" {
		return false, nil
	}
	requirement, err := p.Requirement(ctx)
	if err != nil {
		return false, err
	}
	switch requirement {
	case MFARequiredAll:
		return true, nil
	case MFARequiredAdmins:
		if p.rbac == nil {
			return false, errors.New("mfa policy: admins requirement needs the rbac service")
		}
		return p.rbac.IsSettingsAdministrator(ctx, userID)
	}
	return false, nil
}
