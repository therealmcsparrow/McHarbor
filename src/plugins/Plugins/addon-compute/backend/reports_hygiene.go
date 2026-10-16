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

// Thresholds the hygiene report classifies against. A download that has not
// written progress for cacheStuckAfter is not being watched by anything, and a
// lease no agent has refreshed for leaseStaleAfter describes a guest that is gone.
const (
	cacheStuckAfter = 6 * time.Hour
	leaseStaleAfter = 7 * 24 * time.Hour
)

// Classes a cache row or a lease falls into. They are report vocabulary, not
// stored states, so they live here rather than beside the CacheState constants.
const (
	cacheClassReady      = "ready"
	cacheClassInProgress = "in progress"
	cacheClassStuck      = "stuck"
	cacheClassFailed     = "failed"

	leaseClassCurrent  = "current"
	leaseClassStale    = "stale"
	leaseClassExpired  = "expired"
	leaseClassOrphaned = "orphaned"
)

// classifyCache judges one cache row. A pending or downloading row is only "in
// progress" while it keeps writing; after that it is a promise nobody will keep.
func classifyCache(record imageCacheRecord, now time.Time) string {
	switch record.State {
	case CacheStateReady:
		return cacheClassReady
	case CacheStateFailed:
		return cacheClassFailed
	case CacheStatePending, CacheStateDownloading:
		if record.UpdatedAt.IsZero() || now.Sub(record.UpdatedAt) > cacheStuckAfter {
			return cacheClassStuck
		}
		return cacheClassInProgress
	default:
		return cacheClassStuck
	}
}

// classifyLease judges one lease against the hosts that still run compute. An
// orphan outranks an expiry: a lease on a deleted network cannot be renewed at all.
func classifyLease(record leaseRecord, now time.Time, enabled map[string]bool) string {
	switch {
	case !record.NetworkFound, !enabled[record.EnvironmentID]:
		return leaseClassOrphaned
	case record.ExpiresAt != nil && record.ExpiresAt.Before(now):
		return leaseClassExpired
	case record.UpdatedAt.IsZero() || now.Sub(record.UpdatedAt) > leaseStaleAfter:
		return leaseClassStale
	default:
		return leaseClassCurrent
	}
}

// hygieneInput is everything the analysis reads, as plain rows, so the whole
// judgement can be tested without a database or an agent.
type hygieneInput struct {
	Now          time.Time
	Scoped       bool
	Environments []ComputeEnvironment
	Images       []Image
	Caches       []imageCacheRecord
	Leases       []leaseRecord
	Pools        []Pool
	Locations    LocationsView
}

// hostHygiene is one compute host's line in the cache table.
type hostHygiene struct {
	EnvironmentID string
	Name          string
	ImagePath     string
	ImageSource   string
	Ready         int
	InProgress    int
	Stuck         int
	Failed        int
	CachedBytes   int64
	ReadyPools    int
	Leases        int
	BadLeases     int
}

// imageUsage is one catalog image and the hosts holding it.
type imageUsage struct {
	Image      Image
	ReadyOn    int
	InProgress int
	FailedOn   int
}

// cacheIssue is a cache row that needs an operator.
type cacheIssue struct {
	Record imageCacheRecord
	Image  string
	Host   string
	Class  string
}

// leaseIssue is a lease that is not current.
type leaseIssue struct {
	Record leaseRecord
	Host   string
	Class  string
}

// hygieneAnalysis is every figure, table and finding source the report prints.
type hygieneAnalysis struct {
	Hosts        []hostHygiene
	Images       []imageUsage
	CacheIssues  []cacheIssue
	LeaseIssues  []leaseIssue
	OrphanCaches int
	IdleHosts    int
	Unused       int

	ReadyCopies int
	InProgress  int
	Stuck       int
	Failed      int

	LeasesTotal  int
	Current      int
	Stale        int
	Expired      int
	Orphaned     int
	Unattributed int
}

// analyseHygiene classifies every cache row and lease in scope. Rows for a host
// outside the scope are ignored, so a scoped report never calls another host's
// rows orphaned.
func analyseHygiene(in hygieneInput) hygieneAnalysis {
	var out hygieneAnalysis

	inScope := map[string]bool{}
	enabled := map[string]bool{}
	names := map[string]string{}
	hostIndex := map[string]int{}
	for _, env := range in.Environments {
		inScope[env.EnvironmentID] = true
		names[env.EnvironmentID] = hostName(env)
		if !env.ComputeEnabled {
			continue
		}
		enabled[env.EnvironmentID] = true
		host := hostHygiene{EnvironmentID: env.EnvironmentID, Name: hostName(env)}
		host.ImagePath, host.ImageSource = effectiveImagePath(in.Locations, env.EnvironmentID)
		hostIndex[env.EnvironmentID] = len(out.Hosts)
		out.Hosts = append(out.Hosts, host)
	}
	relevant := func(envID string) bool { return !in.Scoped || inScope[envID] }

	for _, pool := range in.Pools {
		if i, ok := hostIndex[pool.EnvironmentID]; ok && pool.State == StateReady {
			out.Hosts[i].ReadyPools++
		}
	}

	imageIndex := map[string]int{}
	out.Images = make([]imageUsage, 0, len(in.Images))
	for _, image := range in.Images {
		imageIndex[image.ID] = len(out.Images)
		out.Images = append(out.Images, imageUsage{Image: image})
	}

	for _, record := range in.Caches {
		if !relevant(record.EnvironmentID) {
			continue
		}
		i, ok := hostIndex[record.EnvironmentID]
		if !ok {
			out.OrphanCaches++
			continue
		}
		class := classifyCache(record, in.Now)
		host := &out.Hosts[i]
		var usage *imageUsage
		if j, found := imageIndex[record.ImageID]; found {
			usage = &out.Images[j]
		}

		switch class {
		case cacheClassReady:
			host.Ready++
			host.CachedBytes += record.SizeBytes
			out.ReadyCopies++
			if usage != nil {
				usage.ReadyOn++
			}
		case cacheClassInProgress:
			host.InProgress++
			out.InProgress++
			if usage != nil {
				usage.InProgress++
			}
		case cacheClassStuck:
			host.Stuck++
			out.Stuck++
		case cacheClassFailed:
			host.Failed++
			out.Failed++
			if usage != nil {
				usage.FailedOn++
			}
		}
		if class == cacheClassStuck || class == cacheClassFailed {
			label := record.ImageID
			if usage != nil {
				label = usage.Image.DisplayName
			}
			out.CacheIssues = append(out.CacheIssues, cacheIssue{
				Record: record, Image: label, Host: host.Name, Class: class,
			})
		}
	}

	for _, usage := range out.Images {
		if usage.ReadyOn == 0 && usage.InProgress == 0 {
			out.Unused++
		}
	}
	for _, host := range out.Hosts {
		if host.Ready > 0 && host.ReadyPools == 0 {
			out.IdleHosts++
		}
	}

	for _, record := range in.Leases {
		if in.Scoped && (!record.NetworkFound || !inScope[record.EnvironmentID]) {
			continue
		}
		out.LeasesTotal++
		if record.GuestID == "" {
			out.Unattributed++
		}
		class := classifyLease(record, in.Now, enabled)
		if i, ok := hostIndex[record.EnvironmentID]; ok {
			out.Hosts[i].Leases++
			if class != leaseClassCurrent {
				out.Hosts[i].BadLeases++
			}
		}
		switch class {
		case leaseClassCurrent:
			out.Current++
			continue
		case leaseClassStale:
			out.Stale++
		case leaseClassExpired:
			out.Expired++
		case leaseClassOrphaned:
			out.Orphaned++
		}
		host := names[record.EnvironmentID]
		if host == "" {
			host = record.EnvironmentID
		}
		out.LeaseIssues = append(out.LeaseIssues, leaseIssue{Record: record, Host: host, Class: class})
	}

	sort.SliceStable(out.CacheIssues, func(i, j int) bool {
		if out.CacheIssues[i].Class != out.CacheIssues[j].Class {
			return out.CacheIssues[i].Class == cacheClassFailed
		}
		return out.CacheIssues[i].Record.UpdatedAt.Before(out.CacheIssues[j].Record.UpdatedAt)
	})
	sort.SliceStable(out.LeaseIssues, func(i, j int) bool {
		return leaseClassRank(out.LeaseIssues[i].Class) < leaseClassRank(out.LeaseIssues[j].Class)
	})
	return out
}

func leaseClassRank(class string) int {
	switch class {
	case leaseClassOrphaned:
		return 0
	case leaseClassExpired:
		return 1
	default:
		return 2
	}
}

// effectiveImagePath answers where a host keeps images from the stored rows
// alone; an empty path means the agent's own default, which only the agent knows.
func effectiveImagePath(view LocationsView, envID string) (string, string) {
	stored := Locations{ScopeID: envID}
	for _, override := range view.Overrides {
		if override.ScopeID == envID {
			stored = override
			break
		}
	}
	effective := resolve(stored, view.Global, Locations{}, envID)
	return effective.ImagePath, effective.ImageSource
}

// hygieneFindings names what to fix, in the order it costs the operator most.
func hygieneFindings(a hygieneAnalysis) []plugin.ReportFinding {
	var findings []plugin.ReportFinding
	if a.Failed > 0 {
		findings = append(findings, finding("high",
			plural(a.Failed, "image download has failed", "image downloads have failed"),
			"A host with a failed cache entry cannot create a guest from that image; the operator finds out at the end of the create form.",
			"Open Compute → Images on each host listed below, read the recorded error, and retry or evict the entry."))
	}
	if a.Stuck > 0 {
		findings = append(findings, finding("medium",
			fmt.Sprintf("%s not moved in over %d hours", plural(a.Stuck, "image download has", "image downloads have"), int(cacheStuckAfter.Hours())),
			"The row still claims a download is under way, which hides the Download button behind a progress bar that will not move.",
			"Check that the host's agent is online, then evict the entry and download it again."))
	}
	if stale := a.Stale + a.Expired; stale > 0 {
		findings = append(findings, finding("medium",
			plural(stale, "DHCP lease is stale or expired", "DHCP leases are stale or expired"),
			fmt.Sprintf("No agent has refreshed these for over %d days or they are past their expiry, so they reserve addresses for guests that are probably gone.", int(leaseStaleAfter.Hours()/24)),
			"Confirm the guests no longer exist, then release the leases so the addresses return to the pool."))
	}
	if a.Orphaned > 0 {
		findings = append(findings, finding("medium",
			plural(a.Orphaned, "DHCP lease points at a network or host that no longer runs compute", "DHCP leases point at a network or host that no longer runs compute"),
			"Nothing will ever renew or release these rows, so they will stay in the table and in every lease count indefinitely.",
			"Delete the leases, or re-enable compute on the host if it was switched off by mistake."))
	}
	if a.OrphanCaches > 0 {
		findings = append(findings, finding("low",
			plural(a.OrphanCaches, "cache entry belongs to a host that no longer runs compute", "cache entries belong to a host that no longer runs compute"),
			"The files may still be on that host's disk, and McHarbor has no page left that would show or evict them.",
			"Re-enable compute on the host long enough to evict the images, or remove the files on the host by hand."))
	}
	if a.IdleHosts > 0 {
		findings = append(findings, finding("low",
			plural(a.IdleHosts, "host holds images but has no ready storage pool", "hosts hold images but have no ready storage pool"),
			"A guest disk is created in a pool, so these images use space on a host that cannot currently build anything from them.",
			"Create or repair a storage pool on the host, or evict images it will not use."))
	}
	if a.Unused > 0 {
		findings = append(findings, finding("info",
			plural(a.Unused, "catalog image is not held by any host in scope", "catalog images are not held by any host in scope"),
			"Nothing in scope can create a guest from these without downloading them first.",
			"Remove uploads nobody needs; leave built-in entries, which cost nothing until downloaded."))
	}
	return findings
}

// hygieneLead writes the report's opening paragraph.
func hygieneLead(a hygieneAnalysis) string {
	if len(a.Hosts) == 0 {
		return "No host in this scope has compute enabled, so there is no image cache or lease table to assess."
	}
	body := fmt.Sprintf("%s hold %s of catalog images. ",
		plural(len(a.Hosts), "compute host", "compute hosts"),
		plural(a.ReadyCopies, "ready copy", "ready copies"))
	if a.Failed+a.Stuck == 0 {
		body += "No download is failed or stuck. "
	} else {
		body += fmt.Sprintf("%d failed and %d stuck downloads need an operator. ", a.Failed, a.Stuck)
	}
	if bad := a.Stale + a.Expired + a.Orphaned; bad > 0 {
		body += fmt.Sprintf("%d of %s are no longer current and hold addresses nothing will use.",
			bad, plural(a.LeasesTotal, "DHCP lease", "DHCP leases"))
	} else {
		body += fmt.Sprintf("All %s are current.", plural(a.LeasesTotal, "DHCP lease", "DHCP leases"))
	}
	return body
}

// buildHygiene reports image cache state, image use and lease staleness.
func (r *reportProvider) buildHygiene(
	ctx context.Context,
	req plugin.ReportRequest,
	environments []ComputeEnvironment,
	sources []plugin.ReportSourceStatus,
) (plugin.ReportResult, error) {
	in := hygieneInput{
		Now:          time.Now().UTC(),
		Scoped:       req.EnvironmentID != "" || (req.SubjectKind == "compute-host" && req.SubjectID != ""),
		Environments: environments,
	}
	var notes []string

	read := func(key, label string, fn func() (int, error)) {
		started := time.Now()
		count, err := fn()
		status := plugin.ReportSourceStatus{Key: key, Label: label, OK: err == nil,
			DurationMS: time.Since(started).Milliseconds()}
		if err != nil {
			r.plugin.logger.Warn("compute: hygiene report source failed", "source", key, "error", err)
			status.Note = "The read failed; its figures are left out rather than shown as zero."
		} else if count >= maxRows {
			notes = append(notes, fmt.Sprintf("%s reached the %d-row read limit, so the figures from it may be incomplete.", label, maxRows))
		}
		sources = append(sources, status)
	}

	read("compute_images", "Image catalog", func() (int, error) {
		images, err := r.plugin.svc.ListImages(ctx, "", "")
		in.Images = images
		return len(images), err
	})
	read("compute_image_cache", "Image cache", func() (int, error) {
		caches, err := r.plugin.svc.allImageCaches(ctx)
		in.Caches = caches
		return len(caches), err
	})
	read("compute_leases", "DHCP leases", func() (int, error) {
		leases, err := r.plugin.svc.allLeases(ctx)
		in.Leases = leases
		return len(leases), err
	})
	read("compute_pools", "Storage pools", func() (int, error) {
		pools, err := r.plugin.svc.ListPools(ctx, "")
		in.Pools = pools
		return len(pools), err
	})
	read("compute_locations", "Compute locations", func() (int, error) {
		view, err := r.plugin.svc.ListLocations(ctx)
		in.Locations = view
		return len(view.Overrides), err
	})

	a := analyseHygiene(in)
	onlyIssues := req.Options["onlyIssues"]
	notes = append(notes,
		"McHarbor does not record which guest was created from which image, so an image counts as used when a host in scope holds a ready copy of it.",
		"Cache state is what McHarbor last recorded; this report does not ask any agent what it holds.")

	summary := []plugin.ReportBlock{
		leadBlock(hygieneLead(a)),
		{
			Kind: "kpis",
			KPIs: []plugin.ReportKPI{
				{Label: "Compute hosts", Value: fmt.Sprintf("%d", len(a.Hosts))},
				{Label: "Ready image copies", Value: fmt.Sprintf("%d", a.ReadyCopies)},
				{Label: "Failed downloads", Value: fmt.Sprintf("%d", a.Failed), Status: kpiStatus(a.Failed == 0)},
				{Label: "Stuck downloads", Value: fmt.Sprintf("%d", a.Stuck), Status: kpiStatus(a.Stuck == 0)},
				{Label: "Unused images", Value: fmt.Sprintf("%d", a.Unused)},
				{Label: "Leases not current", Value: fmt.Sprintf("%d", a.Stale+a.Expired+a.Orphaned),
					Status: kpiStatus(a.Stale+a.Expired+a.Orphaned == 0),
					Hint:   fmt.Sprintf("Of %d leases in scope.", a.LeasesTotal)},
			},
		},
	}
	if findings := hygieneFindings(a); len(findings) > 0 {
		summary = append(summary, findingsBlock(findings))
	}

	sections := []plugin.ReportSection{
		{Level: "summary", Title: "Summary", Blocks: summary},
		{Level: "tactical", Title: "Image cache by host", Blocks: hygieneHostBlocks(a, onlyIssues)},
		{Level: "tactical", Title: "Image usage", Blocks: hygieneImageBlocks(a, onlyIssues)},
		{Level: "operational", Title: "Cache entries needing attention", Blocks: hygieneCacheIssueBlocks(a)},
		{Level: "operational", Title: "DHCP leases not current", Blocks: hygieneLeaseBlocks(a)},
	}

	return plugin.ReportResult{Sections: sections, Sources: sources, Notes: notes}, nil
}

func hygieneHostBlocks(a hygieneAnalysis, onlyIssues bool) []plugin.ReportBlock {
	rows := make([][]plugin.ReportCell, 0, len(a.Hosts))
	for _, host := range a.Hosts {
		idle := host.Ready > 0 && host.ReadyPools == 0
		if onlyIssues && host.Failed+host.Stuck+host.BadLeases == 0 && !idle {
			continue
		}
		path := host.ImagePath
		if path == "" {
			path = "agent default"
		}
		pools := plugin.ReportCell{Text: fmt.Sprintf("%d", host.ReadyPools)}
		if idle {
			pools.Status = "warning"
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: host.Name},
			{Text: path, Mono: host.ImagePath != ""},
			{Text: host.ImageSource},
			{Text: fmt.Sprintf("%d", host.Ready)},
			{Text: fmt.Sprintf("%d", host.InProgress)},
			countCell(host.Stuck, "warning"),
			countCell(host.Failed, "danger"),
			{Text: humanBytes(host.CachedBytes)},
			pools,
			countCell(host.BadLeases, "warning"),
		})
	}

	blocks := []plugin.ReportBlock{{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Host"}, {Label: "Image location", Mono: true}, {Label: "Set at"},
			{Label: "Ready", Align: "right"}, {Label: "In progress", Align: "right"},
			{Label: "Stuck", Align: "right"}, {Label: "Failed", Align: "right"},
			{Label: "Cached size", Align: "right"}, {Label: "Ready pools", Align: "right"},
			{Label: "Leases not current", Align: "right"},
		},
		Rows: rows,
		Note: "\"Set at\" says whether the image location is a per-host override, the global setting, or the agent's own default.",
	}}

	if a.ReadyCopies+a.InProgress+a.Stuck+a.Failed > 0 {
		points := []plugin.ReportChartPoint{}
		for _, slice := range []struct {
			label string
			value int
			tone  string
		}{
			{cacheClassReady, a.ReadyCopies, "good"},
			{cacheClassInProgress, a.InProgress, ""},
			{cacheClassStuck, a.Stuck, "warn"},
			{cacheClassFailed, a.Failed, "bad"},
		} {
			if slice.value > 0 {
				points = append(points, chartPoint(slice.label, float64(slice.value), slice.tone))
			}
		}
		blocks = append(blocks, donutChart("Cache entries by state", points))
	}
	return blocks
}

func hygieneImageBlocks(a hygieneAnalysis, onlyIssues bool) []plugin.ReportBlock {
	images := append([]imageUsage(nil), a.Images...)
	sort.SliceStable(images, func(i, j int) bool {
		if images[i].FailedOn != images[j].FailedOn {
			return images[i].FailedOn > images[j].FailedOn
		}
		return images[i].ReadyOn < images[j].ReadyOn
	})

	rows := make([][]plugin.ReportCell, 0, len(images))
	for _, usage := range images {
		unused := usage.ReadyOn == 0 && usage.InProgress == 0
		if onlyIssues && !unused && usage.FailedOn == 0 {
			continue
		}
		status := plugin.ReportCell{Text: "in use", Status: "success", Pill: true}
		switch {
		case usage.FailedOn > 0:
			status = plugin.ReportCell{Text: "failing", Status: "danger", Pill: true}
		case unused:
			status = plugin.ReportCell{Text: "unused", Pill: true}
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: usage.Image.DisplayName},
			{Text: usage.Image.Kind},
			{Text: usage.Image.Origin},
			{Text: fmt.Sprintf("%d", usage.ReadyOn)},
			countCell(usage.FailedOn, "danger"),
			status,
		})
	}
	return []plugin.ReportBlock{{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Image"}, {Label: "Kind"}, {Label: "Origin"},
			{Label: "Ready on hosts", Align: "right"}, {Label: "Failed on hosts", Align: "right"},
			{Label: "Status"},
		},
		Rows:    rows,
		Caption: "Failing first, then least held.",
	}}
}

func hygieneCacheIssueBlocks(a hygieneAnalysis) []plugin.ReportBlock {
	if len(a.CacheIssues) == 0 {
		return []plugin.ReportBlock{paraBlock("No cache entry is failed or stuck.")}
	}
	rows := make([][]plugin.ReportCell, 0, len(a.CacheIssues))
	for _, issue := range a.CacheIssues {
		tone := "warning"
		if issue.Class == cacheClassFailed {
			tone = "danger"
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: issue.Image},
			{Text: issue.Host},
			{Text: issue.Class, Status: tone, Pill: true},
			{Text: orDash(issue.Record.Phase)},
			{Text: fmt.Sprintf("%d%%", issue.Record.ProgressPct)},
			{Text: formatDate(issue.Record.UpdatedAt)},
		})
	}
	return []plugin.ReportBlock{{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Image"}, {Label: "Host"}, {Label: "Class"},
			{Label: "Phase"}, {Label: "Progress", Align: "right"}, {Label: "Last change"},
		},
		Rows: rows,
		Note: "The recorded error for each entry is on the host's Images page; it is not copied here because it can name source URLs and host paths.",
	}}
}

func hygieneLeaseBlocks(a hygieneAnalysis) []plugin.ReportBlock {
	if len(a.LeaseIssues) == 0 {
		return []plugin.ReportBlock{paraBlock("Every DHCP lease in scope is current.")}
	}
	rows := make([][]plugin.ReportCell, 0, len(a.LeaseIssues))
	for _, issue := range a.LeaseIssues {
		tone := "warning"
		if issue.Class == leaseClassOrphaned {
			tone = "danger"
		}
		network := issue.Record.NetworkName
		if !issue.Record.NetworkFound {
			network = "deleted"
		}
		expires := "—"
		if issue.Record.ExpiresAt != nil {
			expires = formatDate(*issue.Record.ExpiresAt)
		}
		rows = append(rows, []plugin.ReportCell{
			{Text: issue.Record.IP, Mono: true},
			{Text: issue.Record.MAC, Mono: true},
			{Text: orDash(issue.Record.Hostname)},
			{Text: orDash(network)},
			{Text: orDash(issue.Host)},
			{Text: issue.Class, Status: tone, Pill: true},
			{Text: formatDate(issue.Record.UpdatedAt)},
			{Text: expires},
		})
	}
	return []plugin.ReportBlock{{
		Kind: "table",
		Columns: []plugin.ReportColumn{
			{Label: "Address", Mono: true}, {Label: "MAC", Mono: true}, {Label: "Hostname"},
			{Label: "Network"}, {Label: "Host"}, {Label: "Class"},
			{Label: "Last update"}, {Label: "Expires"},
		},
		Rows:    rows,
		Caption: "Orphaned first, then expired, then stale.",
		Note:    "Hostnames are supplied by the guest and are shown as reported.",
	}}
}

func countCell(count int, tone string) plugin.ReportCell {
	cell := plugin.ReportCell{Text: fmt.Sprintf("%d", count)}
	if count > 0 {
		cell.Status = tone
	}
	return cell
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.Format("2006-01-02 15:04")
}
