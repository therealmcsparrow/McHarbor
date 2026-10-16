// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*kubernetes.Clientset
	store   *clusterStore
}

// NewClientPool creates a Kubernetes client pool backed by the plugin's own
// cluster table.
func NewClientPool(store *clusterStore) *ClientPool {
	return &ClientPool{
		clients: make(map[string]*kubernetes.Clientset),
		store:   store,
	}
}

// Get returns a Kubernetes clientset for the given cluster ID.
func (p *ClientPool) Get(ctx context.Context, clusterID string) (*kubernetes.Clientset, error) {
	resolved, err := p.ResolveClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	clusterID = resolved

	p.mu.RLock()
	if c, ok := p.clients[clusterID]; ok {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[clusterID]; ok {
		return c, nil
	}

	conn, err := p.store.connection(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	c, err := conn.createClient()
	if err != nil {
		return nil, fmt.Errorf("creating Kubernetes client for cluster %s: %w", clusterID, err)
	}
	p.clients[clusterID] = c
	return c, nil
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
}

// Close clears all cached Kubernetes clientsets.
func (p *ClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id := range p.clients {
		delete(p.clients, id)
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
