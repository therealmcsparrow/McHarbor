// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/clusterinfo"
)

// Service is looked up as "cluster" by reporting modules that know only
// clusterinfo, so it must keep satisfying Provider.
var _ clusterinfo.Provider = (*Service)(nil)

// Info returns the read-only readiness and membership view that health,
// compliance and support report on without importing this module.
func (s *Service) Info() clusterinfo.Readiness { return infoView{s: s} }

// infoView converts this module's types to clusterinfo's at the boundary, so
// the OpenAPI schemas keep naming Cluster.ReadyCheck and Cluster.Instance.
type infoView struct{ s *Service }

func (v infoView) Ready(ctx context.Context) (bool, []clusterinfo.ReadyCheck) {
	ok, checks := v.s.Ready(ctx)
	out := make([]clusterinfo.ReadyCheck, len(checks))
	for i, c := range checks {
		out[i] = clusterinfo.ReadyCheck(c)
	}
	return ok, out
}

func (v infoView) Membership() clusterinfo.Membership {
	reg := v.s.Registry()
	if reg == nil {
		return nil
	}
	return membershipView{Registry: reg}
}

// membershipView is a Registry whose List answers in clusterinfo.Instance.
type membershipView struct{ *Registry }

func (m membershipView) List(ctx context.Context) ([]clusterinfo.Instance, error) {
	nodes, err := m.Registry.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]clusterinfo.Instance, len(nodes))
	for i, n := range nodes {
		out[i] = clusterinfo.Instance(n)
	}
	return out, nil
}
