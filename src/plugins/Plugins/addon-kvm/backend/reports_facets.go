// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// kvmPlatform is the token every facet row from this plugin carries.
const kvmPlatform = "kvm"

// kvmFacetDecls lists the facets this plugin answers. Exposure is absent on
// purpose: the plugin models no port forwards and records no NIC addresses, so
// a row would be invented rather than read.
func kvmFacetDecls() []plugin.ReportFacetDecl {
	facets := []string{
		plugin.ReportFacetInventory, plugin.ReportFacetCapacity, plugin.ReportFacetProtection,
		plugin.ReportFacetHealth, plugin.ReportFacetCost,
	}
	out := make([]plugin.ReportFacetDecl, 0, len(facets))
	for _, facet := range facets {
		out = append(out, plugin.ReportFacetDecl{
			Facet: facet, Platform: kvmPlatform, PlatformLabelKey: "vm:reports.platform",
		})
	}
	return out
}

// facetSource is the slice of Service the facet projections read, so a test can
// supply guests without a database.
type facetSource interface {
	List(ctx context.Context, envID string, templates bool) ([]VM, error)
	Disks(ctx context.Context, vmID string) ([]Disk, error)
	Snapshots(ctx context.Context, vmID string) ([]Snapshot, error)
	ListBackupPlans(ctx context.Context, vmID string) ([]BackupPlan, error)
	lastGoodBackupAt(ctx context.Context, vmID string) (time.Time, error)
	guestHealthRows(ctx context.Context, vmID string) (map[string]guestHealth, error)
	readEvents(ctx context.Context, vmID string, since time.Time) (map[string][]vmEvent, error)
	readIntents(ctx context.Context, vmID string, since time.Time) (map[string][]lifecycleIntent, error)
}

// permissionChecker is the one RBAC call a facet needs: the same check the
// plugin's own routes make for the calling user.
type permissionChecker interface {
	HasPermission(userID, envID, permission string) (bool, error)
}

// facetCollector projects the plugin's records into facet rows.
type facetCollector struct {
	src    facetSource
	rbac   permissionChecker
	logger pluginLogger
	now    func() time.Time
}

var _ plugin.ReportFacetContributor = (*reportProvider)(nil)

// ContributeFacet answers one facet for the Reporting plugin, re-checking the
// caller's own view permission because the Reporting plugin's grant says nothing
// about this plugin's data.
func (r *reportProvider) ContributeFacet(
	ctx context.Context,
	req plugin.ReportFacetRequest,
) (plugin.ReportFacetResult, error) {
	if r.plugin.svc == nil {
		return plugin.ReportFacetResult{}, fmt.Errorf("kvm: the plugin is not initialized")
	}
	collector := facetCollector{src: r.plugin.svc, logger: r.plugin.logger, now: time.Now}
	if r.plugin.host.RBAC != nil {
		collector.rbac = r.plugin.host.RBAC
	}
	return collector.contribute(ctx, req)
}

// contribute dispatches on the requested facet under the report build deadline.
func (c facetCollector) contribute(
	ctx context.Context,
	req plugin.ReportFacetRequest,
) (plugin.ReportFacetResult, error) {
	ctx, cancel := context.WithTimeout(ctx, reportBuildTimeout)
	defer cancel()

	perms := []string{"vm.view"}
	if req.Facet == plugin.ReportFacetProtection {
		perms = append(perms, "vm.backup.view")
	}

	var out plugin.ReportFacetResult
	guests, denied := c.loadGuests(ctx, req, perms, &out)
	if denied {
		return out, nil
	}

	switch req.Facet {
	case plugin.ReportFacetInventory:
		c.inventory(ctx, req, guests, &out)
	case plugin.ReportFacetCapacity:
		c.capacity(ctx, req, guests, &out)
	case plugin.ReportFacetProtection:
		c.protection(ctx, req, guests, &out)
	case plugin.ReportFacetHealth:
		c.health(ctx, req, guests, &out)
	case plugin.ReportFacetCost:
		c.cost(ctx, req, guests, &out)
	default:
		return plugin.ReportFacetResult{}, fmt.Errorf("kvm: facet %q is not contributed", req.Facet)
	}
	return out, nil
}

// notPermitted is the source a denied caller sees instead of rows, so the annex
// says the platform was withheld rather than empty.
func notPermitted() plugin.ReportSourceStatus {
	return plugin.ReportSourceStatus{Key: "kvm_permission", Label: "Virtual machines", OK: false, Note: "not permitted"}
}

// allowed reports whether the caller holds every key on envID. No RBAC handle
// or no user means no answer, and no answer means no.
func (c facetCollector) allowed(userID, envID string, perms []string) bool {
	if c.rbac == nil || userID == "" {
		return false
	}
	for _, perm := range perms {
		ok, err := c.rbac.HasPermission(userID, envID, perm)
		if err != nil {
			c.warn("kvm: facet permission check failed", "permission", perm, "error", err)
			return false
		}
		if !ok {
			return false
		}
	}
	return true
}

// warn logs when a logger is wired; the real error stays server-side.
func (c facetCollector) warn(msg string, args ...any) {
	if c.logger != nil {
		c.logger.Warn(msg, args...)
	}
}

// loadGuests reads the guests the caller may see, dropping those on hosts they
// cannot. denied is true when nothing was readable for them at all, and a read
// failure is recorded as a not-OK source with no guests.
func (c facetCollector) loadGuests(
	ctx context.Context,
	req plugin.ReportFacetRequest,
	perms []string,
	out *plugin.ReportFacetResult,
) (guests []VM, denied bool) {
	if req.EnvironmentID != "" && !c.allowed(req.UserID, req.EnvironmentID, perms) {
		out.Sources = append(out.Sources, notPermitted())
		return nil, true
	}

	started := c.now()
	all, err := c.src.List(ctx, req.EnvironmentID, false)
	source := plugin.ReportSourceStatus{
		Key: "kvm_guests", Label: "Virtual machines", OK: err == nil,
		DurationMS: c.now().Sub(started).Milliseconds(),
	}
	if err != nil {
		c.warn("kvm: facet could not read guests", "error", err)
		source.Note = "Virtual machines could not be read."
		out.Sources = append(out.Sources, source)
		return nil, false
	}
	out.Sources = append(out.Sources, source)

	if req.EnvironmentID != "" {
		return all, false
	}
	verdict := map[string]bool{}
	withheld := 0
	for _, vm := range all {
		ok, seen := verdict[vm.EnvironmentID]
		if !seen {
			ok = c.allowed(req.UserID, vm.EnvironmentID, perms)
			verdict[vm.EnvironmentID] = ok
		}
		if ok {
			guests = append(guests, vm)
		} else {
			withheld++
		}
	}
	if withheld > 0 {
		out.Sources = append(out.Sources, notPermitted())
	}
	return guests, withheld > 0 && len(guests) == 0
}

// identity stamps a guest's row. The environment is the KVM host.
func (c facetCollector) identity(req plugin.ReportFacetRequest, vm VM) plugin.ReportFacetIdentity {
	ident := plugin.ReportFacetIdentity{
		Platform: kvmPlatform, Kind: "vm", ID: vm.ID, Name: vm.Name,
		EnvironmentID: vm.EnvironmentID, Host: vm.EnvironmentID,
	}
	if vm.EnvironmentID == req.EnvironmentID {
		ident.Environment = req.Environment
	}
	return ident
}

// reachable splits off guests whose host cannot be reached, because their last
// known figures would be counted as if they were current.
func reachable(guests []VM) (ok []VM, unreachable []string) {
	for _, vm := range guests {
		if vm.HostAvailable {
			ok = append(ok, vm)
		} else {
			unreachable = append(unreachable, vm.Name)
		}
	}
	return ok, unreachable
}

// excludedNote tells the annex which guests the figures leave out.
func excludedNote(names []string) string {
	return fmt.Sprintf("%s could not be read because their host is unreachable and are excluded from the figures.",
		plural(len(names), "guest", "guests"))
}

// diskTotals reads every guest's disks once. Failed guests are counted so the
// annex can say the disk figure is short, instead of reporting a silent zero.
func (c facetCollector) diskTotals(ctx context.Context, guests []VM) (map[string][]Disk, plugin.ReportSourceStatus) {
	started := c.now()
	disks := make(map[string][]Disk, len(guests))
	failed := 0
	for _, vm := range guests {
		list, err := c.src.Disks(ctx, vm.ID)
		if err != nil {
			c.warn("kvm: facet could not read a guest's disks", "vm", vm.ID, "error", err)
			failed++
			continue
		}
		disks[vm.ID] = list
	}
	source := plugin.ReportSourceStatus{
		Key: "kvm_disks", Label: "Guest disks", OK: failed == 0,
		DurationMS: c.now().Sub(started).Milliseconds(),
	}
	if failed > 0 {
		source.Note = fmt.Sprintf("Disks of %s could not be read and count as none.", plural(failed, "guest", "guests"))
	}
	return disks, source
}

// diskBytes totals a guest's disks, leaving out optical media.
func diskBytes(disks []Disk) int64 {
	var total int64
	for _, disk := range disks {
		if !disk.IsCDROM {
			total += disk.SizeBytes
		}
	}
	return total
}

const bytesPerGB = 1 << 30

// inventory lists every guest with its hardware.
func (c facetCollector) inventory(ctx context.Context, req plugin.ReportFacetRequest, guests []VM, out *plugin.ReportFacetResult) {
	disks, source := c.diskTotals(ctx, guests)
	out.Sources = append(out.Sources, source)
	for _, vm := range guests {
		image := vm.GuestOSName
		if image == "" {
			image = vm.GuestOSFamily
		}
		out.Inventory = append(out.Inventory, plugin.ReportInventoryRow{
			FacetIdentity: c.identity(req, vm),
			State:         vm.State,
			Image:         image,
			CPU:           float64(vm.VCPUs),
			MemoryMB:      vm.MemoryMB,
			DiskGB:        float64(diskBytes(disks[vm.ID])) / bytesPerGB,
			Created:       vm.CreatedAt,
		})
	}
}

// capacity reports what guests have been given per host, and disk per pool.
// The plugin does not know a host's own CPU or memory, so Capacity is left zero.
func (c facetCollector) capacity(ctx context.Context, req plugin.ReportFacetRequest, guests []VM, out *plugin.ReportFacetResult) {
	guests, unreachable := reachable(guests)
	if len(unreachable) > 0 {
		out.Notes = append(out.Notes, excludedNote(unreachable))
	}
	out.Notes = append(out.Notes,
		"Processor and memory count running guests only. The host's own capacity is not recorded by this platform, so no capacity figure is given.")

	disks, source := c.diskTotals(ctx, guests)
	out.Sources = append(out.Sources, source)

	type hostTotals struct {
		vcpus    int
		memoryMB int64
	}
	hosts := map[string]*hostTotals{}
	poolBytes := map[[2]string]int64{}
	for _, vm := range guests {
		entry := hosts[vm.EnvironmentID]
		if entry == nil {
			entry = &hostTotals{}
			hosts[vm.EnvironmentID] = entry
		}
		if vm.Running() {
			entry.vcpus += vm.VCPUs
			entry.memoryMB += vm.MemoryMB
		}
		for _, disk := range disks[vm.ID] {
			if !disk.IsCDROM {
				poolBytes[[2]string{vm.EnvironmentID, disk.PoolID}] += disk.SizeBytes
			}
		}
	}

	hostIDs := make([]string, 0, len(hosts))
	for id := range hosts {
		hostIDs = append(hostIDs, id)
	}
	sort.Strings(hostIDs)
	for _, id := range hostIDs {
		ident := plugin.ReportFacetIdentity{
			Platform: kvmPlatform, Kind: "host", ID: id, Name: id, EnvironmentID: id, Host: id,
		}
		if id == req.EnvironmentID {
			ident.Environment = req.Environment
		}
		out.Capacity = append(out.Capacity,
			plugin.ReportCapacityRow{FacetIdentity: ident, Resource: "cpu", Allocated: float64(hosts[id].vcpus), Unit: "vCPU"},
			plugin.ReportCapacityRow{FacetIdentity: ident, Resource: "memory", Allocated: float64(hosts[id].memoryMB), Unit: "MB"})
	}

	pools := make([][2]string, 0, len(poolBytes))
	for key := range poolBytes {
		pools = append(pools, key)
	}
	sort.Slice(pools, func(i, j int) bool {
		if pools[i][0] != pools[j][0] {
			return pools[i][0] < pools[j][0]
		}
		return pools[i][1] < pools[j][1]
	})
	for _, key := range pools {
		ident := plugin.ReportFacetIdentity{
			Platform: kvmPlatform, Kind: "pool", ID: key[1], Name: key[1],
			EnvironmentID: key[0], Host: key[0],
		}
		if key[0] == req.EnvironmentID {
			ident.Environment = req.Environment
		}
		out.Capacity = append(out.Capacity, plugin.ReportCapacityRow{
			FacetIdentity: ident, Resource: "disk",
			Allocated: float64(poolBytes[key]) / bytesPerGB, Unit: "GB",
		})
	}
}

// protection says per guest whether a copy exists: a backup when one ever
// succeeded, otherwise snapshots, which share the pool with the disk they cover.
func (c facetCollector) protection(ctx context.Context, req plugin.ReportFacetRequest, guests []VM, out *plugin.ReportFacetResult) {
	guests, unreachable := reachable(guests)
	if len(unreachable) > 0 {
		out.Notes = append(out.Notes, excludedNote(unreachable))
	}

	started := c.now()
	failed := 0
	for _, vm := range guests {
		snapshots, snapErr := c.src.Snapshots(ctx, vm.ID)
		plans, planErr := c.src.ListBackupPlans(ctx, vm.ID)
		lastBackup, backupErr := c.src.lastGoodBackupAt(ctx, vm.ID)
		if snapErr != nil || planErr != nil || backupErr != nil {
			c.warn("kvm: facet could not read a guest's protection", "vm", vm.ID,
				"snapshots", snapErr, "plans", planErr, "backup", backupErr)
			failed++
			continue
		}
		out.Protection = append(out.Protection, protectionRow(c.identity(req, vm), snapshots, len(plans), lastBackup))
	}

	source := plugin.ReportSourceStatus{
		Key: "kvm_protection", Label: "Snapshots and backups", OK: failed == 0,
		DurationMS: c.now().Sub(started).Milliseconds(),
	}
	if failed > 0 {
		source.Note = fmt.Sprintf("%s could not be read and are left out.", plural(failed, "guest", "guests"))
	}
	out.Sources = append(out.Sources, source)
}

// protectionRow derives one guest's row. A successful backup is off the guest's
// pool; a snapshot is not, so it never sets OffHost.
func protectionRow(ident plugin.ReportFacetIdentity, snapshots []Snapshot, plans int, lastBackup time.Time) plugin.ReportProtectionRow {
	row := plugin.ReportProtectionRow{FacetIdentity: ident, Method: "none"}

	var newest time.Time
	consistency := ""
	for _, snapshot := range snapshots {
		if snapshot.CreatedAt.After(newest) {
			newest = snapshot.CreatedAt
			consistency = snapshot.Consistency
		}
	}

	switch {
	case !lastBackup.IsZero():
		row.Protected, row.Method, row.LastGood, row.OffHost = true, "backup", lastBackup, true
	case len(snapshots) > 0:
		row.Protected, row.Method, row.LastGood = true, "snapshot", newest
		row.Note = "Snapshots share the pool with the disk, so a pool failure takes both."
		if consistency == ConsistencyCrash {
			row.Note += " The newest is crash-consistent."
		}
	case plans > 0:
		row.Note = "A backup plan exists but no backup has ever succeeded."
	default:
		row.Note = "No snapshot and no backup plan."
	}
	return row
}

// health reads guest state, agent health and the period's unintended downs.
func (c facetCollector) health(ctx context.Context, req plugin.ReportFacetRequest, guests []VM, out *plugin.ReportFacetResult) {
	now := c.now().UTC()
	from, to := reportWindow(plugin.ReportRequest{PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd}, now)

	started := c.now()
	healthRows, err := c.src.guestHealthRows(ctx, "")
	out.Sources = append(out.Sources, c.readSource("kvm_guest_health", "Guest agent health", started, err,
		"Guest agent health could not be read and is shown as not checked."))
	if err != nil {
		healthRows = nil
	}

	started = c.now()
	events, err := c.src.readEvents(ctx, "", from)
	out.Sources = append(out.Sources, c.readSource("kvm_vm_events", "QEMU lifecycle events", started, err,
		"QEMU events could not be read, so crashes, stops and resets are not counted."))
	if err != nil {
		events = nil
	}

	started = c.now()
	intents, err := c.src.readIntents(ctx, "", from.Add(-intentPendingCover-intentSkew))
	out.Sources = append(out.Sources, c.readSource("kvm_lifecycle_intents", "Recorded lifecycle actions", started, err,
		"Recorded actions could not be read, so no stop can be told apart from one somebody asked for."))
	if err != nil {
		intents = nil
	}

	inputs := make([]stabilityVMInput, 0, len(guests))
	for _, vm := range guests {
		inputs = append(inputs, stabilityVMInput{
			vm: vm, health: healthPointer(healthRows, vm.ID), events: events[vm.ID], intents: intents[vm.ID],
		})
	}
	assessment := assessStability(inputs, from, to, now)
	byVM := make(map[string]stabilityVMRow, len(assessment.rows))
	for _, row := range assessment.rows {
		byVM[row.vm.ID] = row
	}

	for _, vm := range guests {
		row := byVM[vm.ID]
		status, detail := guestStatus(vm, row, healthPointer(healthRows, vm.ID))
		flaps := 0
		if row.flapping {
			flaps = row.unintended()
		}
		health := plugin.ReportHealthRow{
			FacetIdentity: c.identity(req, vm), Status: status,
			Restarts: row.unintended(), Flaps: flaps, Detail: detail,
		}
		if vm.LastStartedAt != nil && vm.Running() {
			health.Since = *vm.LastStartedAt
		}
		out.Health = append(out.Health, health)
	}
}

// readSource records one read, with a fixed note on failure that names no host,
// token or path; the real error goes to the log.
func (c facetCollector) readSource(key, label string, started time.Time, err error, note string) plugin.ReportSourceStatus {
	source := plugin.ReportSourceStatus{Key: key, Label: label, OK: err == nil, DurationMS: c.now().Sub(started).Milliseconds()}
	if err != nil {
		c.warn("kvm: facet read failed", "source", key, "error", err)
		source.Note = note
	}
	return source
}

// guestStatus maps a guest to up, degraded, down or unknown. A stopped guest is
// down because the report cannot tell a deliberate stop from a failure; the
// unintended-down count beside it does.
func guestStatus(vm VM, row stabilityVMRow, health *guestHealth) (string, string) {
	if !vm.HostAvailable {
		return "unknown", "The host is unreachable."
	}
	switch vm.State {
	case StateRunning:
		switch {
		case row.flapping:
			return "degraded", "Going down repeatedly."
		case row.agentStatus == "danger":
			return "degraded", "Guest agent " + row.agentText + "."
		case health != nil && health.diskAlert:
			return "degraded", "A guest filesystem is nearly full."
		}
		return "up", ""
	case StatePaused, StateMigrating, StateStopping:
		return "degraded", vm.State
	case StateCrashed:
		return "down", "Crashed."
	case StateUnknown:
		return "unknown", ""
	default:
		return "down", vm.State
	}
}

// cost is resource-hours over the period, on an allocated basis: the plugin
// knows what a guest was given, not what it used. Processor and memory count
// from the guest's last start, since stops inside the period are not kept.
func (c facetCollector) cost(ctx context.Context, req plugin.ReportFacetRequest, guests []VM, out *plugin.ReportFacetResult) {
	guests, unreachable := reachable(guests)
	if len(unreachable) > 0 {
		out.Notes = append(out.Notes, excludedNote(unreachable))
	}
	out.Notes = append(out.Notes,
		"Hours are allocated, not measured. Processor and memory count from the guest's last start while it runs; a stop and restart inside the period is not subtracted.")

	now := c.now().UTC()
	from, to := reportWindow(plugin.ReportRequest{PeriodStart: req.PeriodStart, PeriodEnd: req.PeriodEnd}, now)
	disks, source := c.diskTotals(ctx, guests)
	out.Sources = append(out.Sources, source)

	for _, vm := range guests {
		exists := laterOf(from, vm.CreatedAt)
		row := plugin.ReportCostRow{FacetIdentity: c.identity(req, vm), Basis: "allocated"}
		row.DiskGBHours = float64(diskBytes(disks[vm.ID])) / bytesPerGB * hoursBetween(exists, to)
		if vm.Running() {
			running := exists
			if vm.LastStartedAt != nil {
				running = laterOf(running, *vm.LastStartedAt)
			}
			hours := hoursBetween(running, to)
			row.CPUCoreHours = float64(vm.VCPUs) * hours
			row.MemoryGBHours = float64(vm.MemoryMB) / 1024 * hours
		}
		out.Cost = append(out.Cost, row)
	}
}

// laterOf returns the later of two instants.
func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// hoursBetween is the span in hours, never negative.
func hoursBetween(from, to time.Time) float64 {
	if !to.After(from) {
		return 0
	}
	return to.Sub(from).Hours()
}
