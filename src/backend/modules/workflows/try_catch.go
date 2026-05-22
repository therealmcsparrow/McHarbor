// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"fmt"
	"strings"
)

const tryCatchFramesKey = "_tryCatchFrames"

type tryCatchFrame struct {
	NodeID        string `json:"nodeId"`
	ErrorProperty string `json:"errorProperty"`
}

func executeTryCatchNode(node *CanvasNode, msg Msg) (string, Msg, error) {
	out := DeepCloneMsg(msg)
	out = EnsureMsgID(out)
	errorProperty, _ := node.Config["error_property"].(string)
	pushTryCatchFrame(out, tryCatchFrame{
		NodeID:        node.ID,
		ErrorProperty: normalizeTryCatchErrorProperty(errorProperty),
	})
	return "output", out, nil
}

func catchTargetsForMessage(adjacency map[string][]edgeTarget, msg Msg, node *CanvasNode, execErr error) (tryCatchFrame, []edgeTarget, Msg, bool) {
	frames := readTryCatchFrames(msg)
	if len(frames) == 0 {
		return tryCatchFrame{}, nil, nil, false
	}

	caughtIndex := -1
	var caughtFrame tryCatchFrame
	var caughtTargets []edgeTarget
	for i := len(frames) - 1; i >= 0; i-- {
		targets := adjacency[frames[i].NodeID+":catch"]
		if len(targets) == 0 {
			continue
		}
		caughtIndex = i
		caughtFrame = frames[i]
		caughtTargets = targets
		break
	}
	if caughtIndex < 0 {
		return tryCatchFrame{}, nil, nil, false
	}

	out := DeepCloneMsg(msg)
	out = EnsureMsgID(out)
	writeTryCatchFrames(out, frames[:caughtIndex])

	errorInfo := map[string]interface{}{
		"caught":      true,
		"guardNodeId": caughtFrame.NodeID,
		"nodeId":      node.ID,
		"nodeLabel":   node.Label,
		"action":      node.Action,
	}
	if execErr != nil {
		errorInfo["message"] = execErr.Error()
	} else {
		errorInfo["message"] = fmt.Sprintf("%s routed to error output", node.Action)
	}
	SetPath(out, normalizeTryCatchErrorProperty(caughtFrame.ErrorProperty), errorInfo)
	out["_caught"] = errorInfo

	return caughtFrame, caughtTargets, out, true
}

func pushTryCatchFrame(msg Msg, frame tryCatchFrame) {
	if msg == nil || frame.NodeID == "" {
		return
	}
	frames := readTryCatchFrames(msg)
	frames = append(frames, frame)
	writeTryCatchFrames(msg, frames)
}

func readTryCatchFrames(msg Msg) []tryCatchFrame {
	if msg == nil {
		return nil
	}
	raw, ok := msg[tryCatchFramesKey]
	if !ok {
		return nil
	}

	switch value := raw.(type) {
	case []tryCatchFrame:
		out := make([]tryCatchFrame, len(value))
		copy(out, value)
		return out
	case []interface{}:
		frames := make([]tryCatchFrame, 0, len(value))
		for _, item := range value {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			nodeID, _ := m["nodeId"].(string)
			if nodeID == "" {
				continue
			}
			errorProperty, _ := m["errorProperty"].(string)
			frames = append(frames, tryCatchFrame{
				NodeID:        nodeID,
				ErrorProperty: normalizeTryCatchErrorProperty(errorProperty),
			})
		}
		return frames
	default:
		return nil
	}
}

func writeTryCatchFrames(msg Msg, frames []tryCatchFrame) {
	if msg == nil {
		return
	}
	if len(frames) == 0 {
		delete(msg, tryCatchFramesKey)
		return
	}
	raw := make([]interface{}, 0, len(frames))
	for _, frame := range frames {
		raw = append(raw, map[string]interface{}{
			"nodeId":        frame.NodeID,
			"errorProperty": normalizeTryCatchErrorProperty(frame.ErrorProperty),
		})
	}
	msg[tryCatchFramesKey] = raw
}

func normalizeTryCatchErrorProperty(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "error"
	}
	return trimmed
}
