// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"sort"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// errKubeconfigUnsafe marks a kubeconfig that would make McHarbor run a program or read a
// host file on its own machine; the handler maps it to a 400 with the cluster-invalid code.
var errKubeconfigUnsafe = errors.New("kubernetes: kubeconfig carries a credential that is not inline data")

// checkClusterCredentials refuses a kubeconfig that names a credential plugin or a host
// file. It inspects every user and cluster entry, not just the current context, because
// switching the context later must not turn a stored entry live. Messages name the entry
// and the field, never a value, so the cause is safe to return as `detail`.
func checkClusterCredentials(kubeconfig string) error {
	cfg, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		// The parser's text can quote the document, which may hold a secret.
		return fmt.Errorf("%w: the kubeconfig cannot be parsed", errKubeconfigUnsafe)
	}
	for _, name := range sortedKeys(cfg.AuthInfos) {
		if field := unsafeUserField(cfg.AuthInfos[name]); field != "" {
			return fmt.Errorf("%w: user %q sets %s", errKubeconfigUnsafe, name, field)
		}
	}
	for _, name := range sortedKeys(cfg.Clusters) {
		if field := unsafeClusterField(cfg.Clusters[name]); field != "" {
			return fmt.Errorf("%w: cluster %q sets %s", errKubeconfigUnsafe, name, field)
		}
	}
	return nil
}

// guardKubeconfigInput applies checkClusterCredentials to a save request's optional
// kubeconfig; an absent or empty value is left to the callers' own presence rules.
func guardKubeconfigInput(kubeconfig *string) error {
	if kubeconfig == nil || *kubeconfig == "" {
		return nil
	}
	return checkClusterCredentials(*kubeconfig)
}

func unsafeUserField(u *api.AuthInfo) string {
	switch {
	case u == nil:
		return ""
	case u.Exec != nil:
		return "exec"
	case u.AuthProvider != nil:
		return "auth-provider"
	case u.TokenFile != "":
		return "tokenFile"
	case u.ClientCertificate != "":
		return "client-certificate"
	case u.ClientKey != "":
		return "client-key"
	}
	return ""
}

func unsafeClusterField(c *api.Cluster) string {
	switch {
	case c == nil:
		return ""
	case c.CertificateAuthority != "":
		return "certificate-authority"
	case c.ProxyURL != "":
		return "proxy-url"
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// hardenRESTConfig is the backstop behind checkClusterCredentials: a config that still
// carries a credential plugin or file reference is refused, and any exec provider is set to
// deny-all first so a future code path that skips the refusal still cannot start a process.
func hardenRESTConfig(cfg *rest.Config) error {
	if cfg.ExecProvider != nil {
		cfg.ExecProvider.PluginPolicy = api.PluginPolicy{PolicyType: api.PluginPolicyDenyAll}
		return fmt.Errorf("%w: exec provider", errKubeconfigUnsafe)
	}
	switch {
	case cfg.AuthProvider != nil:
		return fmt.Errorf("%w: auth-provider", errKubeconfigUnsafe)
	case cfg.BearerTokenFile != "":
		return fmt.Errorf("%w: tokenFile", errKubeconfigUnsafe)
	case cfg.CertFile != "":
		return fmt.Errorf("%w: client-certificate", errKubeconfigUnsafe)
	case cfg.KeyFile != "":
		return fmt.Errorf("%w: client-key", errKubeconfigUnsafe)
	case cfg.CAFile != "":
		return fmt.Errorf("%w: certificate-authority", errKubeconfigUnsafe)
	}
	return nil
}
