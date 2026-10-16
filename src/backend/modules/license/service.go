// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	corelicense "github.com/therealmcsparrow/mcharbor/core/license"
	"github.com/therealmcsparrow/mcharbor/core/licenseclient"
)

// ErrNoLicense is returned when an operation needs an activated licence
// and there is not one. It is not an error condition for the product —
// only for the operation that asked.
var ErrNoLicense = errors.New("no active licence")

// ErrKeyRequired is an activation with no key pasted into it.
var ErrKeyRequired = errors.New("licence key required")

// Service owns every licensing read and write. Handlers hold none of it.
type Service struct {
	db      *sql.DB
	enc     *encryption.Service
	logger  *slog.Logger
	version string

	// mu guards the cached status snapshot. The banner asks for it on
	// every page load, from every open tab; recomputing the counts and
	// the notice ledger each time would put a licensing query in front of
	// the product's most-loaded surface.
	mu       sync.RWMutex
	snapshot Status
	sampled  bool

	// backoff grows while the licence server is unreachable so an outage
	// does not turn every install on earth into a retry storm.
	backoffMu    sync.Mutex
	backoffUntil time.Time
	backoffSteps int
}

// NewService constructs the licence service. It takes the database and
// the encryption service specifically, not the whole of AppDeps.
func NewService(database *sql.DB, enc *encryption.Service, logger *slog.Logger, version string) *Service {
	return &Service{
		db:      database,
		enc:     enc,
		logger:  logger,
		version: version,
		snapshot: Status{
			Tier:         string(corelicense.TierUnlicensed),
			State:        string(corelicense.StateUnlicensed),
			ServersLimit: corelicense.ServersFor(corelicense.TierUnlicensed),
		},
	}
}

// --- Instance identity ----------------------------------------------------

// EnsureInstallKey mints this install's instance id on the first boot that
// finds none. Idempotent, and safe to call from every node in a cluster:
// the insert is conditional on the table still being empty, so a race
// leaves exactly one id.
//
// The id is the seat. It has to survive restarts and upgrades, which is
// why it lives in the database rather than being derived from a hostname
// or a container id that a redeploy would change underneath it.
func (s *Service) EnsureInstallKey(ctx context.Context) error {
	var existing int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM license_installs`).Scan(&existing); err != nil {
		return fmt.Errorf("checking instance id: %w", err)
	}
	if existing > 0 {
		return nil
	}

	key, err := corelicense.NewInstallKey()
	if err != nil {
		return err
	}
	sealed, err := s.enc.Encrypt(key)
	if err != nil {
		return fmt.Errorf("encrypting instance id: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO license_installs (id, install_key, key_masked, created_at, updated_at)
		 SELECT ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (SELECT 1 FROM license_installs)`,
		xid.New().String(), sealed, corelicense.Mask(key), now, now,
	); err != nil {
		return fmt.Errorf("storing instance id: %w", err)
	}
	return nil
}

// InstanceID returns this installation's seat identity.
//
// Unlike the licence key, this is not a credential: it identifies a seat,
// and an operator releasing that seat from the mcharbor.eu dashboard has
// to be able to read it in full. It is stored encrypted anyway, because
// it shares a table with a column that predates the distinction and
// re-encrypting nothing costs nothing.
func (s *Service) InstanceID(ctx context.Context) (string, error) {
	var sealed string
	err := s.db.QueryRowContext(ctx,
		`SELECT install_key FROM license_installs ORDER BY created_at LIMIT 1`,
	).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading instance id: %w", err)
	}
	id, err := s.enc.Decrypt(sealed)
	if err != nil {
		return "", fmt.Errorf("decrypting instance id: %w", err)
	}
	return id, nil
}

// hostname is what the operator reads in the licence dashboard to tell
// two seats apart. A host that will not name itself is not worth failing
// an activation over.
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// --- Settings -------------------------------------------------------------

// setting reads one `license.*` value, falling back to the supplied
// default when the row is missing or empty.
func (s *Service) setting(ctx context.Context, key, fallback string) string {
	var value sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil {
		return fallback
	}
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return fallback
	}
	return strings.TrimSpace(value.String)
}

func (s *Service) settingInt(ctx context.Context, key string, fallback int) int {
	parsed, err := strconv.Atoi(s.setting(ctx, key, ""))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// SampleInterval is how often the local usage sampler runs.
func (s *Service) SampleInterval(ctx context.Context) time.Duration {
	return time.Duration(s.settingInt(ctx, "license.sample_interval_minutes", 60)) * time.Minute
}

// CheckInInterval is how often the licence server is contacted. Daily is
// what the handshake asks for, and the check-in doubles as the heartbeat
// that keeps this seat alive.
func (s *Service) CheckInInterval(ctx context.Context) time.Duration {
	return time.Duration(s.settingInt(ctx, "license.heartbeat_interval_hours", 24)) * time.Hour
}

// client builds a licence-server client from the current settings.
//
// There is no API key: the three instance endpoints authenticate on the
// licence key in the body. A client with no URL answers ErrNotConfigured
// to everything, which is the right answer for an operator who has
// deliberately pointed licensing at nothing.
func (s *Service) client(ctx context.Context) *licenseclient.Client {
	return licenseclient.New(s.setting(ctx, "license.server_url", ""))
}

// ServerConfigured reports whether there is a licence server to talk to.
func (s *Service) ServerConfigured(ctx context.Context) bool {
	return s.client(ctx).Configured()
}

// --- Licence record -------------------------------------------------------

// licenseRow is the internal read shape, including the fields the API
// deliberately never returns.
type licenseRow struct {
	id            string
	key           string
	keyMasked     string
	serverID      string
	tier          string
	status        string
	valid         bool
	reason        sql.NullString
	seatsTotal    int
	seatsUsed     int
	seatsUnlim    bool
	perpetual     bool
	startsAt      sql.NullString
	expiresAt     sql.NullString
	daysRemaining sql.NullInt64
	autoRenew     bool
	registered    bool
	holdsSeat     bool
	activatedAt   string
	lastChecked   sql.NullString
	lastError     sql.NullString
}

const licenseSelect = `SELECT id, license_key, key_masked, server_license_id, tier, status,
	valid, reason, seats_total, seats_used, seats_unlimited,
	perpetual, starts_at, expires_at, days_remaining, auto_renew,
	instance_registered, holds_seat, activated_at, last_checked_at, last_error
	FROM licenses WHERE active = 1 ORDER BY created_at DESC LIMIT 1`

// activeRow reads the one active licence, or nil when there is none. The
// licence key comes back decrypted; no caller outside this package sees it.
func (s *Service) activeRow(ctx context.Context) (*licenseRow, error) {
	var r licenseRow
	var sealed string

	err := s.db.QueryRowContext(ctx, licenseSelect).Scan(
		&r.id, &sealed, &r.keyMasked, &r.serverID, &r.tier, &r.status,
		&r.valid, &r.reason, &r.seatsTotal, &r.seatsUsed, &r.seatsUnlim,
		&r.perpetual, &r.startsAt, &r.expiresAt, &r.daysRemaining, &r.autoRenew,
		&r.registered, &r.holdsSeat, &r.activatedAt, &r.lastChecked, &r.lastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading active licence: %w", err)
	}

	key, err := s.enc.Decrypt(sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypting licence key: %w", err)
	}
	r.key = key
	return &r, nil
}

// seatLimit reconciles the two sources of a ceiling: the seat allowance
// on the licence, and the tier's own policy.
//
// The licence wins whenever it says anything. `seats.unlimited` is the
// unmetered case — the homelab and non-commercial licences, where the
// count is informational. A licence that arrives with neither an
// unlimited flag nor a positive total falls back to unlimited rather than
// to zero, deliberately: the alternative is to nag a paying customer
// because our own server sent an incomplete payload, and between those
// two failures that one is worse.
func seatLimit(row *licenseRow, tier corelicense.Tier) int {
	if row == nil {
		return corelicense.ServersFor(tier)
	}
	if row.seatsUnlim {
		return corelicense.UnlimitedServers
	}
	if row.seatsTotal > 0 {
		return row.seatsTotal
	}
	return corelicense.UnlimitedServers
}

func (r *licenseRow) toLicense() *License {
	return &License{
		ID:              r.id,
		KeyMasked:       r.keyMasked,
		ServerLicenseID: r.serverID,
		Tier:            r.tier,
		Status:          r.status,
		Valid:           r.valid,
		Reason:          r.reason.String,
		Seats: Seats{
			Unlimited: r.seatsUnlim,
			Total:     r.seatsTotal,
			Used:      r.seatsUsed,
		},
		Term: Term{
			Perpetual:     r.perpetual,
			StartsAt:      r.startsAt.String,
			ExpiresAt:     r.expiresAt.String,
			DaysRemaining: int(r.daysRemaining.Int64),
			AutoRenew:     r.autoRenew,
		},
		InstanceRegistered: r.registered,
		HoldsSeat:          r.holdsSeat,
		ActivatedAt:        r.activatedAt,
		LastCheckedAt:      r.lastChecked.String,
		LastError:          r.lastError.String,
	}
}

// Detail returns the licence page's payload.
func (s *Service) Detail(ctx context.Context) (Detail, error) {
	row, err := s.activeRow(ctx)
	if err != nil {
		return Detail{}, err
	}
	instanceID, err := s.InstanceID(ctx)
	if err != nil {
		return Detail{}, err
	}

	detail := Detail{
		InstanceID:       instanceID,
		InstanceType:     licenseclient.InstanceServer,
		Hostname:         hostname(),
		ServerConfigured: s.ServerConfigured(ctx),
	}
	if row != nil {
		detail.License = row.toLicense()
	}
	return detail, nil
}

// --- Activation -----------------------------------------------------------

// Activate claims a seat with the key the operator pasted, and stores what
// comes back. Any previous licence is retired in the same transaction so
// there is never more than one active row.
//
// Repeating an activation for the same instance id is safe by design: the
// licence server refreshes the details and consumes no second seat, which
// is what makes "paste the key again" a valid recovery for an install
// whose seat was released.
func (s *Service) Activate(ctx context.Context, input ActivateInput, userID string) (*License, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return nil, ErrKeyRequired
	}

	instanceID, err := s.InstanceID(ctx)
	if err != nil {
		return nil, err
	}
	if instanceID == "" {
		return nil, fmt.Errorf("no instance id for this install")
	}

	resp, err := s.client(ctx).Activate(ctx, licenseclient.ActivateRequest{
		Key:          key,
		InstanceID:   instanceID,
		InstanceType: licenseclient.InstanceServer,
		Hostname:     hostname(),
		Version:      s.version,
		MachineID:    licenseclient.MachineID(),
	})
	if err != nil {
		return nil, err
	}

	if err := s.store(ctx, key, resp, userID); err != nil {
		return nil, err
	}
	s.clearBackoff()
	s.invalidate()

	row, err := s.activeRow(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNoLicense
	}
	return row.toLicense(), nil
}

// store writes an activation response, retiring any previous licence.
func (s *Service) store(ctx context.Context, key string, resp *licenseclient.Response, userID string) error {
	sealed, err := s.enc.Encrypt(key)
	if err != nil {
		return fmt.Errorf("encrypting licence key: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	activated := resp.ActivatedAt
	if activated == "" {
		activated = now
	}

	info := resp.License
	if info == nil {
		info = &licenseclient.LicenseInfo{}
	}

	masked := info.Key
	if masked == "" {
		masked = corelicense.MaskLicenseKey(key)
	}

	tier := string(corelicense.TierUnlicensed)
	if parsed, ok := corelicense.ParseTier(info.Type); ok {
		tier = string(parsed)
	}

	holdsSeat := false
	registered := resp.InstanceRegistered
	if resp.Instance != nil {
		holdsSeat = resp.Instance.HoldsSeat
		registered = registered || resp.Instance.HoldsSeat
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting licence transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			s.logger.Warn("licence: rolling back", "error", err)
		}
	}()

	if _, err := tx.ExecContext(ctx,
		`UPDATE licenses SET active = 0, updated_at = ? WHERE active = 1`, now,
	); err != nil {
		return fmt.Errorf("retiring previous licence: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO licenses (id, license_key, key_masked, server_license_id, tier, status,
			valid, reason, seats_total, seats_used, seats_unlimited,
			perpetual, starts_at, expires_at, days_remaining, auto_renew,
			instance_registered, holds_seat, activated_at, last_checked_at,
			activated_by, active, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		xid.New().String(), sealed, masked, info.ID, tier, statusOrPending(info.Status),
		boolToInt(resp.Valid), nullable(resp.Reason),
		info.Seats.Total, info.Seats.Used, boolToInt(info.Seats.Unlimited),
		boolToInt(info.Term.Perpetual), nullable(info.Term.StartsAt), nullable(info.Term.ExpiresAt),
		info.Term.DaysRemaining, boolToInt(info.Term.AutoRenew),
		boolToInt(registered), boolToInt(holdsSeat), activated, now,
		nullable(userID), now, now,
	); err != nil {
		return fmt.Errorf("storing licence: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing licence: %w", err)
	}
	return nil
}

// Deactivate gives the seat back, telling the licence server first. A
// licence server that cannot be reached does not block the local
// deactivation — the operator asked, and an unreachable host frees its
// own seat after 30 quiet days anyway.
func (s *Service) Deactivate(ctx context.Context, userID string) error {
	row, err := s.activeRow(ctx)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrNoLicense
	}

	instanceID, err := s.InstanceID(ctx)
	if err != nil {
		return err
	}

	relErr := s.client(ctx).Deactivate(ctx, licenseclient.DeactivateRequest{
		Key:          row.key,
		InstanceID:   instanceID,
		InstanceType: licenseclient.InstanceServer,
		MachineID:    licenseclient.MachineID(),
	})
	// An instance the licence server never registered is not a failure:
	// there is nothing to give back, and the local row still has to go.
	if relErr != nil && !errors.Is(relErr, licenseclient.ErrInstanceNotFound) {
		s.logger.Warn("licence: releasing the seat on the licence server failed",
			"error", relErr, "license", row.serverID, "actor", userID)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE licenses SET active = 0, status = 'released', holds_seat = 0,
			instance_registered = 0, updated_at = ? WHERE id = ?`,
		now, row.id,
	)
	if err != nil {
		return fmt.Errorf("deactivating licence: %w", err)
	}
	if coredb.RowsAffected(result) == 0 {
		return ErrNoLicense
	}
	s.clearBackoff()
	s.invalidate()
	return nil
}

// --- Sampling -------------------------------------------------------------

// Sample records one usage reading and refreshes the cached status. It is
// the coordinator job body, so it takes no error return: a failure is
// logged and the next tick tries again. Nothing downstream waits on it.
func (s *Service) Sample(ctx context.Context) {
	if _, err := s.sampleOnce(ctx); err != nil {
		s.logger.Error("licence: usage sample failed", "error", err)
	}
}

// sampleOnce does the work and hands back the status it computed, so the
// HTTP path can force a fresh reading without going through the logger.
func (s *Service) sampleOnce(ctx context.Context) (Status, error) {
	c, err := s.count(ctx)
	if err != nil {
		return Status{}, err
	}

	row, err := s.activeRow(ctx)
	if err != nil {
		return Status{}, err
	}

	tier := corelicense.TierUnlicensed
	licensed := false
	var expiresAt *time.Time
	var licenseID any
	daysRemaining := 0

	if row != nil {
		licensed = true
		licenseID = row.id
		daysRemaining = int(row.daysRemaining.Int64)
		if parsed, ok := corelicense.ParseTier(row.tier); ok {
			tier = parsed
		}
		if !row.perpetual && row.expiresAt.Valid {
			if at, parseErr := time.Parse(time.RFC3339, row.expiresAt.String); parseErr == nil {
				expiresAt = &at
			}
		}
	}

	limit := seatLimit(row, tier)

	now := time.Now().UTC()
	previousOver, err := s.lastOverSince(ctx)
	if err != nil {
		return Status{}, err
	}

	state, overSince := corelicense.Evaluate(licensed, c.servers, limit, expiresAt, previousOver, now)

	// The licence server's verdict outranks anything derived locally. It
	// knows about suspensions, revocations and approvals that no local
	// clock can infer, and it is the only party allowed to say a licence
	// stopped being valid.
	if row != nil && !row.valid {
		state = corelicense.StateForReason(row.reason.String)
	}

	sampledAt := now.Format(time.RFC3339)
	if err := s.recordUsage(ctx, usageRecord{
		licenseID: licenseID,
		counts:    c,
		limit:     limit,
		state:     state,
		overSince: overSince,
		sampledAt: sampledAt,
		now:       now,
	}); err != nil {
		return Status{}, err
	}

	status := Status{
		Tier:          string(tier),
		State:         string(state),
		ServersUsed:   c.servers,
		ServersLimit:  limit,
		DaysRemaining: daysRemaining,
		ContactURL:    s.setting(ctx, "license.contact_url", defaultContactURL),
		SampledAt:     sampledAt,
	}
	if expiresAt != nil {
		status.ExpiresAt = expiresAt.Format(time.RFC3339)
	}
	if row != nil {
		status.OfflineDays = offlineDays(row.lastChecked, now)
	}

	if err := s.applyNotice(ctx, &status, state, overSince, expiresAt, now); err != nil {
		return Status{}, err
	}

	s.mu.Lock()
	s.snapshot = status
	s.sampled = true
	s.mu.Unlock()

	return status, nil
}

// UsageRetention is how long one usage record stands for. The sampler
// still runs hourly — the status the banner reads has to stay current, and
// the over-since clock has to keep moving — but the ledger gets one row
// per day, updated in place until the day is out.
//
// Anything shorter turned the renewal evidence into thousands of near
// identical rows nobody could read.
const UsageRetention = 24 * time.Hour

// usageRecord is one reading on its way to the ledger.
type usageRecord struct {
	licenseID any
	counts    counts
	limit     int
	state     corelicense.State
	overSince *time.Time
	sampledAt string
	now       time.Time
}

// recordUsage writes at most one usage row per day.
//
// Inside the day's window the newest row is updated in place, so it always
// reflects the latest reading and the over-since clock survives across
// samples; once the window is out, a new row is inserted and the previous
// day's record is left untouched as evidence.
func (s *Service) recordUsage(ctx context.Context, rec usageRecord) error {
	var id string
	var rawSampledAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, sampled_at FROM license_usage_samples ORDER BY sampled_at DESC LIMIT 1`,
	).Scan(&id, &rawSampledAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reading latest usage sample: %w", err)
	}

	withinDay := false
	if err == nil {
		if at, parseErr := time.Parse(time.RFC3339, rawSampledAt); parseErr == nil {
			withinDay = rec.now.Sub(at) < UsageRetention
		}
	}

	if withinDay {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE license_usage_samples
			 SET license_id = ?, servers_used = ?, servers_limit = ?, agent_count = ?,
				 env_count = ?, user_count = ?, state = ?, over_since = ?, updated_at = ?
			 WHERE id = ?`,
			rec.licenseID, rec.counts.servers, rec.limit, rec.counts.agents,
			rec.counts.environments, rec.counts.users, string(rec.state),
			timeOrNil(rec.overSince), rec.sampledAt, id,
		); err != nil {
			return fmt.Errorf("updating usage sample: %w", err)
		}
		return nil
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO license_usage_samples (id, license_id, servers_used, servers_limit,
			agent_count, env_count, user_count, state, over_since, reported, sampled_at,
			created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		xid.New().String(), rec.licenseID, rec.counts.servers, rec.limit, rec.counts.agents,
		rec.counts.environments, rec.counts.users, string(rec.state),
		timeOrNil(rec.overSince), rec.sampledAt, rec.sampledAt, rec.sampledAt,
	); err != nil {
		return fmt.Errorf("recording usage sample: %w", err)
	}
	return nil
}

// defaultContactURL is where a banner sends somebody who needs to renew,
// extend, or ask about a suspension. The licence server no longer returns
// one per licence, so it is a setting with a sensible default rather than
// a field nobody fills in.
const defaultContactURL = "https://www.mcharbor.eu/licenses"

// lastOverSince carries the over-since clock forward between samples.
func (s *Service) lastOverSince(ctx context.Context) (*time.Time, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT over_since FROM license_usage_samples ORDER BY sampled_at DESC LIMIT 1`,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading over-since clock: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	at, parseErr := time.Parse(time.RFC3339, raw.String)
	if parseErr == nil {
		return &at, nil
	}
	// An over-since timestamp that no longer parses reads as not over.
	return nil, nil
}

// applyNotice fills the notice fields on the status the banner reads.
// NoticeKind is set for every non-OK state and is what makes the band
// visible; NoticeDue is only the ledger cadence.
func (s *Service) applyNotice(
	ctx context.Context,
	status *Status,
	state corelicense.State,
	overSince, expiresAt *time.Time,
	now time.Time,
) error {
	kind := corelicense.NoticeKind(state, status.ServersUsed, status.ServersLimit)
	if kind == "" {
		return nil
	}

	// The clock the cadence counts from: when the overage started, or
	// when the licence lapsed.
	since := time.Time{}
	switch {
	case overSince != nil:
		since = *overSince
	case state == corelicense.StateExpired && expiresAt != nil:
		since = *expiresAt
	}

	lastShown, err := s.lastNotice(ctx, kind)
	if err != nil {
		return err
	}

	// The cadence no longer decides whether the band shows — a standing
	// condition shows for as long as it stands — only how often a showing
	// is written to the ledger.
	due, phase := corelicense.NextShowing(since, lastShown, now)

	status.NoticeKind = kind
	status.NoticePhase = phase
	status.NoticeDue = due
	status.noticeSince = since
	return nil
}

func (s *Service) lastNotice(ctx context.Context, kind string) (time.Time, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT shown_at FROM license_notices WHERE kind = ? ORDER BY shown_at DESC LIMIT 1`, kind,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("reading notice ledger: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return time.Time{}, nil
	}
	at, parseErr := time.Parse(time.RFC3339, raw.String)
	if parseErr == nil {
		return at, nil
	}
	// A ledger timestamp that no longer parses reads as never shown.
	return time.Time{}, nil
}

// --- Status ---------------------------------------------------------------

// Status returns the cached snapshot, sampling once if nothing has run
// yet. Every authenticated user may read this, so it carries counts and
// state and nothing else.
func (s *Service) Status(ctx context.Context) (Status, error) {
	s.mu.RLock()
	cached, sampled := s.snapshot, s.sampled
	s.mu.RUnlock()

	if sampled {
		return cached, nil
	}
	return s.sampleOnce(ctx)
}

// invalidate forces the next Status call to recompute. Called after any
// change that could move the entitlement.
func (s *Service) invalidate() {
	s.mu.Lock()
	s.sampled = false
	s.mu.Unlock()
}

// StatusFor is Status with NoticeDue worked out for userID: each person's
// dismissal closes their own cadence window, never anyone else's.
func (s *Service) StatusFor(ctx context.Context, userID string) (Status, error) {
	st, err := s.Status(ctx)
	if err != nil || st.NoticeKind == "" || userID == "" {
		return st, err
	}
	last, err := s.lastNoticeFor(ctx, userID, st.NoticeKind)
	if err != nil {
		return st, err
	}
	st.NoticeDue, _ = corelicense.NextShowing(st.noticeSince, last, time.Now().UTC())
	return st, nil
}

// AcknowledgeNotice records that userID was shown a banner, which closes their
// cadence window and leaves attributed proof they were told. It writes only
// when the notice is due for them, so a repeated ack is not another row, and it
// no longer touches the shared snapshot: one person cannot hide it for everyone.
// recorded reports whether a row was written. Kinds outside the known set are
// refused rather than stored.
func (s *Service) AcknowledgeNotice(ctx context.Context, userID, kind string) (recorded bool, err error) {
	switch kind {
	case "overage", "expired", "unlicensed", "suspended", "revoked", "pending_approval", "not_started":
	default:
		return false, fmt.Errorf("unknown notice kind %q", kind)
	}
	if userID == "" {
		return false, errors.New("acknowledging a notice needs a user")
	}

	s.mu.RLock()
	snapshot := s.snapshot
	s.mu.RUnlock()

	last, err := s.lastNoticeFor(ctx, userID, kind)
	if err != nil {
		return false, err
	}
	due, phase := corelicense.NextShowing(snapshot.noticeSince, last, time.Now().UTC())
	if !due && !last.IsZero() {
		return false, nil
	}
	if phase == "" {
		phase = corelicense.PhaseDaily
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO license_notices (id, kind, phase, servers_used, servers_limit,
			shown_at, user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), kind, phase, snapshot.ServersUsed, snapshot.ServersLimit,
		now, userID, now, now,
	); err != nil {
		return false, fmt.Errorf("recording notice: %w", err)
	}
	return true, nil
}

// lastNoticeFor is lastNotice for one user's own acknowledgements.
func (s *Service) lastNoticeFor(ctx context.Context, userID, kind string) (time.Time, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT shown_at FROM license_notices WHERE kind = ? AND user_id = ? ORDER BY shown_at DESC LIMIT 1`, kind, userID,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("reading notice ledger: %w", err)
	}
	at, parseErr := time.Parse(time.RFC3339, raw.String)
	if parseErr == nil {
		return at, nil
	}
	// A ledger timestamp that no longer parses reads as never shown.
	return time.Time{}, nil
}

// --- Usage history --------------------------------------------------------

// UsageHistory returns recorded samples, newest first. Local evidence for
// the renewal conversation — the handshake has no usage endpoint, and
// nothing here is ever uploaded.
func (s *Service) UsageHistory(ctx context.Context, page, perPage int) ([]Usage, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM license_usage_samples`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting usage samples: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, servers_used, servers_limit, agent_count, env_count, user_count,
			state, over_since, sampled_at
		 FROM license_usage_samples
		 ORDER BY sampled_at DESC
		 LIMIT ? OFFSET ?`,
		perPage, (page-1)*perPage,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("listing usage samples: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("licence: closing usage rows", "error", err)
		}
	}()

	items := make([]Usage, 0, perPage)
	for rows.Next() {
		var u Usage
		var overSince sql.NullString
		if err := rows.Scan(&u.ID, &u.ServersUsed, &u.ServersLimit, &u.AgentCount,
			&u.EnvCount, &u.UserCount, &u.State, &overSince, &u.SampledAt); err != nil {
			return nil, 0, fmt.Errorf("scanning usage sample: %w", err)
		}
		u.OverSince = overSince.String
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("reading usage samples: %w", err)
	}
	return items, total, nil
}

// --- Helpers --------------------------------------------------------------

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func timeOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return at.UTC().Format(time.RFC3339)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func statusOrPending(status string) string {
	if strings.TrimSpace(status) == "" {
		return "pending"
	}
	return status
}

// offlineDays is how long it has been since the licence server last
// answered anything. Zero while contact is current, and zero for an
// install that has never had a licence to check in for.
//
// It is reported, never enforced: the handshake has no signed offline
// token, so an install that cannot reach the licence server keeps running
// on its cached entitlement and simply says how stale that is.
func offlineDays(lastChecked sql.NullString, now time.Time) int {
	if !lastChecked.Valid || lastChecked.String == "" {
		return 0
	}
	at, err := time.Parse(time.RFC3339, lastChecked.String)
	if err != nil {
		return 0
	}
	days := int(now.Sub(at).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}
