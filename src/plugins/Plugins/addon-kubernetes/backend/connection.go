// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Connection is a resolved, decrypted cluster connection.
//
// It exists for exactly as long as one client build takes. Nothing holds a
// Connection: the ClientPool caches the *clientset*, never the credentials
// that produced it, so a decrypted kubeconfig is never resident longer than
// it has to be.
type Connection struct {
	Kubeconfig  string
	ServerURL   string
	BearerToken string
	CACert      string
}

func (c *Connection) createClient() (*kubernetes.Clientset, error) {
	cfg, err := c.restConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (c *Connection) restConfig() (*rest.Config, error) {
	if c.Kubeconfig != "" {
		cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.Kubeconfig))
		if err != nil {
			return nil, fmt.Errorf("building config from kubeconfig: %w", err)
		}
		return cfg, nil
	}
	if c.ServerURL != "" && c.BearerToken != "" {
		cfg := &rest.Config{Host: c.ServerURL, BearerToken: c.BearerToken}
		if c.CACert != "" {
			cfg.TLSClientConfig.CAData = []byte(c.CACert)
		}
		return cfg, nil
	}
	return nil, fmt.Errorf("cluster has no kubeconfig or bearer-token connection details")
}
