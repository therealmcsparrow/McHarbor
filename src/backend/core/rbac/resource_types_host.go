// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// tableType describes a resource kept in one table; the columns are fixed per type
// and never come from a request, so the query text is not user-influenced.
type tableType struct {
	info             ResourceTypeInfo
	table, key, name string
	state, env       string
	owner            string
}

var hostTableTypes = []tableType{
	{info: ResourceTypeInfo{Type: "stack", Label: "Stack", Category: "stacks", Routes: []ResourceRoute{{"/api/stacks/{name}", "name"}}},
		table: "stacks", key: "name", name: "name", state: "status", env: "environment_id", owner: "owner_user_id"},
	{info: ResourceTypeInfo{Type: "environment", Label: "Environment", Category: "environments", Routes: []ResourceRoute{{"/api/environments/{id}", "id"}},
		Attributes: []ResourceAttribute{{"resource.attrs.connectionType", "string", "socket, tcp, ssh or agent"}}},
		table: "environments", key: "id", name: "name", state: "agent_status"},
	{info: ResourceTypeInfo{Type: "workflow", Label: "Workflow", Category: "workflows", Routes: []ResourceRoute{{"/api/workflows/{id}", "id"}}},
		table: "workflows", key: "id", name: "name", state: "status", owner: "created_by"},
	{info: ResourceTypeInfo{Type: "secret", Label: "Secret", Category: "secrets", Routes: []ResourceRoute{{"/api/secrets/{id}", "id"}}},
		table: "secrets", key: "id", name: "name", env: "environment_id", owner: "created_by"},
	{info: ResourceTypeInfo{Type: "schedule", Label: "Schedule", Category: "schedules", Routes: []ResourceRoute{{"/api/schedules/{id}", "id"}}},
		table: "schedules", key: "id", name: "name", env: "env_id"},
	{info: ResourceTypeInfo{Type: "alert", Label: "Alert rule", Category: "alerts", Routes: []ResourceRoute{{"/api/alerts/{id}", "id"}}},
		table: "alerts", key: "id", name: "name", state: "severity"},
	{info: ResourceTypeInfo{Type: "registry", Label: "Registry", Category: "registries", Routes: []ResourceRoute{{"/api/registries/{id}", "id"}}},
		table: "registries", key: "id", name: "name"},
	{info: ResourceTypeInfo{Type: "storage_location", Label: "Storage location", Category: "storage", Routes: []ResourceRoute{{"/api/storage-locations/{id}", "id"}}},
		table: "storage_locations", key: "id", name: "name"},
	{info: ResourceTypeInfo{Type: "backup_plan", Label: "Container backup plan", Category: "backups", Routes: []ResourceRoute{{"/api/container-backups/{planId}", "planId"}}},
		table: "container_backup_plans", key: "id", name: "name", env: "environment_id", owner: "owner_user_id"},
	{info: ResourceTypeInfo{Type: "git_repo", Label: "Git repository", Category: "git", Routes: []ResourceRoute{{"/api/git/{id}", "id"}}},
		table: "git_repos", key: "id", name: "name", env: "env_id"},
	{info: ResourceTypeInfo{Type: "desired_state", Label: "Desired state", Category: "reconciler", Routes: []ResourceRoute{{"/api/reconciler/{id}", "id"}}},
		table: "desired_states", key: "id", name: "name", state: "mode", env: "env_id"},
	{info: ResourceTypeInfo{Type: "incident", Label: "Incident", Category: "incidents", Routes: []ResourceRoute{{"/api/incidents/{id}", "id"}}},
		table: "incidents", key: "id", name: "subject_name", state: "status", env: "env_id"},
	{info: ResourceTypeInfo{Type: "communication_channel", Label: "Communication channel", Category: "communications", Routes: []ResourceRoute{{"/api/communication-channels/{id}", "id"}}},
		table: "communication_channels", key: "id", name: "name"},
	{info: ResourceTypeInfo{Type: "webhook", Label: "Webhook", Category: "webhooks", Routes: []ResourceRoute{{"/api/webhooks/{id}", "id"}}},
		table: "webhooks", key: "id", name: "name", env: "environment_id"},
	{info: ResourceTypeInfo{Type: "user", Label: "User", Category: "users", Routes: []ResourceRoute{{"/api/users/{id}", "id"}}},
		table: "users", key: "id", name: "username", state: "role"},
	{info: ResourceTypeInfo{Type: "group", Label: "Group", Category: "groups", Routes: []ResourceRoute{{"/api/groups/{id}", "id"}}},
		table: "groups", key: "id", name: "name"},
	{info: ResourceTypeInfo{Type: "role", Label: "Role", Category: "roles", Routes: []ResourceRoute{{"/api/roles/{id}", "id"}}},
		table: "roles", key: "id", name: "name"},
	{info: ResourceTypeInfo{Type: "api_key", Label: "API key", Category: "api_keys", Routes: []ResourceRoute{{"/api/api-keys/{id}", "id"}}},
		table: "api_keys", key: "id", name: "name", owner: "user_id"},
	{info: ResourceTypeInfo{Type: "identity_provider", Label: "Identity provider", Category: "identity", Routes: []ResourceRoute{{"/api/identity-providers/{id}", "id"}}},
		table: "identity_providers", key: "id", name: "name", state: "provider_type"},
}

func (t tableType) resolver(db *sql.DB) ResourceResolver {
	col := func(c string) string {
		if c == "" {
			return "''"
		}
		return "COALESCE(CAST(" + c + " AS TEXT), '')"
	}
	query := fmt.Sprintf("SELECT %s, %s, %s, %s FROM %s WHERE %s = ? LIMIT 1", // SQL identifier: table and columns come from the static tableTypes literal
		col(t.name), col(t.state), col(t.env), col(t.owner), t.table, t.key)
	typ := t.info.Type
	return func(ctx context.Context, envID, id string) (*ResourceAttributes, error) {
		ctx, cancel := context.WithTimeout(ctx, authzQueryTimeout)
		defer cancel()
		out := &ResourceAttributes{Type: typ, ID: id, Labels: map[string]string{}, Attrs: map[string]any{}}
		err := db.QueryRowContext(ctx, query, id).Scan(&out.Name, &out.State, &out.Environment, &out.Owner)
		if errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("abac: resolving %s: %w", typ, err)
		}
		if out.Environment == "" {
			out.Environment = envID
		}
		return out, nil
	}
}

// RegisterHostResourceTypes registers every host-owned resource type with its routes
// and resolver; plugin types arrive later through the plugin runtime.
func RegisterHostResourceTypes(db *sql.DB, pool *docker.ClientPool, logger *slog.Logger) {
	container := NewContainerAttributeProvider(pool, logger)
	RegisterResourceType(ResourceTypeInfo{Type: "container", Label: "Container", Category: "containers",
		Routes:     []ResourceRoute{{"/api/containers/{id}", "id"}},
		Attributes: []ResourceAttribute{{"resource.image", "string", "Image reference the container runs"}}},
		func(ctx context.Context, envID, id string) (*ResourceAttributes, error) {
			return container.Resolve(ctx, envID, id)
		})
	for _, d := range []struct{ typ, label, category, prefix, param string }{
		{"image", "Image", "images", "/api/images/{id}", "id"},
		{"volume", "Volume", "volumes", "/api/volumes/{name}", "name"},
		{"network", "Network", "networks", "/api/networks/{id}", "id"},
	} {
		RegisterResourceType(ResourceTypeInfo{Type: d.typ, Label: d.label, Category: d.category,
			Routes: []ResourceRoute{{d.prefix, d.param}}}, dockerResolver(pool, d.typ))
	}
	for _, t := range hostTableTypes {
		RegisterResourceType(t.info, t.resolver(db))
	}
}

func dockerResolver(pool *docker.ClientPool, typ string) ResourceResolver {
	return func(ctx context.Context, envID, id string) (*ResourceAttributes, error) {
		out := &ResourceAttributes{Type: typ, ID: id, Name: id, Environment: envID, Labels: map[string]string{}, Attrs: map[string]any{}}
		if pool == nil {
			return out, nil
		}
		cli, err := pool.Get(envID) //nolint:contextcheck // ClientPool.Get takes no context and lives in core/docker, outside this package
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		switch typ {
		case "image":
			img, err := cli.ImageInspect(ctx, id)
			if err != nil {
				return nil, err
			}
			if len(img.RepoTags) > 0 {
				out.Name = img.RepoTags[0]
			}
			if img.Config != nil {
				out.Labels = copyLabels(img.Config.Labels)
			}
		case "volume":
			volRes, err := cli.VolumeInspect(ctx, id, client.VolumeInspectOptions{})
			if err != nil {
				return nil, err
			}
			vol := volRes.Volume
			out.Name, out.Labels = vol.Name, copyLabels(vol.Labels)
		case "network":
			netRes, err := cli.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
			if err != nil {
				return nil, err
			}
			net := netRes.Network
			out.Name, out.Labels = net.Name, copyLabels(net.Labels)
			out.Attrs["driver"] = net.Driver
		}
		return out, nil
	}
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[strings.TrimSpace(k)] = v
	}
	return out
}
