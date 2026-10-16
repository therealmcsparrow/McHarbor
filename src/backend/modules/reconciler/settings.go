// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Settings is the reconciler's instance-wide configuration and the state of
// the enforcement circuit breaker.
type Settings struct {
	ConfirmPasses       int    `json:"confirmPasses"`
	EnforceEnabled      bool   `json:"enforceEnabled"`
	MaxActionsPerPass   int    `json:"maxActionsPerPass"`
	CooldownSeconds     int    `json:"cooldownSeconds"`
	FailureThreshold    int    `json:"failureThreshold"`
	BreakerOpen         bool   `json:"breakerOpen"`
	BreakerReason       string `json:"breakerReason,omitempty"`
	BreakerOpenedAt     string `json:"breakerOpenedAt,omitempty"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
}

// Limits on the settings, so a typo cannot turn enforcement into a flood.
const (
	MinConfirmPasses = 1
	MaxConfirmPasses = 10
	MaxActionsCap    = 25
	MaxCooldown      = 86400
	MaxThreshold     = 20
)

// Settings validation failures, one per field. Each maps to a refusal reason key
// through validationReason, so a handler never echoes the formatted message.
var (
	ErrConfirmPassesRange    = errors.New("confirm passes out of range")
	ErrActionsPerPassRange   = errors.New("actions per pass out of range")
	ErrCooldownRange         = errors.New("cooldown out of range")
	ErrFailureThresholdRange = errors.New("failure threshold out of range")
)

// validationReason maps a settings or source validation failure to its refusal
// reason key, or "invalid" for anything it does not recognise.
func validationReason(err error) string {
	switch {
	case errors.Is(err, ErrConfirmPassesRange):
		return "confirm_passes_range"
	case errors.Is(err, ErrActionsPerPassRange):
		return "actions_per_pass_range"
	case errors.Is(err, ErrCooldownRange):
		return "cooldown_range"
	case errors.Is(err, ErrFailureThresholdRange):
		return "failure_threshold_range"
	case errors.Is(err, ErrSourceFieldsRequired):
		return "source_fields_required"
	case errors.Is(err, ErrSourceBranchInvalid):
		return "invalid_branch"
	case errors.Is(err, ErrSourcePathEscapes):
		return "path_escapes_repo"
	}
	return "invalid"
}

// DefaultSettings are what a fresh instance runs with: enforcement off.
func DefaultSettings() Settings {
	return Settings{ConfirmPasses: 2, MaxActionsPerPass: 3, CooldownSeconds: 600, FailureThreshold: 3}
}

// SettingsStore reads and writes the single settings row.
type SettingsStore struct {
	db *sql.DB
}

// NewSettingsStore returns a store over db.
func NewSettingsStore(database *sql.DB) *SettingsStore {
	return &SettingsStore{db: database}
}

// Load returns the settings, or the defaults when the row is missing.
func (s *SettingsStore) Load(ctx context.Context) (Settings, error) {
	out := DefaultSettings()
	var enabled, open int
	var opened sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT confirm_passes, enforce_enabled, max_actions_per_pass, cooldown_seconds, failure_threshold,
		 breaker_open, breaker_reason, breaker_opened_at, consecutive_failures FROM reconciler_settings WHERE id = 'global'`,
	).Scan(&out.ConfirmPasses, &enabled, &out.MaxActionsPerPass, &out.CooldownSeconds, &out.FailureThreshold,
		&open, &out.BreakerReason, &opened, &out.ConsecutiveFailures)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return DefaultSettings(), fmt.Errorf("reading reconciler settings: %w", err)
	}
	out.EnforceEnabled, out.BreakerOpen, out.BreakerOpenedAt = enabled != 0, open != 0, opened.String
	return out, nil
}

// Validate checks the editable fields are in range.
func (st Settings) Validate() error {
	switch {
	case st.ConfirmPasses < MinConfirmPasses || st.ConfirmPasses > MaxConfirmPasses:
		return fmt.Errorf("%w: must be %d to %d", ErrConfirmPassesRange, MinConfirmPasses, MaxConfirmPasses)
	case st.MaxActionsPerPass < 1 || st.MaxActionsPerPass > MaxActionsCap:
		return fmt.Errorf("%w: must be 1 to %d", ErrActionsPerPassRange, MaxActionsCap)
	case st.CooldownSeconds < 0 || st.CooldownSeconds > MaxCooldown:
		return fmt.Errorf("%w: must be 0 to %d seconds", ErrCooldownRange, MaxCooldown)
	case st.FailureThreshold < 1 || st.FailureThreshold > MaxThreshold:
		return fmt.Errorf("%w: must be 1 to %d", ErrFailureThresholdRange, MaxThreshold)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetConfirmPasses changes the debounce, the one setting that is not about
// enforcement and so is edited under reconciler.manage.
func (s *SettingsStore) SetConfirmPasses(ctx context.Context, n int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reconciler_settings SET confirm_passes = ?, updated_at = ? WHERE id = 'global'`,
		n, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("saving confirm passes: %w", err)
	}
	return nil
}

// SetEnforcement saves the enforcement switch and limits; the breaker state is
// not part of it and moves only through Trip and Reset.
func (s *SettingsStore) SetEnforcement(ctx context.Context, st Settings) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reconciler_settings SET enforce_enabled = ?, max_actions_per_pass = ?, cooldown_seconds = ?,
		 failure_threshold = ?, updated_at = ? WHERE id = 'global'`,
		boolInt(st.EnforceEnabled), st.MaxActionsPerPass, st.CooldownSeconds, st.FailureThreshold,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("saving enforcement settings: %w", err)
	}
	return nil
}

// RecordOutcome counts an enforced action. A success clears the run of
// failures; reaching the threshold opens the breaker and reports true once.
func (s *SettingsStore) RecordOutcome(ctx context.Context, ok bool, reason string) (tripped bool, err error) {
	st, err := s.Load(ctx)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if ok {
		_, err = s.db.ExecContext(ctx, `UPDATE reconciler_settings SET consecutive_failures = 0, updated_at = ? WHERE id = 'global'`, now)
		if err != nil {
			return false, fmt.Errorf("recording enforcement success: %w", err)
		}
		return false, nil
	}
	failures := st.ConsecutiveFailures + 1
	trip := !st.BreakerOpen && failures >= st.FailureThreshold
	if trip {
		_, err = s.db.ExecContext(ctx,
			`UPDATE reconciler_settings SET consecutive_failures = ?, breaker_open = 1, breaker_reason = ?,
			 breaker_opened_at = ?, updated_at = ? WHERE id = 'global'`, failures, reason, now, now)
	} else {
		_, err = s.db.ExecContext(ctx,
			`UPDATE reconciler_settings SET consecutive_failures = ?, updated_at = ? WHERE id = 'global'`, failures, now)
	}
	if err != nil {
		return false, fmt.Errorf("recording enforcement failure: %w", err)
	}
	return trip, nil
}

// Reset closes the breaker. It is the only way enforcement resumes after it
// tripped, and it is always a person's decision.
func (s *SettingsStore) Reset(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reconciler_settings SET breaker_open = 0, breaker_reason = '', breaker_opened_at = NULL,
		 consecutive_failures = 0, updated_at = ? WHERE id = 'global'`, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("resetting the breaker: %w", err)
	}
	return nil
}
