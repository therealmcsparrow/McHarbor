// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	msgVMMemorySet    = "vm_memory_set"
	msgVMCPUSet       = "vm_cpu_set"
	msgVMThrottleSet  = "vm_throttle_set"
	msgVMImageInspect = "vm_image_inspect"
)

// agentResizeResult is what a live resize achieved.
type agentResizeResult struct {
	RequestedMemoryMB int64 `json:"requestedMemoryMb,omitempty"`
	ActualMemoryMB    int64 `json:"actualMemoryMb,omitempty"`
	BlockSizeMB       int64 `json:"blockSizeMb,omitempty"`
	RequestedVCPUs    int   `json:"requestedVcpus,omitempty"`
	ActualVCPUs       int   `json:"actualVcpus,omitempty"`
	Complied          bool  `json:"complied"`
}

// agentImageOp is one read-only qemu-img subcommand.
type agentImageOp struct {
	Operation   string `json:"operation"`
	Path        string `json:"path"`
	ComparePath string `json:"comparePath,omitempty"`
	Repair      bool   `json:"repair,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	Format      string `json:"format,omitempty"`
}

// ResizeResult is what the API returns for a live resize.
//
// Requested and Actual are both present because they differ: a memory request
// is rounded to the host's block size, and an unplug is a request the guest
// may decline. A response carrying only the request would make a refusal look
// like a success.
type ResizeResult struct {
	RequestedMemoryMB int64 `json:"requestedMemoryMb,omitempty"`
	ActualMemoryMB    int64 `json:"actualMemoryMb,omitempty"`
	BlockSizeMB       int64 `json:"blockSizeMb,omitempty"`
	RequestedVCPUs    int   `json:"requestedVcpus,omitempty"`
	ActualVCPUs       int   `json:"actualVcpus,omitempty"`
	Complied          bool  `json:"complied"`
}

// ResizeMemory changes a running guest's memory through its virtio-mem device.
func (s *Service) ResizeMemory(ctx context.Context, id string, totalMB int64) (*ResizeResult, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !vm.Running() {
		return nil, ErrNotRunning
	}
	if !vm.VirtioMem {
		return nil, ErrNoResizableMemory
	}
	if totalMB < vm.MemoryMB {
		return nil, fmt.Errorf("%w: below the guest's boot memory of %d MB",
			ErrInvalidResize, vm.MemoryMB)
	}
	if vm.MemoryMaxMB > 0 && totalMB > vm.MemoryMaxMB {
		return nil, fmt.Errorf("%w: above the ceiling of %d MB this guest was started with",
			ErrInvalidResize, vm.MemoryMaxMB)
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMMemorySet, agentPayload{
		VMID: id, Spec: s.buildSpec(ctx, vm), TargetMemoryMB: totalMB,
	})
	if err != nil {
		return nil, err
	}
	out := toResizeResult(reply.Resize)

	// What the guest actually holds, not what was asked for. The distinction
	// is the whole point of the endpoint: a shrink a guest declined must not
	// leave the database claiming it happened.
	if out.ActualMemoryMB > 0 {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_vms
			   SET memory_current_mb = ?, updated_at = ?
			 WHERE id = ?`,
			out.ActualMemoryMB, time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return out, fmt.Errorf("recording the guest's memory: %w", err)
		}
	}
	return out, nil
}

// SetVCPUs plugs or unplugs vCPUs on a running guest.
func (s *Service) SetVCPUs(ctx context.Context, id string, target int) (*ResizeResult, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !vm.Running() {
		return nil, ErrNotRunning
	}
	if target < 1 {
		return nil, fmt.Errorf("%w: a guest needs at least one processor", ErrInvalidResize)
	}
	ceiling := vm.VCPUsMax
	if ceiling <= 0 {
		ceiling = vm.VCPUs
	}
	if target > ceiling {
		// The ceiling is baked into -smp at boot, so this is a restart rather
		// than a refusal of the whole idea — and saying which it is matters.
		return nil, fmt.Errorf("%w: this guest was started with room for %d processors; "+
			"raising that needs a restart", ErrInvalidResize, ceiling)
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMCPUSet, agentPayload{
		VMID: id, TargetVCPUs: target,
	})
	if err != nil {
		return nil, err
	}
	out := toResizeResult(reply.Resize)

	if out.ActualVCPUs > 0 {
		if _, err := s.db.Exec(ctx, `
			UPDATE plugin_addon_kvm_vms SET vcpus = ?, updated_at = ? WHERE id = ?`,
			out.ActualVCPUs, time.Now().UTC().Format(time.RFC3339), id); err != nil {
			return out, fmt.Errorf("recording the guest's processor count: %w", err)
		}
	}
	return out, nil
}

// toResizeResult converts the agent's answer, tolerating its absence.
//
// An agent too old to know the message replies without the field; reporting
// that as "not complied" is honest, because nothing happened.
func toResizeResult(in *agentResizeResult) *ResizeResult {
	if in == nil {
		return &ResizeResult{}
	}
	return &ResizeResult{
		RequestedMemoryMB: in.RequestedMemoryMB,
		ActualMemoryMB:    in.ActualMemoryMB,
		BlockSizeMB:       in.BlockSizeMB,
		RequestedVCPUs:    in.RequestedVCPUs,
		ActualVCPUs:       in.ActualVCPUs,
		Complied:          in.Complied,
	}
}

// ThrottleRequest is a disk's I/O limits. Zero means unlimited.
type ThrottleRequest struct {
	ReadIOPS  int64 `json:"readIops"`
	WriteIOPS int64 `json:"writeIops"`
	ReadBPS   int64 `json:"readBps"`
	WriteBPS  int64 `json:"writeBps"`
}

// minSaneBPS is the floor below which a limit stops a guest booting.
//
// A megabyte a second is already painful; anything under it produces a guest
// that looks hung rather than slow, and explaining that afterwards is worse
// than refusing it now.
const minSaneBPS = 1 << 20

// SetThrottle records a disk's I/O limits and applies them to a running guest.
func (s *Service) SetThrottle(ctx context.Context, vmID, diskID string, req ThrottleRequest) error {
	if err := req.validate(); err != nil {
		return err
	}

	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return err
	}
	disk, ok := findDisk(vm.Disks, diskID)
	if !ok {
		return ErrNotFound
	}

	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_disks
		   SET io_read_iops = ?, io_write_iops = ?, io_read_bps = ?, io_write_bps = ?,
		       updated_at = ?
		 WHERE id = ? AND vm_id = ?`,
		zeroToNull(req.ReadIOPS), zeroToNull(req.WriteIOPS),
		zeroToNull(req.ReadBPS), zeroToNull(req.WriteBPS),
		time.Now().UTC().Format(time.RFC3339), diskID, vmID); err != nil {
		return fmt.Errorf("recording the disk's I/O limits: %w", err)
	}

	// Stored first, applied second. A running guest that cannot be reached
	// still gets the limits at its next start, which is the behaviour an
	// operator expects from a setting they just saved.
	if !vm.Running() {
		return nil
	}
	_, err = s.call(ctx, vm.EnvironmentID, msgVMThrottleSet, agentPayload{
		VMID: vmID,
		Disk: &agentDisk{
			DiskID: disk.ID, Slot: disk.Slot,
			ReadIOPS: req.ReadIOPS, WriteIOPS: req.WriteIOPS,
			ReadBPS: req.ReadBPS, WriteBPS: req.WriteBPS,
		},
	})
	return err
}

// validate refuses limits that would make a guest look broken.
func (r ThrottleRequest) validate() error {
	for _, value := range []int64{r.ReadIOPS, r.WriteIOPS, r.ReadBPS, r.WriteBPS} {
		if value < 0 {
			return fmt.Errorf("%w: a limit cannot be negative", ErrInvalidResize)
		}
	}
	for _, value := range []int64{r.ReadBPS, r.WriteBPS} {
		if value > 0 && value < minSaneBPS {
			return fmt.Errorf("%w: a bandwidth limit under 1 MB/s leaves a guest unable to boot",
				ErrInvalidResize)
		}
	}
	return nil
}

// zeroToNull stores an absent limit as NULL rather than 0, so "unlimited" and
// "not set" stay the same value in the column and in the API.
func zeroToNull(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

// findDisk returns a guest's disk by id.
func findDisk(disks []Disk, diskID string) (Disk, bool) {
	for _, disk := range disks {
		if disk.ID == diskID {
			return disk, true
		}
	}
	return Disk{}, false
}

// InspectImage runs one read-only qemu-img subcommand against a disk.
//
// The operation is checked here and again on the agent. Both matter: this one
// gives the operator a translated refusal, and the agent's is the one that
// actually binds, because it runs as root.
func (s *Service) InspectImage(ctx context.Context, vmID, diskID, operation string, repair bool) (json.RawMessage, error) {
	if !allowedImageOps[operation] {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedImageOp, operation)
	}

	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return nil, err
	}
	disk, ok := findDisk(vm.Disks, diskID)
	if !ok {
		return nil, ErrNotFound
	}
	if repair && vm.Running() {
		return nil, ErrRunning
	}

	reply, err := s.call(ctx, vm.EnvironmentID, msgVMImageInspect, agentPayload{
		VMID: vmID,
		ImageOp: &agentImageOp{
			Operation: operation, Path: disk.VolumePath, Repair: repair,
		},
	})
	if err != nil {
		return nil, err
	}
	return reply.ImageInfo, nil
}

// allowedImageOps is the read-only set. `dd`, `bench` and `amend` are reachable
// from the same binary and are all destructive against a live disk, so the
// allow-list is what keeps them unreachable rather than a convention.
var allowedImageOps = map[string]bool{
	"info":    true,
	"check":   true,
	"map":     true,
	"measure": true,
}

// HandleResizeMemory serves POST /api/vms/{id}/memory.
func (h *Handler) HandleResizeMemory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemoryMB int64 `json:"memoryMb"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	result, err := h.svc.ResizeMemory(r.Context(), chi.URLParam(r, "id"), req.MemoryMB)
	if err != nil {
		h.respondErr(w, r, "resize memory", err)
		return
	}
	ok(w, result)
}

// HandleSetVCPUs serves POST /api/vms/{id}/vcpus.
func (h *Handler) HandleSetVCPUs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VCPUs int `json:"vcpus"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	result, err := h.svc.SetVCPUs(r.Context(), chi.URLParam(r, "id"), req.VCPUs)
	if err != nil {
		h.respondErr(w, r, "set vcpus", err)
		return
	}
	ok(w, result)
}

// HandleSetThrottle serves PUT /api/vms/{id}/disks/{diskId}/throttle.
func (h *Handler) HandleSetThrottle(w http.ResponseWriter, r *http.Request) {
	var req ThrottleRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	if err := h.svc.SetThrottle(r.Context(),
		chi.URLParam(r, "id"), chi.URLParam(r, "diskId"), req); err != nil {
		h.respondErr(w, r, "set io limits", err)
		return
	}
	okMsg(w, msgThrottleSaved)
}

// HandleInspectImage serves POST /api/vms/{id}/disks/{diskId}/inspect.
func (h *Handler) HandleInspectImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Operation string `json:"operation"`
		Repair    bool   `json:"repair"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	info, err := h.svc.InspectImage(r.Context(),
		chi.URLParam(r, "id"), chi.URLParam(r, "diskId"), req.Operation, req.Repair)
	if err != nil {
		h.respondErr(w, r, "inspect image", err)
		return
	}
	ok(w, json.RawMessage(info))
}
