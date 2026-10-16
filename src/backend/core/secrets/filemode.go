// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"context"
	"regexp"
	"strings"
)

// SecretMountPath is where a container's secret files appear. `/run/secrets`
// is the path Docker Swarm uses and the one the docker-library images already
// look in, so an image that supports the `_FILE` convention needs no extra
// configuration to find them.
const SecretMountPath = "/run/secrets"

// FileModePlan is the result of turning a set of environment variables that
// carry `${secret:NAME}` references into file-based injection.
type FileModePlan struct {
	// Env is the environment to actually set on the container. Every entry
	// that held a reference has been replaced by `<VAR>_FILE=<path>`; every
	// other entry is unchanged.
	Env map[string]string
	// Files maps a filename under the mount path to its plaintext content.
	// Empty when nothing referenced a secret, which is the signal to skip the
	// volume entirely.
	Files map[string]string
	// Replaced names the variables that became `_FILE` entries, for logging
	// and for the UI to explain what it did.
	Replaced []string
}

// fileVarNameRE is the character set a filename may use. A variable name is
// already constrained to `[A-Z0-9_]` by the resolver, but an env var key is
// not — it is whatever the operator typed — so it is sanitised before being
// used as a path component.
var fileVarNameRE = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// PlanFileMode resolves every `${secret:NAME}` in `vars` and returns the
// environment to set plus the files to write, instead of returning the values
// inline.
//
// Why this exists: the resolver's ordinary output puts the plaintext in the
// container's environment, where `docker inspect` — and therefore any other
// Docker manager on the host — reads it. That is a documented limit of the
// vault, not a bug, but it is the limit most people trip over. This is the way
// round it: the value goes in a file, and the environment carries the path.
//
// The `_FILE` suffix is the docker-library convention (`POSTGRES_PASSWORD_FILE`,
// `MYSQL_ROOT_PASSWORD_FILE`, `WORDPRESS_DB_PASSWORD_FILE`, …). An image that
// does not honour it will not see the variable at all, which fails loudly at
// start-up rather than silently running with a blank credential — the same
// choice the resolver makes for a missing reference.
//
// A variable whose value carries a reference **embedded in a larger string**
// is left as an ordinary environment entry: `postgres://u:${secret:PW}@host/db`
// has no `_FILE` equivalent, because the image expects a whole DSN, not a
// password file. Those keep the current behaviour, and Replaced does not name
// them.
func (s *Service) PlanFileMode(
	ctx context.Context, vars map[string]string, scope ResolveScope,
) (*FileModePlan, error) {
	plan := &FileModePlan{
		Env:   make(map[string]string, len(vars)),
		Files: map[string]string{},
	}

	for key, value := range vars {
		names, _ := parseReferences(value)

		// Not a reference at all: pass it through untouched.
		if len(names) == 0 {
			plan.Env[key] = value
			continue
		}

		// A reference that is the *whole* value is the only shape a file can
		// stand in for. Anything else is interpolation into a larger string.
		if len(names) != 1 || strings.TrimSpace(value) != referencePrefix+names[0]+"}" {
			resolved, err := s.ResolveValue(ctx, value, scope, key)
			if err != nil {
				return nil, err
			}
			plan.Env[key] = resolved
			continue
		}

		resolved, err := s.ResolveValue(ctx, value, scope, key)
		if err != nil {
			return nil, err
		}

		filename := fileVarNameRE.ReplaceAllString(key, "_")
		plan.Files[filename] = resolved
		plan.Env[key+"_FILE"] = SecretMountPath + "/" + filename
		plan.Replaced = append(plan.Replaced, key)
	}

	return plan, nil
}

// PlanFileModeSlice is PlanFileMode for a Docker-style `KEY=value` slice.
func (s *Service) PlanFileModeSlice(
	ctx context.Context, env []string, scope ResolveScope,
) (*FileModePlan, []string, error) {
	vars := make(map[string]string, len(env))
	// Entries with no `=` are passed through verbatim: `FOO` alone means
	// "inherit FOO from the daemon's environment", which has no value here to
	// put in a file.
	var passthrough []string
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			passthrough = append(passthrough, entry)
			continue
		}
		vars[key] = value
	}

	plan, err := s.PlanFileMode(ctx, vars, scope)
	if err != nil {
		return nil, nil, err
	}

	out := make([]string, 0, len(plan.Env)+len(passthrough))
	out = append(out, passthrough...)
	for key, value := range plan.Env {
		out = append(out, key+"="+value)
	}
	return plan, out, nil
}
