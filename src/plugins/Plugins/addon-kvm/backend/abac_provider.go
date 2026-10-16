// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"

	abacspec "github.com/therealmcsparrow/mcharbor/core/plugin/specs/abac"
)

// ABACResourceTypes declares virtual machines so attribute-based policies can target
// them and the host can resolve which machine a /api/vms/{id} request acts on.
func (p *KVMPlugin) ABACResourceTypes() []abacspec.ResourceType {
	return []abacspec.ResourceType{{
		Type:     "virtual_machine",
		Label:    "Virtual machine",
		Category: "vm",
		Routes:   []abacspec.Route{{Prefix: "/api/vms/{id}", IDParam: "id"}},
		Attributes: []abacspec.Attribute{
			{Path: "resource.attrs.vcpus", Type: "number", Description: "Virtual CPUs"},
			{Path: "resource.attrs.memoryMb", Type: "number", Description: "Memory in MiB"},
			{Path: "resource.attrs.guestOsFamily", Type: "string", Description: "linux, windows …"},
			{Path: "resource.attrs.isTemplate", Type: "bool", Description: "The machine is a template"},
			{Path: "resource.attrs.firmware", Type: "string", Description: "bios or uefi"},
			{Path: "resource.attrs.autostart", Type: "bool", Description: "Starts with the host"},
		},
	}}
}

// ResolveABACResource loads one machine for policy evaluation; an unknown id yields
// no resource rather than an error, so a policy simply sees nothing to match.
func (p *KVMPlugin) ResolveABACResource(ctx context.Context, _, _, id string) (*abacspec.Resource, error) {
	if p.svc == nil || id == "" {
		return nil, nil
	}
	vm, err := p.svc.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &abacspec.Resource{
		ID: vm.ID, Name: vm.Name, State: vm.State, Environment: vm.EnvironmentID,
		Labels: map[string]string{},
		Attrs: map[string]any{
			"vcpus": vm.VCPUs, "memoryMb": vm.MemoryMB, "guestOsFamily": vm.GuestOSFamily,
			"isTemplate": vm.IsTemplate, "firmware": vm.Firmware, "autostart": vm.Autostart,
		},
	}, nil
}

var _ abacspec.Provider = (*KVMPlugin)(nil)
