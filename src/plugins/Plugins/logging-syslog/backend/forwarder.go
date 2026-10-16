// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// maxForwardRows caps one cycle, matching the host-owned forwarder
// this plugin replaces.
const maxForwardRows = 25

// syslogVersion is the RFC5424 VERSION field. The RFC defines exactly
// one version and it is 1.
const syslogVersion = 1

// nilValue is RFC5424's explicit "no value" token. Used for
// STRUCTURED-DATA and for any header field we have nothing for; a
// literal "-" is required, an empty field is a protocol violation.
const nilValue = "-"

// syslogClient implements plugin.LogForwarderAdapterClient.
type syslogClient struct {
	host plugin.HostContext
}

func newSyslogClient() *syslogClient { return &syslogClient{} }

// attach receives the host context during Init.
func (c *syslogClient) attach(host plugin.HostContext) { c.host = host }

// resolved holds the parsed config for one cycle.
type resolved struct {
	// forwarderID is the host's audit_log_forwarders row id. Several
	// syslog destinations may be configured at once, and every read
	// and write of the delivery ledger is scoped by this — otherwise
	// the first destination to deliver an event would mark it
	// delivered for all of them.
	forwarderID    string
	host           string
	port           int
	protocol       string
	facility       string
	tag            string
	maxRetries     int
	timeoutSeconds int
	enabledSince   string
}

// address is the dial target.
func (r resolved) address() string {
	return net.JoinHostPort(r.host, strconv.Itoa(r.port))
}

// validFacilities is the set the old settings form offered. Kept
// identical so an imported config never lands on a value the plugin
// then rejects.
var validFacilities = map[string]int{
	"user": 1, "daemon": 3, "auth": 4, "syslog": 5, "authpriv": 10,
	"local0": 16, "local1": 17, "local2": 18, "local3": 19,
	"local4": 20, "local5": 21, "local6": 22, "local7": 23,
}

// parseConfig reads and normalises the map the host hands over.
func parseConfig(config map[string]string) (resolved, error) {
	r := resolved{
		forwarderID:    strings.TrimSpace(config["__forwarder_id"]),
		host:           strings.TrimSpace(config["host"]),
		port:           atoiDefault(config["port"], 6514),
		protocol:       strings.ToLower(strings.TrimSpace(config["protocol"])),
		facility:       strings.ToLower(strings.TrimSpace(config["facility"])),
		tag:            strings.TrimSpace(config["tag"]),
		maxRetries:     atoiDefault(config["max_retries"], 3),
		timeoutSeconds: atoiDefault(config["timeout_seconds"], 5),
		enabledSince:   config["__enabled_since"],
	}
	if r.protocol == "" {
		r.protocol = "tls"
	}
	if r.facility == "" {
		r.facility = "local0"
	}
	if r.tag == "" {
		r.tag = "mcharbor"
	}
	if r.host == "" {
		return r, errors.New("syslog host is not configured")
	}
	if r.port < 1 || r.port > 65535 {
		return r, errors.New("syslog port must be between 1 and 65535")
	}
	switch r.protocol {
	case "udp", "tcp", "tls":
	default:
		return r, errors.New("syslog protocol must be one of udp, tcp, tls")
	}
	if _, ok := validFacilities[r.facility]; !ok {
		return r, errors.New("unknown syslog facility")
	}
	if !validTag(r.tag) {
		return r, errors.New("syslog tag must be 1-48 printable ASCII characters without spaces")
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

// validTag enforces RFC5424's APP-NAME rule: 1-48 characters of
// printable US-ASCII, no spaces. A tag with a space in it silently
// shifts every following header field by one, so this is checked
// rather than trusted.
func validTag(tag string) bool {
	if len(tag) == 0 || len(tag) > 48 {
		return false
	}
	for _, r := range tag {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}

func atoiDefault(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// severityCode maps a McHarbor audit severity onto an RFC5424
// severity. The mapping is deliberately conservative: audit events
// are records, not incidents, so nothing maps below 3 (Error) — an
// audit forwarder should never be able to page someone by emitting
// severity 0 (Emergency).
func severityCode(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 2 // Critical
	case "error":
		return 3 // Error
	case "warning":
		return 4 // Warning
	case "notice":
		return 5 // Notice
	default:
		return 6 // Informational
	}
}

// priority is RFC5424's PRI: facility * 8 + severity.
func priority(facility string, severity string) int {
	return validFacilities[facility]*8 + severityCode(severity)
}

// encodeMessage builds one RFC5424 message and returns it with the
// SHA-256 of the payload, so delivery history records what was sent.
//
// Format:
//
//	<PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
//
// MSG is the audit entry as compact JSON, prefixed with the RFC5424
// BOM that marks the message as UTF-8. Collectors that parse the
// payload get structured data; collectors that only index text still
// get every field as searchable content.
func encodeMessage(cfg resolved, severity string, timestamp time.Time, msgID string, payload any) (string, string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("encoding syslog payload: %w", err)
	}
	checksum := sha256Hex(body)

	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = nilValue
	}
	if msgID == "" {
		msgID = nilValue
	}

	// RFC5424 TIMESTAMP is RFC3339 with at most 6 fractional digits.
	ts := timestamp.UTC().Format("2006-01-02T15:04:05.000000Z07:00")

	// The BOM is written as an escape, not a literal: a literal
	// U+FEFF in source is an illegal byte order mark to the Go parser.
	msg := fmt.Sprintf("<%d>%d %s %s %s %s %s %s \ufeff%s",
		priority(cfg.facility, severity),
		syslogVersion,
		ts,
		hostname,
		cfg.tag,
		strconv.Itoa(os.Getpid()),
		msgID,
		nilValue,
		string(body),
	)
	return msg, checksum, nil
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
func (c *syslogClient) ensureSchema(ctx context.Context) error {
	_, err := c.host.DB.Exec(ctx, `
		ALTER TABLE plugin_logging_syslog_delivery_history
		ADD COLUMN forwarder_id TEXT NOT NULL DEFAULT ''
	`)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return nil
		}
		return fmt.Errorf("adding forwarder_id to syslog delivery history: %w", err)
	}

	// The column was just created, so every existing row predates
	// multi-instance forwarding and belongs to the one forwarder that
	// could have written it. Attaching them keeps the ledger intact:
	// an unattributed history would read as empty to that forwarder
	// and re-ship its whole backlog to a sink that already has it.
	if _, err := c.host.DB.Exec(ctx, `
		UPDATE plugin_logging_syslog_delivery_history
		SET forwarder_id = COALESCE(
		    (SELECT id FROM audit_log_forwarders
		      WHERE forwarder_type = 'syslog' ORDER BY created_at ASC LIMIT 1), '')
		WHERE forwarder_id = ''
	`); err != nil {
		return fmt.Errorf("backfilling syslog delivery history forwarder ids: %w", err)
	}

	if _, err := c.host.DB.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_plugin_logging_syslog_delivery_forwarder
		ON plugin_logging_syslog_delivery_history(forwarder_id, audit_log_id, status)
	`); err != nil {
		return fmt.Errorf("indexing syslog delivery history by forwarder: %w", err)
	}
	return nil
}

// Test emits a synthetic event so the operator learns whether their
// collector, port, and TLS chain actually work.
func (c *syslogClient) Test(ctx context.Context, config map[string]string) error {
	cfg, err := parseConfig(config)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	message, checksum, err := encodeMessage(cfg, "info", now, "audit.syslog.test", map[string]any{
		"type": "mcharbor.audit.test",
		"entry": map[string]any{
			"action":    "audit.syslog.test",
			"severity":  "info",
			"timestamp": now.Format(time.RFC3339),
		},
	})
	if err != nil {
		return err
	}
	deliverErr := c.deliver(ctx, cfg, message)

	status := "success"
	errText := ""
	if deliverErr != nil {
		status = "failed"
		errText = safeError(deliverErr)
	}
	if recErr := c.record(ctx, cfg.forwarderID, "", status, 1, cfg.protocol, errText, checksum); recErr != nil && c.host.Logger != nil {
		c.host.Logger.Warn("recording syslog test delivery failed", "error", recErr)
	}
	return deliverErr
}

// ForwardPending runs one cycle: find undelivered audit events, emit
// them, record each attempt. See the logging-siem twin and the
// package doc in core/plugin/specs/logging for why the join lives
// here rather than in the host.
func (c *syslogClient) ForwardPending(ctx context.Context, config map[string]string) (plugin.LogForwardResult, error) {
	var result plugin.LogForwardResult
	cfg, err := parseConfig(config)
	if err != nil {
		return result, err
	}
	if cfg.enabledSince == "" {
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
		      SELECT 1 FROM plugin_logging_syslog_delivery_history h
		      WHERE h.audit_log_id = l.id AND h.forwarder_id = ? AND h.status = 'success'
		  )
		  AND (
		      SELECT COUNT(1) FROM plugin_logging_syslog_delivery_history h
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
		id        string
		action    string
		severity  string
		timestamp string
		entry     map[string]any
	}
	var batch []pending
	for rows.Next() {
		var id, action, severity, timestamp string
		var userID, username, entityType, entityID, entityName, details, ip, envID *string
		if err := rows.Scan(&id, &userID, &username, &action, &severity,
			&entityType, &entityID, &entityName, &details, &ip, &envID, &timestamp); err != nil {
			return result, fmt.Errorf("scanning pending audit event: %w", err)
		}
		batch = append(batch, pending{
			id: id, action: action, severity: severity, timestamp: timestamp,
			entry: map[string]any{
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
			},
		})
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("reading pending audit events: %w", err)
	}

	for _, item := range batch {
		attempt, err := c.nextAttempt(ctx, cfg.forwarderID, item.id)
		if err != nil {
			return result, err
		}
		if attempt > cfg.maxRetries {
			result.Skipped++
			continue
		}
		// The event's own timestamp is used, not time.Now(): a
		// collector correlating McHarbor events against other sources
		// needs when the action happened, not when the backlog
		// drained.
		eventTime := parseTimestamp(item.timestamp)
		message, checksum, err := encodeMessage(cfg, item.severity, eventTime, item.action,
			map[string]any{"type": "mcharbor.audit.event", "entry": item.entry})
		if err != nil {
			return result, err
		}
		deliverErr := c.deliver(ctx, cfg, message)
		status := "success"
		errText := ""
		if deliverErr != nil {
			status = "failed"
			errText = safeError(deliverErr)
			result.Failed++
		} else {
			result.Delivered++
		}
		if err := c.record(ctx, cfg.forwarderID, item.id, status, attempt, cfg.protocol, errText, checksum); err != nil {
			return result, err
		}
	}
	return result, nil
}

// deliver opens a connection, writes one message, and closes it.
//
// A connection per message rather than a pooled one: syslog
// collectors routinely drop idle connections, and a stale pooled
// socket fails the *next* event rather than the one that broke it,
// which makes the delivery history lie about which event was lost.
// One event per connection costs a handshake and buys an honest
// per-event outcome.
func (c *syslogClient) deliver(ctx context.Context, cfg resolved, message string) error {
	timeout := time.Duration(cfg.timeoutSeconds) * time.Second
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var conn net.Conn
	var err error
	switch cfg.protocol {
	case "udp":
		var d net.Dialer
		conn, err = d.DialContext(dialCtx, "udp", cfg.address())
	case "tcp":
		var d net.Dialer
		conn, err = d.DialContext(dialCtx, "tcp", cfg.address())
	case "tls":
		d := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: timeout},
			// ServerName is set explicitly so certificate
			// verification checks the configured hostname. The zero
			// Config would still verify, but leaving it implicit
			// invites a later "just add InsecureSkipVerify" patch;
			// naming the field makes the intent visible.
			Config: &tls.Config{ServerName: cfg.host, MinVersion: tls.VersionTLS12},
		}
		conn, err = d.DialContext(dialCtx, "tcp", cfg.address())
	default:
		return errors.New("unsupported syslog protocol")
	}
	if err != nil {
		return fmt.Errorf("connecting to syslog collector: %w", err)
	}
	defer conn.Close()

	if deadline, ok := dialCtx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("setting syslog write deadline: %w", err)
		}
	}

	payload := message
	if cfg.protocol != "udp" {
		// RFC6587 §3.4.1 octet counting. Stream transports have no
		// message boundary of their own, so a collector reading TCP
		// needs the length prefix to know where one event ends —
		// without it a busy stream is parsed as one giant malformed
		// message.
		payload = strconv.Itoa(len(message)) + " " + message
	}
	if _, err := conn.Write([]byte(payload)); err != nil {
		return fmt.Errorf("writing syslog message: %w", err)
	}
	return nil
}

// DeliveryHistory returns recent attempts, newest first.
func (c *syslogClient) DeliveryHistory(ctx context.Context, config map[string]string, limit int) ([]plugin.LogDelivery, error) {
	if limit <= 0 {
		limit = 25
	}
	// History is per destination. Showing every syslog instance's
	// attempts under whichever card was expanded would make two
	// daemons look like one very busy one.
	forwarderID := strings.TrimSpace(config["__forwarder_id"])
	rows, err := c.host.DB.Query(ctx, `
		SELECT id, audit_log_id, status, attempt, protocol, error, created_at
		FROM plugin_logging_syslog_delivery_history
		WHERE forwarder_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, forwarderID, limit)
	if err != nil {
		return nil, fmt.Errorf("reading syslog delivery history: %w", err)
	}
	defer rows.Close()

	out := make([]plugin.LogDelivery, 0, limit)
	for rows.Next() {
		var d plugin.LogDelivery
		var auditLogID *string
		var createdAt string
		if err := rows.Scan(&d.ID, &auditLogID, &d.Status, &d.Attempt, &d.Protocol, &d.Error, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning syslog delivery history: %w", err)
		}
		d.EventID = deref(auditLogID)
		// Syslog has no protocol-level response code; leaving
		// StatusCode zero is what the spec documents for exactly this
		// case, and the UI renders a dash rather than a misleading 0.
		d.CreatedAt = parseTimestamp(createdAt)
		out = append(out, d)
	}
	return out, rows.Err()
}

// record writes one delivery attempt.
func (c *syslogClient) record(ctx context.Context, forwarderID, auditLogID, status string, attempt int, protocol, errText, checksum string) error {
	var logID any
	if auditLogID != "" {
		logID = auditLogID
	}
	_, err := c.host.DB.Exec(ctx, `
		INSERT INTO plugin_logging_syslog_delivery_history
		    (id, forwarder_id, audit_log_id, status, attempt, protocol, error, checksum_sha256)
		VALUES (lower(hex(randomblob(12))), ?, ?, ?, ?, ?, ?, ?)
	`, forwarderID, logID, status, attempt, protocol, errText, checksum)
	if err != nil {
		return fmt.Errorf("recording syslog delivery: %w", err)
	}
	return nil
}

// nextAttempt returns the 1-based attempt number for an event.
func (c *syslogClient) nextAttempt(ctx context.Context, forwarderID, auditLogID string) (int, error) {
	rows, err := c.host.DB.Query(ctx, `
		SELECT COUNT(1) FROM plugin_logging_syslog_delivery_history
		WHERE forwarder_id = ? AND audit_log_id = ?
	`, forwarderID, auditLogID)
	if err != nil {
		return 0, fmt.Errorf("counting syslog delivery attempts: %w", err)
	}
	defer rows.Close()
	count := 0
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, fmt.Errorf("scanning syslog delivery attempt count: %w", err)
		}
	}
	return count + 1, rows.Err()
}

// safeError reduces a transport error to something an operator can
// read without leaking internals into the Logging tab and audit
// exports. net.OpError carries the local socket address, which is
// noise at best and internal topology at worst.
func safeError(err error) string {
	if err == nil {
		return ""
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		return opErr.Err.Error()
	}
	return err.Error()
}

// parseTimestamp accepts both formats SQLite hands back: RFC3339 from
// application writes, and the space-separated form CURRENT_TIMESTAMP
// produces. A zero time on failure renders as "unknown" rather than
// failing the whole history read for one bad row.
func parseTimestamp(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts
		}
	}
	return time.Time{}
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

var _ plugin.LogForwarderAdapterClient = (*syslogClient)(nil)
