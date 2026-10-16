// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

type userLister interface {
	List(ctx context.Context) ([]map[string]any, error)
}

type auditSearcher interface {
	Search(ctx context.Context, userID, query string, limit int) ([]map[string]any, error)
}

func init() {
	Register(&Tool{
		Name:        "users.list",
		Description: "List users the assistant can see. Sensitive fields (email, last login) are redacted unless the requester holds users.manage.",
		InputSchema: objectSchema(map[string]any{}, []string{}),
		Required:    []rbac.Permission{rbac.PermUsersView},
		ReadOnly:    true,
		MaxBytes:    32 * 1024,
		Handler: func(ctx context.Context, sc Scope, _ json.RawMessage) (any, error) {
			d := GetDeps()
			if d == nil || d.UserList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "users service not wired"}
			}
			lister, ok := d.UserList.(userLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "users service does not implement List"}
			}
			out, err := lister.List(ctx)
			if err != nil {
				return nil, fmt.Errorf("list users: %w", err)
			}
			// Redact email unless the requester has users.manage.
			manage, permErr := d.RBAC.HasPermission(ctx, sc.UserID, sc.EnvID, rbac.PermUsersManage)
			if permErr != nil {
				slog.Warn("ai tool: users.manage check failed, redacting user fields", "error", permErr)
			}
			if !manage {
				for _, u := range out {
					delete(u, "email")
					delete(u, "lastLoginAt")
				}
			}
			return map[string]any{"count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "audit.search",
		Description: "Search the audit log by free-text query. Returns the most recent matches. Requires audit.view.",
		InputSchema: objectSchema(map[string]any{
			"query": stringSchema("Free-text search across action/entity/details."),
			"limit": intSchema("Max results. Default 50, max 200."),
		}, []string{"query"}),
		Required: []rbac.Permission{rbac.PermAuditView},
		ReadOnly: true,
		MaxBytes: 64 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			if p.Query == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "query is required"}
			}
			limit := p.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 200 {
				limit = 200
			}
			d := GetDeps()
			if d == nil || d.AuditSearch == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "audit search not wired"}
			}
			searcher, ok := d.AuditSearch.(auditSearcher)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "audit service does not implement Search"}
			}
			out, err := searcher.Search(ctx, sc.UserID, p.Query, limit)
			if err != nil {
				return nil, fmt.Errorf("audit search: %w", err)
			}
			return map[string]any{"query": p.Query, "count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "workflows.list",
		Description: "List workflow definitions the assistant can see.",
		InputSchema: objectSchema(map[string]any{}, []string{}),
		Required:    []rbac.Permission{rbac.PermWorkflowsView},
		ReadOnly:    true,
		MaxBytes:    32 * 1024,
		Handler: func(ctx context.Context, sc Scope, _ json.RawMessage) (any, error) {
			d := GetDeps()
			if d == nil || d.WorkflowList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "workflows service not wired"}
			}
			lister, ok := d.WorkflowList.(workflowLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "workflows service does not implement List"}
			}
			out, err := lister.List(ctx)
			if err != nil {
				return nil, fmt.Errorf("list workflows: %w", err)
			}
			return map[string]any{"count": len(out), "items": out}, nil
		},
	})
}

type workflowLister interface {
	List(ctx context.Context) ([]map[string]any, error)
}
