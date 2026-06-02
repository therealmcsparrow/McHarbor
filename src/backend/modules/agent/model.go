// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

// AgentInfo represents agent status information returned by API endpoints.
type AgentInfo struct {
	EnvID         string `json:"envId"`
	EnvName       string `json:"envName"`
	Status        string `json:"status"`
	Hostname      string `json:"hostname,omitempty"`
	OS            string `json:"os,omitempty"`
	Arch          string `json:"arch,omitempty"`
	AgentVersion  string `json:"agentVersion,omitempty"`
	DockerVersion string `json:"dockerVersion,omitempty"`
	LastSeen      string `json:"lastSeen,omitempty"`
}

// DirectTransferTestRequest selects the source and target agents for a probe.
type DirectTransferTestRequest struct {
	SourceEnvID string `json:"sourceEnvId"`
	TargetEnvID string `json:"targetEnvId"`
}

// DirectTransferTestResult reports the outcome of an agent-to-agent probe.
type DirectTransferTestResult struct {
	Success           bool                      `json:"success"`
	Phase             string                    `json:"phase"`
	SourceEnvID       string                    `json:"sourceEnvId"`
	SourceName        string                    `json:"sourceName,omitempty"`
	SourceVersion     string                    `json:"sourceVersion,omitempty"`
	SourceConnected   bool                      `json:"sourceConnected"`
	TargetEnvID       string                    `json:"targetEnvId"`
	TargetName        string                    `json:"targetName,omitempty"`
	TargetVersion     string                    `json:"targetVersion,omitempty"`
	TargetConnected   bool                      `json:"targetConnected"`
	TargetTransferURL string                    `json:"targetTransferUrl,omitempty"`
	ProbeURL          string                    `json:"probeUrl,omitempty"`
	StatusCode        int                       `json:"statusCode,omitempty"`
	DurationMs        int64                     `json:"durationMs"`
	Error             string                    `json:"error,omitempty"`
	Diagnostic        *DirectTransferDiagnostic `json:"diagnostic,omitempty"`
}

// DirectTransferDiagnostic carries token-safe target receiver auth details.
type DirectTransferDiagnostic struct {
	ReceiverExists  bool   `json:"receiverExists"`
	ReceiverExpired bool   `json:"receiverExpired"`
	ReceiverKind    string `json:"receiverKind,omitempty"`
	KindMatched     bool   `json:"kindMatched"`
	BearerPresent   bool   `json:"bearerPresent"`
	TokenMatched    bool   `json:"tokenMatched"`
	RemoteAddr      string `json:"remoteAddr,omitempty"`
}
