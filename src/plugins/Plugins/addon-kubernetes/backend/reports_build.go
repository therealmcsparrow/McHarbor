// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// The three Kubernetes report builders.
//
// Each returns the content sections for one report type; the Reporting
// plugin filters them to the altitudes the operator asked for and wraps
// them in the cover and the methodology annex. A builder never renders
// and never writes a file — same discipline as the built-in report
// types, and the reason one report drives the screen, the PDF and the
// Word file without three copies of the content.
//
// Content is English, like every built-in report: a stored document's
// wording must not change when it is forwarded to a reader in another
// locale. Only the studio's catalog labels are translated, from this
// plugin's own i18n bundle.

// --- 1. reliability ---------------------------------------------------

func buildReliability(s Summary, scope reportScope, detail bool) []plugin.ReportSection {
	st := clusterStats(s)
	warnings := warningEvents(s)
	var out []plugin.ReportSection

	out = append(out, rsec("strategic", "Reliability at a glance",
		rlead("What is failing, restarting or being warned about in "+scope.scopeLabel()+
			" on cluster "+orFirst(scope.clusterName, scope.clusterID)+", as at the time on the cover."),
		rkpis(
			kpi("Pods not running", fmtInt(len(s.Pods)-st.podsRunning),
				countStatus(len(s.Pods)-st.podsRunning, 2),
				"Pending, failed, unknown or otherwise not in the Running phase."),
			kpi("Pods not ready", fmtInt(len(s.Pods)-st.podsReady),
				countStatus(len(s.Pods)-st.podsReady, 2),
				"Running but failing readiness — accepting no traffic."),
			kpi("Total restarts", fmtInt(st.totalRestarts), countStatus(st.totalRestarts, 10),
				"Container restarts since each pod was created."),
			kpi("Pods restarting often", fmtInt(st.podsRestarting), countStatus(st.podsRestarting, 0),
				"Five or more restarts on a single pod."),
			kpi("Warning events", fmtInt(len(warnings)), countStatus(len(warnings), 5),
				"Inside the cluster's event retention window."),
			kpi("Degraded workloads", fmtInt(st.workloadsTotal-st.workloadsReady),
				countStatus(st.workloadsTotal-st.workloadsReady, 0),
				"Below the replica count they were asked for."),
		),
		donut("Pods by phase", podPhasePoints(s)),
		rfindings(reliabilityFindings(s, st, warnings)...),
	))

	out = append(out, rsec("tactical", "What is failing",
		rhead(2, "Pods with the most restarts"),
		restartTable(s),
		rhead(2, "Pods not running or not ready"),
		unhealthyPodTable(s),
		rhead(2, "Warnings by reason"),
		warningReasonChart(warnings),
		rcallout("warn", "Events are a window, not a history",
			"Kubernetes expires events on its own schedule — one hour by default. An empty "+
				"warning list means nothing was recorded inside that window; it is not evidence "+
				"that the cluster has been stable."),
	))

	if detail {
		out = append(out, rsec("operational", "Warning events", warningEventTable(warnings)))
	}
	return out
}

func reliabilityFindings(s Summary, st clusterSummaryStats, warnings []EventItem) []plugin.ReportFinding {
	var out []plugin.ReportFinding
	if n := len(s.Pods) - st.podsRunning; n > 0 {
		out = append(out, finding("high",
			plural(n, "pod is not running", "pods are not running"),
			"Pods outside the Running phase are scheduled work the cluster has not delivered — "+
				"pending on capacity, failing to pull an image, or crash-looping.",
			"Start with the pods listed under \"Pods not running or not ready\" and the warning "+
				"events naming the same objects."))
	}
	if st.podsRestarting > 0 {
		out = append(out, finding("high",
			plural(st.podsRestarting, "pod is restarting repeatedly", "pods are restarting repeatedly"),
			"Five or more restarts on one pod is a workload the cluster keeps replacing rather "+
				"than running. Each restart is a dropped connection for whoever was using it.",
			"Check the container's exit reason and its resource limits; an OOMKilled loop and a "+
				"failing liveness probe look identical from the restart count alone."))
	}
	if n := crashLoopCount(warnings); n > 0 {
		out = append(out, finding("critical",
			plural(n, "crash-loop warning is recorded", "crash-loop warnings are recorded"),
			"The cluster is explicitly reporting containers that fail immediately on start.",
			"Read the container's last logs before the next restart overwrites them."))
	}
	if n := failedScheduleCount(warnings); n > 0 {
		out = append(out, finding("high",
			plural(n, "scheduling failure is recorded", "scheduling failures are recorded"),
			"The scheduler could not place a pod on any node. The usual causes are insufficient "+
				"CPU or memory, an unsatisfiable node selector, or a taint with no matching toleration.",
			"Compare the pod's resource requests against the node capacity in the Cluster Health report."))
	}
	if len(warnings) == 0 && len(s.Events) == 0 {
		out = append(out, finding("info", "No events were returned",
			"The cluster returned no events at all. That is normal on a quiet cluster inside the "+
				"retention window, but it also happens when the service account may not list events.",
			"If this cluster is busy, confirm the connection's service account has permission to "+
				"list events before reading this section as a clean result."))
	}
	return out
}

// --- 2. storage estate ------------------------------------------------

func buildStorageEstate(s Summary, scope reportScope, detail bool) []plugin.ReportSection {
	bound, pending, otherPVC := pvcPhases(s)
	released, available, _ := pvPhases(s)
	var out []plugin.ReportSection

	out = append(out, rsec("tactical", "Persistent storage",
		rlead("Every persistent volume claim, the volumes behind them and the classes that "+
			"provision them, for "+scope.scopeLabel()+" on cluster "+
			orFirst(scope.clusterName, scope.clusterID)+". Nodes, volumes and storage classes "+
			"are cluster-scoped and are reported cluster-wide whatever the scope."),
		rkpis(
			kpi("Claims", fmtInt(len(s.PVCs)), "", ""),
			kpi("Bound", fmtInt(bound)+" / "+fmtInt(len(s.PVCs)), ratioStatus(bound, len(s.PVCs)),
				"A claim that is not bound is a workload waiting for storage."),
			kpi("Pending claims", fmtInt(pending), countStatus(pending, 0), ""),
			kpi("Volumes", fmtInt(len(s.PVs)), "", ""),
			kpi("Released volumes", fmtInt(released), countStatus(released, 0),
				"Storage still allocated after its claim was deleted."),
			kpi("Storage classes", fmtInt(len(s.StorageClasses)), "", ""),
		),
		donut("Claims by phase", []plugin.ReportChartPoint{
			point("Bound", float64(bound), "good"),
			point("Pending", float64(pending), "bad"),
			point("Other", float64(otherPVC), "warn"),
		}),
		rfindings(storageFindings(s, pending, released, available)...),
	))

	out = append(out, rsec("operational", "Claims and volumes",
		pvcTable(s, detail),
		rhead(2, "Persistent volumes"),
		pvTable(s, detail),
		rhead(2, "Storage classes"),
		storageClassTable(s),
	))
	return out
}

func storageFindings(s Summary, pending, released, available int) []plugin.ReportFinding {
	var out []plugin.ReportFinding
	if pending > 0 {
		out = append(out, finding("high",
			plural(pending, "claim is Pending", "claims are Pending"),
			"A pending claim has no volume behind it. Any pod that mounts it will not start.",
			"Check that a storage class matching the claim exists and that its provisioner is running."))
	}
	if released > 0 {
		out = append(out, finding("medium",
			plural(released, "volume is Released", "volumes are Released"),
			"A released volume's claim is gone but the storage is still allocated, and with a "+
				"Retain policy it stays allocated until someone acts.",
			"Confirm the data is no longer needed, then delete the volume or reclaim it deliberately."))
	}
	if available > 0 {
		out = append(out, finding("low",
			plural(available, "volume is Available and unclaimed", "volumes are Available and unclaimed"),
			"Provisioned capacity that nothing is using.",
			"Reclaim it if the provisioning was accidental; otherwise record why it is held."))
	}
	if len(s.StorageClasses) > 0 && !hasDefaultStorageClass(s) {
		out = append(out, finding("medium", "No default storage class",
			"A claim that names no storage class will stay Pending forever on a cluster with no "+
				"default — a failure mode that looks like a broken application rather than a "+
				"cluster configuration gap.",
			"Mark one storage class as default, or require every claim to name a class explicitly."))
	}
	if len(s.PVCs) == 0 && len(s.PVs) == 0 {
		out = append(out, finding("info", "No persistent storage in scope",
			"Nothing in this scope claims persistent storage. Workloads here keep no state that "+
				"survives a pod being replaced.",
			"If that is not intended, check that the workloads that should be stateful actually "+
				"declare a volume claim."))
	}
	return out
}

// --- 3. network exposure ----------------------------------------------

func buildNetworkExposure(s Summary, scope reportScope, detail bool) []plugin.ReportSection {
	exposed := exposedServices(s)
	uncovered := namespacesWithoutPolicy(s)
	var out []plugin.ReportSection

	out = append(out, rsec("strategic", "What is reachable",
		rlead("Every route into "+scope.scopeLabel()+" on cluster "+
			orFirst(scope.clusterName, scope.clusterID)+": the services published outside the "+
			"cluster, the ingresses in front of them, and the namespaces with no network policy "+
			"restricting traffic between pods."),
		rkpis(
			kpi("Services", fmtInt(len(s.Services)), "", ""),
			kpi("Published outside", fmtInt(len(exposed)), countStatus(len(exposed), 3),
				"LoadBalancer and NodePort services, which are reachable from outside the cluster."),
			kpi("Ingresses", fmtInt(len(s.Ingresses)), "", ""),
			kpi("Network policies", fmtInt(len(s.NetworkPolicies)), countStatus(0, 0), ""),
			kpi("Namespaces without policy", fmtInt(len(uncovered)),
				countStatus(len(uncovered), 0),
				"Traffic between pods in these namespaces is unrestricted."),
			kpi("Endpoints with no address", fmtInt(emptyEndpoints(s)),
				countStatus(emptyEndpoints(s), 1),
				"A service whose endpoints are empty resolves but answers nothing."),
		),
		rfindings(networkFindings(s, exposed, uncovered)...),
	))

	out = append(out, rsec("tactical", "Published services and ingresses",
		exposedServiceTable(exposed),
		rhead(2, "Ingresses"),
		ingressTable(s),
		rhead(2, "Network policy coverage"),
		policyCoverageTable(s, uncovered),
	))

	if detail {
		out = append(out, rsec("operational", "All services",
			serviceTable(s),
			rhead(2, "Endpoints"),
			endpointTable(s),
		))
	}
	return out
}

func networkFindings(s Summary, exposed []ServiceItem, uncovered []string) []plugin.ReportFinding {
	var out []plugin.ReportFinding
	if len(exposed) > 0 {
		out = append(out, finding("medium",
			plural(len(exposed), "service is published outside the cluster",
				"services are published outside the cluster"),
			"LoadBalancer and NodePort services bypass the cluster boundary. Each one is an "+
				"address someone outside can reach directly.",
			"Confirm each is intended to be public, and that anything that should sit behind an "+
				"ingress and its TLS termination does."))
	}
	if len(uncovered) > 0 && len(s.Namespaces) > 0 {
		out = append(out, finding("high",
			plural(len(uncovered), "namespace has no network policy",
				"namespaces have no network policy"),
			"Kubernetes allows all pod-to-pod traffic by default. A namespace with no policy "+
				"places no restriction on what may talk to what inside it.",
			"Apply a default-deny ingress policy per namespace and open only the paths each "+
				"application needs."))
	}
	if n := emptyEndpoints(s); n > 0 {
		out = append(out, finding("medium",
			plural(n, "service has no endpoints", "services have no endpoints"),
			"The service exists and resolves, but no pod is behind it. Callers get a connection "+
				"failure rather than a clear \"not found\".",
			"Check the service's selector against the labels its pods actually carry."))
	}
	if n := ingressesWithoutAddress(s); n > 0 {
		out = append(out, finding("low",
			plural(n, "ingress has no address", "ingresses have no address"),
			"An ingress with no address has not been picked up by a controller, so the host names "+
				"it declares do not route anywhere.",
			"Confirm an ingress controller is installed and that the ingress names its class."))
	}
	return out
}

// --- derived figures --------------------------------------------------

// clusterSummaryStats are the counts more than one report needs, derived
// once so two sections of the same document can never print different
// numbers for the same thing.
type clusterSummaryStats struct {
	nodesReady     int
	podsRunning    int
	podsReady      int
	podsRestarting int
	totalRestarts  int
	workloadsTotal int
	workloadsReady int
	warningEvents  int
}

// restartThreshold is where "has restarted" becomes "keeps restarting".
// Five is the point at which Kubernetes' own backoff has already gone
// exponential, so a pod past it is not recovering on its own.
const restartThreshold int32 = 5

func clusterStats(s Summary) clusterSummaryStats {
	var st clusterSummaryStats
	for _, n := range s.Nodes {
		if strings.EqualFold(n.Status, "Ready") {
			st.nodesReady++
		}
	}
	for _, p := range s.Pods {
		if strings.EqualFold(p.Status, "Running") {
			st.podsRunning++
		}
		if fullyReady(p.Ready) {
			st.podsReady++
		}
		st.totalRestarts += int(p.Restarts)
		if p.Restarts >= restartThreshold {
			st.podsRestarting++
		}
	}
	for _, d := range s.Deployments {
		st.workloadsTotal++
		if fullyReady(d.Ready) {
			st.workloadsReady++
		}
	}
	for _, w := range append(append([]WorkloadItem{}, s.StatefulSets...), s.DaemonSets...) {
		st.workloadsTotal++
		if fullyReady(w.Ready) {
			st.workloadsReady++
		}
	}
	st.warningEvents = len(warningEvents(s))
	return st
}

func warningEvents(s Summary) []EventItem {
	var out []EventItem
	for _, e := range s.Events {
		if strings.EqualFold(e.Type, "Warning") {
			out = append(out, e)
		}
	}
	return out
}

func crashLoopCount(events []EventItem) int {
	n := 0
	for _, e := range events {
		if strings.Contains(strings.ToLower(e.Reason), "crashloop") ||
			strings.Contains(strings.ToLower(e.Reason), "backoff") {
			n++
		}
	}
	return n
}

func failedScheduleCount(events []EventItem) int {
	n := 0
	for _, e := range events {
		if strings.EqualFold(e.Reason, "FailedScheduling") {
			n++
		}
	}
	return n
}

func podPhasePoints(s Summary) []plugin.ReportChartPoint {
	phases := map[string]int{}
	for _, p := range s.Pods {
		phases[strings.TrimSpace(p.Status)]++
	}
	status := func(phase string) string {
		switch strings.ToLower(phase) {
		case "running", "succeeded":
			return "good"
		case "pending":
			return "warn"
		case "failed", "unknown":
			return "bad"
		}
		return "muted"
	}
	keys := make([]string, 0, len(phases))
	for k := range phases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]plugin.ReportChartPoint, 0, len(keys))
	for _, k := range keys {
		out = append(out, point(orFirst(k, "Unknown"), float64(phases[k]), status(k)))
	}
	return out
}

func pvcPhases(s Summary) (bound, pending, other int) {
	for _, c := range s.PVCs {
		switch strings.ToLower(strings.TrimSpace(c.Status)) {
		case "bound":
			bound++
		case "pending":
			pending++
		default:
			other++
		}
	}
	return
}

func pvPhases(s Summary) (released, available, other int) {
	for _, v := range s.PVs {
		switch strings.ToLower(strings.TrimSpace(v.Status)) {
		case "released":
			released++
		case "available":
			available++
		default:
			other++
		}
	}
	return
}

func hasDefaultStorageClass(s Summary) bool {
	for _, c := range s.StorageClasses {
		if c.Default {
			return true
		}
	}
	return false
}

// exposedServices is every service reachable from outside the cluster.
// ClusterIP services are excluded deliberately: they are the default and
// listing them here would bury the handful that actually matter.
func exposedServices(s Summary) []ServiceItem {
	var out []ServiceItem
	for _, svc := range s.Services {
		switch strings.ToLower(strings.TrimSpace(svc.Type)) {
		case "loadbalancer", "nodeport":
			out = append(out, svc)
		}
	}
	return out
}

// namespacesWithoutPolicy lists namespaces holding pods but no network
// policy. A namespace with no workloads is excluded: an empty namespace
// with no policy is not an exposure, and counting it would inflate the
// figure the report leads with.
func namespacesWithoutPolicy(s Summary) []string {
	withPolicy := map[string]bool{}
	for _, p := range s.NetworkPolicies {
		withPolicy[p.Namespace] = true
	}
	withPods := map[string]bool{}
	for _, p := range s.Pods {
		withPods[p.Namespace] = true
	}
	var out []string
	for ns := range withPods {
		if !withPolicy[ns] {
			out = append(out, ns)
		}
	}
	sort.Strings(out)
	return out
}

func emptyEndpoints(s Summary) int {
	n := 0
	for _, e := range s.Endpoints {
		if strings.TrimSpace(e.Addresses) == "" || strings.TrimSpace(e.Addresses) == "-" {
			n++
		}
	}
	return n
}

func ingressesWithoutAddress(s Summary) int {
	n := 0
	for _, i := range s.Ingresses {
		if strings.TrimSpace(i.Address) == "" {
			n++
		}
	}
	return n
}
