// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HTTP layer.
//
// Handlers parse, validate, call the service, audit and respond — nothing
// else. No client-go calls, no SQL. Errors are logged server-side with their
// detail and returned to the client as a stable code, never as `err.Error()`:
// a Kubernetes error routinely carries an API server hostname, a namespace and
// sometimes a token hint, none of which has any business leaving the process.

// maxLogTailLines caps the ?tail= parameter. The viewer renders into a single
// scroll container, so anything beyond this is a download, not a read.
const maxLogTailLines int64 = 10000

type apiEnvelope struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
	// Detail is err's redacted text on an authenticated request; RequestID
	// ties the failure to the server log line that carries the raw error.
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// Error and message codes. Deliberately coarse — they tell the UI which
// message to show, not what went wrong internally. The plugin's own i18n
// bundle owns the sentences; these used to be `i18n.ErrK8s*` constants in the
// host's message catalog, which is one of the things this migration removes.
const (
	codeUnauthorized    = "err.kubernetes.unauthorized"
	codeBadRequest      = "err.kubernetes.invalid_request"
	codeInvalidReplicas = "err.kubernetes.invalid_replicas"
	codeNotFound        = "err.kubernetes.not_found"
	codeScaleFailed     = "err.kubernetes.scale_failed"
	codeRestartFailed   = "err.kubernetes.restart_failed"
	codeDeleteFailed    = "err.kubernetes.delete_failed"
	codeLogsFailed      = "err.kubernetes.logs_failed"
	codeClusterInvalid  = "err.kubernetes.cluster_invalid"
	codeClusterNotFound = "err.kubernetes.cluster_not_found"
	codeClusterSaveFail = "err.kubernetes.cluster_save_failed"
	msgScaled           = "msg.kubernetes.scaled"
	msgRestarted        = "msg.kubernetes.restarted"
	msgDeleted          = "msg.kubernetes.deleted"
	msgClusterSaved     = "msg.kubernetes.cluster_saved"
	msgClusterRemoved   = "msg.kubernetes.cluster_removed"
	maxRequestBodyBytes = 2 << 20 // 2 MiB — a kubeconfig, generously
)

func writeJSON(w http.ResponseWriter, status int, body apiEnvelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func ok(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Data: data})
}

func okMsg(w http.ResponseWriter, code string) {
	writeJSON(w, http.StatusOK, apiEnvelope{Success: true, Code: code})
}

func fail(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, apiEnvelope{Success: false, Code: code, Error: code})
}

// failCause is fail plus the request ID and err's redacted text, so the
// operator sees why the call failed and can quote the ID against the log.
func failCause(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	writeJSON(w, status, apiEnvelope{
		Success:   false,
		Code:      code,
		Error:     code,
		Detail:    response.DetailFor(r, err, code),
		RequestID: requestid.From(r.Context()),
	})
}

func (p *KubernetesPlugin) logErr(msg string, err error, args ...any) {
	if p.host.Logger == nil {
		return
	}
	p.host.Logger.Error(msg, append([]any{"error", err}, args...)...)
}

func (p *KubernetesPlugin) audit(event string, fields map[string]any) {
	if p.host.Audit == nil {
		return
	}
	_ = p.host.Audit.Emit(event, fields)
}

// authed answers whether the caller has a session. The host's permission
// middleware has already run by the time a handler is reached — this is the
// belt to that braces, and the place the 401 code is chosen.
func (p *KubernetesPlugin) authed(w http.ResponseWriter, r *http.Request) bool {
	if auth.RequireAuth(r) != nil {
		return true
	}
	fail(w, http.StatusUnauthorized, codeUnauthorized)
	return false
}

// clusterID reads the target cluster from the query string. Empty means "the
// default cluster", which the pool resolves.
func clusterID(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("cluster"))
}

func decodeBody(r *http.Request, dst any) error {
	defer func() { _, _ = io.Copy(io.Discard, r.Body) }()
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes)).Decode(dst)
}

// --- clusters -----------------------------------------------------------

func (p *KubernetesPlugin) handleListClusters(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	clusters, err := p.store.list(r.Context())
	if err != nil {
		p.logErr("kubernetes list clusters failed", err)
		failCause(w, r, http.StatusInternalServerError, codeClusterSaveFail, err)
		return
	}
	ok(w, clusters)
}

func (p *KubernetesPlugin) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	cluster, err := p.store.get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		p.clusterError(w, r, "kubernetes get cluster failed", err)
		return
	}
	ok(w, cluster)
}

func (p *KubernetesPlugin) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	var in ClusterInput
	if err := decodeBody(r, &in); err != nil {
		failCause(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	cluster, err := p.store.create(r.Context(), in)
	if err != nil {
		p.logErr("kubernetes create cluster failed", err)
		failCause(w, r, http.StatusBadRequest, codeClusterInvalid, err)
		return
	}
	p.audit("plugin.kubernetes.cluster.created", map[string]any{"id": cluster.ID, "name": cluster.Name})
	writeJSON(w, http.StatusCreated, apiEnvelope{Success: true, Data: cluster, Code: msgClusterSaved})
}

func (p *KubernetesPlugin) handleUpdateCluster(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	var in ClusterInput
	if err := decodeBody(r, &in); err != nil {
		failCause(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	cluster, err := p.store.update(r.Context(), id, in)
	if err != nil {
		p.clusterError(w, r, "kubernetes update cluster failed", err)
		return
	}
	// Connection details may have changed, and a cached clientset was built
	// from the old ones — it would keep talking to the previous API server
	// until the process restarted.
	p.pool.Remove(id)
	p.audit("plugin.kubernetes.cluster.updated", map[string]any{"id": cluster.ID, "name": cluster.Name})
	ok(w, cluster)
}

func (p *KubernetesPlugin) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := p.store.remove(r.Context(), id); err != nil {
		p.clusterError(w, r, "kubernetes delete cluster failed", err)
		return
	}
	p.pool.Remove(id)
	p.audit("plugin.kubernetes.cluster.deleted", map[string]any{"id": id})
	okMsg(w, msgClusterRemoved)
}

func (p *KubernetesPlugin) clusterError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if errors.Is(err, errClusterNotFound) {
		fail(w, http.StatusNotFound, codeClusterNotFound)
		return
	}
	if errors.Is(err, errKubeconfigUnsafe) {
		p.logErr(msg, err)
		failCause(w, r, http.StatusBadRequest, codeClusterInvalid, err)
		return
	}
	if errors.Is(err, errBadEnvironmentID) {
		fail(w, http.StatusBadRequest, codeClusterInvalid)
		return
	}
	p.logErr(msg, err)
	failCause(w, r, http.StatusInternalServerError, codeClusterSaveFail, err)
}

// --- cluster reads ------------------------------------------------------

// handleSummary returns the whole cluster inventory in one response.
//
// A failed summary is still a 200: the payload's `connected: false` plus a
// `reason` code is the answer the page renders, and turning that into an HTTP
// error would make "no cluster configured yet" look like a server fault.
func (p *KubernetesPlugin) handleSummary(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id := clusterID(r)
	summary, err := p.svc.Summary(r.Context(), id, r.URL.Query().Get("namespace"))
	if err != nil {
		p.logErr("kubernetes summary failed", err, "cluster", id)
	}
	ok(w, summary)
}

func (p *KubernetesPlugin) handlePod(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id := clusterID(r)
	detail, err := p.svc.Pod(r.Context(), id, chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		p.logErr("kubernetes pod detail failed", err, "cluster", id)
		failCause(w, r, http.StatusNotFound, codeNotFound, err)
		return
	}
	ok(w, detail)
}

func (p *KubernetesPlugin) handleDeployment(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id := clusterID(r)
	detail, err := p.svc.Deployment(r.Context(), id, chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		p.logErr("kubernetes deployment detail failed", err, "cluster", id)
		failCause(w, r, http.StatusNotFound, codeNotFound, err)
		return
	}
	ok(w, detail)
}

// handlePodLogs streams one pod container's logs as plain text.
// GET /api/kubernetes/pods/{namespace}/{name}/logs?container=&tail=&previous=
func (p *KubernetesPlugin) handlePodLogs(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id := clusterID(r)
	namespace := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	container := r.URL.Query().Get("container")

	// Default to the last 500 lines. An unbounded read on a chatty pod is a
	// multi-megabyte response the viewer cannot use.
	tail := int64(500)
	if raw := r.URL.Query().Get("tail"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 || parsed > maxLogTailLines {
			fail(w, http.StatusBadRequest, codeBadRequest)
			return
		}
		tail = parsed
	}
	previous := r.URL.Query().Get("previous") == "true"

	stream, err := p.svc.PodLogs(r.Context(), id, namespace, name, container, tail, previous)
	if err != nil {
		p.logErr("kubernetes pod logs failed", err, "cluster", id, "namespace", namespace, "name", name)
		failCause(w, r, http.StatusInternalServerError, codeLogsFailed, err)
		return
	}
	defer func() { _ = stream.Close() }()

	// Headers go out before the first copy so the client starts rendering
	// while the API server is still sending.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, stream); err != nil {
		// The response is already committed, so there is no status left to
		// change — a client that navigated away is the usual cause.
		if p.host.Logger != nil {
			p.host.Logger.Warn("kubernetes pod log stream ended early",
				"cluster", id, "namespace", namespace, "name", name, "error", err)
		}
	}
}

// --- cluster writes -----------------------------------------------------

func (p *KubernetesPlugin) handleScaleDeployment(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	var req ScaleRequest
	if err := decodeBody(r, &req); err != nil {
		failCause(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}
	// A nil pointer means the field was absent, which is not the same as a
	// deliberate scale to zero.
	if req.Replicas == nil || *req.Replicas < 0 || *req.Replicas > maxReplicas {
		fail(w, http.StatusBadRequest, codeInvalidReplicas)
		return
	}

	id, namespace, name := clusterID(r), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if err := p.svc.ScaleDeployment(r.Context(), id, namespace, name, *req.Replicas); err != nil {
		p.logErr("kubernetes scale deployment failed", err, "cluster", id, "namespace", namespace, "name", name)
		failCause(w, r, http.StatusInternalServerError, codeScaleFailed, err)
		return
	}
	p.auditWorkload("plugin.kubernetes.deployment.scaled", id, namespace, name, map[string]any{
		"replicas": *req.Replicas,
	})
	okMsg(w, msgScaled)
}

func (p *KubernetesPlugin) handleRestartDeployment(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id, namespace, name := clusterID(r), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if err := p.svc.RestartDeployment(r.Context(), id, namespace, name); err != nil {
		p.logErr("kubernetes restart deployment failed", err, "cluster", id, "namespace", namespace, "name", name)
		failCause(w, r, http.StatusInternalServerError, codeRestartFailed, err)
		return
	}
	p.auditWorkload("plugin.kubernetes.deployment.restarted", id, namespace, name, nil)
	okMsg(w, msgRestarted)
}

func (p *KubernetesPlugin) handleDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id, namespace, name := clusterID(r), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if err := p.svc.DeleteDeployment(r.Context(), id, namespace, name); err != nil {
		p.logErr("kubernetes delete deployment failed", err, "cluster", id, "namespace", namespace, "name", name)
		failCause(w, r, http.StatusInternalServerError, codeDeleteFailed, err)
		return
	}
	p.auditWorkload("plugin.kubernetes.deployment.deleted", id, namespace, name, nil)
	okMsg(w, msgDeleted)
}

func (p *KubernetesPlugin) handleDeletePod(w http.ResponseWriter, r *http.Request) {
	if !p.authed(w, r) {
		return
	}
	id, namespace, name := clusterID(r), chi.URLParam(r, "namespace"), chi.URLParam(r, "name")
	if err := p.svc.DeletePod(r.Context(), id, namespace, name); err != nil {
		p.logErr("kubernetes delete pod failed", err, "cluster", id, "namespace", namespace, "name", name)
		failCause(w, r, http.StatusInternalServerError, codeDeleteFailed, err)
		return
	}
	p.auditWorkload("plugin.kubernetes.pod.deleted", id, namespace, name, nil)
	okMsg(w, msgDeleted)
}

// auditWorkload records a mutating action. Namespace and name identify the
// object; cluster identifies where it lives.
func (p *KubernetesPlugin) auditWorkload(event, cluster, namespace, name string, extra map[string]any) {
	fields := map[string]any{"cluster": cluster, "namespace": namespace, "name": name}
	for k, v := range extra {
		fields[k] = v
	}
	p.audit(event, fields)
}
