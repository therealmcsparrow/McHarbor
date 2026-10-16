// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"strings"
)

// ABACPolicy is an admin-authored rule evaluated by the goja sandbox.
// A policy is "active" only when its enabled flag is true AND at least
// one binding row references it. The Expression is a JavaScript snippet
// that returns truthy to match the request.
type ABACPolicy struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Effect       string   `json:"effect"` // "allow" | "deny"
	ResourceType string   `json:"resourceType"`
	Expression   string   `json:"expression"`
	Priority     int      `json:"priority"`
	Enabled      bool     `json:"enabled"`
	Permissions  []string `json:"permissions"`
	CreatedBy    string   `json:"createdBy"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// PolicyService is the CRUD layer for ABAC policies. It is constructed
// at startup and mounted via /api/rbac/policies by the admin handler.
type PolicyService struct {
	db *sql.DB
}

// NewPolicyService builds a PolicyService.
func NewPolicyService(db *sql.DB) *PolicyService {
	return &PolicyService{db: db}
}

// ErrPolicyNotFound is returned when a requested policy id does not exist.
var ErrPolicyNotFound = errors.New("rbac policy not found")

// List returns all policies, sorted by priority then name.
//
// Each returned policy has its Permissions slice populated so the JSON
// shape matches Get(). The caller controls access via the
// /api/rbac/policies middleware; this method does no auth.
func (s *PolicyService) List(ctx context.Context) ([]ABACPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, effect, resource_type, expression,
		       priority, enabled, COALESCE(created_by, ''), created_at, updated_at
		  FROM rbac_policies
		 ORDER BY priority ASC, name ASC
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing rbac policies: %w", err)
	}
	defer closeRows(rows)

	var out []ABACPolicy
	for rows.Next() {
		var p ABACPolicy
		var enabled int
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Effect, &p.ResourceType,
			&p.Expression, &p.Priority, &enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning rbac policy: %w", err)
		}
		p.Enabled = enabled == 1
		p.Permissions = []string{}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rbac policies: %w", err)
	}

	if err := s.hydratePermissions(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// hydratePermissions fetches every binding row for the supplied policies
// in a single query and assigns them by policy id. Initialises any
// missing Permissions slice to a non-nil empty slice so JSON consumers
// receive `[]` rather than `null`.
func (s *PolicyService) hydratePermissions(ctx context.Context, policies []ABACPolicy) error {
	if len(policies) == 0 {
		return nil
	}

	placeholders := make([]string, len(policies))
	args := make([]any, len(policies))
	for i, p := range policies {
		placeholders[i] = "?"
		args[i] = p.ID
	}

	query := fmt.Sprintf( // SQL identifier: the interpolated list holds only "?" placeholders
		`SELECT policy_id, permission_key
		  FROM rbac_policy_bindings
		 WHERE policy_id IN (%s)
		 ORDER BY policy_id ASC, permission_key ASC`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("loading policy bindings: %w", err)
	}
	defer closeRows(rows)

	byID := make(map[string][]string, len(policies))
	for i := range policies {
		byID[policies[i].ID] = policies[i].Permissions
	}
	for rows.Next() {
		var policyID, perm string
		if err := rows.Scan(&policyID, &perm); err != nil {
			return fmt.Errorf("scanning policy binding: %w", err)
		}
		byID[policyID] = append(byID[policyID], perm)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating policy bindings: %w", err)
	}

	for i := range policies {
		perms := byID[policies[i].ID]
		if perms == nil {
			perms = []string{}
		}
		policies[i].Permissions = perms
	}
	return nil
}

// Get loads a single policy with its permissions.
func (s *PolicyService) Get(ctx context.Context, id string) (*ABACPolicy, error) {
	var p ABACPolicy
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, effect, resource_type, expression,
		       priority, enabled, COALESCE(created_by, ''), created_at, updated_at
		  FROM rbac_policies
		 WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Effect, &p.ResourceType,
		&p.Expression, &p.Priority, &enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPolicyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading rbac policy: %w", err)
	}
	p.Enabled = enabled == 1
	perms, err := s.permissionsFor(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Permissions = perms
	return &p, nil
}

// CreateInput is the validated body for POST /api/rbac/policies.
type CreateInput struct {
	Name         string
	Description  string
	Effect       string
	ResourceType string
	Expression   string
	Priority     int
	Enabled      bool
	Permissions  []string
	CreatedBy    string
}

// Create validates and inserts a new policy.
func (s *PolicyService) Create(ctx context.Context, in CreateInput) (*ABACPolicy, error) {
	if err := validatePolicyInput(in); err != nil {
		return nil, err
	}
	id, err := newPolicyID()
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer coredb.RollbackTx(tx)

	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO rbac_policies
		    (id, name, description, effect, resource_type, expression, priority, enabled, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.Description, in.Effect, in.ResourceType, in.Expression, in.Priority, enabledInt, in.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("inserting rbac policy: %w", err)
	}

	if err := bindPermissions(ctx, tx, id, in.Permissions); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rbac_policy_history (id, policy_id, actor_id, action, details)
		VALUES (?, ?, ?, 'create', ?)`,
		newHistoryID(), id, in.CreatedBy, "policy created"); err != nil {
		return nil, fmt.Errorf("recording policy history: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing rbac policy: %w", err)
	}

	return s.Get(ctx, id)
}

// UpdateInput is the validated body for PUT /api/rbac/policies/{id}.
type UpdateInput struct {
	Name         string
	Description  string
	Effect       string
	ResourceType string
	Expression   string
	Priority     int
	Enabled      bool
	Permissions  []string
	ActorID      string
}

// Update modifies an existing policy and rewrites its permission rows.
func (s *PolicyService) Update(ctx context.Context, id string, in UpdateInput) (*ABACPolicy, error) {
	if err := validatePolicyInput(CreateInput{
		Name: in.Name, Description: in.Description, Effect: in.Effect,
		ResourceType: in.ResourceType, Expression: in.Expression,
		Priority: in.Priority, Permissions: in.Permissions,
	}); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer coredb.RollbackTx(tx)

	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE rbac_policies
		   SET name = ?, description = ?, effect = ?, resource_type = ?,
		       expression = ?, priority = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		in.Name, in.Description, in.Effect, in.ResourceType, in.Expression,
		in.Priority, enabledInt, id)
	if err != nil {
		return nil, fmt.Errorf("updating rbac policy: %w", err)
	}
	rows := coredb.RowsAffected(res)
	if rows == 0 {
		return nil, ErrPolicyNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM rbac_policy_bindings WHERE policy_id = ?`, id); err != nil {
		return nil, fmt.Errorf("clearing policy bindings: %w", err)
	}
	if err := bindPermissions(ctx, tx, id, in.Permissions); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO rbac_policy_history (id, policy_id, actor_id, action, details)
		VALUES (?, ?, ?, 'update', ?)`,
		newHistoryID(), id, in.ActorID, "policy updated"); err != nil {
		return nil, fmt.Errorf("recording policy history: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing rbac policy update: %w", err)
	}

	return s.Get(ctx, id)
}

// Delete removes a policy and its bindings (CASCADE).
func (s *PolicyService) Delete(ctx context.Context, id, actorID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rbac_policies WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting rbac policy: %w", err)
	}
	rows := coredb.RowsAffected(res)
	if rows == 0 {
		return ErrPolicyNotFound
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO rbac_policy_history (id, policy_id, actor_id, action, details)
		VALUES (?, ?, ?, 'delete', ?)`,
		newHistoryID(), id, actorID, "policy deleted"); err != nil {
		return fmt.Errorf("recording policy history: %w", err)
	}
	return nil
}

// SetEnabled toggles a policy's enabled flag. Used by the admin UI
// "Enable" / "Disable" buttons when reviewing seeded policies.
func (s *PolicyService) SetEnabled(ctx context.Context, id string, enabled bool, actorID string) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE rbac_policies SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, enabledInt, id)
	if err != nil {
		return fmt.Errorf("updating policy enabled: %w", err)
	}
	rows := coredb.RowsAffected(res)
	if rows == 0 {
		return ErrPolicyNotFound
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO rbac_policy_history (id, policy_id, actor_id, action, details)
		VALUES (?, ?, ?, ?, ?)`,
		newHistoryID(), id, actorID, action, "policy "+action+"d"); err != nil {
		return fmt.Errorf("recording policy history: %w", err)
	}
	return nil
}

// PolicyHistoryEntry is one recorded change to a policy, as returned newest first by the history listing.
type PolicyHistoryEntry struct {
	ID        string `json:"id"`
	PolicyID  string `json:"policyId"`
	ActorID   string `json:"actorId"`
	Action    string `json:"action"`
	Details   string `json:"details"`
	CreatedAt string `json:"createdAt"`
}

// History returns the audit trail for a policy.
func (s *PolicyService) History(ctx context.Context, policyID string, limit int) ([]PolicyHistoryEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, policy_id, actor_id, action, COALESCE(details, ''), created_at
		  FROM rbac_policy_history
		 WHERE policy_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ?`, policyID, limit)
	if err != nil {
		return nil, fmt.Errorf("loading policy history: %w", err)
	}
	defer closeRows(rows)
	var out []PolicyHistoryEntry
	for rows.Next() {
		var e PolicyHistoryEntry
		if err := rows.Scan(&e.ID, &e.PolicyID, &e.ActorID, &e.Action, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// permissionsFor returns the permissions a policy is bound to.
func (s *PolicyService) permissionsFor(ctx context.Context, policyID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT permission_key FROM rbac_policy_bindings WHERE policy_id = ?`, policyID)
	if err != nil {
		return nil, fmt.Errorf("loading policy bindings: %w", err)
	}
	defer closeRows(rows)
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// bindPermissions inserts one row per permission key. tx may be a *sql.Tx.
func bindPermissions(ctx context.Context, tx *sql.Tx, policyID string, perms []string) error {
	seen := make(map[string]bool)
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rbac_policy_bindings (id, policy_id, permission_key)
			VALUES (?, ?, ?)`, newHistoryID(), policyID, p); err != nil {
			return fmt.Errorf("inserting policy binding: %w", err)
		}
	}
	return nil
}

// validatePolicyInput enforces the minimum shape required to insert or
// update a policy. It does NOT evaluate the expression — that happens
// on the admin "Test" button so admins get fast feedback.
func validatePolicyInput(in CreateInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("policy name is required")
	}
	if in.Effect != "allow" && in.Effect != "deny" {
		return errors.New("policy effect must be 'allow' or 'deny'")
	}
	if strings.TrimSpace(in.Expression) == "" {
		return errors.New("policy expression is required")
	}
	if len(in.Expression) > 8192 {
		return errors.New("policy expression too long (max 8192 chars)")
	}
	if len(in.Permissions) == 0 {
		return errors.New("at least one permission binding is required")
	}
	if in.Priority < 0 || in.Priority > 10000 {
		return errors.New("policy priority must be between 0 and 10000")
	}
	return nil
}

// LoadActiveForPermission returns all enabled policies that bind to
// the given permission key. The slice is ordered by priority ascending.
// Used by the ABAC evaluator to decide which policies to evaluate.
func (s *PolicyService) LoadActiveForPermission(ctx context.Context, perm string) ([]ABACPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.description, p.effect, p.resource_type, p.expression,
		       p.priority, p.enabled, COALESCE(p.created_by, ''), p.created_at, p.updated_at
		  FROM rbac_policies p
		  INNER JOIN rbac_policy_bindings b ON b.policy_id = p.id
		 WHERE p.enabled = 1 AND b.permission_key = ?
		 ORDER BY p.priority ASC, p.name ASC`,
		perm)
	if err != nil {
		return nil, fmt.Errorf("loading active policies: %w", err)
	}
	defer closeRows(rows)

	var out []ABACPolicy
	for rows.Next() {
		var p ABACPolicy
		var enabled int
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Effect, &p.ResourceType,
			&p.Expression, &p.Priority, &enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Enabled = enabled == 1
		out = append(out, p)
	}
	return out, rows.Err()
}
