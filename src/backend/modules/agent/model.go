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
