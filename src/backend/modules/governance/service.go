// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package governance answers "would this be allowed, and why" and "where does
// the grant model contradict itself" without a second authorization engine.
package governance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// ErrInvalidInput is returned when a simulation names no user, or neither a
// permission nor a route.
var ErrInvalidInput = errors.New("simulation needs a user and a permission or route")

// ErrRouteNotFound is returned when method and path match no registered route.
var ErrRouteNotFound = errors.New("no registered route matches")

// Explainer is the RBAC entry point the simulator calls.
type Explainer interface {
	Explain(ctx context.Context, req rbac.ExplainRequest) (*rbac.Explanation, error)
}

// Service backs the simulator and the conflict report.
type Service struct {
	db       *sql.DB
	explain  Explainer
	policies *rbac.PolicyService
	routes   func() []apispec.Operation
}

// NewService builds the service; routes supplies the registered operations so a
// route's gates are read off the router rather than restated here.
func NewService(database *sql.DB, explainer Explainer, routes func() []apispec.Operation) *Service {
	return &Service{db: database, explain: explainer, policies: rbac.NewPolicyService(database), routes: routes}
}

// SimulateInput selects the subject and either a permission or a route.
type SimulateInput struct {
	UserID        string `json:"userId"`
	EnvironmentID string `json:"environmentId"`
	StackName     string `json:"stackName"`
	Permission    string `json:"permission"`
	Method        string `json:"method"`
	Path          string `json:"path"`
}

// GateResult is one route gate and the decision for each key it accepts.
type GateResult struct {
	Scope     string              `json:"scope"`
	Any       bool                `json:"any"`
	Allowed   bool                `json:"allowed"`
	Decisions []*rbac.Explanation `json:"decisions"`
}

// SimulateResult is the simulator's answer. Route is empty for a permission-only
// simulation; for a route, Allowed requires every gate in the chain to pass.
type SimulateResult struct {
	Allowed bool         `json:"allowed"`
	Route   string       `json:"route,omitempty"`
	Method  string       `json:"method,omitempty"`
	Exempt  string       `json:"exempt,omitempty"`
	Gates   []GateResult `json:"gates"`
}

// Simulate evaluates a permission, or every gate on a route, for another user.
func (s *Service) Simulate(ctx context.Context, in SimulateInput) (*SimulateResult, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	in.Permission = strings.TrimSpace(in.Permission)
	in.Path = strings.TrimSpace(in.Path)
	if in.UserID == "" || (in.Permission == "" && in.Path == "") {
		return nil, ErrInvalidInput
	}
	if in.Path == "" {
		scope := rbac.ScopeEnvironment
		if in.StackName != "" {
			scope = rbac.ScopeStack
		}
		exp, err := s.explain.Explain(ctx, rbac.ExplainRequest{UserID: in.UserID, EnvironmentID: in.EnvironmentID,
			StackName: in.StackName, Permission: rbac.Permission(in.Permission), Scope: scope})
		if err != nil {
			return nil, err
		}
		return &SimulateResult{Allowed: exp.Allowed, Gates: []GateResult{{Scope: string(scope), Allowed: exp.Allowed, Decisions: []*rbac.Explanation{exp}}}}, nil
	}

	op, params := s.matchOperation(strings.ToUpper(strings.TrimSpace(in.Method)), in.Path)
	if op == nil {
		return nil, ErrRouteNotFound
	}
	stack := in.StackName
	if stack == "" {
		stack = params["name"]
	}
	out := &SimulateResult{Allowed: true, Route: op.Path, Method: op.Method, Exempt: op.Exempt, Gates: []GateResult{}}
	for _, gate := range op.Perms {
		res, err := s.evaluateGate(ctx, in, stack, gate)
		if err != nil {
			return nil, err
		}
		out.Gates = append(out.Gates, *res)
		out.Allowed = out.Allowed && res.Allowed
	}
	return out, nil
}

func (s *Service) evaluateGate(ctx context.Context, in SimulateInput, stack string, gate rbac.PermRequirement) (*GateResult, error) {
	res := &GateResult{Scope: string(gate.Scope), Any: gate.Any, Decisions: []*rbac.Explanation{}}
	for _, key := range gate.Keys {
		env := in.EnvironmentID
		scope := gate.Scope
		switch scope {
		case rbac.ScopeGlobal:
			env = ""
		case rbac.ScopeOwner, rbac.ScopeAccount:
			scope = rbac.ScopeEnvironment
		}
		exp, err := s.explain.Explain(ctx, rbac.ExplainRequest{UserID: in.UserID, EnvironmentID: env, StackName: stack, Permission: key, Scope: scope,
			Method: strings.ToUpper(in.Method), Path: in.Path})
		if err != nil {
			return nil, err
		}
		res.Decisions = append(res.Decisions, exp)
		if exp.Allowed {
			res.Allowed = true
		}
	}
	if gate.Scope == rbac.ScopeAccount && len(gate.Keys) == 0 {
		exp, err := s.explain.Explain(ctx, rbac.ExplainRequest{UserID: in.UserID, Permission: rbac.PermAccountManage, Scope: rbac.ScopeGlobal})
		if err != nil {
			return nil, err
		}
		res.Decisions = append(res.Decisions, exp)
		res.Allowed = exp.Allowed
	}
	return res, nil
}

// matchOperation resolves a concrete or pattern path to a registered operation.
// A static segment beats a parameter, as it does in chi.
func (s *Service) matchOperation(method, path string) (*apispec.Operation, map[string]string) {
	if s.routes == nil {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/api") {
		path = "/api" + ensureSlash(path)
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	want := strings.Split(strings.Trim(path, "/"), "/")
	var best *apispec.Operation
	var bestParams map[string]string
	bestScore := -1
	for _, op := range s.routes() {
		if method != "" && op.Method != method {
			continue
		}
		pattern := strings.Split(strings.Trim(op.Path, "/"), "/")
		if len(pattern) != len(want) {
			continue
		}
		params := map[string]string{}
		score := 0
		ok := true
		for i, seg := range pattern {
			switch {
			case seg == want[i]:
				score++
			case strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}"):
				params[strings.Trim(seg, "{}")] = want[i]
			default:
				ok = false
			}
			if !ok {
				break
			}
		}
		if ok && score > bestScore {
			o := op
			best, bestParams, bestScore = &o, params, score
		}
	}
	return best, bestParams
}

func ensureSlash(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

// Conflicts returns every wildcard grant and shadowed ABAC policy.
func (s *Service) Conflicts(ctx context.Context) ([]rbac.ConflictWarning, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, permissions, COALESCE(is_system, 0) FROM roles ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("reading roles: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Warn("governance: closing rows failed", "error", cerr)
		}
	}()
	var roles []rbac.RoleGrantSet
	for rows.Next() {
		var r rbac.RoleGrantSet
		var raw string
		var system int
		if err := rows.Scan(&r.ID, &r.Name, &raw, &system); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		if json.Unmarshal([]byte(raw), &r.Permissions) != nil {
			continue
		}
		r.IsSystem = system == 1
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	policies, err := s.policies.List(ctx)
	if err != nil {
		return nil, err
	}
	return rbac.DetectConflicts(roles, policies), nil
}
