// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// Agent errorCodes a host-link refusal can carry. They are named here because
// the host's protocol constants are not part of the plugin surface.
const (
	rpcNameTaken        = "name_taken"
	rpcParentNotFound   = "parent_not_found"
	rpcPermissionDenied = "permission_denied"
)

// ErrInvalidHostLink marks a host interface name or IPv6 setting the service
// refused before any agent was asked.
var ErrInvalidHostLink = errors.New("invalid host interface settings")

// hostLinkNamePattern is the agent's own rule for a host interface name: a
// kernel interface name is 15 bytes at most, and a leading symbol breaks tools.
var hostLinkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)

// reservedLinkPrefixes belong to the interfaces McHarbor generates and removes
// by name, so an operator-chosen name there could be torn down with a network.
var reservedLinkPrefixes = []string{"mcharbor-", "mcht", "mchv", "mchd"}

// validateHostLinkName checks an operator-chosen host interface name. Empty is
// valid and means the generated default.
func validateHostLinkName(name string) error {
	if name == "" {
		return nil
	}
	if !hostLinkNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q must be 1-15 letters, digits, dots, dashes or underscores and start with a letter or digit",
			ErrInvalidHostLink, name)
	}
	lower := strings.ToLower(name)
	for _, prefix := range reservedLinkPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return fmt.Errorf("%w: names starting with %q are reserved for interfaces McHarbor generates",
				ErrInvalidHostLink, prefix)
		}
	}
	return nil
}

// validateHostLinkFields checks the host interface name and the IPv6 settings
// of a direct network.
func validateHostLinkFields(req NetworkCreateRequest) error {
	if req.HostLinkName != "" && !req.HostAccess {
		return fmt.Errorf("%w: a host interface name needs host access", ErrInvalidHostLink)
	}
	if err := validateHostLinkName(req.HostLinkName); err != nil {
		return err
	}
	if req.HostAccessIPv6 == "" {
		return nil
	}
	if !req.IPv6Enabled {
		return fmt.Errorf("%w: a host IPv6 address needs IPv6 turned on", ErrInvalidHostLink)
	}
	if !req.HostAccess {
		return fmt.Errorf("%w: a host IPv6 address needs host access", ErrInvalidHostLink)
	}
	ip, _, err := net.ParseCIDR(req.HostAccessIPv6)
	if err != nil || ip.To4() != nil {
		return fmt.Errorf("%w: the host IPv6 address must be written as an address and prefix, like fd00::9/64",
			ErrInvalidHostLink)
	}
	return nil
}

// hostLinkErrorCode maps an agent host-link errorCode, as the bridge exposes
// it, to the plugin's own error code and HTTP status.
func hostLinkErrorCode(rpcCode string) (status int, code string, ok bool) {
	switch rpcCode {
	case rpcNameTaken:
		return http.StatusConflict, codeHostLinkNameTaken, true
	case rpcParentNotFound:
		return http.StatusBadRequest, codeHostLinkParentMissing, true
	case rpcPermissionDenied:
		return http.StatusBadGateway, codeHostLinkPermission, true
	}
	return 0, "", false
}

// isHostLinkRefusal reports whether err carries one of the agent's host-link
// codes.
func isHostLinkRefusal(err error) bool {
	_, _, ok := hostLinkErrorCode(agentRPCCode(err))
	return ok
}
