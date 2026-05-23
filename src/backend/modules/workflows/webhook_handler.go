// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// HandleIncomingWebhook executes an active webhook-triggered workflow without session auth.
func (h *Handler) HandleIncomingWebhook(w http.ResponseWriter, r *http.Request) {
	path := normalizeWebhookPath(strings.TrimPrefix(r.URL.Path, "/api/workflows/webhooks"))
	wf, node, err := h.service.FindActiveWebhookTrigger(r.Context(), path, r.Method)
	if err != nil {
		h.app.Logger.Error("workflow webhook: lookup failed", "error", err, "path", path)
		http.Error(w, "workflow webhook lookup failed", http.StatusInternalServerError)
		return
	}
	if wf == nil || node == nil {
		http.NotFound(w, r)
		return
	}

	secret, _ := node.Config["secret"].(string)
	if strings.TrimSpace(secret) != "" && !webhookSecretMatches(r, secret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	triggerMsg, err := buildWebhookTriggerMsg(r)
	if err != nil {
		http.Error(w, "invalid webhook payload", http.StatusBadRequest)
		return
	}

	emit := func(event string, data interface{}) {
		payload, marshalErr := json.Marshal(data)
		if marshalErr != nil {
			h.app.Logger.Error("workflow webhook: marshal event failed", "error", marshalErr, "workflowID", wf.ID, "event", event)
			return
		}
		h.hub.Publish(wf.ID, ExecutionEvent{Event: event, Data: payload})
	}

	result := h.service.ExecuteWorkflow(r.Context(), wf, workflowRunOptions{
		WorkflowID:  wf.ID,
		Trigger:     "webhook",
		StartNodeID: node.ID,
		StartMsg:    triggerMsg,
	}, emit)
	h.service.RecordRun(wf.ID, "webhook", result.Status, result.DurationMs, result.NodesExecuted, result.Error)

	writeWebhookResponse(w, result, h.app.Logger)
}

func buildWebhookTriggerMsg(r *http.Request) (Msg, error) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	payload := interface{}("")
	if len(bodyBytes) > 0 {
		payload = decodeWorkflowBody(r.Header.Get("Content-Type"), bodyBytes)
	}
	if len(bodyBytes) == 0 && len(r.URL.Query()) > 0 {
		payload = queryValuesToMap(r.URL.Query())
	}

	msg := NewMsg(payload)
	msg["topic"] = "webhook"
	msg["req"] = map[string]interface{}{
		"method":     r.Method,
		"path":       normalizeWebhookPath(strings.TrimPrefix(r.URL.Path, "/api/workflows/webhooks")),
		"query":      queryValuesToMap(r.URL.Query()),
		"headers":    flattenHeaders(r.Header),
		"remoteAddr": r.RemoteAddr,
	}
	msg["headers"] = flattenHeaders(r.Header)
	if len(bodyBytes) > 0 {
		msg["_bodyRaw"] = string(bodyBytes)
	}
	return msg, nil
}

func writeWebhookResponse(w http.ResponseWriter, result workflowRunResult, logger *slog.Logger) {
	resp := result.Response
	if resp == nil {
		status := http.StatusOK
		if result.Status == "failed" {
			status = http.StatusInternalServerError
		}
		body := responseBodyFromMsg("", "application/json", result.LastOutput)
		if len(body) == 0 {
			body = []byte(`{"success":true}`)
		}
		resp = &executionResponse{
			StatusCode: status,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       body,
		}
	}

	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	if resp.StatusCode <= 0 {
		resp.StatusCode = http.StatusOK
	}
	w.WriteHeader(resp.StatusCode)
	if len(resp.Body) > 0 {
		if _, err := w.Write(resp.Body); err != nil && logger != nil {
			logger.Warn("workflow webhook: writing response body failed", "error", err)
		}
	}
}

func webhookSecretMatches(r *http.Request, expected string) bool {
	candidates := []string{
		r.Header.Get("X-Webhook-Secret"),
		r.Header.Get("X-Workflow-Secret"),
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		candidates = append(candidates, strings.TrimSpace(auth[7:]))
	}

	for _, candidate := range candidates {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}

func queryValuesToMap(values url.Values) map[string]interface{} {
	out := make(map[string]interface{}, len(values))
	for key, items := range values {
		if len(items) == 1 {
			out[key] = items[0]
			continue
		}
		converted := make([]interface{}, len(items))
		for i, item := range items {
			converted[i] = item
		}
		out[key] = converted
	}
	return out
}
