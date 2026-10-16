// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// DeniedAuditHook is invoked whenever a permission check denies a
// request. It is set at startup (see SetDeniedAuditHook) and called
// from the various RequirePermission* middlewares. The hook is
// best-effort — failures inside it MUST NOT affect the HTTP
// response (which is already a 403 by the time the hook runs).
type DeniedAuditHook func(r *http.Request, user *auth.User, perm Permission, envID string)

// DeniedAlertHook fires when the same user denies within a short
// window — useful for triggering an alert. The default rate-limit
// is 5 denials within 60s; the second parameter carries the count
// in the current window so callers can decide what to do.
type DeniedAlertHook func(r *http.Request, user *auth.User, perm Permission, recentCount int)

var (
	deniedAuditMu   sync.RWMutex
	deniedAuditHook DeniedAuditHook
	deniedAlertHook DeniedAlertHook
)

// SetDeniedAuditHook installs the hooks. Pass nil to disable.
func SetDeniedAuditHook(auditHook DeniedAuditHook, alertHook DeniedAlertHook) {
	deniedAuditMu.Lock()
	defer deniedAuditMu.Unlock()
	deniedAuditHook = auditHook
	deniedAlertHook = alertHook
}

// RecordDenied is called from the various Require* middleware paths
// after a 403 has been decided. Centralising the call here means
// the policy (rate limit, logging) lives in one place.
//
// `envID` may be empty for global-scope permissions.
func RecordDenied(r *http.Request, user *auth.User, perm Permission, envID string) {
	observeDenied(perm)
	deniedAuditMu.RLock()
	auditHook := deniedAuditHook
	alertHook := deniedAlertHook
	deniedAuditMu.RUnlock()

	if auditHook != nil {
		// Recover from any panic — this runs after the response is
		// already decided.
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("denied audit hook panic", "panic", rec, "perm", perm)
				}
			}()
			auditHook(r, user, perm, envID)
		}()
	}

	if alertHook != nil && user != nil {
		n := deniedRateLimiter.observe(user.ID, perm, time.Now())
		if n >= deniedRateLimiter.threshold {
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						slog.Error("denied alert hook panic", "panic", rec, "perm", perm)
					}
				}()
				alertHook(r, user, perm, n)
			}()
		}
	}
}

// --- internal rate limiter ----------------------------------------

const (
	deniedRateWindow     = 60 * time.Second
	deniedRateThreshold  = 5
	deniedRateBucketSize = 256
)

type deniedRateBucket struct {
	count int
	first time.Time
}

type deniedRateLimiterState struct {
	mu        sync.Mutex
	buckets   map[string]*deniedRateBucket
	threshold int
	window    time.Duration
}

var deniedRateLimiter = &deniedRateLimiterState{
	buckets:   make(map[string]*deniedRateBucket),
	threshold: deniedRateThreshold,
	window:    deniedRateWindow,
}

func (d *deniedRateLimiterState) observe(userID string, perm Permission, now time.Time) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := userID + "|" + string(perm)
	b, ok := d.buckets[key]
	if !ok || now.Sub(b.first) > d.window {
		b = &deniedRateBucket{count: 0, first: now}
		d.buckets[key] = b
		// Bound the map — drop oldest if it grows past the cap.
		if len(d.buckets) >= deniedRateBucketSize {
			d.evictOldest()
		}
	}
	b.count++
	return b.count
}

func (d *deniedRateLimiterState) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	for k, b := range d.buckets {
		if oldestKey == "" || b.first.Before(oldestTime) {
			oldestKey = k
			oldestTime = b.first
		}
	}
	if oldestKey != "" {
		delete(d.buckets, oldestKey)
	}
}

// ConfigureDeniedAlertThreshold overrides the default alert
// threshold (5 denials within 60s). Pass threshold=0 to disable.
func ConfigureDeniedAlertThreshold(threshold int, window time.Duration) {
	deniedRateLimiter.mu.Lock()
	defer deniedRateLimiter.mu.Unlock()
	deniedRateLimiter.threshold = threshold
	if window > 0 {
		deniedRateLimiter.window = window
	}
}

// DefaultDeniedAuditHook is the standard implementation: writes an
// audit log entry with action "permission.denied".
func DefaultDeniedAuditHook(auditLog *audit.Logger) DeniedAuditHook {
	return func(r *http.Request, user *auth.User, perm Permission, envID string) {
		if auditLog == nil {
			return
		}
		if !deniedAuditThrottle.allow(userIDOrEmpty(user)+"|"+string(perm)+"|"+r.URL.Path, time.Now()) {
			return
		}
		details := "permission=" + string(perm) + " env=" + envID + " path=" + r.URL.Path
		auditLog.Log(r, audit.Entry{
			Action:        "permission.denied",
			EntityType:    "user",
			EntityID:      userIDOrEmpty(user),
			EntityName:    usernameOrEmpty(user),
			EnvironmentID: envID,
			Details:       details,
		})
	}
}

func userIDOrEmpty(u *auth.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}

func usernameOrEmpty(u *auth.User) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// ─── ABAC deny hooks ─────────────────────────────────────────────────

// ABACDeniedHook is invoked when an ABAC policy denies a request.
// Best-effort like the role-based hook — panics are recovered.
type ABACDeniedHook func(req AuthzRequest, policyID string, attrs *Attributes)

// var so package-level set/get can race-friendly read.
var (
	abacMu     sync.RWMutex
	abacDenied ABACDeniedHook
)

// SetABACDeniedHook installs the ABAC deny hook. Pass nil to disable.
func SetABACDeniedHook(h ABACDeniedHook) {
	abacMu.Lock()
	defer abacMu.Unlock()
	abacDenied = h
}

// RecordABACDenied records an ABAC deny. Callers from the ABAC
// evaluator pass the resolved attributes and the matched policy id.
func RecordABACDenied(attrs *Attributes, req AuthzRequest, policyID string) {
	abacMu.RLock()
	hook := abacDenied
	abacMu.RUnlock()

	if hook == nil {
		return
	}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("abac denied hook panic", "panic", rec, "policy_id", policyID)
			}
		}()
		hook(req, policyID, attrs)
	}()
}

// DefaultABACDeniedHook writes an audit log entry tagged "abac.denied".
func DefaultABACDeniedHook(auditLog *audit.Logger) ABACDeniedHook {
	return func(req AuthzRequest, policyID string, attrs *Attributes) {
		if auditLog == nil {
			return
		}
		userID := ""
		if attrs != nil && attrs.User != nil {
			userID = attrs.User.ID
		}
		if !deniedAuditThrottle.allow("abac|"+userID+"|"+policyID+"|"+string(req.Permission), time.Now()) {
			return
		}
		details := "policy_id=" + policyID + " perm=" + string(req.Permission)
		if attrs != nil {
			if attrs.Resource != nil {
				details += " resource_type=" + attrs.Resource.Type
				details += " resource_id=" + attrs.Resource.ID
			}
			if attrs.User != nil {
				details += " user_id=" + attrs.User.ID
			}
		}
		auditLog.Log(nil, audit.Entry{
			Action:        "abac.denied",
			EntityType:    "policy",
			EntityID:      policyID,
			EnvironmentID: req.EnvironmentID,
			Details:       details,
		})
	}
}

// auditThrottle keeps the audit chain readable when a page polls a route the
// user may not use: the first denial in a window is recorded, repeats are not.
type auditThrottle struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
}

var deniedAuditThrottle = &auditThrottle{seen: map[string]time.Time{}, window: time.Minute}

func (a *auditThrottle) allow(key string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if last, ok := a.seen[key]; ok && now.Sub(last) < a.window {
		return false
	}
	if len(a.seen) >= 4096 {
		for k, t := range a.seen {
			if now.Sub(t) >= a.window {
				delete(a.seen, k)
			}
		}
		if len(a.seen) >= 4096 {
			a.seen = map[string]time.Time{}
		}
	}
	a.seen[key] = now
	return true
}
