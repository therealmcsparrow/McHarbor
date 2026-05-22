// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "time"

type nodeEmission struct {
	Port string
	Msg  Msg
}

type executionResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

type aggregateBuffer struct {
	Messages []Msg
	Expected int
}

// ExecutionRuntime holds per-run state used by nodes that need to coordinate
// across multiple messages or write an HTTP response.
type ExecutionRuntime struct {
	pendingEmissions map[string][]nodeEmission
	joinBuffers      map[string][]Msg
	aggregateBuffers map[string]*aggregateBuffer
	rateLimitBuckets map[string][]time.Time
	response         *executionResponse
}

func newExecutionRuntime() *ExecutionRuntime {
	return &ExecutionRuntime{
		pendingEmissions: make(map[string][]nodeEmission),
		joinBuffers:      make(map[string][]Msg),
		aggregateBuffers: make(map[string]*aggregateBuffer),
		rateLimitBuckets: make(map[string][]time.Time),
	}
}

func (rt *ExecutionRuntime) setEmissions(nodeID string, emissions []nodeEmission) {
	if rt == nil {
		return
	}
	if len(emissions) == 0 {
		delete(rt.pendingEmissions, nodeID)
		return
	}
	rt.pendingEmissions[nodeID] = emissions
}

func (rt *ExecutionRuntime) takeEmissions(nodeID string) []nodeEmission {
	if rt == nil {
		return nil
	}
	emissions := rt.pendingEmissions[nodeID]
	delete(rt.pendingEmissions, nodeID)
	return emissions
}

func (rt *ExecutionRuntime) setResponse(statusCode int, headers map[string]string, body []byte) {
	if rt == nil {
		return
	}
	clonedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		clonedHeaders[k] = v
	}
	clonedBody := make([]byte, len(body))
	copy(clonedBody, body)
	rt.response = &executionResponse{
		StatusCode: statusCode,
		Headers:    clonedHeaders,
		Body:       clonedBody,
	}
}

func (rt *ExecutionRuntime) responseSnapshot() *executionResponse {
	if rt == nil || rt.response == nil {
		return nil
	}
	headers := make(map[string]string, len(rt.response.Headers))
	for k, v := range rt.response.Headers {
		headers[k] = v
	}
	body := make([]byte, len(rt.response.Body))
	copy(body, rt.response.Body)
	return &executionResponse{
		StatusCode: rt.response.StatusCode,
		Headers:    headers,
		Body:       body,
	}
}

type workflowRunOptions struct {
	WorkflowID         string
	Trigger            string
	StartNodeID        string
	StartMsg           Msg
	StartInputMsg      Msg
	FallbackEnvID      string
	AutoEnqueueLinkIns bool
	Timeout            time.Duration
}

type workflowRunResult struct {
	Status        string
	DurationMs    int64
	NodesExecuted int
	LastOutput    Msg
	Error         string
	Response      *executionResponse
}
