// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import "fmt"

// Config sizes the simulated estate and how far back its history reaches. It is
// chosen when demo mode is entered or reset and travels in the marker, because
// the whole estate is regenerated from it on every boot.
type Config struct {
	Environments          int `json:"environments"`
	Containers            int `json:"containers"`
	VirtualMachines       int `json:"virtualMachines"`
	SystemContainers      int `json:"systemContainers"`
	KubernetesClusters    int `json:"kubernetesClusters"`
	KubernetesDeployments int `json:"kubernetesDeployments"`
	SwarmClusters         int `json:"swarmClusters"`
	SwarmNodes            int `json:"swarmNodes"`
	SwarmServices         int `json:"swarmServices"`
	HistoryDays           int `json:"historyDays"`
}

// Range is the inclusive span a Config field may take.
type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Limits bounds each Config field; they keep a demo startable on a laptop and
// are what the settings form enforces too.
type Limits struct {
	Environments          Range `json:"environments"`
	Containers            Range `json:"containers"`
	VirtualMachines       Range `json:"virtualMachines"`
	SystemContainers      Range `json:"systemContainers"`
	KubernetesClusters    Range `json:"kubernetesClusters"`
	KubernetesDeployments Range `json:"kubernetesDeployments"`
	SwarmClusters         Range `json:"swarmClusters"`
	SwarmNodes            Range `json:"swarmNodes"`
	SwarmServices         Range `json:"swarmServices"`
	HistoryDays           Range `json:"historyDays"`
}

// DefaultConfig is the estate demo mode has always shown.
func DefaultConfig() Config {
	return Config{
		Environments:          16,
		Containers:            1212,
		VirtualMachines:       120,
		SystemContainers:      80,
		KubernetesClusters:    3,
		KubernetesDeployments: 264,
		SwarmClusters:         2,
		SwarmNodes:            9,
		SwarmServices:         60,
		HistoryDays:           90,
	}
}

// DefaultLimits are the bounds Validate enforces.
func DefaultLimits() Limits {
	return Limits{
		Environments:          Range{1, 48},
		Containers:            Range{0, 6000},
		VirtualMachines:       Range{0, 600},
		SystemContainers:      Range{0, 400},
		KubernetesClusters:    Range{0, 9},
		KubernetesDeployments: Range{0, 1500},
		SwarmClusters:         Range{0, 5},
		SwarmNodes:            Range{0, 60},
		SwarmServices:         Range{0, 400},
		HistoryDays:           Range{7, 365},
	}
}

// FieldError names the Config field that is out of range, so the form can
// point at it.
type FieldError struct {
	Field string
	Range Range
}

// Error describes the field and its allowed range.
func (e *FieldError) Error() string {
	return fmt.Sprintf("%s must be between %d and %d", e.Field, e.Range.Min, e.Range.Max)
}

// Validate checks every field against DefaultLimits and the one cross-field
// rule: each swarm cluster needs at least one node.
func (c Config) Validate() error {
	l := DefaultLimits()
	checks := []struct {
		name string
		v    int
		r    Range
	}{
		{"environments", c.Environments, l.Environments},
		{"containers", c.Containers, l.Containers},
		{"virtualMachines", c.VirtualMachines, l.VirtualMachines},
		{"systemContainers", c.SystemContainers, l.SystemContainers},
		{"kubernetesClusters", c.KubernetesClusters, l.KubernetesClusters},
		{"kubernetesDeployments", c.KubernetesDeployments, l.KubernetesDeployments},
		{"swarmClusters", c.SwarmClusters, l.SwarmClusters},
		{"swarmNodes", c.SwarmNodes, l.SwarmNodes},
		{"swarmServices", c.SwarmServices, l.SwarmServices},
		{"historyDays", c.HistoryDays, l.HistoryDays},
	}
	for _, ch := range checks {
		if ch.v < ch.r.Min || ch.v > ch.r.Max {
			return &FieldError{Field: ch.name, Range: ch.r}
		}
	}
	if c.SwarmClusters > 0 && c.SwarmNodes < c.SwarmClusters {
		return &FieldError{Field: "swarmNodes", Range: Range{c.SwarmClusters, l.SwarmNodes.Max}}
	}
	return nil
}

// Effective returns the marker's config, or the defaults for a marker written
// before sizes were configurable.
func (m *Marker) Effective() Config {
	if m == nil || m.Config == nil {
		return DefaultConfig()
	}
	return *m.Config
}
