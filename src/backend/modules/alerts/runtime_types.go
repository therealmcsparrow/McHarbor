// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
)

// MetricSample is the alert engine's normalized container metric snapshot.
type MetricSample struct {
	ID         string
	Name       string
	CPUPercent float64
	MemPercent float64
}

// HostMetrics contains the host-level data the host source alerts on.
//
// Everything here already exists on metrics.HostMetricsResponse; the
// adapter in internal/bootstrap maps it across so this module keeps no
// dependency on the metrics package.
type HostMetrics struct {
	CPUPercent float64
	MemPercent float64
	Load1      float64
	Load5      float64
	Load15     float64
	MemTotal   int64
	DiskTotal  int64
}

// ContainerSummary is the alert engine's normalized container list item.
type ContainerSummary struct {
	ID    string
	Names []string
	State string
}

// ContainerHealth carries the healthcheck verdict from inspect.
// Status is Docker's own vocabulary: "starting", "healthy",
// "unhealthy", or empty when the image declares no healthcheck.
type ContainerHealth struct {
	Status string
}

// ContainerInspectState carries the per-container state the container
// source alerts on.
type ContainerInspectState struct {
	FinishedAt string
	StartedAt  string
	ExitCode   int
	OOMKilled  bool
	Health     *ContainerHealth
}

// ContainerInspect is the minimal container inspect payload needed by alerts.
type ContainerInspect struct {
	State        *ContainerInspectState
	RestartCount int
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
//
// DB is read-only from the alert engine's perspective apart from its
// own tables: the platform source queries backup runs, scans, workflow
// runs and audit rows that other modules own, and never writes to them.
type EngineDeps struct {
	Metrics    MetricsSource
	Containers ContainerSource
	SystemInfo SystemInfoSource
	DB         *sql.DB
}
