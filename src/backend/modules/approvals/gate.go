// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package approvals

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

type replayKey struct{}

// withReplay marks a context as the replay of an approved request, which is the
// only way a guarded request passes the gate while its rule is enabled.
func withReplay(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, replayKey{}, id)
}

func replayID(ctx context.Context) string {
	id, _ := ctx.Value(replayKey{}).(string)
	return id
}

// PermissionChecker is the part of the RBAC service the gate needs. RouteDecision
// is the environment-scoped route gate itself, policies included.
type PermissionChecker interface {
	RouteDecision(r *http.Request, userID, envID string, perm rbac.Permission) (bool, error)
	HasStackPermission(ctx context.Context, userID, envID, stackName string, perm rbac.Permission) (bool, error)
}

// Gate is the protected-router middleware that holds destructive requests.
type Gate struct {
	svc    *Service
	rbac   PermissionChecker
	audit  *audit.Logger
	logger *slog.Logger
	notify func(req *Request)
}

// NewGate builds the gate; notify is optional and tells approvers a request waits.
func NewGate(svc *Service, checker PermissionChecker, auditLog *audit.Logger, logger *slog.Logger, notify func(*Request)) *Gate {
	if logger == nil {
		logger = slog.Default()
	}
	return &Gate{svc: svc, rbac: checker, audit: auditLog, logger: logger, notify: notify}
}

// Middleware holds a guarded request when a rule applies and the caller would
// otherwise be allowed through, answering 202 with the approval id instead of
// running the handler. With no rule it is a pass-through, which is the default.
func (g *Gate) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		route, params := matchRoute(r.Method, strings.TrimPrefix(r.URL.Path, "/api"))
		if route == nil || replayID(r.Context()) != "" {
			next.ServeHTTP(w, r)
			return
		}
		user := auth.UserFromContext(r.Context())
		if user == nil || user.ID == "system" {
			next.ServeHTTP(w, r)
			return
		}
		env := response.ParseEnvID(r)
		rule, err := g.svc.RuleFor(r.Context(), route.Category, env)
		if err != nil {
			g.logger.Error("approvals: resolving rule", "error", err, "category", route.Category)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if rule == nil {
			next.ServeHTTP(w, r)
			return
		}
		allowed, err := g.callerMayRun(r, user.ID, env, route, params)
		if err != nil {
			g.logger.Error("approvals: permission check", "error", err, "category", route.Category)
			response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
			return
		}
		if !allowed {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if route.SkipWhenDryRun && bodyDryRun(body) {
			next.ServeHTTP(w, r)
			return
		}
		g.hold(w, r, user, env, rule, route, params, body)
	})
}

// callerMayRun mirrors the route's own gate: a caller it would refuse is sent
// on to that refusal rather than queued, so an approval can never widen access.
func (g *Gate) callerMayRun(r *http.Request, userID, env string, route *guardedRoute, params map[string]string) (bool, error) {
	for _, perm := range route.Perms {
		var ok bool
		var err error
		if route.StackParam != "" {
			ok, err = g.rbac.HasStackPermission(r.Context(), userID, env, params[route.StackParam], perm)
		} else {
			ok, err = g.rbac.RouteDecision(r, userID, env, perm)
		}
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func (g *Gate) hold(w http.ResponseWriter, r *http.Request, user *auth.User, env string, rule *Rule, route *guardedRoute, params map[string]string, body []byte) {
	resource := params[route.ResourceParam]
	if resource == "" {
		resource = bodyField(body, route.ResourceBodyField)
	}
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	keyID := ""
	if key := rbac.APIKeyFromContext(r.Context()); key != nil {
		keyID = key.KeyID
	}
	req, created, err := g.svc.Hold(r.Context(), HoldInput{
		Rule: rule, Category: route.Category, RequesterID: user.ID, RequesterName: user.Username,
		EnvironmentID: env, Method: r.Method, Path: path, RoutePattern: "/api" + route.Pattern,
		ResourceName: resource, Body: body, APIKeyID: keyID,
	})
	if err != nil {
		g.logger.Error("approvals: holding request", "error", err, "category", route.Category)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	if created {
		if g.audit != nil {
			g.audit.Log(r, audit.Entry{
				Action:        "approval.request",
				Severity:      "warning",
				EntityType:    "approval",
				EntityID:      req.ID,
				EntityName:    route.Category,
				Details:       fmt.Sprintf("held %s %s resource=%s required=%d", r.Method, "/api"+route.Pattern, resource, req.RequiredApprovers),
				EnvironmentID: env,
			})
		}
		if g.notify != nil {
			g.notify(req)
		}
	}
	response.ErrCodeWithData(w, r, http.StatusAccepted, i18n.ErrForbidden, HeldResponse{
		Reason:            ReasonApprovalRequired,
		ApprovalID:        req.ID,
		Category:          req.Category,
		RequiredApprovers: req.RequiredApprovers,
		ExpiresAt:         req.ExpiresAt,
	})
}
