// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// alertColumns is the shared SELECT list. `condition` is the legacy
// free-text column: unread by the engine, but still read here so a
// pre-migration rule can be converted on first load.
const alertColumns = `id, name, severity, source_type, condition_key, condition_config,
	COALESCE(target, ''), environment_ids, cooldown_seconds, notify_on_recovery,
	message_template, send_in_app, enabled, created_at, updated_at, COALESCE(condition, '')`

// Service handles alert rule business logic and database operations.
type Service struct {
	db *sql.DB
}

var (
	errAlertDestinationRequired = errors.New("alert destination required")
	errAlertConditionUnknown    = errors.New("alert condition unknown")
)

// NewService creates a new alerts service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// List returns a paginated list of alert rules and the total count.
func (s *Service) List(ctx context.Context, page, perPage int) ([]Alert, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM alerts").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting alerts: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+alertColumns+" FROM alerts ORDER BY name ASC LIMIT ? OFFSET ?",
		perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing alerts: %w", err)
	}

	items, err := s.collect(ctx, rows)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// ListEnabled returns all enabled alert rules with a safety limit for
// background evaluation.
func (s *Service) ListEnabled(ctx context.Context) ([]Alert, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+alertColumns+" FROM alerts WHERE enabled = 1 ORDER BY name ASC LIMIT 1000")
	if err != nil {
		return nil, fmt.Errorf("listing enabled alerts: %w", err)
	}

	return s.collect(ctx, rows)
}

// ByID returns one rule, or nil when it does not exist.
func (s *Service) ByID(ctx context.Context, id string) (*Alert, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+alertColumns+" FROM alerts WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("getting alert %s: %w", id, err)
	}

	items, err := s.collect(ctx, rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	return &items[0], nil
}

// collect scans a result set, loads destinations, backfills legacy
// conditions, and marks source availability.
func (s *Service) collect(ctx context.Context, rows *sql.Rows) ([]Alert, error) {
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	items := make([]Alert, 0, 32)
	for rows.Next() {
		alert, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning alert row: %w", err)
		}
		items = append(items, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating alert rows: %w", err)
	}

	for i := range items {
		s.backfillLegacyCondition(ctx, &items[i])

		destinations, err := s.destinationsFor(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Destinations = destinations
		items[i].SourceAvailable = registry.available(items[i].SourceType)

		if decl, ok := registry.condition(items[i].SourceType, items[i].ConditionKey); ok {
			items[i].Condition = applyConditionDefaults(items[i].Condition, decl)
		}
	}

	return items, nil
}

// backfillLegacyCondition converts a pre-migration free-text condition
// into structured form, once, on first read.
//
// Migration 079 deliberately left this to Go: translating "> 80%" or
// "5m" in SQL would mean a second implementation of that grammar, and
// two implementations of a parser drift. Running the shipped parsers
// here makes the conversion identical to the behaviour it replaces.
//
// A failed write is logged by the caller's next read attempt rather
// than surfaced: the in-memory rule is already correct, so the only
// cost of a failed write is doing it again next time.
func (s *Service) backfillLegacyCondition(ctx context.Context, alert *Alert) {
	if alert.legacyCondition == "" {
		return
	}
	if !isZeroCondition(alert.Condition) {
		return
	}

	decl, ok := registry.condition(alert.SourceType, alert.ConditionKey)
	if !ok {
		return
	}

	converted := parseLegacyCondition(alert.ConditionKey, alert.legacyCondition, decl)
	if isZeroCondition(converted) {
		return
	}

	alert.Condition = converted

	encoded, err := encodeConditionConfig(converted)
	if err != nil {
		return
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE alerts SET condition_config = ? WHERE id = ? AND condition_config = '{}'",
		encoded, alert.ID); err != nil {
		slog.Warn("alerts: backfilling condition config failed", "alert", alert.ID, "error", err)
	}
}

// destinationsFor loads a rule's channels, joined to their names for
// display.
//
// A LEFT JOIN, not an inner one: a destination whose channel was
// deleted must still appear, with an empty name, so the operator can
// see which rule stopped notifying and why.
func (s *Service) destinationsFor(ctx context.Context, alertID string) ([]AlertDestination, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.channel_id, COALESCE(d.min_severity, ''),
		       COALESCE(c.name, ''), COALESCE(c.channel_type, '')
		  FROM alert_destinations d
		  LEFT JOIN communication_channels c ON c.id = d.channel_id
		 WHERE d.alert_id = ?
		 ORDER BY c.name ASC
		 LIMIT 100`, alertID)
	if err != nil {
		return nil, fmt.Errorf("listing alert destinations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]AlertDestination, 0, 4)
	for rows.Next() {
		var destination AlertDestination
		if err := rows.Scan(&destination.ID, &destination.ChannelID, &destination.MinSeverity,
			&destination.ChannelName, &destination.ChannelType); err != nil {
			return nil, fmt.Errorf("scanning alert destination: %w", err)
		}
		out = append(out, destination)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating alert destinations: %w", err)
	}

	return out, nil
}

// Create inserts a new alert rule and its destinations.
func (s *Service) Create(ctx context.Context, input CreateAlertInput) (*Alert, error) {
	if !hasAlertDestination(input.Destinations, input.SendInApp) {
		return nil, errAlertDestinationRequired
	}

	decl, ok := registry.condition(input.SourceType, input.ConditionKey)
	if !ok {
		return nil, errAlertConditionUnknown
	}

	condition := applyConditionDefaults(input.Condition, decl)
	if err := validateCondition(condition, decl); err != nil {
		return nil, err
	}

	encodedCondition, err := encodeConditionConfig(condition)
	if err != nil {
		return nil, err
	}
	encodedEnvironments, err := encodeEnvironmentIDs(input.EnvironmentIDs)
	if err != nil {
		return nil, err
	}

	id := xid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	notifyOnRecovery := true
	if input.NotifyOnRecovery != nil {
		notifyOnRecovery = *input.NotifyOnRecovery
	}

	err = inTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO alerts (id, name, severity, type, condition, source_type, condition_key,
			                    condition_config, target, environment_ids, cooldown_seconds,
			                    notify_on_recovery, message_template, send_in_app, enabled,
			                    created_at, updated_at)
			VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			id, input.Name, normalizeSeverity(input.Severity),
			// `type` is NOT NULL and nothing reads it any more; it is
			// written as the condition key so a rollback to the
			// previous binary sees something meaningful rather than
			// an empty string.
			input.ConditionKey,
			input.SourceType, input.ConditionKey, encodedCondition,
			input.Target, encodedEnvironments, input.CooldownSeconds,
			notifyOnRecovery, input.MessageTemplate, input.SendInApp, now, now,
		); err != nil {
			return fmt.Errorf("inserting alert: %w", err)
		}

		return replaceDestinations(ctx, tx, id, input.Destinations, now)
	})
	if err != nil {
		return nil, err
	}

	return s.ByID(ctx, id)
}

// Update applies partial updates to an alert rule.
func (s *Service) Update(ctx context.Context, id string, input UpdateAlertInput) (*Alert, error) {
	current, err := s.ByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("checking alert existence: %w", err)
	}
	if current == nil {
		return nil, nil
	}

	next := *current

	if input.Name != nil {
		next.Name = *input.Name
	}
	if input.Severity != nil {
		next.Severity = normalizeSeverity(*input.Severity)
	}
	if input.SourceType != nil {
		next.SourceType = *input.SourceType
	}
	if input.ConditionKey != nil {
		next.ConditionKey = *input.ConditionKey
	}
	if input.Condition != nil {
		next.Condition = *input.Condition
	}
	if input.Target != nil {
		next.Target = *input.Target
	}
	if input.EnvironmentIDs != nil {
		next.EnvironmentIDs = *input.EnvironmentIDs
	}
	if input.SendInApp != nil {
		next.SendInApp = *input.SendInApp
	}
	if input.CooldownSeconds != nil {
		next.CooldownSeconds = *input.CooldownSeconds
	}
	if input.NotifyOnRecovery != nil {
		next.NotifyOnRecovery = *input.NotifyOnRecovery
	}
	if input.MessageTemplate != nil {
		next.MessageTemplate = *input.MessageTemplate
	}
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}

	decl, ok := registry.condition(next.SourceType, next.ConditionKey)
	if !ok {
		return nil, errAlertConditionUnknown
	}

	next.Condition = applyConditionDefaults(next.Condition, decl)
	if err := validateCondition(next.Condition, decl); err != nil {
		return nil, err
	}

	// Destinations are replaced wholesale when present. A nil slice
	// means "leave them alone"; an empty one means "remove them", and
	// removing the last destination of a rule with no in-app delivery
	// would leave a rule that fires into nothing.
	destinations := destinationInputsFrom(next.Destinations)
	if input.Destinations != nil {
		destinations = *input.Destinations
	}
	if !hasAlertDestination(destinations, next.SendInApp) {
		return nil, errAlertDestinationRequired
	}

	encodedCondition, err := encodeConditionConfig(next.Condition)
	if err != nil {
		return nil, err
	}
	encodedEnvironments, err := encodeEnvironmentIDs(next.EnvironmentIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	err = inTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE alerts
			   SET name = ?, severity = ?, type = ?, source_type = ?, condition_key = ?,
			       condition_config = ?, target = ?, environment_ids = ?, cooldown_seconds = ?,
			       notify_on_recovery = ?, message_template = ?, send_in_app = ?, enabled = ?,
			       updated_at = ?
			 WHERE id = ?`,
			next.Name, next.Severity, next.ConditionKey, next.SourceType, next.ConditionKey,
			encodedCondition, next.Target, encodedEnvironments, next.CooldownSeconds,
			next.NotifyOnRecovery, next.MessageTemplate, next.SendInApp, next.Enabled,
			now, id,
		); err != nil {
			return fmt.Errorf("updating alert: %w", err)
		}

		if input.Destinations == nil {
			return nil
		}
		return replaceDestinations(ctx, tx, id, destinations, now)
	})
	if err != nil {
		return nil, err
	}

	// A rule whose condition, scope or enablement changed is watching
	// something different now, so its firing state is about the old
	// question. Clearing it costs one duplicate notification at worst;
	// keeping it would suppress the first real one.
	if conditionChanged(*current, next) {
		if err := newStateStore(s.db).deleteForAlert(ctx, id); err != nil {
			return nil, err
		}
	}

	return s.ByID(ctx, id)
}

// Delete removes an alert rule by ID. Returns true if a row was deleted.
//
// Destinations and states go with it; history deliberately does not —
// "what fired last night" is exactly the question asked right after
// deleting a noisy rule.
func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM alerts WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("deleting alert %s: %w", id, err)
	}

	deleted := db.RowsAffected(result) > 0
	if deleted {
		if err := newStateStore(s.db).deleteForAlert(ctx, id); err != nil {
			return true, err
		}
	}

	return deleted, nil
}

// Events returns a page of alert history.
func (s *Service) Events(ctx context.Context, filter EventFilter, page, perPage int) ([]AlertEvent, int64, error) {
	return newHistoryStore(s.db).list(ctx, filter, page, perPage)
}

// replaceDestinations swaps a rule's destination set inside a
// transaction.
func replaceDestinations(ctx context.Context, tx *sql.Tx, alertID string, destinations []DestinationInput, now string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM alert_destinations WHERE alert_id = ?", alertID); err != nil {
		return fmt.Errorf("clearing alert destinations: %w", err)
	}

	seen := make(map[string]struct{}, len(destinations))
	for _, destination := range destinations {
		channelID := strings.TrimSpace(destination.ChannelID)
		if channelID == "" {
			continue
		}
		if _, duplicate := seen[channelID]; duplicate {
			continue
		}
		seen[channelID] = struct{}{}

		minSeverity := strings.TrimSpace(destination.MinSeverity)
		var storedSeverity any
		if minSeverity != "" && validAlertSeverities[minSeverity] {
			storedSeverity = minSeverity
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO alert_destinations (id, alert_id, channel_id, min_severity, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			xid.New().String(), alertID, channelID, storedSeverity, now, now); err != nil {
			return fmt.Errorf("inserting alert destination: %w", err)
		}
	}

	return nil
}

func scanAlert(rows *sql.Rows) (Alert, error) {
	var a Alert
	var severity, sourceType, conditionKey, conditionConfig sql.NullString
	var environmentIDs, messageTemplate sql.NullString
	var cooldown sql.NullInt64
	var notifyOnRecovery, sendInApp, enabled sql.NullBool

	if err := rows.Scan(&a.ID, &a.Name, &severity, &sourceType, &conditionKey, &conditionConfig,
		&a.Target, &environmentIDs, &cooldown, &notifyOnRecovery, &messageTemplate,
		&sendInApp, &enabled, &a.CreatedAt, &a.UpdatedAt, &a.legacyCondition); err != nil {
		return Alert{}, err
	}

	a.Severity = normalizeSeverity(severity.String)
	a.SourceType = sourceType.String
	a.ConditionKey = conditionKey.String
	a.Condition = decodeConditionConfig(conditionConfig.String)
	a.EnvironmentIDs = decodeEnvironmentIDs(environmentIDs.String)
	a.CooldownSeconds = int(cooldown.Int64)
	a.NotifyOnRecovery = notifyOnRecovery.Bool
	a.MessageTemplate = messageTemplate.String
	a.SendInApp = sendInApp.Bool
	a.Enabled = enabled.Bool
	a.Destinations = []AlertDestination{}

	return a, nil
}

// hasAlertDestination enforces that a rule notifies somebody.
func hasAlertDestination(destinations []DestinationInput, sendInApp bool) bool {
	if sendInApp {
		return true
	}

	for _, destination := range destinations {
		if strings.TrimSpace(destination.ChannelID) != "" {
			return true
		}
	}

	return false
}

func destinationInputsFrom(destinations []AlertDestination) []DestinationInput {
	out := make([]DestinationInput, 0, len(destinations))
	for _, destination := range destinations {
		out = append(out, DestinationInput{
			ChannelID:   destination.ChannelID,
			MinSeverity: destination.MinSeverity,
		})
	}
	return out
}

// conditionChanged reports whether an update changed what the rule
// watches, as opposed to how it is delivered.
func conditionChanged(before, after Alert) bool {
	return before.SourceType != after.SourceType ||
		before.ConditionKey != after.ConditionKey ||
		!conditionConfigEqual(before.Condition, after.Condition) ||
		before.Target != after.Target ||
		before.Enabled != after.Enabled ||
		strings.Join(before.EnvironmentIDs, ",") != strings.Join(after.EnvironmentIDs, ",")
}

// isZeroCondition reports whether a config carries no operator,
// threshold, duration or parameters. ConditionConfig holds a map, so
// it cannot be compared with ==.
func isZeroCondition(cfg ConditionConfig) bool {
	return cfg.Operator == "" &&
		cfg.Threshold == 0 &&
		cfg.Unit == "" &&
		cfg.ForSeconds == 0 &&
		cfg.WindowSeconds == 0 &&
		len(cfg.Params) == 0
}

// conditionConfigEqual compares two configs field by field, including
// the params map.
func conditionConfigEqual(a, b ConditionConfig) bool {
	if a.Operator != b.Operator ||
		a.Threshold != b.Threshold ||
		a.Unit != b.Unit ||
		a.ForSeconds != b.ForSeconds ||
		a.WindowSeconds != b.WindowSeconds ||
		len(a.Params) != len(b.Params) {
		return false
	}

	for key, value := range a.Params {
		if b.Params[key] != value {
			return false
		}
	}

	return true
}

// inTx runs fn in a transaction bound to ctx, so a cancelled request abandons the write instead of finishing it in the background.
func inTx(ctx context.Context, conn *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %w (original: %w)", rbErr, err)
		}
		return err
	}
	return tx.Commit()
}
