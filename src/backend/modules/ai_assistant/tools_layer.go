// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

// BuildPluginTools converts the per-user filtered ai_tools catalog
// into the wire-format plugin.LLMTool slice the provider adapters
// consume. The catalog is filtered by RBAC and the request's
// environment scope, so the model only sees tools it could legally
// invoke right now.
//
// Returns nil when the user has no callable tools (RBAC denies all).
func BuildPluginTools(ctx context.Context, authz ai_tools.Authorizer, userID, envID string) []plugin.LLMTool {
	catalog := ai_tools.CatalogFor(ctx, authz, userID, envID)
	if len(catalog) == 0 {
		return nil
	}
	out := make([]plugin.LLMTool, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, plugin.LLMTool{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.InputSchema,
		})
	}
	return out
}

// DispatchPluginToolCall runs one structured tool call emitted by the
// model. Read-only tools execute immediately and the returned message
// is fed back to the model as the next turn's tool result. Mutating
// or consent-required tools persist a pending Action and return a
// short acknowledgement for the model — the actual execution waits
// on the user's approve/deny click in the action panel.
//
// The returned (modelMsg, pendingAction) tells the chat loop what to
// do next:
//
//	modelMsg != ""      → append as a tool-result message and loop
//	pendingAction != nil → break the loop, let the model write its
//	                       user-facing summary referencing action.ID
//	both empty          → tool ran with no observable result; loop
//	err != nil          → return the error to the caller; the model
//	                       will see it on the next turn as a tool-
//	                       result message with the error text
func DispatchPluginToolCall(ctx context.Context, svc *Service, scope ActionScope, call plugin.LLMToolCall) (modelMsg string, pendingAction *Action, err error) {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return "", nil, fmt.Errorf("empty tool name")
	}
	args := call.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	t, ok := ai_tools.Get(name)
	if !ok {
		return "", nil, fmt.Errorf("tool %q is not registered", name)
	}

	if t.RequiresConsent || !t.ReadOnly {
		action, perr := svc.PreviewAction(ctx, ActionPreviewRequest{
			ConversationID: scope.ConversationID,
			Tool:           name,
			Arguments:      args,
			Scope:          scope,
		})
		if perr != nil {
			return "", nil, fmt.Errorf("preview tool %s: %w", name, perr)
		}
		summary := actionPlanForModel(action)
		return summary, action, nil
	}

	result, callErr := ai_tools.Call(ctx, svc.actionAuthorizer(), nil, ai_tools.GetCache(), ai_tools.CallRequest{
		Name:      name,
		Arguments: args,
		Scope: ai_tools.Scope{
			UserID:         scope.UserID,
			EnvID:          scope.EnvID,
			StackName:      scope.StackName,
			ConversationID: scope.ConversationID,
		},
	})
	if callErr != nil {
		return fmt.Sprintf("tool %s failed: %s", name, callErr.Message), nil, nil //nolint:nilerr // a failed tool is reported to the model as text, not as a turn error
	}
	encoded, mErr := json.Marshal(result.Data)
	if mErr != nil {
		return "", nil, fmt.Errorf("encode tool result for %s: %w", name, mErr)
	}
	return string(encoded), nil, nil
}

// actionPlanForModel produces the short tool-result message the model
// sees when a mutating tool is previewed. The model uses it to write
// the user-facing "I prepared an action…" reply and to reference the
// action ID for downstream turns.
func actionPlanForModel(a *Action) string {
	if a == nil {
		return ""
	}
	status := string(a.Status)
	if a.Error != "" {
		status = status + ": " + a.Error
	}
	return fmt.Sprintf(
		"action %s queued (%s) — user must approve before execution. "+
			"Tool: %s. Args: %s. Plan: %s",
		a.ID, status, a.Tool, a.ArgsJSON, a.PlanJSON,
	)
}

// LoadRecentActionOutcomes returns the executed-or-denied actions for
// the conversation that the model has not yet been told about. The
// chat loop prepends these to the next user turn so the model knows
// what happened to the actions it queued on the previous turn.
//
// We only return outcomes that landed in a terminal state
// (executed/failed/denied/expired) AFTER the last user message in
// the conversation — anything earlier has already been fed back.
func LoadRecentActionOutcomes(ctx context.Context, db *sql.DB, conversationID string, sinceRFC3339 string) ([]plugin.LLMMessage, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, tool, COALESCE(args_json, ''), COALESCE(plan_json, ''),
		        COALESCE(result_json, ''), COALESCE(error, ''), status, executed_at
		 FROM ai_assistant_action_log
		 WHERE conversation_id = ? AND executed_at >= ?
		   AND status IN ('executed','failed','denied','expired')
		 ORDER BY executed_at ASC`,
		conversationID, sinceRFC3339,
	)
	if err != nil {
		return nil, fmt.Errorf("query action outcomes: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	var out []plugin.LLMMessage
	for rows.Next() {
		var (
			id, tool, argsJSON, planJSON, resultJSON, errStr, status string
			executedAt                                               string
		)
		if scanErr := rows.Scan(&id, &tool, &argsJSON, &planJSON, &resultJSON, &errStr, &status, &executedAt); scanErr != nil {
			return nil, fmt.Errorf("scan action outcome: %w", scanErr)
		}
		var summary strings.Builder
		fmt.Fprintf(&summary,
			"action %s (%s) — tool=%s status=%s at=%s",
			id, tool, tool, status, executedAt,
		)
		if resultJSON != "" {
			summary.WriteString("\nresult: ")
			summary.WriteString(resultJSON)
		}
		if errStr != "" {
			summary.WriteString("\nerror: ")
			summary.WriteString(errStr)
		}
		out = append(out, plugin.LLMMessage{
			Role:    "user",
			Content: summary.String(),
		})
	}
	return out, rows.Err()
}
