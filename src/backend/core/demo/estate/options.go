// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"math"
	"strconv"
	"time"
)

// Options sizes the estate. Zero values keep the built-in estate, so the
// defaults stay what demo mode has always shown.
type Options struct {
	Environments int
	Containers   int
}

// DefaultEnvironments is how many hosts the built-in estate has.
func DefaultEnvironments() int { return len(hostSpecs) }

// DefaultContainers is roughly how many containers the built-in estate targets.
func DefaultContainers() int {
	total := 0
	for _, s := range hostSpecs {
		total += s.target
	}
	return total
}

// priority is the order hosts are chosen in when fewer than the full set are
// asked for, so a small estate still has production, homelab, dev and edge.
var priority = []string{
	"prod-eu-1", "homelab", "dev", "staging-eu", "prod-us-1", "edge-store-01", "data-platform", "ci-runners",
	"dmz", "prod-eu-2", "staging-us", "edge-store-02", "prod-eu-3", "prod-us-2", "edge-store-03", "edge-store-04",
}

func selectSpecs(n int) []hostSpec {
	if n <= 0 || n == len(hostSpecs) {
		return append([]hostSpec(nil), hostSpecs...)
	}
	byKey := make(map[string]hostSpec, len(hostSpecs))
	for _, s := range hostSpecs {
		byKey[s.key] = s
	}
	out := make([]hostSpec, 0, n)
	for i := 0; i < n; i++ {
		base := byKey[priority[i%len(priority)]]
		if round := i / len(priority); round > 0 {
			suffix := strconv.Itoa(round + 1)
			base.key += "-" + suffix
			base.name += " " + suffix
		}
		out = append(out, base)
	}
	return out
}

// BuildWith creates the estate sized by opts from seed. A default Options gives
// exactly what Build does.
func BuildWith(seed uint64, now time.Time, opts Options) *Estate {
	specs := selectSpecs(opts.Environments)
	if opts.Containers > 0 {
		sum := 0
		for _, s := range specs {
			sum += s.target
		}
		factor := float64(opts.Containers) / float64(max(sum, 1))
		for i := range specs {
			specs[i].target = max(1, int(math.Round(float64(specs[i].target)*factor)))
		}
	}
	e := &Estate{byKey: make(map[string]*Host)}
	for i, spec := range specs {
		h := buildHost(spec, seed+uint64(i)*7919, now)
		e.Hosts = append(e.Hosts, h)
		e.byKey[h.Key] = h
	}
	return e
}

// HostSpec describes a host added after the estate is built, such as a Swarm
// manager; Stacks names templates from the built-in catalogue.
type HostSpec struct {
	Key        string
	Name       string
	Region     string
	Role       string
	CPUs       int
	MemGB      int
	DiskTB     int
	Distro     string
	Arch       string
	Compute    bool
	Containers int
	Stacks     []string
}

// AddHost builds a host from spec and adds it to the estate. It must be called
// before the estate is shared, since the host list is not guarded.
func (e *Estate) AddHost(spec HostSpec, seed uint64, now time.Time) *Host {
	if spec.Distro == "" {
		spec.Distro = "ubuntu"
	}
	if spec.Arch == "" {
		spec.Arch = "x86_64"
	}
	h := buildHost(hostSpec{
		key: spec.Key, name: spec.Name, region: spec.Region, role: spec.Role, target: spec.Containers,
		cpus: max(spec.CPUs, 1), memGB: max(spec.MemGB, 1), diskTB: max(spec.DiskTB, 1), distro: spec.Distro,
		compute: spec.Compute, arch: spec.Arch, stacks: spec.Stacks,
	}, seed^hashString(spec.Key), now)
	e.Hosts = append(e.Hosts, h)
	e.byKey[h.Key] = h
	return h
}
