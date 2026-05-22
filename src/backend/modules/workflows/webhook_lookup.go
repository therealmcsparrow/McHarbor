// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Service) FindActiveWebhookTrigger(ctx context.Context, requestPath, method string) (*Workflow, *CanvasNode, error) {
	normalizedPath := normalizeWebhookPath(requestPath)
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "POST"
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, description, status, canvas_data, variables, created_by, updated_by, last_run_at, created_at, updated_at FROM workflows WHERE status = 'active' LIMIT 1000",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("querying active workflows: %w", err)
	}
	defer rows.Close()

	var matchedWorkflow *Workflow
	var matchedNode *CanvasNode

	for rows.Next() {
		var wf Workflow
		if err := rows.Scan(&wf.ID, &wf.Name, &wf.Description, &wf.Status, &wf.CanvasData, &wf.Variables, &wf.CreatedBy, &wf.UpdatedBy, &wf.LastRunAt, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
			continue
		}

		var canvas CanvasData
		if err := jsonUnmarshalCanvas(wf.CanvasData, &canvas); err != nil {
			continue
		}

		blockedNodeIDs := make(map[string]bool)
		for _, g := range canvas.Groups {
			if !g.Blocked {
				continue
			}
			for _, nodeID := range g.NodeIDs {
				blockedNodeIDs[nodeID] = true
			}
		}

		for i := range canvas.Nodes {
			node := &canvas.Nodes[i]
			if node.Action != "webhook-trigger" || node.Disabled || blockedNodeIDs[node.ID] {
				continue
			}

			nodeMethod, _ := node.Config["method"].(string)
			if nodeMethod == "" {
				nodeMethod = "POST"
			}
			if strings.ToUpper(nodeMethod) != method {
				continue
			}

			nodePath, _ := node.Config["path"].(string)
			if normalizeWebhookPath(nodePath) != normalizedPath {
				continue
			}

			if matchedWorkflow != nil {
				return nil, nil, fmt.Errorf("multiple webhook triggers match %s %s", method, normalizedPath)
			}

			wfCopy := wf
			nodeCopy := *node
			matchedWorkflow = &wfCopy
			matchedNode = &nodeCopy
		}
	}

	return matchedWorkflow, matchedNode, nil
}

func normalizeWebhookPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return strings.TrimRight(trimmed, "/")
}

func jsonUnmarshalCanvas(raw string, canvas *CanvasData) error {
	return json.Unmarshal([]byte(raw), canvas)
}
