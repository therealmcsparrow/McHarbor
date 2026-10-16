// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// PluginGateway is the live plugin routing table as the runbook engine sees it:
// whether a plugin is loaded, and a handler that serves only plugin routes.
// Dispatching into it rather than the host router matters — the plugin router is
// mounted behind every host route, so a host path that ever shadowed a plugin one
// would silently answer a step meant for the plugin that declared the route.
type PluginGateway interface {
	Loaded(slug string) bool
	// Providers returns the runbook-provider capability of every loaded plugin, in a
	// stable order; a plugin that is not loaded has none, so its steps are not offered.
	Providers() []plugincaps.Capability
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// RouterGateway adapts the host's live plugin router and capability registry to
// PluginGateway.
type RouterGateway struct {
	Routes       *router.PluginRouter
	Capabilities *plugincaps.Registry
}

// Providers answers from the capability registry, which holds only loaded plugins.
func (g RouterGateway) Providers() []plugincaps.Capability {
	if g.Capabilities == nil {
		return nil
	}
	return g.Capabilities.OfKind(plugincaps.KindRunbookProvider)
}

// Loaded reports whether slug currently serves at least one route, which is
// true from load to disable or uninstall and false otherwise.
func (g RouterGateway) Loaded(slug string) bool {
	if g.Routes == nil {
		return false
	}
	return g.Routes.Routes()[slug] > 0
}

// ServeHTTP hands the request to whichever plugin router is current.
func (g RouterGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.Routes == nil {
		http.NotFound(w, r)
		return
	}
	g.Routes.ServeHTTP(w, r)
}

// maxPluginResponse bounds what a step reads back from a plugin before
// redaction trims it to MaxOutputBytes.
const maxPluginResponse = 1 << 20

// ErrPluginNotLoaded is returned when a step names a plugin that is not
// installed or not loaded; the message names the plugin.
var ErrPluginNotLoaded = errors.New("runbooks: plugin not loaded")

// pluginResponse is the envelope every plugin route answers with.
type pluginResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
}

// captured is a bounded in-memory response, so a chatty plugin cannot make a
// step hold megabytes it will throw away.
type captured struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *captured) Header() http.Header { return c.header }

func (c *captured) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *captured) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	room := maxPluginResponse - c.body.Len()
	if room > 0 {
		if len(p) > room {
			c.body.Write(p[:room])
		} else {
			c.body.Write(p)
		}
	}
	return len(p), nil
}

// dispatchPlugin invokes the entry's plugin route in-process as op. The request
// carries op as its authenticated user — the operator in a manual run, the
// service principal in an unattended one — so the plugin's own permission
// middleware evaluates that principal's roles, exactly as for a browser call.
func (s *Service) dispatchPlugin(ctx context.Context, op Operator, envID string, entry catalogEntry, step Step) (string, error) {
	call := entry.Call
	entry.Plugin = s.providerFor(entry.Domain)
	if entry.Plugin == "" {
		return "", fmt.Errorf("%w: no installed plugin serves %s steps, so %s cannot run",
			ErrPluginNotLoaded, entry.Domain, entry.Key)
	}
	params := step.ActionParams
	if target := entry.targetParam(); target != "" && strings.TrimSpace(params[target]) == "" {
		return "", fmt.Errorf("the step has no %s to act on: pin one in the editor or start the run from one", target)
	}
	for _, spec := range entry.ParamSpecs {
		if spec.Required && strings.TrimSpace(params[spec.Name]) == "" {
			return "", fmt.Errorf("the step's %s parameter is empty", spec.Name)
		}
	}
	path, err := expandPath(call.Path, params, envID)
	if err != nil {
		return "", err
	}
	body, err := callBody(call.Body, params)
	if err != nil {
		return "", err
	}
	res, err := s.servePlugin(ctx, op, call.Method, path, callQuery(call.Query, params, envID), body)
	if err != nil {
		return "", err
	}
	return readPluginResponse(entry, res, params, step.Kind == StepAction)
}

// servePlugin builds the request the way the /api mount would hand it to the
// plugin router — absolute URL, route path relative to /api — so the plugin's
// middleware and handlers see what they would see from a browser.
func (s *Service) servePlugin(ctx context.Context, op Operator, method, path string, query url.Values, body []byte) (*captured, error) {
	if user := auth.UserFromContext(ctx); user == nil || user.ID != op.UserID {
		ctx = context.WithValue(ctx, auth.UserContextKey, &auth.User{ID: op.UserID, Username: op.Username})
	}
	rctx := chi.NewRouteContext()
	rctx.RoutePath = strings.TrimPrefix(path, "/api")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	target := path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building the plugin request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	res := &captured{header: http.Header{}}
	s.Plugins.ServeHTTP(res, req)
	if res.status == 0 {
		res.status = http.StatusOK
	}
	return res, nil
}

// expandPath fills a route's placeholders. Every value is path-escaped, so a
// parameter can never add a segment and reach a route the entry does not name.
func expandPath(pattern string, params map[string]string, envID string) (string, error) {
	var out strings.Builder
	rest := pattern
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			out.WriteString(rest)
			return out.String(), nil
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return "", fmt.Errorf("malformed route %q", pattern)
		}
		out.WriteString(rest[:open])
		name := rest[open+1 : open+end]
		value, err := placeholder(name, params, envID)
		if err != nil {
			return "", err
		}
		out.WriteString(url.PathEscape(value))
		rest = rest[open+end+1:]
	}
}

func placeholder(name string, params map[string]string, envID string) (string, error) {
	if name == "$env" {
		if envID == "" {
			return "", errors.New("the step needs an environment, and this run has none")
		}
		return envID, nil
	}
	base, part, split := strings.Cut(name, ".")
	value := strings.TrimSpace(params[base])
	if !split {
		return value, nil
	}
	namespace, workload, ok := strings.Cut(value, "/")
	if !ok || namespace == "" || workload == "" {
		return "", fmt.Errorf("the %s parameter must be namespace/name, got %q", base, value)
	}
	if part == "namespace" {
		return namespace, nil
	}
	return workload, nil
}

func callQuery(spec map[string]string, params map[string]string, envID string) url.Values {
	query := url.Values{}
	for key, source := range spec {
		value := ""
		switch {
		case source == "$env":
			value = envID
		case strings.HasPrefix(source, "="):
			value = strings.TrimPrefix(source, "=")
		default:
			value = strings.TrimSpace(params[source])
		}
		if value != "" {
			query.Set(key, value)
		}
	}
	return query
}

func callBody(fields []bodyField, params map[string]string) ([]byte, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	body := map[string]any{}
	for _, f := range fields {
		raw := strings.TrimSpace(params[f.Param])
		if raw == "" {
			continue
		}
		switch f.Type {
		case ParamInt:
			n, err := strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("the %s parameter is not a whole number", f.Param)
			}
			body[f.JSON] = n
		case ParamBool:
			body[f.JSON] = raw == "true"
		default:
			body[f.JSON] = raw
		}
	}
	return json.Marshal(body)
}

// readPluginResponse turns the plugin's answer into what the run records: the
// output of a diagnostic, a confirmation for an action, or an error naming the
// plugin and what it said.
func readPluginResponse(entry catalogEntry, res *captured, params map[string]string, action bool) (string, error) {
	raw := res.body.Bytes()
	if !strings.Contains(res.header.Get("Content-Type"), "json") {
		if res.status >= http.StatusBadRequest {
			return "", pluginRefusal(entry.Plugin, res.status, "", strings.TrimSpace(string(raw)))
		}
		return redact(string(raw)), nil
	}
	var env pluginResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("%s answered %d with a body that is not JSON", entry.Plugin, res.status)
	}
	if res.status >= http.StatusBadRequest || !env.Success {
		return "", pluginRefusal(entry.Plugin, res.status, env.Code, firstNonEmpty(env.Error, env.Message))
	}
	call := entry.Call
	switch {
	case call.PickID != "":
		return pickItem(entry, env.Data, params[call.PickID])
	case call.Text != "":
		var fields map[string]any
		if err := json.Unmarshal(env.Data, &fields); err == nil {
			if text, ok := fields[call.Text].(string); ok {
				return redact(text), nil
			}
		}
	}
	if action {
		return fmt.Sprintf("%s: %s", entry.Key, firstNonEmpty(env.Message, env.Code, "done")), nil
	}
	return redact(pretty(env.Data)), nil
}

func pickItem(entry catalogEntry, data json.RawMessage, id string) (string, error) {
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return "", fmt.Errorf("%s answered with something that is not a list", entry.Plugin)
	}
	for _, item := range items {
		if fmt.Sprint(item["id"]) == id {
			raw, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				return "", err
			}
			return redact(string(raw)), nil
		}
	}
	return "", fmt.Errorf("%s has no %s with id %q in this environment", entry.Plugin, entry.targetParam(), id)
}

// pluginRefusal names the plugin, the status and the plugin's own code. A 403
// here means the plugin's gate refused after the runbook's own check passed — a
// policy bound to the object, or a key granted in another environment.
func pluginRefusal(plugin string, status int, code, message string) error {
	detail := firstNonEmpty(code, message)
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s did not accept the request's identity (401)", plugin)
	case http.StatusForbidden:
		if detail == "" || detail == "forbidden" {
			return fmt.Errorf("%s refused the request (403): its own permission check denied it", plugin)
		}
		return fmt.Errorf("%s refused the request (403): %s", plugin, detail)
	case http.StatusNotFound:
		if detail == "" || strings.Contains(detail, "404 page not found") {
			return fmt.Errorf("%s does not serve this route (404): the plugin may be an older version", plugin)
		}
		return fmt.Errorf("%s found no such object (404): %s", plugin, detail)
	}
	if detail == "" {
		return fmt.Errorf("%s answered %d", plugin, status)
	}
	return fmt.Errorf("%s answered %d: %s", plugin, status, detail)
}

func pretty(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
