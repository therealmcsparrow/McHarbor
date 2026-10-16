// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package ai_assistant runs the AI assistant: conversations, consent-gated tool actions, knowledge and provider orchestration.
package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

// ActionStatus is the lifecycle of a consent-required tool call.
// Pending: created, awaiting user approval. Approved: user said yes,
// execution is queued. Executed: ran cleanly. Failed: ran but the
// tool errored. Denied: user said no. Expired: 24h passed without
// a decision.
type ActionStatus string

const (
	// ActionPending marks an action awaiting user approval.
	ActionPending ActionStatus = "pending"
	// ActionApproved marks an action the user approved and queued for execution.
	ActionApproved ActionStatus = "approved"
	// ActionExecuted marks an action whose tool ran cleanly.
	ActionExecuted ActionStatus = "executed"
	// ActionFailed marks an action whose tool returned an error.
	ActionFailed ActionStatus = "failed"
	// ActionDenied marks an action the user refused.
	ActionDenied ActionStatus = "denied"
	// ActionExpired marks an action that received no decision within 24 hours.
	ActionExpired ActionStatus = "expired"
)

// Action is the persisted record of one consent-required tool call.
// The plan_json field carries the dry-run output (what would happen);
// result_json carries the actual execution result once it runs.
type Action struct {
	ID             string       `json:"id"`
	ConversationID string       `json:"conversationId"`
	UserID         string       `json:"userId"`
	Tool           string       `json:"tool"`
	ArgsJSON       string       `json:"argsJson,omitempty"`
	PlanJSON       string       `json:"planJson,omitempty"`
	ResultJSON     string       `json:"resultJson,omitempty"`
	ConsentAt      *time.Time   `json:"consentAt,omitempty"`
	ExecutedAt     time.Time    `json:"executedAt"`
	IdempotencyKey string       `json:"idempotencyKey,omitempty"`
	Status         ActionStatus `json:"status"`
	Error          string       `json:"error,omitempty"`
}

// ActionPreviewRequest is the body of POST /ai-assistant/actions/preview.
// A preview runs the tool in dry-run mode (no side effects) and
// persists a pending Action so the user can review + consent.
type ActionPreviewRequest struct {
	ConversationID string          `json:"conversationId"`
	Tool           string          `json:"tool"`
	Arguments      json.RawMessage `json:"arguments"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	Scope          ActionScope     `json:"scope"`
}

// ActionScope lets the caller pin the action to a conversation / env /
// user. UserID is filled by the handler so the client can't impersonate.
type ActionScope struct {
	UserID         string `json:"userId,omitempty"`
	EnvID          string `json:"envId,omitempty"`
	StackName      string `json:"stackName,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
}

// ActionConsentRequest is the body of POST /ai-assistant/actions/{id}/consent.
// Approve=true records the consent timestamp and transitions the
// action to `approved`. Approve=false moves it to `denied`.
type ActionConsentRequest struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason,omitempty"`
}

// persistAction inserts an Action row and returns the same value
// (with the assigned ID).
func (s *Service) persistAction(ctx context.Context, a *Action) error {
	if a.ID == "" {
		a.ID = xid.New().String()
	}
	if a.ExecutedAt.IsZero() {
		a.ExecutedAt = time.Now().UTC()
	}
	if a.Status == "" {
		a.Status = ActionPending
	}
	consentAt := ""
	if a.ConsentAt != nil {
		consentAt = a.ConsentAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_assistant_action_log
		   (id, conversation_id, user_id, tool, args_json, plan_json, result_json,
		    consent_at, executed_at, idempotency_key, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ConversationID, a.UserID, a.Tool, a.ArgsJSON, a.PlanJSON, a.ResultJSON,
		consentAt, a.ExecutedAt.UTC().Format(time.RFC3339),
		nullString(a.IdempotencyKey), a.Error,
	)
	if err != nil {
		return fmt.Errorf("persist action: %w", err)
	}
	return nil
}

// updateAction applies a partial update (status / consent / result /
// error) to an existing Action row.
func (s *Service) updateAction(ctx context.Context, a *Action) error {
	consentAt := ""
	if a.ConsentAt != nil {
		consentAt = a.ConsentAt.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE ai_assistant_action_log
		 SET status = ?, consent_at = ?, plan_json = ?, result_json = ?, error = ?
		 WHERE id = ?`,
		string(a.Status), consentAt, a.PlanJSON, a.ResultJSON, a.Error, a.ID,
	)
	if err != nil {
		return fmt.Errorf("update action: %w", err)
	}
	return nil
}

// getAction returns the Action with the given id. Returns (nil, nil)
// if not found; only DB errors return (nil, err).
func (s *Service) getAction(ctx context.Context, id string) (*Action, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, user_id, tool, COALESCE(args_json, ''),
		        COALESCE(plan_json, ''), COALESCE(result_json, ''),
		        COALESCE(consent_at, ''), executed_at,
		        COALESCE(idempotency_key, ''), status, COALESCE(error, '')
		 FROM ai_assistant_action_log WHERE id = ?`,
		id,
	)
	var a Action
	var consentAt, executedAt string
	if err := row.Scan(&a.ID, &a.ConversationID, &a.UserID, &a.Tool, &a.ArgsJSON,
		&a.PlanJSON, &a.ResultJSON, &consentAt, &executedAt,
		&a.IdempotencyKey, &a.Status, &a.Error,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if consentAt != "" {
		if t, err := time.Parse(time.RFC3339, consentAt); err == nil {
			a.ConsentAt = &t
		}
	}
	if t, err := time.Parse(time.RFC3339, executedAt); err == nil {
		a.ExecutedAt = t
	}
	return &a, nil
}

// listActions returns the user's recent actions, newest first. limit
// is clamped to [1, 200].
func (s *Service) listActions(ctx context.Context, userID string, limit int) ([]Action, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, conversation_id, user_id, tool, COALESCE(args_json, ''),
		        COALESCE(plan_json, ''), COALESCE(result_json, ''),
		        COALESCE(consent_at, ''), executed_at,
		        COALESCE(idempotency_key, ''), status, COALESCE(error, '')
		 FROM ai_assistant_action_log
		 WHERE user_id = ?
		 ORDER BY executed_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]Action, 0, limit)
	for rows.Next() {
		var a Action
		var consentAt, executedAt string
		if err := rows.Scan(&a.ID, &a.ConversationID, &a.UserID, &a.Tool, &a.ArgsJSON,
			&a.PlanJSON, &a.ResultJSON, &consentAt, &executedAt,
			&a.IdempotencyKey, &a.Status, &a.Error,
		); err != nil {
			return nil, err
		}
		if consentAt != "" {
			if t, err := time.Parse(time.RFC3339, consentAt); err == nil {
				a.ConsentAt = &t
			}
		}
		if t, err := time.Parse(time.RFC3339, executedAt); err == nil {
			a.ExecutedAt = t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// findByIdempotency looks up an existing action by (user_id, idempotency_key).
// The 24h dedupe window is enforced here.
func (s *Service) findByIdempotency(ctx context.Context, userID, key string) (*Action, error) {
	if key == "" {
		return nil, nil
	}
	cutoff := time.Now().Add(-assistantHistoryWindow).UTC().Format(time.RFC3339)
	row := s.db.QueryRowContext(ctx,
		`SELECT id, conversation_id, user_id, tool, COALESCE(args_json, ''),
		        COALESCE(plan_json, ''), COALESCE(result_json, ''),
		        COALESCE(consent_at, ''), executed_at,
		        COALESCE(idempotency_key, ''), status, COALESCE(error, '')
		 FROM ai_assistant_action_log
		 WHERE user_id = ? AND idempotency_key = ? AND executed_at >= ?
		 ORDER BY executed_at DESC LIMIT 1`,
		userID, key, cutoff,
	)
	var a Action
	var consentAt, executedAt string
	if err := row.Scan(&a.ID, &a.ConversationID, &a.UserID, &a.Tool, &a.ArgsJSON,
		&a.PlanJSON, &a.ResultJSON, &consentAt, &executedAt,
		&a.IdempotencyKey, &a.Status, &a.Error,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if consentAt != "" {
		if t, err := time.Parse(time.RFC3339, consentAt); err == nil {
			a.ConsentAt = &t
		}
	}
	if t, err := time.Parse(time.RFC3339, executedAt); err == nil {
		a.ExecutedAt = t
	}
	return &a, nil
}

// PreviewAction runs a tool in dry-run mode and persists a pending
// Action. Returns the action with plan_json filled in so the
// frontend can render the consent dialog.
func (s *Service) PreviewAction(ctx context.Context, req ActionPreviewRequest) (*Action, error) {
	if strings.TrimSpace(req.Tool) == "" {
		return nil, errors.New("tool is required")
	}
	// Idempotency: if the same key was used in the last 24h, return
	// the previous Action untouched.
	if req.IdempotencyKey != "" {
		prev, err := s.findByIdempotency(ctx, req.Scope.UserID, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if prev != nil {
			return prev, nil
		}
	}

	// Run the tool with DryRun=true so handlers can short-circuit
	// side effects but still describe what they would do. The
	// actual tool registry lives in ai_tools.
	plan, perr := s.previewToolCall(ctx, req.Scope, req.Tool, req.Arguments)
	action := &Action{
		ConversationID: req.ConversationID,
		UserID:         req.Scope.UserID,
		Tool:           req.Tool,
		ArgsJSON:       string(req.Arguments),
		IdempotencyKey: req.IdempotencyKey,
		Status:         ActionPending,
		ExecutedAt:     time.Now().UTC(),
	}
	if perr != nil {
		action.Status = ActionFailed
		action.Error = perr.Error()
	}
	action.ArgsJSON = redactedRawJSON(req.Arguments)
	if plan != nil {
		b, err := marshalRedactedJSON(plan)
		if err != nil {
			return nil, err
		}
		action.PlanJSON = string(b)
	}
	if err := s.persistAction(ctx, action); err != nil {
		return nil, err
	}
	return action, nil
}

// ConsentAction applies the user's approve/deny decision to a
// pending action. Approve sets the consent_at timestamp and the
// status to approved. Deny sets status to denied and stops.
func (s *Service) ConsentAction(ctx context.Context, id, userID string, req ActionConsentRequest) (*Action, error) {
	a, err := s.getAction(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("action not found")
	}
	if a.UserID != userID {
		return nil, fmt.Errorf("action not found")
	}
	if a.Status != ActionPending {
		return nil, fmt.Errorf("action is not pending (status=%s)", a.Status)
	}
	now := time.Now().UTC()
	a.ConsentAt = &now
	if req.Approve {
		a.Status = ActionApproved
	} else {
		a.Status = ActionDenied
		if req.Reason != "" {
			a.Error = "denied: " + req.Reason
		}
	}
	if err := s.updateAction(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// ExecuteAction runs an approved action via the tool registry and
// stores the result. Idempotency: a key already executed returns the
// previous result.
func (s *Service) ExecuteAction(ctx context.Context, id, userID string) (*Action, error) {
	a, err := s.getAction(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("action not found")
	}
	if a.UserID != userID {
		return nil, fmt.Errorf("action not found")
	}
	if a.Status == ActionExecuted {
		return a, nil // already ran
	}
	if a.Status != ActionApproved {
		return nil, fmt.Errorf("action is not approved (status=%s)", a.Status)
	}
	args := json.RawMessage(a.ArgsJSON)
	scope := ActionScope{
		UserID:         a.UserID,
		ConversationID: a.ConversationID,
	}
	// Match the scope the original preview used by reading the env
	// from plan_json; otherwise leave blank.
	scope.EnvID = planEnvID(a.PlanJSON)
	result, rerr := s.executeToolCall(ctx, scope, a.Tool, args)
	if result != nil {
		b, err := marshalRedactedJSON(result)
		if err != nil {
			return nil, err
		}
		a.ResultJSON = string(b)
	}
	if rerr != nil {
		a.Status = ActionFailed
		a.Error = rerr.Error()
	} else {
		a.Status = ActionExecuted
	}
	if err := s.updateAction(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// previewToolCall runs a tool in DryRun mode. Tools that don't yet
// implement DryRun still execute normally; Phase 4 wires per-tool
// dry-run flags. The returned plan is what the consent dialog renders.
func (s *Service) previewToolCall(ctx context.Context, scope ActionScope, tool string, args json.RawMessage) (any, error) {
	_, ok := ai_tools.Get(tool)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", tool)
	}
	req := ai_tools.CallRequest{
		Name:      tool,
		Arguments: args,
		Scope: ai_tools.Scope{
			UserID:         scope.UserID,
			EnvID:          scope.EnvID,
			StackName:      scope.StackName,
			ConversationID: scope.ConversationID,
		},
		DryRun: true,
	}
	authz := s.actionAuthorizer()
	res, callErr := ai_tools.Call(ctx, authz, nil, ai_tools.GetCache(), req)
	if callErr != nil {
		return nil, fmt.Errorf("%s", callErr.Message)
	}
	return res.Data, nil
}

// executeToolCall runs a tool for real (DryRun=false). The caller is
// responsible for having collected consent first.
func (s *Service) executeToolCall(ctx context.Context, scope ActionScope, tool string, args json.RawMessage) (any, error) {
	t, ok := ai_tools.Get(tool)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", tool)
	}
	_ = t
	req := ai_tools.CallRequest{
		Name:      tool,
		Arguments: args,
		Scope: ai_tools.Scope{
			UserID:         scope.UserID,
			EnvID:          scope.EnvID,
			StackName:      scope.StackName,
			ConversationID: scope.ConversationID,
		},
		DryRun:         false,
		ConsentGranted: true,
	}
	authz := s.actionAuthorizer()
	// Unlike the preview path this call has real side effects, so it goes
	// through the audit hook. The hook captures the actor from ctx because
	// there is no *http.Request at this layer.
	var hook ai_tools.AuditHook
	if s.audit != nil {
		hook = ctxAuditHook{logger: s.audit, user: auth.UserFromContext(ctx)}
	}
	res, callErr := ai_tools.Call(ctx, authz, hook, ai_tools.GetCache(), req)
	if callErr != nil {
		return nil, fmt.Errorf("%s", callErr.Message)
	}
	return res.Data, nil
}

// planEnvID pulls the environment ID out of a stored plan_json blob.
// Preview plans render it as "envId"; a few tools emit the snake_case
// "env_id". Returns "" when the plan is empty, unparsable, or env-less.
func planEnvID(planJSON string) string {
	if planJSON == "" {
		return ""
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return ""
	}
	if v, ok := plan["envId"].(string); ok {
		return v
	}
	if v, ok := plan["env_id"].(string); ok {
		return v
	}
	return ""
}

// actionAuthorizer adapts s.rbac to the ai_tools.Authorizer interface.
// When the service has no RBAC (tests, dev mode) it returns
// permissive=true so the action plane still works.
func (s *Service) actionAuthorizer() ai_tools.Authorizer {
	return rbacActionAuthorizer{svc: s.rbac}
}

type rbacActionAuthorizer struct {
	svc *rbac.Service
}

func (a rbacActionAuthorizer) HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error) {
	if a.svc == nil {
		return true, nil
	}
	return a.svc.HasPermission(ctx, userID, envID, perm)
}

func redactedRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(ai_tools.RedactJSON(raw, ai_tools.RedactStandard))
}

func marshalRedactedJSON(v any) ([]byte, error) {
	redacted := ai_tools.Redact(v, ai_tools.RedactStandard)
	b, err := json.Marshal(redacted)
	if err != nil {
		return nil, fmt.Errorf("marshal redacted action payload: %w", err)
	}
	return b, nil
}
