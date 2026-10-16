// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// SettingSessionIdleTimeout is the settings key holding the automatic
// sign-out window, in seconds. Seeded by migration 091.
const SettingSessionIdleTimeout = "security.session_idle_timeout_seconds"

const (
	// DefaultIdleTimeoutSeconds matches the seeded settings row.
	DefaultIdleTimeoutSeconds = 600

	// MinIdleTimeoutSeconds is the shortest window the API accepts.
	// Anything below a minute fights the browser's own throttling of
	// background timers and would sign people out mid-sentence.
	MinIdleTimeoutSeconds = 60

	// MaxIdleTimeoutSeconds is a day, which is also the absolute
	// session lifetime — a longer idle window could never fire.
	MaxIdleTimeoutSeconds = 86400

	// sessionPolicyCacheTTL bounds how stale a cached policy may be.
	// The value is read on every authenticated request, so it is not
	// worth a SELECT each time; fifteen seconds is short enough that
	// an admin who tightens the window sees it take effect while they
	// are still looking at the page.
	sessionPolicyCacheTTL = 15 * time.Second
)

// SessionPolicy is the runtime-configurable session behaviour. It is
// separate from SessionDuration (the absolute 24h lifetime), which is
// not admin-tunable.
type SessionPolicy struct {
	// IdleTimeoutSeconds signs a session out after this many seconds
	// without user activity. 0 disables automatic sign-out.
	//
	// "Activity" is what the browser reports through
	// POST /auth/session/touch, not raw request traffic: a dashboard
	// polling in a forgotten tab is the app being busy, not the user.
	IdleTimeoutSeconds int
}

// IdleTimeout returns the window as a duration, or 0 when automatic
// sign-out is disabled.
func (p SessionPolicy) IdleTimeout() time.Duration {
	if p.IdleTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(p.IdleTimeoutSeconds) * time.Second
}

// DefaultSessionPolicy matches the seeded settings row.
func DefaultSessionPolicy() SessionPolicy {
	return SessionPolicy{IdleTimeoutSeconds: DefaultIdleTimeoutSeconds}
}

// LoadSessionPolicy reads the policy from settings, falling back to
// the default for a missing or unparseable row. An out-of-range value
// is clamped rather than rejected: a bad row must not be able to lock
// every user out (0 < v < min) or, worse, silently disable the policy.
func LoadSessionPolicy(ctx context.Context, db *sql.DB) SessionPolicy {
	p := DefaultSessionPolicy()
	if db == nil {
		return p
	}
	v, ok := readSettingInt(ctx, db, SettingSessionIdleTimeout)
	if !ok || v < 0 {
		return p
	}
	p.IdleTimeoutSeconds = ClampIdleTimeout(v)
	return p
}

// ClampIdleTimeout normalises a candidate window. 0 passes through
// untouched because it is the explicit "disabled" value; everything
// else is pulled inside [Min, Max].
func ClampIdleTimeout(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	if seconds < MinIdleTimeoutSeconds {
		return MinIdleTimeoutSeconds
	}
	if seconds > MaxIdleTimeoutSeconds {
		return MaxIdleTimeoutSeconds
	}
	return seconds
}

// sessionPolicyCache memoises the settings lookup for a few seconds.
type sessionPolicyCache struct {
	mu       sync.Mutex
	policy   SessionPolicy
	loadedAt time.Time
}

// SessionPolicy returns the current policy, reading settings at most
// once per sessionPolicyCacheTTL.
func (s *Service) SessionPolicy() SessionPolicy {
	return s.sessionPolicy(context.Background())
}

// SessionPolicyContext is SessionPolicy bound to the caller's context, so a cancelled request stops the settings read on a cache miss.
func (s *Service) SessionPolicyContext(ctx context.Context) SessionPolicy {
	return s.sessionPolicy(ctx)
}

// sessionPolicy is SessionPolicy with the caller's context for the cache-miss settings read.
func (s *Service) sessionPolicy(ctx context.Context) SessionPolicy {
	s.policyCache.mu.Lock()
	defer s.policyCache.mu.Unlock()

	if !s.policyCache.loadedAt.IsZero() &&
		time.Since(s.policyCache.loadedAt) < sessionPolicyCacheTTL {
		return s.policyCache.policy
	}

	ctx, cancel := context.WithTimeout(ctx, authSettingsQueryTimeout)
	defer cancel()

	s.policyCache.policy = LoadSessionPolicy(ctx, s.db)
	s.policyCache.loadedAt = time.Now()
	return s.policyCache.policy
}

// InvalidateSessionPolicy drops the cached policy so the next read
// hits the database. Called after an admin saves a new window.
func (s *Service) InvalidateSessionPolicy() {
	s.policyCache.mu.Lock()
	s.policyCache.loadedAt = time.Time{}
	s.policyCache.mu.Unlock()
}
