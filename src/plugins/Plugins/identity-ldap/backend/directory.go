// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// ldapOperationTimeout bounds every request on an open connection, so a directory
// that accepts the socket and then stalls cannot hold a sign-in open.
const ldapOperationTimeout = 30 * time.Second

var (
	// errDirectoryUserNotFound is a login name that matched no entry, or more than
	// one; both are a failed sign-in, never a directory fault.
	errDirectoryUserNotFound = errors.New("identity-ldap: user not found")
	// errDirectoryInvalidCredentials is a user bind the directory rejected as a
	// wrong password (result code 49).
	errDirectoryInvalidCredentials = errors.New("identity-ldap: invalid credentials")
)

// ldapEntry is what the directory says about one login name.
type ldapEntry struct {
	DN       string
	Username string
	Email    string
	Name     string
	Groups   []string
}

// ldapDirectory opens sessions against one provider. The sign-in route depends on
// it rather than on *ldap.Conn so the flow's ordering is testable without a server.
type ldapDirectory interface {
	Open(prov ldapProviderConfig) (ldapSession, error)
}

// ldapSession is one connection's worth of sign-in work: find the entry as the
// service account, then bind as the user.
type ldapSession interface {
	FindUser(username string) (*ldapEntry, error)
	Authenticate(dn, password string) error
	Close()
}

// netDirectory is the ldapDirectory that talks to a real server.
type netDirectory struct{}

// Open dials the provider and applies the per-request timeout.
func (netDirectory) Open(prov ldapProviderConfig) (ldapSession, error) {
	conn, err := prov.dial()
	if err != nil {
		return nil, err
	}
	conn.SetTimeout(ldapOperationTimeout)
	return &netSession{conn: conn, prov: prov}, nil
}

// netSession is an ldapSession over one *ldap.Conn.
type netSession struct {
	conn *ldap.Conn
	prov ldapProviderConfig
}

// FindUser binds as the service account (anonymous when none is configured) and
// resolves username to exactly one entry, with its groups.
func (s *netSession) FindUser(username string) (*ldapEntry, error) {
	if s.prov.BindDN != "" {
		if err := s.conn.Bind(s.prov.BindDN, s.prov.BindPassword); err != nil {
			return nil, fmt.Errorf("service-account bind: %w", err)
		}
	}
	filter := fmt.Sprintf(s.prov.UserFilter, ldap.EscapeFilter(username))
	attrs := []string{"dn", s.prov.UserAttr, s.prov.EmailAttr, s.prov.NameAttr, "memberOf"}
	res, err := s.conn.Search(ldap.NewSearchRequest(
		s.prov.UserBaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, int(ldapOperationTimeout.Seconds()), false, filter, attrs, nil,
	))
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			return nil, errDirectoryUserNotFound
		}
		return nil, fmt.Errorf("ldap search: %w", err)
	}
	if len(res.Entries) != 1 {
		return nil, errDirectoryUserNotFound
	}
	entry := res.Entries[0]
	out := &ldapEntry{
		DN:       entry.DN,
		Username: entry.GetAttributeValue(s.prov.UserAttr),
		Email:    entry.GetAttributeValue(s.prov.EmailAttr),
		Name:     entry.GetAttributeValue(s.prov.NameAttr),
	}
	for _, member := range entry.GetAttributeValues("memberOf") {
		out.Groups = append(out.Groups, groupNameFromDN(member))
	}
	if len(out.Groups) == 0 {
		out.Groups = s.searchGroups(entry.DN)
	}
	return out, nil
}

// searchGroups finds the groups naming dn as a member, for directories without a
// memberOf overlay. A failure yields no groups rather than a failed sign-in,
// because group sync is a side effect of signing in, not a condition of it.
func (s *netSession) searchGroups(dn string) []string {
	base := s.prov.GroupBaseDN
	if base == "" {
		base = s.prov.UserBaseDN
	}
	filter := strings.ReplaceAll(s.prov.GroupFilter, "%s", ldap.EscapeFilter(dn))
	res, err := s.conn.Search(ldap.NewSearchRequest(
		base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, int(ldapOperationTimeout.Seconds()), false, filter, []string{s.prov.GroupAttr}, nil,
	))
	if err != nil {
		return nil
	}
	groups := make([]string, 0, len(res.Entries))
	for _, entry := range res.Entries {
		if name := entry.GetAttributeValue(s.prov.GroupAttr); name != "" {
			groups = append(groups, name)
		}
	}
	return groups
}

// Authenticate binds as dn with the user's password, telling a rejected password
// apart from a directory fault so only the first counts toward lockout.
func (s *netSession) Authenticate(dn, password string) error {
	if password == "" {
		return errDirectoryInvalidCredentials
	}
	if err := s.conn.Bind(dn, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return errDirectoryInvalidCredentials
		}
		return fmt.Errorf("user bind: %w", err)
	}
	return nil
}

// Close releases the connection.
func (s *netSession) Close() {
	s.conn.Close()
}
