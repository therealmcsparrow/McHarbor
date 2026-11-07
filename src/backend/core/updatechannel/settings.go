// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package updatechannel holds the prerelease switches and the policy that reads
// them, apart from the self-update settings struct so a stale whole-struct save
// can never overwrite a switch.
package updatechannel

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// category keeps these rows out of the self_update category that
// SelfUpdateChecker.Settings reads and SaveSettings rewrites.
const category = "update_channels"

// Settings is the stored channel state; every field reads off on a fresh install.
type Settings struct {
	McHarborPrereleases     bool `json:"mcharborPrereleases"`
	AgentPrereleases        bool `json:"agentPrereleases"`
	PluginPrereleases       bool `json:"pluginPrereleases"`
	McHarborAutoPrereleases bool `json:"mcharborAutoPrereleases"`
	AgentAutoPrereleases    bool `json:"agentAutoPrereleases"`
}

// Patch carries only the fields a client sent; a nil field stays as stored.
type Patch struct {
	McHarborPrereleases     *bool `json:"mcharborPrereleases,omitempty"`
	AgentPrereleases        *bool `json:"agentPrereleases,omitempty"`
	PluginPrereleases       *bool `json:"pluginPrereleases,omitempty"`
	McHarborAutoPrereleases *bool `json:"mcharborAutoPrereleases,omitempty"`
	AgentAutoPrereleases    *bool `json:"agentAutoPrereleases,omitempty"`
}

// Change is one switch that moved, for the audit row.
type Change struct {
	Key string `json:"key"`
	Old bool   `json:"old"`
	New bool   `json:"new"`
}

// field binds a JSON name and settings key to its Settings and Patch members.
type field struct {
	name  string
	key   string
	get   func(*Settings) *bool
	patch func(*Patch) *bool
}

// fields is the allow-list of keys this package ever writes.
var fields = []field{
	{"mcharborPrereleases", "update_channels_mcharbor", func(s *Settings) *bool { return &s.McHarborPrereleases }, func(p *Patch) *bool { return p.McHarborPrereleases }},
	{"agentPrereleases", "update_channels_agents", func(s *Settings) *bool { return &s.AgentPrereleases }, func(p *Patch) *bool { return p.AgentPrereleases }},
	{"pluginPrereleases", "update_channels_plugins", func(s *Settings) *bool { return &s.PluginPrereleases }, func(p *Patch) *bool { return p.PluginPrereleases }},
	{"mcharborAutoPrereleases", "update_channels_mcharbor_auto", func(s *Settings) *bool { return &s.McHarborAutoPrereleases }, func(p *Patch) *bool { return p.McHarborAutoPrereleases }},
	{"agentAutoPrereleases", "update_channels_agents_auto", func(s *Settings) *bool { return &s.AgentAutoPrereleases }, func(p *Patch) *bool { return p.AgentAutoPrereleases }},
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func read(ctx context.Context, q querier) (Settings, error) {
	var s Settings
	rows, err := q.QueryContext(ctx, `SELECT key, value FROM settings WHERE category = ? LIMIT 1000`, category)
	if err != nil {
		return s, fmt.Errorf("loading update channel settings: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	for rows.Next() {
		var key string
		var value sql.NullString
		if err := rows.Scan(&key, &value); err != nil {
			return s, fmt.Errorf("scanning update channel setting: %w", err)
		}
		for _, f := range fields {
			if f.key == key {
				*f.get(&s) = value.String == "true"
			}
		}
	}
	if err := rows.Err(); err != nil {
		return s, fmt.Errorf("iterating update channel settings: %w", err)
	}
	return s, nil
}

// Load returns the stored channel state, all off when nothing was ever saved.
func Load(ctx context.Context, db *sql.DB) (Settings, error) {
	return read(ctx, db)
}

// Apply writes only the fields set in p inside one transaction and returns the
// state before and after, so two saves cannot lose each other's switch.
func Apply(ctx context.Context, db *sql.DB, p Patch) (before, after Settings, err error) {
	err = coredb.RetryOnBusy(ctx, 5, func() error {
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("beginning update channel save: %w", txErr)
		}
		defer func() { _ = tx.Rollback() }() //nolint:errcheck // rollback after a commit returns ErrTxDone by design
		b, rErr := read(ctx, tx)
		if rErr != nil {
			return rErr
		}
		a := b
		now := time.Now().UTC().Format(time.RFC3339)
		for _, f := range fields {
			v := f.patch(&p)
			if v == nil {
				continue
			}
			*f.get(&a) = *v
			val := "false"
			if *v {
				val = "true"
			}
			if _, eErr := tx.ExecContext(ctx,
				`INSERT INTO settings (id, category, key, value, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?)
				 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
				xid.New().String(), category, f.key, val, now, now); eErr != nil {
				return fmt.Errorf("saving update channel setting %s: %w", f.key, eErr)
			}
		}
		if cErr := tx.Commit(); cErr != nil {
			return fmt.Errorf("committing update channel save: %w", cErr)
		}
		before, after = b, a
		return nil
	})
	return before, after, err
}

// Diff lists the switches whose value differs between before and after.
func Diff(before, after Settings) []Change {
	var out []Change
	for _, f := range fields {
		o, n := *f.get(&before), *f.get(&after)
		if o != n {
			out = append(out, Change{Key: f.name, Old: o, New: n})
		}
	}
	return out
}
