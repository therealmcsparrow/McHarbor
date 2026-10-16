// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// containerLister is the subset of the containers service the AI
// tools use. Defined here as an interface so the tools package stays
// free of heavy Docker / module imports; the real *containers.Service
// satisfies it implicitly.
type containerLister interface {
	List(ctx context.Context, envID string) ([]map[string]any, error)
}

// containerInspecter returns full docker inspect JSON for one container.
type containerInspecter interface {
	Inspect(ctx context.Context, envID, containerID string) (map[string]any, error)
}

// containerLogReader returns the last N log lines for a container.
type containerLogReader interface {
	Logs(ctx context.Context, envID, containerID string, tail int, since, until time.Time) ([]string, error)
}

func init() {
	Register(&Tool{
		Name:        "containers.list",
		Description: "List all containers (running, stopped, paused) on an environment. Returns name, image, state, status, ports, labels.",
		InputSchema: objectSchema(map[string]any{
			"env_id": stringSchema("Environment ID. Empty = current default environment."),
		}, []string{}),
		Required: []rbac.Permission{rbac.PermContainersView},
		ReadOnly: true,
		MaxBytes: 64 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				EnvID string `json:"env_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				slog.Debug("ai tool: unreadable optional arguments, using defaults", "error", err)
			}
			lister, err := resolveContainerLister()
			if err != nil {
				return nil, err
			}
			envID := p.EnvID
			if envID == "" {
				envID = sc.EnvID
			}
			out, err := lister.List(ctx, envID)
			if err != nil {
				return nil, fmt.Errorf("list containers: %w", err)
			}
			return map[string]any{
				"envId":   envID,
				"count":   len(out),
				"running": countByState(out, "running"),
				"stopped": countByState(out, "exited"),
				"items":   out,
			}, nil
		},
	})

	Register(&Tool{
		Name:        "containers.inspect",
		Description: "Return the full docker inspect JSON for one container. Output is large; prefer containers.list first.",
		InputSchema: objectSchema(map[string]any{
			"env_id":       stringSchema("Environment ID."),
			"container_id": stringSchema("Container ID or name."),
		}, []string{"container_id"}),
		Required: []rbac.Permission{rbac.PermContainersView},
		ReadOnly: true,
		MaxBytes: 128 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				EnvID       string `json:"env_id"`
				ContainerID string `json:"container_id"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			if p.ContainerID == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "container_id is required"}
			}
			ins, err := resolveContainerInspecter()
			if err != nil {
				return nil, err
			}
			envID := p.EnvID
			if envID == "" {
				envID = sc.EnvID
			}
			data, err := ins.Inspect(ctx, envID, p.ContainerID)
			if err != nil {
				return nil, fmt.Errorf("inspect container: %w", err)
			}
			return map[string]any{
				"envId":       envID,
				"containerId": p.ContainerID,
				"data":        data,
			}, nil
		},
	})

	Register(&Tool{
		Name:        "containers.logs",
		Description: "Tail the last N lines of a container's logs. Use a small tail (50-200) for diagnostics.",
		InputSchema: objectSchema(map[string]any{
			"env_id":       stringSchema("Environment ID."),
			"container_id": stringSchema("Container ID or name."),
			"tail":         intSchema("Number of lines to return. Default 100, max 5000."),
		}, []string{"container_id"}),
		Required: []rbac.Permission{rbac.PermContainersView, rbac.PermLogsView},
		ReadOnly: true,
		MaxBytes: 256 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				EnvID       string `json:"env_id"`
				ContainerID string `json:"container_id"`
				Tail        int    `json:"tail"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			if p.ContainerID == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "container_id is required"}
			}
			tail := p.Tail
			if tail <= 0 {
				tail = 100
			}
			if tail > 5000 {
				tail = 5000
			}
			rdr, err := resolveContainerLogReader()
			if err != nil {
				return nil, err
			}
			envID := p.EnvID
			if envID == "" {
				envID = sc.EnvID
			}
			lines, err := rdr.Logs(ctx, envID, p.ContainerID, tail, time.Time{}, time.Time{})
			if err != nil {
				return nil, fmt.Errorf("read logs: %w", err)
			}
			return map[string]any{
				"envId":       envID,
				"containerId": p.ContainerID,
				"tail":        tail,
				"lineCount":   len(lines),
				"lines":       lines,
			}, nil
		},
	})

	Register(&Tool{
		Name:        "containers.stats",
		Description: "Return CPU/memory/network/block I/O stats snapshot for one container.",
		InputSchema: objectSchema(map[string]any{
			"env_id":       stringSchema("Environment ID."),
			"container_id": stringSchema("Container ID or name."),
		}, []string{"container_id"}),
		Required: []rbac.Permission{rbac.PermContainersView, rbac.PermMetricsView},
		ReadOnly: true,
		MaxBytes: 16 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			// Stats go through the env_provider; Phase 1 returns the
			// same summary that backs the chat context. The dedicated
			// stats service is a Phase 7 enhancement.
			deps := GetDeps()
			if deps == nil || deps.ContainerList == nil {
				return map[string]any{"message": "stats service not configured"}, nil
			}
			return map[string]any{
				"hint": "Use the containers.list summary fields (cpuPct, memPct) for quick diagnostics; full stats are available via the container detail page.",
			}, nil
		},
	})
}

// --- resolvers: pull concrete services out of the Deps box ---

func resolveContainerLister() (containerLister, error) {
	d := GetDeps()
	if d == nil || d.ContainerList == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "containers service not wired"}
	}
	v, ok := d.ContainerList.(containerLister)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "containers service does not implement List"}
	}
	return v, nil
}

func resolveContainerInspecter() (containerInspecter, error) {
	d := GetDeps()
	if d == nil || d.ContainerInspect == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "containers inspect service not wired"}
	}
	v, ok := d.ContainerInspect.(containerInspecter)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "containers service does not implement Inspect"}
	}
	return v, nil
}

func resolveContainerLogReader() (containerLogReader, error) {
	d := GetDeps()
	if d == nil || d.ContainerExec == nil {
		// The LogReader interface is implemented by the containers
		// service; we reuse the same field for now.
		return nil, &CallError{Code: ErrToolDisabled, Message: "container logs service not wired"}
	}
	v, ok := d.ContainerExec.(containerLogReader)
	if !ok {
		return nil, &CallError{Code: ErrToolDisabled, Message: "containers service does not implement Logs"}
	}
	return v, nil
}

func countByState(items []map[string]any, state string) int {
	n := 0
	for _, it := range items {
		if s, _ := it["State"].(string); strings.EqualFold(s, state) {
			n++
		}
	}
	return n
}

// --- JSON Schema helpers ---

func objectSchema(props map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func intSchema(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}
