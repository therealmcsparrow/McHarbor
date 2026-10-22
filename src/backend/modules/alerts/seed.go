// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// defaultDiskRulesKey is the settings marker that makes the default-rule seed run
// once, so a rule the operator later deletes is not brought back.
const defaultDiskRulesKey = "alerts.default_disk_rules_seeded"

// Default host disk thresholds, used when no enabled host_fs_percent rule covers
// an environment.
const (
	DefaultDiskWarningPercent  = 85.0
	DefaultDiskCriticalPercent = 95.0
)

type defaultRule struct {
	name       string
	severity   string
	condition  string
	cfg        ConditionConfig
	cooldown   int
	recoveryOn bool
}

func defaultDiskRules() []defaultRule {
	return []defaultRule{
		{"Host disk usage warning", "warning", conditionHostFSPercent,
			ConditionConfig{Operator: ">=", Threshold: DefaultDiskWarningPercent, Unit: "%", ForSeconds: 300}, 0, true},
		{"Host disk usage critical", "critical", conditionHostFSPercent,
			ConditionConfig{Operator: ">=", Threshold: DefaultDiskCriticalPercent, Unit: "%"}, 0, true},
		{"Host disk data missing", "warning", conditionHostFSStale,
			ConditionConfig{ForSeconds: 600}, 0, true},
	}
}

// SeedDefaultRules creates the three default disk rules once, in one transaction
// with the marker, and attaches every enabled channel at critical severity. It runs
// at boot as the system, so a concurrent second boot sees the marker and does nothing.
func (s *Service) SeedDefaultRules(ctx context.Context) error {
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339)
		// Claiming the marker first makes the seed single-winner on every driver: a
		// second node booting at the same time inserts nothing and returns.
		claim, err := tx.ExecContext(ctx,
			`INSERT INTO settings (id, category, key, value, created_at, updated_at)
			 VALUES (?, 'alerts', ?, '1', ?, ?)
			 ON CONFLICT(key) DO NOTHING`,
			xid.New().String(), defaultDiskRulesKey, now, now)
		if err != nil {
			return fmt.Errorf("claiming %s: %w", defaultDiskRulesKey, err)
		}
		if db.RowsAffected(claim) == 0 {
			return nil
		}

		channels, err := enabledChannelIDs(ctx, tx)
		if err != nil {
			return err
		}
		destinations := make([]DestinationInput, 0, len(channels))
		for _, id := range channels {
			destinations = append(destinations, DestinationInput{ChannelID: id, MinSeverity: "critical"})
		}

		for _, rule := range defaultDiskRules() {
			encoded, err := encodeConditionConfig(rule.cfg)
			if err != nil {
				return err
			}
			id := xid.New().String()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO alerts (id, name, severity, type, condition, source_type, condition_key,
				                    condition_config, target, environment_ids, cooldown_seconds,
				                    notify_on_recovery, message_template, send_in_app, enabled,
				                    created_at, updated_at)
				VALUES (?, ?, ?, ?, '', ?, ?, ?, '', '', ?, ?, '', 1, 1, ?, ?)`,
				id, rule.name, rule.severity, rule.condition, sourceHost, rule.condition, encoded,
				rule.cooldown, rule.recoveryOn, now, now); err != nil {
				return fmt.Errorf("seeding alert %q: %w", rule.name, err)
			}
			if err := replaceDestinations(ctx, tx, id, destinations, now); err != nil {
				return err
			}
		}

		return nil
	})
}

func enabledChannelIDs(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM communication_channels WHERE enabled = 1 ORDER BY name ASC LIMIT 1000")
	if err != nil {
		return nil, fmt.Errorf("listing enabled channels: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing channel rows failed", "error", err)
		}
	}()
	ids := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning channel id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating channels: %w", err)
	}
	return ids, nil
}

// HostFSThresholds returns the lowest warning and critical thresholds of the
// enabled host_fs_percent rules that cover envID, so the dashboard colour follows
// the rules the operator edited; with no such rule it returns the 85/95 defaults.
func (s *Service) HostFSThresholds(ctx context.Context, envID string) (warning, critical float64, err error) {
	rules, err := s.ListEnabled(ctx)
	if err != nil {
		return DefaultDiskWarningPercent, DefaultDiskCriticalPercent, fmt.Errorf("listing alert rules: %w", err)
	}
	warning, critical = 0, 0
	for _, rule := range rules {
		if rule.SourceType != sourceHost || rule.ConditionKey != conditionHostFSPercent {
			continue
		}
		if op := rule.Condition.Operator; op != "" && op != ">" && op != ">=" {
			continue
		}
		if len(rule.EnvironmentIDs) > 0 && !containsString(rule.EnvironmentIDs, envID) {
			continue
		}
		switch normalizeSeverity(rule.Severity) {
		case "critical":
			critical = lowest(critical, rule.Condition.Threshold)
		case "warning":
			warning = lowest(warning, rule.Condition.Threshold)
		}
	}
	if warning == 0 {
		warning = DefaultDiskWarningPercent
	}
	if critical == 0 {
		critical = DefaultDiskCriticalPercent
	}
	return warning, critical, nil
}

func lowest(current, candidate float64) float64 {
	if current == 0 || candidate < current {
		return candidate
	}
	return current
}
