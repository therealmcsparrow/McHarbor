// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

// ComponentVersion describes a McHarbor component version.
type ComponentVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentVersion describes the version state of one agent environment.
type AgentVersion struct {
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

// VersionInfo is the combined McHarbor and agent version payload.
type VersionInfo struct {
	McHarbor ComponentVersion `json:"mcharbor"`
	Agents   []AgentVersion   `json:"agents"`
}

// SelfUpdateRequest is the JSON body for POST /api/versions/self-update.
// Image is optional: when empty, the current image is reused (i.e. the
// helper re-pulls the same tag to refresh the digest, then recreates the
// container with the new image).
type SelfUpdateRequest struct {
	Image string `json:"image"`
}

// SelfUpdateResult is the response from POST /api/versions/self-update.
// The container is scheduled to restart shortly; the operator
// disconnects and reconnects once the new image is up.
type SelfUpdateResult struct {
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	TargetImage   string `json:"targetImage"`
	Output        string `json:"output"`
}
