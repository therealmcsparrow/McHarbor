// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	coredemo "github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// vaultSeededKey marks demo.db as carrying the vault story.
const (
	vaultSeededKey   = "demo.vault_seeded"
	vaultSeedVersion = "1"
	// vaultRefsPerTemplate caps how many stacks of one template reference a
	// secret, so a large estate does not bury the references panel.
	vaultRefsPerTemplate = 3
)

// vaultSeedTables are the tables the vault seed writes.
var vaultSeedTables = []string{"settings", "users", "secrets", "secret_versions", "secret_references"}

// vaultSeedCounts sizes what one seed wrote.
type vaultSeedCounts struct{ Secrets, Versions, References int }

// demoStack is one estate stack a seeded secret can be scoped to or used by.
type demoStack struct{ envID, name string }

// vaultSeeded reports whether demo.db already carries this version of the seed.
func vaultSeeded(ctx context.Context, conn *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, conn, "settings")
	if err != nil || !ok {
		return false, err
	}
	var value string
	err = conn.QueryRowContext(ctx, "SELECT COALESCE(value, '') FROM settings WHERE key = ?", vaultSeededKey).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("reading %s: %w", vaultSeededKey, err)
	}
	return value == vaultSeedVersion, nil
}

// seedVault fills the secrets vault with credentials the demo stacks plausibly
// use. Values are derived from the marker secret, so none is in the source and a
// demo reset mints new ones; the settings flag is written last, so a seed that
// fails part-way is completed on the next boot rather than left half done.
func seedVault(ctx context.Context, conn *sql.DB, enc *encryption.Service, e *estate.Estate,
	envIDs map[string]string, m *coredemo.Marker, now time.Time) (vaultSeedCounts, error) {
	var counts vaultSeedCounts
	if !coredemo.Active() {
		return counts, errNotDemo
	}
	seeded, err := vaultSeeded(ctx, conn)
	if err != nil || seeded {
		return counts, err
	}
	if ok, err := allTablesExist(ctx, conn, vaultSeedTables...); err != nil || !ok {
		return counts, err
	}

	envOrder, stacks := vaultTargets(e, envIDs)
	owners, err := vaultOwners(ctx, conn)
	if err != nil {
		return counts, err
	}
	svc := secrets.NewService(conn, enc, nil)

	for _, spec := range demoVaultSecrets() {
		envID, stackName, ok := vaultScope(spec, envOrder, stacks)
		if !ok {
			continue
		}
		id, versions, err := insertVaultSecret(ctx, conn, enc, m, spec, envID, stackName, owners[spec.owner], now)
		if err != nil {
			return counts, err
		}
		counts.Secrets++
		counts.Versions += versions

		refs := 0
		for _, ref := range spec.refs {
			for _, st := range stacks[ref.template] {
				applied := spec.versions
				if refs < spec.stale {
					applied = spec.versions - 1
				}
				name, env, field := st.name, st.envID, ref.field
				if err := svc.TouchReference(ctx, secrets.SecretReference{
					SecretID: id, ResourceType: "stack", ResourceID: st.name,
					ResourceName: &name, EnvironmentID: &env, Field: &field, AppliedVersion: applied,
				}); err != nil {
					return counts, fmt.Errorf("seeding reference %s -> %s: %w", spec.name, st.name, err)
				}
				refs++
			}
		}
		counts.References += refs
	}

	if _, err := conn.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, vaultSeededKey, vaultSeedVersion); err != nil {
		return counts, fmt.Errorf("marking the vault seeded: %w", err)
	}
	return counts, nil
}

// vaultTargets lists the seeded environments in estate order and, per stack
// template, the demo stacks built from it. Stacks come out of a map, so they are
// sorted to keep the same stack the owner of a secret across boots.
func vaultTargets(e *estate.Estate, envIDs map[string]string) ([]string, map[string][]demoStack) {
	var envOrder []string
	stacks := make(map[string][]demoStack)
	for _, h := range e.Hosts {
		envID, ok := envIDs[h.Key]
		if !ok {
			continue
		}
		envOrder = append(envOrder, envID)
		names := make([]string, 0)
		for _, st := range h.Stacks() {
			names = append(names, st.Name)
		}
		slices.Sort(names)
		for _, name := range names {
			tpl := name
			if i := strings.IndexByte(name, '-'); i > 0 {
				tpl = name[:i]
			}
			if len(stacks[tpl]) < vaultRefsPerTemplate {
				stacks[tpl] = append(stacks[tpl], demoStack{envID: envID, name: name})
			}
		}
	}
	return envOrder, stacks
}

// vaultOwners maps each demo username to its user id. The demo team is seeded
// by the history pass, and a secret whose owner is missing is left unattributed
// rather than failing the foreign key.
func vaultOwners(ctx context.Context, conn *sql.DB) (map[string]*string, error) {
	owners := make(map[string]*string)
	// bounded by demoVaultSecrets, a fixed fixture list.
	for _, spec := range demoVaultSecrets() {
		if _, done := owners[spec.owner]; done {
			continue
		}
		id := stableID("user", spec.owner)
		var found string
		err := conn.QueryRowContext(ctx, "SELECT id FROM users WHERE id = ?", id).Scan(&found)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			owners[spec.owner] = nil
		case err != nil:
			return nil, fmt.Errorf("reading demo user %s: %w", spec.owner, err)
		default:
			owners[spec.owner] = &found
		}
	}
	return owners, nil
}

// vaultScope resolves where a secret lives. A stack or environment the estate
// does not have skips the secret instead of scoping it somewhere misleading.
func vaultScope(spec vaultSecret, envOrder []string, stacks map[string][]demoStack) (envID, stackName *string, ok bool) {
	switch spec.scope {
	case secrets.ScopeGlobal:
		return nil, nil, true
	case secrets.ScopeEnvironment:
		if spec.envIndex >= len(envOrder) {
			return nil, nil, false
		}
		return &envOrder[spec.envIndex], nil, true
	case secrets.ScopeStack:
		owned := stacks[spec.template]
		if len(owned) == 0 {
			return nil, nil, false
		}
		return &owned[0].envID, &owned[0].name, true
	}
	return nil, nil, false
}

// insertVaultSecret writes one secret and the history rows of its earlier
// versions, spacing the rotations evenly across the secret's lifetime.
func insertVaultSecret(ctx context.Context, conn *sql.DB, enc *encryption.Service, m *coredemo.Marker,
	spec vaultSecret, envID, stackName, owner *string, now time.Time) (string, int, error) {
	scopeKey := secrets.ScopeKey(spec.name, envID, stackName)
	id := stableID("secret", scopeKey)
	created := now.AddDate(0, 0, -spec.createdDays)
	span := time.Duration(spec.createdDays) * 24 * time.Hour / time.Duration(spec.versions+1)
	rotated := func(v int) time.Time { return created.Add(time.Duration(v-1) * span) }

	value, err := enc.Encrypt(vaultValue(m, spec, spec.versions))
	if err != nil {
		return "", 0, fmt.Errorf("encrypting demo secret %s: %w", spec.name, err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO secrets (id, name, description, kind, group_name, scope, environment_id, stack_name,
			scope_key, value, version, generated, system, last_used_at, last_revealed_at,
			created_by, created_at, updated_at, auto_apply)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		id, spec.name, spec.description, spec.kind, spec.group, spec.scope, envID, stackName,
		scopeKey, value, spec.versions, boolToInt(spec.generated),
		vaultAgo(now, spec.usedDays, 5), vaultAgo(now, spec.revealDays, 14),
		owner, rfc(created), rfc(rotated(spec.versions)), boolToInt(spec.autoApply)); err != nil {
		return "", 0, fmt.Errorf("seeding demo secret %s: %w", spec.name, err)
	}

	versions := 0
	for v := 1; v < spec.versions; v++ {
		old, err := enc.Encrypt(vaultValue(m, spec, v))
		if err != nil {
			return "", versions, fmt.Errorf("encrypting demo secret %s v%d: %w", spec.name, v, err)
		}
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO secret_versions (id, secret_id, version, value, rotated_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`,
			stableID("secret_version", scopeKey+":"+strconv.Itoa(v)), id, v, old, owner, rfc(rotated(v+1))); err != nil {
			return "", versions, fmt.Errorf("seeding demo secret %s v%d: %w", spec.name, v, err)
		}
		versions++
	}
	return id, versions, nil
}

// vaultAgo turns an age in days into a timestamp, offset by hours so a column of
// them does not all read midnight. A negative age is an event that never happened.
func vaultAgo(now time.Time, days, hours int) *string {
	if days < 0 {
		return nil
	}
	s := rfc(now.AddDate(0, 0, -days).Add(-time.Duration(hours) * time.Hour))
	return &s
}

// vaultValue derives version v of a secret's plaintext from the marker secret,
// shaped like the kind it claims to be so a reveal looks like the real thing.
func vaultValue(m *coredemo.Marker, spec vaultSecret, v int) string {
	material := func(n int) []byte {
		var out []byte
		for i := 0; len(out) < n; i++ {
			mac := hmac.New(sha256.New, []byte(m.Secret))
			mac.Write([]byte("vault:" + spec.name + ":" + strconv.Itoa(v) + ":" + strconv.Itoa(i)))
			out = mac.Sum(out)
		}
		return out[:n]
	}
	token := base64.RawURLEncoding.EncodeToString(material(32))
	switch spec.kind {
	case "password":
		return token[:24]
	case "env":
		if spec.prefix != "" {
			raw := strings.ToUpper(hex.EncodeToString(material(10)))
			return spec.prefix + "T" + raw[:8] + "/B" + raw[8:18] + "/" + token[:24]
		}
		return token
	case "private_key":
		return pemBlock("PRIVATE KEY", material(1190))
	case "cert":
		return pemBlock("CERTIFICATE", material(890))
	case "ssh_key":
		return pemBlock("OPENSSH PRIVATE KEY", material(400))
	case "json":
		return fmt.Sprintf(`{"type":"service_account","project_id":"acme-demo","private_key_id":%q,"client_email":"loki@acme-demo.iam.gserviceaccount.com"}`,
			hex.EncodeToString(material(20)))
	}
	return spec.prefix + hex.EncodeToString(material(20))
}

// pemBlock wraps der-shaped bytes the way PEM does, 64 columns to a line.
func pemBlock(label string, der []byte) string {
	body := base64.StdEncoding.EncodeToString(der)
	var b strings.Builder
	b.WriteString("-----BEGIN " + label + "-----\n")
	for len(body) > 64 {
		b.WriteString(body[:64] + "\n")
		body = body[64:]
	}
	b.WriteString(body + "\n-----END " + label + "-----\n")
	return b.String()
}
