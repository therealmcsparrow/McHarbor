// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"encoding/json"
	"strings"

	"github.com/rs/xid"
)

// Msg is the standard message object passed between workflow nodes (Node-RED pattern).
type Msg = map[string]interface{}

// NewMsg creates a new msg with a unique _msgid and the given payload.
func NewMsg(payload interface{}) Msg {
	return Msg{
		"_msgid":  xid.New().String(),
		"payload": payload,
	}
}

// EnsureMsgID ensures the msg has a _msgid field. Returns the msg.
func EnsureMsgID(msg Msg) Msg {
	if msg == nil {
		return NewMsg(nil)
	}
	if _, ok := msg["_msgid"]; !ok {
		msg["_msgid"] = xid.New().String()
	}
	return msg
}

// CloneMsg creates a shallow copy of msg (for branching).
func CloneMsg(msg Msg) Msg {
	if msg == nil {
		return nil
	}
	out := make(Msg, len(msg))
	for k, v := range msg {
		out[k] = v
	}
	return out
}

// DeepCloneMsg creates a deep copy of msg using JSON round-tripping.
// If marshalling fails, it falls back to CloneMsg.
func DeepCloneMsg(msg Msg) Msg {
	if msg == nil {
		return nil
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return CloneMsg(msg)
	}

	var out Msg
	if err := json.Unmarshal(data, &out); err != nil {
		return CloneMsg(msg)
	}
	return out
}

// WrapInMsg wraps arbitrary data into a msg. If data is already a msg with _msgid,
// returns it. Otherwise creates NewMsg(data).
func WrapInMsg(data interface{}) Msg {
	if m, ok := data.(Msg); ok {
		if _, hasMsgID := m["_msgid"]; hasMsgID {
			return m
		}
	}
	return NewMsg(data)
}

// GetPath resolves a dot-notation path on a nested map (e.g., "payload.status").
// Returns the value and whether it exists.
func GetPath(obj map[string]interface{}, path string) (interface{}, bool) {
	if obj == nil || path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	current := interface{}(obj)

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

// SetPath sets a value at a dot-notation path, creating intermediate maps as needed.
func SetPath(obj map[string]interface{}, path string, value interface{}) {
	if obj == nil || path == "" {
		return
	}

	parts := strings.Split(path, ".")
	current := obj

	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			child := make(map[string]interface{})
			current[parts[i]] = child
			current = child
			continue
		}
		child, ok := next.(map[string]interface{})
		if !ok {
			child = make(map[string]interface{})
			current[parts[i]] = child
		}
		current = child
	}

	current[parts[len(parts)-1]] = value
}

// ParseWorkflowVariables parses a JSON string of workflow variables into a map.
func ParseWorkflowVariables(raw string) map[string]interface{} {
	if raw == "" || raw == "{}" {
		return make(map[string]interface{})
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return make(map[string]interface{})
	}
	return result
}

// DeletePath removes a value at a dot-notation path.
func DeletePath(obj map[string]interface{}, path string) {
	if obj == nil || path == "" {
		return
	}

	parts := strings.Split(path, ".")
	current := obj

	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			return
		}
		child, ok := next.(map[string]interface{})
		if !ok {
			return
		}
		current = child
	}

	delete(current, parts[len(parts)-1])
}
