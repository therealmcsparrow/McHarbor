// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// referencePattern is the `${secret:NAME}` syntax, with an optional leading
// `$` that escapes it. The name group uses the same character class as
// ValidateName, so a reference can only ever name a secret that could exist —
// `${secret:lower_case}` is not a reference at all and passes through
// untouched rather than failing a deploy over a typo in unrelated text.
const referencePrefix = "${secret:"

// ResolveScope tells the resolver where to look and what to record. The two
// halves are separate on purpose: EnvironmentID and StackName drive
// precedence, while ResourceType and ResourceID drive `secret_references`,
// and a container resolving against a stack's scope is a real case.
type ResolveScope struct {
	EnvironmentID string
	StackName     string

	// ResourceType is 'stack' | 'container' | 'app' | 'workflow' | 'module'.
	// Empty means "resolve but record nothing", which is what a dry run or a
	// validation pass wants.
	ResourceType string
	ResourceID   string
	// ResourceName is what an operator reads when asked to approve a restart.
	// ResourceID becomes a container id once the container exists, and nobody
	// recognises their database from a 64-character digest. Optional; the id
	// stands in when it is empty.
	ResourceName string
}

// ErrUnresolved names a reference that matched nothing. It is deliberately
// its own type rather than a sentinel: the deploy error has to name the
// reference, or the operator is left grepping a compose file for which of
// fifteen variables was wrong.
type ErrUnresolved struct {
	Name string
}

func (e *ErrUnresolved) Error() string {
	return "no secret named " + e.Name + " is visible at this scope"
}

// Is lets errors.Is(err, ErrNotFound) match an unresolved reference, so the
// handler's existing mapping onto i18n.ErrSecretNotFound covers this too.
func (e *ErrUnresolved) Is(target error) bool { return target == ErrNotFound }

// ContainsReference reports whether a value carries an unescaped
// `${secret:...}`. Used by the startup guard to spot stored values that no
// resolver covers.
func ContainsReference(value string) bool {
	names, _ := parseReferences(value)
	return len(names) > 0
}

// parseReferences returns the distinct secret names a value references, and
// the value with every escape (`$${secret:X}`) collapsed to its literal form.
//
// The escape is handled here rather than in a second pass because the two
// interact: `$$${secret:X}` is an escaped `$` followed by a live reference,
// and only a single left-to-right scan gets that right.
func parseReferences(value string) ([]string, string) {
	var (
		names []string
		seen  = map[string]bool{}
		out   strings.Builder
		i     int
	)
	for i < len(value) {
		idx := strings.Index(value[i:], referencePrefix)
		if idx < 0 {
			out.WriteString(value[i:])
			break
		}
		start := i + idx

		end := strings.Index(value[start:], "}")
		if end < 0 {
			// An unterminated `${secret:` is not a reference. Emit the rest
			// verbatim rather than guessing where it was meant to end.
			out.WriteString(value[i:])
			break
		}
		end += start

		name := value[start+len(referencePrefix) : end]
		if !nameRe.MatchString(name) {
			// Not a well-formed reference — pass it through untouched.
			out.WriteString(value[i : end+1])
			i = end + 1
			continue
		}

		// An immediately preceding '$' escapes the reference: emit the
		// literal `${secret:NAME}` with the escape character consumed.
		escaped := start > i && value[start-1] == '$'
		if escaped {
			out.WriteString(value[i : start-1])
			out.WriteString(value[start : end+1])
			i = end + 1
			continue
		}

		out.WriteString(value[i:start])
		out.WriteString(value[start : end+1])
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		i = end + 1
	}
	return names, out.String()
}

// lookup finds the secret visible at a scope, narrowest first: a stack-scoped
// secret beats an environment-scoped one, which beats a global one, which beats
// an application-scoped one. First match wins.
//
// The precedence is what makes "one NAME, different value per environment"
// work, and it is why a stack can override a shared credential without
// renaming anything.
func (s *Service) lookup(ctx context.Context, name string, scope ResolveScope) (string, string, int, error) {
	type candidate struct {
		query string
		args  []any
	}
	candidates := make([]candidate, 0, 4)

	if scope.StackName != "" {
		candidates = append(candidates, candidate{
			query: `SELECT id, value, version FROM secrets
			        WHERE name = ? AND scope = 'stack' AND stack_name = ?
			          AND (environment_id IS NULL OR environment_id = ?)
			        ORDER BY environment_id DESC LIMIT 1`,
			args: []any{name, scope.StackName, scope.EnvironmentID},
		})
	}
	if scope.EnvironmentID != "" {
		candidates = append(candidates, candidate{
			query: `SELECT id, value, version FROM secrets
			        WHERE name = ? AND scope = 'environment' AND environment_id = ?
			        LIMIT 1`,
			args: []any{name, scope.EnvironmentID},
		})
	}
	candidates = append(candidates, candidate{
		query: `SELECT id, value, version FROM secrets
		        WHERE name = ? AND scope = 'global' LIMIT 1`,
		args: []any{name},
	})
	candidates = append(candidates, candidate{
		query: `SELECT id, value, version FROM secrets
		        WHERE name = ? AND scope = 'application' LIMIT 1`,
		args: []any{name},
	})

	// Bounded at four candidates — stack, environment, global, application —
	// so the per-iteration read is scope precedence, not an N+1 over a result
	// set. Application is last deliberately: it can only ever resolve a name
	// that resolves to nothing today.
	for _, c := range candidates {
		var (
			id, ciphertext string
			version        int
		)
		err := s.db.QueryRowContext(ctx, c.query, c.args...).Scan(&id, &ciphertext, &version)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", "", 0, fmt.Errorf("looking up secret %s: %w", name, err)
		}

		// One of three named callers of decryptValue, which is the single
		// place this module calls encryption.Decrypt. See its comment.
		plaintext, err := s.decryptValue(ciphertext, id)
		if err != nil {
			return "", "", 0, fmt.Errorf("decrypting secret %s: %w", name, err)
		}
		return id, plaintext, version, nil
	}

	return "", "", 0, &ErrUnresolved{Name: name}
}

// ResolveValue substitutes every `${secret:NAME}` in one string.
//
// A miss **fails**. It never substitutes an empty string — that is how a
// database gets started with a blank root password, and it fails silently
// enough that nobody notices until it is on the internet.
func (s *Service) ResolveValue(ctx context.Context, value string, scope ResolveScope, field string) (string, error) {
	names, literal := parseReferences(value)
	if len(names) == 0 {
		return literal, nil
	}

	resolved := literal
	for _, name := range names {
		id, plaintext, version, err := s.lookup(ctx, name, scope)
		if err != nil {
			return "", err
		}
		resolved = strings.ReplaceAll(resolved, referencePrefix+name+"}", plaintext)
		s.recordUse(ctx, id, version, scope, field)
	}
	return resolved, nil
}

// ResolveMap resolves a whole environment map. Values are resolved into a new
// map; the caller's input is never mutated, because a caller that passed the
// stored map by accident would otherwise write plaintext back to the database
// on its next save.
func (s *Service) ResolveMap(ctx context.Context, vars map[string]string, scope ResolveScope) (map[string]string, error) {
	if len(vars) == 0 {
		return vars, nil
	}
	out := make(map[string]string, len(vars))
	for key, value := range vars {
		resolved, err := s.ResolveValue(ctx, value, scope, key)
		if err != nil {
			return nil, err
		}
		out[key] = resolved
	}
	return out, nil
}

// ResolveSlice resolves a Docker-style `KEY=value` environment slice. Only the
// value half is scanned — a reference in a variable's *name* is a typo, not a
// feature.
func (s *Service) ResolveSlice(ctx context.Context, env []string, scope ResolveScope) ([]string, error) {
	if len(env) == 0 {
		return env, nil
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			out = append(out, entry)
			continue
		}
		resolved, err := s.ResolveValue(ctx, value, scope, key)
		if err != nil {
			return nil, err
		}
		out = append(out, key+"="+resolved)
	}
	return out, nil
}

// recordUse stamps last_used_at and upserts the reference row, recording the
// version the resource is about to be built with. Best-effort: the deploy has
// already resolved successfully by the time this runs, and failing it over
// bookkeeping would turn a working deployment into a failed one.
//
// The stamp is optimistic — a deploy that fails after resolving leaves the
// reference claiming a version that never reached the host. Reporting a stale
// resource as current is the milder error than the reverse, which would have
// the impact list nag about resources that are already correct.
func (s *Service) recordUse(ctx context.Context, secretID string, version int, scope ResolveScope, field string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		"UPDATE secrets SET last_used_at = ? WHERE id = ?", now, secretID,
	); err != nil {
		slog.Warn("recording secret use failed", "secret_id", secretID, "error", err)
	}

	if scope.ResourceType == "" || scope.ResourceID == "" {
		return
	}
	var environmentID *string
	if scope.EnvironmentID != "" {
		environmentID = &scope.EnvironmentID
	}
	var fieldPtr *string
	if field != "" {
		fieldPtr = &field
	}
	resourceName := scope.ResourceName
	if resourceName == "" {
		resourceName = scope.ResourceID
	}
	if err := s.TouchReference(ctx, SecretReference{
		SecretID:       secretID,
		ResourceType:   scope.ResourceType,
		ResourceID:     scope.ResourceID,
		ResourceName:   &resourceName,
		EnvironmentID:  environmentID,
		Field:          fieldPtr,
		AppliedVersion: version,
	}); err != nil {
		slog.Warn("recording secret reference failed", "secret_id", secretID, "error", err)
	}
}
