// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
)

// decisionAttributes loads what a policy can read for the route gate; each lookup
// fails soft so a broken provider never locks anyone out.
func (s *Service) decisionAttributes(ctx context.Context, req AuthzRequest) *Attributes {
	attrs := &Attributes{Resource: req.ResourceAttributes, Request: req.Request}
	if u, err := FetchUserAttributes(ctx, s.db, req.UserID); err == nil {
		attrs.User = u
	}
	if req.EnvironmentID != "" {
		attrs.Env, attrs.envUnresolved = s.envAttributes(ctx, req.EnvironmentID)
	}
	if attrs.Resource == nil && req.ResourceType != "" && req.ResourceID != "" {
		if r := s.attributes.Resolve(ctx, req); r != nil {
			attrs.Resource = r
		}
	}
	if attrs.Resource == nil && req.ResourceType != "" {
		attrs.Resource = &ResourceAttributes{Type: req.ResourceType, ID: req.ResourceID}
	}
	return attrs
}

// envAttributes is the one environment lookup the route gate, Authorize and Explain
// share. The id is the request's own, so it is always exposed; unresolved reports a
// failed lookup, after which only env.id may be trusted.
func (s *Service) envAttributes(ctx context.Context, envID string) (env *EnvAttributes, unresolved bool) {
	if s.db == nil {
		return &EnvAttributes{ID: envID}, true
	}
	e, err := FetchEnvAttributes(ctx, s.db, envID)
	if err != nil {
		// A missing row is not an error here (FetchEnvAttributes returns the bare id),
		// so this is a real read failure worth an operator's attention.
		slog.WarnContext(ctx, "abac: environment lookup failed; env-dependent policies fail closed", "environment_id", envID, "error", err)
		return &EnvAttributes{ID: envID}, true
	}
	return e, false
}

// envRef finds every mention of the env bundle; envIDRef is the one mention that
// stays trustworthy when the environment row could not be read.
var (
	envRef   = regexp.MustCompile(`\benv\b`)
	envIDRef = regexp.MustCompile(`^\s*\.\s*id\b`)
)

// readsUnresolvedEnv reports whether expression mentions env in any form other than
// env.id. It is textual and errs towards true: a false positive only makes an
// unresolved lookup more conservative. String literals are blanked first, so the id
// in `env.id != "env-prod"` is not mistaken for a read of the env bundle.
func readsUnresolvedEnv(expression string) bool {
	code := stripStringLiterals(expression)
	for _, loc := range envRef.FindAllStringIndex(code, -1) {
		if !envIDRef.MatchString(code[loc[1]:]) {
			return true
		}
	}
	return false
}

// stripStringLiterals empties every '…' and "…" literal, honouring backslash escapes.
// Template literals are kept whole because ${env.name} inside one is a real read. An
// unterminated literal is dropped to the end, which only hides text that cannot compile.
func stripStringLiterals(s string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
			b.WriteByte(c)
		case quote == 0:
			b.WriteByte(c)
		case c == '\\':
			i++
		case c == quote:
			quote = 0
			b.WriteByte(c)
		}
	}
	return b.String()
}

// evalPolicies is the one policy walk the route gate, Authorize and Explain share: the
// first enabled policy, in priority order, whose resource type fits and whose expression
// holds. record audits a broken expression and a matching deny; Explain passes false so
// a preview writes nothing.
//
// When the environment lookup failed, a policy reading env beyond env.id fails closed:
// a deny applies without evaluation and an allow is skipped, because an empty env.name
// would otherwise satisfy `env.name != "prod"` and grant in prod.
func (s *Service) evalPolicies(ctx context.Context, policies []ABACPolicy, req AuthzRequest, attrs *Attributes, record bool) *ABACPolicy {
	for i := range policies {
		p := &policies[i]
		if p.ResourceType != "" && p.ResourceType != req.ResourceType {
			continue
		}
		if attrs != nil && attrs.envUnresolved && readsUnresolvedEnv(p.Expression) {
			if p.Effect != "deny" {
				continue
			}
			if record {
				RecordABACDenied(attrs, req, p.ID)
			}
			return p
		}
		res := s.abacExec.Evaluate(ctx, p.Expression, attrs)
		if res.Err != nil {
			// A broken expression is shown to the admin who wrote it and skipped;
			// it never denies the end user.
			if record && errors.Is(res.Err, ErrABACInvalidExpression) {
				RecordABACDenied(attrs, req, p.ID)
			}
			continue
		}
		if !res.Matched {
			continue
		}
		if record && p.Effect == "deny" {
			RecordABACDenied(attrs, req, p.ID)
		}
		return p
	}
	return nil
}
