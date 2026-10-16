// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package audit serves the audit-log API: listing, export, verification, retention and log forwarding.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/therealmcsparrow/mcharbor/core/redact"
)

const (
	aiActionEntityType = "ai_assistant_action"
	maxTrailPayload    = 4000
	maxTrailAuditRows  = 5000
)

// aiToolActions are the tool-plane events the trail lists beside the consent
// flow; they are keyed by conversation, not by action.
var aiToolActions = []string{"ai.tool.call", "ai.tool.denied", "ai.tool.sanitized"}

// AITrailStep is one point in an action's life: the preview that created it, a
// consent decision, or an execution outcome.
type AITrailStep struct {
	Kind      string `json:"kind"`
	AuditID   string `json:"auditId,omitempty"`
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor,omitempty"`
	Details   string `json:"details,omitempty"`
}

// AITrailGroup is every audit row and the operational record for one AI action
// id; payloads pass through the assistant's redaction before they leave here.
type AITrailGroup struct {
	ActionID       string        `json:"actionId"`
	Tool           string        `json:"tool"`
	ConversationID string        `json:"conversationId,omitempty"`
	UserID         string        `json:"userId,omitempty"`
	Username       string        `json:"username,omitempty"`
	EnvironmentID  *string       `json:"environmentId,omitempty"`
	Status         string        `json:"status"`
	StartedAt      string        `json:"startedAt"`
	LastAt         string        `json:"lastAt"`
	Arguments      string        `json:"arguments,omitempty"`
	Plan           string        `json:"plan,omitempty"`
	Result         string        `json:"result,omitempty"`
	Error          string        `json:"error,omitempty"`
	Truncated      bool          `json:"truncated"`
	RecordMissing  bool          `json:"recordMissing"`
	Steps          []AITrailStep `json:"steps"`
}

// sanitizeTrailPayload redacts a stored payload with the assistant's standard
// profile and caps its length; a value that is not JSON is redacted as text.
func sanitizeTrailPayload(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	var out string
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		encoded, err := json.MarshalIndent(redact.Value(decoded, redact.Standard), "", "  ")
		if err != nil {
			return "", false
		}
		out = string(encoded)
	} else if redacted, ok := redact.Value(raw, redact.Standard).(string); ok {
		out = redacted
	}
	if utf8.RuneCountInString(out) <= maxTrailPayload {
		return out, false
	}
	return string([]rune(out)[:maxTrailPayload]), true
}

// trailStepKind maps an audit action to the step it records.
func trailStepKind(action string) string {
	return strings.TrimPrefix(action, "ai.action.")
}

type aiTrailFilter struct {
	Page     int
	PerPage  int
	ActorIDs []string
	From     time.Time
	To       time.Time
}

const aiTrailIDSource = `
	SELECT entity_id AS action_id, timestamp AS ts, COALESCE(user_id, '') AS uid
	FROM audit_logs
	WHERE entity_type = 'ai_assistant_action' AND action LIKE 'ai.action.%'
	  AND entity_id IS NOT NULL AND entity_id <> ''
	UNION ALL
	SELECT id AS action_id, executed_at AS ts, user_id AS uid
	FROM ai_assistant_action_log`

// ListAIActions pages AI action ids newest first, taken from the audit rows and
// the action log together so a preview that never got a decision still shows.
func (s *Service) ListAIActions(ctx context.Context, filter aiTrailFilter) ([]AITrailGroup, int64, error) {
	where := []string{}
	args := []any{}
	if len(filter.ActorIDs) > 0 {
		where = append(where, "uid IN ("+placeholders(len(filter.ActorIDs))+")")
		for _, id := range filter.ActorIDs {
			args = append(args, id)
		}
	}
	if !filter.From.IsZero() {
		where = append(where, "ts >= ?")
		args = append(args, filter.From.UTC().Format(time.RFC3339))
	}
	if !filter.To.IsZero() {
		where = append(where, "ts <= ?")
		args = append(args, filter.To.UTC().Format(time.RFC3339))
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}
	grouped := "SELECT action_id, MAX(ts) AS last_ts FROM (" + aiTrailIDSource + ") src" + whereClause + " GROUP BY action_id"

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+grouped+") counted", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting ai trail actions: %w", err)
	}
	pageArgs := append(append([]any{}, args...), filter.PerPage, (filter.Page-1)*filter.PerPage)
	rows, err := s.db.QueryContext(ctx, grouped+" ORDER BY last_ts DESC, action_id DESC LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing ai trail actions: %w", err)
	}
	ids, err := collectTrailActionIDs(rows)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return []AITrailGroup{}, total, nil
	}

	groups := make(map[string]*AITrailGroup, len(ids))
	for _, id := range ids {
		groups[id] = &AITrailGroup{ActionID: id, Steps: []AITrailStep{}, RecordMissing: true}
	}
	if err := s.fillTrailRecords(ctx, ids, groups); err != nil {
		return nil, 0, err
	}
	if err := s.fillTrailSteps(ctx, ids, groups); err != nil {
		return nil, 0, err
	}
	missing := []string{}
	for _, g := range groups {
		if g.Username == "" && g.UserID != "" {
			missing = append(missing, g.UserID)
		}
	}
	names := s.usernamesByID(ctx, missing)
	out := make([]AITrailGroup, 0, len(ids))
	for _, id := range ids {
		g := groups[id]
		if g.Username == "" {
			g.Username = names[g.UserID]
		}
		out = append(out, *g)
	}
	return out, total, nil
}

const (
	trailRecordsSelect = `SELECT id, conversation_id, user_id, tool, args_json, plan_json, result_json,
		executed_at, status, error FROM ai_assistant_action_log WHERE id IN (`
	trailRecordsTail = `) LIMIT 1000`
	trailStepsSelect = `SELECT id, entity_id, action, timestamp, user_id, username, entity_name, details, environment_id
		FROM audit_logs WHERE entity_type = ? AND entity_id IN (`
	trailStepsTail = `) ORDER BY timestamp ASC, id ASC LIMIT ?`
)

func (s *Service) fillTrailRecords(ctx context.Context, ids []string, groups map[string]*AITrailGroup) error {
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	query := trailRecordsSelect + placeholders(len(ids)) + trailRecordsTail // SQL identifier: placeholders() emits only "?" markers
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("reading ai action records: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()
	for rows.Next() {
		var id, conversation, userID, tool, argsJSON, planJSON, resultJSON, executedAt, status, errText string
		if err := rows.Scan(&id, &conversation, &userID, &tool, &argsJSON, &planJSON, &resultJSON, &executedAt, &status, &errText); err != nil {
			return fmt.Errorf("scanning ai action record: %w", err)
		}
		g := groups[id]
		if g == nil {
			continue
		}
		g.RecordMissing = false
		g.ConversationID = conversation
		g.UserID = userID
		g.Tool = tool
		g.Status = status
		g.StartedAt = normalizeAuditTime(executedAt)
		var cut [4]bool
		g.Arguments, cut[0] = sanitizeTrailPayload(argsJSON)
		g.Plan, cut[1] = sanitizeTrailPayload(planJSON)
		g.Result, cut[2] = sanitizeTrailPayload(resultJSON)
		g.Error, cut[3] = sanitizeTrailPayload(errText)
		g.Truncated = cut[0] || cut[1] || cut[2] || cut[3]
		g.Steps = append(g.Steps, AITrailStep{Kind: "preview", Timestamp: g.StartedAt})
	}
	return rows.Err()
}

func (s *Service) fillTrailSteps(ctx context.Context, ids []string, groups map[string]*AITrailGroup) error {
	args := []any{aiActionEntityType}
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, maxTrailAuditRows)
	query := trailStepsSelect + placeholders(len(ids)) + trailStepsTail // SQL identifier: placeholders() emits only "?" markers
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("reading ai action audit rows: %w", err)
	}
	defer func() { logRowsClose(rows.Close()) }()
	for rows.Next() {
		var id, entityID, action, ts string
		var userID, username, tool, details, envID sql.NullString
		if err := rows.Scan(&id, &entityID, &action, &ts, &userID, &username, &tool, &details, &envID); err != nil {
			return fmt.Errorf("scanning ai action audit row: %w", err)
		}
		g := groups[entityID]
		if g == nil {
			continue
		}
		stepDetails, _ := sanitizeTrailPayload(details.String)
		g.Steps = append(g.Steps, AITrailStep{
			Kind:      trailStepKind(action),
			AuditID:   id,
			Timestamp: normalizeAuditTime(ts),
			Actor:     username.String,
			Details:   stepDetails,
		})
		if g.UserID == "" {
			g.UserID = userID.String
		}
		if g.Username == "" {
			g.Username = username.String
		}
		if g.Tool == "" {
			g.Tool = tool.String
		}
		if g.EnvironmentID == nil && envID.Valid && envID.String != "" {
			env := envID.String
			g.EnvironmentID = &env
		}
		if g.Status == "" || g.RecordMissing {
			g.Status = trailStepKind(action)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range groups {
		if len(g.Steps) == 0 {
			continue
		}
		if g.StartedAt == "" {
			g.StartedAt = g.Steps[0].Timestamp
		}
		g.LastAt = g.Steps[len(g.Steps)-1].Timestamp
	}
	return nil
}

// ListAIToolEvents pages the ai.tool.* rows with their details redacted, since
// a denial message can echo arguments the model supplied.
func (s *Service) ListAIToolEvents(ctx context.Context, filter listFilter) ([]Log, int64, error) {
	filter.Actions = aiToolActions
	filter.Action = ""
	logs, total, err := s.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for i := range logs {
		if logs[i].Details != nil {
			redacted, _ := sanitizeTrailPayload(*logs[i].Details)
			logs[i].Details = &redacted
		}
		logs[i].StateBefore, logs[i].StateAfter, logs[i].ChangedKeys = nil, nil, nil
	}
	return logs, total, nil
}

// collectTrailActionIDs drains rows into action ids and closes them, so the caller can issue follow-up queries on the same pool connection.
func collectTrailActionIDs(rows *sql.Rows) ([]string, error) {
	defer func() { logRowsClose(rows.Close()) }()
	ids := []string{}
	for rows.Next() {
		var id, last string
		if err := rows.Scan(&id, &last); err != nil {
			return nil, fmt.Errorf("scanning ai trail action: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
