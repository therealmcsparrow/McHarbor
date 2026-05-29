// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "errors"

var ErrInvalidWorkflowExport = errors.New("invalid workflow export")

// Workflow represents a workflow with its canvas data.
type Workflow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CanvasData  string `json:"canvasData"`
	Variables   string `json:"variables"`
	CreatedBy   string `json:"createdBy"`
	UpdatedBy   string `json:"updatedBy"`
	LastRunAt   string `json:"lastRunAt,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// WorkflowExport is the portable per-workflow export file format.
type WorkflowExport struct {
	Kind       string               `json:"kind"`
	Version    int                  `json:"version"`
	ExportedAt string               `json:"exportedAt"`
	Workflow   WorkflowExportRecord `json:"workflow"`
}

// WorkflowExportRecord is the workflow payload included in an export file.
type WorkflowExportRecord struct {
	OriginalID        string `json:"originalId,omitempty"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Status            string `json:"status,omitempty"`
	CanvasData        string `json:"canvasData"`
	Variables         string `json:"variables"`
	OriginalCreatedAt string `json:"originalCreatedAt,omitempty"`
	OriginalUpdatedAt string `json:"originalUpdatedAt,omitempty"`
}

// CreateInput is the request body for creating a workflow.
type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateInput is the request body for updating a workflow.
type UpdateInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	CanvasData  *string `json:"canvasData,omitempty"`
	Variables   *string `json:"variables,omitempty"`
}

// ExecuteInput is the request body for executing a workflow.
type ExecuteInput struct {
	TriggerNodeID string `json:"triggerNodeId"`
}

// CanvasData represents the parsed canvas JSON.
type CanvasData struct {
	Nodes  []CanvasNode  `json:"nodes"`
	Edges  []CanvasEdge  `json:"edges"`
	Groups []CanvasGroup `json:"groups,omitempty"`
}

// CanvasGroup represents a visual group of nodes.
type CanvasGroup struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Color   string   `json:"color"`
	NodeIDs []string `json:"nodeIds"`
	Blocked bool     `json:"blocked,omitempty"`
}

// CanvasNode represents a node on the canvas.
type CanvasNode struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Action       string                 `json:"action"`
	Label        string                 `json:"label"`
	Config       map[string]interface{} `json:"config"`
	Debug        bool                   `json:"debug,omitempty"`
	BlockedPorts []string               `json:"blockedPorts,omitempty"`
	Skip         bool                   `json:"skip,omitempty"`
	Disabled     bool                   `json:"disabled,omitempty"`
}

// CanvasEdge represents a connection between nodes.
type CanvasEdge struct {
	ID           string       `json:"id"`
	SourceNodeID string       `json:"sourceNodeId"`
	SourcePort   string       `json:"sourcePort"`
	TargetNodeID string       `json:"targetNodeId"`
	TargetPort   string       `json:"targetPort"`
	Sniffer      *EdgeSniffer `json:"sniffer,omitempty"`
}

// EdgeSniffer holds sniffer metadata for an edge.
type EdgeSniffer struct {
	Name string `json:"name"`
}

// WorkflowRun represents a single execution of a workflow.
type WorkflowRun struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	Status     string `json:"status"`
	Trigger    string `json:"trigger"`
	DurationMs int64  `json:"durationMs"`
	NodeCount  int    `json:"nodeCount"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

// NodeStartedData is sent when a node begins execution.
type NodeStartedData struct {
	NodeID string `json:"nodeId"`
	Label  string `json:"label"`
	Action string `json:"action"`
}

// NodeCompletedData is sent when a node finishes execution.
type NodeCompletedData struct {
	NodeID     string      `json:"nodeId"`
	Label      string      `json:"label"`
	Action     string      `json:"action"`
	OutputPort string      `json:"outputPort"`
	Input      interface{} `json:"input"`
	Config     interface{} `json:"config"`
	Output     interface{} `json:"output"`
	DurationMs int64       `json:"durationMs"`
}

// NodeFailedData is sent when a node fails.
type NodeFailedData struct {
	NodeID string `json:"nodeId"`
	Label  string `json:"label"`
	Action string `json:"action"`
	Error  string `json:"error"`
}

// EdgeTraversedData is sent when data flows along an edge.
type EdgeTraversedData struct {
	EdgeID       string `json:"edgeId"`
	SourceNodeID string `json:"sourceNodeId"`
	TargetNodeID string `json:"targetNodeId"`
}

// DebugData is sent for debug/log/sniffer messages.
type DebugData struct {
	NodeID  string      `json:"nodeId"`
	Label   string      `json:"label"`
	Level   string      `json:"level"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Source  string      `json:"source"`
}

// RunStatusData is sent for overall run status changes.
type RunStatusData struct {
	Status     string `json:"status"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// LinkMessage represents a stored message from a link-out node.
type LinkMessage struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	NodeID     string `json:"nodeId"`
	Name       string `json:"name"`
	Msg        string `json:"msg"`
	UpdatedAt  string `json:"updatedAt"`
}

// LinkOutputInfo describes a link-out node for the dropdown select.
type LinkOutputInfo struct {
	WorkflowID   string `json:"workflowId"`
	WorkflowName string `json:"workflowName"`
	NodeID       string `json:"nodeId"`
	Name         string `json:"name"`
}

// FlowContext holds scoped variables for a workflow execution.
type FlowContext struct {
	FlowVars   map[string]interface{} `json:"flowVars"`
	GlobalVars map[string]interface{} `json:"globalVars"`
	Runtime    *ExecutionRuntime      `json:"-"`
}

// containerMetricSnapshot holds a point-in-time metric reading for a container.
type containerMetricSnapshot struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsage   uint64  `json:"mem_usage"`
	MemLimit   uint64  `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
	NetRx      uint64  `json:"net_rx"`
	NetTx      uint64  `json:"net_tx"`
	BlockRead  uint64  `json:"block_read"`
	BlockWrite uint64  `json:"block_write"`
	PIDs       uint64  `json:"pids"`
}
