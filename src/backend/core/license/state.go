// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import "time"

// State is what an install's licensing looks like right now.
//
// None of these states gates a request. They pick a banner and a
// notice cadence, and they are recorded against each usage sample so the
// renewal conversation has facts rather than recollections.
type State string

const (
	// StateOK — licensed, inside the entitlement, not expired.
	StateOK State = "ok"
	// StateOver — more servers in use than the licence covers. Everything
	// keeps running; the customer is told.
	StateOver State = "over"
	// StateExpired — the licence's own validity has run out. Also keeps
	// running: a billing event is not an outage.
	StateExpired State = "expired"
	// StateUnlicensed — nothing activated yet.
	StateUnlicensed State = "unlicensed"
	// StateSuspended — an administrator paused the licence, usually over
	// billing. Reinstatement needs no new key, so the install keeps
	// checking in.
	StateSuspended State = "suspended"
	// StateRevoked — terminal. The key is dead and its seats were
	// released. The only way out is a new key.
	StateRevoked State = "revoked"
	// StatePending — an NFR licence McHarbor has not approved yet. The key
	// is real; its term has not started. Approval flips the same key live.
	StatePending State = "pending_approval"
	// StateNotStarted — the term begins in the future.
	StateNotStarted State = "not_started"
)

// StateForReason maps the licence server's machine-readable reason onto a
// local state. An unrecognised reason becomes StateExpired rather than
// StateOK: a licence the server declined to call valid must never read as
// healthy just because this build is older than the reason it was given.
func StateForReason(reason string) State {
	switch reason {
	case "suspended":
		return StateSuspended
	case "revoked":
		return StateRevoked
	case "pending_approval":
		return StatePending
	case "not_started":
		return StateNotStarted
	case "expired":
		return StateExpired
	default:
		return StateExpired
	}
}

// Terminal reports whether a state is one no amount of further checking
// in can improve. Only revocation is: everything else — an expiry, a
// suspension, an approval still pending — is resolved by somebody else
// acting on the same key.
func Terminal(state State) bool {
	return state == StateRevoked
}

// Evaluate decides the current state and maintains the over-since clock.
//
// The returned time is the moment usage first went over the entitlement,
// which is what the notice cadence counts its fourteen days from. It is
// carried forward while the install stays over and cleared the moment
// usage comes back inside the limit — a customer who removes the seventh
// server gets the clock reset, not a punished record.
//
// Expiry outranks overage: an install that is both expired and over is
// reported as expired, because renewing is the action that resolves both.
func Evaluate(
	licensed bool,
	used, limit int,
	expiresAt *time.Time,
	overSince *time.Time,
	now time.Time,
) (State, *time.Time) {
	over := limit != UnlimitedServers && used > limit

	nextOverSince := overSince
	switch {
	case !over:
		nextOverSince = nil
	case overSince == nil:
		at := now
		nextOverSince = &at
	}

	if !licensed {
		return StateUnlicensed, nextOverSince
	}
	if expiresAt != nil && !now.Before(*expiresAt) {
		return StateExpired, nextOverSince
	}
	if over {
		return StateOver, nextOverSince
	}
	return StateOK, nextOverSince
}

// Notice kinds. Each one is a distinct banner on its own cadence clock,
// and the ledger keys its showings off these strings.
const (
	NoticeExpired    = "expired"
	NoticeOverage    = "overage"
	NoticeUnlicensed = "unlicensed"
	NoticeSuspended  = "suspended"
	NoticeRevoked    = "revoked"
	NoticePending    = "pending_approval"
	NoticeNotStarted = "not_started"
)

// NoticeKind is the banner a state calls for, or the empty string when the
// state is one nobody needs interrupting about.
//
// An unlicensed install always produces a notice, whether or not anything
// is running yet: "no licence" is a standing condition, not a threshold
// that has to be crossed first.
//
// The count and limit are no longer consulted; they stay in the signature
// because every call site already has them and a future tier may want a
// threshold again.
func NoticeKind(state State, _, _ int) string {
	switch state {
	case StateExpired:
		return NoticeExpired
	case StateOver:
		return NoticeOverage
	case StateUnlicensed:
		return NoticeUnlicensed
	case StateSuspended:
		return NoticeSuspended
	case StateRevoked:
		return NoticeRevoked
	case StatePending:
		return NoticePending
	case StateNotStarted:
		return NoticeNotStarted
	case StateOK:
		return ""
	default:
		return ""
	}
}
