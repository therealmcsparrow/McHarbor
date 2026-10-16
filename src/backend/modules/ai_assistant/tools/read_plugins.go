// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

func init() {
	Register(&Tool{
		Name:        "app.capabilities",
		Description: "List AI-visible McHarbor core tools and installed plugin contribution surfaces. Use this before acting across plugins or unfamiliar app areas.",
		InputSchema: objectSchema(map[string]any{
			"include_plugins": map[string]any{"type": "boolean", "description": "Include installed plugin routes/menu/widgets/nodes/forms. Default true."},
		}, []string{}),
		Required: []rbac.Permission{rbac.PermAIAssistantUse},
		ReadOnly: true,
		MaxBytes: 96 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				IncludePlugins *bool `json:"include_plugins"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				slog.Debug("ai tool: unreadable optional arguments, using defaults", "error", err)
			}
			includePlugins := true
			if p.IncludePlugins != nil {
				includePlugins = *p.IncludePlugins
			}

			tools := CatalogFor(ctx, depsAuthorizer{}, sc.UserID, sc.EnvID)
			out := map[string]any{
				"tools": map[string]any{
					"count": len(tools),
					"items": tools,
				},
				"execution": map[string]any{
					"readOnly": "read-only tools can be executed directly by /api/ai-assistant/tools/call or by the chat orchestrator",
					"mutating": "mutating or plugin route actions require /api/ai-assistant/actions/preview, user consent, then /execute",
				},
			}
			if includePlugins {
				plugins, err := pluginCapabilitySnapshot(ctx, sc.UserID, sc.EnvID)
				if err != nil {
					return nil, err
				}
				out["plugins"] = plugins
			}
			return out, nil
		},
	})

	Register(&Tool{
		Name:        "plugins.capabilities",
		Description: "List installed plugin routes, UI contributions, workflow nodes, widgets, forms, and adapter categories visible to the current user.",
		InputSchema: objectSchema(map[string]any{
			"plugin_slug": stringSchema("Optional plugin slug to filter to one plugin."),
		}, []string{}),
		Required: []rbac.Permission{rbac.PermPluginsView},
		ReadOnly: true,
		MaxBytes: 96 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				PluginSlug string `json:"plugin_slug"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				slog.Debug("ai tool: unreadable optional arguments, using defaults", "error", err)
			}
			plugins, err := pluginCapabilitySnapshot(ctx, sc.UserID, sc.EnvID)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(p.PluginSlug) == "" {
				return plugins, nil
			}
			slug := strings.TrimSpace(p.PluginSlug)
			filtered := map[string]any{"plugin_slug": slug}
			for key, value := range plugins {
				if items, ok := value.([]map[string]any); ok {
					filtered[key] = filterPluginItems(items, slug)
				}
			}
			return filtered, nil
		},
	})

	Register(&Tool{
		Name:        "plugins.route.plan",
		Description: "Prepare a plugin API route invocation plan. This validates the route exists and whether approval is required; it does not bypass plugin RBAC or execute unsafe routes directly.",
		InputSchema: objectSchema(map[string]any{
			"method": stringSchema("HTTP method declared by the plugin route, for example GET or POST."),
			"path":   stringSchema("Declared plugin route path, for example /api/my-plugin/items."),
			"body":   map[string]any{"type": "object", "description": "Optional JSON body for mutating plugin routes."},
		}, []string{"method", "path"}),
		Required:        []rbac.Permission{rbac.PermPluginsView},
		ReadOnly:        false,
		RequiresConsent: true,
		MaxBytes:        16 * 1024,
		Handler: func(ctx context.Context, sc Scope, args json.RawMessage) (any, error) {
			var p struct {
				Method string         `json:"method"`
				Path   string         `json:"path"`
				Body   map[string]any `json:"body,omitempty"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}
			method := strings.ToUpper(strings.TrimSpace(p.Method))
			path := strings.TrimSpace(p.Path)
			if method == "" || path == "" {
				return nil, &CallError{Code: ErrNotFound, Message: "method and path are required"}
			}
			route, err := findPluginRoute(ctx, method, path)
			if err != nil {
				return nil, err
			}
			if route == nil {
				return nil, &CallError{Code: ErrNotFound, Message: "plugin route not found"}
			}
			if perm, _ := route["requires_permission"].(string); perm != "" {
				if err := requireDynamicPermission(ctx, sc.UserID, sc.EnvID, perm); err != nil {
					return nil, err
				}
			}
			return map[string]any{
				"route":            route,
				"body":             p.Body,
				"approvalRequired": true,
				"execution":        "Route is declared by an installed plugin. Use the action consent flow before executing plugin-owned mutations.",
				"safety":           "Secrets are not included. Host RBAC and plugin route permissions still apply at execution time.",
			}, nil
		},
	})
}

type depsAuthorizer struct{}

func (depsAuthorizer) HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error) {
	d := GetDeps()
	if d == nil || d.RBAC == nil {
		return false, nil
	}
	return d.RBAC.HasPermission(ctx, userID, envID, perm)
}

func pluginCapabilitySnapshot(ctx context.Context, userID, envID string) (map[string]any, error) {
	d := GetDeps()
	if d == nil || d.DB == nil {
		return map[string]any{}, nil
	}
	routes, err := pluginRoutes(ctx, userID, envID)
	if err != nil {
		return nil, err
	}
	menu, err := queryPluginRows(ctx, "plugin_menu", "SELECT origin AS plugin_slug, COALESCE(parent, '') AS parent, label_ns, label_key, path, COALESCE(icon, '') AS icon, COALESCE(permission, '') AS required_permission FROM plugin_menu ORDER BY origin ASC, sort_order ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	menu = filterByDynamicPermission(ctx, menu, userID, envID)
	widgets, err := queryPluginRows(ctx, "plugin_widgets", "SELECT origin AS plugin_slug, key, title, COALESCE(category, '') AS category, default_size FROM plugin_widgets ORDER BY origin ASC, key ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	nodes, err := queryPluginRows(ctx, "plugin_nodes", "SELECT origin AS plugin_slug, key, category, display_name, COALESCE(requires_permission, '') AS required_permission FROM plugin_nodes ORDER BY origin ASC, key ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	nodes = filterByDynamicPermission(ctx, nodes, userID, envID)
	forms, err := queryPluginRows(ctx, "plugin_forms", "SELECT plugin_slug, resource, kind, form_type, component, COALESCE(priority, 0) AS priority FROM plugin_forms ORDER BY plugin_slug ASC, resource ASC, kind ASC, priority DESC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	permissions, err := queryPluginRows(ctx, "plugin_permissions", "SELECT origin AS plugin_slug, permission_key, description, scope FROM plugin_permissions ORDER BY origin ASC, permission_key ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	topbar, err := queryPluginRows(ctx, "plugin_topbar", "SELECT origin AS plugin_slug, slot, label_ns, label_key, COALESCE(icon, '') AS icon, path, visibility FROM plugin_topbar ORDER BY origin ASC, sort_order ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	contextMenus, err := queryPluginRows(ctx, "plugin_context_menus", "SELECT origin AS plugin_slug, page_id, label_ns, label_key, COALESCE(icon, '') AS icon, COALESCE(requires_permission, '') AS required_permission FROM plugin_context_menus ORDER BY origin ASC, page_id ASC, sort_order ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	contextMenus = filterByDynamicPermission(ctx, contextMenus, userID, envID)
	tabs, err := queryPluginRows(ctx, "plugin_tabs", "SELECT origin AS plugin_slug, page_id, tab_key, label_ns, label_key, COALESCE(icon, '') AS icon, COALESCE(requires_permission, '') AS required_permission FROM plugin_tabs ORDER BY origin ASC, page_id ASC, sort_order ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	tabs = filterByDynamicPermission(ctx, tabs, userID, envID)
	return map[string]any{
		"routes":       routes,
		"menu":         menu,
		"widgets":      widgets,
		"nodes":        nodes,
		"forms":        forms,
		"permissions":  permissions,
		"topbar":       topbar,
		"contextMenus": contextMenus,
		"tabs":         tabs,
		"adapters":     pluginAdapterSnapshot(),
	}, nil
}

type pluginAdapterSource interface {
	NotificationAdapterSpecs() map[string]plugin.NotificationAdapterSpec
	StorageAdapterSpecs() map[string]plugin.StorageAdapterSpec
	AIAdapterSpecs() map[string]plugin.AIAdapterSpec
	IdentityAdapterSpecs() map[string]plugin.IdentityAdapterSpec
}

func pluginAdapterSnapshot() map[string]any {
	d := GetDeps()
	if d == nil || d.PluginAdapters == nil {
		return map[string]any{}
	}
	src, ok := d.PluginAdapters.(pluginAdapterSource)
	if !ok {
		return map[string]any{}
	}
	return map[string]any{
		"notification": src.NotificationAdapterSpecs(),
		"storage":      src.StorageAdapterSpecs(),
		"ai":           src.AIAdapterSpecs(),
		"identity":     src.IdentityAdapterSpecs(),
	}
}

func pluginRoutes(ctx context.Context, userID, envID string) ([]map[string]any, error) {
	rows, err := queryPluginRows(ctx, "plugin_routes", "SELECT origin AS plugin_slug, method, path, COALESCE(requires_permission, '') AS requires_permission, is_public FROM plugin_routes ORDER BY origin ASC, path ASC LIMIT 1000")
	if err != nil {
		return nil, err
	}
	filtered := rows[:0]
	for _, row := range rows {
		perm, _ := row["requires_permission"].(string)
		if perm == "" {
			filtered = append(filtered, row)
			continue
		}
		if requireDynamicPermission(ctx, userID, envID, perm) == nil {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func findPluginRoute(ctx context.Context, method, path string) (map[string]any, error) {
	d := GetDeps()
	if d == nil || d.DB == nil {
		return nil, &CallError{Code: ErrToolDisabled, Message: "plugin route registry not wired"}
	}
	var origin, foundMethod, foundPath, permission string
	var public int
	err := d.DB.QueryRowContext(ctx, `
		SELECT origin, method, path, COALESCE(requires_permission, ''), is_public
		  FROM plugin_routes
		 WHERE UPPER(method) = ? AND path = ?
		 LIMIT 1`, method, path).Scan(&origin, &foundMethod, &foundPath, &permission, &public)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("finding plugin route: %w", err)
	}
	return map[string]any{
		"plugin_slug":         origin,
		"method":              foundMethod,
		"path":                foundPath,
		"requires_permission": permission,
		"public":              public == 1,
	}, nil
}

func queryPluginRows(ctx context.Context, table, query string) ([]map[string]any, error) {
	d := GetDeps()
	if d == nil || d.DB == nil {
		return []map[string]any{}, nil
	}
	if !tableExists(ctx, d.DB, table) {
		return []map[string]any{}, nil
	}
	rows, err := d.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", table, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading %s columns: %w", table, err)
	}
	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", table, err)
		}
		item := make(map[string]any, len(cols))
		for i, col := range cols {
			switch v := values[i].(type) {
			case []byte:
				item[col] = string(v)
			case int64:
				if strings.HasPrefix(col, "is_") {
					item[col] = v == 1
				} else {
					item[col] = v
				}
			default:
				item[col] = v
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func tableExists(ctx context.Context, db *sql.DB, table string) bool {
	var name string
	err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1", table).Scan(&name)
	return err == nil && name == table
}

func filterPluginItems(items []map[string]any, slug string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if itemSlug, _ := item["plugin_slug"].(string); itemSlug == slug {
			out = append(out, item)
		}
	}
	return out
}

func filterByDynamicPermission(ctx context.Context, items []map[string]any, userID, envID string) []map[string]any {
	filtered := items[:0]
	for _, item := range items {
		perm, _ := item["required_permission"].(string)
		if perm == "" || requireDynamicPermission(ctx, userID, envID, perm) == nil {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func requireDynamicPermission(ctx context.Context, userID, envID, perm string) error {
	d := GetDeps()
	if d == nil || d.RBAC == nil {
		return &CallError{Code: ErrForbidden, Message: "rbac service not wired"}
	}
	allowed, err := d.RBAC.HasPermission(ctx, userID, envID, rbac.Permission(perm))
	if err != nil {
		slog.Error("ai tool: rbac check failed", "user", userID, "permission", perm, "error", err)
		return &CallError{Code: ErrUnauthorized, Message: "rbac check failed"}
	}
	if !allowed {
		return &CallError{Code: ErrForbidden, Message: "missing required permission", Details: perm}
	}
	return nil
}
