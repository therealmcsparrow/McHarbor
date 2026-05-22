// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker_info

// DockerSystemInfo contains extended Docker daemon information.
type DockerSystemInfo struct {
	// Server
	ID            string `json:"id"`
	ServerVersion string `json:"serverVersion"`
	APIVersion    string `json:"apiVersion"`
	MinAPIVersion string `json:"minApiVersion"`
	GitCommit     string `json:"gitCommit"`
	GoVersion     string `json:"goVersion"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	KernelVersion string `json:"kernelVersion"`
	Hostname      string `json:"hostname"`

	// Resources
	NCPU     int   `json:"ncpu"`
	MemTotal int64 `json:"memTotal"`

	// Storage
	StorageDriver string            `json:"storageDriver"`
	DockerRootDir string            `json:"dockerRootDir"`
	DriverStatus  [][]string        `json:"driverStatus"`

	// Runtime
	CgroupDriver  string   `json:"cgroupDriver"`
	CgroupVersion string   `json:"cgroupVersion"`
	DefaultRuntime string  `json:"defaultRuntime"`
	Runtimes      []string `json:"runtimes"`

	// Counts
	Containers        int `json:"containers"`
	ContainersRunning int `json:"containersRunning"`
	ContainersPaused  int `json:"containersPaused"`
	ContainersStopped int `json:"containersStopped"`
	Images            int `json:"images"`

	// Security
	SecurityOptions []string `json:"securityOptions"`

	// Plugins
	PluginsVolume  []string `json:"pluginsVolume"`
	PluginsNetwork []string `json:"pluginsNetwork"`
	PluginsLog     []string `json:"pluginsLog"`

	// Labels
	Labels []string `json:"labels"`

	// Swarm
	SwarmActive   bool   `json:"swarmActive"`
	SwarmNodeID   string `json:"swarmNodeId"`
	SwarmManagers int    `json:"swarmManagers"`
	SwarmNodes    int    `json:"swarmNodes"`

	// Logging
	LoggingDriver string `json:"loggingDriver"`

	// Isolation (Windows)
	Isolation string `json:"isolation"`
}
