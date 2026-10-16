// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package selfguard holds the host_self.manage requirement on McHarbor's own containers
// and host. On the environment McHarbor runs on, containers.manage reaches McHarbor's
// keys and database through exec, recreate or a socket-mounting container, so those
// paths need the global host_self.manage key as well.
package selfguard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// Permission is the key a McHarbor-targeting operation needs on top of its own.
const Permission = rbac.PermHostSelfManage

// ErrRequired is the sentinel a service returns when an operation needs host_self.manage
// and the request's principal lacks it; errors.As a *Refusal for the details.
var ErrRequired = errors.New("host_self.manage required")

// Refusal says what was refused and why, so the handler can audit and answer with it.
type Refusal struct {
	// Code is the i18n code the response carries.
	Code i18n.MsgCode
	// Target is the docker.SelfTarget kind, or "host_access" / "stack".
	Target string
	// Reasons are host-access reason codes, or the services and their reasons.
	Reasons []string
}

// Error describes the refusal for logs; the operator sees the translated Code.
func (e *Refusal) Error() string {
	msg := "host_self.manage required: " + e.Target
	if len(e.Reasons) > 0 {
		msg += " (" + strings.Join(e.Reasons, ",") + ")"
	}
	return msg
}

// Is makes errors.Is(err, ErrRequired) hold for every Refusal.
func (e *Refusal) Is(target error) bool { return target == ErrRequired }

// DeniedData is the 403 payload: the key to grant and what triggered the requirement.
type DeniedData struct {
	Permission string   `json:"permission"`
	Target     string   `json:"target,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
}

type decision struct {
	once    sync.Once
	decide  func() (bool, error)
	allowed bool
	err     error
	// req and log let a service deep in the request path audit a refusal against the
	// originating request, with its user and client address.
	req *http.Request
	log *audit.Logger
}

type ctxKey struct{}

// Middleware attaches a lazy host_self.manage decision for the request's principal, so
// any service down the request path can ask Allowed without holding the RBAC service.
// It is mounted after authentication in the protected route group; log receives the
// refusals AuditRefusal records.
func Middleware(svc *rbac.Service, log *audit.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.UserFromContext(r.Context())
			if user == nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			d := &decision{decide: func() (bool, error) {
				if user.ID == "system" {
					return true, nil
				}
				if svc == nil {
					return false, nil
				}
				return svc.HasPermission(ctx, user.ID, "", Permission)
			}, log: log}
			r = r.WithContext(context.WithValue(ctx, ctxKey{}, d))
			d.req = r
			next.ServeHTTP(w, r)
		})
	}
}

// WithDecision fixes the answer Allowed gives for ctx; fixtures and tests use it.
func WithDecision(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, ctxKey{}, &decision{decide: func() (bool, error) { return allowed, nil }})
}

// Allowed reports whether ctx's principal holds the global host_self.manage. A context
// with no user is McHarbor acting for itself (a job, the self-update flow) and is allowed;
// a user with no decision attached is refused, so a route mounted outside the middleware
// fails closed rather than open.
func Allowed(ctx context.Context) (bool, error) {
	if d, ok := ctx.Value(ctxKey{}).(*decision); ok {
		d.once.Do(func() { d.allowed, d.err = d.decide() })
		return d.allowed, d.err
	}
	user := auth.UserFromContext(ctx)
	if user == nil || user.ID == "system" {
		return true, nil
	}
	return false, nil
}

// Attended is Allowed for a path that may run with nobody behind it. Only a decision the
// middleware (or WithDecision) attached can allow; a bare context is refused, because a
// reconciler pass or a GitOps promotion has no principal whose host_self.manage it could ask.
func Attended(ctx context.Context) (bool, error) {
	d, ok := ctx.Value(ctxKey{}).(*decision)
	if !ok {
		return false, nil
	}
	d.once.Do(func() { d.allowed, d.err = d.decide() })
	return d.allowed, d.err
}

// RequireAttended returns a Refusal with code, target and reasons unless Attended allows ctx.
// Callers invoke it only once they know the operation reaches McHarbor.
func RequireAttended(ctx context.Context, code i18n.MsgCode, target string, reasons []string) error {
	allowed, err := Attended(ctx)
	if err != nil {
		return fmt.Errorf("deciding host_self.manage: %w", err)
	}
	if allowed {
		return nil
	}
	return &Refusal{Code: code, Target: target, Reasons: reasons}
}

// Refuse answers 403 with the refusal's code and a DeniedData payload naming the key,
// records the denial for the RBAC denied-alert hook, and writes a warning audit row.
func Refuse(w http.ResponseWriter, r *http.Request, log *audit.Logger, ref *Refusal, entry audit.Entry) {
	record(r, log, ref, entry)
	Deny(w, r, ref)
}

// Deny writes the 403 alone, for a refusal a service already audited with AuditRefusal.
func Deny(w http.ResponseWriter, r *http.Request, ref *Refusal) {
	response.ErrCodeWithData(w, r, http.StatusForbidden, ref.Code, DeniedData{
		Permission: string(Permission), Target: ref.Target, Reasons: ref.Reasons,
	})
}

// AuditRefusal records ref against the request ctx came from, for a service that refuses
// below the handler (a compose up, an app install). Outside a request it does nothing.
func AuditRefusal(ctx context.Context, ref *Refusal, entry audit.Entry) {
	d, ok := ctx.Value(ctxKey{}).(*decision)
	if !ok || d.req == nil {
		return
	}
	record(d.req, d.log, ref, entry)
}

func record(r *http.Request, log *audit.Logger, ref *Refusal, entry audit.Entry) {
	rbac.RecordDenied(r, auth.UserFromContext(r.Context()), Permission, entry.EnvironmentID)
	if log == nil {
		return
	}
	entry.Action = "host_self.denied"
	entry.Severity = "warning"
	details := "permission=" + string(Permission) + " target=" + ref.Target
	if len(ref.Reasons) > 0 {
		details += " reasons=" + strings.Join(ref.Reasons, ",")
	}
	if entry.Details != "" {
		details = entry.Details + " " + details
	}
	entry.Details = details
	log.Log(r, entry)
}
