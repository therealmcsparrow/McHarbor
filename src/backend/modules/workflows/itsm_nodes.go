// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// ITSM node keys. The three actions open, update and resolve a ticket; the
// trigger fires when an ITSM reports a ticket's status.
const (
	NodeITSMOpen    = "itsm-open"
	NodeITSMUpdate  = "itsm-update"
	NodeITSMResolve = "itsm-resolve"
	NodeITSMStatus  = "itsm-status-trigger"
)

// ITSMEmitter queues a ticket operation on an incident-capable channel.
type ITSMEmitter interface {
	Emit(ctx context.Context, channelID string, ev incident.Event) error
}

var itsmEmitter atomic.Pointer[ITSMEmitter]

// SetITSMEmitter installs the ITSM service the action nodes emit through.
func SetITSMEmitter(e ITSMEmitter) {
	if e == nil {
		itsmEmitter.Store(nil)
		return
	}
	itsmEmitter.Store(&e)
}

// isITSMAction reports whether a node key opens, updates or resolves tickets,
// which is what saving it into a workflow needs alerts.manage for.
func isITSMAction(action string) bool {
	return action == NodeITSMOpen || action == NodeITSMUpdate || action == NodeITSMResolve
}

// canvasHasITSMActions reports whether a canvas contains a ticket action node.
func canvasHasITSMActions(raw string) bool {
	if raw == "" {
		return false
	}
	var canvas CanvasData
	if err := json.Unmarshal([]byte(raw), &canvas); err != nil {
		return false
	}
	for _, n := range canvas.Nodes {
		if isITSMAction(n.Action) {
			return true
		}
	}
	return false
}

func (s *Service) executeITSM(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	p := itsmEmitter.Load()
	if p == nil {
		return "", nil, fmt.Errorf("ITSM is not available")
	}
	state := incident.StateFiring
	switch node.Action {
	case NodeITSMUpdate:
		state = incident.StateUpdate
	case NodeITSMResolve:
		state = incident.StateResolved
	}
	channelID, _ := node.Config["channel_id"].(string)
	rawKey, _ := node.Config["dedup_key"].(string)
	if strings.TrimSpace(rawKey) == "" {
		rawKey = "wf:{{_msgid}}"
	}
	severity, _ := node.Config["severity"].(string)
	if severity == "" {
		severity = string(incident.SeverityWarning)
	}
	rawTitle, _ := node.Config["title"].(string)
	rawMessage, _ := node.Config["message"].(string)
	title := strings.TrimSpace(renderMsgTemplate(rawTitle, msg))
	if title == "" {
		title = "Workflow " + node.Label
	}
	ev := incident.Event{
		DedupKey: strings.TrimSpace(renderMsgTemplate(rawKey, msg)), State: state, Severity: incident.Severity(severity),
		Title: title, Message: renderMsgTemplate(rawMessage, msg), Source: "workflow", OccurredAt: time.Now().UTC(),
	}
	out := EnsureMsgID(CloneMsg(msg))
	if strings.TrimSpace(channelID) == "" {
		out["payload"] = map[string]interface{}{"error": "a ticketing channel is required"}
		return "error", out, nil
	}
	if err := (*p).Emit(ctx, channelID, ev); err != nil {
		s.logger.Error("workflows: ITSM emit failed", "error", err, "channelID", channelID, "node", node.Action)
		out["payload"] = map[string]interface{}{"error": "ticket operation not queued", "channelId": channelID, "dedupKey": ev.DedupKey}
		return "error", out, nil
	}
	out["payload"] = map[string]interface{}{"queued": true, "channelId": channelID, "dedupKey": ev.DedupKey, "state": string(state)}
	return "output", out, nil
}

// ITSMStatusMatches reports whether an itsm-status-trigger node's config
// accepts a status report: an empty channel or state list accepts any.
func ITSMStatusMatches(cfg map[string]interface{}, channelID, state string) bool {
	if want, _ := cfg["channel_id"].(string); want != "" && want != channelID {
		return false
	}
	states := configStringSlice(cfg["states"])
	if len(states) == 0 {
		return true
	}
	for _, s := range states {
		if s == state {
			return true
		}
	}
	return false
}
