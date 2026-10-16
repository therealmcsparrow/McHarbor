// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import "encoding/json"

// Info represents agent status information returned by API endpoints.
type Info struct {
	EnvID        string `json:"envId"`
	EnvName      string `json:"envName"`
	Status       string `json:"status"`
	Hostname     string `json:"hostname,omitempty"`
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	AgentVersion string `json:"agentVersion,omitempty"`
	// AgentBuild comes from the live connection, not the environments row:
	// nothing persists it, and a build stamp is only meaningful for the agent
	// that is connected right now. Empty for a disconnected or older agent.
	AgentBuild    string `json:"agentBuild,omitempty"`
	DockerVersion string `json:"dockerVersion,omitempty"`
	LastSeen      string `json:"lastSeen,omitempty"`

	// Health is the derived state - healthy, degraded, unhealthy or unknown -
	// and is orthogonal to Status. A connected agent whose keepalive has gone
	// stale, or whose round trip has blown past the point where ordinary Docker
	// calls start timing out, is connected and degraded at the same time. See
	// health.go for the derivation.
	Health string `json:"health"`
	// LatencyMs is the last measured ping/pong round trip, omitted when there
	// is no live connection to measure.
	LatencyMs *int `json:"latencyMs,omitempty"`
}

// LogEntry is one line of the remote agent's own log, replayed from the
// agent's offline buffer after a reconnect.
type LogEntry struct {
	ID       string          `json:"id"`
	Level    string          `json:"level"`
	Message  string          `json:"message"`
	Attrs    json.RawMessage `json:"attrs,omitempty"`
	LoggedAt string          `json:"loggedAt"`
}

// DirectTransferTestRequest selects the source and target agents for a probe.
type DirectTransferTestRequest struct {
	SourceEnvID string `json:"sourceEnvId"`
	TargetEnvID string `json:"targetEnvId"`
}

// DirectTransferTestResult reports the outcome of an agent-to-agent probe.
type DirectTransferTestResult struct {
	Success           bool   `json:"success"`
	Phase             string `json:"phase"`
	SourceEnvID       string `json:"sourceEnvId"`
	SourceName        string `json:"sourceName,omitempty"`
	SourceVersion     string `json:"sourceVersion,omitempty"`
	SourceConnected   bool   `json:"sourceConnected"`
	TargetEnvID       string `json:"targetEnvId"`
	TargetName        string `json:"targetName,omitempty"`
	TargetVersion     string `json:"targetVersion,omitempty"`
	TargetConnected   bool   `json:"targetConnected"`
	TargetTransferURL string `json:"targetTransferUrl,omitempty"`
	ProbeURL          string `json:"probeUrl,omitempty"`
	// CandidatesTried counts how many addresses were probed before one
	// answered, so an operator can see when a transfer is only working via a
	// late candidate and tighten the configuration.
	CandidatesTried int `json:"candidatesTried,omitempty"`
	// CandidateCount is how many addresses the target advertised in total.
	CandidateCount  int                       `json:"candidateCount,omitempty"`
	StatusCode      int                       `json:"statusCode,omitempty"`
	DurationMs      int64                     `json:"durationMs"`
	Error           string                    `json:"error,omitempty"`
	Receiver        *DirectTransferReceiver   `json:"receiver,omitempty"`
	ResponderMarker string                    `json:"responderMarker,omitempty"`
	Diagnostic      *DirectTransferDiagnostic `json:"diagnostic,omitempty"`
}

// DirectTransferReceiver carries token-safe prepared receiver metadata.
type DirectTransferReceiver struct {
	TransferID       string `json:"transferId"`
	Kind             string `json:"kind"`
	ExpiresAt        string `json:"expiresAt"`
	TokenFingerprint string `json:"tokenFingerprint"`
	AgentMarker      string `json:"agentMarker,omitempty"`
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
	ResponderMarker string `json:"responderMarker,omitempty"`
}
