// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubernetes

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ConnectionType describes how a Kubernetes cluster is connected.
type ConnectionType string

const (
	ConnKubeconfig  ConnectionType = "kubeconfig"
	ConnBearerToken ConnectionType = "bearer"
	ConnInCluster   ConnectionType = "in-cluster"
)

// Connection holds the resolved Kubernetes connection parameters.
type Connection struct {
	Type        ConnectionType
	Kubeconfig  string // raw kubeconfig YAML blob
	ServerURL   string // e.g. https://k8s.example.com:6443
	BearerToken string
	CACert      string // PEM-encoded CA certificate
	Namespace   string // default namespace
}

// createClientset builds a kubernetes.Clientset from the connection config.
func (c *Connection) createClientset() (*kubernetes.Clientset, error) {
	cfg, err := c.restConfig()
	if err != nil {
		return nil, fmt.Errorf("building rest config: %w", err)
	}
	return kubernetes.NewForConfig(cfg)
}

// restConfig builds a *rest.Config from the connection settings.
func (c *Connection) restConfig() (*rest.Config, error) {
	switch c.Type {
	case ConnKubeconfig:
		return clientcmd.RESTConfigFromKubeConfig([]byte(c.Kubeconfig))

	case ConnBearerToken:
		cfg := &rest.Config{
			Host:        c.ServerURL,
			BearerToken: c.BearerToken,
		}
		if c.CACert != "" {
			cfg.TLSClientConfig = rest.TLSClientConfig{
				CAData: []byte(c.CACert),
			}
		} else {
			cfg.TLSClientConfig = rest.TLSClientConfig{
				Insecure: true,
			}
		}
		return cfg, nil

	case ConnInCluster:
		return rest.InClusterConfig()

	default:
		return nil, fmt.Errorf("unsupported kubernetes connection type: %s", c.Type)
	}
}
