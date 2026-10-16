// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

const (
	settingPublic  = "webpush_vapid_public"
	settingPrivate = "webpush_vapid_private"
	settingsCat    = "webpush"

	// staleAfter is how long a subscription may fail every attempt before it
	// is deleted whatever the status code, so dead devices cannot accumulate.
	staleAfter = 30 * 24 * time.Hour
	maxBackoff = 6 * time.Hour
	maxLabel   = 80
	maxUA      = 300
)

// ErrNotFound means the subscription does not exist or belongs to someone else.
var ErrNotFound = errors.New("push subscription not found")

// ErrInvalidSubscription covers a malformed or disallowed browser subscription.
var ErrInvalidSubscription = errors.New("invalid push subscription")

// maxSubscriptionsPerUser bounds one account's devices; every alert fans out to
// each of them, so an unbounded list is a way to amplify delivery.
const maxSubscriptionsPerUser = 20

// ErrTooManySubscriptions refuses a new device for a user already at the cap.
var ErrTooManySubscriptions = errors.New("too many push subscriptions")

// allowedPushHosts are the push services browsers actually use. The server
// POSTs to whatever endpoint it is given, so accepting any URL would let a
// user make McHarbor call an internal address.
var allowedPushHosts = []string{
	"fcm.googleapis.com",
	"android.googleapis.com",
	"push.services.mozilla.com",
	"push.apple.com",
	"notify.windows.com",
}

// sender posts one encrypted push; a variable interface so tests need no network.
type sender interface {
	Send(ctx context.Context, payload []byte, sub *webpush.Subscription, opts *webpush.Options) (int, error)
}

type webpushSender struct{}

// Send implements sender with the real Web Push protocol.
func (webpushSender) Send(ctx context.Context, payload []byte, sub *webpush.Subscription, opts *webpush.Options) (int, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, payload, sub, opts)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("push: closing response body failed", "error", cerr)
		}
	}()
	return resp.StatusCode, nil
}

// Service owns the VAPID keypair, the subscriptions and delivery.
type Service struct {
	db      *sql.DB
	enc     *encryption.Service
	rbac    *rbac.Service
	logger  *slog.Logger
	sender  sender
	subject string
	now     func() time.Time

	keyMu sync.Mutex
	queue chan job
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
}

type job struct {
	n   Notification
	aud Audience
}

// NewService returns a push service. subject identifies the operator to push
// services (a mailto: or https: URL), as VAPID requires.
func NewService(database *sql.DB, enc *encryption.Service, rbacSvc *rbac.Service, logger *slog.Logger, subject string) *Service {
	return &Service{
		db: database, enc: enc, rbac: rbacSvc, logger: logger, sender: webpushSender{},
		subject: subject, now: time.Now, queue: make(chan job, 256), done: make(chan struct{}),
	}
}

// Start runs the delivery worker; Notify only enqueues, so an alert never waits
// on a push service.
func (s *Service) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.done:
				return
			case j := <-s.queue:
				sendCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				s.deliver(sendCtx, j.n, j.aud)
				cancel()
			}
		}
	}()
}

// Stop ends the worker after the delivery in flight.
func (s *Service) Stop() {
	s.once.Do(func() { close(s.done) })
	s.wg.Wait()
}

// Notify queues n for every subscribed user in aud. A full queue drops the
// notification with a warning rather than blocking the caller.
func (s *Service) Notify(n Notification, aud Audience) {
	select {
	case s.queue <- job{n: n, aud: aud}:
	default:
		s.logger.Warn("push: queue full, notification dropped", "title", n.Title)
	}
}

// PublicKey returns the VAPID public key, generating the keypair on first use.
func (s *Service) PublicKey(ctx context.Context) (string, error) {
	pub, _, err := s.keys(ctx)
	return pub, err
}

func (s *Service) keys(ctx context.Context) (string, string, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	pub, errPub := s.readSetting(ctx, settingPublic)
	encPriv, errPriv := s.readSetting(ctx, settingPrivate)
	if errPub == nil && errPriv == nil && pub != "" && encPriv != "" {
		priv, err := s.enc.Decrypt(encPriv)
		if err != nil {
			return "", "", fmt.Errorf("decrypting the VAPID private key: %w", err)
		}
		return pub, priv, nil
	}
	return s.generateLocked(ctx)
}

func (s *Service) generateLocked(ctx context.Context) (string, string, error) {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("generating VAPID keys: %w", err)
	}
	encPriv, err := s.enc.Encrypt(priv)
	if err != nil {
		return "", "", fmt.Errorf("encrypting the VAPID private key: %w", err)
	}
	if err := s.writeSetting(ctx, settingPrivate, encPriv); err != nil {
		return "", "", err
	}
	if err := s.writeSetting(ctx, settingPublic, pub); err != nil {
		return "", "", err
	}
	return pub, priv, nil
}

// Regenerate replaces the instance keypair. Every existing subscription was
// made against the old public key and can never be delivered to again, so all
// of them are deleted in the same step.
func (s *Service) Regenerate(ctx context.Context) (string, int64, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions`)
	if err != nil {
		return "", 0, fmt.Errorf("deleting subscriptions: %w", err)
	}
	pub, _, err := s.generateLocked(ctx)
	if err != nil {
		return "", 0, err
	}
	return pub, db.RowsAffected(res), nil
}

func (s *Service) readSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v, err
}

func (s *Service) writeSetting(ctx context.Context, key, value string) error {
	now := s.now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (id, category, key, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
		xid.New().String(), settingsCat, key, value, now, now)
	if err != nil {
		return fmt.Errorf("writing setting %s: %w", key, err)
	}
	return nil
}

// validateSubscription rejects anything that is not a browser push endpoint.
func validateSubscription(req SubscribeRequest) error {
	u, err := url.Parse(req.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Port() != "" {
		return fmt.Errorf("%w: endpoint must be an https URL", ErrInvalidSubscription)
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, h := range allowedPushHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("%w: %s is not a known push service", ErrInvalidSubscription, host)
	}
	p256, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(req.Keys.P256dh, "="))
	if err != nil || len(p256) != 65 {
		return fmt.Errorf("%w: p256dh is not an uncompressed P-256 point", ErrInvalidSubscription)
	}
	auth, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(req.Keys.Auth, "="))
	if err != nil || len(auth) != 16 {
		return fmt.Errorf("%w: auth secret must be 16 bytes", ErrInvalidSubscription)
	}
	return nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}

// Subscribe stores a device for userID; subscribing the same browser again
// updates its label rather than creating a duplicate.
func (s *Service) Subscribe(ctx context.Context, userID string, req SubscribeRequest) (*Subscription, error) {
	if err := validateSubscription(req); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(req.Endpoint))
	hash := hex.EncodeToString(sum[:])
	// A new device counts against the cap; re-subscribing a known browser only
	// refreshes its row, so it is never refused.
	var devices, known int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN endpoint_hash = ? THEN 1 ELSE 0 END), 0)
		   FROM push_subscriptions WHERE user_id = ?`, hash, userID).Scan(&devices, &known); err != nil {
		return nil, fmt.Errorf("counting subscriptions: %w", err)
	}
	if known == 0 && devices >= maxSubscriptionsPerUser {
		return nil, ErrTooManySubscriptions
	}
	endpoint, err := s.enc.Encrypt(req.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("encrypting endpoint: %w", err)
	}
	p256, err := s.enc.Encrypt(req.Keys.P256dh)
	if err != nil {
		return nil, fmt.Errorf("encrypting key: %w", err)
	}
	auth, err := s.enc.Encrypt(req.Keys.Auth)
	if err != nil {
		return nil, fmt.Errorf("encrypting auth secret: %w", err)
	}
	now := s.now().UTC().Format(time.RFC3339)
	label, ua := clip(req.Label, maxLabel), clip(req.UserAgent, maxUA)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (id, user_id, endpoint_hash, endpoint, p256dh, auth_secret, label, user_agent, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, endpoint_hash) DO UPDATE SET label = excluded.label, user_agent = excluded.user_agent,
			endpoint = excluded.endpoint, p256dh = excluded.p256dh, auth_secret = excluded.auth_secret,
			failure_count = 0, first_failure_at = NULL, next_attempt_at = NULL, updated_at = excluded.updated_at`,
		xid.New().String(), userID, hash, endpoint, p256, auth, label, ua, now, now)
	if err != nil {
		return nil, fmt.Errorf("storing subscription: %w", err)
	}
	var sub Subscription
	var last sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT id, label, user_agent, last_success_at, failure_count, created_at
		FROM push_subscriptions WHERE user_id = ? AND endpoint_hash = ?`, userID, hash).
		Scan(&sub.ID, &sub.Label, &sub.UserAgent, &last, &sub.FailureCount, &sub.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("reading subscription: %w", err)
	}
	if last.Valid {
		sub.LastSuccessAt = &last.String
	}
	return &sub, nil
}

// List returns userID's devices, without their credentials.
func (s *Service) List(ctx context.Context, userID string) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, user_agent, last_success_at, failure_count, created_at
		FROM push_subscriptions WHERE user_id = ? ORDER BY created_at DESC LIMIT 1000`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			s.logger.Error("push: closing rows failed", "error", cerr)
		}
	}()
	out := []Subscription{}
	for rows.Next() {
		var sub Subscription
		var last sql.NullString
		if err := rows.Scan(&sub.ID, &sub.Label, &sub.UserAgent, &last, &sub.FailureCount, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning subscription: %w", err)
		}
		if last.Valid {
			sub.LastSuccessAt = &last.String
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// Delete revokes one of userID's own devices.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("deleting subscription: %w", err)
	}
	if db.RowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}
