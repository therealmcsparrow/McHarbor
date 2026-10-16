// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"sort"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"
)

// nodeLabel is the one constant label every exported series carries in a cluster.
// It has a single value per process, so it adds no cardinality within a scrape.
const nodeLabel = "node_id"

var nodeIdentity atomic.Pointer[string]

// SetNodeID labels every series this process exports with its cluster node id, so
// scrapes of several nodes behind one job stay apart; the label is added at gather
// time because the metric families are built before the id is known.
func SetNodeID(id string) {
	nodeIdentity.Store(&id)
}

// NodeGatherer wraps g so each metric carries node_id when SetNodeID was called.
func NodeGatherer(g prometheus.Gatherer) prometheus.Gatherer {
	return nodeGatherer{inner: g}
}

type nodeGatherer struct{ inner prometheus.Gatherer }

// Gather implements prometheus.Gatherer.
func (n nodeGatherer) Gather() ([]*dto.MetricFamily, error) {
	families, err := n.inner.Gather()
	p := nodeIdentity.Load()
	if p == nil || *p == "" {
		return families, err
	}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			if hasLabel(m, nodeLabel) {
				continue
			}
			m.Label = append(m.Label, &dto.LabelPair{Name: proto.String(nodeLabel), Value: proto.String(*p)})
			sort.Slice(m.Label, func(i, j int) bool { return m.Label[i].GetName() < m.Label[j].GetName() })
		}
	}
	return families, err
}

func hasLabel(m *dto.Metric, name string) bool {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return true
		}
	}
	return false
}
