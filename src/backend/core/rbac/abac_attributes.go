// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package rbac holds the permission catalog and the checks that decide whether a caller may act on a resource.
package rbac

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/docker"

	"github.com/moby/moby/client"
)

// AttributeProvider is the interface each resource type implements to
// resolve its attributes for an ABAC evaluation. The provider runs only
// when the candidate policy has a non-empty resource_type that matches.
//
// Providers are best-effort: if they fail to fetch (e.g. Docker daemon
// offline), we return an empty attributes map and let the policy fall
// through to the role-based check. The audit log records the failure.
type AttributeProvider interface {
	// ResourceType returns the matching key (e.g. "container").
	ResourceType() string
	// Resolve returns the attributes for the named resource in the
	// given environment. Errors are logged but never fatal.
	Resolve(ctx context.Context, envID, resourceID string) (*ResourceAttributes, error)
}

// ResourceAttributesResolver is the optional hook attached to a
// Require* middleware. It runs before the Authorize() call and
// returns the resource attributes so the policies can evaluate them.
// Implementations that return (nil, nil) skip the ABAC step.
type ResourceAttributesResolver func(ctx context.Context, req AuthzRequest) (*ResourceAttributes, error)

// AttributeBundle is the registry of attribute providers built at
// startup. It is set once on the rbac.Service via SetAttributeBundle.
type AttributeBundle struct {
	providers map[string]AttributeProvider
	auditLog  *audit.Logger
	logger    *slog.Logger
}

// NewAttributeBundle builds an empty registry.
func NewAttributeBundle(logger *slog.Logger, auditLog *audit.Logger) *AttributeBundle {
	return &AttributeBundle{
		providers: make(map[string]AttributeProvider),
		auditLog:  auditLog,
		logger:    logger,
	}
}

// Register adds a provider. Overwrites duplicates.
func (b *AttributeBundle) Register(p AttributeProvider) {
	if p == nil {
		return
	}
	b.providers[p.ResourceType()] = p
}

// Resolve calls the matching provider for the resource type, if any.
// Returns nil when no provider matches — the caller should skip ABAC.
func (b *AttributeBundle) Resolve(ctx context.Context, req AuthzRequest) *ResourceAttributes {
	if req.ResourceType == "" || req.ResourceID == "" {
		return nil
	}
	p, ok := b.providers[req.ResourceType]
	if !ok {
		return b.resolveRegistered(ctx, req)
	}
	attrs, err := p.Resolve(ctx, req.EnvironmentID, req.ResourceID)
	if err != nil {
		if b.logger != nil {
			b.logger.Warn("abac: attribute provider failed",
				"resource_type", req.ResourceType,
				"resource_id", req.ResourceID,
				"error", err)
		}
		return nil
	}
	return attrs
}

// --- UserAttributes resolver --------------------------------------------

// FetchUserAttributes loads the calling user's roles, groups, and
// free-form labels for ABAC evaluation. Cached read: one query per
// call. The caller (Authorize) is allowed to skip this when the
// system user is bypassed.
func FetchUserAttributes(ctx context.Context, db *sql.DB, userID string) (*UserAttributes, error) {
	if db == nil {
		return nil, fmt.Errorf("nil db")
	}
	var u UserAttributes
	err := db.QueryRowContext(ctx, `
		SELECT id, username, COALESCE(email, '')
		  FROM users
		 WHERE id = ?`, userID,
	).Scan(&u.ID, &u.Username, &u.Email)
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT r.name FROM roles r
		  INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?
		 UNION
		SELECT r.name FROM roles r
		  INNER JOIN group_roles gr ON gr.role_id = r.id
		  INNER JOIN group_members gm ON gm.group_id = gr.group_id
		 WHERE gm.user_id = ?`,
		userID, userID)
	if err != nil {
		return nil, fmt.Errorf("loading user roles: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		u.Roles = append(u.Roles, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	gRows, err := db.QueryContext(ctx, `
		SELECT g.name FROM groups g
		  INNER JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("loading user groups: %w", err)
	}
	defer closeRows(gRows)
	for gRows.Next() {
		var name string
		if err := gRows.Scan(&name); err != nil {
			return nil, err
		}
		u.Groups = append(u.Groups, name)
	}
	if err := gRows.Err(); err != nil {
		return nil, err
	}

	// User labels: a free-form key/value bag used by seed policies.
	// Stored as a JSON-ish "k=v" list in the settings table under
	// "user_labels:{userID}". The schema is intentionally simple:
	// admins can update via the user profile page.
	u.Labels = loadUserLabels(ctx, db, userID)

	return &u, nil
}

func loadUserLabels(ctx context.Context, db *sql.DB, userID string) map[string]string {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value FROM user_labels WHERE user_id = ?`, userID)
	if err != nil {
		return map[string]string{}
	}
	defer closeRows(rows)
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return map[string]string{}
	}
	return out
}

// --- Container attribute provider ---------------------------------------

// ContainerAttributeProvider resolves a container's labels from the
// Docker daemon. Uses the ClientPool so it works for both local Docker
// and remote agents.
type ContainerAttributeProvider struct {
	pool   *docker.ClientPool
	logger *slog.Logger
}

// NewContainerAttributeProvider builds the provider.
func NewContainerAttributeProvider(pool *docker.ClientPool, logger *slog.Logger) *ContainerAttributeProvider {
	return &ContainerAttributeProvider{pool: pool, logger: logger}
}

// ResourceType returns "container".
func (p *ContainerAttributeProvider) ResourceType() string { return "container" }

// Resolve fetches a single container's labels and summary. It uses
// ContainerInspect (not the cheaper list path) because the seed policies
// query labels that may not be surfaced on ContainerSummary in older
// Docker versions.
func (p *ContainerAttributeProvider) Resolve(ctx context.Context, envID, resourceID string) (*ResourceAttributes, error) {
	if p.pool == nil {
		return nil, fmt.Errorf("nil docker pool")
	}
	cli, err := p.pool.Get(envID) //nolint:contextcheck // ClientPool.Get takes no context and lives in core/docker, outside this package
	if err != nil {
		return nil, fmt.Errorf("docker pool: %w", err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 30*1_000_000_000)
	defer cancel()

	infoRes, err := cli.ContainerInspect(inspectCtx, resourceID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("container inspect: %w", err)
	}
	info := infoRes.Container

	attrs := &ResourceAttributes{
		Type:   "container",
		ID:     info.ID,
		Image:  info.Config.Image,
		State:  string(info.State.Status),
		Labels: map[string]string{},
	}
	if info.Config != nil {
		for k, v := range info.Config.Labels {
			attrs.Labels[k] = v
		}
	}
	// Some labels are reserved by the engine; keep them too.
	return attrs, nil
}

// --- EnvAttributes resolver ---------------------------------------------

// FetchEnvAttributes loads the environment's name and connection type for the request.
// env.type is the connection type (socket, tcp, agent …), as PolicyVariables documents.
func FetchEnvAttributes(ctx context.Context, db *sql.DB, envID string) (*EnvAttributes, error) {
	if envID == "" {
		return &EnvAttributes{}, nil
	}
	var e EnvAttributes
	e.ID = envID
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(name, ''), COALESCE(connection_type, '')
		  FROM environments WHERE id = ?`, envID,
	).Scan(&e.Name, &e.Type)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("loading env: %w", err)
	}
	return &e, nil
}

// --- helpers ------------------------------------------------------------

// AuthzAttributes builds the full Attributes struct used by the
// executor. It looks up user, env, and resource attributes in parallel
// where possible. Errors are non-fatal — empty attributes mean the
// policy will evaluate against null.
func AuthzAttributes(ctx context.Context, db *sql.DB, bundle *AttributeBundle, user *auth.User, req AuthzRequest, now time.Time) *Attributes {
	if now.IsZero() {
		now = time.Now()
	}
	out := &Attributes{
		Now: snapshotTime(now),
	}
	if user != nil {
		u, err := FetchUserAttributes(ctx, db, user.ID)
		if err == nil {
			out.User = u
		}
	}
	if req.EnvironmentID != "" {
		e, err := FetchEnvAttributes(ctx, db, req.EnvironmentID)
		if err == nil {
			out.Env = e
		}
	}
	if bundle != nil {
		if r := bundle.Resolve(ctx, req); r != nil {
			out.Resource = r
		}
	}
	return out
}
