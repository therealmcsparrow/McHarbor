// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deviceauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
)

const (
	codeTTL = 10 * time.Minute
	// pollInterval keeps a polling CLI inside the auth group's rate limit of
	// ten requests a minute per address, with room for a browser login beside it.
	pollInterval = 10 * time.Second
	// userCodeAlphabet has no vowels or look-alikes, so a code cannot spell a
	// word or be misread aloud.
	userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ"
	verificationPath = "/device"
	maxLabelLength   = 128
)

// KeyMinter is the part of the API key service a completed flow needs.
type KeyMinter interface {
	Create(ctx context.Context, userID string, input *apikeys.CreateAPIKeyInput) (*apikeys.CreateAPIKeyResult, error)
}

// Service runs device authorization over the device_codes table.
type Service struct {
	db   *sql.DB
	keys KeyMinter
	now  func() time.Time
}

// NewService returns a Service that mints approved keys through keys.
func NewService(database *sql.DB, keys KeyMinter) *Service {
	return &Service{db: database, keys: keys, now: time.Now}
}

// Start opens a request and returns the codes. Only a hash of the device code
// is stored, so a database read does not hand anyone a pending sign-in.
func (s *Service) Start(ctx context.Context, in StartInput) (*StartResult, error) {
	s.prune(ctx)
	deviceCode, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	for attempt := 0; attempt < 5; attempt++ {
		userCode, err := randomUserCode()
		if err != nil {
			return nil, err
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO device_codes (id, device_code_hash, user_code, status, requested_host, requested_os_user, expires_at, created_at, updated_at)
			 VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?)`,
			xid.New().String(), hashCode(deviceCode), userCode, clean(in.Host), clean(in.OSUser),
			now.Add(codeTTL).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err == nil {
			return &StartResult{
				DeviceCode: deviceCode, UserCode: userCode, VerificationPath: verificationPath,
				ExpiresIn: int(codeTTL.Seconds()), Interval: int(pollInterval.Seconds()),
			}, nil
		}
		if !isUniqueViolation(err) {
			return nil, fmt.Errorf("storing device code: %w", err)
		}
	}
	return nil, errors.New("deviceauth: could not allocate a unique user code")
}

// Poll answers one poll. The key is minted here, on the first poll after
// approval, so its plaintext exists only in this one response.
func (s *Service) Poll(ctx context.Context, deviceCode string) (*TokenResult, error) {
	var (
		id, status, approver, name, roles, scopes, envs, expiresAt string
		keyExpires, lastPolled                                     sql.NullString
		approverID                                                 sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, status, approved_by_user_id, key_name, role_ids, scopes, environment_ids, key_expires_at, last_polled_at, expires_at
		 FROM device_codes WHERE device_code_hash = ?`, hashCode(deviceCode)).
		Scan(&id, &status, &approverID, &name, &roles, &scopes, &envs, &keyExpires, &lastPolled, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("reading device code: %w", err)
	}
	approver = approverID.String
	now := s.now().UTC()

	switch status {
	case "consumed":
		return nil, ErrInvalid
	case "denied":
		return nil, ErrDenied
	}
	if exp, err := time.Parse(time.RFC3339, expiresAt); err == nil && !now.Before(exp) {
		return nil, ErrExpired
	}
	if status == "pending" {
		if lastPolled.Valid {
			if last, err := time.Parse(time.RFC3339, lastPolled.String); err == nil && now.Sub(last) < pollInterval {
				return nil, ErrSlowDown
			}
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE device_codes SET last_polled_at = ?, updated_at = ? WHERE id = ?`,
			now.Format(time.RFC3339), now.Format(time.RFC3339), id); err != nil {
			return nil, fmt.Errorf("recording poll: %w", err)
		}
		return nil, ErrPending
	}

	res, err := s.db.ExecContext(ctx, `UPDATE device_codes SET status = 'consumed', updated_at = ? WHERE id = ? AND status = 'approved'`,
		now.Format(time.RFC3339), id)
	if err != nil {
		return nil, fmt.Errorf("consuming device code: %w", err)
	}
	if db.RowsAffected(res) != 1 {
		return nil, ErrInvalid
	}
	input := &apikeys.CreateAPIKeyInput{Name: name}
	if keyExpires.Valid {
		input.ExpiresAt = &keyExpires.String
	}
	for target, raw := range map[*[]string]string{&input.RoleIDs: roles, &input.Scopes: scopes, &input.EnvironmentIDs: envs} {
		if err := json.Unmarshal([]byte(raw), target); err != nil {
			return nil, fmt.Errorf("decoding approved grants: %w", err)
		}
	}
	key, err := s.keys.Create(ctx, approver, input)
	if err != nil {
		return nil, fmt.Errorf("minting approved key: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE device_codes SET api_key_id = ? WHERE id = ?`, key.ID, id); err != nil {
		return nil, fmt.Errorf("linking minted key: %w", err)
	}
	return key, nil
}

// Lookup returns a pending, unexpired request by the code a person typed.
func (s *Service) Lookup(ctx context.Context, userCode string) (*Request, error) {
	var req Request
	err := s.db.QueryRowContext(ctx,
		`SELECT user_code, requested_host, requested_os_user, expires_at, created_at FROM device_codes
		 WHERE user_code = ? AND status = 'pending' AND expires_at > ?`,
		NormalizeUserCode(userCode), s.now().UTC().Format(time.RFC3339)).
		Scan(&req.UserCode, &req.Host, &req.OSUser, &req.ExpiresAt, &req.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading device request: %w", err)
	}
	return &req, nil
}

// Approve records the key the approver agreed to. The caller has already
// checked the grants against the approver's own permissions.
func (s *Service) Approve(ctx context.Context, userCode, approverID string, in ApproveInput) error {
	grants := make([]string, 3)
	for i, v := range [][]string{in.RoleIDs, in.Scopes, in.EnvironmentIDs} {
		if v == nil {
			v = []string{}
		}
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encoding grants: %w", err)
		}
		grants[i] = string(b)
	}
	now := s.now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE device_codes SET status = 'approved', approved_by_user_id = ?, key_name = ?, role_ids = ?, scopes = ?,
		 environment_ids = ?, key_expires_at = ?, updated_at = ?
		 WHERE user_code = ? AND status = 'pending' AND expires_at > ?`,
		approverID, clean(in.Name), grants[0], grants[1], grants[2], in.ExpiresAt, now,
		NormalizeUserCode(userCode), now)
	return s.changedOne(res, err)
}

// Deny refuses a pending request; the terminal learns so on its next poll.
func (s *Service) Deny(ctx context.Context, userCode, approverID string) error {
	now := s.now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE device_codes SET status = 'denied', approved_by_user_id = ?, updated_at = ?
		 WHERE user_code = ? AND status = 'pending' AND expires_at > ?`,
		approverID, now, NormalizeUserCode(userCode), now)
	return s.changedOne(res, err)
}

func (s *Service) changedOne(res sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("updating device request: %w", err)
	}
	if db.RowsAffected(res) != 1 {
		return ErrNotFound
	}
	return nil
}

// prune drops requests an hour past expiry; a failure only leaves rows behind.
func (s *Service) prune(ctx context.Context) {
	cutoff := s.now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM device_codes WHERE expires_at < ?`, cutoff); err != nil {
		return
	}
}

// NormalizeUserCode accepts what a person types — any case, with or without the
// dash or spaces — and returns the stored XXXX-XXXX form.
func NormalizeUserCode(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if strings.ContainsRune(userCodeAlphabet, r) {
			b.WriteRune(r)
		}
	}
	code := b.String()
	if len(code) != 8 {
		return code
	}
	return code[:4] + "-" + code[4:]
}

// randomUserCode draws each letter by rejection sampling, so no letter of the
// alphabet is likelier than another.
func randomUserCode() (string, error) {
	limit := byte(256 - 256%len(userCodeAlphabet))
	out := make([]byte, 0, 8)
	buf := make([]byte, 16)
	for len(out) < 8 {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generating user code: %w", err)
		}
		for _, b := range buf {
			if b < limit && len(out) < 8 {
				out = append(out, userCodeAlphabet[int(b)%len(userCodeAlphabet)])
			}
		}
	}
	return string(out[:4]) + "-" + string(out[4:]), nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating device code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// clean bounds a caller-supplied label and strips control characters, since it
// is shown to an approver and must not be able to rewrite the page around it.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len([]rune(s)) > maxLabelLength {
		s = string([]rune(s)[:maxLabelLength])
	}
	return s
}

func isUniqueViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
