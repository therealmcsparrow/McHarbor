// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubernetes

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	"k8s.io/client-go/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EncryptionService decrypts sensitive fields stored in the database.
type EncryptionService interface {
	Decrypt(encrypted string) (string, error)
}

// ClientPool manages Kubernetes clientsets per environment.
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*kubernetes.Clientset
	db      *sql.DB
	enc     EncryptionService
	logger  *slog.Logger
}

// NewClientPool creates a new Kubernetes client pool.
func NewClientPool(db *sql.DB, enc EncryptionService, logger *slog.Logger) *ClientPool {
	return &ClientPool{
		clients: make(map[string]*kubernetes.Clientset),
		db:      db,
		enc:     enc,
		logger:  logger,
	}
}

// Get returns a Kubernetes clientset for the given environment ID.
func (p *ClientPool) Get(envID string) (*kubernetes.Clientset, error) {
	p.mu.RLock()
	if c, ok := p.clients[envID]; ok {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if c, ok := p.clients[envID]; ok {
		return c, nil
	}

	conn, err := p.resolveConnection(envID)
	if err != nil {
		return nil, err
	}

	cs, err := conn.createClientset()
	if err != nil {
		return nil, fmt.Errorf("creating Kubernetes client for env %s: %w", envID, err)
	}

	p.clients[envID] = cs
	p.logger.Info("kubernetes client created", "env", envID)
	return cs, nil
}

// Remove removes a client from the pool.
func (p *ClientPool) Remove(envID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, envID)
}

// Ping checks if a Kubernetes cluster is reachable by listing server version.
func (p *ClientPool) Ping(ctx context.Context, envID string) (string, error) {
	cs, err := p.Get(envID)
	if err != nil {
		return "", err
	}

	info, err := cs.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("pinging kubernetes cluster: %w", err)
	}
	return info.GitVersion, nil
}

// ListNamespaces returns all namespace names for the given environment.
func (p *ClientPool) ListNamespaces(ctx context.Context, envID string) ([]string, error) {
	cs, err := p.Get(envID)
	if err != nil {
		return nil, err
	}

	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces: %w", err)
	}

	names := make([]string, len(nsList.Items))
	for i, ns := range nsList.Items {
		names[i] = ns.Name
	}
	return names, nil
}

// Close clears all clients from the pool.
func (p *ClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id := range p.clients {
		delete(p.clients, id)
	}
}

// resolveConnection resolves the Kubernetes connection for the given environment ID.
func (p *ClientPool) resolveConnection(envID string) (*Connection, error) {
	var kubeconfig, serverURL, bearerToken, caCert, namespace sql.NullString

	err := p.db.QueryRow(
		`SELECT kubeconfig, k8s_server_url, k8s_bearer_token, k8s_ca_cert, k8s_namespace
		 FROM environments WHERE id = ? AND orchestrator_type = 'kubernetes'`, envID,
	).Scan(&kubeconfig, &serverURL, &bearerToken, &caCert, &namespace)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("kubernetes environment not found: %s", envID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying kubernetes environment: %w", err)
	}

	conn := &Connection{
		Namespace: "default",
	}

	if namespace.Valid && namespace.String != "" {
		conn.Namespace = namespace.String
	}

	// Determine connection type and decrypt sensitive fields
	if kubeconfig.Valid && kubeconfig.String != "" {
		decrypted, err := p.enc.Decrypt(kubeconfig.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting kubeconfig: %w", err)
		}
		conn.Type = ConnKubeconfig
		conn.Kubeconfig = decrypted
	} else if serverURL.Valid && serverURL.String != "" {
		conn.Type = ConnBearerToken
		conn.ServerURL = serverURL.String

		if bearerToken.Valid && bearerToken.String != "" {
			decrypted, err := p.enc.Decrypt(bearerToken.String)
			if err != nil {
				return nil, fmt.Errorf("decrypting bearer token: %w", err)
			}
			conn.BearerToken = decrypted
		}

		if caCert.Valid && caCert.String != "" {
			decrypted, err := p.enc.Decrypt(caCert.String)
			if err != nil {
				return nil, fmt.Errorf("decrypting CA cert: %w", err)
			}
			conn.CACert = decrypted
		}
	} else {
		conn.Type = ConnInCluster
	}

	return conn, nil
}
