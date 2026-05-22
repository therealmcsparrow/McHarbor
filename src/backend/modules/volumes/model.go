// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package volumes

// CreateRequest is the JSON body for POST /volumes.
type CreateRequest struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driverOpts,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// VolumeSummary is a simplified volume info for list responses.
// JSON tags use PascalCase to match Docker API convention and frontend types.
type VolumeSummary struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  string            `json:"CreatedAt"`
	Status     map[string]any    `json:"Status,omitempty"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Options    map[string]string `json:"Options,omitempty"`
	UsageData  *UsageData        `json:"UsageData,omitempty"`
	RefCount   int               `json:"RefCount"`
}

// UsageData represents volume usage statistics.
type UsageData struct {
	Size     int64 `json:"Size"`
	RefCount int64 `json:"RefCount"`
}
