// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "strings"

func workflowErrorPayload(message string, extra map[string]interface{}) map[string]interface{} {
	payload := make(map[string]interface{}, len(extra)+1)
	payload["error"] = message
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func workflowErrorMsg(msg Msg, message string, extra map[string]interface{}) Msg {
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = workflowErrorPayload(message, extra)
	return out
}

func sanitizeComposeResolutionError(err error) string {
	if err == nil {
		return "invalid compose source configuration"
	}

	switch strings.TrimSpace(err.Error()) {
	case "compose file not found":
		return "compose file not found"
	case "compose_content is required when compose_source is inline":
		return "compose content is required for inline compose sources"
	case "compose_path is required when compose_source is file":
		return "compose path is required for file compose sources"
	case "compose_content, compose_path, or msg.payload compose data is required":
		return "compose content, compose path, or message payload compose data is required"
	default:
		return "invalid compose source configuration"
	}
}
