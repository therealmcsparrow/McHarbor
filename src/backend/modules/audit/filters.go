// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"log/slog"
	"strings"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
)

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// likeEscape escapes a value for a LIKE pattern using '!' as the escape
// character, which behaves the same on SQLite and PostgreSQL.
func likeEscape(value string) string {
	r := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return r.Replace(value)
}

// actorClause matches the actor by username, by user id, by any id the username
// resolved to, and optionally by `user_id=<id>` inside details.
func actorClause(filter listFilter) (string, []any) {
	actor := strings.TrimSpace(filter.Actor)
	if actor == "" {
		return "", nil
	}
	parts := []string{"LOWER(COALESCE(al.username, '')) = ?", "al.user_id = ?"}
	args := []any{strings.ToLower(actor), actor}
	ids := uniqueStrings(append([]string{actor}, filter.ActorIDs...))
	if len(filter.ActorIDs) > 0 {
		parts = append(parts, "al.user_id IN ("+placeholders(len(filter.ActorIDs))+")")
		for _, id := range filter.ActorIDs {
			args = append(args, id)
		}
	}
	if filter.ActorInDetails {
		for _, id := range ids {
			parts = append(parts, "al.details LIKE ? ESCAPE '!'")
			args = append(args, "%user!_id="+likeEscape(id)+"%")
		}
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// resolveActorIDs finds the user ids an actor filter names, so a filter by
// username also matches rows whose stored username predates a rename.
func (s *Service) resolveActorIDs(ctx context.Context, actor string) []string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM users WHERE LOWER(username) = ? OR id = ? LIMIT 10", strings.ToLower(actor), actor)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("audit actor lookup failed", "error", err)
		}
		return nil
	}
	defer func() { logRowsClose(rows.Close()) }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	logRowsClose(rows.Err())
	return ids
}

// parseSeverities validates a repeatable severity filter against the stored set.
func parseSeverities(raw []string) ([]string, bool) {
	out := []string{}
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part == "" {
				continue
			}
			if !coreAudit.ValidSeverity(part) {
				return nil, false
			}
			out = append(out, part)
		}
	}
	return uniqueStrings(out), true
}

// usernamesByID resolves display names for rows written without one.
func (s *Service) usernamesByID(ctx context.Context, ids []string) map[string]string {
	names := map[string]string{}
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return names
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, username FROM users WHERE id IN ("+placeholders(len(ids))+") LIMIT 1000", args...) // SQL identifier: placeholders() emits only "?" markers
	if err != nil {
		return names
	}
	defer func() { logRowsClose(rows.Close()) }()
	for rows.Next() {
		var id, username string
		if err := rows.Scan(&id, &username); err == nil {
			names[id] = username
		}
	}
	logRowsClose(rows.Err())
	return names
}

// logRowsClose reports a failed rows.Close through slog, because a deferred close has no caller to return the error to.
func logRowsClose(err error) {
	if err != nil {
		slog.Warn("closing sql rows failed", "error", err)
	}
}
