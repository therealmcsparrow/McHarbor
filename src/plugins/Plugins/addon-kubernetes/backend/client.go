// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Sentinel connection failures. Callers match on these to tell "the operator
// has not added a cluster yet" apart from "the cluster is down", which are
// different messages for the reader and different fixes.
var (
	// ErrNoClusters — no Kubernetes cluster has been added to the plugin.
	ErrNoClusters = errors.New("no Kubernetes clusters configured")
	// ErrNotACluster — the id does not name a cluster this plugin owns.
	ErrNotACluster = errors.New("not a Kubernetes cluster")
)

// ClientPool manages Kubernetes clientsets per cluster.
//
// This used to live in the host as `core/kubernetes`, reading connection
// details out of the `environments` table. Both moved here with the plugin:
// the host no longer knows what a Kubernetes cluster is, and the credentials
// live in `plugin_kubernetes_clusters` behind the plugin's own fenced DB
// handle. What did not change is the caching contract — client-go dials
// lazily, so a cached clientset is cheap and a dead cluster is only
// discovered on the first real call.
//
// Get additionally gates every clientset on a cached /version probe, so no
// caller reads from an address that is not a Kubernetes API server.
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*kubernetes.Clientset
	trust   map[string]*apiTrust
	store   *clusterStore
	now     func() time.Time
}

// NewClientPool creates a Kubernetes client pool backed by the plugin's own
// cluster table.
func NewClientPool(store *clusterStore) *ClientPool {
	return &ClientPool{
		clients: make(map[string]*kubernetes.Clientset),
		trust:   make(map[string]*apiTrust),
		store:   store,
		now:     time.Now,
	}
}

// Get returns a Kubernetes clientset for the given cluster ID, once the cluster has
// answered a real /version probe within versionProbeTTL. An address that answers 2xx
// without a Kubernetes version document yields errAPIUnverified and no clientset.
func (p *ClientPool) Get(ctx context.Context, clusterID string) (*kubernetes.Clientset, error) {
	resolved, err := p.ResolveClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	clusterID = resolved

	cs, trust, err := p.entry(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if err := trust.ensure(ctx, func(ctx context.Context) error { return probeAPIServer(ctx, cs) }); err != nil {
		return nil, fmt.Errorf("verifying Kubernetes cluster %s: %w", clusterID, err)
	}
	return cs, nil
}

// entry returns the cached clientset and its trust, building both on first use.
func (p *ClientPool) entry(ctx context.Context, clusterID string) (*kubernetes.Clientset, *apiTrust, error) {
	p.mu.RLock()
	if c, ok := p.clients[clusterID]; ok {
		t := p.trust[clusterID]
		p.mu.RUnlock()
		return c, t, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[clusterID]; ok {
		return c, p.trust[clusterID], nil
	}

	conn, err := p.store.connection(ctx, clusterID)
	if err != nil {
		return nil, nil, err
	}
	trust := newAPITrust(p.now)
	c, err := conn.createClientFor(trust)
	if err != nil {
		return nil, nil, fmt.Errorf("creating Kubernetes client for cluster %s: %w", clusterID, err)
	}
	p.clients[clusterID] = c
	p.trust[clusterID] = trust
	return c, trust, nil
}

// ResolveClusterID resolves empty/default aliases to the default cluster.
func (p *ClientPool) ResolveClusterID(ctx context.Context, clusterID string) (string, error) {
	if clusterID != "" && clusterID != "default" {
		return clusterID, nil
	}
	return p.store.defaultClusterID(ctx)
}

// Remove drops one cached clientset. Called after a cluster's connection
// details change, so the next request dials with the new ones rather than
// reusing a clientset built from the old.
func (p *ClientPool) Remove(clusterID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, clusterID)
	delete(p.trust, clusterID)
}

// Close clears all cached Kubernetes clientsets.
func (p *ClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id := range p.clients {
		delete(p.clients, id)
	}
	for id := range p.trust {
		delete(p.trust, id)
	}
}

// pluginDB is the narrow slice of the host DB handle the store needs. Named
// so the store can be constructed in tests without a full HostContext.
type pluginDB = plugin.DBHandle

// pluginEncryption is the host's AES-256-GCM service, reached through the
// plugin contract. The ciphertext this writes is the same ciphertext the host
// used to write, which is what lets migration 147 hand credentials over
// without re-encrypting them.
type pluginEncryption = plugin.EncryptionHandle
