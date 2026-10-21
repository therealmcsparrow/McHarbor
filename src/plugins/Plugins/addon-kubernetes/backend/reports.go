// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Kubernetes reports.
//
// The Reporting plugin owns the Report Studio, the library, the PDF and
// Word writers and the schedules. It does not own the questions — and
// it has no business knowing what a DaemonSet is. So this plugin
// registers a *report provider* through the host's `reporting` adapter
// family: it declares the report types it can answer about a cluster
// (and the cross-platform facets it contributes rows to) and builds the sections that answer them, and Reporting supplies the
// cover, the period, the methodology annex and the three renderers.
//
// The split is the point. Neither plugin imports the other; both depend
// only on core/plugin/specs/reporting, which is why an installation
// with no Reporting plugin loses nothing here (the provider registers
// and is simply never asked) and an installation with no Kubernetes
// plugin sees no Kubernetes shelf in the studio.

// Report type keys. Prefixed with the plugin slug because the Reporting
// plugin's library rows, schedules and stored documents all key on this
// string — an unprefixed "cluster-health" could collide with a built-in
// type and silently re-point every stored report of that type.
const (
	reportReliability    = "kubernetes-reliability"
	reportStorage        = "kubernetes-storage-estate"
	reportNetworkExposed = "kubernetes-network-exposure"
)

// Scope kinds this provider owns. Prefixed for the same reason the type
// keys are: the Reporting plugin passes an unknown kind straight
// through to whoever declared it, and "namespace" unqualified would be
// a land grab on a word every orchestrator uses.
const (
	subjectCluster   = "kubernetes-cluster"
	subjectNamespace = "kubernetes-namespace"
)

// reportCategory is the studio shelf these five sit on. A category of
// its own rather than folding into "platform": a cluster's questions
// are genuinely different from a Docker host's — there is a scheduler
// to satisfy, a control plane to keep quorate and a namespace boundary
// that owns quota — and mixing them would make both shelves harder to
// read.
const reportCategory = "kubernetes"

// The source keys the Methodology annex prints. One per API surface the
// report actually reads, so a reader can see that (say) the storage
// estate was read but the events were not.
const (
	srcClusterAPI = "kubernetes_cluster"
	srcNodes      = "kubernetes_nodes"
	srcWorkloads  = "kubernetes_workloads"
	srcNetwork    = "kubernetes_network"
	srcStorageAPI = "kubernetes_storage"
	srcEvents     = "kubernetes_events"
)

// kubernetesReportSpec declares the shelf.
//
// Every type is point-in-time. That is not a shortcut: the Kubernetes
// API server answers about the cluster as it is now, and this plugin
// keeps no history of its own, so a period picker on any of these would
// print a date range on a cover the figures underneath cannot honour.
// The one thing that does look backwards — the event stream — is bound
// by the cluster's own event TTL, which the reliability report states
// rather than implies.
func kubernetesReportSpec() plugin.ReportProviderSpec {
	// Both scopes on every type. A cluster is the obvious one; a
	// namespace is the unit a team actually owns, and reading a
	// hundred-namespace cluster's inventory to find one team's pods is
	// the same poor substitute a container-scoped report exists to fix.
	scopes := []string{subjectCluster, subjectNamespace}
	// The three switches the Reporting plugin already translates in its
	// own bundle. Declaring them keeps a contributed report's configure
	// dialog identical to a built-in one's.
	common := []plugin.ReportOptionDecl{
		{Key: "include_detail", Default: true},
		{Key: "include_glossary", Default: true},
	}

	return plugin.ReportProviderSpec{
		Slug:        "addon-kubernetes",
		DisplayName: "Kubernetes",
		Types: []plugin.ReportTypeDecl{
			{
				Key:                    reportReliability,
				Category:               reportCategory,
				Levels:                 []string{"strategic", "tactical", "operational"},
				Subjects:               scopes,
				EnvironmentScoped:      false,
				PointInTime:            true,
				Options:                common,
				TitleEN:                "Kubernetes Reliability Review",
				SubtitleEN:             "Restarting pods, unready workloads and what the cluster is warning about",
				LabelKey:               "reporting.type.reliability.name",
				DescriptionKey:         "reporting.type.reliability.description",
				CategoryLabelKey:       "reporting.category.label",
				CategoryDescriptionKey: "reporting.category.description",
			},
			{
				Key:                    reportStorage,
				Category:               reportCategory,
				Levels:                 []string{"tactical", "operational"},
				Subjects:               scopes,
				EnvironmentScoped:      false,
				PointInTime:            true,
				Options:                common,
				TitleEN:                "Kubernetes Storage Estate",
				SubtitleEN:             "Claims, volumes and storage classes, and which of them nothing is using",
				LabelKey:               "reporting.type.storage.name",
				DescriptionKey:         "reporting.type.storage.description",
				CategoryLabelKey:       "reporting.category.label",
				CategoryDescriptionKey: "reporting.category.description",
			},
			{
				Key:                    reportNetworkExposed,
				Category:               reportCategory,
				Levels:                 []string{"strategic", "tactical", "operational"},
				Subjects:               scopes,
				EnvironmentScoped:      false,
				PointInTime:            true,
				Options:                common,
				TitleEN:                "Kubernetes Network Exposure",
				SubtitleEN:             "What is reachable from outside the cluster, and which namespaces have no network policy",
				LabelKey:               "reporting.type.network.name",
				DescriptionKey:         "reporting.type.network.description",
				CategoryLabelKey:       "reporting.category.label",
				CategoryDescriptionKey: "reporting.category.description",
			},
		},
		Facets: kubernetesFacetDecls(),
	}
}

// reportProvider is the runtime half.
//
// It holds the plugin rather than the service so it reads whatever is
// current: a cluster added after boot, or a connection edited and its
// clientset dropped from the pool, is picked up on the next report
// without the provider caching anything of its own.
type reportProvider struct {
	plugin *KubernetesPlugin
	// reader supplies facet snapshots; nil means the live cluster reader.
	// Tests set it to feed a fixed snapshot.
	reader facetReader
}

var _ plugin.ReportProviderClient = (*reportProvider)(nil)

// reportScope is the resolved target of one report: which cluster, and
// which namespace within it.
type reportScope struct {
	clusterID   string
	clusterName string
	namespace   string
}

// scopeLabel is what the section prose calls the target.
func (s reportScope) scopeLabel() string {
	if s.namespace != "" {
		return "namespace " + s.namespace
	}
	return "the whole cluster"
}

// resolveScope turns the studio's subject selection into a cluster and
// a namespace.
//
// A namespace subject carries its cluster in the id (`<clusterID>/<ns>`)
// rather than relying on the default cluster, because the picker lists
// namespaces across every configured cluster and two clusters routinely
// have a namespace called `production`. Choosing one and silently
// reporting on the other's would be the worst kind of wrong: a document
// that looks right.
func (r *reportProvider) resolveScope(ctx context.Context, req plugin.ReportRequest) (reportScope, error) {
	var scope reportScope
	switch req.SubjectKind {
	case subjectCluster:
		scope.clusterID = strings.TrimSpace(req.SubjectID)
	case subjectNamespace:
		id, ns := splitNamespaceSubject(req.SubjectID)
		scope.clusterID, scope.namespace = id, ns
	}

	resolved, err := r.plugin.pool.ResolveClusterID(ctx, scope.clusterID)
	if err != nil {
		return scope, err
	}
	scope.clusterID = resolved
	scope.clusterName = r.clusterName(ctx, resolved)
	// A cluster's configured default namespace is a scope the operator
	// already chose once, when they added the connection. Honouring it
	// keeps a report consistent with what every page of this plugin
	// shows for the same cluster.
	if scope.namespace == "" {
		if cl, err := r.plugin.store.get(ctx, resolved); err == nil {
			scope.namespace = strings.TrimSpace(cl.Namespace)
		}
	}
	return scope, nil
}

// splitNamespaceSubject unpacks `<clusterID>/<namespace>`. A bare value
// with no separator is read as a namespace on the default cluster,
// which is what a subject saved before this encoding existed looks
// like.
func splitNamespaceSubject(raw string) (clusterID, namespace string) {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "/"); idx > 0 {
		return raw[:idx], raw[idx+1:]
	}
	return "", raw
}

func (r *reportProvider) clusterName(ctx context.Context, id string) string {
	cl, err := r.plugin.store.get(ctx, id)
	if err != nil {
		return id
	}
	return orFirst(cl.Name, id)
}

// Subjects lists what a report may be narrowed to.
//
// Both listings go through this plugin's own store and API client, so
// they are bounded by the clusters the operator configured — the
// Reporting plugin never learns a server URL, a namespace layout or a
// cluster it was not already showing in its own picker.
func (r *reportProvider) Subjects(ctx context.Context, kind string, req plugin.ReportRequest) ([]plugin.ReportSubjectOption, error) {
	if !r.mayView(req) {
		// An empty picker rather than an error: the studio renders
		// "nothing to narrow to" for it, and a user who may not read
		// this cluster has no business learning its namespace layout
		// from an error message either.
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	clusters, err := r.plugin.store.list(ctx)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: list clusters for report scope: %w", err)
	}

	switch kind {
	case subjectCluster:
		out := make([]plugin.ReportSubjectOption, 0, len(clusters))
		for _, c := range clusters {
			if !c.IsActive {
				continue
			}
			hint := c.Namespace
			if hint == "" {
				hint = "all namespaces"
			}
			if c.IsDefault {
				hint += " · default"
			}
			out = append(out, plugin.ReportSubjectOption{ID: c.ID, Name: c.Name, Hint: hint})
		}
		return out, nil

	case subjectNamespace:
		var out []plugin.ReportSubjectOption
		for _, c := range clusters {
			if !c.IsActive {
				continue
			}
			// One summary per cluster is the cheapest listing that is
			// still honest: the namespace list is the only part read,
			// and a cluster that will not answer contributes nothing
			// rather than failing the whole picker.
			summary, err := r.plugin.svc.Summary(ctx, c.ID, "")
			if err != nil || !summary.Connected {
				continue
			}
			for _, ns := range summary.Namespaces {
				out = append(out, plugin.ReportSubjectOption{
					ID:   c.ID + "/" + ns.Name,
					Name: ns.Name,
					Hint: c.Name,
				})
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Hint == out[j].Hint {
				return out[i].Name < out[j].Name
			}
			return out[i].Hint < out[j].Hint
		})
		return out, nil
	}
	return nil, nil
}

// Build answers one report.
//
// Everything comes from a single Summary call, which is the same call
// every read tab of this plugin makes: one round trip to the API server
// per report rather than one per section, and a report that can never
// disagree with the page the operator was just looking at.
func (r *reportProvider) Build(ctx context.Context, req plugin.ReportRequest) (plugin.ReportResult, error) {
	started := time.Now()

	// The Reporting plugin gated this request on `reporting.generate`.
	// That is its permission, not ours: it says the caller may produce
	// documents, not that they may read a Kubernetes cluster. Without
	// this check a report would be a way around `kubernetes.view` —
	// every pod name, image, ingress host and namespace in the estate,
	// handed to someone the interface refuses.
	if !r.mayView(req) {
		return plugin.ReportResult{}, errReportForbidden
	}

	scope, err := r.resolveScope(ctx, req)
	if err != nil {
		// No cluster to report on is a document, not a failure: an
		// operator who scheduled this report and then removed the last
		// cluster deserves a page that says so rather than a job that
		// errors every month with no explanation in the library.
		return unreachableResult(req, scope, "no Kubernetes cluster is configured", started), nil
	}

	summary, err := r.plugin.svc.Summary(ctx, scope.clusterID, scope.namespace)
	if err != nil || !summary.Connected {
		// The error itself is deliberately not carried into the
		// document: a client-go failure routinely names the API server
		// host, the service account and sometimes a token hint, none of
		// which has any business in a circulated PDF. The reason code is
		// enough for a reader to act on, and the detail is already in
		// the plugin's own log.
		if err != nil && r.plugin.host.Logger != nil {
			r.plugin.host.Logger.Error("kubernetes: report collection failed",
				"cluster", scope.clusterID, "type", req.TypeKey, "error", err)
		}
		return unreachableResult(req, scope, reasonText(summary.Reason), started), nil
	}

	sources := summarySources(summary, req.TypeKey, time.Since(started).Milliseconds())

	var sections []plugin.ReportSection
	switch req.TypeKey {
	case reportReliability:
		sections = buildReliability(summary, scope, detailWanted(req))
	case reportStorage:
		sections = buildStorageEstate(summary, scope, detailWanted(req))
	case reportNetworkExposed:
		sections = buildNetworkExposure(summary, scope, detailWanted(req))
	default:
		// Unreachable through the host — it dispatches on the keys this
		// provider registered — but a report type added to the spec and
		// not to this switch would otherwise ship an empty document.
		return plugin.ReportResult{}, fmt.Errorf("kubernetes: no builder for report type %q", req.TypeKey)
	}

	return plugin.ReportResult{
		Sections: sections,
		Sources:  sources,
		Notes:    buildNotes(summary, scope),
	}, nil
}

// errReportForbidden is returned when the requesting account lacks
// `kubernetes.view`. The Reporting plugin surfaces it as a failed
// generation; it deliberately carries no cluster detail, because an
// error message is not a place to tell someone what they were refused
// access to.
var errReportForbidden = errors.New("kubernetes: caller lacks kubernetes.view")

// mayView re-checks this plugin's own read permission for the account
// the report is being generated for.
//
// Scoped to the empty environment because Kubernetes clusters are this
// plugin's own records rather than host environments — there is no
// environment id to scope against, and passing the host's would ask the
// wrong question. A missing RBAC handle or a missing user id denies:
// a report that cannot establish who asked must not answer.
func (r *reportProvider) mayView(req plugin.ReportRequest) bool {
	return r.mayViewUser(req.UserID, req.TypeKey)
}

// mayViewUser is mayView for a bare user id, shared with the facet path so
// both gate on the same key the same way. what only labels the log line.
func (r *reportProvider) mayViewUser(userID, what string) bool {
	if r.plugin.host.RBAC == nil || strings.TrimSpace(userID) == "" {
		return false
	}
	allowed, err := r.plugin.host.RBAC.HasPermission(userID, "", "kubernetes.view")
	if err != nil {
		if r.plugin.host.Logger != nil {
			r.plugin.host.Logger.Error("kubernetes: report permission check failed",
				"type", what, "error", err)
		}
		return false
	}
	return allowed
}

// detailWanted reads the shared "include_detail" switch. Absent means
// on: a report generated by a caller that sent no options at all (a
// schedule saved before the option existed) should carry more rather
// than less.
func detailWanted(req plugin.ReportRequest) bool {
	if v, ok := req.Options["include_detail"]; ok {
		return v
	}
	return true
}

// reasonText turns the summary's machine-readable reason into the one
// line the document prints. Deliberately short and free of any
// connection detail.
func reasonText(reason SummaryReason) string {
	switch reason {
	case ReasonNoEnvironment:
		return "no Kubernetes cluster is configured"
	case ReasonNotKubernetes:
		return "the selected target is not a Kubernetes cluster"
	case ReasonUnreachable:
		return "the cluster's API server did not answer"
	case ReasonCredentialsRefused:
		return "the stored kubeconfig uses credentials that are not inline data"
	case ReasonAPIUnverified:
		return "the address answered, but not as a Kubernetes API server"
	}
	return "the cluster could not be read"
}

// unreachableResult is the document produced when there is nothing to
// read.
//
// It is a real report rather than an error for the same reason the
// Reporting plugin marks a missing source "not available" instead of
// scoring it zero: silence and health look identical in a table of
// zeros, and only one of them is worth acting on.
func unreachableResult(req plugin.ReportRequest, scope reportScope, why string, started time.Time) plugin.ReportResult {
	target := scope.clusterName
	if target == "" {
		target = "the selected cluster"
	}
	return plugin.ReportResult{
		Sections: []plugin.ReportSection{{
			Level: "summary",
			Title: "Executive summary",
			Blocks: []plugin.ReportBlock{
				{Kind: "paragraph", Lead: true, Text: "This report could not be produced: " + why + "."},
				{Kind: "callout", Tone: "warn", Title: "No data collected",
					Text: "Nothing below should be read as a measurement of " + target +
						". No figure in this document was derived from the cluster, and the " +
						"absence of findings is the absence of a reading, not a clean result."},
				{Kind: "list", Items: []string{
					"Check the cluster is listed and active under Kubernetes → Clusters.",
					"Confirm its kubeconfig or bearer token has not expired.",
					"Confirm the API server is reachable from this McHarbor installation.",
				}},
			},
		}},
		Sources: []plugin.ReportSourceStatus{{
			Key: srcClusterAPI, Label: "Kubernetes API server", OK: false,
			Note: why, DurationMS: time.Since(started).Milliseconds(),
		}},
	}
}

// summarySources reports what the one Summary call actually answered,
// broken out per API surface so the annex is specific about which part
// of the cluster went missing.
//
// Every list helper in service.go swallows its own error and returns an
// empty slice, so "empty" and "unreadable" are genuinely
// indistinguishable from here. That is stated rather than guessed at:
// each row carries the note, and no row claims a read succeeded when it
// only failed to complain.
func summarySources(summary Summary, typeKey string, elapsed int64) []plugin.ReportSourceStatus {
	out := []plugin.ReportSourceStatus{{
		Key: srcClusterAPI, Label: "Kubernetes API server", OK: true,
		Note:       "server " + orFirst(summary.Version, "version unknown"),
		DurationMS: elapsed,
	}}

	add := func(key, label string, n int, when bool) {
		if !when {
			return
		}
		note := ""
		if n == 0 {
			note = "nothing returned; an empty result and an unreadable one are not distinguished by the Kubernetes list API"
		}
		out = append(out, plugin.ReportSourceStatus{Key: key, Label: label, OK: n > 0, Note: note})
	}

	add(srcWorkloads, "Workloads", len(summary.Pods)+len(summary.Deployments)+
		len(summary.StatefulSets)+len(summary.DaemonSets), typeKey == reportReliability)
	add(srcNetwork, "Services and ingresses", len(summary.Services)+len(summary.Ingresses),
		typeKey == reportNetworkExposed)
	add(srcStorageAPI, "Persistent storage", len(summary.PVCs)+len(summary.PVs)+len(summary.StorageClasses),
		typeKey == reportStorage)
	add(srcEvents, "Cluster events", len(summary.Events), typeKey == reportReliability)
	return out
}

// buildNotes are the caveats that qualify the whole document rather
// than any one section.
func buildNotes(summary Summary, scope reportScope) []string {
	notes := []string{
		"Collected from the Kubernetes API server for cluster " +
			orFirst(scope.clusterName, scope.clusterID) + ", at the moment shown on the cover.",
	}
	if scope.namespace != "" {
		notes = append(notes, "Narrowed to namespace "+scope.namespace+
			". Cluster-scoped objects — nodes, persistent volumes and storage classes — "+
			"are still reported cluster-wide, because they have no namespace to narrow to.")
	}
	notes = append(notes,
		"Events are bound by the cluster's own event TTL (one hour by default). "+
			"An empty event list means nothing was recorded inside that window, not that "+
			"nothing has ever gone wrong.")
	if len(summary.Nodes) == 0 {
		notes = append(notes, "Node detail was not returned. The service account this "+
			"connection uses may not be permitted to list nodes.")
	}
	return notes
}

// orFirst returns the first non-empty value. A local helper rather than
// a shared one: this file is compiled into a Go plugin, and reaching
// for the host's string utilities would widen its reach set for one
// three-line function.
func orFirst(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
