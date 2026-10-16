// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import "time"

// APIKeyRoleDetail is one assigned role, with the permissions it carries.
// The detail page needs the permission list; the table does not, which is
// why this lives on the detail DTO and not on APIKey.
type APIKeyRoleDetail struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// APIKeyEnvironmentDetail names an environment the key is fenced to.
type APIKeyEnvironmentDetail struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Key lifecycle states as reported to the UI.
const (
	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
	StatusDeleted = "deleted"
)

// APIKeyDetail is the single-key view. It separates four things the UI must
// never conflate:
//
//   - RoleDetails          what each assigned role would grant
//   - Scopes               permissions granted directly on the key
//   - ConfiguredPermissions the union of the two, i.e. what the key asks for
//   - EffectivePermissions  that union intersected with the owner's rights,
//     i.e. what the key can actually do right now
//
// For a tombstone the effective set is the snapshot taken at deletion and
// EffectiveSnapshot is true, because recomputing it later would silently
// answer a different question.
type APIKeyDetail struct {
	APIKey
	RoleDetails           []APIKeyRoleDetail        `json:"roleDetails"`
	Environments          []APIKeyEnvironmentDetail `json:"environments"`
	ConfiguredPermissions []string                  `json:"configuredPermissions"`
	EffectivePermissions  []string                  `json:"effectivePermissions"`
	EffectiveSnapshot     bool                      `json:"effectiveSnapshot"`
}

// APIKeyUsagePoint is one bucket of the request-count series.
type APIKeyUsagePoint struct {
	Timestamp string `json:"timestamp"`
	Count     int64  `json:"count"`
}

// APIKeyUsageSeries is the graph payload plus the summary cards above it.
//
// CoveredFrom/CoveredTo report what the stored data actually spans, which
// need not be the requested window: a key created yesterday cannot answer a
// 30-day question, and a chart that silently pretends otherwise is worse
// than one that admits the shortfall.
type APIKeyUsageSeries struct {
	APIKeyID          string             `json:"apiKeyId"`
	From              string             `json:"from"`
	To                string             `json:"to"`
	CoveredFrom       string             `json:"coveredFrom,omitempty"`
	CoveredTo         string             `json:"coveredTo,omitempty"`
	BucketSeconds     int                `json:"bucketSeconds"`
	Total             int64              `json:"total"`
	SuccessCount      int64              `json:"successCount"`
	ClientErrorCount  int64              `json:"clientErrorCount"`
	ServerErrorCount  int64              `json:"serverErrorCount"`
	AverageDurationMs float64            `json:"averageDurationMs"`
	UniqueClients     int64              `json:"uniqueClients"`
	Points            []APIKeyUsagePoint `json:"points"`
}

// APIKeyUsageEvent is one recorded request, as returned by the drill-down.
// Every field here is metadata: there is deliberately no header, body, query
// string, token or hash anywhere in this struct.
type APIKeyUsageEvent struct {
	ID             string  `json:"id"`
	APIKeyID       string  `json:"apiKeyId"`
	OccurredAt     string  `json:"occurredAt"`
	Method         string  `json:"method"`
	RoutePattern   string  `json:"routePattern"`
	StatusCode     int     `json:"statusCode"`
	DurationMs     int64   `json:"durationMs"`
	AuthOutcome    string  `json:"authOutcome"`
	EnvironmentID  *string `json:"environmentId"`
	PeerIP         string  `json:"peerIp"`
	ClientIP       string  `json:"clientIp"`
	ClientIPSource string  `json:"clientIpSource"`
	UserAgent      string  `json:"userAgent"`
}

// UsageEventFilter narrows the paginated drill-down. Every field becomes a
// bound parameter; none is ever concatenated into SQL.
type UsageEventFilter struct {
	Page          int
	PerPage       int
	From          time.Time
	To            time.Time
	Method        string
	StatusClass   string // "2xx" | "3xx" | "4xx" | "5xx"
	AuthOutcome   string
	EnvironmentID string
}

// UsagePurgeResult reports what a retention run or a manual clear removed.
type UsagePurgeResult struct {
	EventsDeleted     int64 `json:"eventsDeleted"`
	TombstonesDeleted int64 `json:"tombstonesDeleted"`
	RetentionDays     int   `json:"retentionDays"`
}
