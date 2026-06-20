// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

// PreferenceRequest is the JSON body for POST /autoheal/preference/{id}.
type PreferenceRequest struct {
	Enabled bool `json:"enabled"`
}

// Preference is the response body for GET /autoheal/preference/{id}.
type Preference struct {
	ContainerID  string `json:"containerId"`
	ContainerName string `json:"containerName"`
	Enabled      bool   `json:"enabled"`
	LastHealAt   string `json:"lastHealAt,omitempty"`
	RestartCount int    `json:"restartCount"`
	WasEverHealthy bool `json:"wasEverHealthy"`
}

// HealEvent is one auto-heal action taken by the engine. Used for audit
// logging and in-app notifications.
type HealEvent struct {
	EnvID         string
	ContainerID   string
	ContainerName string
	Reason        string
}
