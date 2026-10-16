// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Swarm reports and facets.
//
// The plugin owns one report of its own, capacity and placement (is the work
// spread across the machines meant to carry it), registered as a report
// provider through the host's `reporting` adapter family
// (core/plugin/specs/reporting). What the cluster has to say to every
// platform-neutral report (inventory, capacity, health, exposure, cost) is
// contributed as facet rows from reports_facets.go, which reads through the
// same collector below.
//
// Neither plugin imports the other. An installation without Reporting loses
// nothing here (the provider registers and is simply never asked), and an
// installation without Swarm sees no Swarm shelf at all.

// Report type keys.
//
// Unchanged from the string Reporting used, deliberately: the library rows,
// the schedules and the stored documents all key on it.
const (
	reportSwarmCapacity = "swarm-capacity-placement"
)

// reportCategory is the studio shelf the report sits on.
//
// A shelf of its own rather than folding into "platform", for the reason
// it always had one: a standalone daemon has no quorum to lose, no
// placement to balance and no converging service to wait on, so the
// questions are genuinely different from a single host's.
const reportCategory = "swarm"

// Source keys for the Methodology annex. One per Docker listing the
// reports actually read, so a reader can see that (say) the nodes were
// read but the secrets were not — and, when a source is marked not-OK,
// that its absence is a gap in the evidence rather than a zero.
const (
	srcSwarmCluster  = "swarm"
	srcSwarmNodes    = "swarm_nodes"
	srcSwarmServices = "swarm_services"
	srcSwarmTasks    = "swarm_tasks"
	srcSwarmNetworks = "swarm_networks"
	srcSwarmSpecs    = "swarm_service_specs"
)

// optIncludeDetail is Reporting's own shared switch, already translated in
// its bundle. Declaring it keeps a contributed report's configure dialog
// identical to a built-in one's.
const (
	optIncludeDetail   = "include_detail"
	optIncludeGlossary = "include_glossary"
)

// swarmReportSpec declares the shelf and the facets.
//
// The type declaration is the one the Reporting catalog carried, so an
// operator's saved schedules keep asking for the same document.
// `EnvironmentScoped` is true because Swarm is a property of one
// environment's daemon, and a provider has to say it out loud.
func swarmReportSpec() plugin.ReportProviderSpec {
	common := []plugin.ReportOptionDecl{
		{Key: optIncludeDetail, Default: true},
		{Key: optIncludeGlossary, Default: true},
	}

	return plugin.ReportProviderSpec{
		Slug:        "addon-swarm",
		DisplayName: "Docker Swarm",
		Types: []plugin.ReportTypeDecl{
			{
				Key:                    reportSwarmCapacity,
				Category:               reportCategory,
				Levels:                 []string{levelTactical, levelOperational},
				EnvironmentScoped:      true,
				DefaultDays:            30,
				Options:                common,
				TitleEN:                "Swarm Capacity and Placement",
				SubtitleEN:             "Schedulable capacity, where the work landed, and what one lost node costs",
				LabelKey:               "reporting.type.capacity-placement.name",
				DescriptionKey:         "reporting.type.capacity-placement.description",
				CategoryLabelKey:       "reporting.category.label",
				CategoryDescriptionKey: "reporting.category.description",
			},
		},
		Facets: swarmFacetDecls(),
	}
}

// reportProvider is the runtime half.
//
// It holds the plugin rather than the service so it reads whatever is
// current: the handler's service is attached at Init and the capability
// cache behind it is invalidated by every lifecycle mutation, so a cluster
// initialised after boot is picked up on the next report without the
// provider caching anything of its own.
type reportProvider struct {
	plugin *SwarmPlugin
	// src overrides the plugin's own service; only tests set it.
	src swarmReader
}

var (
	_ plugin.ReportProviderClient   = (*reportProvider)(nil)
	_ plugin.ReportFacetContributor = (*reportProvider)(nil)
)

// swarmReader is the slice of Service the reports read, so a test can stand
// in for a Docker daemon.
type swarmReader interface {
	Overview(ctx context.Context, envID string) (ClusterOverview, error)
	ListNodes(ctx context.Context, envID string) ([]Node, error)
	ListServices(ctx context.Context, envID string) ([]ServiceSummary, error)
	ListTasks(ctx context.Context, envID string, f TaskFilter) ([]Task, error)
	ListOverlayNetworks(ctx context.Context, envID string) ([]OverlayNetwork, error)
	InspectService(ctx context.Context, envID, id string) (ServiceDetail, error)
}

// errReportForbidden is returned when the requesting account lacks
// `swarm.view`. Reporting surfaces it as a failed generation; it
// deliberately carries no cluster detail, because an error message is not
// a place to tell someone what they were refused access to.
var errReportForbidden = errors.New("swarm: caller lacks swarm.view")

// Subjects lists nothing.
//
// The type declares no subject kind: a Swarm report covers
// the cluster the selected environment belongs to, and the environment
// picker is already the scope control. Returning nil is the contract's way
// of saying "this provider owns no kind of that name" — the studio renders
// "nothing to narrow to" for it.
func (r *reportProvider) Subjects(_ context.Context, _ string, _ plugin.ReportRequest) ([]plugin.ReportSubjectOption, error) {
	return nil, nil
}

// Build answers one report.
func (r *reportProvider) Build(ctx context.Context, req plugin.ReportRequest) (plugin.ReportResult, error) {
	started := time.Now()
	ask := reportAsk{
		UserID: req.UserID, EnvironmentID: req.EnvironmentID,
		Environment: req.Environment, Label: req.TypeKey,
	}

	// Reporting gated this request on `reporting.generate`. That is its
	// permission, not ours: it says the caller may produce documents, not
	// that they may read this cluster. Without this check a report would
	// be a way around `swarm.view` — every service name, image, published
	// port and node hostname in the cluster, handed to someone the
	// interface refuses.
	if !r.may(ask, "swarm.view") {
		return plugin.ReportResult{}, errReportForbidden
	}

	if r.service() == nil {
		return unreachableResult(req, "this installation has no connection to a Docker daemon", started), nil
	}
	if strings.TrimSpace(req.EnvironmentID) == "" {
		return unreachableResult(req, "no environment was selected", started), nil
	}

	snap := r.collect(ctx, ask, needs{nodes: true, services: true, tasks: true})
	if !snap.capable() {
		return plugin.ReportResult{
			Sections: []plugin.ReportSection{notACluster(req, snap)},
			Sources:  snap.sources,
			Notes:    snap.notes(req.Environment, req.EnvironmentID),
		}, nil
	}

	if req.TypeKey != reportSwarmCapacity {
		// Unreachable through the host — it dispatches on the keys this
		// provider registered — but a type added to the spec and not here
		// would otherwise ship an empty document.
		return plugin.ReportResult{}, fmt.Errorf("swarm: no builder for report type %q", req.TypeKey)
	}

	return plugin.ReportResult{
		Sections: buildSwarmCapacity(snap, detailWanted(req)),
		Sources:  snap.sources,
		Notes:    snap.notes(req.Environment, req.EnvironmentID),
	}, nil
}

// service is the reader the reports use: the test override, else the
// plugin's own Swarm service, or nil before Init has attached one. Nil is a
// real state rather than a defensive check: the loader can register adapters
// on a plugin whose host wired no Docker pool, and a report is a better place
// to say so than a panic.
func (r *reportProvider) service() swarmReader {
	if r.src != nil {
		return r.src
	}
	if r.plugin == nil || r.plugin.handler == nil || r.plugin.handler.svc == nil {
		return nil
	}
	return r.plugin.handler.svc
}

// reportAsk is who is asking about which environment, whatever the entry
// point (a report Build or a facet contribution). Label names the caller in
// the log line of a failed read.
type reportAsk struct {
	UserID        string
	EnvironmentID string
	Environment   string
	Label         string
}

// may re-checks one of this plugin's own permission keys for the account
// the report is being generated for.
//
// Scoped to the request's environment, which is the scope every Swarm read
// key carries in the catalog. A missing RBAC handle or a missing user id
// denies: a report that cannot establish who asked must not answer.
func (r *reportProvider) may(ask reportAsk, perm string) bool {
	host := r.plugin.host
	if host.RBAC == nil || strings.TrimSpace(ask.UserID) == "" {
		return false
	}
	allowed, err := host.RBAC.HasPermission(ask.UserID, ask.EnvironmentID, perm)
	if err != nil {
		if host.Logger != nil {
			host.Logger.Error("swarm: report permission check failed",
				"type", ask.Label, "permission", perm, "error", err)
		}
		return false
	}
	return allowed
}

// detailWanted reads Reporting's shared "include_detail" switch. Absent
// means on: a report generated by a caller that sent no options at all — a
// schedule saved before the option existed — should carry more rather than
// less.
func detailWanted(req plugin.ReportRequest) bool {
	if v, ok := req.Options[optIncludeDetail]; ok {
		return v
	}
	return true
}

// noticeLevel picks the highest altitude the caller kept, so a notice
// lands in a section that will actually be rendered.
//
// Reporting drops any section whose level was unchecked, so hard-coding
// operational here would let a reader who asked for only the strategic
// section receive a document with no content and no explanation of why.
func noticeLevel(req plugin.ReportRequest) string {
	for _, want := range []string{levelStrategic, levelTactical, levelOperational} {
		for _, have := range req.Levels {
			if strings.EqualFold(strings.TrimSpace(have), want) {
				return want
			}
		}
	}
	return levelOperational
}

// --- collection -------------------------------------------------------

// reportSnapshot is one reading of the cluster.
//
// Every listing is taken once and shared by every section, so a document
// can never contradict itself between two of its own tables — and so a
// cluster with a hundred services is listed once rather than per section.
type reportSnapshot struct {
	overview ClusterOverview
	nodes    []Node
	services []ServiceSummary
	tasks    []Task
	networks []OverlayNetwork
	// specs holds each service's reservation and limit, keyed by service id,
	// for the services whose inspect answered.
	specs map[string]ServiceResources

	// clusterOK records whether the overview itself answered. It is the
	// difference between "this daemon is not in a cluster" (a correct,
	// unremarkable answer) and "we could not tell" (a gap in the
	// evidence), and the two must never be printed as each other.
	clusterOK bool

	sources []plugin.ReportSourceStatus
}

// needs names the listings one caller wants, so a facet that has no use for
// the task history does not pay for it.
type needs struct {
	nodes, services, tasks, networks, specs bool
}

// capable reports whether the environment is part of a cluster that
// answered. A worker whose control plane is unreachable is not capable
// here even though `Capability.Capable` is true on it: every listing below
// requires a manager, so a report built from one would be a page of zeros.
func (s *reportSnapshot) capable() bool {
	return s.clusterOK && s.overview.Capability.Capable && s.overview.Capability.ControlAvailable
}

// notes are the caveats that qualify the whole document rather than any
// one section.
func (s *reportSnapshot) notes(environment, environmentID string) []string {
	notes := []string{
		"Read from the Docker daemon of environment " +
			orDash(environment, environmentID) + ", at the moment shown on the cover.",
	}
	if s.capable() && s.overview.Capability.NodeRole != "manager" {
		notes = append(notes, "The daemon this report was collected from is a Swarm worker. "+
			"SwarmKit answers control-plane listings only on a manager, so parts of this "+
			"document may be thinner than the same report taken from a manager.")
	}
	if s.capable() {
		notes = append(notes, "Docker retains a bounded task history per service. A failure that "+
			"fell out of that window is absent from this document, which is not the same as "+
			"it not having happened.")
	}
	return notes
}

// collect reads exactly what the caller needs.
//
// Each read is gated on the same permission key that gates its own
// endpoint, so a report can never assemble something the interface would
// have refused — a denied listing is recorded as a source that did not
// answer, exactly like an unreachable one, because to the reader it is one.
func (r *reportProvider) collect(ctx context.Context, ask reportAsk, want needs) *reportSnapshot {
	snap := &reportSnapshot{}
	svc := r.service()
	env := ask.EnvironmentID

	r.read(ctx, ask, snap, srcSwarmCluster, "Swarm cluster", "swarm.view", func(ctx context.Context) (int, error) {
		overview, err := svc.Overview(ctx, env)
		if err != nil {
			return 0, err
		}
		snap.overview = overview
		snap.clusterOK = true
		if overview.Cluster == nil {
			return 0, nil
		}
		return 1, nil
	})

	// Nothing below exists on a standalone daemon, and asking anyway
	// produces a page of "not a swarm" errors in the annex that say
	// nothing the cluster row has not already said.
	if !snap.capable() {
		return snap
	}

	if want.nodes {
		r.read(ctx, ask, snap, srcSwarmNodes, "Swarm nodes", "swarm_nodes.view", func(ctx context.Context) (int, error) {
			nodes, err := svc.ListNodes(ctx, env)
			snap.nodes = nodes
			return len(nodes), err
		})
	}
	if want.services || want.specs {
		r.read(ctx, ask, snap, srcSwarmServices, "Swarm services", "swarm_services.view", func(ctx context.Context) (int, error) {
			services, err := svc.ListServices(ctx, env)
			snap.services = services
			return len(services), err
		})
	}
	if want.tasks {
		r.read(ctx, ask, snap, srcSwarmTasks, "Swarm tasks", "swarm_tasks.view", func(ctx context.Context) (int, error) {
			tasks, err := svc.ListTasks(ctx, env, TaskFilter{})
			snap.tasks = tasks
			return len(tasks), err
		})
	}
	if want.networks {
		r.read(ctx, ask, snap, srcSwarmNetworks, "Overlay networks", "swarm_networks.view", func(ctx context.Context) (int, error) {
			networks, err := svc.ListOverlayNetworks(ctx, env)
			snap.networks = networks
			return len(networks), err
		})
	}
	if want.specs && len(snap.services) > 0 {
		r.readSpecs(ctx, ask, snap)
	}
	return snap
}

// specWorkers bounds the concurrent service inspects: a cluster with a few
// hundred services is read in seconds without opening a few hundred daemon
// calls at once.
const specWorkers = 8

// errSpecsPartial marks a resource read where some services could not be
// inspected; the underlying errors stay out of the document (see read).
var errSpecsPartial = errors.New("swarm: some services could not be inspected")

// readSpecs inspects every listed service for its resource reservation, which
// the list view does not carry. One source covers the lot; it is not-OK when
// any inspect failed, because a figure built from a partial read is a lower
// bound and the annex must say so.
func (r *reportProvider) readSpecs(ctx context.Context, ask reportAsk, snap *reportSnapshot) {
	svc := r.service()
	snap.specs = make(map[string]ServiceResources, len(snap.services))
	r.read(ctx, ask, snap, srcSwarmSpecs, "Swarm service resources", "swarm_services.view", func(ctx context.Context) (int, error) {
		var (
			mu     sync.Mutex
			wg     sync.WaitGroup
			failed int
		)
		sem := make(chan struct{}, specWorkers)
		for _, s := range snap.services {
			if ctx.Err() != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				continue
			}
			sem <- struct{}{}
			wg.Add(1)
			// goroutine lifecycle: each worker ends when its inspect returns and wg.Wait below joins them all.
			go func(id string) {
				defer wg.Done()
				defer func() { <-sem }()
				detail, err := svc.InspectService(ctx, ask.EnvironmentID, id)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failed++
					return
				}
				snap.specs[id] = detail.Resources
			}(s.ID)
		}
		wg.Wait()
		if failed > 0 {
			return len(snap.specs), errSpecsPartial
		}
		return len(snap.specs), nil
	})
}

// read runs one listing and records its outcome in the annex.
//
// The error itself never reaches the document. A Docker failure routinely
// names the socket path, the TLS material or the daemon host, none of
// which has any business in a circulated PDF — so the note is a fixed
// sentence and the detail goes to this plugin's own log.
func (r *reportProvider) read(
	ctx context.Context,
	ask reportAsk,
	snap *reportSnapshot,
	key, label, perm string,
	fn func(context.Context) (int, error),
) {
	if !r.may(ask, perm) {
		snap.sources = append(snap.sources, plugin.ReportSourceStatus{
			Key: key, Label: label, OK: false,
			Note: "the requesting account may not read this, so it is absent from every " +
				"figure in this document rather than counted as zero",
		})
		return
	}

	started := time.Now()
	// The Docker SDK is given a deadline of its own so one wedged listing
	// cannot consume the whole generation's budget and leave the remaining
	// sources unread with no explanation.
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	n, err := fn(callCtx)
	status := plugin.ReportSourceStatus{
		Key: key, Label: label, OK: err == nil,
		DurationMS: time.Since(started).Milliseconds(),
	}
	switch {
	case err != nil:
		status.Note = "the Docker daemon did not answer this listing"
		if host := r.plugin.host; host.Logger != nil {
			host.Logger.Error("swarm: report collection failed",
				"source", key, "type", ask.Label, "env", ask.EnvironmentID, "error", err)
		}
	case n == 0:
		status.Note = "nothing returned; the cluster holds none of these"
	}
	snap.sources = append(snap.sources, status)
}

// --- the documents produced when there is nothing to report on --------

// notACluster is what every Swarm report prints instead of content when
// the selected environment is a standalone daemon, or when the daemon
// could not be reached at all.
//
// The two cases are told apart deliberately. A standalone daemon is a
// correct, unremarkable answer; a daemon that did not answer is a gap in
// the evidence, and a report that presents the second as the first is
// telling the reader the estate is simpler than it is.
func notACluster(req plugin.ReportRequest, snap *reportSnapshot) plugin.ReportSection {
	if !snap.clusterOK {
		return rsec(noticeLevel(req), "Swarm",
			rcallout(statusWarn, "Swarm could not be read",
				"The cluster could not be read for this environment, so nothing in this "+
					"report describes Swarm. This is a gap in the evidence, not a finding "+
					"about the estate. The Methodology annex records which listings failed."))
	}
	if snap.overview.Capability.Capable && !snap.overview.Capability.ControlAvailable {
		return rsec(noticeLevel(req), "Swarm",
			rcallout(statusWarn, "Collected from a worker node",
				"This environment's daemon is part of a cluster but is a worker, and SwarmKit "+
					"answers control-plane listings only on a manager. Nothing below could be "+
					"read. Run this report against an environment whose daemon is a manager."))
	}
	return rsec(noticeLevel(req), "Swarm",
		rcallout(statusInfo, "Not a Swarm environment",
			"The environment named on the cover is a standalone Docker daemon: it is not a "+
				"member of a Swarm cluster, so there are no nodes, services or tasks to "+
				"report on. Run this report against an environment whose daemon has joined "+
				"a cluster, or use the container reports for a standalone host."))
}

// unreachableResult is the document produced when there was nothing to
// even attempt a reading against.
//
// A real report rather than an error, for the same reason Reporting marks
// a missing source "not available" instead of scoring it zero: silence and
// health look identical in a table of zeros, and only one of them is worth
// acting on.
func unreachableResult(req plugin.ReportRequest, why string, started time.Time) plugin.ReportResult {
	return plugin.ReportResult{
		Sections: []plugin.ReportSection{{
			Level: "summary",
			Title: "Executive summary",
			Blocks: []plugin.ReportBlock{
				{Kind: "paragraph", Lead: true, Text: "This report could not be produced: " + why + "."},
				{Kind: "callout", Tone: statusWarn, Title: "No data collected",
					Text: "Nothing below should be read as a measurement of " +
						orDash(req.Environment, "the selected environment") +
						". No figure in this document was derived from the cluster, and the " +
						"absence of findings is the absence of a reading, not a clean result."},
				{Kind: "list", Items: []string{
					"Check the environment is listed and connected under Environments.",
					"Confirm its Docker daemon is reachable from this McHarbor installation.",
					"Confirm the daemon has joined a Swarm cluster and that this node is a manager.",
				}},
			},
		}},
		Sources: []plugin.ReportSourceStatus{{
			Key: srcSwarmCluster, Label: "Swarm cluster", OK: false,
			Note: why, DurationMS: time.Since(started).Milliseconds(),
		}},
	}
}
