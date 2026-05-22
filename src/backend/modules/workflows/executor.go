// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const maxWorkflowExecutionSteps = 2000

type workflowEmitter func(event string, data interface{})

type queueItem struct {
	node *CanvasNode
	msg  Msg
}

type edgeTarget struct {
	edge *CanvasEdge
	node *CanvasNode
}

func enqueueTargetsForEmission(
	emit workflowEmitter,
	queue []queueItem,
	sourceNodeID string,
	sourceLabel string,
	targets []edgeTarget,
	msg Msg,
) []queueItem {
	for _, target := range targets {
		safeEmit(emit, "edge.traversed", EdgeTraversedData{
			EdgeID:       target.edge.ID,
			SourceNodeID: sourceNodeID,
			TargetNodeID: target.node.ID,
		})

		if target.edge.Sniffer != nil {
			snifferName := target.edge.Sniffer.Name
			if snifferName == "" {
				snifferName = "Sniffer"
			}
			targetLabel := target.node.Label
			if targetLabel == "" {
				targetLabel = target.node.ID
			}
			safeEmit(emit, "debug", DebugData{
				NodeID:  sourceNodeID,
				Label:   snifferName,
				Level:   "info",
				Message: fmt.Sprintf("Sniffer [%s]: data flowing from %s to %s", snifferName, sourceLabel, targetLabel),
				Data:    msg,
				Source:  "sniffer",
			})
		}

		if canDeliverFromEdge(target.node, target.edge) {
			queue = append(queue, queueItem{node: target.node, msg: DeepCloneMsg(msg)})
		}
	}
	return queue
}

func (s *Service) ExecuteWorkflow(ctx context.Context, wf *Workflow, opts workflowRunOptions, emit workflowEmitter) workflowRunResult {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := workflowRunResult{Status: "completed"}
	runStart := time.Now()

	var canvas CanvasData
	if err := json.Unmarshal([]byte(wf.CanvasData), &canvas); err != nil {
		result.Status = "failed"
		result.Error = "invalid workflow canvas"
		result.DurationMs = time.Since(runStart).Milliseconds()
		safeEmit(emit, "run.completed", RunStatusData{Status: result.Status, DurationMs: result.DurationMs, Error: result.Error})
		return result
	}

	nodeMap := make(map[string]*CanvasNode, len(canvas.Nodes))
	for i := range canvas.Nodes {
		nodeMap[canvas.Nodes[i].ID] = &canvas.Nodes[i]
	}

	startNode := nodeMap[opts.StartNodeID]
	if opts.StartNodeID != "" && startNode == nil {
		result.Status = "failed"
		result.Error = "start node not found"
		result.DurationMs = time.Since(runStart).Milliseconds()
		safeEmit(emit, "run.completed", RunStatusData{Status: result.Status, DurationMs: result.DurationMs, Error: result.Error})
		return result
	}

	blockedGroupNodeIDs := make(map[string]bool)
	for _, g := range canvas.Groups {
		if !g.Blocked {
			continue
		}
		for _, nodeID := range g.NodeIDs {
			blockedGroupNodeIDs[nodeID] = true
		}
	}

	adjacency := make(map[string][]edgeTarget)
	for i := range canvas.Edges {
		e := &canvas.Edges[i]
		sourceNode := nodeMap[e.SourceNodeID]
		targetNode := nodeMap[e.TargetNodeID]
		if targetNode == nil || !canSendToEdge(sourceNode, e) {
			continue
		}
		key := e.SourceNodeID + ":" + e.SourcePort
		adjacency[key] = append(adjacency[key], edgeTarget{edge: e, node: targetNode})
	}

	flowCtx := &FlowContext{
		FlowVars:   ParseWorkflowVariables(wf.Variables),
		GlobalVars: map[string]interface{}{},
		Runtime:    newExecutionRuntime(),
	}
	flowCtx.FlowVars["_workflowId"] = opts.WorkflowID

	queue := make([]queueItem, 0, 4)
	if startNode != nil {
		queue = append(queue, queueItem{node: startNode, msg: opts.StartInputMsg})
	}
	if opts.AutoEnqueueLinkIns {
		for i := range canvas.Nodes {
			if canvas.Nodes[i].Action == "link-in" && (startNode == nil || canvas.Nodes[i].ID != startNode.ID) {
				queue = append(queue, queueItem{node: nodeMap[canvas.Nodes[i].ID], msg: nil})
			}
		}
	}

	if len(queue) == 0 {
		result.Status = "failed"
		result.Error = "no runnable entry nodes"
		result.DurationMs = time.Since(runStart).Milliseconds()
		safeEmit(emit, "run.completed", RunStatusData{Status: result.Status, DurationMs: result.DurationMs, Error: result.Error})
		return result
	}

	s.UpdateLastRunAt(opts.WorkflowID)
	safeEmit(emit, "run.started", RunStatusData{Status: "running"})

	startOverrideUsed := false
	hadNodeFailure := false

	for len(queue) > 0 {
		select {
		case <-execCtx.Done():
			result.Status = "cancelled"
			result.Error = execCtx.Err().Error()
			result.DurationMs = time.Since(runStart).Milliseconds()
			safeEmit(emit, "run.cancelled", RunStatusData{Status: "cancelled", DurationMs: result.DurationMs})
			return result
		default:
		}

		if result.NodesExecuted >= maxWorkflowExecutionSteps {
			result.Status = "failed"
			result.Error = fmt.Sprintf("workflow exceeded %d execution steps", maxWorkflowExecutionSteps)
			break
		}

		item := queue[0]
		queue = queue[1:]
		node := item.node
		if node == nil {
			continue
		}

		if node.Disabled || blockedGroupNodeIDs[node.ID] {
			safeEmit(emit, "node.completed", NodeCompletedData{
				NodeID:     node.ID,
				Label:      node.Label,
				Action:     node.Action,
				OutputPort: "",
				Input:      item.msg,
				Config:     node.Config,
				Output:     map[string]interface{}{"_disabled": true},
				DurationMs: 0,
			})
			continue
		}

		nodeStart := time.Now()
		safeEmit(emit, "node.started", NodeStartedData{
			NodeID: node.ID,
			Label:  node.Label,
			Action: node.Action,
		})

		var (
			outputPort string
			output     Msg
			execErr    error
		)

		if node.Skip {
			outputPort = firstOutputPort(node.Action)
			output = DeepCloneMsg(item.msg)
			output = EnsureMsgID(output)
			output["_skipped"] = true
		} else if !startOverrideUsed && opts.StartMsg != nil && node.ID == opts.StartNodeID {
			startOverrideUsed = true
			outputPort = "output"
			output = DeepCloneMsg(opts.StartMsg)
			output = EnsureMsgID(output)
		} else if node.Action == "link-in" && item.msg != nil {
			outputPort = "output"
			output = DeepCloneMsg(item.msg)
			output = EnsureMsgID(output)
		} else {
			outputPort, output, execErr = s.ExecuteNode(execCtx, node, item.msg, flowCtx, opts.FallbackEnvID)
		}

		durationMs := time.Since(nodeStart).Milliseconds()
		result.NodesExecuted++

		emissions := flowCtx.Runtime.takeEmissions(node.ID)
		if len(emissions) == 0 && outputPort != "" {
			emissions = []nodeEmission{{Port: outputPort, Msg: output}}
		}

		displayOutput := output
		if displayOutput == nil && len(emissions) == 1 {
			displayOutput = emissions[0].Msg
		}
		if displayOutput == nil && len(emissions) > 1 {
			displayOutput = Msg{
				"_batch": map[string]interface{}{
					"count": len(emissions),
					"port":  emissions[0].Port,
				},
			}
		}

		if execErr != nil && outputPort == "" && len(emissions) == 0 {
			safeEmit(emit, "node.failed", NodeFailedData{
				NodeID: node.ID,
				Label:  node.Label,
				Action: node.Action,
				Error:  "Node execution failed",
			})

			if caughtFrame, caughtTargets, caughtMsg, caught := catchTargetsForMessage(adjacency, item.msg, node, execErr); caught {
				sourceLabel := caughtFrame.NodeID
				if guardNode := nodeMap[caughtFrame.NodeID]; guardNode != nil && guardNode.Label != "" {
					sourceLabel = guardNode.Label
				}
				result.LastOutput = caughtMsg
				queue = enqueueTargetsForEmission(emit, queue, caughtFrame.NodeID, sourceLabel, caughtTargets, caughtMsg)
				continue
			}

			hadNodeFailure = true
			if result.Error == "" {
				result.Error = execErr.Error()
			}
			continue
		}

		if displayOutput != nil {
			perf, _ := displayOutput["_performance"].(map[string]interface{})
			if perf == nil {
				perf = map[string]interface{}{}
			}
			perf[node.ID] = map[string]interface{}{"durationMs": durationMs}
			displayOutput["_performance"] = perf
			result.LastOutput = displayOutput
		}

		safeEmit(emit, "node.completed", NodeCompletedData{
			NodeID:     node.ID,
			Label:      node.Label,
			Action:     node.Action,
			OutputPort: outputPort,
			Input:      item.msg,
			Config:     node.Config,
			Output:     displayOutput,
			DurationMs: durationMs,
		})

		switch node.Action {
		case "debug":
			emitDebugNodeEvent(func(event string, data interface{}) {
				safeEmit(emit, event, data)
			}, node, displayOutput)
		case "log":
			logMsg := "Log output"
			if m, ok := node.Config["message"].(string); ok && m != "" {
				logMsg = m
			}
			safeEmit(emit, "debug", DebugData{
				NodeID:  node.ID,
				Label:   node.Label,
				Level:   "info",
				Message: logMsg,
				Data:    displayOutput,
				Source:  "debug-node",
			})
		default:
			if node.Debug {
				safeEmit(emit, "debug", DebugData{
					NodeID:  node.ID,
					Label:   node.Label,
					Level:   "info",
					Message: "Node debug output",
					Data:    displayOutput,
					Source:  "debug-node",
				})
			}
		}

		for _, emission := range emissions {
			result.LastOutput = emission.Msg
			if emission.Port == "" {
				continue
			}

			key := node.ID + ":" + emission.Port
			targets := adjacency[key]
			if len(targets) == 0 && emission.Port == "error" {
				if caughtFrame, caughtTargets, caughtMsg, caught := catchTargetsForMessage(adjacency, emission.Msg, node, nil); caught {
					sourceLabel := caughtFrame.NodeID
					if guardNode := nodeMap[caughtFrame.NodeID]; guardNode != nil && guardNode.Label != "" {
						sourceLabel = guardNode.Label
					}
					result.LastOutput = caughtMsg
					queue = enqueueTargetsForEmission(emit, queue, caughtFrame.NodeID, sourceLabel, caughtTargets, caughtMsg)
					continue
				}
			}
			queue = enqueueTargetsForEmission(emit, queue, node.ID, node.Label, targets, emission.Msg)
		}
	}

	result.DurationMs = time.Since(runStart).Milliseconds()
	result.Response = flowCtx.Runtime.responseSnapshot()
	if result.Status == "failed" {
		safeEmit(emit, "run.completed", RunStatusData{Status: "failed", DurationMs: result.DurationMs, Error: result.Error})
		return result
	}
	if hadNodeFailure {
		result.Status = "completed"
	}
	safeEmit(emit, "run.completed", RunStatusData{Status: result.Status, DurationMs: result.DurationMs, Error: result.Error})
	return result
}

func safeEmit(emit workflowEmitter, event string, data interface{}) {
	if emit != nil {
		emit(event, data)
	}
}
