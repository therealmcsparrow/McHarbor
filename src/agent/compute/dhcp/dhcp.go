// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package dhcp serves addresses on McHarbor's own managed bridges.
//
// A managed network promises three things — an address, a gateway and a way
// out — and until this package existed it delivered only the middle one: the
// bridge carried a gateway address that nothing handed out and nothing routed
// through. Guests came up with no address at all.
//
// It is deliberately small. This serves McHarbor's own bridges, whose address
// range McHarbor chose and whose only clients are guests McHarbor created, so
// it implements the four messages that path needs and refuses everything it
// does not recognise rather than growing toward a general-purpose server.
package dhcp

import (
	"errors"
	"net"
	"time"
)

// DefaultLeaseTime is how long a guest keeps an address.
//
// An hour rather than a day: guests here are created and destroyed constantly,
// and a long lease means a rebuilt guest with a new MAC slowly exhausts a /24
// while the old entries sit unreachable.
const DefaultLeaseTime = time.Hour

// ErrNotLinux is returned on hosts where guest networking cannot run.
var ErrNotLinux = errors.New("dhcp: managed networks require a Linux host")

// Network is one managed network's addressing.
type Network struct {
	NetworkID string
	Bridge    string
	// ServerIP is the bridge's own address, offered as both router and DNS
	// unless DNS says otherwise.
	ServerIP net.IP
	Subnet   *net.IPNet
	// RangeStart and RangeEnd bound the pool. Zero values derive a range from
	// the subnet, skipping the network, broadcast and gateway addresses.
	RangeStart net.IP
	RangeEnd   net.IP
	LeaseTime  time.Duration
	// DNS servers offered to guests. Empty offers the gateway itself, which
	// works when the host runs a resolver and is harmless when it does not —
	// the guest falls back to whatever its image configured.
	DNS []net.IP
}

// Lease is one address assignment, reported as it happens.
type Lease struct {
	NetworkID string
	MAC       string
	IP        string
	// Hostname is whatever the guest claimed. It is untrusted and carried for
	// display only.
	Hostname  string
	Action    string // granted | renewed | expired | released
	ExpiresAt time.Time
}

// Lease actions, matching what the server records against a network.
const (
	ActionGranted  = "granted"
	ActionRenewed  = "renewed"
	ActionExpired  = "expired"
	ActionReleased = "released"
)
