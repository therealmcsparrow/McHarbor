// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type target struct {
	id, userID, endpoint, p256dh, auth, lang string
	nextAttempt                              sql.NullString
	failures                                 int
}

// outcome is what one delivery attempt did to its subscription.
type outcome int

const (
	delivered outcome = iota
	failed
	removed
	skipped
)

// deliver sends n to every eligible subscription. Recipients are chosen by
// permission here because no part of the notification pipeline names users.
func (s *Service) deliver(ctx context.Context, n Notification, aud Audience) {
	targets, err := s.targets(ctx, "")
	if err != nil {
		s.logger.Error("push: loading subscriptions failed", "error", err)
		return
	}
	allowed := map[string]bool{}
	for _, t := range targets {
		ok, seen := allowed[t.userID]
		if !seen {
			ok = s.userMay(ctx, t.userID, aud)
			allowed[t.userID] = ok
		}
		if ok {
			s.sendOne(ctx, t, n)
		}
	}
	s.pruneStale(ctx)
}

func (s *Service) userMay(ctx context.Context, userID string, aud Audience) bool {
	if aud.Permission == "" {
		return true
	}
	if s.rbac == nil {
		return false
	}
	ok, err := s.rbac.HasPermission(ctx, userID, aud.EnvID, aud.Permission)
	if err != nil {
		s.logger.Error("push: permission check failed", "user", userID, "permission", aud.Permission, "error", err)
		return false
	}
	return ok
}

// SendTest pushes a test notification to userID's own devices only and waits
// for the result, because it is what the operator presses first.
func (s *Service) SendTest(ctx context.Context, userID string, n Notification) (TestResult, error) {
	targets, err := s.targets(ctx, userID)
	if err != nil {
		return TestResult{}, err
	}
	var res TestResult
	for _, t := range targets {
		t.nextAttempt = sql.NullString{}
		switch s.sendOne(ctx, t, n) {
		case delivered:
			res.Sent++
		case removed:
			res.Removed++
		default:
			res.Failed++
		}
	}
	return res, nil
}

func (s *Service) targets(ctx context.Context, userID string) ([]target, error) {
	q := `SELECT ps.id, ps.user_id, ps.endpoint, ps.p256dh, ps.auth_secret, ps.next_attempt_at, ps.failure_count,
		COALESCE(u.preferred_language, '')
		FROM push_subscriptions ps LEFT JOIN users u ON u.id = ps.user_id`
	args := []any{}
	if userID != "" {
		q += ` WHERE ps.user_id = ?`
		args = append(args, userID)
	}
	fallback := s.defaultLanguage(ctx)
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY ps.created_at LIMIT 1000`, args...)
	if err != nil {
		return nil, fmt.Errorf("querying subscriptions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			s.logger.Error("push: closing rows failed", "error", cerr)
		}
	}()
	var out []target
	for rows.Next() {
		var t target
		var encEndpoint, encP256, encAuth string
		var lang string
		if err := rows.Scan(&t.id, &t.userID, &encEndpoint, &encP256, &encAuth, &t.nextAttempt, &t.failures, &lang); err != nil {
			return nil, fmt.Errorf("scanning subscription: %w", err)
		}
		t.lang = supportedLanguage(lang)
		if t.lang == "" {
			t.lang = fallback
		}
		if t.endpoint, err = s.enc.Decrypt(encEndpoint); err != nil {
			s.logger.Error("push: undecryptable subscription skipped", "id", t.id, "error", err)
			continue
		}
		if t.p256dh, err = s.enc.Decrypt(encP256); err != nil {
			s.logger.Error("push: undecryptable subscription skipped", "id", t.id, "error", err)
			continue
		}
		if t.auth, err = s.enc.Decrypt(encAuth); err != nil {
			s.logger.Error("push: undecryptable subscription skipped", "id", t.id, "error", err)
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) sendOne(ctx context.Context, t target, n Notification) outcome {
	now := s.now().UTC()
	if t.nextAttempt.Valid {
		if next, err := time.Parse(time.RFC3339, t.nextAttempt.String); err == nil && now.Before(next) {
			return skipped
		}
	}
	pub, priv, err := s.keys(ctx)
	if err != nil {
		s.logger.Error("push: VAPID keys unavailable", "error", err)
		return failed
	}
	n = localize(n, t.lang)
	payload, err := json.Marshal(n)
	if err != nil {
		s.logger.Error("push: encoding payload failed", "error", err)
		return failed
	}
	urgency := webpush.UrgencyNormal
	if n.Severity == "critical" || n.Severity == "error" {
		urgency = webpush.UrgencyHigh
	}
	status, err := s.sender.Send(ctx, payload, &webpush.Subscription{
		Endpoint: t.endpoint,
		Keys:     webpush.Keys{P256dh: t.p256dh, Auth: t.auth},
	}, &webpush.Options{
		Subscriber: s.subject, VAPIDPublicKey: pub, VAPIDPrivateKey: priv,
		TTL: 3600, Urgency: urgency, Topic: n.Tag,
	})

	switch {
	case err == nil && status >= 200 && status < 300:
		s.exec(ctx, `UPDATE push_subscriptions SET last_success_at = ?, failure_count = 0, first_failure_at = NULL,
			next_attempt_at = NULL, updated_at = ? WHERE id = ?`, now.Format(time.RFC3339), now.Format(time.RFC3339), t.id)
		return delivered
	case err == nil && (status == http.StatusNotFound || status == http.StatusGone):
		s.exec(ctx, `DELETE FROM push_subscriptions WHERE id = ?`, t.id)
		s.logger.Info("push: subscription expired and was removed", "id", t.id, "status", status)
		return removed
	default:
		wait := time.Minute << min(t.failures, 12)
		if wait > maxBackoff || wait <= 0 {
			wait = maxBackoff
		}
		s.exec(ctx, `UPDATE push_subscriptions SET failure_count = failure_count + 1,
			first_failure_at = COALESCE(first_failure_at, ?), next_attempt_at = ?, updated_at = ? WHERE id = ?`,
			now.Format(time.RFC3339), now.Add(wait).Format(time.RFC3339), now.Format(time.RFC3339), t.id)
		s.logger.Warn("push: delivery failed", "id", t.id, "status", status, "error", err, "retryIn", wait)
		return failed
	}
}

// defaultLanguage is the instance's default_language setting, the language a
// user who never chose one sees; English when it is unset or unsupported.
func (s *Service) defaultLanguage(ctx context.Context) string {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'default_language'`).Scan(&v); err != nil {
		return "en"
	}
	if lang := supportedLanguage(v); lang != "" {
		return lang
	}
	return "en"
}

// pruneStale deletes subscriptions that have failed every attempt for
// staleAfter, whatever the code, so a table of dead devices cannot grow forever.
func (s *Service) pruneStale(ctx context.Context) {
	cutoff := s.now().UTC().Add(-staleAfter).Format(time.RFC3339)
	s.exec(ctx, `DELETE FROM push_subscriptions WHERE first_failure_at IS NOT NULL AND first_failure_at < ?`, cutoff)
}

func (s *Service) exec(ctx context.Context, q string, args ...any) {
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		s.logger.Error("push: bookkeeping write failed", "error", err)
	}
}
