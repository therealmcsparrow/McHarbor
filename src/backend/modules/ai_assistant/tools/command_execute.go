// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// McHarborCommand is the narrow application-command envelope the AI
// action plane can execute. It intentionally targets McHarbor service
// operations, not host shell commands.
type McHarborCommand struct {
	Domain   string         `json:"domain"`
	Action   string         `json:"action"`
	EnvID    string         `json:"env_id,omitempty"`
	TargetID string         `json:"target_id"`
	Args     map[string]any `json:"args,omitempty"`
	UserID   string         `json:"user_id,omitempty"`
}

// McHarborCommandExecutor is implemented by the host package so this
// tools package does not import Docker/container modules directly.
type McHarborCommandExecutor interface {
	ExecuteMcHarborCommand(ctx context.Context, cmd McHarborCommand) (any, error)
}

func init() {
	Register(&Tool{
		Name:        "mcharbor.command.execute",
		Description: "Preview or execute an allowlisted McHarbor application command. Initial scope: container start, stop, restart, pause, unpause, and kill. Requires the AI action consent flow for execution.",
		InputSchema: objectSchema(map[string]any{
			"domain":    stringSchema("Command domain. Currently only 'container' is supported."),
			"action":    stringSchema("Allowlisted command action: start, stop, restart, pause, unpause, or kill."),
			"env_id":    stringSchema("Environment ID. Empty = current scope/default environment."),
			"target_id": stringSchema("Target resource ID or name, for container commands this is the container ID or name."),
			"args": map[string]any{
				"type":        "object",
				"description": "Optional command arguments. stop supports timeout_seconds; kill supports signal.",
			},
		}, []string{"domain", "action", "target_id"}),
		Required:        []rbac.Permission{rbac.PermAIAssistantUse, rbac.PermContainersManage},
		ReadOnly:        false,
		RequiresConsent: true,
		MaxBytes:        16 * 1024,
		Timeout:         45 * time.Second,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			cmd, err := parseMcHarborCommand(args, sc)
			if err != nil {
				return nil, err
			}
			if sc.DryRun {
				return commandPlan(cmd), nil
			}

			exec, err := resolveMcHarborCommandExecutor()
			if err != nil {
				return nil, err
			}
			result, err := exec.ExecuteMcHarborCommand(ctx, cmd)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"domain":   cmd.Domain,
				"action":   cmd.Action,
				"envId":    cmd.EnvID,
				"targetId": cmd.TargetID,
				"status":   "executed",
				"result":   result,
			}, nil
		},
	})
}

func parseMcHarborCommand(raw json.RawMessage, sc Scope) (McHarborCommand, error) {
	var cmd McHarborCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return McHarborCommand{}, err
	}
	cmd.Domain = strings.ToLower(strings.TrimSpace(cmd.Domain))
	cmd.Action = strings.ToLower(strings.TrimSpace(cmd.Action))
	cmd.EnvID = strings.TrimSpace(cmd.EnvID)
	cmd.TargetID = strings.TrimSpace(cmd.TargetID)
	cmd.UserID = sc.UserID
	if cmd.EnvID == "" {
		cmd.EnvID = sc.EnvID
	}
	if cmd.Domain == "" || cmd.Action == "" || cmd.TargetID == "" {
		return McHarborCommand{}, &CallError{Code: ErrNotFound, Message: "domain, action, and target_id are required"}
	}
	if cmd.Domain != "container" {
		return McHarborCommand{}, &CallError{Code: ErrForbidden, Message: "command domain is not allowlisted", Details: cmd.Domain}
	}
	if !isAllowedContainerCommand(cmd.Action) {
		return McHarborCommand{}, &CallError{Code: ErrForbidden, Message: "container command is not allowlisted", Details: cmd.Action}
	}
	if cmd.Args == nil {
		cmd.Args = map[string]any{}
	}
	return cmd, nil
}

func isAllowedContainerCommand(action string) bool {
	switch action {
	case "start", "stop", "restart", "pause", "unpause", "kill":
		return true
	default:
		return false
	}
}

func commandPlan(cmd McHarborCommand) map[string]any {
	return map[string]any{
		"domain":           cmd.Domain,
		"action":           cmd.Action,
		"envId":            cmd.EnvID,
		"targetId":         cmd.TargetID,
		"args":             cmd.Args,
		"approvalRequired": true,
		"willExecute":      false,
		"execution":        "Approve this AI action before McHarbor executes the command through the backend service layer.",
		"safety":           "This is an allowlisted McHarbor application command. RBAC, consent, redaction, protected-resource checks, and audit logging still apply at execution time.",
	}
}

func resolveMcHarborCommandExecutor() (McHarborCommandExecutor, error) {
	d := GetDeps()
	if d == nil || d.CommandExecutor == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "mcharbor command executor not wired"}
	}
	exec, ok := d.CommandExecutor.(McHarborCommandExecutor)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "mcharbor command executor has incompatible type"}
	}
	return exec, nil
}
