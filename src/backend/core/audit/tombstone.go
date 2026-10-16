// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"
)

// PrunedRow is the part of an expiring audit row a tombstone needs: its place in
// the chain, not its content.
type PrunedRow struct {
	ID        string
	Severity  string
	Timestamp string
	PrevHash  string
	RowHash   string
}

// Tombstone stands in for a contiguous run of pruned chain links: the verifier
// may step from EntryHash to ExitHash, and Seal proves retention wrote the step.
type Tombstone struct {
	ID             string `json:"id"`
	EntryHash      string `json:"entryHash"`
	ExitHash       string `json:"exitHash"`
	RowCount       int64  `json:"rowCount"`
	FirstRowID     string `json:"firstRowId"`
	LastRowID      string `json:"lastRowId"`
	FirstTimestamp string `json:"firstTimestamp"`
	LastTimestamp  string `json:"lastTimestamp"`
	Severities     string `json:"severities"`
	CreatedAt      string `json:"createdAt"`
	Seal           string `json:"-"`
}

// SealTombstone returns the HMAC over every field but the seal, with the same
// secret as row_hash, so a hand-written tombstone cannot hide a deletion.
func SealTombstone(secret string, t Tombstone) (string, error) {
	payload, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("marshal audit tombstone: %w", err)
	}
	mac := hmac.New(sha256.New, []byte("audit-chain-tombstone:"+secret))
	if _, err := mac.Write(payload); err != nil {
		return "", fmt.Errorf("seal audit tombstone: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// PlanTombstones groups the sealed rows among victims into maximal runs linked
// by prev_hash and returns one sealed tombstone per run; unsealed rows need none.
func PlanTombstones(secret string, victims []PrunedRow, now time.Time) ([]Tombstone, error) {
	byPrev := map[string]PrunedRow{}
	deletedHashes := map[string]bool{}
	sealed := []PrunedRow{}
	for _, v := range victims {
		if v.RowHash == "" {
			continue
		}
		sealed = append(sealed, v)
		byPrev[v.PrevHash] = v
		deletedHashes[v.RowHash] = true
	}
	created := now.UTC().Format(time.RFC3339)
	tombstones := []Tombstone{}
	for _, head := range sealed {
		if deletedHashes[head.PrevHash] {
			continue
		}
		t := Tombstone{
			ID:             xid.New().String(),
			EntryHash:      head.PrevHash,
			FirstRowID:     head.ID,
			FirstTimestamp: head.Timestamp,
			CreatedAt:      created,
		}
		counts := map[string]int64{}
		current := head
		visited := map[string]bool{}
		for {
			visited[current.ID] = true
			t.RowCount++
			counts[current.Severity]++
			t.ExitHash = current.RowHash
			t.LastRowID = current.ID
			t.LastTimestamp = current.Timestamp
			next, ok := byPrev[current.RowHash]
			if !ok || visited[next.ID] {
				break
			}
			current = next
		}
		t.Severities = severitySummary(counts)
		seal, err := SealTombstone(secret, t)
		if err != nil {
			return nil, err
		}
		t.Seal = seal
		tombstones = append(tombstones, t)
	}
	return tombstones, nil
}

func severitySummary(counts map[string]int64) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+":"+strconv.FormatInt(counts[k], 10))
	}
	return strings.Join(parts, ",")
}

// ChainBridge lets a chain walker cross gaps retention left behind, and only
// through tombstones whose seal verifies under the audit secret.
type ChainBridge struct {
	// byEntry is a slice per hash because a pruned tail and a later run pruned
	// after new rows linked to the same survivor can share an entry hash.
	byEntry map[string][]Tombstone
	// Total and Invalid count the tombstones loaded and those whose seal failed.
	Total   int64
	Invalid int64
}

// Cross follows sealed tombstones from the hash the walker last saw to the
// prev_hash of the next surviving row, returning the pruned rows it stepped over.
func (b *ChainBridge) Cross(from, to string) (int64, bool) {
	if b == nil || from == to {
		return 0, false
	}
	return b.cross(from, to, map[string]bool{})
}

func (b *ChainBridge) cross(from, to string, seen map[string]bool) (int64, bool) {
	if seen[from] {
		return 0, false
	}
	seen[from] = true
	for _, t := range b.byEntry[from] {
		if t.ExitHash == to {
			return t.RowCount, true
		}
		if rows, ok := b.cross(t.ExitHash, to, seen); ok {
			return t.RowCount + rows, true
		}
	}
	return 0, false
}

// NewChainBridge indexes tombstones by entry hash, discarding any whose seal does
// not verify so a forged tombstone reads as the break it is covering.
func NewChainBridge(secret string, tombstones []Tombstone) *ChainBridge {
	b := &ChainBridge{byEntry: map[string][]Tombstone{}}
	for _, t := range tombstones {
		b.Total++
		want, err := SealTombstone(secret, t)
		if err != nil || !hmac.Equal([]byte(want), []byte(t.Seal)) {
			b.Invalid++
			continue
		}
		b.byEntry[t.EntryHash] = append(b.byEntry[t.EntryHash], t)
	}
	return b
}

// LoadChainBridge reads every tombstone; a database that predates migration 094
// yields an empty bridge rather than an error, since it can hold no gaps to cross.
func LoadChainBridge(ctx context.Context, db *sql.DB, secret string) (*ChainBridge, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, entry_hash, exit_hash, row_count, first_row_id, last_row_id,
		       first_timestamp, last_timestamp, severities, seal, created_at
		FROM audit_chain_tombstones
		ORDER BY first_timestamp ASC
		LIMIT 1000000
	`)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "no such table") || strings.Contains(lower, "does not exist") {
			return NewChainBridge(secret, nil), nil
		}
		return nil, fmt.Errorf("reading audit chain tombstones: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close audit rows", "error", err)
		}
	}()
	tombstones := []Tombstone{}
	for rows.Next() {
		var t Tombstone
		if err := rows.Scan(&t.ID, &t.EntryHash, &t.ExitHash, &t.RowCount, &t.FirstRowID, &t.LastRowID,
			&t.FirstTimestamp, &t.LastTimestamp, &t.Severities, &t.Seal, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit chain tombstone: %w", err)
		}
		tombstones = append(tombstones, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit chain tombstones: %w", err)
	}
	return NewChainBridge(secret, tombstones), nil
}
