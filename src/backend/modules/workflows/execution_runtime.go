// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "time"

// ExecutionRuntime holds per-run state that is shared between the
// executor loop and the runtime-aware node implementations
// (range/loop, join, aggregate, http-response, webhook-response).
// It is attached to the FlowContext so any node can read or
// mutate the buffers and the captured HTTP response.
type ExecutionRuntime struct {
	// emissions holds the per-port output messages produced by a
	// range or loop node during the current step. The executor
	// reads and clears them via takeEmissions after the node runs.
	emissions map[string][]nodeEmission
	// joinBuffers accumulates incoming messages for a join node
	// until its expected input count is reached.
	joinBuffers map[string][]Msg
	// aggregateBuffers collects the messages feeding an
	// aggregate node before the final reduction runs.
	aggregateBuffers map[string]*aggregateBuffer
	// response, when non-nil, is the captured HTTP response that a
	// webhook-response or http-response node installed. The
	// executor copies it onto workflowRunResult via
	// responseSnapshot() so the handler can write it back.
	response *executionResponse
}

// newExecutionRuntime allocates an empty runtime ready to be
// attached to a FlowContext.
func newExecutionRuntime() *ExecutionRuntime {
	return &ExecutionRuntime{
		emissions:        make(map[string][]nodeEmission),
		joinBuffers:      make(map[string][]Msg),
		aggregateBuffers: make(map[string]*aggregateBuffer),
	}
}

// setEmissions records the multi-port outputs of a node so the
// executor loop can fan them out to downstream nodes.
func (r *ExecutionRuntime) setEmissions(nodeID string, emissions []nodeEmission) {
	r.emissions[nodeID] = emissions
}

// takeEmissions returns and clears the emissions recorded for a
// node. Returns nil when the node did not register any.
func (r *ExecutionRuntime) takeEmissions(nodeID string) []nodeEmission {
	emissions := r.emissions[nodeID]
	delete(r.emissions, nodeID)
	return emissions
}

// setResponse captures the HTTP response a webhook-response or
// http-response node wants the workflow invocation to return.
func (r *ExecutionRuntime) setResponse(statusCode int, headers map[string]string, body []byte) {
	r.response = &executionResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}
}

// responseSnapshot returns the captured HTTP response (or nil if
// no response node ran). The executor assigns the result onto
// workflowRunResult.Response so the handler can serialise it.
func (r *ExecutionRuntime) responseSnapshot() *executionResponse {
	return r.response
}

// nodeEmission is one port-tagged message produced by a node.
// Multi-port outputs (range, loop, batch nodes) emit one
// nodeEmission per downstream message.
type nodeEmission struct {
	Port string
	Msg  Msg
}

// aggregateBuffer collects the messages feeding an aggregate
// node before the final reduction runs. Expected is updated from
// the incoming msg's parts metadata when present.
type aggregateBuffer struct {
	Expected int
	Messages []Msg
}

// executionResponse is the captured HTTP response a
// webhook-response / http-response node wants the workflow
// invocation to return to the caller.
type executionResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// workflowRunOptions controls a single ExecuteWorkflow call.
// Trigger values used across the codebase are "manual",
// "auto", "webhook", and "link".
type workflowRunOptions struct {
	// Timeout caps the total run duration. A zero value falls
	// back to the executor's default.
	Timeout time.Duration
	// WorkflowID is the workflow being executed. Stored on
	// FlowContext.FlowVars["_workflowId"] so scoped expressions
	// can reference the current run.
	WorkflowID string
	// Trigger describes why this run started.
	Trigger string
	// StartNodeID, if non-empty, begins execution at this node.
	StartNodeID string
	// StartMsg, if non-empty, overrides the initial input for the
	// StartNodeID node. Distinguish from StartInputMsg below.
	StartMsg Msg
	// StartInputMsg feeds every auto-enqueued link-in node and
	// supplies the initial queue item for StartNodeID.
	StartInputMsg Msg
	// FallbackEnvID is passed to ExecuteNode for nodes that do
	// not declare their own environment.
	FallbackEnvID string
	// AutoEnqueueLinkIns enables auto-enqueuing every link-in
	// node in the canvas so they can receive external payloads.
	AutoEnqueueLinkIns bool
}

// workflowRunResult is the outcome of a single ExecuteWorkflow
// call. Fields are populated by the executor and consumed by
// the HTTP handlers and the trigger service.
type workflowRunResult struct {
	Status        string
	Error         string
	DurationMs    int64
	NodesExecuted int
	LastOutput    Msg
	// Response, if non-nil, is the captured HTTP response from a
	// webhook-response or http-response node.
	Response *executionResponse
}