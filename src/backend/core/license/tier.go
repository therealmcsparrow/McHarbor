// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package license holds the pure licensing rules: which tiers exist, what
// state an install is in, and when it is due to be told about it.
//
// Nothing here touches a database, a clock it did not receive, or the
// network. That is the point — the two genuinely tricky rules in this
// feature (the state machine and the notice cadence) are the ones a bug
// would put in front of a paying customer, so they are written as pure
// functions that a table test can exhaust.
package license

// Tier is one of the six licence kinds McHarbor issues.
type Tier string

// The licence tiers, from no licence at all through enterprise.
const (
	// TierUnlicensed is the absence of a licence, not a licence of its
	// own. It is entitled to nothing, so every running server is over —
	// which is exactly what makes an unlicensed install say so.
	TierUnlicensed    Tier = "unlicensed"
	TierHomelab       Tier = "homelab"
	TierNonCommercial Tier = "non_commercial"
	TierTrial         Tier = "trial"
	TierNFR           Tier = "nfr"
	TierCommercial    Tier = "commercial"
	TierEnterprise    Tier = "enterprise"
)

// UnlimitedServers is the entitlement value meaning "no ceiling".
//
// Zero is a real ceiling of zero, and the two must never be conflated:
// zero is what an unlicensed install gets, and unlimited is what the free
// tiers get. Getting this backwards would either nag every homelab user
// forever or hand every unlicensed install an unlimited entitlement.
const UnlimitedServers = -1

// FreeTierServers is the ceiling on the time-boxed free tiers. Trial and
// NFR are evaluation licences, so they are capped; homelab and
// non-commercial are perpetual and uncapped.
const FreeTierServers = 5

// ServersFor is the entitlement a tier carries by policy, before the
// licence server has a say.
//
//   - unlicensed          0          nothing is entitled; say so
//   - homelab             unlimited  free, perpetual, uncapped
//   - non_commercial      unlimited  free, perpetual, uncapped
//   - trial               5          time-boxed evaluation
//   - nfr                 5          time-boxed evaluation
//   - commercial          per licence
//   - enterprise          per licence
//
// This is the fallback, not the answer. A licence carries its own seat
// allowance (`seats.total`, or `seats.unlimited`), and that is what the
// caller prefers whenever the licence server has spoken. Policy applies
// to an install with no licence to read, and to the free tiers the seat
// count is only informational for. See modules/license.seatLimit.
func ServersFor(t Tier) int {
	switch t {
	case TierUnlicensed:
		return 0
	case TierHomelab, TierNonCommercial:
		return UnlimitedServers
	case TierTrial, TierNFR:
		return FreeTierServers
	case TierCommercial, TierEnterprise:
		return UnlimitedServers
	default:
		return 0
	}
}

// ParseTier maps a stored or submitted string onto a known tier. The
// boolean reports whether it was recognised; an unknown tier is never
// silently coerced, because coercing it would hand someone an entitlement
// they did not buy.
func ParseTier(value string) (Tier, bool) {
	switch Tier(value) {
	case TierUnlicensed:
		return TierUnlicensed, true
	case TierHomelab:
		return TierHomelab, true
	case TierNonCommercial:
		return TierNonCommercial, true
	case TierTrial:
		return TierTrial, true
	case TierNFR:
		return TierNFR, true
	case TierCommercial:
		return TierCommercial, true
	case TierEnterprise:
		return TierEnterprise, true
	default:
		return TierUnlicensed, false
	}
}
