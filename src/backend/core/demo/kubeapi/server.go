// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxBody = 1 << 20

type resource struct {
	group, version, name, kind string
	namespaced                 bool
	verbs                      []string
	short                      []string
	list                       func(s *Sim, c *cluster, now time.Time) []M
}

func (r resource) groupVersion() string {
	if r.group == "" {
		return r.version
	}
	return r.group + "/" + r.version
}

var (
	allVerbs = []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"}
)

func staticLister(name string) func(*Sim, *cluster, time.Time) []M {
	return func(_ *Sim, c *cluster, _ time.Time) []M {
		out := make([]M, 0, len(c.static[name]))
		for _, st := range c.static[name] {
			out = append(out, st.obj)
		}
		return out
	}
}

func workloadLister(kind string) func(*Sim, *cluster, time.Time) []M {
	return func(s *Sim, c *cluster, now time.Time) []M {
		out := []M{}
		for _, w := range c.workloads {
			switch {
			case kind == "ReplicaSet" && w.kind == "Deployment":
				out = append(out, s.renderReplicaSets(c, w, now)...)
			case w.kind != kind:
			case kind == "Deployment":
				out = append(out, s.renderDeployment(c, w, now))
			case kind == "StatefulSet":
				out = append(out, s.renderStatefulSet(c, w, now))
			case kind == "DaemonSet":
				out = append(out, s.renderDaemonSet(c, w, now))
			case kind == "Job":
				out = append(out, s.renderJob(c, w))
			}
		}
		return out
	}
}

var resources = []resource{
	{version: "v1", name: "namespaces", kind: "Namespace", verbs: allVerbs, short: []string{"ns"}, list: func(s *Sim, c *cluster, _ time.Time) []M {
		out := make([]M, 0, len(c.namespaces))
		for _, n := range c.namespaces {
			out = append(out, s.renderNamespace(n))
		}
		return out
	}},
	{version: "v1", name: "nodes", kind: "Node", verbs: allVerbs, short: []string{"no"}, list: func(s *Sim, c *cluster, now time.Time) []M {
		out := make([]M, 0, len(c.nodes))
		for _, n := range c.nodes {
			out = append(out, s.renderNode(c, n, now))
		}
		return out
	}},
	{version: "v1", name: "pods", kind: "Pod", namespaced: true, verbs: allVerbs, short: []string{"po"}, list: func(s *Sim, c *cluster, now time.Time) []M {
		pods := c.sortedPods()
		out := make([]M, 0, len(pods))
		for _, p := range pods {
			out = append(out, s.renderPod(c, p, now))
		}
		return out
	}},
	{version: "v1", name: "services", kind: "Service", namespaced: true, verbs: allVerbs, short: []string{"svc"}, list: func(s *Sim, c *cluster, _ time.Time) []M {
		out := make([]M, 0, len(c.services))
		for _, sv := range c.services {
			out = append(out, s.renderService(sv))
		}
		return out
	}},
	{version: "v1", name: "endpoints", kind: "Endpoints", namespaced: true, verbs: allVerbs, short: []string{"ep"}, list: func(s *Sim, c *cluster, now time.Time) []M {
		byNS := map[string][]*pod{}
		for _, p := range c.sortedPods() {
			byNS[p.ns] = append(byNS[p.ns], p)
		}
		out := make([]M, 0, len(c.services))
		for _, sv := range c.services {
			out = append(out, s.renderEndpoints(c, sv, byNS, now))
		}
		return out
	}},
	{version: "v1", name: "events", kind: "Event", namespaced: true, verbs: allVerbs, short: []string{"ev"}, list: func(s *Sim, c *cluster, now time.Time) []M {
		return s.sortedEvents(c, now)
	}},
	{version: "v1", name: "configmaps", kind: "ConfigMap", namespaced: true, verbs: allVerbs, short: []string{"cm"}, list: staticLister("configmaps")},
	{version: "v1", name: "secrets", kind: "Secret", namespaced: true, verbs: allVerbs, list: staticLister("secrets")},
	{version: "v1", name: "serviceaccounts", kind: "ServiceAccount", namespaced: true, verbs: allVerbs, short: []string{"sa"}, list: staticLister("serviceaccounts")},
	{version: "v1", name: "resourcequotas", kind: "ResourceQuota", namespaced: true, verbs: allVerbs, short: []string{"quota"}, list: func(s *Sim, c *cluster, now time.Time) []M {
		out := make([]M, 0, len(c.quotas))
		for _, q := range c.quotas {
			out = append(out, s.renderQuota(c, q, now))
		}
		return out
	}},
	{version: "v1", name: "limitranges", kind: "LimitRange", namespaced: true, verbs: allVerbs, short: []string{"limits"}, list: staticLister("limitranges")},
	{version: "v1", name: "persistentvolumeclaims", kind: "PersistentVolumeClaim", namespaced: true, verbs: allVerbs, short: []string{"pvc"}, list: staticLister("persistentvolumeclaims")},
	{version: "v1", name: "persistentvolumes", kind: "PersistentVolume", verbs: allVerbs, short: []string{"pv"}, list: staticLister("persistentvolumes")},
	{group: "apps", version: "v1", name: "deployments", kind: "Deployment", namespaced: true, verbs: allVerbs, short: []string{"deploy"}, list: workloadLister("Deployment")},
	{group: "apps", version: "v1", name: "replicasets", kind: "ReplicaSet", namespaced: true, verbs: allVerbs, short: []string{"rs"}, list: workloadLister("ReplicaSet")},
	{group: "apps", version: "v1", name: "statefulsets", kind: "StatefulSet", namespaced: true, verbs: allVerbs, short: []string{"sts"}, list: workloadLister("StatefulSet")},
	{group: "apps", version: "v1", name: "daemonsets", kind: "DaemonSet", namespaced: true, verbs: allVerbs, short: []string{"ds"}, list: workloadLister("DaemonSet")},
	{group: "batch", version: "v1", name: "jobs", kind: "Job", namespaced: true, verbs: allVerbs, list: workloadLister("Job")},
	{group: "batch", version: "v1", name: "cronjobs", kind: "CronJob", namespaced: true, verbs: allVerbs, short: []string{"cj"}, list: staticLister("cronjobs")},
	{group: "networking.k8s.io", version: "v1", name: "ingresses", kind: "Ingress", namespaced: true, verbs: allVerbs, short: []string{"ing"}, list: staticLister("ingresses")},
	{group: "networking.k8s.io", version: "v1", name: "networkpolicies", kind: "NetworkPolicy", namespaced: true, verbs: allVerbs, short: []string{"netpol"}, list: staticLister("networkpolicies")},
	{group: "storage.k8s.io", version: "v1", name: "storageclasses", kind: "StorageClass", verbs: allVerbs, short: []string{"sc"}, list: staticLister("storageclasses")},
	{group: "autoscaling", version: "v2", name: "horizontalpodautoscalers", kind: "HorizontalPodAutoscaler", namespaced: true, verbs: allVerbs, short: []string{"hpa"}, list: func(s *Sim, c *cluster, now time.Time) []M {
		out := make([]M, 0, len(c.hpas))
		for _, h := range c.hpas {
			out = append(out, s.renderHPA(c, h, now))
		}
		return out
	}},
}

func findResource(gv, name string) (resource, bool) {
	for _, r := range resources {
		if r.groupVersion() == gv && r.name == name {
			return r, true
		}
	}
	return resource{}, false
}

type handler struct {
	sim    *Sim
	tokens map[string]string
}

// Handler serves every cluster; tokens maps bearer token -> cluster key, and an
// unknown or missing token gets a 401 Status. Routing by token lets one loopback
// listener stand in for all three apiservers.
func (s *Sim) Handler(tokens map[string]string) http.Handler {
	copied := make(map[string]string, len(tokens))
	for k, v := range tokens {
		copied[k] = v
	}
	return &handler{sim: s, tokens: copied}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	key, ok := h.tokens[token]
	if !strings.HasPrefix(auth, "Bearer ") || token == "" || !ok {
		writeStatus(w, &apiError{code: http.StatusUnauthorized, reason: "Unauthorized", message: "Unauthorized"}, nil)
		return
	}
	c := h.sim.byKey[key]
	if c == nil {
		writeStatus(w, &apiError{code: http.StatusUnauthorized, reason: "Unauthorized", message: "Unauthorized"}, nil)
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	switch path {
	case "version":
		writeJSON(w, http.StatusOK, versionInfo(c))
		return
	case "healthz", "livez", "readyz":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "ok"); err != nil {
			return
		}
		return
	case "api":
		writeJSON(w, http.StatusOK, M{"kind": "APIVersions", "versions": []string{"v1"},
			"serverAddressByClientCIDRs": []M{{"clientCIDR": "0.0.0.0/0", "serverAddress": r.Host}}})
		return
	case "apis":
		writeJSON(w, http.StatusOK, groupList())
		return
	}
	segs := strings.Split(path, "/")
	var gv string
	var rest []string
	switch {
	case len(segs) >= 2 && segs[0] == "api" && segs[1] == "v1":
		gv, rest = "v1", segs[2:]
	case len(segs) >= 3 && segs[0] == "apis":
		gv, rest = segs[1]+"/"+segs[2], segs[3:]
	case len(segs) == 2 && segs[0] == "apis":
		if g, ok := groupByName(segs[1]); ok {
			writeJSON(w, http.StatusOK, g)
			return
		}
		writeStatus(w, notFoundPath(), nil)
		return
	default:
		writeStatus(w, notFoundPath(), nil)
		return
	}
	if len(rest) == 0 {
		if list, ok := resourceList(gv); ok {
			writeJSON(w, http.StatusOK, list)
			return
		}
		writeStatus(w, notFoundPath(), nil)
		return
	}
	ns := ""
	if rest[0] == "namespaces" && len(rest) >= 3 {
		ns, rest = rest[1], rest[2:]
	}
	res, ok := findResource(gv, rest[0])
	if !ok || (ns != "" && !res.namespaced) {
		writeStatus(w, notFoundPath(), nil)
		return
	}
	name, sub := "", ""
	if len(rest) > 1 {
		name = rest[1]
	}
	if len(rest) > 2 {
		sub = strings.Join(rest[2:], "/")
	}
	if name != "" && res.namespaced && ns == "" {
		writeStatus(w, notFoundPath(), nil)
		return
	}
	h.dispatch(w, r, c, res, ns, name, sub)
}

func notFoundPath() *apiError {
	return &apiError{code: http.StatusNotFound, reason: "NotFound", message: "the server could not find the requested resource"}
}

func methodNotAllowed(what string) *apiError {
	return &apiError{code: http.StatusMethodNotAllowed, reason: "MethodNotAllowed",
		message: what + " is not supported by the McHarbor demo apiserver"}
}

func notFound(res resource, name string) (*apiError, M) {
	return &apiError{code: http.StatusNotFound, reason: "NotFound", message: fmt.Sprintf("%s %q not found", qualified(res), name)},
		M{"name": name, "group": res.group, "kind": res.name}
}

func qualified(res resource) string {
	if res.group == "" {
		return res.name
	}
	return res.name + "." + res.group
}

func (h *handler) dispatch(w http.ResponseWriter, r *http.Request, c *cluster, res resource, ns, name, sub string) {
	s := h.sim
	q := r.URL.Query()
	if w8 := q.Get("watch"); w8 == "true" || w8 == "1" {
		writeStatus(w, methodNotAllowed("watch"), nil)
		return
	}
	switch sub {
	case "", "log", "scale":
	default:
		writeStatus(w, methodNotAllowed(res.name+"/"+sub), nil)
		return
	}
	if (sub == "log" && res.name != "pods") || (sub == "scale" && res.name != "deployments") {
		writeStatus(w, methodNotAllowed(res.name+"/"+sub), nil)
		return
	}
	var body []byte
	if r.Body != nil && (r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodPost) {
		b, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			writeStatus(w, badRequest("reading request body failed"), nil)
			return
		}
		body = b
	}

	s.mu.Lock()
	now := s.clock()
	s.reconcile(c, now)
	status, obj, text, apiErr, details := h.handleLocked(r, c, res, ns, name, sub, body, now)
	s.mu.Unlock()

	switch {
	case apiErr != nil:
		writeStatus(w, apiErr, details)
	case sub == "log":
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, text); err != nil {
			return
		}
	default:
		writeJSON(w, status, obj)
	}
}

func (h *handler) handleLocked(r *http.Request, c *cluster, res resource, ns, name, sub string, body []byte, now time.Time) (int, M, string, *apiError, M) {
	s := h.sim
	q := r.URL.Query()
	if name == "" {
		if r.Method != http.MethodGet {
			return 0, nil, "", methodNotAllowed(r.Method + " on a collection"), nil
		}
		list, err := listObjects(res, s.filtered(c, res, ns, now), q)
		if err != nil {
			return 0, nil, "", err, nil
		}
		list["metadata"] = M{"resourceVersion": strconv.FormatInt(c.rv, 10)}
		return http.StatusOK, list, "", nil, nil
	}
	switch {
	case res.name == "pods" && sub == "log" && r.Method == http.MethodGet:
		p := c.pods[podKey(ns, name)]
		if p == nil {
			e, d := notFound(res, name)
			return 0, nil, "", e, d
		}
		opt := logOptions{container: q.Get("container"), previous: q.Get("previous") == "true", tail: -1,
			timestamps: q.Get("timestamps") == "true"}
		if v, err := strconv.Atoi(q.Get("tailLines")); err == nil && v >= 0 {
			opt.tail = v
		}
		if v, err := strconv.Atoi(q.Get("limitBytes")); err == nil && v > 0 {
			opt.limitBytes = v
		}
		text, e := s.podLogs(c, p, opt, now)
		return http.StatusOK, nil, text, e, nil
	case res.name == "deployments":
		return h.deployment(r, c, res, ns, name, sub, body, now)
	case res.name == "pods" && sub == "" && r.Method == http.MethodDelete:
		p := c.pods[podKey(ns, name)]
		if p == nil {
			e, d := notFound(res, name)
			return 0, nil, "", e, d
		}
		obj := s.renderPod(c, p, now)
		obj["metadata"].(M)["deletionTimestamp"] = ts(now)
		obj["metadata"].(M)["deletionGracePeriodSeconds"] = 30
		s.deletePod(c, p, now)
		return http.StatusOK, obj, "", nil, nil
	case res.name == "nodes" && sub == "" && r.Method == http.MethodPatch:
		n := c.nodeByName(name)
		if n == nil {
			e, d := notFound(res, name)
			return 0, nil, "", e, d
		}
		patch, e := decodePatch(r, body)
		if e != nil {
			return 0, nil, "", e, nil
		}
		s.applyNodePatch(c, n, patch, now)
		return http.StatusOK, s.renderNode(c, n, now), "", nil, nil
	case r.Method == http.MethodGet && sub == "":
		for _, obj := range s.filtered(c, res, ns, now) {
			if nameOf(obj) == name {
				return http.StatusOK, obj, "", nil, nil
			}
		}
		e, d := notFound(res, name)
		return 0, nil, "", e, d
	}
	return 0, nil, "", methodNotAllowed(r.Method + " on " + res.name), nil
}

func (h *handler) deployment(r *http.Request, c *cluster, res resource, ns, name, sub string, body []byte, now time.Time) (int, M, string, *apiError, M) {
	s := h.sim
	wl := c.wlIndex[wlKey("Deployment", ns, name)]
	if wl == nil {
		e, d := notFound(res, name)
		return 0, nil, "", e, d
	}
	switch {
	case sub == "scale" && r.Method == http.MethodGet:
		return http.StatusOK, s.renderScale(c, wl, now), "", nil, nil
	case sub == "scale" && r.Method == http.MethodPut:
		replicas, err := decodeScaleReplicas(r.Header.Get("Content-Type"), body)
		if err != nil {
			return 0, nil, "", badRequest("invalid Scale body: " + err.Error()), nil
		}
		if replicas < 0 {
			return 0, nil, "", &apiError{code: http.StatusUnprocessableEntity, reason: "Invalid",
				message: fmt.Sprintf("Scale.autoscaling %q is invalid: spec.replicas: Invalid value: %d: must be greater than or equal to 0", name, replicas)}, nil
		}
		s.setReplicas(c, wl, replicas, now)
		s.reconcile(c, now)
		return http.StatusOK, s.renderScale(c, wl, now), "", nil, nil
	case sub == "" && r.Method == http.MethodGet:
		return http.StatusOK, s.renderDeployment(c, wl, now), "", nil, nil
	case sub == "" && r.Method == http.MethodPatch:
		patch, e := decodePatch(r, body)
		if e != nil {
			return 0, nil, "", e, nil
		}
		s.applyDeploymentPatch(c, wl, patch, now)
		s.reconcile(c, now)
		return http.StatusOK, s.renderDeployment(c, wl, now), "", nil, nil
	case sub == "" && r.Method == http.MethodDelete:
		s.deleteWorkload(c, wl, now)
		return http.StatusOK, M{"kind": "Status", "apiVersion": "v1", "metadata": M{}, "status": "Success",
			"details": M{"name": name, "group": "apps", "kind": "deployments", "uid": wl.uid}}, "", nil, nil
	}
	return 0, nil, "", methodNotAllowed(r.Method + " on deployments/" + sub), nil
}

func decodePatch(r *http.Request, body []byte) (M, *apiError) {
	ct := r.Header.Get("Content-Type")
	if !strings.Contains(ct, "merge-patch+json") {
		return nil, &apiError{code: http.StatusUnsupportedMediaType, reason: "UnsupportedMediaType",
			message: fmt.Sprintf("the body of the request was in an unknown format - accepted media types include: application/merge-patch+json, application/strategic-merge-patch+json; got %q", ct)}
	}
	var patch M
	if err := json.Unmarshal(body, &patch); err != nil {
		return nil, badRequest("error decoding patch: " + err.Error())
	}
	return patch, nil
}

func (s *Sim) filtered(c *cluster, res resource, ns string, now time.Time) []M {
	all := res.list(s, c, now)
	if ns == "" {
		return all
	}
	out := make([]M, 0, len(all))
	for _, o := range all {
		if md, ok := o["metadata"].(M); ok && md["namespace"] == ns {
			out = append(out, o)
		}
	}
	return out
}

func nameOf(obj M) string {
	md, ok := obj["metadata"].(M)
	if !ok {
		return ""
	}
	n, _ := md["name"].(string)
	return n
}

func listObjects(res resource, items []M, q map[string][]string) (M, *apiError) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	labelSel, err := parseLabelSelector(get("labelSelector"))
	if err != nil {
		return nil, badRequest("unable to parse requirement: " + err.Error())
	}
	fieldSel, err := parseFieldSelector(get("fieldSelector"))
	if err != nil {
		return nil, badRequest(err.Error())
	}
	out := make([]M, 0, len(items))
	for _, o := range items {
		if labelSel.matches(labelsOf(o)) && fieldSel.matches(o) {
			out = append(out, o)
		}
	}
	if res.name != "events" {
		sort.SliceStable(out, func(i, j int) bool {
			ni, nj := namespaceOf(out[i]), namespaceOf(out[j])
			if ni != nj {
				return ni < nj
			}
			return nameOf(out[i]) < nameOf(out[j])
		})
	}
	if limit, err := strconv.Atoi(get("limit")); err == nil && limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return M{"kind": res.kind + "List", "apiVersion": res.groupVersion(), "items": out}, nil
}

func namespaceOf(obj M) string {
	md, ok := obj["metadata"].(M)
	if !ok {
		return ""
	}
	n, _ := md["namespace"].(string)
	return n
}

func labelsOf(obj M) map[string]string {
	md, ok := obj["metadata"].(M)
	if !ok {
		return nil
	}
	switch l := md["labels"].(type) {
	case map[string]string:
		return l
	case M:
		out := map[string]string{}
		for k, v := range l {
			out[k] = fmt.Sprint(v)
		}
		return out
	}
	return nil
}

func versionInfo(c *cluster) M {
	v := strings.TrimPrefix(c.version, "v")
	parts := strings.SplitN(v, ".", 3)
	return M{
		"major": parts[0], "minor": parts[1], "gitVersion": c.version, "gitCommit": hashHex(40, c.version),
		"gitTreeState": "clean", "buildDate": "2024-08-14T19:04:38Z", "goVersion": "go1.22.5",
		"compiler": "gc", "platform": "linux/" + c.arch,
	}
}

type apiGroup struct {
	name     string
	versions []string
}

var apiGroups = []apiGroup{
	{name: "apps", versions: []string{"v1"}},
	{name: "batch", versions: []string{"v1"}},
	{name: "networking.k8s.io", versions: []string{"v1"}},
	{name: "storage.k8s.io", versions: []string{"v1"}},
	{name: "autoscaling", versions: []string{"v2", "v1"}},
}

func groupDoc(g apiGroup) M {
	versions := make([]M, 0, len(g.versions))
	for _, v := range g.versions {
		versions = append(versions, M{"groupVersion": g.name + "/" + v, "version": v})
	}
	return M{"name": g.name, "versions": versions, "preferredVersion": versions[0]}
}

func groupList() M {
	groups := make([]M, 0, len(apiGroups))
	for _, g := range apiGroups {
		groups = append(groups, groupDoc(g))
	}
	return M{"kind": "APIGroupList", "apiVersion": "v1", "groups": groups}
}

func groupByName(name string) (M, bool) {
	for _, g := range apiGroups {
		if g.name == name {
			doc := groupDoc(g)
			doc["kind"], doc["apiVersion"] = "APIGroup", "v1"
			return doc, true
		}
	}
	return nil, false
}

func resourceList(gv string) (M, bool) {
	known := gv == "v1"
	for _, g := range apiGroups {
		for _, v := range g.versions {
			if g.name+"/"+v == gv {
				known = true
			}
		}
	}
	if !known {
		return nil, false
	}
	out := []M{}
	for _, r := range resources {
		if r.groupVersion() != gv {
			continue
		}
		entry := M{"name": r.name, "singularName": strings.ToLower(r.kind), "namespaced": r.namespaced, "kind": r.kind, "verbs": r.verbs}
		if len(r.short) > 0 {
			entry["shortNames"] = r.short
		}
		out = append(out, entry)
		switch r.name {
		case "pods":
			out = append(out, M{"name": "pods/log", "singularName": "", "namespaced": true, "kind": "Pod", "verbs": []string{"get"}})
		case "deployments":
			out = append(out, M{"name": "deployments/scale", "singularName": "", "namespaced": true, "group": "autoscaling", "version": "v1",
				"kind": "Scale", "verbs": []string{"get", "patch", "update"}})
		}
	}
	return M{"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": gv, "resources": out}, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"InternalError","code":500}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(raw); err != nil {
		return
	}
}

func writeStatus(w http.ResponseWriter, e *apiError, details M) {
	st := M{"kind": "Status", "apiVersion": "v1", "metadata": M{}, "status": "Failure",
		"message": e.message, "reason": e.reason, "code": e.code}
	if details != nil {
		st["details"] = details
	}
	writeJSON(w, e.code, st)
}
