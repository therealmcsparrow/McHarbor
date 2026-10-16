// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package clusterinfo is the read-only view of cluster readiness and membership
// that health, compliance and support report on without importing the cluster module.
package clusterinfo

import "context"

// ReadyCheck is one readiness condition. It carries a name and a verdict only: the
// probe is public, so a failure never explains itself to an anonymous caller.
// Its fields must stay identical to the cluster module's ReadyCheck, which converts to it.
type ReadyCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

// Instance is one node as the registry sees it; its fields must stay identical to
// the cluster module's Instance, which converts to it.
type Instance struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	Version      string `json:"version"`
	Build        string `json:"build"`
	HARole       string `json:"haRole"`
	State        string `json:"state"`
	AdvertiseURL string `json:"advertiseUrl"`
	Agents       int    `json:"agents"`
	StartedAt    string `json:"startedAt"`
	HeartbeatAt  string `json:"heartbeatAt"`
	Alive        bool   `json:"alive"`
	Self         bool   `json:"self"`
}

// Membership is this node's identity in the instance registry plus the topology
// it can see; the cluster module's Registry implements it.
type Membership interface {
	NodeID() string
	Role() string
	State() string
	List(ctx context.Context) ([]Instance, error)
}

// Readiness is the cluster service's view for reporting modules; Membership is
// nil until main attaches the registry.
type Readiness interface {
	Ready(ctx context.Context) (bool, []ReadyCheck)
	Membership() Membership
}

// Provider is what the service registered as "cluster" satisfies; a reporting
// module type-asserts it and calls Info at request time.
type Provider interface {
	Info() Readiness
}

// HAMode is what the service registered as "ha" exposes: the current HA mode.
type HAMode interface {
	Mode() string
}
