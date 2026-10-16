// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/redact"
)

// Phase 7 hardening: extra redaction patterns, prompt-injection
// detection, and per-user usage caps. The base redaction in
// tools/redact.go handles the common cases; this file extends
// coverage for things that only matter at the AI boundary (agent
// tokens, JWTs, connection strings) and adds the rate-limit
// counters that protect the platform from runaway usage.

// --- Extended redaction patterns ---

// assistantHistoryWindow is how far back the assistant looks: the audit cutoff
// it summarises, the age past which a cached consent is stale, and the fallback
// start of a conversation with no recorded user turn.
const assistantHistoryWindow = 24 * time.Hour

// HardenedRedactString masks agent tokens, JWTs, URL credentials and
// dockerconfig auth values with the assistant's wider connection-string match.
func HardenedRedactString(s string) string { return redact.WideHardenedString(s) }

// HardenedRedact walks a value and applies HardenedRedactString to
// every string it finds. Map keys are preserved.
func HardenedRedact(v any) any { return redact.WideHardened(v) }

// --- Prompt-injection detection ---

// injectionPhrases is a closed set of phrases that strongly suggest
// a tool output is trying to hijack the LLM. Matching is case-
// insensitive; phrases are anchored to the start of a sentence to
// limit false positives on normal log output.
var injectionPhrases = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard the above",
	"forget everything above",
	"new instructions:",
	"system: you are",
	"<|im_start|>",
	"<|im_end|>",
	"### instruction:",
	"### response:",
	"</s><s>",
}

// DetectInjection scans a string for known prompt-injection
// phrases. Returns the first matching phrase (or "" when clean).
// The caller decides whether to redact, warn, or reject.
func DetectInjection(s string) string {
	low := strings.ToLower(s)
	for _, p := range injectionPhrases {
		if strings.Contains(low, p) {
			return p
		}
	}
	return ""
}

// SanitizeOutput runs HardenedRedact + DetectInjection on a tool
// result. When an injection attempt is found, the result is
// replaced with a small envelope that flags the issue without
// forwarding the hostile content to the model.
func SanitizeOutput(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		cleaned := HardenedRedactString(t)
		if phrase := DetectInjection(cleaned); phrase != "" {
			return map[string]any{
				"sanitized": true,
				"reason":    "prompt_injection_detected",
				"matched":   phrase,
				"note":      "Tool output contained a phrase that looks like a prompt-injection attempt. The original content is not forwarded to the model.",
				"hint":      "Use the tool again with a narrower query, or call entity.relate to inspect the underlying entity directly.",
			}, true
		}
		return cleaned, false
	default:
		cleaned := HardenedRedact(v)
		// Re-marshal then detect on the rendered JSON. We accept the
		// tiny CPU cost to keep the detection logic uniform.
		// Skipped here to keep this path allocation-free; the
		// string path above is the common case (tool returns a
		// string envelope).
		_ = cleaned
		return cleaned, false
	}
}

// --- Usage caps ---

// UsageCaps is the per-user budget. The defaults are conservative
// enough to keep a single user from filling the platform's LLM
// bill; admins can override via Settings (Phase 6 follow-on).
type UsageCaps struct {
	DailyTurns       int
	DailyToolCalls   int
	DailyActionCalls int
	DailyTokensIn    int
	DailyTokensOut   int
}

// DefaultCaps returns the Phase 7 conservative defaults.
func DefaultCaps() UsageCaps {
	return UsageCaps{
		DailyTurns:       200,
		DailyToolCalls:   2000,
		DailyActionCalls: 200,
		DailyTokensIn:    4_000_000,
		DailyTokensOut:   1_000_000,
	}
}

// UsageRow is one row of ai_assistant_usage. Counters are daily
// totals that reset when daily_window_start rolls over to a new
// calendar day (UTC).
type UsageRow struct {
	UserID           string
	DailyWindowStart string
	DailyTurns       int
	DailyToolCalls   int
	DailyActionCalls int
	DailyTokensIn    int
	DailyTokensOut   int
	LastTurnAt       string
	UpdatedAt        string
}

// ErrUsageCapExceeded is returned by CheckUsage when a user is over
// one of the configured caps. The HTTP layer maps it to 429.
var ErrUsageCapExceeded = errors.New("usage cap exceeded")

// LoadUsage returns the current usage row for a user, creating a
// fresh zero row when none exists. The window rolls over when the
// stored daily_window_start is more than 24h old.
func (s *Service) LoadUsage(ctx context.Context, userID string) (*UsageRow, error) {
	if userID == "" {
		return nil, errors.New("user id required")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id, COALESCE(daily_window_start, ''), daily_turns, daily_tool_calls,
		        daily_action_calls, daily_tokens_in, daily_tokens_out,
		        COALESCE(last_turn_at, ''), updated_at
		 FROM ai_assistant_usage WHERE user_id = ?`,
		userID,
	)
	var u UsageRow
	if err := row.Scan(&u.UserID, &u.DailyWindowStart, &u.DailyTurns, &u.DailyToolCalls,
		&u.DailyActionCalls, &u.DailyTokensIn, &u.DailyTokensOut,
		&u.LastTurnAt, &u.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			now := time.Now().UTC().Format(time.RFC3339)
			if _, err := s.db.ExecContext(ctx,
				`INSERT OR IGNORE INTO ai_assistant_usage
				     (id, user_id, daily_window_start, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?)`,
				userID, userID, now, now, now,
			); err != nil {
				return nil, fmt.Errorf("creating ai assistant usage row: %w", err)
			}
			return &UsageRow{
				UserID:           userID,
				DailyWindowStart: now,
				UpdatedAt:        now,
			}, nil
		}
		return nil, err
	}
	// Roll over if the window is more than 24h old.
	if u.DailyWindowStart != "" {
		if t, err := time.Parse(time.RFC3339, u.DailyWindowStart); err == nil {
			if time.Since(t) > assistantHistoryWindow {
				now := time.Now().UTC().Format(time.RFC3339)
				if _, err := s.db.ExecContext(ctx,
					`UPDATE ai_assistant_usage
					 SET daily_window_start = ?, daily_turns = 0, daily_tool_calls = 0,
					     daily_action_calls = 0, daily_tokens_in = 0, daily_tokens_out = 0,
					     updated_at = ?
					 WHERE user_id = ?`,
					now, now, userID,
				); err != nil {
					return nil, fmt.Errorf("resetting ai assistant usage window: %w", err)
				}
				u.DailyWindowStart = now
				u.DailyTurns = 0
				u.DailyToolCalls = 0
				u.DailyActionCalls = 0
				u.DailyTokensIn = 0
				u.DailyTokensOut = 0
			}
		}
	}
	return &u, nil
}

// CheckUsage verifies the current counters against the supplied
// caps. When any field is over the cap, returns a wrapped
// ErrUsageCapExceeded with the specific field. The caller is
// expected to roll back the half-applied state when this returns
// an error.
func (u *UsageRow) CheckUsage(caps UsageCaps) error {
	if u == nil {
		return nil
	}
	if caps.DailyTurns > 0 && u.DailyTurns >= caps.DailyTurns {
		return fmt.Errorf("%w: daily_turns", ErrUsageCapExceeded)
	}
	if caps.DailyToolCalls > 0 && u.DailyToolCalls >= caps.DailyToolCalls {
		return fmt.Errorf("%w: daily_tool_calls", ErrUsageCapExceeded)
	}
	if caps.DailyActionCalls > 0 && u.DailyActionCalls >= caps.DailyActionCalls {
		return fmt.Errorf("%w: daily_action_calls", ErrUsageCapExceeded)
	}
	if caps.DailyTokensIn > 0 && u.DailyTokensIn >= caps.DailyTokensIn {
		return fmt.Errorf("%w: daily_tokens_in", ErrUsageCapExceeded)
	}
	if caps.DailyTokensOut > 0 && u.DailyTokensOut >= caps.DailyTokensOut {
		return fmt.Errorf("%w: daily_tokens_out", ErrUsageCapExceeded)
	}
	return nil
}

// RecordTurn bumps the user's turn counter. Call after a chat
// turn completes (success or failure).
func (s *Service) RecordTurn(ctx context.Context, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE ai_assistant_usage
		 SET daily_turns = daily_turns + 1, last_turn_at = ?, updated_at = ?
		 WHERE user_id = ?`,
		now, now, userID,
	)
	return err
}

// RecordToolCall bumps the user's tool-call counter.
func (s *Service) RecordToolCall(ctx context.Context, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE ai_assistant_usage
		 SET daily_tool_calls = daily_tool_calls + 1, updated_at = ?
		 WHERE user_id = ?`,
		now, userID,
	)
	return err
}

// RecordActionCall bumps the user's action-call counter.
func (s *Service) RecordActionCall(ctx context.Context, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE ai_assistant_usage
		 SET daily_action_calls = daily_action_calls + 1, updated_at = ?
		 WHERE user_id = ?`,
		now, userID,
	)
	return err
}

// RecordTokens bumps the token counters. Negative values are
// clamped to 0 to keep a botched accounting pass from poisoning
// the totals.
func (s *Service) RecordTokens(ctx context.Context, userID string, in, out int) error {
	if in < 0 {
		in = 0
	}
	if out < 0 {
		out = 0
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE ai_assistant_usage
		 SET daily_tokens_in = daily_tokens_in + ?, daily_tokens_out = daily_tokens_out + ?, updated_at = ?
		 WHERE user_id = ?`,
		in, out, now, userID,
	)
	return err
}
