// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// AI tool adapters: the ai_assistant module declares which capabilities its
// tools need (as `any` injectors on ai_tools.Deps) and this file supplies
// them from the concrete module services. Living here keeps ai_assistant
// free of imports from sibling modules.

package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	aiassistant "github.com/therealmcsparrow/mcharbor/modules/ai_assistant"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/environments"
	"github.com/therealmcsparrow/mcharbor/modules/images"
	"github.com/therealmcsparrow/mcharbor/modules/networks"
	"github.com/therealmcsparrow/mcharbor/modules/schedules"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
	"github.com/therealmcsparrow/mcharbor/modules/updates"
	"github.com/therealmcsparrow/mcharbor/modules/users"
	"github.com/therealmcsparrow/mcharbor/modules/volumes"
	"github.com/therealmcsparrow/mcharbor/modules/workflows"
)

// --- containers ----------------------------------------------------------

// containerListAdapter wraps *containers.Service so it satisfies the
// ai_tools containerLister / containerInspecter / containerLogReader
// interfaces. The AI tools want []map[string]any; the real service
// returns strongly-typed structs. The adapter does the conversion.
type containerListAdapter struct {
	svc *containers.Service
}

func (a *containerListAdapter) List(ctx context.Context, envID string) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("containers service not wired")
	}
	items, err := a.svc.List(ctx, envID, true)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, c := range items {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out = append(out, map[string]any{
			"id":           c.ID,
			"name":         name,
			"image":        c.Image,
			"imageId":      c.ImageID,
			"state":        c.State,
			"status":       c.Status,
			"ports":        c.Ports,
			"labels":       c.Labels,
			"stackName":    c.StackName,
			"stackService": c.StackService,
			"created":      c.Created,
			"protected":    c.Protected,
		})
	}
	return out, nil
}

func (a *containerListAdapter) Inspect(ctx context.Context, envID, id string) (map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("containers service not wired")
	}
	v, err := a.svc.Inspect(ctx, envID, id)
	if err != nil {
		return nil, err
	}
	// The tool output leaves for an external model and has no reveal path, so every
	// credential-looking value is withheld and a McHarbor container's keys with them.
	envredact.Policy{McHarbor: coredocker.IsMcHarborInstance(v)}.Container(&v)
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *containerListAdapter) Logs(ctx context.Context, envID, id string, tail int, since, until time.Time) ([]string, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("containers service not wired")
	}
	if tail <= 0 {
		tail = 100
	}
	if tail > 5000 {
		tail = 5000
	}
	q := containers.LogsQuery{
		Stdout: true,
		Stderr: true,
		Tail:   fmt.Sprintf("%d", tail),
	}
	if !since.IsZero() {
		q.Since = since.UTC().Format(time.RFC3339Nano)
	}
	_ = until // services.LogsQuery has no Until field; until-time tailing is Phase 7.
	raw, err := a.svc.Logs(ctx, envID, id, q)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

type mcharborCommandAdapter struct {
	containers *containers.Service
	audit      *audit.Logger
}

func (a *mcharborCommandAdapter) ExecuteMcHarborCommand(ctx context.Context, cmd ai_tools.McHarborCommand) (any, error) {
	if cmd.Domain != "container" {
		return nil, fmt.Errorf("unsupported command domain %q", cmd.Domain)
	}
	if a.containers == nil {
		return nil, fmt.Errorf("containers service not wired")
	}

	// Resolve the display name before mutating: a killed or stopped
	// container is still inspectable, but resolving up front keeps the
	// audit label correct for any future destructive command too.
	name := a.containers.AuditName(ctx, cmd.EnvID, cmd.TargetID)

	switch cmd.Action {
	case "start":
		if err := a.containers.Start(ctx, cmd.EnvID, cmd.TargetID); err != nil {
			return nil, err
		}
	case "stop":
		if err := a.containers.Stop(ctx, cmd.EnvID, cmd.TargetID, intArg(cmd.Args, "timeout_seconds")); err != nil {
			return nil, err
		}
	case "restart":
		if err := a.containers.Restart(ctx, cmd.EnvID, cmd.TargetID); err != nil {
			return nil, err
		}
	case "pause":
		if err := a.containers.Pause(ctx, cmd.EnvID, cmd.TargetID); err != nil {
			return nil, err
		}
	case "unpause":
		if err := a.containers.Unpause(ctx, cmd.EnvID, cmd.TargetID); err != nil {
			return nil, err
		}
	case "kill":
		if err := a.containers.Kill(ctx, cmd.EnvID, cmd.TargetID, stringArg(cmd.Args, "signal")); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported container command %q", cmd.Action)
	}

	a.logCommandAudit(ctx, cmd, name)

	return map[string]any{
		"message":    "command executed",
		"domain":     cmd.Domain,
		"action":     cmd.Action,
		"envId":      cmd.EnvID,
		"targetId":   cmd.TargetID,
		"targetType": "container",
	}, nil
}

// logCommandAudit writes the same container-typed audit entry the REST
// handlers write, so an AI-driven stop/start shows up next to a UI-driven
// one instead of only as an opaque ai_assistant_action row. The actor comes
// from the request context, which survives the whole call chain
// (handler r.Context() -> ExecuteAction -> executeToolCall -> ai_tools.Call
// -> tool handler -> here); there is no *http.Request this deep, so the
// entry carries no client IP.
func (a *mcharborCommandAdapter) logCommandAudit(ctx context.Context, cmd ai_tools.McHarborCommand, name string) {
	if a.audit == nil {
		return
	}
	entry := audit.Entry{
		Action:        cmd.Action,
		EntityType:    "container",
		EntityID:      cmd.TargetID,
		EntityName:    name,
		EnvironmentID: cmd.EnvID,
		Details:       commandAuditDetails(cmd),
	}
	if user := auth.UserFromContext(ctx); user != nil {
		a.audit.LogWithUser(nil, user.ID, user.Username, entry)
		return
	}
	if cmd.UserID != "" {
		entry.Details += " user_id=" + cmd.UserID
	}
	a.audit.Log(nil, entry)
}

// commandAuditDetails marks the entry as AI-originated and carries the
// only two command arguments the allowlist accepts.
func commandAuditDetails(cmd ai_tools.McHarborCommand) string {
	details := "source=ai_assistant"
	if timeout := intArg(cmd.Args, "timeout_seconds"); timeout > 0 {
		details += " timeout_seconds=" + strconv.Itoa(timeout)
	}
	if signal := stringArg(cmd.Args, "signal"); signal != "" {
		details += " signal=" + signal
	}
	return details
}

func intArg(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// --- images --------------------------------------------------------------

type imageAdapter struct {
	svc *images.Service
}

func (a *imageAdapter) List(ctx context.Context, envID string) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("images service not wired")
	}
	items, err := a.svc.List(ctx, envID, true)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, im := range items {
		out = append(out, map[string]any{
			"id":       im.ID,
			"repoTags": im.RepoTags,
			"size":     im.Size,
			"created":  im.Created,
		})
	}
	return out, nil
}

func (a *imageAdapter) Inspect(ctx context.Context, envID, id string) (map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("images service not wired")
	}
	v, err := a.svc.Inspect(ctx, envID, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":      v.ID,
		"tags":    v.RepoTags,
		"created": v.Created,
		"size":    v.Size,
	}
	if v.Architecture != "" {
		out["architecture"] = v.Architecture
	}
	if v.Os != "" {
		out["os"] = v.Os
	}
	return out, nil
}

// --- volumes / networks / stacks ----------------------------------------

type volumeAdapter struct {
	svc *volumes.Service
}

func (a *volumeAdapter) List(ctx context.Context, envID string) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("volumes service not wired")
	}
	items, err := a.svc.List(ctx, envID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, v := range items {
		row := map[string]any{
			"name":       v.Name,
			"driver":     v.Driver,
			"mountpoint": v.Mountpoint,
			"createdAt":  v.CreatedAt,
			"labels":     v.Labels,
			"scope":      v.Scope,
			"refCount":   v.RefCount,
			"protected":  v.Protected,
		}
		if v.UsageData != nil {
			row["usageData"] = v.UsageData
		}
		out = append(out, row)
	}
	return out, nil
}

type networkAdapter struct {
	svc *networks.Service
}

func (a *networkAdapter) List(ctx context.Context, envID string) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("networks service not wired")
	}
	items, err := a.svc.List(ctx, envID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, n := range items {
		out = append(out, map[string]any{
			"id":         n.ID,
			"name":       n.Name,
			"driver":     n.Driver,
			"scope":      n.Scope,
			"internal":   n.Internal,
			"attachable": n.Attachable,
			"containers": n.Containers,
			"created":    n.Created,
			"builtIn":    n.BuiltIn,
			"labels":     n.Labels,
		})
	}
	return out, nil
}

type stackAdapter struct {
	svc *stacks.Service
}

func (a *stackAdapter) List(ctx context.Context, envID string) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("stacks service not wired")
	}
	items, err := a.svc.List(ctx, envID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, s := range items {
		svcCount := len(s.Services)
		containers := 0
		for _, svc := range s.Services {
			if svc.ContainerID != "" {
				containers++
			}
		}
		row := map[string]any{
			"id":             s.ID,
			"name":           s.Name,
			"status":         s.Status,
			"type":           s.Type,
			"createdAt":      s.CreatedAt,
			"updatedAt":      s.UpdatedAt,
			"protected":      s.Protected,
			"serviceCount":   svcCount,
			"containerCount": containers,
		}
		if s.EnvironmentID != nil {
			row["environmentId"] = *s.EnvironmentID
		}
		if s.Description != nil {
			row["description"] = *s.Description
		}
		out = append(out, row)
	}
	return out, nil
}

// --- environments -------------------------------------------------------

type environmentAdapter struct {
	svc *environments.Service
}

func (a *environmentAdapter) List(ctx context.Context) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("environments service not wired")
	}
	items, err := a.svc.ListContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		out = append(out, map[string]any{
			"id":             e.ID,
			"name":           e.Name,
			"connectionType": e.ConnectionType,
			"isDefault":      e.IsDefault,
			"isActive":       e.IsActive,
		})
	}
	return out, nil
}

// --- users / audit / workflows -------------------------------------------

type userAdapter struct {
	svc *users.Service
}

func (a *userAdapter) List(ctx context.Context) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("users service not wired")
	}
	items, _, err := a.svc.List(ctx, 1, 200)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, u := range items {
		out = append(out, map[string]any{
			"id":          u.ID,
			"username":    u.Username,
			"displayName": u.DisplayName,
			"email":       u.Email,
			"role":        u.Role,
			"isActive":    u.IsActive,
			"lastLogin":   u.LastLogin,
		})
	}
	return out, nil
}

type auditAdapter struct {
	db *sql.DB
}

func (a *auditAdapter) Search(ctx context.Context, userID, query string, limit int) ([]map[string]any, error) {
	if a.db == nil {
		return nil, fmt.Errorf("audit db not wired")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	_ = userID // Phase 7 narrows results to what the user can see.
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, action, entity_type, entity_id, entity_name, details, timestamp
		FROM audit_log
		WHERE action LIKE ? OR entity_type LIKE ? OR entity_name LIKE ? OR details LIKE ?
		ORDER BY timestamp DESC
		LIMIT ?`,
		"%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var (
			id, action, ts                            string
			entityType, entityID, entityName, details sql.NullString
		)
		if err := rows.Scan(&id, &action, &entityType, &entityID, &entityName, &details, &ts); err != nil {
			return nil, err
		}
		row := map[string]any{
			"id":        id,
			"action":    action,
			"timestamp": ts,
		}
		if entityType.Valid {
			row["entityType"] = entityType.String
		}
		if entityID.Valid {
			row["entityId"] = entityID.String
		}
		if entityName.Valid {
			row["entityName"] = entityName.String
		}
		if details.Valid {
			row["details"] = details.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type workflowAdapter struct {
	svc *workflows.Service
}

func (a *workflowAdapter) List(ctx context.Context) ([]map[string]any, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("workflows service not wired")
	}
	items, _, err := a.svc.List(ctx, "", 1, 200)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, w := range items {
		out = append(out, map[string]any{
			"id":          w.ID,
			"name":        w.Name,
			"description": w.Description,
			"status":      w.Status,
			"updatedAt":   w.UpdatedAt,
			"lastRunAt":   w.LastRunAt,
		})
	}
	return out, nil
}

// AIToolServices collects the module services the AI tool adapters are
// built from. Fields left nil are tolerated: the matching tools report
// "service not wired" at call time instead of failing startup.
type AIToolServices struct {
	Containers   *containers.Service
	Images       *images.Service
	Volumes      *volumes.Service
	Networks     *networks.Service
	Stacks       *stacks.Service
	Environments *environments.Service
	Users        *users.Service
	Workflows    *workflows.Service
	Alerts       *alerts.Service
	Updates      *updates.Service
	Schedules    *schedules.Service
}

// NewAIToolBundle adapts the module services to the capability set
// ai_assistant.WireTools installs into the AI tool registry.
func NewAIToolBundle(db *sql.DB, auditLog *audit.Logger, svcs AIToolServices) *aiassistant.ServiceBundle {
	containerAdapter := &containerListAdapter{svc: svcs.Containers}
	return &aiassistant.ServiceBundle{
		ContainerList:    containerAdapter,
		ContainerExec:    containerAdapter,
		ContainerInspect: containerAdapter,
		ImageList:        &imageAdapter{svc: svcs.Images},
		VolumeList:       &volumeAdapter{svc: svcs.Volumes},
		NetworkList:      &networkAdapter{svc: svcs.Networks},
		StackList:        &stackAdapter{svc: svcs.Stacks},
		EnvironmentList:  &environmentAdapter{svc: svcs.Environments},
		UserList:         &userAdapter{svc: svcs.Users},
		AuditSearch:      &auditAdapter{db: db},
		WorkflowList:     &workflowAdapter{svc: svcs.Workflows},
		CommandExecutor: &mcharborCommandAdapter{
			containers: svcs.Containers,
			audit:      auditLog,
		},
		AlertCatalog:       &alertCatalogAdapter{},
		WorkflowAuthor:     &workflowAuthorAdapter{svc: svcs.Workflows},
		AlertAuthor:        &alertAuthorAdapter{svc: svcs.Alerts},
		UpdatePolicyAuthor: &updatePolicyAuthorAdapter{svc: svcs.Updates},
		ScheduleAuthor:     &scheduleAuthorAdapter{svc: svcs.Schedules},
	}
}
