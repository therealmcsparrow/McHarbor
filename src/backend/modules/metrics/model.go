// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

// HostInfo contains system-level information about the Docker host.
type HostInfo struct {
	NCPU          int    `json:"ncpu"`
	MemTotal      int64  `json:"memTotal"`
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

// HostMetricsResponse combines host info and disk usage.
type HostMetricsResponse struct {
	Host HostInfo  `json:"host"`
	Disk DiskUsage `json:"disk"`
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
