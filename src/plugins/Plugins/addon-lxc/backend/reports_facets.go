// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// facetViewPermission is the key this plugin's own list routes are gated by,
// so a report never shows a caller more than the containers page would.
const facetViewPermission = "ct.view"

// Source keys for the reads a facet makes.
const (
	srcContainers = "lxc_containers"
	srcSnapshots  = "lxc_snapshots"
	srcBackups    = "lxc_backups"
	srcHostCaps   = "lxc_host_capacity"
)

const gibibyte = float64(1 << 30)

// lxcFacets is what this provider answers, in the order the shared reports
// print them. Exposure is missing on purpose: see lxcReportSpec.
var lxcFacets = []string{
	plugin.ReportFacetInventory,
	plugin.ReportFacetCapacity,
	plugin.ReportFacetProtection,
	plugin.ReportFacetHealth,
	plugin.ReportFacetCost,
}

// facetSource is the seam between the projections and the plugin's data, so a
// test can supply containers without a database or an agent.
type facetSource interface {
	containers(ctx context.Context, envID string) ([]Container, error)
	snapshots(ctx context.Context, containerID string) ([]Snapshot, error)
	lastGoodBackup(ctx context.Context, containerID string) (time.Time, error)
	hostCapacity(ctx context.Context, envID string) (cpus int, memoryBytes int64, err error)
}

// facetPermissions is the slice of the host's RBAC handle a facet needs.
type facetPermissions interface {
	HasPermission(userID, envID, perm string) (bool, error)
}

// liveFacetSource reads through the plugin's own service.
type liveFacetSource struct{ svc *Service }

func (l liveFacetSource) containers(ctx context.Context, envID string) ([]Container, error) {
	return l.svc.List(ctx, envID, false)
}

func (l liveFacetSource) snapshots(ctx context.Context, containerID string) ([]Snapshot, error) {
	return l.svc.Snapshots(ctx, containerID)
}

func (l liveFacetSource) lastGoodBackup(ctx context.Context, containerID string) (time.Time, error) {
	return l.svc.lastGoodBackupAt(ctx, containerID)
}

func (l liveFacetSource) hostCapacity(ctx context.Context, envID string) (int, int64, error) {
	if l.svc.rpc == nil {
		return 0, 0, fmt.Errorf("lxc: no agent bridge")
	}
	capability, err := l.svc.probe(ctx, envID)
	if err != nil {
		return 0, 0, fmt.Errorf("probing host capacity: %w", err)
	}
	return capability.HostCPUs, capability.HostMemoryBytes, nil
}

var _ plugin.ReportFacetContributor = (*reportProvider)(nil)

// facetSourceOrLive returns the injected source, or the live one.
func (r *reportProvider) facetSourceOrLive() facetSource {
	if r.source != nil {
		return r.source
	}
	return liveFacetSource{svc: r.plugin.svc}
}

// facetPermsOrLive returns the injected checker, or the host's RBAC handle.
// A nil handle is returned as a nil interface so it denies.
func (r *reportProvider) facetPermsOrLive() facetPermissions {
	if r.perms != nil {
		return r.perms
	}
	if r.plugin != nil && r.plugin.host.RBAC != nil {
		return r.plugin.host.RBAC
	}
	return nil
}

func (r *reportProvider) logFacetError(msg string, args ...any) {
	if r.plugin != nil && r.plugin.logger != nil {
		r.plugin.logger.Error(msg, args...)
	}
}

// ContributeFacet answers one facet across every container the caller may
// view. A denied caller gets no rows and a "not permitted" source; a read that
// fails becomes a not-OK source with fixed text, because the underlying error
// can name a host or a path and must not reach a circulated document.
func (r *reportProvider) ContributeFacet(
	ctx context.Context,
	req plugin.ReportFacetRequest,
) (plugin.ReportFacetResult, error) {
	switch req.Facet {
	case plugin.ReportFacetInventory, plugin.ReportFacetCapacity, plugin.ReportFacetProtection,
		plugin.ReportFacetHealth, plugin.ReportFacetCost:
	default:
		return plugin.ReportFacetResult{}, fmt.Errorf("lxc: facet %q is not contributed", req.Facet)
	}
	if r.plugin == nil || (r.source == nil && r.plugin.svc == nil) {
		return plugin.ReportFacetResult{}, fmt.Errorf("lxc: the plugin is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	denied := func() plugin.ReportFacetResult {
		return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{{
			Key: srcContainers, Label: "System containers", OK: false, Note: "not permitted",
		}}}
	}
	perms := r.facetPermsOrLive()
	if perms == nil || strings.TrimSpace(req.UserID) == "" {
		return denied(), nil
	}

	src := r.facetSourceOrLive()
	started := time.Now()
	all, err := src.containers(ctx, req.EnvironmentID)
	if err != nil {
		r.logFacetError("lxc: facet container list failed", "facet", req.Facet, "error", err)
		return plugin.ReportFacetResult{Sources: []plugin.ReportSourceStatus{
			failedFacetSource(srcContainers, "System containers", "the container list could not be read"),
		}}, nil
	}

	// One permission check per host: ct.view is environment-scoped, and a failed
	// lookup is a denial because an errored check is not evidence the caller may read.
	allowedEnv := map[string]bool{}
	var visible []Container
	for _, ct := range all {
		ok, seen := allowedEnv[ct.EnvironmentID]
		if !seen {
			granted, permErr := perms.HasPermission(req.UserID, ct.EnvironmentID, facetViewPermission)
			if permErr != nil {
				r.logFacetError("lxc: facet permission check failed", "facet", req.Facet, "error", permErr)
			}
			ok = permErr == nil && granted
			allowedEnv[ct.EnvironmentID] = ok
		}
		if ok {
			visible = append(visible, ct)
		}
	}
	if len(all) > 0 && len(visible) == 0 {
		return denied(), nil
	}

	var res plugin.ReportFacetResult
	res.Sources = append(res.Sources, plugin.ReportSourceStatus{
		Key: srcContainers, Label: "System containers", OK: true,
		DurationMS: time.Since(started).Milliseconds(),
	})
	if hidden := len(all) - len(visible); hidden > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf(
			"%s omitted because the caller may not view their host.",
			plural(hidden, "system container is", "system containers are")))
	}

	p := facetProjector{req: req, now: time.Now()}
	switch req.Facet {
	case plugin.ReportFacetInventory:
		res.Inventory = p.inventory(visible)
		res.Notes = append(res.Notes, p.unreachableNote(visible, "shown with state unknown"))
	case plugin.ReportFacetCapacity:
		r.collectCapacity(ctx, src, p, visible, &res)
	case plugin.ReportFacetProtection:
		r.collectProtection(ctx, src, p, visible, &res)
	case plugin.ReportFacetHealth:
		res.Health = p.health(visible)
		res.Notes = append(res.Notes,
			"Restart and flap counts are not recorded for system containers, so they read zero.")
	case plugin.ReportFacetCost:
		res.Cost = p.cost(visible)
		res.Notes = append(res.Notes,
			"Hours are allocated, not measured: a container's limits times the time it has been running, "+
				"counted from its last start. A container stopped during the period is not charged for the time it ran, "+
				"because no state history is kept. A container with no limit contributes zero for that resource.")
		res.Notes = append(res.Notes, p.unreachableNote(visible, "left out because its state is unknown"))
	}
	res.Notes = compactNotes(res.Notes)
	return res, nil
}

// failedFacetSource is a not-OK source. The note is fixed text.
func failedFacetSource(key, label, note string) plugin.ReportSourceStatus {
	return plugin.ReportSourceStatus{Key: key, Label: label, OK: false, Note: note}
}

func compactNotes(notes []string) []string {
	out := notes[:0]
	for _, n := range notes {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// facetProjector turns containers into rows. It holds no I/O, so each
// projection can be tested from a slice of containers.
type facetProjector struct {
	req plugin.ReportFacetRequest
	now time.Time
}

func (p facetProjector) identity(ct Container) plugin.ReportFacetIdentity {
	envName := ct.EnvironmentID
	if p.req.EnvironmentID == ct.EnvironmentID && p.req.Environment != "" {
		envName = p.req.Environment
	}
	return plugin.ReportFacetIdentity{
		Platform:      facetPlatform,
		Kind:          "ct",
		ID:            ct.ID,
		Name:          ct.Name,
		EnvironmentID: ct.EnvironmentID,
		Environment:   envName,
		Host:          envName,
		Owner:         ct.PoolID,
	}
}

func (p facetProjector) hostIdentity(envID string) plugin.ReportFacetIdentity {
	name := envID
	if p.req.EnvironmentID == envID && p.req.Environment != "" {
		name = p.req.Environment
	}
	return plugin.ReportFacetIdentity{
		Platform: facetPlatform, Kind: "host", ID: envID, Name: name,
		EnvironmentID: envID, Environment: name, Host: name,
	}
}

// unreachableNote names how many containers sit on a host the agent cannot
// reach; what happens to them depends on the facet, so the caller says so.
func (p facetProjector) unreachableNote(cts []Container, treatment string) string {
	n := 0
	for _, ct := range cts {
		if !ct.HostAvailable {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%s on an unreachable host, %s.",
		plural(n, "system container is", "system containers are"), treatment)
}

// effectiveState is the state to report: a container whose host cannot be
// reached has no trustworthy state, whatever was last stored.
func effectiveState(ct Container) string {
	if !ct.HostAvailable {
		return StateUnknown
	}
	return ct.State
}

func (p facetProjector) inventory(cts []Container) []plugin.ReportInventoryRow {
	rows := make([]plugin.ReportInventoryRow, 0, len(cts))
	for _, ct := range cts {
		rows = append(rows, plugin.ReportInventoryRow{
			FacetIdentity: p.identity(ct),
			State:         effectiveState(ct),
			Image:         strings.TrimSpace(ct.OSFamily + " " + ct.OSVersion),
			CPU:           cpuCores(ct.Limits.CPUMax),
			MemoryMB:      ct.Limits.MemoryMaxBytes / mebibyte,
			DiskGB:        float64(ct.Limits.DiskQuotaBytes) / gibibyte,
			Created:       ct.CreatedAt,
		})
	}
	return rows
}

// health maps a container state onto up, degraded, down or unknown. A stopped
// container is down: the shared report cannot tell deliberate from accidental,
// and Detail says which state it was.
func (p facetProjector) health(cts []Container) []plugin.ReportHealthRow {
	rows := make([]plugin.ReportHealthRow, 0, len(cts))
	for _, ct := range cts {
		row := plugin.ReportHealthRow{FacetIdentity: p.identity(ct), Detail: ct.State}
		switch {
		case !ct.HostAvailable:
			row.Status = "unknown"
			row.Detail = "host unreachable"
		case ct.State == StateRunning:
			row.Status = "up"
			if ct.LastStartedAt != nil {
				row.Since = *ct.LastStartedAt
			}
		case ct.State == StateStopped, ct.State == StateCrashed:
			row.Status = "down"
			if ct.State == StateCrashed && ct.ExitReason != "" {
				row.Detail = "crashed: " + ct.ExitReason
			}
		case ct.State == StateUnknown:
			row.Status = "unknown"
		default:
			row.Status = "degraded"
		}
		rows = append(rows, row)
	}
	return rows
}

// cost charges allocated resource-hours to each container running now, from
// its last start (or creation) to the end of the period, clipped to the period.
func (p facetProjector) cost(cts []Container) []plugin.ReportCostRow {
	end := p.req.PeriodEnd
	if end.IsZero() || end.After(p.now) {
		end = p.now
	}
	rows := make([]plugin.ReportCostRow, 0, len(cts))
	for _, ct := range cts {
		if !ct.HostAvailable || !isLiveState(ct.State) {
			continue
		}
		since := ct.CreatedAt
		if ct.LastStartedAt != nil {
			since = *ct.LastStartedAt
		}
		if p.req.PeriodStart.After(since) {
			since = p.req.PeriodStart
		}
		hours := end.Sub(since).Hours()
		if hours <= 0 {
			continue
		}
		rows = append(rows, plugin.ReportCostRow{
			FacetIdentity: p.identity(ct),
			CPUCoreHours:  cpuCores(ct.Limits.CPUMax) * hours,
			MemoryGBHours: float64(ct.Limits.MemoryMaxBytes) / gibibyte * hours,
			DiskGBHours:   float64(ct.Limits.DiskQuotaBytes) / gibibyte * hours,
			Basis:         "allocated",
		})
	}
	return rows
}

// cpuCores reads cpu.max ("quota period", or "max") as a core count; zero
// means no limit.
func cpuCores(cpuMax string) float64 {
	parts := strings.Fields(cpuMax)
	if len(parts) == 0 || parts[0] == "max" {
		return 0
	}
	quota, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || quota <= 0 {
		return 0
	}
	period := float64(defaultCPUPeriod)
	if len(parts) > 1 {
		if v, err := strconv.ParseFloat(parts[1], 64); err == nil && v > 0 {
			period = v
		}
	}
	return quota / period
}

// collectCapacity projects per-host allocation against what the host has. Only
// containers running now count, because a stopped one reserves nothing; a host
// whose capacity cannot be probed gets no rows rather than rows against zero.
func (r *reportProvider) collectCapacity(
	ctx context.Context,
	src facetSource,
	p facetProjector,
	cts []Container,
	res *plugin.ReportFacetResult,
) {
	type alloc struct {
		cpu, mem float64
	}
	byHost := map[string]*alloc{}
	var hosts []string
	for _, ct := range cts {
		a := byHost[ct.EnvironmentID]
		if a == nil {
			a = &alloc{}
			byHost[ct.EnvironmentID] = a
			hosts = append(hosts, ct.EnvironmentID)
		}
		if !ct.HostAvailable || !isLiveState(ct.State) {
			continue
		}
		a.cpu += cpuCores(ct.Limits.CPUMax)
		a.mem += float64(ct.Limits.MemoryMaxBytes) / gibibyte
	}
	sort.Strings(hosts)

	failed := 0
	for _, envID := range hosts {
		if ctx.Err() != nil {
			failed++
			continue
		}
		cpus, mem, err := src.hostCapacity(ctx, envID)
		if err != nil {
			r.logFacetError("lxc: facet host capacity failed", "environment", envID, "error", err)
			failed++
			continue
		}
		a := byHost[envID]
		id := p.hostIdentity(envID)
		if cpus > 0 {
			res.Capacity = append(res.Capacity, plugin.ReportCapacityRow{
				FacetIdentity: id, Resource: "cpu", Allocated: a.cpu, Capacity: float64(cpus), Unit: "cores",
			})
		}
		if mem > 0 {
			res.Capacity = append(res.Capacity, plugin.ReportCapacityRow{
				FacetIdentity: id, Resource: "memory", Allocated: a.mem, Capacity: float64(mem) / gibibyte, Unit: "GiB",
			})
		}
	}
	source := plugin.ReportSourceStatus{Key: srcHostCaps, Label: "Host capacity", OK: failed == 0}
	if failed > 0 {
		source.Note = fmt.Sprintf("%s could not be probed and have no capacity rows.", plural(failed, "host", "hosts"))
	}
	res.Sources = append(res.Sources, source)

	res.Notes = append(res.Notes,
		"Allocated is the sum of the limits of the containers running now. A container with no CPU or memory limit adds "+
			"nothing for that resource, so a host with unlimited containers can be fuller than its row says. Use is not measured here.")
	res.Notes = append(res.Notes, p.unreachableNote(cts, "not counted because its state is unknown"))
}

// collectProtection reads each container's snapshots and newest good backup.
// A snapshot lives on the pool it protects, so only a backup is OffHost.
func (r *reportProvider) collectProtection(
	ctx context.Context,
	src facetSource,
	p facetProjector,
	cts []Container,
	res *plugin.ReportFacetResult,
) {
	snapStarted := time.Now()
	unreadSnaps, unreadBackups := 0, 0
	for _, ct := range cts {
		if ctx.Err() != nil {
			unreadSnaps++
			continue
		}
		snaps, err := src.snapshots(ctx, ct.ID)
		if err != nil {
			r.logFacetError("lxc: facet snapshots unreadable", "container", ct.ID, "error", err)
			unreadSnaps++
			continue
		}
		backupAt, backupErr := src.lastGoodBackup(ctx, ct.ID)
		if backupErr != nil {
			r.logFacetError("lxc: facet backups unreadable", "container", ct.ID, "error", backupErr)
			unreadBackups++
		}
		res.Protection = append(res.Protection, protectionRow(p.identity(ct), snaps, backupAt, backupErr != nil))
	}

	snapSource := plugin.ReportSourceStatus{
		Key: srcSnapshots, Label: "Container snapshots", OK: unreadSnaps == 0,
		DurationMS: time.Since(snapStarted).Milliseconds(),
	}
	if unreadSnaps > 0 {
		snapSource.Note = fmt.Sprintf("%s could not be read and are left out rather than reported as unprotected.",
			plural(unreadSnaps, "container's snapshots", "containers' snapshots"))
	}
	backupSource := plugin.ReportSourceStatus{Key: srcBackups, Label: "Container backups", OK: unreadBackups == 0}
	if unreadBackups > 0 {
		backupSource.Note = fmt.Sprintf("%s could not be read, so the rows say so.",
			plural(unreadBackups, "container's backup history", "containers' backup history"))
	}
	res.Sources = append(res.Sources, snapSource, backupSource)
	res.Notes = append(res.Notes,
		"A snapshot is on the same pool as its container, so it is not off-host; only a backup is.")
}

// protectionRow prefers the backup when there is one, because it survives the
// pool failing, and names the newest snapshot alongside it.
func protectionRow(id plugin.ReportFacetIdentity, snaps []Snapshot, backupAt time.Time, backupUnread bool) plugin.ReportProtectionRow {
	var newest time.Time
	for _, s := range snaps {
		if s.CreatedAt.After(newest) {
			newest = s.CreatedAt
		}
	}
	row := plugin.ReportProtectionRow{FacetIdentity: id, Method: "none"}
	switch {
	case !backupAt.IsZero():
		row.Protected, row.Method, row.LastGood, row.OffHost = true, "backup", backupAt, true
		if !newest.IsZero() {
			row.Note = "newest snapshot " + newest.Format("2006-01-02")
		}
	case !newest.IsZero():
		row.Protected, row.Method, row.LastGood = true, "snapshot", newest
		row.Note = "no off-host backup"
	default:
		row.Note = "no snapshot and no backup"
	}
	if backupUnread {
		row.Note = strings.TrimSpace(row.Note + " (backup history could not be read)")
	}
	return row
}

// plural renders "1 container" / "3 containers" without a second format string
// at every call site.
func plural(count int, one, many string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, one)
	}
	return fmt.Sprintf("%d %s", count, many)
}
