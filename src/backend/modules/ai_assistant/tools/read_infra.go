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

type volumeLister interface {
	List(ctx context.Context, envID string) ([]map[string]any, error)
}

type networkLister interface {
	List(ctx context.Context, envID string) ([]map[string]any, error)
}

type stackLister interface {
	List(ctx context.Context, envID string) ([]map[string]any, error)
}

func init() {
	Register(&Tool{
		Name:        "volumes.list",
		Description: "List all named volumes on an environment.",
		InputSchema: objectSchema(map[string]any{
			"env_id": stringSchema("Environment ID."),
		}, []string{}),
		Required: []rbac.Permission{rbac.PermVolumesView},
		ReadOnly: true,
		MaxBytes: 32 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			envID, err := envFromArgs(args, sc)
			if err != nil {
				return nil, err
			}
			d := GetDeps()
			if d == nil || d.VolumeList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "volumes service not wired"}
			}
			lister, ok := d.VolumeList.(volumeLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "volumes service does not implement List"}
			}
			out, err := lister.List(ctx, envID)
			if err != nil {
				return nil, fmt.Errorf("list volumes: %w", err)
			}
			return map[string]any{"envId": envID, "count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "networks.list",
		Description: "List all networks on an environment.",
		InputSchema: objectSchema(map[string]any{
			"env_id": stringSchema("Environment ID."),
		}, []string{}),
		Required: []rbac.Permission{rbac.PermNetworksView},
		ReadOnly: true,
		MaxBytes: 32 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			envID, err := envFromArgs(args, sc)
			if err != nil {
				return nil, err
			}
			d := GetDeps()
			if d == nil || d.NetworkList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "networks service not wired"}
			}
			lister, ok := d.NetworkList.(networkLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "networks service does not implement List"}
			}
			out, err := lister.List(ctx, envID)
			if err != nil {
				return nil, fmt.Errorf("list networks: %w", err)
			}
			return map[string]any{"envId": envID, "count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "stacks.list",
		Description: "List all Compose stacks on an environment with status and deployed containers.",
		InputSchema: objectSchema(map[string]any{
			"env_id": stringSchema("Environment ID."),
		}, []string{}),
		Required: []rbac.Permission{rbac.PermStacksView},
		ReadOnly: true,
		MaxBytes: 32 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			envID, err := envFromArgs(args, sc)
			if err != nil {
				return nil, err
			}
			d := GetDeps()
			if d == nil || d.StackList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "stacks service not wired"}
			}
			lister, ok := d.StackList.(stackLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "stacks service does not implement List"}
			}
			out, err := lister.List(ctx, envID)
			if err != nil {
				return nil, fmt.Errorf("list stacks: %w", err)
			}
			return map[string]any{"envId": envID, "count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "environments.list",
		Description: "List all Docker and agent environments the assistant can see.",
		InputSchema: objectSchema(map[string]any{}, []string{}),
		Required:    []rbac.Permission{rbac.PermEnvironmentsView},
		ReadOnly:    true,
		MaxBytes:    32 * 1024,
		Handler: func(ctx context.Context, sc Scope, _ json.RawMessage) (any, error) {
			d := GetDeps()
			if d == nil || d.EnvironmentList == nil {
				return nil, &CallError{Code: ErrToolDisabled, Message: "environments service not wired"}
			}
			lister, ok := d.EnvironmentList.(environmentLister)
			if !ok {
				return nil, &CallError{Code: ErrToolDisabled, Message: "environments service does not implement List"}
			}
			out, err := lister.List(ctx)
			if err != nil {
				return nil, fmt.Errorf("list environments: %w", err)
			}
			return map[string]any{"count": len(out), "items": out}, nil
		},
	})

	Register(&Tool{
		Name:        "system.info",
		Description: "Return McHarbor version, build, hostname, uptime. Read-only.",
		InputSchema: objectSchema(map[string]any{}, []string{}),
		Required:    []rbac.Permission{},
		ReadOnly:    true,
		MaxBytes:    4 * 1024,
		Handler: func(_ context.Context, _ Scope, _ json.RawMessage) (any, error) {
			// Phase 1 returns a static snapshot. The bootstrap will
			// inject the real values via Deps.SystemInfo in Phase 2.
			return map[string]any{
				"version":  "see-version-file",
				"hostname": "see-sysinfo",
				"note":     "system.info is stubbed in Phase 1; full values land in Phase 2.",
			}, nil
		},
	})
}

type environmentLister interface {
	List(ctx context.Context) ([]map[string]any, error)
}

func envFromArgs(args json.RawMessage, sc Scope) (string, error) {
	var p struct {
		EnvID string `json:"env_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		slog.Debug("ai tool: unreadable optional arguments, using defaults", "error", err)
	}
	if p.EnvID != "" {
		return p.EnvID, nil
	}
	if sc.EnvID != "" {
		return sc.EnvID, nil
	}
	// Empty envID is tolerated. The underlying service resolves a
	// default environment, or the tool surfaces a clearer error.
	return "", nil
}
