// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

// HostInfo contains system-level information about the Docker host.
type HostInfo struct {
	NCPU          int    `json:"ncpu"`
	MemTotal      int64  `json:"memTotal"`
	MemUsed       int64  `json:"memUsed"`
	MemPercent    float64 `json:"memPercent"`
	CPUPercent    float64 `json:"cpuPercent"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	ServerVersion string `json:"serverVersion"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	KernelVersion string `json:"kernelVersion"`
	Hostname      string `json:"hostname"`
	Uptime        int64  `json:"uptime"`
	SystemTime    string `json:"systemTime"`
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
	ID         string  `json:"id"`
	Name       string  `json:"name"`
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
