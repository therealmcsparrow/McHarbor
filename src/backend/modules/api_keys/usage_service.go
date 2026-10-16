// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apikeys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

const (
	// maxUsageWindow bounds one series request. Set well past the default
	// retention so a legitimate long report is never refused, while still
	// rejecting a `from` of 1970 that would scan the whole table.
	maxUsageWindow = 400 * 24 * time.Hour

	// defaultUsageRange is the window when none is asked for.
	defaultUsageRange = 24 * time.Hour

	// maxUsagePoints caps a series. More points than a chart can draw is
	// wasted bytes, and the cap is what stops a hand-crafted narrow bucket
	// from turning one request into a million rows of JSON.
	maxUsagePoints = 1000

	// usagePruneBatchSize matches the activity collector's bounded delete:
	// SQLite's write lock is held per batch, not for a whole table scan.
	usagePruneBatchSize = 5000
)

// usageRanges are the shorthand windows, matching the resource-history
// grammar so an operator who knows one knows the other. The UI offers a
// subset; the endpoint accepts all of them for direct API clients.
var usageRanges = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"12h": 12 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

// ParseUsageWindow resolves the requested window from an explicit from/to
// pair or a range shorthand, mirroring the metrics history parser.
func ParseUsageWindow(rangeName, fromRaw, toRaw string) (time.Time, time.Time, error) {
	now := time.Now().UTC()

	if fromRaw != "" || toRaw != "" {
		from, err := parseUsageTime(fromRaw, now.Add(-defaultUsageRange))
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to, err := parseUsageTime(toRaw, now)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return validateUsageWindow(from, to)
	}

	window := defaultUsageRange
	if rangeName != "" {
		d, ok := usageRanges[rangeName]
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("unknown range %q", rangeName)
		}
		window = d
	}
	return validateUsageWindow(now.Add(-window), now)
}

func parseUsageTime(raw string, fallback time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unparseable timestamp %q", raw)
}

func validateUsageWindow(from, to time.Time) (time.Time, time.Time, error) {
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("window end must be after its start")
	}
	if to.Sub(from) > maxUsageWindow {
		return time.Time{}, time.Time{}, fmt.Errorf("window exceeds %s", maxUsageWindow)
	}
	return from, to, nil
}

// bucketSecondsFor picks a grid that keeps the series under maxUsagePoints
// while landing on a duration a human recognises on an axis.
func bucketSecondsFor(from, to time.Time) int {
	candidates := []int{60, 300, 600, 1800, 3600, 6 * 3600, 12 * 3600, 24 * 3600}
	span := int(to.Sub(from).Seconds())
	for _, bucket := range candidates {
		if span/bucket <= maxUsagePoints {
			return bucket
		}
	}
	return candidates[len(candidates)-1]
}

// UsageSeries returns the request-count graph and the summary above it.
//
// Buckets are zero-filled across the whole window. A sparse series would let
// the area chart join two distant samples with a straight line, drawing a
// slope through hours in which the key did nothing at all — the visual
// opposite of the truth.
func (s *Service) UsageSeries(ctx context.Context, id string, from, to time.Time) (*APIKeyUsageSeries, error) {
	bucket := bucketSecondsFor(from, to)

	series := &APIKeyUsageSeries{
		APIKeyID:      id,
		From:          from.UTC().Format(time.RFC3339),
		To:            to.UTC().Format(time.RFC3339),
		BucketSeconds: bucket,
		Points:        []APIKeyUsagePoint{},
	}

	// Summary first: one pass for every headline figure, so the cards and
	// the graph cannot disagree about the same window.
	var (
		total, success, clientErr, serverErr, uniqueClients int64
		avgDuration                                         float64
		coveredFrom, coveredTo                              *string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN status_code < 400 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END), 0),
		       COALESCE(AVG(duration_ms), 0),
		       COUNT(DISTINCT NULLIF(client_ip, '')),
		       MIN(occurred_at), MAX(occurred_at)
		  FROM api_key_usage_events
		 WHERE api_key_id = ? AND occurred_at >= ? AND occurred_at < ?`,
		id, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
	).Scan(&total, &success, &clientErr, &serverErr, &avgDuration, &uniqueClients, &coveredFrom, &coveredTo)
	if err != nil {
		return nil, fmt.Errorf("summarising api key usage: %w", err)
	}

	series.Total = total
	series.SuccessCount = success
	series.ClientErrorCount = clientErr
	series.ServerErrorCount = serverErr
	series.AverageDurationMs = avgDuration
	series.UniqueClients = uniqueClients
	if coveredFrom != nil {
		series.CoveredFrom = *coveredFrom
	}
	if coveredTo != nil {
		series.CoveredTo = *coveredTo
	}

	// Counts per bucket. strftime('%s') gives epoch seconds, so integer
	// division floors each row onto the grid without a per-row round trip.
	rows, err := s.db.QueryContext(ctx, `
		SELECT (CAST(strftime('%s', occurred_at) AS INTEGER) / ?) * ? AS bucket_start,
		       COUNT(*)
		  FROM api_key_usage_events
		 WHERE api_key_id = ? AND occurred_at >= ? AND occurred_at < ?
		 GROUP BY bucket_start
		 ORDER BY bucket_start ASC
		 LIMIT ?`,
		bucket, bucket, id,
		from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339),
		maxUsagePoints+1,
	)
	if err != nil {
		return nil, fmt.Errorf("querying api key usage series: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	counts := map[int64]int64{}
	for rows.Next() {
		var bucketStart, count int64
		if err := rows.Scan(&bucketStart, &count); err != nil {
			return nil, fmt.Errorf("scanning api key usage bucket: %w", err)
		}
		counts[bucketStart] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api key usage buckets: %w", err)
	}

	step := int64(bucket)
	start := (from.UTC().Unix() / step) * step
	end := to.UTC().Unix()
	for ts := start; ts < end && len(series.Points) < maxUsagePoints; ts += step {
		series.Points = append(series.Points, APIKeyUsagePoint{
			Timestamp: time.Unix(ts, 0).UTC().Format(time.RFC3339),
			Count:     counts[ts],
		})
	}

	return series, nil
}

// UsageEvents returns one page of the request history.
func (s *Service) UsageEvents(ctx context.Context, id string, filter UsageEventFilter) ([]APIKeyUsageEvent, int64, error) {
	where := []string{"api_key_id = ?"}
	args := []any{id}

	if !filter.From.IsZero() {
		where = append(where, "occurred_at >= ?")
		args = append(args, filter.From.UTC().Format(time.RFC3339))
	}
	if !filter.To.IsZero() {
		where = append(where, "occurred_at < ?")
		args = append(args, filter.To.UTC().Format(time.RFC3339))
	}
	if filter.Method != "" {
		where = append(where, "method = ?")
		args = append(args, filter.Method)
	}
	if filter.AuthOutcome != "" {
		where = append(where, "auth_outcome = ?")
		args = append(args, filter.AuthOutcome)
	}
	if filter.EnvironmentID != "" {
		where = append(where, "environment_id = ?")
		args = append(args, filter.EnvironmentID)
	}
	// The status filter is a class, not a code, so it becomes a bound range
	// rather than a substring match on a number.
	if lower, upper, ok := statusClassRange(filter.StatusClass); ok {
		where = append(where, "status_code >= ? AND status_code < ?")
		args = append(args, lower, upper)
	}

	clause := " WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_key_usage_events`+clause, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting api key usage events: %w", err)
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	perPage := filter.PerPage
	if perPage < 1 || perPage > 1000 {
		perPage = 25
	}

	pageArgs := append(append([]any{}, args...), perPage, (page-1)*perPage)
	query := usageEventsSelect + clause + usageEventsTail // SQL identifier: clause is built from fixed predicates with "?" bindings
	rows, err := s.db.QueryContext(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying api key usage events: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()

	events := []APIKeyUsageEvent{}
	for rows.Next() {
		var event APIKeyUsageEvent
		var envID *string
		if err := rows.Scan(&event.ID, &event.APIKeyID, &event.OccurredAt, &event.Method,
			&event.RoutePattern, &event.StatusCode, &event.DurationMs, &event.AuthOutcome,
			&envID, &event.PeerIP, &event.ClientIP, &event.ClientIPSource, &event.UserAgent); err != nil {
			return nil, 0, fmt.Errorf("scanning api key usage event: %w", err)
		}
		event.EnvironmentID = envID
		events = append(events, event)
	}
	return events, total, rows.Err()
}

// statusClassRange turns "4xx" into the half-open range [400, 500).
func statusClassRange(class string) (int, int, bool) {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "2xx":
		return 200, 300, true
	case "3xx":
		return 300, 400, true
	case "4xx":
		return 400, 500, true
	case "5xx":
		return 500, 600, true
	default:
		return 0, 0, false
	}
}

// PurgeUsageByRetention deletes events and tombstones past the configured
// horizon. Events age from when the request happened; tombstones from when
// the key was deleted. Live and revoked keys are never touched here.
func (s *Service) PurgeUsageByRetention(ctx context.Context) (UsagePurgeResult, error) {
	days := coreSettings.ReadRetentionSettingsContext(ctx, s.db).APIKeyUsageRetentionDays
	result := UsagePurgeResult{RetentionDays: days}
	if days <= 0 {
		return result, nil // 0 = keep forever
	}

	events, err := s.pruneOlderThan(ctx, "api_key_usage_events", "occurred_at", days)
	if err != nil {
		return result, err
	}
	tombstones, err := s.pruneOlderThan(ctx, "api_key_tombstones", "deleted_at", days)
	if err != nil {
		return result, err
	}

	result.EventsDeleted = events
	result.TombstonesDeleted = tombstones
	return result, nil
}

// ClearUsage removes every usage event and every tombstone immediately.
//
// It deliberately does not touch api_keys: an operator clearing history is
// saying "forget what happened", not "revoke my automation". Live and revoked
// credentials keep working.
func (s *Service) ClearUsage(ctx context.Context) (UsagePurgeResult, error) {
	result := UsagePurgeResult{RetentionDays: coreSettings.ReadRetentionSettingsContext(ctx, s.db).APIKeyUsageRetentionDays}

	events, err := s.deleteAllBatched(ctx, pruneTableUsageEvents)
	if err != nil {
		return result, err
	}
	tombstones, err := s.deleteAllBatched(ctx, pruneTableTombstones)
	if err != nil {
		return result, err
	}

	result.EventsDeleted = events
	result.TombstonesDeleted = tombstones
	return result, nil
}

// pruneOlderThan deletes in bounded batches so the write lock is held for
// milliseconds at a time rather than for a whole table scan.
//
// julianday() on both sides rather than a string comparison: stored values are
// RFC3339 ("2026-05-25T00:00:02Z") while datetime('now', '-N days') returns
// SQLite's own layout ("2026-05-25 00:00:00"), and ' ' sorts before 'T', so a
// direct comparison silently never matches at the date boundary.
func (s *Service) pruneOlderThan(ctx context.Context, table, column string, days int) (int64, error) {
	// table and column are package constants at every call site, never
	// request input; only the bound parameters below come from outside.
	query := fmt.Sprintf( // SQL identifier: table and column are package constants at every call site
		`DELETE FROM %s WHERE id IN (SELECT id FROM %s WHERE julianday(%s) < julianday('now', '-' || ? || ' days') LIMIT ?)`,
		table, table, column,
	)

	var total int64
	for {
		result, err := s.db.ExecContext(ctx, query, days, usagePruneBatchSize)
		if err != nil {
			return total, fmt.Errorf("pruning %s: %w", table, err)
		}
		deleted := db.RowsAffected(result)
		total += deleted
		if deleted < usagePruneBatchSize {
			return total, nil
		}
	}
}

// pruneTable names a table this service may batch-delete from.
//
// It is a distinct type rather than a string so the identifier spliced into
// the DELETE below cannot come from anywhere but the two constants here — the
// only values of the type that exist.
type pruneTable string

const (
	pruneTableUsageEvents pruneTable = "api_key_usage_events"
	pruneTableTombstones  pruneTable = "api_key_tombstones"
)

func (s *Service) deleteAllBatched(ctx context.Context, table pruneTable) (int64, error) {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id IN (SELECT id FROM %s LIMIT ?)`, table, table) // SQL identifier: pruneTable constant, never request input

	var total int64
	for {
		result, err := s.db.ExecContext(ctx, query, usagePruneBatchSize)
		if err != nil {
			return total, fmt.Errorf("clearing %s: %w", table, err)
		}
		deleted := db.RowsAffected(result)
		total += deleted
		if deleted < usagePruneBatchSize {
			return total, nil
		}
	}
}

const (
	usageEventsSelect = `
		SELECT id, api_key_id, occurred_at, method, route_pattern, status_code,
		       duration_ms, auth_outcome, environment_id, peer_ip, client_ip,
		       client_ip_source, user_agent
		  FROM api_key_usage_events`
	usageEventsTail = `
		 ORDER BY occurred_at DESC
		 LIMIT ? OFFSET ?`
)
