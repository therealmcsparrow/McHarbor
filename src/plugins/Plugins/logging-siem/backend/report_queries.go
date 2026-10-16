// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"time"
)

// storedTimeLayout is how CURRENT_TIMESTAMP writes created_at. Period
// bounds are passed in the same layout so the comparison stays a plain
// string comparison the created_at index can serve.
const storedTimeLayout = "2006-01-02 15:04:05"

// reportRowLimit bounds every report read. Each query is grouped per
// forwarder (or per forwarder and reason), so the cap is far above any
// real installation and only guards against a pathological history.
const reportRowLimit = 1000

func storedTime(t time.Time) string {
	return t.UTC().Format(storedTimeLayout)
}

// parseStoredTime accepts both layouts that reach these tables:
// CURRENT_TIMESTAMP's for delivery rows and RFC3339 for the host's
// enabled_since watermark. A zero time means "unknown", never an error.
func parseStoredTime(raw *string) time.Time {
	if raw == nil {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, storedTimeLayout} {
		if ts, err := time.Parse(layout, *raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

// reportForwarders lists the configured SIEM forwarders with each one's
// last successful event delivery before end. The subquery on this
// plugin's own table is also what satisfies the host's table-scope guard.
func (c *siemClient) reportForwarders(ctx context.Context, end time.Time, forwarderID string) ([]forwarderRow, error) {
	rows, err := c.host.DB.Query(ctx, `
		SELECT f.id, f.name, f.enabled, f.enabled_since,
		       (SELECT MAX(h.created_at) FROM plugin_logging_siem_delivery_history h
		         WHERE h.forwarder_id = f.id AND h.status = 'success'
		           AND h.audit_log_id IS NOT NULL AND h.created_at < ?)
		FROM audit_log_forwarders f
		WHERE f.forwarder_type = ? AND (? = '' OR f.id = ?)
		ORDER BY f.name
		LIMIT ?
	`, storedTime(end), siemForwarderType, forwarderID, forwarderID, reportRowLimit)
	if err != nil {
		return nil, fmt.Errorf("reading siem forwarders: %w", err)
	}
	defer rows.Close()

	var out []forwarderRow
	for rows.Next() {
		var r forwarderRow
		var enabled int
		var enabledSince, lastSuccess *string
		if err := rows.Scan(&r.ID, &r.Name, &enabled, &enabledSince, &lastSuccess); err != nil {
			return nil, fmt.Errorf("scanning siem forwarder: %w", err)
		}
		r.Enabled = enabled == 1
		r.EnabledSince = parseStoredTime(enabledSince)
		r.LastSuccess = parseStoredTime(lastSuccess)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading siem forwarders: %w", err)
	}
	return out, nil
}

// reportTotals counts each forwarder's attempts in [start, end).
// Undelivered counts events with a failure in the period and no success
// at any time, so an event delivered on a later retry is not counted.
func (c *siemClient) reportTotals(ctx context.Context, start, end time.Time, forwarderID string) ([]totalsRow, error) {
	rows, err := c.host.DB.Query(ctx, `
		SELECT h.forwarder_id,
		       SUM(CASE WHEN h.audit_log_id IS NOT NULL AND h.status = 'success' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN h.audit_log_id IS NOT NULL AND h.status = 'failed' THEN 1 ELSE 0 END),
		       SUM(CASE WHEN h.audit_log_id IS NULL THEN 1 ELSE 0 END),
		       COUNT(DISTINCT CASE
		           WHEN h.audit_log_id IS NOT NULL AND h.status = 'failed' AND NOT EXISTS (
		               SELECT 1 FROM plugin_logging_siem_delivery_history s
		               WHERE s.forwarder_id = h.forwarder_id AND s.audit_log_id = h.audit_log_id
		                 AND s.status = 'success')
		           THEN h.audit_log_id END),
		       MIN(CASE WHEN h.audit_log_id IS NOT NULL AND h.status = 'success' THEN h.created_at END),
		       MAX(CASE WHEN h.audit_log_id IS NOT NULL AND h.status = 'success' THEN h.created_at END),
		       MAX(CASE WHEN h.audit_log_id IS NOT NULL AND h.status = 'failed' THEN h.created_at END)
		FROM plugin_logging_siem_delivery_history h
		WHERE h.created_at >= ? AND h.created_at < ? AND (? = '' OR h.forwarder_id = ?)
		GROUP BY h.forwarder_id
		LIMIT ?
	`, storedTime(start), storedTime(end), forwarderID, forwarderID, reportRowLimit)
	if err != nil {
		return nil, fmt.Errorf("reading siem delivery totals: %w", err)
	}
	defer rows.Close()

	var out []totalsRow
	for rows.Next() {
		var r totalsRow
		var firstSuccess, lastSuccess, lastFailure *string
		if err := rows.Scan(&r.ForwarderID, &r.Delivered, &r.Failed, &r.Tests, &r.Undelivered,
			&firstSuccess, &lastSuccess, &lastFailure); err != nil {
			return nil, fmt.Errorf("scanning siem delivery totals: %w", err)
		}
		r.FirstSuccess = parseStoredTime(firstSuccess)
		r.LastSuccess = parseStoredTime(lastSuccess)
		r.LastFailure = parseStoredTime(lastFailure)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading siem delivery totals: %w", err)
	}
	return out, nil
}

// reportGaps returns, per forwarder, the longest interval between two
// consecutive successful event deliveries in [start, end). SQLite fills
// the bare columns of a MAX() aggregate from the row that won, which is
// what keeps this one row per forwarder instead of one per delivery.
func (c *siemClient) reportGaps(ctx context.Context, start, end time.Time, forwarderID string) ([]gapRow, error) {
	rows, err := c.host.DB.Query(ctx, `
		SELECT forwarder_id, prev_at, created_at,
		       MAX(julianday(created_at) - julianday(prev_at))
		FROM (
		    SELECT forwarder_id, created_at,
		           LAG(created_at) OVER (PARTITION BY forwarder_id ORDER BY created_at) AS prev_at
		    FROM plugin_logging_siem_delivery_history
		    WHERE status = 'success' AND audit_log_id IS NOT NULL
		      AND created_at >= ? AND created_at < ? AND (? = '' OR forwarder_id = ?)
		) g
		WHERE prev_at IS NOT NULL
		GROUP BY forwarder_id
		LIMIT ?
	`, storedTime(start), storedTime(end), forwarderID, forwarderID, reportRowLimit)
	if err != nil {
		return nil, fmt.Errorf("reading siem delivery gaps: %w", err)
	}
	defer rows.Close()

	var out []gapRow
	for rows.Next() {
		var r gapRow
		var from, to *string
		var days *float64
		if err := rows.Scan(&r.ForwarderID, &from, &to, &days); err != nil {
			return nil, fmt.Errorf("scanning siem delivery gap: %w", err)
		}
		r.From = parseStoredTime(from)
		r.To = parseStoredTime(to)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading siem delivery gaps: %w", err)
	}
	return out, nil
}

// reportErrors returns each distinct stored failure text per forwarder
// in [start, end), most frequent first. Grouping into reasons happens in
// Go, where the normalisation can be tested.
func (c *siemClient) reportErrors(ctx context.Context, start, end time.Time, forwarderID string) ([]errorRow, error) {
	rows, err := c.host.DB.Query(ctx, `
		SELECT forwarder_id, error, COUNT(1), MAX(created_at)
		FROM plugin_logging_siem_delivery_history
		WHERE status = 'failed' AND audit_log_id IS NOT NULL
		  AND created_at >= ? AND created_at < ? AND (? = '' OR forwarder_id = ?)
		GROUP BY forwarder_id, error
		ORDER BY COUNT(1) DESC
		LIMIT ?
	`, storedTime(start), storedTime(end), forwarderID, forwarderID, reportRowLimit)
	if err != nil {
		return nil, fmt.Errorf("reading siem delivery errors: %w", err)
	}
	defer rows.Close()

	var out []errorRow
	for rows.Next() {
		var r errorRow
		var lastSeen *string
		if err := rows.Scan(&r.ForwarderID, &r.Error, &r.Count, &lastSeen); err != nil {
			return nil, fmt.Errorf("scanning siem delivery error: %w", err)
		}
		r.LastSeen = parseStoredTime(lastSeen)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading siem delivery errors: %w", err)
	}
	return out, nil
}
