// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A .vv file for remote-viewer.
//
// The browser's SPICE client needs nothing from here: it opens console
// sessions of kind "spice" through the host's console gateway, one per SPICE
// channel, exactly as noVNC opens a "vnc" one. This is the other path, for an
// operator who prefers the native viewer: the agent opens a short-lived port
// guarded by a one-time password and this file tells remote-viewer where it is.

// ErrSpiceUnavailable means the guest's host cannot serve SPICE: its QEMU has
// no SPICE, or its agent predates it.
var ErrSpiceUnavailable = errors.New("this guest's host cannot serve a SPICE console")

// SpiceFile is a rendered .vv file and the name to save it under.
type SpiceFile struct {
	Name    string
	Content []byte
}

// SpiceFile mints a remote-viewer ticket and renders the .vv file for it.
//
// host overrides the address written into the file, for an operator who
// reaches the guest's host by a name the agent cannot know; empty uses the
// first address the agent reports.
func (s *Service) SpiceFile(ctx context.Context, vmID, host string) (SpiceFile, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return SpiceFile{}, err
	}
	if !vm.Running() {
		return SpiceFile{}, ErrNotRunning
	}
	if host != "" && !validMigrationAddress(host) {
		return SpiceFile{}, invalid(codeBadRequest, "that is not a host name or address")
	}
	if !s.SupportsFeature(ctx, vm.EnvironmentID, FeatureSPICE) {
		return SpiceFile{}, ErrSpiceUnavailable
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMSpiceTicket, agentPayload{VMID: vm.ID})
	if err != nil {
		return SpiceFile{}, err
	}
	ticket := reply.SpiceTicket
	if ticket == nil || ticket.Port <= 0 || ticket.Password == "" {
		return SpiceFile{}, ErrSpiceUnavailable
	}
	if host == "" {
		if len(ticket.Addresses) == 0 {
			return SpiceFile{}, fmt.Errorf("%w: the host reported no address a viewer could reach", ErrSpiceUnavailable)
		}
		host = ticket.Addresses[0]
	}
	return SpiceFile{
		Name:    safeFileName(vm.Name) + ".vv",
		Content: renderVV(host, ticket.Port, ticket.Password, vm.Name),
	}, nil
}

// renderVV writes the virt-viewer connection file. delete-this-file asks
// remote-viewer to remove it once read, since it holds a password.
func renderVV(host string, port int, password, title string) []byte {
	var b strings.Builder
	b.WriteString("[virt-viewer]\n")
	b.WriteString("type=spice\n")
	b.WriteString("host=" + vvValue(host) + "\n")
	b.WriteString("port=" + strconv.Itoa(port) + "\n")
	b.WriteString("password=" + vvValue(password) + "\n")
	b.WriteString("title=" + vvValue(title) + " - McHarbor\n")
	b.WriteString("delete-this-file=1\n")
	b.WriteString("secure-attention=ctrl+alt+end\n")
	b.WriteString("release-cursor=shift+f12\n")
	b.WriteString("toggle-fullscreen=shift+f11\n")
	return []byte(b.String())
}

// vvValue keeps a value on its own line: a newline in a guest name must not
// become a second key in the file.
func vvValue(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

// safeFileName reduces a guest name to characters every OS accepts in a file
// name.
func safeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "console"
	}
	return b.String()
}
