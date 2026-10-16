// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// ErrTargetKindUnknown is returned for a selector kind no plugin list backs.
var ErrTargetKindUnknown = errors.New("runbooks: unknown target kind")

// TargetRefusedError reports that the plugin refused to list its objects to this
// operator, which the handler answers as a 403 rather than an empty list.
type TargetRefusedError struct {
	Plugin string
	Status int
}

// Error names the plugin and its status; it is logged, never sent to a client.
func (e *TargetRefusedError) Error() string {
	return fmt.Sprintf("runbooks: %s refused the target list (%d)", e.Plugin, e.Status)
}

// targetSource is the plugin list route behind one selector kind, and where in
// each item the id, the label and a detail line are read from. IDJoin builds a
// composite id — a pod is namespace/name, because that is how its routes are keyed.
type targetSource struct {
	Domain string
	Path   string
	Query  map[string]string
	Items  string
	ID     string
	IDJoin []string
	Name   []string
	Detail []string
}

// TargetKindViews reports which selector kinds are backed by a loaded plugin, each with
// its plugin-owned label, so the editor offers only what it can fill and titles it right.
func (s *Service) TargetKindViews() []TargetKindView {
	out := []TargetKindView{}
	for _, c := range s.providers() {
		for _, t := range c.Targets {
			out = append(out, TargetKindView{Kind: t.Kind, LabelKey: t.LabelKey, LabelNS: c.I18nNS, EmptyLabelKey: t.EmptyLabelKey})
		}
	}
	slices.SortFunc(out, func(a, b TargetKindView) int { return strings.Compare(a.Kind, b.Kind) })
	return out
}

// Targets lists the objects of one kind as op sees them. The list is read
// through the plugin's own route, so its view key is enforced by the plugin and
// the selector never offers an object the operator could not open.
func (s *Service) Targets(ctx context.Context, op Operator, kind, envID, cluster string) ([]TargetOption, error) {
	src, ok := s.targetSources()[kind]
	if !ok {
		return nil, ErrTargetKindUnknown
	}
	plugin := s.providerFor(src.Domain)
	runCtx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	query := callQuery(src.Query, map[string]string{"cluster": cluster}, envID)
	res, err := s.servePlugin(runCtx, op, http.MethodGet, src.Path, query, nil)
	if err != nil {
		return nil, err
	}
	if res.status >= http.StatusBadRequest {
		return nil, &TargetRefusedError{Plugin: plugin, Status: res.status}
	}
	var env pluginResponse
	if err := json.Unmarshal(res.body.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("%s answered its list with a body that is not JSON: %w", plugin, err)
	}
	return parseTargets(plugin, src, env.Data)
}

func parseTargets(plugin string, src targetSource, data json.RawMessage) ([]TargetOption, error) {
	var items []map[string]any
	if src.Items == "" {
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("%s answered its list with something that is not a list: %w", plugin, err)
		}
	} else {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("%s answered with something that is not an object: %w", plugin, err)
		}
		if raw, ok := wrapper[src.Items]; ok && len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, fmt.Errorf("%s answered %s with something that is not a list: %w", plugin, src.Items, err)
			}
		}
	}
	out := make([]TargetOption, 0, len(items))
	for _, item := range items {
		id := field(item, src.ID)
		if len(src.IDJoin) > 0 {
			parts := make([]string, 0, len(src.IDJoin))
			for _, key := range src.IDJoin {
				parts = append(parts, field(item, key))
			}
			id = strings.Join(parts, "/")
		}
		if strings.Trim(id, "/") == "" {
			continue
		}
		out = append(out, TargetOption{ID: id, Name: firstField(item, src.Name, id), Detail: joinFields(item, src.Detail)})
	}
	return out, nil
}

func field(item map[string]any, key string) string {
	if key == "" {
		return ""
	}
	value, ok := item[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstField(item map[string]any, keys []string, fallback string) string {
	for _, key := range keys {
		if v := field(item, key); v != "" {
			return v
		}
	}
	return fallback
}

func joinFields(item map[string]any, keys []string) string {
	parts := []string{}
	for _, key := range keys {
		if v := field(item, key); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " · ")
}
