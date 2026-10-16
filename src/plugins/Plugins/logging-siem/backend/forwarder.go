// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// maxForwardRows caps one cycle. Twenty-five matches the batch the
// host-owned forwarder used: small enough that a wedged collector
// cannot hold the cycle open past the host's deadline, large enough
// that a busy install drains its backlog in a few ticks.
const maxForwardRows = 25

// siemClient implements plugin.LogForwarderAdapterClient.
type siemClient struct {
	host plugin.HostContext
	http *http.Client
}

func newSIEMClient() *siemClient {
	return &siemClient{
		// Per-request deadlines come from the config's timeout_seconds
		// via context, so the client itself carries no global timeout;
		// one shared client keeps connection reuse across cycles.
		http: &http.Client{},
	}
}

// attach receives the host context during Init.
func (c *siemClient) attach(host plugin.HostContext) { c.host = host }

// resolved holds the parsed config for one cycle.
type resolved struct {
	// forwarderID is the host's audit_log_forwarders row id. Several
	// SIEM destinations may be configured at once, and every read and
	// write of the delivery ledger is scoped by this — otherwise the
	// first destination to deliver an event would mark it delivered
	// for all of them.
	forwarderID    string
	endpointURL    string
	signingSecret  string
	maxRetries     int
	timeoutSeconds int
	enabledSince   string
}

// parseConfig reads the map the host hands over. Secrets arrive
// already decrypted.
func parseConfig(config map[string]string) (resolved, error) {
	r := resolved{
		forwarderID:    strings.TrimSpace(config["__forwarder_id"]),
		endpointURL:    strings.TrimSpace(config["endpoint_url"]),
		signingSecret:  config["signing_secret"],
		maxRetries:     atoiDefault(config["max_retries"], 3),
		timeoutSeconds: atoiDefault(config["timeout_seconds"], 10),
		enabledSince:   config["__enabled_since"],
	}
	if r.endpointURL == "" {
		return r, errors.New("endpoint URL is not configured")
	}
	if !validEndpoint(r.endpointURL) {
		return r, errors.New("endpoint URL must be an absolute http or https URL")
	}
	if r.signingSecret == "" {
		return r, errors.New("signing secret is not configured")
	}
	if r.forwarderID == "" {
		// The host always injects this. An empty value means an older
		// host is driving a newer plugin; refusing is safer than
		// writing unattributable ledger rows that would later dedup
		// against every instance at once.
		return r, errors.New("forwarder id is missing from the host configuration")
	}
	return r, nil
}

// validEndpoint mirrors the host-side validateSIEMURL this plugin
// replaces: absolute, http or https, with a host component. Anything
// else is a config mistake worth catching before it becomes a
// confusing transport error.
func validEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

func atoiDefault(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}


// ensureSchema adds the forwarder_id column to an install that predates
// multi-instance forwarding.
//
// Plugin migrations run at install time only: a bundled plugin whose
// files are refreshed in place on upgrade never revisits its
// migrations/ directory, so 0002_forwarder_id.sql reaches fresh
// installs and nothing else. Doing it here as well makes the plugin
// self-healing on the upgrade path, and both routes converge on the
// same shape — a duplicate-column error from the second one to run is
// the expected outcome, not a failure.
func (c *siemClient) ensureSchema(ctx context.Context) error {
	_, err := c.host.DB.Exec(ctx, `
		ALTER TABLE plugin_logging_siem_delivery_history
		ADD COLUMN forwarder_id TEXT NOT NULL DEFAULT ''
	`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil
		}
		return fmt.Errorf("adding forwarder_id to siem delivery history: %w", err)
	}

	// The column was just created, so every existing row predates
	// multi-instance forwarding and belongs to the one forwarder that
	// could have written it. Attaching them keeps the ledger intact:
	// an unattributed history would read as empty to that forwarder
	// and re-ship its whole backlog to a sink that already has it.
	if _, err := c.host.DB.Exec(ctx, `
		UPDATE plugin_logging_siem_delivery_history
		SET forwarder_id = COALESCE(
		    (SELECT id FROM audit_log_forwarders
		      WHERE forwarder_type = 'siem' ORDER BY created_at ASC LIMIT 1), '')
		WHERE forwarder_id = ''
	`); err != nil {
		return fmt.Errorf("backfilling siem delivery history forwarder ids: %w", err)
	}

	if _, err := c.host.DB.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_plugin_logging_siem_delivery_forwarder
		ON plugin_logging_siem_delivery_history(forwarder_id, audit_log_id, status)
	`); err != nil {
		return fmt.Errorf("indexing siem delivery history by forwarder: %w", err)
	}
	return nil
}

// Test posts a synthetic event so the operator learns whether their
// endpoint, credential, and network path actually work.
func (c *siemClient) Test(ctx context.Context, config map[string]string) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"type": "mcharbor.audit.test",
		"entry": map[string]any{
			"action":    "audit.siem.test",
			"severity":  "info",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	_, checksum, _, statusCode, deliverErr := c.deliver(ctx, cfg, payload)

	// A Test is recorded like any other attempt. An operator who
	// clicks Test and then looks at the history panel should see what
	// just happened; a Test that leaves no trace is the reason
	// "did my test actually go out?" is a support question.
	status := "success"
	errText := ""
	if deliverErr != nil {
		status = "failed"
		errText = safeError(deliverErr)
	}
	if recErr := c.record(ctx, cfg.forwarderID, "", status, 1, statusCode, errText, checksum); recErr != nil && c.host.Logger != nil {
		c.host.Logger.Warn("recording siem test delivery failed", "error", recErr)
	}
	return deliverErr
}

// ForwardPending runs one cycle: find undelivered audit events,
// deliver them, record each attempt.
//
// The pending set is a join between the host's audit_logs and this
// plugin's own delivery history — an event is pending when it has no
// successful delivery and fewer than max_retries attempts. Neither
// side can compute that alone, which is why this plugin reads
// audit_logs (declared as consumes_permissions: audit.read in the
// manifest) rather than the host pushing a batch it cannot filter.
//
// Events are never forwarded from before enabled_since, so switching
// forwarding on for a year-old install does not replay a year of
// history into a fresh collector.
func (c *siemClient) ForwardPending(ctx context.Context, config map[string]string) (plugin.LogForwardResult, error) {
	var result plugin.LogForwardResult
	cfg, err := parseConfig(config)
	if err != nil {
		return result, err
	}
	if cfg.enabledSince == "" {
		// No watermark means forwarding was never switched on. Nothing
		// to do — not an error.
		return result, nil
	}

	rows, err := c.host.DB.Query(ctx, `
		SELECT l.id, l.user_id, l.username, l.action, l.severity,
		       l.entity_type, l.entity_id, l.entity_name, l.details,
		       l.ip_address, l.environment_id, l.timestamp
		FROM audit_logs l
		WHERE l.timestamp >= ?
		  AND l.action NOT LIKE 'audit.forwarder.%'
		  AND NOT EXISTS (
		      SELECT 1 FROM plugin_logging_siem_delivery_history h
		      WHERE h.audit_log_id = l.id AND h.forwarder_id = ? AND h.status = 'success'
		  )
		  AND (
		      SELECT COUNT(1) FROM plugin_logging_siem_delivery_history h
		      WHERE h.audit_log_id = l.id AND h.forwarder_id = ?
		  ) < ?
		ORDER BY l.timestamp ASC
		LIMIT ?
	`, cfg.enabledSince, cfg.forwarderID, cfg.forwarderID, cfg.maxRetries, maxForwardRows)
	if err != nil {
		return result, fmt.Errorf("querying pending audit events: %w", err)
	}
	defer rows.Close()

	type pending struct {
		id    string
		entry map[string]any
	}
	var batch []pending
	for rows.Next() {
		var id, action, severity, timestamp string
		var userID, username, entityType, entityID, entityName, details, ip, envID *string
		if err := rows.Scan(&id, &userID, &username, &action, &severity,
			&entityType, &entityID, &entityName, &details, &ip, &envID, &timestamp); err != nil {
			return result, fmt.Errorf("scanning pending audit event: %w", err)
		}
		batch = append(batch, pending{id: id, entry: map[string]any{
			"id":            id,
			"userId":        deref(userID),
			"username":      deref(username),
			"action":        action,
			"severity":      severity,
			"entityType":    deref(entityType),
			"entityId":      deref(entityID),
			"entityName":    deref(entityName),
			"details":       deref(details),
			"ipAddress":     deref(ip),
			"environmentId": deref(envID),
			"timestamp":     timestamp,
		}})
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("reading pending audit events: %w", err)
	}

	// Rows are read fully before any delivery so the query is not held
	// open across N network round-trips.
	for _, item := range batch {
		attempt, err := c.nextAttempt(ctx, cfg.forwarderID, item.id)
		if err != nil {
			return result, err
		}
		if attempt > cfg.maxRetries {
			result.Skipped++
			continue
		}
		_, checksum, _, statusCode, deliverErr := c.deliver(ctx, cfg,
			map[string]any{"type": "mcharbor.audit.event", "entry": item.entry})
		status := "success"
		errText := ""
		if deliverErr != nil {
			status = "failed"
			errText = safeError(deliverErr)
			result.Failed++
		} else {
			result.Delivered++
		}
		if err := c.record(ctx, cfg.forwarderID, item.id, status, attempt, statusCode, errText, checksum); err != nil {
			return result, err
		}
	}
	return result, nil
}

// deliver POSTs one payload. Returns body, checksum, signature,
// status code, error — the same tuple shape the host-owned forwarder
// used, so the recorded columns stay identical.
func (c *siemClient) deliver(ctx context.Context, cfg resolved, payload any) ([]byte, string, string, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", "", 0, fmt.Errorf("encoding audit siem payload: %w", err)
	}
	checksum := sha256Hex(body)
	signature := hmacSHA256Hex([]byte(cfg.signingSecret), body)

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.endpointURL, bytes.NewReader(body))
	if err != nil {
		return body, checksum, signature, 0, fmt.Errorf("creating audit siem request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-McHarbor-Checksum-SHA256", checksum)
	req.Header.Set("X-McHarbor-Signature", "sha256="+signature)
	req.Header.Set("X-McHarbor-Signature-Version", "v1")

	resp, err := c.http.Do(req)
	if err != nil {
		return body, checksum, signature, 0, fmt.Errorf("delivering audit siem event: %w", err)
	}
	defer resp.Body.Close()
	if _, copyErr := io.Copy(io.Discard, resp.Body); copyErr != nil {
		return body, checksum, signature, resp.StatusCode, fmt.Errorf("reading audit siem response: %w", copyErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, checksum, signature, resp.StatusCode, fmt.Errorf("audit siem endpoint returned status %d", resp.StatusCode)
	}
	return body, checksum, signature, resp.StatusCode, nil
}

// DeliveryHistory returns recent attempts, newest first.
func (c *siemClient) DeliveryHistory(ctx context.Context, config map[string]string, limit int) ([]plugin.LogDelivery, error) {
	if limit <= 0 {
		limit = 25
	}
	// History is per destination. Showing every SIEM instance's
	// attempts under whichever card was expanded would make two
	// collectors look like one very busy one.
	forwarderID := strings.TrimSpace(config["__forwarder_id"])
	rows, err := c.host.DB.Query(ctx, `
		SELECT id, audit_log_id, status, attempt, status_code, error, created_at
		FROM plugin_logging_siem_delivery_history
		WHERE forwarder_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, forwarderID, limit)
	if err != nil {
		return nil, fmt.Errorf("reading siem delivery history: %w", err)
	}
	defer rows.Close()

	out := make([]plugin.LogDelivery, 0, limit)
	for rows.Next() {
		var d plugin.LogDelivery
		var auditLogID *string
		var createdAt string
		if err := rows.Scan(&d.ID, &auditLogID, &d.Status, &d.Attempt, &d.StatusCode, &d.Error, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning siem delivery history: %w", err)
		}
		d.EventID = deref(auditLogID)
		d.Protocol = "https"
		if ts, err := time.Parse(time.RFC3339, createdAt); err == nil {
			d.CreatedAt = ts
		} else if ts, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			d.CreatedAt = ts
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// record writes one delivery attempt.
func (c *siemClient) record(ctx context.Context, forwarderID, auditLogID, status string, attempt, statusCode int, errText, checksum string) error {
	var logID any
	if auditLogID != "" {
		logID = auditLogID
	}
	_, err := c.host.DB.Exec(ctx, `
		INSERT INTO plugin_logging_siem_delivery_history
		    (id, forwarder_id, audit_log_id, status, attempt, status_code, error, checksum_sha256)
		VALUES (lower(hex(randomblob(12))), ?, ?, ?, ?, ?, ?, ?)
	`, forwarderID, logID, status, attempt, statusCode, errText, checksum)
	if err != nil {
		return fmt.Errorf("recording siem delivery: %w", err)
	}
	return nil
}

// nextAttempt returns the 1-based attempt number for an event.
func (c *siemClient) nextAttempt(ctx context.Context, forwarderID, auditLogID string) (int, error) {
	rows, err := c.host.DB.Query(ctx, `
		SELECT COUNT(1) FROM plugin_logging_siem_delivery_history
		WHERE forwarder_id = ? AND audit_log_id = ?
	`, forwarderID, auditLogID)
	if err != nil {
		return 0, fmt.Errorf("counting siem delivery attempts: %w", err)
	}
	defer rows.Close()
	count := 0
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, fmt.Errorf("scanning siem delivery attempt count: %w", err)
		}
	}
	return count + 1, rows.Err()
}

// safeError strips anything an operator must not see from a transport
// error before it is stored in delivery history and rendered in the
// Logging tab.
//
// A url.Error carries the full request URL, which for many collectors
// embeds the token in a query parameter or path segment — writing that
// verbatim into a table the Logging tab renders would leak the
// credential into the UI and into any audit export. The endpoint is
// already visible in the config form, so nothing is lost by removing
// it here.
func safeError(err error) string {
	if err == nil {
		return ""
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Hex(key, data []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

var _ plugin.LogForwarderAdapterClient = (*siemClient)(nil)
