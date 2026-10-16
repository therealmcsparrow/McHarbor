// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	corelicense "github.com/therealmcsparrow/mcharbor/core/license"
	"github.com/therealmcsparrow/mcharbor/core/licenseclient"
)

// maxBackoffSteps caps the exponential backoff at 2^6 = 64 minutes. Past
// that there is no value in backing off further; the server is down, not
// rate-limiting us.
const maxBackoffSteps = 6

// CheckIn re-confirms the licence with the licence server and refreshes
// the local entitlement.
//
// It is a coordinator job body, so it returns nothing: every failure is
// recorded against the licence row and retried later. No path in this
// function can change what the product will do for its user — an
// unreachable licence server says nothing about the licence, and the
// cached entitlement is kept exactly as it was.
func (s *Service) CheckIn(ctx context.Context) {
	if err := s.checkInOnce(ctx); err != nil {
		if errors.Is(err, licenseclient.ErrNotConfigured) || errors.Is(err, ErrNoLicense) {
			// Nothing to talk to, or nothing to talk about. Not a fault.
			return
		}
		s.logger.Warn("licence: check-in failed", "error", err)
	}
}

// RefreshNow runs a check-in immediately, ignoring any backoff. This is
// the support path behind POST /license/refresh: somebody has just fixed
// the far side and wants to see it reflected without waiting a day.
func (s *Service) RefreshNow(ctx context.Context) error {
	s.clearBackoff()
	if err := s.checkInOnce(ctx); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// checkInOnce is the client state machine from the handshake, in one
// place. The order of the branches is the contract:
//
//  1. A transport failure changes nothing at all.
//  2. An unknown key is a typo, not a revocation.
//  3. A revoked licence is terminal — stop asking.
//  4. Every other `valid: false` reason keeps checking in, because
//     somebody else acting on the same key is what resolves it.
//  5. A valid licence whose instance is no longer registered has lost its
//     seat, and claims one again rather than heartbeating into the void.
func (s *Service) checkInOnce(ctx context.Context) error {
	if s.inBackoff() {
		return nil
	}

	row, err := s.activeRow(ctx)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrNoLicense
	}

	// Revocation is terminal and every seat on the key was released with
	// it. Continuing to ask would be traffic that cannot change anything.
	if !row.valid && row.reason.String == licenseclient.ReasonRevoked {
		return nil
	}

	client := s.client(ctx)
	if !client.Configured() {
		return licenseclient.ErrNotConfigured
	}

	instanceID, err := s.InstanceID(ctx)
	if err != nil {
		return err
	}

	resp, err := client.Check(ctx, licenseclient.CheckRequest{
		Key:        row.key,
		InstanceID: instanceID,
		Version:    s.version,
		MachineID:  licenseclient.MachineID(),
	})
	if err != nil {
		s.recordCheckInError(ctx, row.id, err)
		// Only an unreachable server earns a backoff. A definitive 4xx
		// will answer identically on the next tick, and the tick is a day
		// apart — there is nothing to protect the far side from.
		if errors.Is(err, licenseclient.ErrUnavailable) || errors.Is(err, licenseclient.ErrRateLimited) {
			s.backOff()
		}
		return err
	}

	s.clearBackoff()
	if err := s.applyCheck(ctx, row.id, resp); err != nil {
		return err
	}
	s.invalidate()

	// The seat is gone: an operator released it, or this host went quiet
	// for the 30 days the licence server waits before reclaiming one.
	// Claiming it again is the documented response; heartbeating against
	// a seat we no longer hold is not.
	if resp.Valid && !resp.InstanceRegistered {
		s.logger.Info("licence: seat no longer registered, claiming it again",
			"instance", instanceID)
		if _, actErr := s.Activate(ctx, ActivateInput{Key: row.key}, ""); actErr != nil {
			s.logger.Warn("licence: re-activation failed", "error", actErr)
			return actErr
		}
	}
	return nil
}

// applyCheck writes back whatever the licence server just said: a renewed
// term, an approved NFR licence, a suspension, a revocation, a changed
// seat count. None of it gates anything locally.
func (s *Service) applyCheck(ctx context.Context, licenseID string, resp *licenseclient.Response) error {
	now := time.Now().UTC().Format(time.RFC3339)

	info := resp.License
	if info == nil {
		info = &licenseclient.LicenseInfo{}
	}

	// An unknown type from a newer licence server is not a reason to
	// corrupt the local row: keep the tier we have and carry on.
	tier := ""
	if parsed, ok := corelicense.ParseTier(info.Type); ok {
		tier = string(parsed)
	}

	holdsSeat := resp.InstanceRegistered
	if resp.Instance != nil {
		holdsSeat = resp.Instance.HoldsSeat
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE licenses
		    SET status = ?,
		        tier = CASE WHEN ? = '' THEN tier ELSE ? END,
		        valid = ?,
		        reason = ?,
		        seats_total = ?,
		        seats_used = ?,
		        seats_unlimited = ?,
		        perpetual = ?,
		        starts_at = COALESCE(?, starts_at),
		        expires_at = ?,
		        days_remaining = ?,
		        auto_renew = ?,
		        instance_registered = ?,
		        holds_seat = ?,
		        last_checked_at = ?,
		        last_error = NULL,
		        updated_at = ?
		  WHERE id = ?`,
		statusOrPending(info.Status), tier, tier,
		boolToInt(resp.Valid), nullable(resp.Reason),
		info.Seats.Total, info.Seats.Used, boolToInt(info.Seats.Unlimited),
		boolToInt(info.Term.Perpetual), nullable(info.Term.StartsAt), nullable(info.Term.ExpiresAt),
		info.Term.DaysRemaining, boolToInt(info.Term.AutoRenew),
		boolToInt(resp.InstanceRegistered), boolToInt(holdsSeat),
		now, now, licenseID,
	); err != nil {
		return err
	}
	return nil
}

// recordCheckInError leaves a *classified* failure on the licence row so
// the licence tab can say what is wrong.
//
// The stored value is one of a fixed set of tokens the frontend
// translates, never the Go error: `lastError` is returned by GET /license,
// and a wrapped transport error would put the licence server URL and
// whatever the far side said into an API response.
func (s *Service) recordCheckInError(ctx context.Context, licenseID string, cause error) {
	reason := checkInErrorReason(cause)
	s.logger.Warn("licence: check-in error", "reason", reason, "error", cause)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE licenses SET last_error = ?, updated_at = ? WHERE id = ?`,
		reason, now, licenseID,
	); err != nil {
		s.logger.Error("licence: recording the check-in error failed", "error", err)
	}
}

// checkInErrorReason collapses any failure onto a token the licence tab
// can translate. Anything unrecognised becomes "unknown" rather than
// leaking its text.
func checkInErrorReason(cause error) string {
	switch {
	case errors.Is(cause, licenseclient.ErrNotConfigured):
		return "not_configured"
	case errors.Is(cause, licenseclient.ErrNoKey):
		return "no_key"
	case errors.Is(cause, licenseclient.ErrUnknownKey):
		return "unknown_key"
	case errors.Is(cause, licenseclient.ErrInstanceNotFound):
		return "instance_not_found"
	case errors.Is(cause, licenseclient.ErrSeatLimit):
		return "seat_limit_reached"
	case errors.Is(cause, licenseclient.ErrRefused):
		if reason := licenseclient.Reason(cause); reason != "" {
			return reason
		}
		return "refused"
	case errors.Is(cause, licenseclient.ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(cause, licenseclient.ErrRateLimited):
		return "rate_limited"
	case errors.Is(cause, licenseclient.ErrUnavailable):
		return "unavailable"
	default:
		return "unknown"
	}
}

// --- Backoff --------------------------------------------------------------

func (s *Service) inBackoff() bool {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()
	return time.Now().Before(s.backoffUntil)
}

// backOff doubles the wait after each consecutive failure, with jitter so
// a fleet of installs that all lost the licence server at the same moment
// does not return in lockstep.
func (s *Service) backOff() {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()

	if s.backoffSteps < maxBackoffSteps {
		s.backoffSteps++
	}
	base := time.Duration(1<<s.backoffSteps) * time.Minute
	jitter := time.Duration(rand.Int64N(int64(base / 4))) //nolint:gosec // backoff jitter, not security-sensitive
	s.backoffUntil = time.Now().Add(base + jitter)
}

func (s *Service) clearBackoff() {
	s.backoffMu.Lock()
	defer s.backoffMu.Unlock()
	s.backoffSteps = 0
	s.backoffUntil = time.Time{}
}
