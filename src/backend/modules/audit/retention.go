// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"fmt"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
)

// RetentionView is the audit retention policy as the Audit Center edits it: the
// shared default from Settings, each severity's own value if set, and the result.
type RetentionView struct {
	DefaultDays int             `json:"defaultDays"`
	Severities  []string        `json:"severities"`
	Overrides   map[string]*int `json:"overrides"`
	Effective   map[string]int  `json:"effective"`
	MaxDays     int             `json:"maxDays"`
}

// RetentionUpdate carries one entry per severity to change; a null value clears
// the override so that severity follows the default again.
type RetentionUpdate struct {
	Overrides map[string]*int `json:"overrides"`
}

// Retention returns the effective per-severity policy.
func (s *Service) Retention(ctx context.Context) (RetentionView, error) {
	defaultDays := coreSettings.ReadRetentionSettingsContext(ctx, s.db).AuditRetentionDays
	policy, err := coreAudit.ReadRetentionPolicy(ctx, s.db, defaultDays)
	if err != nil {
		return RetentionView{}, err
	}
	view := RetentionView{
		DefaultDays: defaultDays,
		Severities:  append([]string{}, coreAudit.Severities...),
		Overrides:   map[string]*int{},
		Effective:   map[string]int{},
		MaxDays:     coreAudit.MaxRetentionDays,
	}
	for _, severity := range coreAudit.Severities {
		if days, ok := policy.BySeverity[severity]; ok {
			value := days
			view.Overrides[severity] = &value
		} else {
			view.Overrides[severity] = nil
		}
		view.Effective[severity] = policy.DaysFor(severity)
	}
	return view, nil
}

// validateRetentionUpdate rejects an unknown severity or an out-of-range value
// before anything is written.
func validateRetentionUpdate(update RetentionUpdate) error {
	if len(update.Overrides) == 0 {
		return fmt.Errorf("no retention overrides supplied")
	}
	for severity, days := range update.Overrides {
		if !coreAudit.ValidSeverity(severity) {
			return fmt.Errorf("unknown severity %q", severity)
		}
		if days != nil && (*days < 0 || *days > coreAudit.MaxRetentionDays) {
			return fmt.Errorf("retention for %s out of range", severity)
		}
	}
	return nil
}

// UpdateRetention stores the overrides and returns the view before and after,
// so the handler can audit exactly what moved.
func (s *Service) UpdateRetention(ctx context.Context, update RetentionUpdate) (RetentionView, RetentionView, error) {
	before, err := s.Retention(ctx)
	if err != nil {
		return RetentionView{}, RetentionView{}, err
	}
	if err := coreAudit.WriteRetentionOverrides(ctx, s.db, update.Overrides); err != nil {
		return before, RetentionView{}, err
	}
	after, err := s.Retention(ctx)
	return before, after, err
}
