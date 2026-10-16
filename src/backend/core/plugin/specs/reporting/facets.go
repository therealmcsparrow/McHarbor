// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reporting

import (
	"context"
	"time"
)

// The facets a provider may contribute to. The set is closed: every value
// has a row type below and a section builder in the Reporting plugin, so a
// facet nobody can print is refused at Register rather than silently dropped.
const (
	FacetInventory  = "inventory"
	FacetCapacity   = "capacity"
	FacetProtection = "protection"
	FacetHealth     = "health"
	FacetExposure   = "exposure"
	FacetCost       = "cost"
)

// Facets returns the closed facet set in the order the shared reports print it.
func Facets() []string {
	return []string{FacetInventory, FacetCapacity, FacetProtection, FacetHealth, FacetExposure, FacetCost}
}

// IsFacet reports whether f is one of the closed facet set.
func IsFacet(f string) bool {
	for _, k := range Facets() {
		if k == f {
			return true
		}
	}
	return false
}

// FacetDecl is one cross-platform question a provider answers. Platform is
// the short token its rows carry ("kvm"); one (Facet, Platform) pair has one
// owner, so two providers cannot both claim to be the KVM inventory.
type FacetDecl struct {
	Facet    string
	Platform string
	// PlatformLabelKey names the platform in the provider's own i18n
	// namespace, for the studio and the ad-hoc planner. Empty falls back to
	// the provider's DisplayName.
	PlatformLabelKey string
}

// FacetRequest is what the Reporting plugin hands a provider for one facet.
// UserID is the only basis for the provider's own permission check: a denied
// caller gets no rows and a not-OK source, never another user's view.
type FacetRequest struct {
	Facet         string
	EnvironmentID string
	Environment   string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	// Cookie and Lang come from the calling request. Never log, store or
	// render either.
	Cookie string
	Lang   string
	User   string
	UserID string
}

// FacetIdentity is the columns every facet row shares, so one table can list
// a container, a VM and a pod side by side.
type FacetIdentity struct {
	Platform      string
	Kind          string
	ID            string
	Name          string
	EnvironmentID string
	Environment   string
	Host          string
	// Owner is whatever groups the object for its operator: a stack, a
	// namespace, a pool.
	Owner string
}

// InventoryRow answers "what runs, where, on what".
type InventoryRow struct {
	FacetIdentity
	State    string
	Image    string
	CPU      float64
	MemoryMB int64
	DiskGB   float64
	Created  time.Time
}

// CapacityRow is one resource on one host, pool or node. HasUsed separates a
// measured zero from a platform that cannot measure use at all.
type CapacityRow struct {
	FacetIdentity
	Resource  string
	Allocated float64
	Used      float64
	HasUsed   bool
	Capacity  float64
	Unit      string
}

// ProtectionRow says whether an object is recoverable. OffHost is the one a
// reviewer asks about: a snapshot on the same pool as its disk is not a backup.
type ProtectionRow struct {
	FacetIdentity
	Protected      bool
	Method         string
	LastGood       time.Time
	OffHost        bool
	RPOTargetHours float64
	Note           string
}

// HealthRow is one object's state. Status is "up", "degraded", "down" or
// "unknown"; anything else is read as unknown.
type HealthRow struct {
	FacetIdentity
	Status   string
	Restarts int
	Flaps    int
	Since    time.Time
	Detail   string
}

// ExposureRow is one reachable endpoint. Scope is "public", "internal" or
// "cluster"; Via is how it is published.
type ExposureRow struct {
	FacetIdentity
	Protocol   string
	Port       int
	TargetPort int
	Address    string
	Scope      string
	Via        string
}

// CostRow is resource-hours over the request period. Basis is "allocated"
// when the platform can only report what was reserved, "measured" when it
// sampled real use; the report never adds the two without saying so.
type CostRow struct {
	FacetIdentity
	CPUCoreHours  float64
	MemoryGBHours float64
	DiskGBHours   float64
	Basis         string
}

// FacetResult is one provider's answer. Only the slice matching the requested
// facet is read. Blocks is an optional platform passage printed under the
// platform's sub-heading, for what a row cannot say (a Swarm quorum).
type FacetResult struct {
	Inventory  []InventoryRow
	Capacity   []CapacityRow
	Protection []ProtectionRow
	Health     []HealthRow
	Exposure   []ExposureRow
	Cost       []CostRow

	Blocks  []Block
	Sources []SourceStatus
	Notes   []string
}

// ReportFacetContributor is implemented by a ReportProviderClient whose spec
// declares Facets. It is a separate interface so a provider with report types
// only is not forced to answer facets.
type ReportFacetContributor interface {
	ContributeFacet(ctx context.Context, req FacetRequest) (FacetResult, error)
}

// FacetProvider is one declared (facet, platform) pair and who owns it.
type FacetProvider struct {
	Slug        string
	DisplayName string
	Decl        FacetDecl
}

// FacetContribution is one provider's outcome for a CollectFacet call. Err is
// set when the provider failed; the consumer records it in the annex rather
// than failing the document.
type FacetContribution struct {
	Provider FacetProvider
	Result   FacetResult
	Err      error
}

// ReportFacetHandle is the consumer half, asserted on HostContext.Reporting
// rather than added to ReportingHandle, so a consumer built before facets
// existed keeps its method set.
type ReportFacetHandle interface {
	// FacetProviders lists who declared facet, in registration order.
	FacetProviders(facet string) []FacetProvider

	// CollectFacet asks every declaring provider concurrently and returns
	// one contribution each, failures included.
	CollectFacet(ctx context.Context, facet string, req FacetRequest) []FacetContribution
}
