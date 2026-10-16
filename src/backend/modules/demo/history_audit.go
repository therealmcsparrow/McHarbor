// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
)

// auditRow is one seeded audit entry before it is sealed into the chain.
type auditRow struct {
	id, action, severity, timestamp  string
	userID, username, ip             *string
	entityType, entityID, entityName *string
	details, envID, envName          *string
}

// auditKind is one family of audited actions with its relative frequency.
type auditKind struct {
	weight int
	build  func(hc *historyCtx, r *auditRow)
}

var auditKinds = []auditKind{
	{30, auditContainer},
	{12, auditStack},
	{8, auditImage},
	{13, auditLogin},
	{4, auditAutoheal},
	{3, auditVolumeNetwork},
	{3, auditSettings},
	{3, auditWorkflow},
	{2, auditEnvironment},
	{2, auditUserAdmin},
	{1, auditSchedule},
	{1, auditAlert},
}

// seedHistoryAudit writes the audit history as one transaction and seals it
// into the HMAC chain, so the audit verifier still reports the chain intact.
func seedHistoryAudit(ctx context.Context, conn *sql.DB, hc *historyCtx) error {
	ok, err := tableExists(ctx, conn, "audit_logs")
	if err != nil || !ok {
		return err
	}
	var exists int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs WHERE id = ?", stableID("audit", "0")).Scan(&exists); err != nil {
		return fmt.Errorf("checking earlier audit seed: %w", err)
	}
	if exists > 0 {
		return nil
	}
	rows := buildAuditRows(hc)
	sealed := auditChainWritable(ctx, conn)
	// The logger signs with the instance secret from the loaded config; main removes
	// MCHARBOR_SECRET from the environment once config has loaded.
	secret := hc.secret
	return withTx(ctx, conn, func(tx *sql.Tx) error {
		if err := insertAuditRows(ctx, tx, rows); err != nil {
			return err
		}
		if !sealed {
			return nil
		}
		return resealAuditChain(ctx, tx, secret)
	})
}

// auditChainWritable reports whether the chain is on and rows may be re-sealed:
// with WORM on, rows already written cannot be updated, so the history is
// written unsealed, which the verifier skips rather than failing on.
func auditChainWritable(ctx context.Context, conn *sql.DB) bool {
	var chain, worm sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'audit.hash_chain.enabled'").Scan(&chain); err != nil && err != sql.ErrNoRows {
		return false
	}
	if err := conn.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'audit.worm'").Scan(&worm); err != nil && err != sql.ErrNoRows {
		return false
	}
	return !strings.EqualFold(chain.String, "false") && !strings.EqualFold(worm.String, "on")
}

func buildAuditRows(hc *historyCtx) []auditRow {
	total := 0
	for _, k := range auditKinds {
		total += k.weight
	}
	rows := make([]auditRow, 0, hc.auditRows)
	for i := 0; i < hc.auditRows; i++ {
		r := auditRow{id: stableID("audit", fmt.Sprint(i)), severity: "info"}
		n := hc.rng.IntN(total)
		for _, k := range auditKinds {
			if n < k.weight {
				k.build(hc, &r)
				break
			}
			n -= k.weight
		}
		if r.timestamp == "" {
			r.timestamp = rfc(hc.busyTime())
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].timestamp == rows[j].timestamp {
			return rows[i].id < rows[j].id
		}
		return rows[i].timestamp < rows[j].timestamp
	})
	return rows
}

func insertAuditRows(ctx context.Context, tx *sql.Tx, rows []auditRow) error {
	args := make([][]any, 0, len(rows))
	for _, r := range rows {
		args = append(args, []any{r.id, r.userID, r.username, r.action, r.severity, r.entityType, r.entityID,
			r.entityName, r.details, r.ip, r.envID, r.envName, r.timestamp, r.timestamp, r.timestamp})
	}
	return insertAll(ctx, tx, `
		INSERT INTO audit_logs (id, user_id, username, action, severity, entity_type, entity_id, entity_name,
		                        details, ip_address, environment_id, environment_name, timestamp, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args)
}

// auditFix is one chain link that must be rewritten.
type auditFix struct{ id, prev, hash string }

// resealAuditChain walks the whole log in verifier order and rewrites every
// link that no longer follows its predecessor. The seeded rows predate what
// this boot already logged, so those later rows are re-linked behind them.
func resealAuditChain(ctx context.Context, tx *sql.Tx, secret string) error {
	fixes, err := auditChainFixes(ctx, tx, secret)
	if err != nil {
		return err
	}
	args := make([][]any, 0, len(fixes))
	for _, f := range fixes {
		args = append(args, []any{nullIfBlank(f.prev), f.hash, f.id})
	}
	return insertAll(ctx, tx, "UPDATE audit_logs SET prev_hash = ?, row_hash = ? WHERE id = ?", args)
}

func auditChainFixes(ctx context.Context, tx *sql.Tx, secret string) ([]auditFix, error) {
	// No LIMIT: the demo audit chain is re-sealed end to end, and a partial read would break the hashes.
	rows, err := tx.QueryContext(ctx, `
		SELECT id, user_id, username, action, severity, entity_type, entity_id, entity_name,
		       details, state_before, state_after, changed_keys, ip_address, environment_id,
		       api_key_id, timestamp, created_at, updated_at, COALESCE(prev_hash, ''), COALESCE(row_hash, '')
		FROM audit_logs
		ORDER BY timestamp ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("reading audit chain: %w", err)
	}
	defer func() { logCloseErr(rows.Close()) }()
	var fixes []auditFix
	prev := ""
	for rows.Next() {
		var h audit.HashRow
		var userID, username, entityType, entityID, entityName, details sql.NullString
		var before, after, changed, ip, envID, apiKey sql.NullString
		var storedPrev, storedHash string
		if err := rows.Scan(&h.ID, &userID, &username, &h.Action, &h.Severity, &entityType, &entityID, &entityName,
			&details, &before, &after, &changed, &ip, &envID, &apiKey, &h.Timestamp, &h.CreatedAt, &h.UpdatedAt,
			&storedPrev, &storedHash); err != nil {
			return nil, fmt.Errorf("scanning audit row: %w", err)
		}
		h.UserID, h.Username, h.EntityType = strPtr(userID), strPtr(username), strPtr(entityType)
		h.EntityID, h.EntityName, h.Details = strPtr(entityID), strPtr(entityName), strPtr(details)
		h.StateBefore, h.StateAfter, h.ChangedKeys = strPtr(before), strPtr(after), strPtr(changed)
		h.IPAddress, h.EnvironmentID, h.APIKeyID = strPtr(ip), strPtr(envID), strPtr(apiKey)
		h.PrevHash = prev
		hash, err := audit.ComputeRowHash(secret, h)
		if err != nil {
			return nil, err
		}
		if storedPrev != prev || storedHash != hash {
			fixes = append(fixes, auditFix{h.ID, prev, hash})
		}
		prev = hash
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit chain: %w", err)
	}
	return fixes, nil
}

func strPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func nullIfBlank(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func sp(s string) *string { return &s }

func (hc *historyCtx) actAs(r *auditRow) {
	a := hc.pickActor()
	if a.id == "" {
		return
	}
	r.userID, r.username, r.ip = sp(a.id), sp(a.username), sp(a.ip)
}

func (hc *historyCtx) onHost(r *auditRow, h *historyHost) {
	r.envID, r.envName = sp(h.envID), sp(h.host.Name)
}

// maybeFail turns roughly p of the entries into failures, with the reason a
// real handler would have recorded.
func (hc *historyCtx) maybeFail(r *auditRow, p float64, reasons ...string) {
	if len(reasons) == 0 || hc.rng.Float64() >= p {
		return
	}
	r.severity = "error"
	r.details = sp("failed: " + reasons[hc.rng.IntN(len(reasons))])
}

func pick[T any](hc *historyCtx, xs []T) T { return xs[hc.rng.IntN(len(xs))] }

func auditContainer(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	c, ok := hc.pickContainer(h)
	if !ok {
		auditLogin(hc, r)
		return
	}
	hc.actAs(r)
	hc.onHost(r, h)
	r.action = pick(hc, []string{"restart", "restart", "restart", "start", "start", "stop", "stop", "update", "recreate", "kill", "rename", "pause", "unpause", "create", "delete"})
	switch r.action {
	case "stop", "kill", "delete", "pause":
		r.severity = "warning"
	case "start", "create":
		r.severity = "success"
	}
	r.entityType, r.entityID, r.entityName = sp("container"), sp(c.ID), sp(c.Name)
	hc.maybeFail(r, 0.03, "container is marked as protected", "timeout waiting for container to stop", "conflict: container is already running", "agent did not answer within 30s")
}

func auditStack(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	if len(h.stacks) == 0 {
		auditContainer(hc, r)
		return
	}
	hc.actAs(r)
	hc.onHost(r, h)
	name := pick(hc, h.stacks)
	r.action = pick(hc, []string{"up", "up", "restart", "update", "update", "stop", "down", "reinstall", "reveal_env_vars"})
	if r.action == "stop" || r.action == "down" {
		r.severity = "warning"
	}
	r.entityType, r.entityID, r.entityName = sp("stack"), sp(stableID("stack", h.host.Key+"/"+name)), sp(name)
	hc.maybeFail(r, 0.05, "compose: service \"api\" failed to build", "image pull timed out", "port 443 is already allocated")
}

func auditImage(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	c, ok := hc.pickContainer(h)
	if !ok {
		auditLogin(hc, r)
		return
	}
	hc.actAs(r)
	hc.onHost(r, h)
	r.action = pick(hc, []string{"pull", "pull", "pull", "delete", "tag", "prune"})
	r.entityType = sp("image")
	if r.action == "prune" {
		r.severity = "warning"
		r.details = sp(fmt.Sprintf("reclaimed %d MB", 120+hc.rng.IntN(4000)))
		return
	}
	r.entityID, r.entityName = sp(c.ImageID), sp(c.Image)
	if r.action == "delete" {
		r.severity = "warning"
	}
	hc.maybeFail(r, 0.04, "manifest unknown", "image is in use by a running container", "toomanyrequests: rate limit exceeded")
}

func auditLogin(hc *historyCtx, r *auditRow) {
	a := hc.pickActor()
	if a.id == "" {
		r.action, r.entityType = "audit.verified.scheduled", sp("audit_log")
		return
	}
	r.userID, r.username, r.ip = sp(a.id), sp(a.username), sp(a.ip)
	r.entityType, r.entityID, r.entityName = sp("user"), sp(a.id), sp(a.username)
	r.action, r.severity = "login", "success"
	if hc.rng.Float64() < 0.12 {
		r.action, r.severity = "login_failed", "error"
		r.details = sp("invalid credentials")
	}
}

func auditAutoheal(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	c, ok := hc.pickContainer(h)
	if !ok {
		return
	}
	hc.onHost(r, h)
	r.action, r.severity = "autoheal", "warning"
	r.entityType, r.entityID, r.entityName = sp("container"), sp(c.ID), sp(c.Name)
	r.details = sp(pick(hc, []string{"restarted after 3 failed health checks", "restarted: exited with code 137", "restarted: exited with code 1"}))
}

func auditVolumeNetwork(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	hc.actAs(r)
	hc.onHost(r, h)
	name := h.host.Key
	if len(h.stacks) > 0 {
		name = pick(hc, h.stacks)
	}
	if hc.rng.IntN(2) == 0 {
		r.entityType, r.entityName = sp("volume"), sp(name+"_data")
		r.action = pick(hc, []string{"create", "delete", "prune"})
	} else {
		r.entityType, r.entityName = sp("network"), sp(name+"_default")
		r.action = pick(hc, []string{"create", "create", "delete"})
	}
	if r.action == "delete" || r.action == "prune" {
		r.severity = "warning"
	}
}

func auditSettings(hc *historyCtx, r *auditRow) {
	hc.actAs(r)
	r.action, r.entityType = "update", sp("settings")
	r.entityName = sp(pick(hc, []string{"general", "notifications", "security", "retention", "updates", "appearance", "authentication"}))
}

func auditWorkflow(hc *historyCtx, r *auditRow) {
	hc.actAs(r)
	t := pick(hc, workflowTemplates)
	r.entityType, r.entityID, r.entityName = sp("workflow"), sp(stableID("workflow", t.name)), sp(t.name)
	r.action = pick(hc, []string{"update", "update", "update", "create", "run", "workflow.exported"})
}

func auditEnvironment(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	hc.actAs(r)
	hc.onHost(r, h)
	r.action = pick(hc, []string{"update", "update", "update", "create"})
	r.entityType, r.entityID, r.entityName = sp("environment"), sp(h.envID), sp(h.host.Name)
}

func auditUserAdmin(hc *historyCtx, r *auditRow) {
	hc.actAs(r)
	target := hc.pickActor()
	r.entityType, r.entityID, r.entityName = sp("user"), sp(target.id), sp(target.username)
	r.action = pick(hc, []string{"update", "assign_role", "unassign_role", "user.session.revoke", "user.unlock", "create", "add_member"})
	if r.action == "add_member" {
		r.entityType, r.entityName = sp("group"), sp(pick(hc, demoGroups).name)
	}
}

func auditSchedule(hc *historyCtx, r *auditRow) {
	h := hc.pickHost()
	hc.actAs(r)
	hc.onHost(r, h)
	r.action = pick(hc, []string{"create", "update", "update", "delete"})
	r.entityType, r.entityName = sp("schedule"), sp("Nightly restart "+h.host.Key)
}

func auditAlert(hc *historyCtx, r *auditRow) {
	hc.actAs(r)
	t := pick(hc, alertTemplates)
	r.action = pick(hc, []string{"alert.created", "alert.updated", "alert.updated", "alert.tested"})
	r.entityType, r.entityName = sp("alert"), sp(t.name)
}
