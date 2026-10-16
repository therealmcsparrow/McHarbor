// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

// HostInfo contains system-level information about the Docker host.
type HostInfo struct {
	NCPU          int     `json:"ncpu"`
	MemTotal      int64   `json:"memTotal"`
	MemUsed       int64   `json:"memUsed"`
	MemPercent    float64 `json:"memPercent"`
	CPUPercent    float64 `json:"cpuPercent"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	ServerVersion string  `json:"serverVersion"`
	OS            string  `json:"os"`
	Architecture  string  `json:"architecture"`
	KernelVersion string  `json:"kernelVersion"`
	Hostname      string  `json:"hostname"`
	Uptime        int64   `json:"uptime"`
	SystemTime    string  `json:"systemTime"`
}

// DiskUsage contains Docker disk usage breakdown.
type DiskUsage struct {
	ImagesSize     int64 `json:"imagesSize"`
	ContainersSize int64 `json:"containersSize"`
	VolumesSize    int64 `json:"volumesSize"`
	BuildCacheSize int64 `json:"buildCacheSize"`
	Total          int64 `json:"total"`
}

// HostFSUsage contains the host filesystem usage of the path that holds the
// Docker data root (typically `/`). It is best-effort and reports zeros when
// the host filesystem cannot be read (e.g. agent environments where the
// agent does not expose a statfs equivalent).
type HostFSUsage struct {
	Path    string  `json:"path"`
	Total   int64   `json:"total"`
	Used    int64   `json:"used"`
	Percent float64 `json:"percent"`
}

// HostMetricsResponse combines host info, Docker disk usage, and host FS usage.
type HostMetricsResponse struct {
	Host       HostInfo    `json:"host"`
	Disk       DiskUsage   `json:"disk"`
	HostFS     HostFSUsage `json:"hostFs"`
	AgentLimit bool        `json:"agentLimit"`
}

// ContainerMetric holds calculated stats for a single container.
type ContainerMetric struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Key is the container's stable identity (docker.ContainerKey): what the
	// history tables are keyed on, so a series survives the container being
	// recreated under a new ID. Not serialised — it exists for the collector's
	// write path, and the live stats endpoints have no use for it.
	Key string `json:"-"`

	CPUPercent float64 `json:"cpuPercent"`
	MemUsage   int64   `json:"memUsage"`
	MemLimit   int64   `json:"memLimit"`
	MemPercent float64 `json:"memPercent"`
	NetRx      int64   `json:"netRx"`
	NetTx      int64   `json:"netTx"`
	BlockRead  int64   `json:"blockRead"`
	BlockWrite int64   `json:"blockWrite"`
	PIDs       uint64  `json:"pids"`
}

// PruneRequest is the JSON body for POST /metrics/host/prune.
type PruneRequest struct {
	Type    string `json:"type"`              // "system" | "builder" | "volumes" | "images" | "containers" | "networks"
	Volumes bool   `json:"volumes,omitempty"` // only meaningful for "system" — explicit opt-in
	Confirm bool   `json:"confirm"`           // required for any destructive prune
}

// PruneResult reports what was reclaimed by a prune operation.
type PruneResult struct {
	Type           string `json:"type"`
	ItemsDeleted   int64  `json:"itemsDeleted"`
	SpaceReclaimed int64  `json:"spaceReclaimed"`
}

// MetricPoint is one point of a resource-metric series.
//
// Raw and rollup points carry the same fields so a chart never has to branch on
// resolution. For a raw point the "max" fields equal their average counterpart
// and Samples is 1; for a folded point they describe the bucket.
//
// The counter fields come in pairs on purpose: NetRx and friends are Docker's
// cumulative totals since container start, while the *Delta fields are the
// movement across this point's interval. A throughput chart wants the delta; a
// "total transferred" figure wants the counter. Averaging the counter itself
// would be meaningless, which is why the rollup never stores an average of one.
type MetricPoint struct {
	Timestamp  string `json:"timestamp"`
	Resolution string `json:"resolution"` // "raw" (30s) | "rollup" (bucketed)
	Samples    int    `json:"samples"`

	CPUPercent float64 `json:"cpuPercent"`
	CPUMax     float64 `json:"cpuMax"`

	// GPUPercent is mean utilisation across the host's GPUs, or nil
	// when this host reported none. A pointer rather than a float so
	// the frontend can omit the series entirely instead of drawing a
	// flat zero line for every host without a GPU.
	GPUPercent *float64 `json:"gpuPercent,omitempty"`
	MemUsed    int64    `json:"memUsed"`
	MemUsedMax int64    `json:"memUsedMax"`
	MemLimit   int64    `json:"memLimit"`
	MemPercent float64  `json:"memPercent"`

	NetRx      int64 `json:"netRx"`
	NetTx      int64 `json:"netTx"`
	BlockRead  int64 `json:"blockRead"`
	BlockWrite int64 `json:"blockWrite"`

	NetRxDelta      int64 `json:"netRxDelta"`
	NetTxDelta      int64 `json:"netTxDelta"`
	BlockReadDelta  int64 `json:"blockReadDelta"`
	BlockWriteDelta int64 `json:"blockWriteDelta"`

	// Nil rather than zero when no hourly disk sample fell in this interval,
	// so "not measured" stays distinguishable from "measured as empty".
	SizeRw     *int64 `json:"sizeRw,omitempty"`
	SizeRootFs *int64 `json:"sizeRootFs,omitempty"`
}

// MetricSeries is a stored history response for one container or for an
// environment aggregate.
//
// CoveredFrom/CoveredTo report what the series actually contains, which is not
// always what was asked for: history only starts accumulating when the
// collector first runs, and the rollup horizon is finite. Callers that print a
// period — reports above all — must compare these against From/To and say so
// when the window is not fully covered.
type MetricSeries struct {
	EnvironmentID string `json:"environmentId"`
	ContainerID   string `json:"containerId,omitempty"`
	Name          string `json:"name,omitempty"`

	From        string `json:"from"`
	To          string `json:"to"`
	CoveredFrom string `json:"coveredFrom,omitempty"`
	CoveredTo   string `json:"coveredTo,omitempty"`

	// BucketSeconds is the grid of the points below: 30 for raw resolution,
	// metricsBucketSeconds when the window was wide enough to fold on read.
	BucketSeconds int `json:"bucketSeconds"`

	Points []MetricPoint `json:"points"`
}
