// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// WorkflowDraft is the workflows.create payload. CanvasData and Variables are raw
// JSON objects; empty values fall back to the workflows module's own defaults.
type WorkflowDraft struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CanvasData  string `json:"canvasData,omitempty"`
	Variables   string `json:"variables,omitempty"`
}

// AlertDraft is the alerts.create payload. SourceType and ConditionKey must name a
// pair the alert registry currently knows, which alerts.conditions enumerates.
type AlertDraft struct {
	Name                  string   `json:"name"`
	Severity              string   `json:"severity,omitempty"`
	SourceType            string   `json:"sourceType"`
	ConditionKey          string   `json:"conditionKey"`
	Operator              string   `json:"operator,omitempty"`
	Threshold             float64  `json:"threshold,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	ForSeconds            int      `json:"forSeconds,omitempty"`
	WindowSeconds         int      `json:"windowSeconds,omitempty"`
	Target                string   `json:"target,omitempty"`
	EnvironmentIDs        []string `json:"environmentIds,omitempty"`
	DestinationChannelIDs []string `json:"destinationChannelIds,omitempty"`
	SendInApp             bool     `json:"sendInApp"`
	CooldownSeconds       int      `json:"cooldownSeconds,omitempty"`
	NotifyOnRecovery      bool     `json:"notifyOnRecovery"`
	MessageTemplate       string   `json:"messageTemplate,omitempty"`
}

// UpdatePolicyDraft is the updates.policy.create payload.
type UpdatePolicyDraft struct {
	Name           string `json:"name"`
	ContainerMatch string `json:"containerMatch,omitempty"`
	ImageMatch     string `json:"imageMatch,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	Strategy       string `json:"strategy,omitempty"`
	AutoRestart    bool   `json:"autoRestart"`
}

// ScheduleDraft is the schedules.create payload.
type ScheduleDraft struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Cron        string `json:"cron"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	EnvID       string `json:"envId,omitempty"`
}

// workflowAuthor, and the three author interfaces below it, are implemented in
// internal/bootstrap so this package never imports the modules that own the writes.
type workflowAuthor interface {
	CreateWorkflow(ctx context.Context, in WorkflowDraft) (map[string]any, error)
}

type alertAuthor interface {
	CreateAlert(ctx context.Context, in AlertDraft) (map[string]any, error)
}

type updatePolicyAuthor interface {
	CreateUpdatePolicy(ctx context.Context, in UpdatePolicyDraft) (map[string]any, error)
}

type scheduleAuthor interface {
	CreateSchedule(ctx context.Context, in ScheduleDraft) (map[string]any, error)
}

type alertCatalogReader interface {
	AlertConditions(ctx context.Context) (any, error)
}

// The three maps below close the value sets the model may choose from; anything
// outside them is rejected before any service is reached.
var (
	validAuthoringSeverities = map[string]bool{"critical": true, "warning": true, "info": true}
	validUpdateStrategies    = map[string]bool{"latest": true, "semver": true, "digest": true}
	validScheduleActions     = map[string]bool{"start": true, "stop": true, "restart": true, "exec": true}
)

// emptyWorkflowCanvas mirrors what the workflows module writes for a blank workflow,
// so a draft created here opens identically in the editor.
const emptyWorkflowCanvas = `{"nodes":[],"edges":[],"groups":[],"viewport":{"x":0,"y":0,"zoom":1}}`

func init() {
	Register(&Tool{
		Name:        "alerts.conditions",
		Description: "List every alert source and the conditions it exposes, with the operators, units and default thresholds each accepts. Call this before alerts.create to pick a valid source_type / condition_key pair.",
		InputSchema: objectSchema(map[string]any{}, []string{}),
		Required:    []rbac.Permission{rbac.PermAlertsView},
		ReadOnly:    true,
		MaxBytes:    64 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			reader, err := resolveAlertCatalogReader()
			if err != nil {
				return nil, err
			}
			catalog, cErr := reader.AlertConditions(ctx)
			if cErr != nil {
				return nil, fmt.Errorf("read alert condition catalog: %w", cErr)
			}
			return catalog, nil
		},
	})

	Register(&Tool{
		Name:        "workflows.create",
		Description: "Create a new workflow. It always lands as a draft so a human enables it. Pass canvas_data to seed the canvas with nodes and edges; omit it for an empty canvas the user wires up in the editor. Requires approval.",
		InputSchema: objectSchema(map[string]any{
			"name":        stringSchema("Workflow name."),
			"description": stringSchema("What the workflow does."),
			"canvas_data": stringSchema("Optional canvas JSON object with nodes, edges, groups and viewport keys, encoded as a string."),
			"variables":   stringSchema("Optional workflow variables as a JSON object encoded as a string."),
		}, []string{"name"}),
		Required:        []rbac.Permission{rbac.PermAIAssistantAct, rbac.PermWorkflowsManage},
		ReadOnly:        false,
		RequiresConsent: true,
		MaxBytes:        32 * 1024,
		Timeout:         45 * time.Second,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				CanvasData  string `json:"canvas_data"`
				Variables   string `json:"variables"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			draft := WorkflowDraft{
				Name:        strings.TrimSpace(p.Name),
				Description: strings.TrimSpace(p.Description),
				CanvasData:  strings.TrimSpace(p.CanvasData),
				Variables:   strings.TrimSpace(p.Variables),
			}
			if draft.Name == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "name is required"}
			}
			if draft.CanvasData == "" {
				draft.CanvasData = emptyWorkflowCanvas
			}
			if err := requireJSONObject(draft.CanvasData, "canvas_data"); err != nil {
				return nil, err
			}
			if draft.Variables != "" {
				if err := requireJSONObject(draft.Variables, "variables"); err != nil {
					return nil, err
				}
			}
			if sc.DryRun {
				return authoringPlan("workflow", draft.Name, sc.EnvID, map[string]any{
					"description": draft.Description,
					"status":      "draft",
					"nodeCount":   canvasNodeCount(draft.CanvasData),
				}), nil
			}
			author, err := resolveWorkflowAuthor()
			if err != nil {
				return nil, err
			}
			created, cErr := author.CreateWorkflow(ctx, draft)
			if cErr != nil {
				return nil, fmt.Errorf("create workflow: %w", cErr)
			}
			return authoringResult("workflow", created), nil
		},
	})

	Register(&Tool{
		Name:        "alerts.create",
		Description: "Create an alert rule. source_type and condition_key must come from alerts.conditions. With no destination channels the rule notifies in-app only. Requires approval.",
		InputSchema: objectSchema(map[string]any{
			"name":          stringSchema("Rule name."),
			"severity":      stringSchema("One of: critical, warning, info. Defaults to warning."),
			"source_type":   stringSchema("Alert source, from alerts.conditions."),
			"condition_key": stringSchema("Condition key within that source, from alerts.conditions."),
			"operator":      stringSchema("Comparison operator for threshold conditions, e.g. gt, gte, lt, lte."),
			"threshold": map[string]any{
				"type":        "number",
				"description": "Threshold value for threshold conditions.",
			},
			"unit":           stringSchema("Threshold unit, when the condition declares one."),
			"for_seconds":    intSchema("How long the condition must hold before firing."),
			"window_seconds": intSchema("Measurement window for count-in-window conditions."),
			"target":         stringSchema("Container or environment name/id pattern, or * for all."),
			"environment_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Restrict the rule to these environment IDs. Empty means every active environment.",
			},
			"destination_channel_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Communication channel IDs to notify.",
			},
			"send_in_app": map[string]any{
				"type":        "boolean",
				"description": "Deliver to the in-app notification centre. Forced on when no destination channels are given.",
			},
			"cooldown_seconds": intSchema("Minimum seconds between repeat notifications."),
			"notify_on_recovery": map[string]any{
				"type":        "boolean",
				"description": "Also notify when the condition clears. Defaults to true.",
			},
			"message_template": stringSchema("Optional message template override."),
		}, []string{"name", "source_type", "condition_key"}),
		Required:        []rbac.Permission{rbac.PermAIAssistantAct, rbac.PermAlertsManage},
		ReadOnly:        false,
		RequiresConsent: true,
		MaxBytes:        32 * 1024,
		Timeout:         45 * time.Second,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Name                  string   `json:"name"`
				Severity              string   `json:"severity"`
				SourceType            string   `json:"source_type"`
				ConditionKey          string   `json:"condition_key"`
				Operator              string   `json:"operator"`
				Threshold             float64  `json:"threshold"`
				Unit                  string   `json:"unit"`
				ForSeconds            int      `json:"for_seconds"`
				WindowSeconds         int      `json:"window_seconds"`
				Target                string   `json:"target"`
				EnvironmentIDs        []string `json:"environment_ids"`
				DestinationChannelIDs []string `json:"destination_channel_ids"`
				SendInApp             *bool    `json:"send_in_app"`
				CooldownSeconds       int      `json:"cooldown_seconds"`
				NotifyOnRecovery      *bool    `json:"notify_on_recovery"`
				MessageTemplate       string   `json:"message_template"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			draft := AlertDraft{
				Name:                  strings.TrimSpace(p.Name),
				Severity:              strings.ToLower(strings.TrimSpace(p.Severity)),
				SourceType:            strings.TrimSpace(p.SourceType),
				ConditionKey:          strings.TrimSpace(p.ConditionKey),
				Operator:              strings.TrimSpace(p.Operator),
				Threshold:             p.Threshold,
				Unit:                  strings.TrimSpace(p.Unit),
				ForSeconds:            p.ForSeconds,
				WindowSeconds:         p.WindowSeconds,
				Target:                strings.TrimSpace(p.Target),
				EnvironmentIDs:        trimmedList(p.EnvironmentIDs),
				DestinationChannelIDs: trimmedList(p.DestinationChannelIDs),
				CooldownSeconds:       p.CooldownSeconds,
				MessageTemplate:       p.MessageTemplate,
				NotifyOnRecovery:      p.NotifyOnRecovery == nil || *p.NotifyOnRecovery,
			}
			if draft.Name == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "name is required"}
			}
			if draft.SourceType == "" || draft.ConditionKey == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "source_type and condition_key are required; call alerts.conditions to list them"}
			}
			if draft.Severity == "" {
				draft.Severity = "warning"
			}
			if !validAuthoringSeverities[draft.Severity] {
				return nil, &CallError{Code: ErrForbidden, Message: "severity must be critical, warning, or info", Details: draft.Severity}
			}
			if len(draft.DestinationChannelIDs) == 0 {
				draft.SendInApp = true
			} else {
				draft.SendInApp = p.SendInApp == nil || *p.SendInApp
			}
			if draft.CooldownSeconds < 0 || draft.ForSeconds < 0 || draft.WindowSeconds < 0 {
				return nil, &CallError{Code: ErrForbidden, Message: "durations must not be negative"}
			}
			if sc.DryRun {
				return authoringPlan("alert", draft.Name, sc.EnvID, map[string]any{
					"severity":       draft.Severity,
					"sourceType":     draft.SourceType,
					"conditionKey":   draft.ConditionKey,
					"operator":       draft.Operator,
					"threshold":      draft.Threshold,
					"target":         draft.Target,
					"sendInApp":      draft.SendInApp,
					"destinations":   len(draft.DestinationChannelIDs),
					"environmentIds": draft.EnvironmentIDs,
				}), nil
			}
			author, err := resolveAlertAuthor()
			if err != nil {
				return nil, err
			}
			created, cErr := author.CreateAlert(ctx, draft)
			if cErr != nil {
				return nil, fmt.Errorf("create alert: %w", cErr)
			}
			return authoringResult("alert", created), nil
		},
	})

	Register(&Tool{
		Name:        "updates.policy.create",
		Description: "Create a container update policy: which containers or images it matches, on what cron schedule it checks, which tag strategy it follows, and whether it restarts the container afterwards. Requires approval.",
		InputSchema: objectSchema(map[string]any{
			"name":            stringSchema("Policy name."),
			"container_match": stringSchema("Container name pattern the policy applies to, e.g. web-* or * for all."),
			"image_match":     stringSchema("Image reference pattern the policy applies to, e.g. nginx:*."),
			"schedule":        stringSchema("Cron expression for the update check, e.g. 0 3 * * *."),
			"strategy":        stringSchema("One of: latest, semver, digest. Defaults to latest."),
			"auto_restart": map[string]any{
				"type":        "boolean",
				"description": "Restart the container after a successful update.",
			},
		}, []string{"name"}),
		Required:        []rbac.Permission{rbac.PermAIAssistantAct, rbac.PermUpdatesManage},
		ReadOnly:        false,
		RequiresConsent: true,
		MaxBytes:        16 * 1024,
		Timeout:         45 * time.Second,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Name           string `json:"name"`
				ContainerMatch string `json:"container_match"`
				ImageMatch     string `json:"image_match"`
				Schedule       string `json:"schedule"`
				Strategy       string `json:"strategy"`
				AutoRestart    bool   `json:"auto_restart"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			draft := UpdatePolicyDraft{
				Name:           strings.TrimSpace(p.Name),
				ContainerMatch: strings.TrimSpace(p.ContainerMatch),
				ImageMatch:     strings.TrimSpace(p.ImageMatch),
				Schedule:       strings.TrimSpace(p.Schedule),
				Strategy:       strings.ToLower(strings.TrimSpace(p.Strategy)),
				AutoRestart:    p.AutoRestart,
			}
			if draft.Name == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "name is required"}
			}
			if draft.Strategy == "" {
				draft.Strategy = "latest"
			}
			if !validUpdateStrategies[draft.Strategy] {
				return nil, &CallError{Code: ErrForbidden, Message: "strategy must be latest, semver, or digest", Details: draft.Strategy}
			}
			if draft.ContainerMatch == "" && draft.ImageMatch == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "container_match or image_match is required so the policy cannot match everything by accident"}
			}
			if sc.DryRun {
				return authoringPlan("update_policy", draft.Name, sc.EnvID, map[string]any{
					"containerMatch": draft.ContainerMatch,
					"imageMatch":     draft.ImageMatch,
					"schedule":       draft.Schedule,
					"strategy":       draft.Strategy,
					"autoRestart":    draft.AutoRestart,
				}), nil
			}
			author, err := resolveUpdatePolicyAuthor()
			if err != nil {
				return nil, err
			}
			created, cErr := author.CreateUpdatePolicy(ctx, draft)
			if cErr != nil {
				return nil, fmt.Errorf("create update policy: %w", cErr)
			}
			return authoringResult("update_policy", created), nil
		},
	})

	Register(&Tool{
		Name:        "schedules.create",
		Description: "Create a scheduled automation that runs a container action on a cron expression. Allowed actions: start, stop, restart, exec. Requires approval.",
		InputSchema: objectSchema(map[string]any{
			"name":        stringSchema("Schedule name."),
			"description": stringSchema("What the schedule does."),
			"cron":        stringSchema("Cron expression, e.g. 0 4 * * *."),
			"action":      stringSchema("One of: start, stop, restart, exec."),
			"target":      stringSchema("Container ID or name the action runs against."),
			"env_id":      stringSchema("Environment ID. Empty = current scope."),
		}, []string{"name", "cron", "action", "target"}),
		Required:        []rbac.Permission{rbac.PermAIAssistantAct, rbac.PermSchedulesManage},
		ReadOnly:        false,
		RequiresConsent: true,
		MaxBytes:        16 * 1024,
		Timeout:         45 * time.Second,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Cron        string `json:"cron"`
				Action      string `json:"action"`
				Target      string `json:"target"`
				EnvID       string `json:"env_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			draft := ScheduleDraft{
				Name:        strings.TrimSpace(p.Name),
				Description: strings.TrimSpace(p.Description),
				Cron:        strings.TrimSpace(p.Cron),
				Action:      strings.ToLower(strings.TrimSpace(p.Action)),
				Target:      strings.TrimSpace(p.Target),
				EnvID:       strings.TrimSpace(p.EnvID),
			}
			if draft.EnvID == "" {
				draft.EnvID = sc.EnvID
			}
			if draft.Name == "" || draft.Cron == "" || draft.Target == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "name, cron, and target are required"}
			}
			if !validScheduleActions[draft.Action] {
				return nil, &CallError{Code: ErrForbidden, Message: "action must be start, stop, restart, or exec", Details: draft.Action}
			}
			if sc.DryRun {
				return authoringPlan("schedule", draft.Name, draft.EnvID, map[string]any{
					"cron":        draft.Cron,
					"action":      draft.Action,
					"target":      draft.Target,
					"description": draft.Description,
				}), nil
			}
			author, err := resolveScheduleAuthor()
			if err != nil {
				return nil, err
			}
			created, cErr := author.CreateSchedule(ctx, draft)
			if cErr != nil {
				return nil, fmt.Errorf("create schedule: %w", cErr)
			}
			return authoringResult("schedule", created), nil
		},
	})
}

// authoringPlan renders the dry-run card the consent dialog shows. The envId key is
// load-bearing: ExecuteAction reads it back to rebuild the RBAC scope of the preview.
func authoringPlan(kind, name, envID string, fields map[string]any) map[string]any {
	plan := map[string]any{
		"kind":             kind,
		"operation":        "create",
		"name":             name,
		"envId":            envID,
		"approvalRequired": true,
		"willExecute":      false,
		"safety":           "Nothing has been created yet. RBAC, redaction and audit logging all run again when the approved action executes.",
	}
	for k, v := range fields {
		plan[k] = v
	}
	return plan
}

func authoringResult(kind string, created map[string]any) map[string]any {
	return map[string]any{
		"kind":      kind,
		"operation": "create",
		"status":    "executed",
		"result":    created,
	}
}

// requireJSONObject rejects a JSON array or scalar, which would otherwise reach the
// service layer and be stored as a canvas the editor cannot open.
func requireJSONObject(raw, field string) error {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil || obj == nil {
		return &CallError{Code: ErrForbidden, Message: field + " must be a JSON object"}
	}
	return nil
}

func canvasNodeCount(canvas string) int {
	var parsed struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(canvas), &parsed); err != nil {
		return 0
	}
	return len(parsed.Nodes)
}

func trimmedList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveWorkflowAuthor, like the four resolvers below it, turns a missing or
// mistyped injector into a tool-disabled error rather than a nil-pointer panic.
func resolveWorkflowAuthor() (workflowAuthor, error) {
	d := GetDeps()
	if d == nil || d.WorkflowAuthor == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "workflow authoring not wired"}
	}
	a, ok := d.WorkflowAuthor.(workflowAuthor)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "workflow author has incompatible type"}
	}
	return a, nil
}

func resolveAlertAuthor() (alertAuthor, error) {
	d := GetDeps()
	if d == nil || d.AlertAuthor == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "alert authoring not wired"}
	}
	a, ok := d.AlertAuthor.(alertAuthor)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "alert author has incompatible type"}
	}
	return a, nil
}

func resolveUpdatePolicyAuthor() (updatePolicyAuthor, error) {
	d := GetDeps()
	if d == nil || d.UpdatePolicyAuthor == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "update policy authoring not wired"}
	}
	a, ok := d.UpdatePolicyAuthor.(updatePolicyAuthor)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "update policy author has incompatible type"}
	}
	return a, nil
}

func resolveScheduleAuthor() (scheduleAuthor, error) {
	d := GetDeps()
	if d == nil || d.ScheduleAuthor == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "schedule authoring not wired"}
	}
	a, ok := d.ScheduleAuthor.(scheduleAuthor)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "schedule author has incompatible type"}
	}
	return a, nil
}

func resolveAlertCatalogReader() (alertCatalogReader, error) {
	d := GetDeps()
	if d == nil || d.AlertCatalog == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "alert catalog not wired"}
	}
	a, ok := d.AlertCatalog.(alertCatalogReader)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "alert catalog has incompatible type"}
	}
	return a, nil
}
