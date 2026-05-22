// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import "context"

// MetricSample is the alert engine's normalized container metric snapshot.
type MetricSample struct {
	ID         string
	Name       string
	CPUPercent float64
	MemPercent float64
}

// HostMetrics contains the host-level data needed by disk alerts.
type HostMetrics struct {
	DiskTotal int64
}

// ContainerSummary is the alert engine's normalized container list item.
type ContainerSummary struct {
	ID    string
	Names []string
	State string
}

// ContainerInspectState carries the stopped timestamp used by down alerts.
type ContainerInspectState struct {
	FinishedAt string
}

// ContainerInspect is the minimal container inspect payload needed by alerts.
type ContainerInspect struct {
	State *ContainerInspectState
}

// SystemInfo carries the Docker daemon storage metadata needed by disk alerts.
type SystemInfo struct {
	DriverStatus [][]string
}

// ImageUpdateCheck represents a single image update result.
type ImageUpdateCheck struct {
	ContainerID     string
	ContainerName   string
	CurrentDigest   string
	RemoteDigest    string
	UpdateAvailable bool
}

// MetricsSource loads host and container metrics for alert evaluation.
type MetricsSource interface {
	AllContainerStats(context.Context, string) ([]MetricSample, error)
	HostInfo(context.Context, string) (*HostMetrics, error)
}

// ContainerSource loads container state used by alert evaluation.
type ContainerSource interface {
	ListContainers(context.Context, string, bool) ([]ContainerSummary, error)
	InspectContainer(context.Context, string, string) (*ContainerInspect, error)
	CheckImageUpdates(context.Context, string, []string) ([]ImageUpdateCheck, error)
}

// SystemInfoSource loads Docker daemon details for disk alerts.
type SystemInfoSource interface {
	SystemInfo(context.Context, string) (*SystemInfo, error)
}

// EngineDeps supplies runtime dependencies without coupling the alerts module
// directly to sibling feature packages.
type EngineDeps struct {
	Metrics    MetricsSource
	Containers ContainerSource
	SystemInfo SystemInfoSource
}
