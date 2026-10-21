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
	return c.createClientFor(nil)
}

// createClientFor builds a clientset whose transport consults trust before it lets an
// upstream error body through; the pool passes the trust its /version probe fills in.
func (c *Connection) createClientFor(trust *apiTrust) (*kubernetes.Clientset, error) {
	cfg, err := c.restConfigFor(trust)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (c *Connection) restConfig() (*rest.Config, error) {
	return c.restConfigFor(nil)
}

func (c *Connection) restConfigFor(trust *apiTrust) (*rest.Config, error) {
	if c.Kubeconfig != "" {
		// The guard runs before the build because tokenFile is read inside it, and rows
		// from migration 071 or a restored backup never passed the save-time check.
		if err := checkClusterCredentials(c.Kubeconfig); err != nil {
			return nil, err
		}
		cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(c.Kubeconfig))
		if err != nil {
			return nil, fmt.Errorf("building config from kubeconfig: %w", err)
		}
		if err := hardenRESTConfig(cfg); err != nil {
			return nil, err
		}
		applyDestinationPolicy(cfg, trust)
		return cfg, nil
	}
	if c.ServerURL != "" && c.BearerToken != "" {
		cfg := &rest.Config{Host: c.ServerURL, BearerToken: c.BearerToken}
		if c.CACert != "" {
			cfg.TLSClientConfig.CAData = []byte(c.CACert)
		}
		applyDestinationPolicy(cfg, trust)
		return cfg, nil
	}
	return nil, fmt.Errorf("cluster has no kubeconfig or bearer-token connection details")
}
