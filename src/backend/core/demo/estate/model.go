// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package estate is the in-memory world demo mode simulates: hosts with their
// Docker containers, images, volumes, networks and Compose stacks. Virtual
// machines and Kubernetes live in sibling packages keyed by the same host key.
// Nothing in it touches real infrastructure.
package estate

import (
	"sync"
	"time"
)

// ContainerState mirrors the Docker states the UI renders distinctly.
type ContainerState string

// The container states the simulator produces.
const (
	StateCreated    ContainerState = "created"
	StateRunning    ContainerState = "running"
	StatePaused     ContainerState = "paused"
	StateRestarting ContainerState = "restarting"
	StateExited     ContainerState = "exited"
	StateDead       ContainerState = "dead"
)

// PortBinding is one published or exposed container port.
type PortBinding struct {
	Private int
	Public  int
	Proto   string
}

// MountPoint is one volume or bind mount of a container.
type MountPoint struct {
	Type        string
	Name        string
	Source      string
	Destination string
	RW          bool
}

// Container is one simulated Docker container. Profile picks its log flavour
// and its CPU/memory baseline so a database looks like a database.
type Container struct {
	ID            string
	Name          string
	Image         string
	ImageID       string
	Command       string
	Entrypoint    []string
	Created       time.Time
	State         ContainerState
	ExitCode      int
	Error         string
	StartedAt     time.Time
	FinishedAt    time.Time
	Health        string
	RestartCount  int
	RestartPolicy string
	Labels        map[string]string
	Env           []string
	Ports         []PortBinding
	Mounts        []MountPoint
	Networks      map[string]string
	Hostname      string
	WorkingDir    string
	User          string
	Profile       string
	CPUBase       float64
	MemBase       int64
	MemLimit      int64
	SizeRw        int64
}

// Image is one simulated image; Dangling images carry no tag.
type Image struct {
	ID       string
	RepoTags []string
	Digest   string
	Created  time.Time
	Size     int64
	Labels   map[string]string
	Arch     string
	OS       string
}

// Volume is one simulated named volume.
type Volume struct {
	Name      string
	Driver    string
	Created   time.Time
	Labels    map[string]string
	SizeBytes int64
}

// Network is one simulated Docker network.
type Network struct {
	ID       string
	Name     string
	Driver   string
	Scope    string
	Subnet   string
	Gateway  string
	Internal bool
	Created  time.Time
	Labels   map[string]string
}

// Exec is a pending or finished exec session inside a container.
type Exec struct {
	ID          string
	ContainerID string
	Cmd         []string
	Tty         bool
	Running     bool
	ExitCode    int
}

// Stack is a Compose project; its containers carry the compose labels, and
// Compose is the file text the stacks page shows.
type Stack struct {
	Name    string
	Compose string
	Env     map[string]string
}

// Host is one simulated environment. Every field behind mu changes only through
// Host methods so the events feed sees every mutation.
type Host struct {
	Key           string
	Name          string
	Region        string
	Role          string
	Hostname      string
	OS            string
	Distro        string
	OSVersion     string
	Kernel        string
	Arch          string
	CPUs          int
	MemTotal      int64
	DiskTotal     int64
	DiskUsed      int64
	DockerVersion string
	BootedAt      time.Time
	Compute       bool
	seed          uint64

	mu         sync.RWMutex
	containers map[string]*Container
	byName     map[string]string
	images     map[string]*Image
	volumes    map[string]*Volume
	networks   map[string]*Network
	execs      map[string]*Exec
	stacks     map[string]*Stack

	events *broadcaster
}

// Estate is every simulated Docker host, built once per process from a fixed seed.
type Estate struct {
	Hosts []*Host
	byKey map[string]*Host
}

// Host returns the host keyed key, or nil.
func (e *Estate) Host(key string) *Host { return e.byKey[key] }

func newHost(key string) *Host {
	return &Host{
		Key:        key,
		containers: make(map[string]*Container),
		byName:     make(map[string]string),
		images:     make(map[string]*Image),
		volumes:    make(map[string]*Volume),
		networks:   make(map[string]*Network),
		execs:      make(map[string]*Exec),
		stacks:     make(map[string]*Stack),
		events:     newBroadcaster(),
	}
}
