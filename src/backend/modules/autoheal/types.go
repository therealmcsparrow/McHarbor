// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package autoheal

import "context"

// LabelKey is the Docker label that opts a container into the auto-heal loop.
// Set to "enabled" to opt in, "disabled" to opt out. Missing label = opted out.
const LabelKey = "com.mcharbor.autoheal"
const LabelEnabled = "enabled"
const LabelDisabled = "disabled"

// ContainersSource is the minimal container interface the engine and
// service depend on so they don't have to import the containers module.
type ContainersSource interface {
	ListRunning(ctx context.Context, envID string) ([]Container, error)
	Inspect(ctx context.Context, envID, id string) (*Inspect, error)
	Restart(ctx context.Context, envID, id string) error
	SetLabel(ctx context.Context, envID, id, key, value string) error
	UnsetLabel(ctx context.Context, envID, id, key string) error
}

// Container is a minimal container summary used by the engine.
type Container struct {
	ID      string
	Name    string
	State   string
	Labels  map[string]string
}

// Inspect is the minimal container inspect payload needed by the engine.
type Inspect struct {
	State *InspectState
}

// InspectState is the health-aware subset of the container State struct.
type InspectState struct {
	Health      *Health
	Running     bool
	StartedAt   string
	FinishedAt  string
}

// Health is the Docker healthcheck report.
type Health struct {
	Status string // "starting" | "healthy" | "unhealthy" | "none"
}
