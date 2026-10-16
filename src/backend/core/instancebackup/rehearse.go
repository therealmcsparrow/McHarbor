// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

// Rehearsal is what standing an export up in isolation found. A backup nobody
// has restored is a hypothesis; this is what turns it into a fact.
type Rehearsal struct {
	OK            bool          `json:"ok"`
	Tables        int           `json:"tables"`
	Rows          int64         `json:"rows"`
	Mismatches    []string      `json:"mismatches"`
	ChainVerified bool          `json:"chainVerified"`
	ChainRows     int64         `json:"chainRows"`
	Anchors       int           `json:"anchors"`
	Problem       string        `json:"problem,omitempty"`
	Duration      time.Duration `json:"durationNs"`
}

// Rehearse restores an export into a throwaway SQLite database and data
// directory under scratch, then checks it: every table's rows against the
// manifest, and the audit chain against the export's own secret. The live
// instance is never opened.
func Rehearse(ctx context.Context, scratch string, open func() (io.ReadCloser, error), s Secret) (Rehearsal, error) {
	started := time.Now()
	out := Rehearsal{Mismatches: []string{}}
	dir, err := os.MkdirTemp(scratch, "rehearsal-")
	if err != nil {
		return out, fmt.Errorf("preparing the rehearsal: %w", err)
	}
	defer func() { logCleanup("rehearsal directory", os.RemoveAll(dir)) }()

	database, err := coredb.OpenSQLite(filepath.Join(dir, "rehearsal.db")) //nolint:contextcheck // OpenSQLite is plugin-linked API without a ctx parameter
	if err != nil {
		return out, fmt.Errorf("opening the rehearsal database: %w", err)
	}
	defer func() { logCleanup("database", database.Close()) }()
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil { //nolint:contextcheck // Migrate is plugin-linked API without a ctx parameter
		return out, fmt.Errorf("preparing the rehearsal schema: %w", err)
	}
	// The throwaway target gets random key material, so no rehearsal database is ever
	// encrypted under a key or secret anyone could know in advance.
	fresh := make([]byte, 48)
	if _, err := rand.Read(fresh); err != nil {
		return out, fmt.Errorf("preparing the rehearsal key: %w", err)
	}
	t := Target{DB: database, Driver: coredb.DriverSQLite, DataDir: dir, EncryptionKey: fresh[:32], Secret: hex.EncodeToString(fresh[32:])}
	res, err := Restore(ctx, t, open, RestoreOptions{
		Secret: s, Mode: ModeFull, IncludeAudit: true, Actor: "rehearsal", Via: "rehearsal",
		Emit: func(context.Context, Intent) error { return nil },
	})
	if err != nil {
		out.Problem = err.Error()
		out.Duration = time.Since(started)
		return out, nil
	}
	out.Tables, out.Rows = res.Tables, res.Rows

	// bounded by the tables listed in the artifact manifest.
	for _, tbl := range res.Plan.Manifest.Tables {
		lower := strings.ToLower(tbl.Name)
		if InstallerOwned[lower] {
			continue
		}
		var n int64
		if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+coredb.QuoteIdent(coredb.DriverSQLite, tbl.Name)).Scan(&n); err != nil {
			out.Mismatches = append(out.Mismatches, fmt.Sprintf("%s: missing after restore", tbl.Name))
			continue
		}
		want := tbl.Rows
		if lower == "audit_logs" {
			want++
		}
		// The rehearsal commits its own restore marker into settings, replacing
		// one the source may carry from a restore of its own.
		if lower == "settings" && n == tbl.Rows+1 {
			want = n
		}
		if n != want {
			out.Mismatches = append(out.Mismatches, fmt.Sprintf("%s: %d rows, the manifest lists %d", tbl.Name, n, tbl.Rows))
		}
	}
	secret, err := os.ReadFile(filepath.Join(dir, ".secret"))
	if err != nil {
		out.Problem = "the restored audit secret is missing"
	} else {
		verified, rows, anchors, problem := verifyChain(ctx, database, string(secret))
		out.ChainVerified, out.ChainRows, out.Anchors = verified, rows, anchors
		if !verified {
			out.Problem = problem
		}
	}
	out.OK = out.Problem == "" && len(out.Mismatches) == 0 && out.ChainVerified
	out.Duration = time.Since(started)
	return out, nil
}

// verifyChain walks audit_logs as the audit module's verifier does. It lives
// here too because a rehearsal runs with no server and no module wiring.
func verifyChain(ctx context.Context, database *sql.DB, secret string) (bool, int64, int, string) {
	// Retention prunes the oldest rows and seals what it removed, so a restored
	// chain legitimately starts or jumps where a tombstone vouches for the gap.
	bridge, err := audit.LoadChainBridge(ctx, database, secret)
	if err != nil {
		return false, 0, 0, "reading the restored audit tombstones: " + err.Error()
	}
	// No LIMIT: the restored audit chain is verified end to end.
	rows, err := database.QueryContext(ctx, `SELECT id, user_id, username, action, severity, entity_type, entity_id, entity_name,
		details, state_before, state_after, changed_keys, ip_address, environment_id, api_key_id,
		timestamp, created_at, updated_at, prev_hash, row_hash FROM audit_logs ORDER BY timestamp ASC, id ASC`)
	if err != nil {
		return false, 0, 0, "reading the restored audit log: " + err.Error()
	}
	defer func() { logCleanup("rows", rows.Close()) }()
	prev := ""
	var n int64
	anchors := 0
	for rows.Next() {
		var r audit.HashRow
		var userID, username, entityType, entityID, entityName, details, before, after, keys, ip, env, apiKey, prevHash, rowHash sql.NullString
		if err := rows.Scan(&r.ID, &userID, &username, &r.Action, &r.Severity, &entityType, &entityID, &entityName,
			&details, &before, &after, &keys, &ip, &env, &apiKey, &r.Timestamp, &r.CreatedAt, &r.UpdatedAt, &prevHash, &rowHash); err != nil {
			return false, n, anchors, "reading the restored audit log: " + err.Error()
		}
		if rowHash.String == "" {
			continue
		}
		r.UserID, r.Username, r.EntityType, r.EntityID, r.EntityName = ptr(userID), ptr(username), ptr(entityType), ptr(entityID), ptr(entityName)
		r.Details, r.StateBefore, r.StateAfter, r.ChangedKeys, r.IPAddress = ptr(details), ptr(before), ptr(after), ptr(keys), ptr(ip)
		r.EnvironmentID, r.APIKeyID = ptr(env), ptr(apiKey)
		if prevHash.String != prev {
			if _, bridged := bridge.Cross(prev, prevHash.String); !bridged {
				return false, n, anchors, fmt.Sprintf("audit row %s does not link to the row before it", r.ID)
			}
		}
		r.PrevHash = prevHash.String
		want, err := audit.ComputeRowHash(secret, r)
		if err != nil || want != rowHash.String {
			return false, n, anchors, fmt.Sprintf("audit row %s does not verify", r.ID)
		}
		if r.Action == AnchorAction {
			anchors++
		}
		prev = rowHash.String
		n++
	}
	if err := rows.Err(); err != nil {
		return false, n, anchors, "reading the restored audit log: " + err.Error()
	}
	return true, n, anchors, ""
}

func ptr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
