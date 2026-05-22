// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

func isBlockedPort(node *CanvasNode, portKey string) bool {
	if node == nil {
		return false
	}

	for _, blockedPort := range node.BlockedPorts {
		if blockedPort == portKey {
			return true
		}
	}

	return false
}

func canSendToEdge(sourceNode *CanvasNode, edge *CanvasEdge) bool {
	if sourceNode == nil || edge == nil {
		return false
	}

	if isBlockedPort(sourceNode, "out:"+edge.SourcePort) {
		return false
	}

	return true
}

func canDeliverFromEdge(targetNode *CanvasNode, edge *CanvasEdge) bool {
	if targetNode == nil || edge == nil {
		return false
	}

	return !isBlockedPort(targetNode, "in:"+edge.TargetPort)
}
