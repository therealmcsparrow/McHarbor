// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"math"
	"strconv"
	"time"
)

// Size sets how many clusters there are and roughly how many application
// deployments they hold in total. A negative value keeps the built-in amount.
type Size struct {
	Clusters    int
	Deployments int
}

// DefaultClusters is how many clusters the built-in estate has.
func DefaultClusters() int { return len(clusterSpecs) }

// DefaultDeployments is how many application deployments the built-in
// clusters hold, not counting the system workloads every cluster carries.
func DefaultDeployments() int { return appCount(clusterSpecs) }

func appCount(specs []clusterSpec) int {
	n := 0
	for _, spec := range specs {
		for _, ns := range spec.apps {
			n += ns.apps
		}
	}
	return n
}

// NewSized builds the estate for seed with size: the first clusters of the
// built-in set, cloned with a numbered suffix past it, and application
// deployments scaled toward the requested total.
func NewSized(seed uint64, now time.Time, size Size) *Sim {
	specs := append([]clusterSpec(nil), clusterSpecs...)
	if size.Clusters >= 0 {
		specs = specs[:0]
		for i := 0; i < size.Clusters; i++ {
			spec := clusterSpecs[i%len(clusterSpecs)]
			if round := i / len(clusterSpecs); round > 0 {
				suffix := strconv.Itoa(round + 1)
				spec.key += "-" + suffix
				spec.name += "-" + suffix
				spec.domain = spec.key + ".demo.example.com"
				spec.notReady = ""
			}
			spec.apps = append([]nsSpec(nil), spec.apps...)
			specs = append(specs, spec)
		}
	}
	if size.Deployments >= 0 {
		base := appCount(specs)
		if base > 0 {
			factor := float64(size.Deployments) / float64(base)
			for i := range specs {
				for j := range specs[i].apps {
					specs[i].apps[j].apps = int(math.Round(float64(specs[i].apps[j].apps) * factor))
				}
			}
		}
	}
	return newFromSpecs(seed, now, specs)
}
