// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// computePlatform is the token this plugin's facet rows carry.
const computePlatform = "compute"

// facetViewPermission is the key the plugin's own list routes gate on; a
// facet read re-checks it because the Reporting plugin's caller may hold only
// report rights.
const facetViewPermission = "compute.view"

const bytesPerGiB = float64(1 << 30)

var _ plugin.ReportFacetContributor = (*reportProvider)(nil)

// facetSource is the slice of Service a facet read needs, so the projections
// can be tested against a fake without a database or an agent.
type facetSource interface {
	ListEnvironments(ctx context.Context) ([]ComputeEnvironment, error)
	ListPools(ctx context.Context, envID string) ([]Pool, error)
	ListImages(ctx context.Context, envID, kind string) ([]Image, error)
}

// facetPermissions is the slice of the host RBAC handle a facet read needs.
type facetPermissions interface {
	HasPermission(userID, envID, perm string) (bool, error)
}

// ContributeFacet answers one facet of the shared reports for this plugin's
// platform. A source that fails is recorded not-OK rather than returned as an
// error, so the consumer prints a partial section and says why it is partial.
func (r *reportProvider) ContributeFacet(
	ctx context.Context,
	req plugin.ReportFacetRequest,
) (plugin.ReportFacetResult, error) {
	if r.plugin.svc == nil {
		return plugin.ReportFacetResult{}, fmt.Errorf("compute: the plugin is not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	var perms facetPermissions
	if r.plugin.host.RBAC != nil {
		perms = r.plugin.host.RBAC
	}
	return collectComputeFacet(ctx, r.plugin.svc, perms, req)
}

// collectComputeFacet reads and projects one facet. A nil perms denies every
// caller: an unwired RBAC handle is not evidence anybody may read.
func collectComputeFacet(
	ctx context.Context,
	src facetSource,
	perms facetPermissions,
	req plugin.ReportFacetRequest,
) (plugin.ReportFacetResult, error) {
	switch req.Facet {
	case plugin.ReportFacetInventory, plugin.ReportFacetCapacity, plugin.ReportFacetHealth:
	default:
		return plugin.ReportFacetResult{}, fmt.Errorf("compute: facet %q is not contributed", req.Facet)
	}

	var out plugin.ReportFacetResult
	started := time.Now()
	all, err := src.ListEnvironments(ctx)
	if err != nil {
		out.Sources = append(out.Sources, plugin.ReportSourceStatus{
			Key: "compute_hosts", Label: "Compute hosts", OK: false,
			Note:       "The host list could not be read.",
			DurationMS: time.Since(started).Milliseconds(),
		})
		return out, nil
	}
	out.Sources = append(out.Sources, plugin.ReportSourceStatus{
		Key: "compute_hosts", Label: "Compute hosts", OK: true,
		DurationMS: time.Since(started).Milliseconds(),
	})

	var candidates []ComputeEnvironment
	for _, env := range all {
		if !env.ComputeEnabled || (req.EnvironmentID != "" && env.EnvironmentID != req.EnvironmentID) {
			continue
		}
		candidates = append(candidates, env)
	}

	// A failed permission lookup is a denial: an errored check is not evidence
	// the caller may read.
	var envs []ComputeEnvironment
	for _, env := range candidates {
		if allowed, permErr := hasFacetPermission(perms, req.UserID, env.EnvironmentID); permErr == nil && allowed {
			envs = append(envs, env)
		}
	}
	if len(candidates) > 0 && len(envs) == 0 {
		return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{{
			Key: "compute_hosts", Label: "Compute hosts", OK: false, Note: "not permitted",
		}}}, nil
	}
	if len(envs) < len(candidates) {
		out.Notes = append(out.Notes, fmt.Sprintf(
			"%s omitted because the caller may not view them.",
			plural(len(candidates)-len(envs), "compute host is", "compute hosts are")))
	}

	switch req.Facet {
	case plugin.ReportFacetInventory:
		collectInventory(ctx, src, req, envs, &out)
	case plugin.ReportFacetCapacity:
		collectCapacity(ctx, src, req, envs, &out)
	default:
		out.Health = projectHealth(req, envs)
		for _, env := range envs {
			if !env.Available {
				out.Notes = append(out.Notes, fmt.Sprintf("%s could not be read (%s).",
					hostName(env), unreachableHint(env.UnavailableReason)))
			}
		}
	}
	return out, nil
}

func hasFacetPermission(perms facetPermissions, userID, envID string) (bool, error) {
	if perms == nil || userID == "" {
		return false, nil
	}
	return perms.HasPermission(userID, envID, facetViewPermission)
}

func facetIdentity(req plugin.ReportFacetRequest, env ComputeEnvironment, kind, id, name string) plugin.ReportFacetIdentity {
	envName := req.Environment
	if envName == "" || req.EnvironmentID != env.EnvironmentID {
		envName = hostName(env)
	}
	return plugin.ReportFacetIdentity{
		Platform:      computePlatform,
		Kind:          kind,
		ID:            id,
		Name:          name,
		EnvironmentID: env.EnvironmentID,
		Environment:   envName,
		Host:          hostName(env),
	}
}

// readPools reads every reachable host's pools, and records one aggregate
// source so a failure names a count rather than a host.
func readPools(
	ctx context.Context,
	src facetSource,
	envs []ComputeEnvironment,
	out *plugin.ReportFacetResult,
) map[string][]Pool {
	started := time.Now()
	pools := make(map[string][]Pool, len(envs))
	failed, reachable := 0, 0
	for _, env := range envs {
		if !env.Available {
			continue
		}
		reachable++
		list, err := src.ListPools(ctx, env.EnvironmentID)
		if err != nil {
			failed++
			continue
		}
		pools[env.EnvironmentID] = list
	}
	status := plugin.ReportSourceStatus{
		Key: "compute_pools", Label: "Storage pools", OK: failed == 0,
		DurationMS: time.Since(started).Milliseconds(),
	}
	if failed > 0 {
		status.Note = fmt.Sprintf("Pools could not be read on %d of %d reachable hosts.", failed, reachable)
	}
	out.Sources = append(out.Sources, status)
	return pools
}

func collectInventory(
	ctx context.Context,
	src facetSource,
	req plugin.ReportFacetRequest,
	envs []ComputeEnvironment,
	out *plugin.ReportFacetResult,
) {
	pools := readPools(ctx, src, envs, out)

	started := time.Now()
	images, err := src.ListImages(ctx, req.EnvironmentID, "")
	status := plugin.ReportSourceStatus{
		Key: "compute_images", Label: "Image catalog", OK: err == nil,
		DurationMS: time.Since(started).Milliseconds(),
	}
	if err != nil {
		status.Note = "The image catalog could not be read."
		images = nil
	}
	out.Sources = append(out.Sources, status)

	out.Inventory = projectInventory(req, envs, pools, images)
}

func collectCapacity(
	ctx context.Context,
	src facetSource,
	req plugin.ReportFacetRequest,
	envs []ComputeEnvironment,
	out *plugin.ReportFacetResult,
) {
	pools := readPools(ctx, src, envs, out)
	out.Capacity = projectCapacity(req, envs, pools)
	out.Notes = append(out.Notes,
		"Compute reports each host's physical CPU and memory and its pools' space; what guests are allocated against them is reported by the guest platforms.")
}

// projectInventory turns hosts, pools and the image catalog into rows. An
// unreachable host is still a row, so the count is not silently smaller.
func projectInventory(
	req plugin.ReportFacetRequest,
	envs []ComputeEnvironment,
	pools map[string][]Pool,
	images []Image,
) []plugin.ReportInventoryRow {
	rows := make([]plugin.ReportInventoryRow, 0, len(envs))
	for _, env := range envs {
		row := plugin.ReportInventoryRow{
			FacetIdentity: facetIdentity(req, env, "host", env.EnvironmentID, hostName(env)),
			State:         "ready",
		}
		switch {
		case !env.Available:
			row.State = "unreachable"
		case env.Capability != nil:
			row.Image = env.Capability.OS + " / " + env.Capability.Arch
			row.CPU = float64(env.Capability.HostCPUs)
			row.MemoryMB = env.Capability.HostMemoryBytes >> 20
		}
		rows = append(rows, row)

		for _, pool := range pools[env.EnvironmentID] {
			identity := facetIdentity(req, env, "pool", pool.ID, pool.Name)
			identity.Owner = hostName(env)
			rows = append(rows, plugin.ReportInventoryRow{
				FacetIdentity: identity,
				State:         pool.State,
				Image:         pool.Backend,
				DiskGB:        float64(pool.CapacityBytes) / bytesPerGiB,
				Created:       pool.CreatedAt,
			})
		}
	}

	for _, image := range images {
		state := "catalog"
		if image.Cache != nil {
			state = image.Cache.State
		}
		identity := plugin.ReportFacetIdentity{
			Platform: computePlatform,
			Kind:     "image",
			ID:       image.ID,
			Name:     image.DisplayName,
			Owner:    image.Kind,
		}
		if req.EnvironmentID != "" {
			identity.EnvironmentID = req.EnvironmentID
			identity.Environment = req.Environment
		}
		rows = append(rows, plugin.ReportInventoryRow{
			FacetIdentity: identity,
			State:         state,
			Image:         fmt.Sprintf("%s %s %s", image.Distro, image.Release, image.Arch),
			DiskGB:        float64(image.SizeBytes) / bytesPerGiB,
			Created:       image.CreatedAt,
		})
	}
	return rows
}

// projectCapacity emits one disk row per pool and one cpu and memory row per
// probed host. Used is only claimed when the pool reports a capacity, because
// a pool that was never sampled has an unknown use, not a zero one.
func projectCapacity(
	req plugin.ReportFacetRequest,
	envs []ComputeEnvironment,
	pools map[string][]Pool,
) []plugin.ReportCapacityRow {
	var rows []plugin.ReportCapacityRow
	for _, env := range envs {
		if env.Available && env.Capability != nil {
			if env.Capability.HostCPUs > 0 {
				rows = append(rows, plugin.ReportCapacityRow{
					FacetIdentity: facetIdentity(req, env, "host", env.EnvironmentID, hostName(env)),
					Resource:      "cpu",
					Capacity:      float64(env.Capability.HostCPUs),
					Unit:          "cores",
				})
			}
			if env.Capability.HostMemoryBytes > 0 {
				rows = append(rows, plugin.ReportCapacityRow{
					FacetIdentity: facetIdentity(req, env, "host", env.EnvironmentID, hostName(env)),
					Resource:      "memory",
					Capacity:      float64(env.Capability.HostMemoryBytes) / bytesPerGiB,
					Unit:          "GiB",
				})
			}
		}
		for _, pool := range pools[env.EnvironmentID] {
			identity := facetIdentity(req, env, "pool", pool.ID, pool.Name)
			identity.Owner = hostName(env)
			rows = append(rows, plugin.ReportCapacityRow{
				FacetIdentity: identity,
				Resource:      "disk",
				Allocated:     float64(pool.AllocatedBytes) / bytesPerGiB,
				Used:          float64(pool.UsedBytes) / bytesPerGiB,
				HasUsed:       pool.CapacityBytes > 0,
				Capacity:      float64(pool.CapacityBytes) / bytesPerGiB,
				Unit:          "GiB",
			})
		}
	}
	return rows
}

// projectHealth maps a host to up, degraded or down. A host never probed is
// unknown rather than down: nobody has asked it yet.
func projectHealth(req plugin.ReportFacetRequest, envs []ComputeEnvironment) []plugin.ReportHealthRow {
	rows := make([]plugin.ReportHealthRow, 0, len(envs))
	for _, env := range envs {
		row := plugin.ReportHealthRow{
			FacetIdentity: facetIdentity(req, env, "host", env.EnvironmentID, hostName(env)),
			Status:        "up",
		}
		if env.ProbedAt != nil {
			row.Since = *env.ProbedAt
		}
		switch {
		case !env.Available:
			row.Status = "down"
			row.Detail = unreachableHint(env.UnavailableReason)
		case env.Capability == nil:
			row.Status = "unknown"
			row.Detail = "never checked"
		default:
			blockers := 0
			for _, item := range env.Capability.Items {
				if item.Critical && !item.Present {
					blockers++
				}
			}
			if blockers > 0 {
				row.Status = "degraded"
				row.Detail = plural(blockers, "critical check failing", "critical checks failing")
			}
		}
		rows = append(rows, row)
	}
	return rows
}
