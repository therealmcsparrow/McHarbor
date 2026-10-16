// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package kubeapi is an in-process fake kube-apiserver for demo mode. It serves
// three simulated clusters over plain JSON so the Kubernetes plugin's typed
// client-go talks to it exactly as to a real cluster, and nothing real is dialled.
package kubeapi

import (
	"sort"
	"sync"
	"time"
)

// M is a JSON object as the apiserver writes it; rendering into maps keeps the
// package free of k8s.io types, which the backend module must not depend on.
type M = map[string]any

// ClusterInfo names one simulated cluster; Key routes requests and Name is what
// the plugin's cluster row shows.
type ClusterInfo struct{ Key, Name string }

// Sim is the simulated estate. One mutex guards every cluster because requests
// are short and mutations must see a consistent view of pods and their owners.
type Sim struct {
	mu       sync.Mutex
	clock    func() time.Time
	base     time.Time
	rng      *rng
	clusters []*cluster
	byKey    map[string]*cluster
}

type container struct {
	name, image string
	port        int32
	cpu, mem    string
	cpuLim      string
	memLim      string
	style       string
}

type node struct {
	name, uid     string
	idx           int
	created       time.Time
	controlPlane  bool
	arch, zone    string
	cpu, mem      string
	ready         bool
	unschedulable bool
	internalIP    string
	ipCounter     int
	rv            int64
	labels        map[string]string
}

type namespace struct {
	name, uid string
	created   time.Time
	labels    map[string]string
	rv        int64
}

type replicaSet struct {
	name, uid, hash string
	created         time.Time
	revision        int
	rv              int64
}

type workload struct {
	kind           string
	ns, name, uid  string
	labels         map[string]string
	annotations    map[string]string
	created        time.Time
	replicas       int32
	paused         bool
	containers     []container
	tplAnnotations map[string]string
	fault          string
	generation     int64
	tplVersion     int
	rolledVersion  int
	rs             []*replicaSet
	hold           time.Time
	serviceAccount string
	rv             int64
	jobFailed      bool
	cronJob        string
	cronJobUID     string
	completed      time.Time
	restartPolicy  string
	serviceName    string
}

type pod struct {
	ns, name, uid  string
	labels         map[string]string
	owner          *workload
	ownerRS        *replicaSet
	node           string
	created        time.Time
	readyAt        time.Time
	containers     []container
	fault          string
	restarts       int32
	ip             string
	rv             int64
	serviceAccount string
	hash           string
}

type event struct {
	ns, name, uid   string
	kind, objName   string
	objNS, objUID   string
	objAPIVersion   string
	reason, message string
	typ             string
	component, host string
	first, last     time.Time
	count           int32
	period          time.Duration
	podKey          string
	rv              int64
}

type service struct {
	ns, name, uid string
	created       time.Time
	selector      map[string]string
	port, target  int32
	portName      string
	protocol      string
	typ           string
	clusterIP     string
	lbIP          string
	headless      bool
	labels        map[string]string
	extraPorts    []servicePort
	rv            int64
}

type servicePort struct {
	name     string
	port     int32
	target   int32
	protocol string
}

type hpa struct {
	ns, name, uid string
	created       time.Time
	target        string
	min, max      int32
	util          int
	rv            int64
}

type quota struct {
	ns, uid string
	created time.Time
	rv      int64
}

type static struct {
	ns, name string
	obj      M
}

type cluster struct {
	key, name, version string
	arch               string
	net                int
	nodes              []*node
	namespaces         []*namespace
	workloads          []*workload
	wlIndex            map[string]*workload
	pods               map[string]*pod
	services           []*service
	hpas               []*hpa
	quotas             []*quota
	spec               clusterSpec
	events             []*event
	static             map[string][]static
	rv                 int64
	eventSeq           int
}

// New builds the estate for seed. now anchors every creation timestamp, so two
// Sims built from the same seed and now render byte-identical objects.
func New(seed uint64, now time.Time) *Sim {
	return NewSized(seed, now, Size{Clusters: -1, Deployments: -1})
}

func newFromSpecs(seed uint64, now time.Time, specs []clusterSpec) *Sim {
	s := &Sim{
		clock: time.Now,
		base:  now.UTC().Truncate(time.Second),
		rng:   newRNG(seed),
		byKey: map[string]*cluster{},
	}
	for _, spec := range specs {
		c := s.generate(spec)
		s.clusters = append(s.clusters, c)
		s.byKey[c.key] = c
	}
	return s
}

// Clusters lists the simulated clusters in a stable order; the first is the one
// SeedDB marks as the plugin's default.
func (s *Sim) Clusters() []ClusterInfo {
	out := make([]ClusterInfo, 0, len(s.clusters))
	for _, c := range s.clusters {
		out = append(out, ClusterInfo{Key: c.key, Name: c.name})
	}
	return out
}

// Counts reports the current size of the estate across all clusters, after any
// pending convergence, so callers see the same numbers a list would return.
func (s *Sim) Counts() (nodes, namespaces, deployments, pods int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	for _, c := range s.clusters {
		s.reconcile(c, now)
		nodes += len(c.nodes)
		namespaces += len(c.namespaces)
		for _, w := range c.workloads {
			if w.kind == "Deployment" {
				deployments++
			}
		}
		pods += len(c.pods)
	}
	return nodes, namespaces, deployments, pods
}

func (c *cluster) nextRV() int64 {
	c.rv++
	return c.rv
}

func wlKey(kind, ns, name string) string { return kind + "/" + ns + "/" + name }

func podKey(ns, name string) string { return ns + "/" + name }

func (c *cluster) sortedPods() []*pod {
	out := make([]*pod, 0, len(c.pods))
	for _, p := range c.pods {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ns != out[j].ns {
			return out[i].ns < out[j].ns
		}
		return out[i].name < out[j].name
	})
	return out
}

func (c *cluster) podsOf(w *workload) []*pod {
	out := make([]*pod, 0, w.replicas)
	for _, p := range c.pods {
		if p.owner == w {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].created.Equal(out[j].created) {
			return out[i].created.Before(out[j].created)
		}
		return out[i].name < out[j].name
	})
	return out
}

func (c *cluster) nodeByName(name string) *node {
	for _, n := range c.nodes {
		if n.name == name {
			return n
		}
	}
	return nil
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
