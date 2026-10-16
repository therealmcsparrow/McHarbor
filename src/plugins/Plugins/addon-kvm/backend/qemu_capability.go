// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// msgVMQEMUCapability asks a host what its QEMU can actually do.
const msgVMQEMUCapability = "vm_qemu_capability"

// VMFeature keys carried in QEMUCapability.Features.
//
// They mirror the agent's own constants by value rather than by import: the
// host's shared compute spec is linked into every plugin binary, so a typed
// field there would take every installed plugin offline for a capability list.
const (
	FeatureVirtioMem   = "virtio_mem"
	FeatureCPUHotplug  = "cpu_hotplug"
	FeatureHugePages   = "hugepages"
	FeatureNUMA        = "numa"
	FeatureVirtioFS    = "virtiofs"
	FeatureVSock       = "vsock"
	FeatureIOThrottle  = "io_throttle"
	FeatureIOThread    = "iothread"
	FeatureDirtyBitmap = "dirty_bitmap"
	FeatureBlockdevBkp = "blockdev_backup"
	FeatureVirtioGPU   = "virtio_gpu"
	FeatureVirtioSnd   = "virtio_snd"
	FeatureUSBU2F      = "usb_u2f"
	// FeatureHotplug is live disk, disc and NIC changes on a running guest.
	FeatureHotplug = "hotplug"
	// FeatureDisplayResize is a QEMU whose console can change a virtio-gpu
	// guest's resolution; the console hint reads it.
	FeatureDisplayResize = "display_resize"
)

// agentHugePages is the host's hugepage pool as the agent reports it.
type agentHugePages struct {
	Total    int64 `json:"total"`
	Free     int64 `json:"free"`
	SizeKB   int64 `json:"sizeKb,omitempty"`
	Surplus  int64 `json:"surplus,omitempty"`
	Reserved int64 `json:"reserved,omitempty"`
}

// agentNUMANode is one host NUMA node as the agent reports it.
type agentNUMANode struct {
	ID          int   `json:"id"`
	CPUs        []int `json:"cpus,omitempty"`
	MemoryBytes int64 `json:"memoryBytes,omitempty"`
	FreeBytes   int64 `json:"freeBytes,omitempty"`
}

// agentQEMUCapability is the vm_qemu_capability reply.
type agentQEMUCapability struct {
	Binary       string          `json:"binary,omitempty"`
	Arch         string          `json:"arch,omitempty"`
	Version      string          `json:"version,omitempty"`
	MachineTypes []string        `json:"machineTypes,omitempty"`
	Devices      []string        `json:"devices,omitempty"`
	CPUModels    []string        `json:"cpuModels,omitempty"`
	QMPCommands  []string        `json:"qmpCommands,omitempty"`
	Features     map[string]bool `json:"features,omitempty"`
	HugePages    *agentHugePages `json:"hugePages,omitempty"`
	NUMANodes    []agentNUMANode `json:"numaNodes,omitempty"`
}

// QEMUCapability is what the UI gates its VM controls on.
//
// Devices and QMPCommands are deliberately not forwarded: they run to several
// hundred entries each and no frontend reads them. What travels is the derived
// feature map, the CPU models the picker needs, and the machine types a pinned
// type is checked against.
type QEMUCapability struct {
	Version      string          `json:"version,omitempty"`
	Arch         string          `json:"arch,omitempty"`
	MachineTypes []string        `json:"machineTypes,omitempty"`
	CPUModels    []string        `json:"cpuModels,omitempty"`
	Features     map[string]bool `json:"features"`
	HugePages    *agentHugePages `json:"hugePages,omitempty"`
	NUMANodes    []agentNUMANode `json:"numaNodes,omitempty"`
	ProbedAt     time.Time       `json:"probedAt"`
}

// capabilityTTL is how long a probe answer is reused.
//
// The agent already caches its own probe, so this only bounds the round trip.
// A host's QEMU changes on a package upgrade, which is rare enough that ten
// minutes of staleness is invisible and frequent enough that never refreshing
// would leave a newly-capable host looking incapable until a restart.
const capabilityTTL = 10 * time.Minute

// cachedCapability is one host's answer plus when it was taken.
type cachedCapability struct {
	value   *QEMUCapability
	fetched time.Time
}

// QEMUCapability returns what this host's QEMU can do, from cache when fresh.
func (s *Service) QEMUCapability(ctx context.Context, envID string) (*QEMUCapability, error) {
	if cached := s.cachedCapability(envID); cached != nil {
		return cached, nil
	}

	reply, err := s.call(ctx, envID, msgVMQEMUCapability, agentPayload{})
	if err != nil {
		return nil, err
	}
	if reply.QEMUCapability == nil {
		// An agent too old to know the message answers the generic refusal
		// rather than this field, and every feature then reads as
		// unavailable — which is the safe direction: a control the operator
		// cannot use renders disabled instead of failing at use time.
		return &QEMUCapability{Features: map[string]bool{}, ProbedAt: time.Now().UTC()}, nil
	}

	out := &QEMUCapability{
		Version:      reply.QEMUCapability.Version,
		Arch:         reply.QEMUCapability.Arch,
		MachineTypes: reply.QEMUCapability.MachineTypes,
		CPUModels:    reply.QEMUCapability.CPUModels,
		Features:     reply.QEMUCapability.Features,
		HugePages:    reply.QEMUCapability.HugePages,
		NUMANodes:    reply.QEMUCapability.NUMANodes,
		ProbedAt:     time.Now().UTC(),
	}
	if out.Features == nil {
		out.Features = map[string]bool{}
	}
	s.storeCapability(envID, out)
	return out, nil
}

// cachedCapability returns a fresh cached answer, or nil.
func (s *Service) cachedCapability(envID string) *QEMUCapability {
	s.capabilityMu.RLock()
	defer s.capabilityMu.RUnlock()

	entry, ok := s.capabilities[envID]
	if !ok || entry.value == nil {
		return nil
	}
	if time.Since(entry.fetched) > capabilityTTL {
		return nil
	}
	return entry.value
}

// storeCapability caches one host's answer.
func (s *Service) storeCapability(envID string, value *QEMUCapability) {
	s.capabilityMu.Lock()
	defer s.capabilityMu.Unlock()

	if s.capabilities == nil {
		s.capabilities = map[string]*cachedCapability{}
	}
	s.capabilities[envID] = &cachedCapability{value: value, fetched: time.Now()}
}

// SupportsFeature reports whether a host can do one thing.
//
// An unreachable host answers false, and that is the right default for every
// caller here: a start that emits a device the host has no model for fails
// with QEMU's wording, which is the outcome the capability gate exists to
// replace.
func (s *Service) SupportsFeature(ctx context.Context, envID, feature string) bool {
	capability, err := s.QEMUCapability(ctx, envID)
	if err != nil || capability == nil {
		return false
	}
	return capability.Features[feature]
}

// MachineTypeAvailable reports whether a VM's pinned machine type still exists
// on its host.
//
// QEMU removes old machine types on a schedule, and a guest pinned to a removed
// one does not start. Saying so before the start is the whole point: afterwards
// the operator has a failed unit and QEMU's own message, which names the type
// but not that it used to be there.
func (s *Service) MachineTypeAvailable(ctx context.Context, envID, machineType string) (bool, bool) {
	machineType = strings.TrimSpace(machineType)
	if machineType == "" {
		return true, false
	}
	capability, err := s.QEMUCapability(ctx, envID)
	if err != nil || capability == nil || len(capability.MachineTypes) == 0 {
		// No list means the question could not be asked, which is not the same
		// as the answer being no.
		return true, false
	}
	for _, candidate := range capability.MachineTypes {
		if candidate == machineType {
			return true, true
		}
	}
	return false, true
}

// HandleQEMUCapability serves GET /api/vms/capability.
//
// It is keyed by environment rather than by VM because it describes the host,
// and the create dialog needs it before any VM exists.
func (h *Handler) HandleQEMUCapability(w http.ResponseWriter, r *http.Request) {
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	capability, err := h.svc.QEMUCapability(r.Context(), envID)
	if err != nil {
		h.respondErr(w, r, "read qemu capability", err)
		return
	}
	ok(w, capability)
}
