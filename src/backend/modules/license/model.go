// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package license

import "time"

// License is the activated licence as the API returns it.
//
// Deliberately absent: the licence key itself. It is stored encrypted and
// never leaves the backend — the masked form is what a human ever needs,
// including on a support call.
type License struct {
	ID              string `json:"id"`
	KeyMasked       string `json:"keyMasked"`
	ServerLicenseID string `json:"serverLicenseId,omitempty"`
	Tier            string `json:"tier"`
	Status          string `json:"status"`
	// Valid is the licence server's verdict, and the field that decides.
	// A lapsed licence answers successfully with Valid false and a Reason.
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`
	Seats  Seats  `json:"seats"`
	Term   Term   `json:"term"`
	// InstanceRegistered false alongside Valid true means this install
	// lost its seat and has to claim one again.
	InstanceRegistered bool   `json:"instanceRegistered"`
	HoldsSeat          bool   `json:"holdsSeat"`
	ActivatedAt        string `json:"activatedAt"`
	LastCheckedAt      string `json:"lastCheckedAt,omitempty"`
	LastError          string `json:"lastError,omitempty"`
}

// Seats is the licence's seat allowance. Unlimited outranks Total: the
// homelab and non-commercial licences are unmetered, and their counts are
// informational.
type Seats struct {
	Unlimited bool `json:"unlimited"`
	Total     int  `json:"total"`
	Used      int  `json:"used"`
}

// Term is the licence's validity window. Perpetual outranks ExpiresAt.
type Term struct {
	Perpetual     bool   `json:"perpetual"`
	StartsAt      string `json:"startsAt,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	DaysRemaining int    `json:"daysRemaining"`
	AutoRenew     bool   `json:"autoRenew"`
}

// Detail is the payload behind license.view: the licence, this install's
// seat identity, and whether there is a licence server to talk to.
type Detail struct {
	License *License `json:"license"`
	// InstanceID is this installation's seat. Not a credential — the
	// licence key is — and the operator needs it in full, because
	// releasing a seat from the mcharbor.eu dashboard addresses it by
	// exactly this value.
	InstanceID       string `json:"instanceId"`
	InstanceType     string `json:"instanceType"`
	Hostname         string `json:"hostname,omitempty"`
	ServerConfigured bool   `json:"serverConfigured"`
}

// Status is the banner's only dependency, and the one licence payload
// every authenticated user may read. Counts, state, and where to write —
// no licence key, no seat identity, no customer record.
type Status struct {
	Tier         string `json:"tier"`
	State        string `json:"state"`
	ServersUsed  int    `json:"serversUsed"`
	ServersLimit int    `json:"serversLimit"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
	// DaysRemaining is what turns a dead trial into a renewal
	// conversation: the banner surfaces it well before it reaches zero.
	DaysRemaining int    `json:"daysRemaining"`
	NoticeDue     bool   `json:"noticeDue"`
	NoticeKind    string `json:"noticeKind,omitempty"`
	NoticePhase   string `json:"noticePhase,omitempty"`
	ContactURL    string `json:"contactUrl,omitempty"`
	OfflineDays   int    `json:"offlineDays"`
	SampledAt     string `json:"sampledAt,omitempty"`

	// noticeSince is when the current notice condition started, the clock each
	// user's own cadence counts from; NoticeDue is worked out per reader from it.
	noticeSince time.Time
}

// Usage is one recorded sample, as the usage table renders it.
//
// Samples stay local. The licence server's handshake has no usage
// endpoint, so this history is evidence for the renewal conversation and
// nothing is ever uploaded from it.
type Usage struct {
	ID           string `json:"id"`
	ServersUsed  int    `json:"serversUsed"`
	ServersLimit int    `json:"serversLimit"`
	AgentCount   int    `json:"agentCount"`
	EnvCount     int    `json:"envCount"`
	UserCount    int    `json:"userCount"`
	State        string `json:"state"`
	OverSince    string `json:"overSince,omitempty"`
	SampledAt    string `json:"sampledAt"`
}

// ActivateInput is the whole activation form: one key, pasted.
type ActivateInput struct {
	Key string `json:"key"`
}

// NoticeAckInput records that a banner was actually put in front of
// someone. The kind keeps different banners on different clocks.
type NoticeAckInput struct {
	Kind string `json:"kind"`
}

// counts is the internal shape the counter returns. Unexported because
// nothing outside this package should be inventing usage numbers.
type counts struct {
	servers      int
	agents       int
	environments int
	users        int
}
