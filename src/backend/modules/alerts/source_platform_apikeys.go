// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
)

// apiKeyAges reports every active key's age in days. The subject is the key, so
// rotating it (which revokes it) drops the subject and resolves the alert.
func (s *platformSource) apiKeyAges(ctx context.Context) ([]alerting.AlertObservation, error) {
	return s.apiKeyObservations(ctx, false)
}

// apiKeysExpiring reports days left for every active key that has an expiry
// still ahead of it; keys without one never expire and are not observed.
func (s *platformSource) apiKeysExpiring(ctx context.Context) ([]alerting.AlertObservation, error) {
	return s.apiKeyObservations(ctx, true)
}

func (s *platformSource) apiKeyObservations(ctx context.Context, expiring bool) ([]alerting.AlertObservation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT k.id, k.name, COALESCE(u.username, ''), k.created_at, COALESCE(k.expires_at, '')
		  FROM api_keys k
		  LEFT JOIN users u ON u.id = k.user_id
		 WHERE k.is_active = 1
		 LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	now := time.Now().UTC()
	out := make([]alerting.AlertObservation, 0, 16)
	for rows.Next() {
		var id, name, owner, created, expires string
		if err := rows.Scan(&id, &name, &owner, &created, &expires); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		var days float64
		if expiring {
			exp, err := parseDBTime(expires)
			if expires == "" || err != nil || !exp.After(now) {
				continue
			}
			days = exp.Sub(now).Hours() / 24
		} else {
			born, err := parseDBTime(created)
			if err != nil {
				continue
			}
			days = now.Sub(born).Hours() / 24
		}
		out = append(out, alerting.AlertObservation{
			SubjectID:   id,
			SubjectName: name,
			Value:       float64(int(days)),
			ValueLabel:  fmt.Sprintf("%d days", int(days)),
			Context:     map[string]string{"owner": owner},
		})
	}
	return out, rows.Err()
}
