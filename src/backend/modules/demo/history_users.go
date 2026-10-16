// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// demoPerson is one seeded colleague. They carry no password, so none of them
// can sign in and the first-user check still sees only the real accounts.
type demoPerson struct {
	username string
	name     string
	role     string
	group    string
}

var demoPeople = []demoPerson{
	{"a.jansen", "Anouk Jansen", "operator", "Platform"},
	{"m.okafor", "Michael Okafor", "operator", "Platform"},
	{"s.lindqvist", "Sofia Lindqvist", "operator", "Platform"},
	{"r.mehta", "Rahul Mehta", "operator", "Platform"},
	{"l.moreau", "Lucas Moreau", "operator", "SRE"},
	{"k.nakamura", "Kenji Nakamura", "operator", "SRE"},
	{"e.garcia", "Elena Garcia", "operator", "SRE"},
	{"t.becker", "Tobias Becker", "operator", "SRE"},
	{"p.oconnor", "Patrick O'Connor", "operator", "SRE"},
	{"j.devries", "Julia de Vries", "editor", "Developers"},
	{"d.kowalski", "Dawid Kowalski", "editor", "Developers"},
	{"c.santos", "Camila Santos", "editor", "Developers"},
	{"n.haddad", "Nadia Haddad", "editor", "Developers"},
	{"b.andersen", "Bjorn Andersen", "editor", "Developers"},
	{"y.chen", "Yuki Chen", "editor", "Developers"},
	{"o.adeyemi", "Olu Adeyemi", "editor", "Developers"},
	{"f.rossi", "Francesca Rossi", "editor", "Developers"},
	{"h.muller", "Hannah Muller", "viewer", "Store Ops"},
	{"i.petrov", "Ivan Petrov", "viewer", "Store Ops"},
	{"g.dubois", "Gabrielle Dubois", "viewer", "Store Ops"},
	{"w.smit", "Willem Smit", "viewer", "Store Ops"},
	{"z.ahmed", "Zara Ahmed", "viewer", "Store Ops"},
	{"m.silva", "Mateus Silva", "viewer", "Store Ops"},
	{"e.novak", "Eva Novak", "viewer", "Developers"},
	{"auditor", "Compliance Auditor", "viewer", ""},
}

// demoGroups maps each seeded group to the preset role it grants.
var demoGroups = []struct {
	name, description, roleID string
}{
	{"Platform", "Owns the hosts, the agents and the shared platform stacks", "role_operator"},
	{"SRE", "On call for production; restarts, rollbacks and incident response", "role_operator"},
	{"Developers", "Ship application stacks to staging and production", "role_editor"},
	{"Store Ops", "Read-only view of the store and edge environments", "role_viewer"},
}

var presetRoleIDs = map[string]string{
	"operator": "role_operator",
	"editor":   "role_editor",
	"viewer":   "role_viewer",
}

// seedHistoryUsers adds the demo colleagues and their groups, and loads every
// account (real ones included, untouched) as an actor for the audit history.
func seedHistoryUsers(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	ok, err := tableExists(ctx, conn, "users")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	created := rfc(hc.now.Add(-hc.window - 30*24*time.Hour))
	err = withTx(ctx, conn, func(tx *sql.Tx) error {
		rows := make([][]any, 0, len(demoPeople))
		for i, p := range demoPeople {
			lastLogin := rfc(hc.anyTime(10 * 24 * time.Hour))
			rows = append(rows, []any{
				stableID("user", p.username), p.username, p.username + "@acme.example", p.name, p.role,
				lastLogin, fmt.Sprintf("10.20.%d.%d", 1+i/200, 10+i%200), created, created,
			})
		}
		return insertAll(ctx, tx, `
			INSERT OR IGNORE INTO users (id, username, email, password_hash, display_name, role, auth_provider,
			                             is_active, last_login, last_login_ip, created_at, updated_at)
			VALUES (?, ?, ?, '', ?, ?, 'local', 1, ?, ?, ?, ?)`, rows)
	})
	if err != nil {
		return err
	}
	if err := seedHistoryGroups(ctx, conn, created); err != nil {
		return err
	}
	return loadHistoryActors(ctx, conn, hc)
}

func seedHistoryGroups(ctx context.Context, conn *sql.DB, created string) error {
	ok, err := allTablesExist(ctx, conn, "groups", "group_members", "group_roles", "user_roles", "roles")
	if err != nil || !ok {
		return err
	}
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		groupIDs := make(map[string]string, len(demoGroups))
		for _, g := range demoGroups {
			if _, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO groups (id, name, description, is_system, created_at, updated_at)
				VALUES (?, ?, ?, 0, ?, ?)`, stableID("group", g.name), g.name, g.description, created, created); err != nil {
				return fmt.Errorf("adding group %s: %w", g.name, err)
			}
			var id string
			if err := tx.QueryRowContext(ctx, "SELECT id FROM groups WHERE name = ?", g.name).Scan(&id); err != nil {
				return fmt.Errorf("reading group %s: %w", g.name, err)
			}
			groupIDs[g.name] = id
			if roleExists(ctx, tx, g.roleID) {
				if _, err := tx.ExecContext(ctx, `
					INSERT OR IGNORE INTO group_roles (id, group_id, role_id, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?)`, stableID("group_role", g.name), id, g.roleID, created, created); err != nil {
					return fmt.Errorf("granting group %s its role: %w", g.name, err)
				}
			}
		}
		var members, grants [][]any
		// bounded by demoPeople, a fixed fixture list.
		for _, p := range demoPeople {
			userID := stableID("user", p.username)
			var hash string
			if err := tx.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", userID).Scan(&hash); err != nil || hash != "" {
				continue
			}
			if gid, ok := groupIDs[p.group]; ok {
				members = append(members, []any{stableID("group_member", p.username), gid, userID, created, created})
			}
			if p.group == "" {
				if roleID := presetRoleIDs[p.role]; roleExists(ctx, tx, roleID) {
					grants = append(grants, []any{stableID("user_role", p.username), userID, roleID, created, created})
				}
			}
		}
		if err := insertAll(ctx, tx, `
			INSERT OR IGNORE INTO group_members (id, group_id, user_id, source, created_at, updated_at)
			VALUES (?, ?, ?, 'manual', ?, ?)`, members); err != nil {
			return err
		}
		return insertAll(ctx, tx, `
			INSERT OR IGNORE INTO user_roles (id, user_id, role_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`, grants)
	})
}

func roleExists(ctx context.Context, tx *sql.Tx, id string) bool {
	var found string
	return id != "" && tx.QueryRowContext(ctx, "SELECT id FROM roles WHERE id = ?", id).Scan(&found) == nil
}

// loadHistoryActors reads every person account back. Real accounts act more
// often than any single colleague, so the history reads as their estate.
func loadHistoryActors(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	rows, err := conn.QueryContext(ctx, `
		SELECT id, username, password_hash != '' FROM users
		WHERE COALESCE(principal_kind, 'user') = 'user' AND username NOT LIKE '\_\_%' ESCAPE '\'
		ORDER BY created_at, id LIMIT 1000`)
	if err != nil {
		return fmt.Errorf("listing users: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	i := 0
	for rows.Next() {
		var a historyActor
		var hasPassword bool
		if err := rows.Scan(&a.id, &a.username, &hasPassword); err != nil {
			return fmt.Errorf("reading user: %w", err)
		}
		a.weight = 2
		if hasPassword {
			a.weight = 12
		}
		if strings.HasPrefix(a.username, "auditor") {
			a.weight = 1
		}
		a.ip = fmt.Sprintf("10.20.%d.%d", 1+i/200, 10+i%200)
		i++
		hc.actors = append(hc.actors, a)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating users: %w", err)
	}
	return nil
}
