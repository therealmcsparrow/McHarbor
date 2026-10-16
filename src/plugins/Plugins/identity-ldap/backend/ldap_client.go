// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ldapAdapter is the per-type client dispatched by the host's
// identity dispatcher for Test + FetchGroups. BeginAuth /
// HandleCallback / MetadataXML return ErrUnsupported because LDAP
// authenticates via a form-based bind-and-rebind, not a redirect dance.
// The plugin-owned POST /api/identity-ldap/auth route
// handles the actual login flow.
type ldapAdapter struct {
	plugin *IdentityLdapPlugin
}

// ldapConfigFromMap pulls the typed config fields out of the map
// the host dispatcher hands us. Missing fields fall back to the
// defaults defined on the plugin struct.
func (a *ldapAdapter) ldapConfigFromMap(config map[string]string) ldapProviderConfig {
	return ldapConfigFromMap(config)
}

// ldapConfigFromMap is the adapter-free form the sign-in route uses on the map
// host.Identity returns, so Test and sign-in read the same keys the same way.
func ldapConfigFromMap(config map[string]string) ldapProviderConfig {
	return ldapProviderConfig{
		URL:            strings.TrimSpace(config["url"]),
		BindDN:         config["bind_dn"],
		BindPassword:   config["bind_password"],
		UserBaseDN:     config["user_base_dn"],
		UserFilter:     config["user_filter"],
		UserAttr:       config["user_attr"],
		EmailAttr:      config["email_attr"],
		NameAttr:       config["name_attr"],
		StartTLS:       config["start_tls"] == "true",
		SkipTLSVerify:  config["skip_tls_verify"] == "true",
		GroupBaseDN:    config["group_base_dn"],
		GroupFilter:    config["group_filter"],
		GroupAttr:      config["group_attr"],
	}
}

// ldapDialTimeout bounds the TCP connect, which the ldap package otherwise leaves
// to the OS, so an unreachable directory fails a sign-in instead of holding it.
const ldapDialTimeout = 10 * time.Second

// ldapProviderConfig mirrors the ConfigFieldDecl keys declared in
// main.go. Defaults are applied for fields that the host didn't pass.
type ldapProviderConfig struct {
	URL           string
	BindDN        string
	BindPassword  string
	UserBaseDN    string
	UserFilter    string
	UserAttr      string
	EmailAttr     string
	NameAttr      string
	StartTLS      bool
	SkipTLSVerify bool
	GroupBaseDN   string
	GroupFilter   string
	GroupAttr     string
}

func (c ldapProviderConfig) withDefaults() ldapProviderConfig {
	if c.UserFilter == "" {
		c.UserFilter = "(uid=%s)"
	}
	if c.UserAttr == "" {
		c.UserAttr = "uid"
	}
	if c.EmailAttr == "" {
		c.EmailAttr = "mail"
	}
	if c.NameAttr == "" {
		c.NameAttr = "cn"
	}
	if c.GroupFilter == "" {
		c.GroupFilter = "(|(member=%s)(uniqueMember=%s))"
	}
	if c.GroupAttr == "" {
		c.GroupAttr = "cn"
	}
	return c
}

// dial opens an LDAP connection using the provider's url + TLS settings.
// Caller is responsible for calling Close.
func (c ldapProviderConfig) dial() (*ldap.Conn, error) {
	if c.URL == "" {
		return nil, errors.New("ldap url is required")
	}
	dialOpts := []ldap.DialOpt{ldap.DialWithDialer(&net.Dialer{Timeout: ldapDialTimeout})}
	isLDAPS := strings.HasPrefix(c.URL, "ldaps://")
	if isLDAPS {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if c.SkipTLSVerify {
			tlsCfg.InsecureSkipVerify = true
		}
		dialOpts = append(dialOpts, ldap.DialWithTLSConfig(tlsCfg))
	}
	conn, err := ldap.DialURL(c.URL, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %w", err)
	}
	if c.StartTLS && !isLDAPS {
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: extractHost(c.URL),
		}
		if c.SkipTLSVerify {
			tlsCfg.InsecureSkipVerify = true
		}
		if err := conn.StartTLS(tlsCfg); err != nil {
			conn.Close()
			return nil, fmt.Errorf("ldap starttls: %w", err)
		}
	}
	return conn, nil
}

// groupNameFromDN reduces a group DN to its cn, which is what the host's group
// mapper keys on. Best-effort: a DN without a cn is returned whole.
func groupNameFromDN(dn string) string {
	idx := strings.Index(strings.ToUpper(dn), "CN=")
	if idx < 0 {
		return dn
	}
	rest := dn[idx+3:]
	if comma := strings.Index(rest, ","); comma > 0 {
		return rest[:comma]
	}
	return rest
}

func extractHost(rawURL string) string {
	stripped := strings.TrimPrefix(rawURL, "ldap://")
	stripped = strings.TrimPrefix(stripped, "ldaps://")
	if idx := strings.Index(stripped, ":"); idx > 0 {
		return stripped[:idx]
	}
	if idx := strings.Index(stripped, "/"); idx > 0 {
		return stripped[:idx]
	}
	return stripped
}

// --- IdentityAdapterClient implementation ------------------------
//
// Only Test and FetchGroups are implemented. BeginAuth /
// HandleCallback / MetadataXML return ErrUnsupported because LDAP
// has no SP metadata document and no OAuth/SAML redirect dance.
// The plugin-owned POST /api/identity-ldap/auth route
// (registered in main.go) handles the form-post login flow directly.

func (a *ldapAdapter) Test(ctx context.Context, config map[string]string) error {
	prov := a.ldapConfigFromMap(config).withDefaults()
	if prov.URL == "" {
		return errors.New("ldap url is required")
	}
	if prov.UserBaseDN == "" {
		return errors.New("user_base_dn is required")
	}
	conn, err := prov.dial()
	if err != nil {
		return err
	}
	defer conn.Close()
	if prov.BindDN != "" {
		if err := conn.Bind(prov.BindDN, prov.BindPassword); err != nil {
			return fmt.Errorf("service-account bind: %w", err)
		}
	}
	// Probe: search for a single user with an absurd filter. We
	// expect zero results; the goal is just to verify the bind +
	// search path round-trips.
	probe := ldap.NewSearchRequest(
		prov.UserBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		1, 10, false, "(uid=mcharbor-probe-nonexistent-user)",
		[]string{"dn"}, nil,
	)
	if _, err := conn.Search(probe); err != nil {
		return fmt.Errorf("ldap probe search: %w", err)
	}
	return nil
}

func (a *ldapAdapter) BeginAuth(_ context.Context, _ map[string]string, _ plugin.IdentityAuthRequest) (*plugin.IdentityAuthResult, error) {
	return nil, fmt.Errorf("identity-ldap: %w (LDAP uses form-based authentication)", plugin.ErrUnsupported)
}

func (a *ldapAdapter) HandleCallback(_ context.Context, _ map[string]string, _ plugin.IdentityCallbackRequest) (*plugin.IdentityCallbackResult, error) {
	return nil, fmt.Errorf("identity-ldap: %w (LDAP does not implement redirect dance)", plugin.ErrUnsupported)
}

func (a *ldapAdapter) FetchGroups(ctx context.Context, config map[string]string) ([]plugin.IdentityGroup, error) {
	prov := a.ldapConfigFromMap(config).withDefaults()
	if prov.URL == "" {
		return nil, errors.New("ldap url is required")
	}
	if prov.UserBaseDN == "" {
		return nil, errors.New("user_base_dn is required for fetching groups")
	}
	groupBase := prov.GroupBaseDN
	if groupBase == "" {
		groupBase = prov.UserBaseDN
	}
	conn, err := prov.dial()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if prov.BindDN != "" {
		if err := conn.Bind(prov.BindDN, prov.BindPassword); err != nil {
			return nil, fmt.Errorf("service-account bind: %w", err)
		}
	}

	// Walk every user that matches prov.UserFilter, then collect the
	// groups each one belongs to via memberOf. The host group-mapper
	// expects IdentityGroup{ID, Name, Description} entries.
	search := ldap.NewSearchRequest(
		prov.UserBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 30, false,
		fmt.Sprintf("(&(objectClass=%s)(%s=*))", "person", prov.UserAttr),
		[]string{"dn", "memberOf"},
		nil,
	)
	res, err := conn.Search(search)
	if err != nil {
		return nil, fmt.Errorf("ldap user search: %w", err)
	}

	seen := make(map[string]struct{})
	out := make([]plugin.IdentityGroup, 0, len(res.Entries))
	for _, entry := range res.Entries {
		for _, member := range entry.GetAttributeValues("memberOf") {
			if _, dup := seen[member]; dup {
				continue
			}
			seen[member] = struct{}{}
			out = append(out, plugin.IdentityGroup{
				ID:   member,
				Name: groupNameFromDN(member),
			})
		}
	}
	return out, nil
}

func (a *ldapAdapter) MetadataXML(_ context.Context, _ map[string]string, _ string) ([]byte, error) {
	return nil, fmt.Errorf("identity-ldap: %w (LDAP has no SP metadata)", plugin.ErrUnsupported)
}