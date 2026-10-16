// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
)

// cloneCleanupTimeout bounds removing what a failed or cancelled clone made. It
// runs on a context detached from the request, which is usually the reason the
// clone is being cleaned up at all.
const cloneCleanupTimeout = 5 * time.Minute

// handleVMClone answers vm_clone_request: one volume per disk in the source's
// own pool, full or linked, then a definition for the new guest.
//
// Everything that makes the clone itself — its UUID, MACs and cloud-init
// instance-id — arrives in the clone spec; the agent regenerates nothing. What
// it made is torn down again if any disk fails or the clone is cancelled with
// vm_cancel on the clone's id, so a failure leaves no half-guest behind.
func (rt *computeRuntime) handleVMClone(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.Clone == nil || req.Spec == nil {
		return errors.New("compute: vm_clone_request carried no clone or no source spec")
	}
	if err := validateGuestID(req.VMID); err != nil {
		return err
	}
	target := req.Clone.TargetVMID
	copies, err := planVMClone(req.Spec.Disks, target)
	if err != nil {
		return err
	}

	// A copy of a disk that is being written is a disk from no single moment.
	if states, _, err := rt.vms.Query(ctx, []string{req.VMID}); err == nil {
		for _, state := range states {
			if state.VMID == req.VMID && (state.State == vm.StateRunning || state.State == vm.StatePaused) {
				return fmt.Errorf("compute: %s is running; stop it before cloning", req.VMID)
			}
		}
	}

	cloneCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	rt.vms.RegisterCancel(target, cancel)
	defer rt.vms.Cancel(target)

	type made struct {
		driver storage.Driver
		pool   storage.Pool
		path   string
	}
	var created []made
	rollback := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), cloneCleanupTimeout)
		defer cleanupCancel()
		for _, volume := range created {
			if err := volume.driver.DeleteVolume(cleanupCtx, volume.pool, volume.path); err != nil {
				rt.logger.Warn("compute: removing a failed clone's volume", "vm", target,
					"path", volume.path, "error", err)
			}
		}
	}

	volumes := make(map[int]clonedVolume, len(copies))
	for i, plan := range copies {
		rt.publishCloneProgress(target, percentOf(i, len(copies)), "disks")
		driver, err := rt.pools.Get(plan.Source.PoolBackend)
		if err != nil {
			rollback()
			return err
		}
		pool := storage.Pool{ID: plan.Source.PoolID, Backend: plan.Source.PoolBackend, Target: plan.Source.PoolTarget}
		volume, err := driver.CloneVolume(cloneCtx, pool, plan.Source.VolumePath, plan.TargetName, req.Clone.Linked)
		if err != nil {
			rollback()
			if cloneCtx.Err() != nil && ctx.Err() == nil {
				return fmt.Errorf("compute: the clone of %s was cancelled", req.VMID)
			}
			return err
		}
		created = append(created, made{driver: driver, pool: pool, path: volume.Path})
		// Only a dir pool links through a qcow2 backing reference; zfs, btrfs and
		// lvm share blocks underneath and the clone is a volume of its own.
		backed := req.Clone.Linked && plan.Source.PoolBackend == storage.BackendDir && !volume.IsBlock
		if backed {
			if err := rt.backedVolumes.record(volume.Path); err != nil {
				rt.logger.Warn("compute: recording a linked clone volume", "path", volume.Path, "error", err)
			}
		}
		volumes[plan.Source.Slot] = clonedVolume{
			Path: volume.Path, Format: volume.Format, SizeBytes: volume.SizeBytes, Backed: backed,
		}
	}

	disks := clonedDisks(req.Spec.Disks, volumes)
	spec := *req.Spec
	if req.Clone.Spec != nil {
		spec = *req.Clone.Spec
	}
	spec.Disks = disks
	if err := rt.defineVM(cloneCtx, &VMPayload{VMID: target, Spec: &spec}); err != nil {
		rollback()
		if deleteErr := rt.vms.Delete(context.WithoutCancel(ctx), target); deleteErr != nil {
			rt.logger.Warn("compute: removing a failed clone's definition", "vm", target, "error", deleteErr)
		}
		return err
	}

	reply.Disks = disks
	rt.publishCloneProgress(target, 100, "done")
	return nil
}

// publishCloneProgress reports a clone's progress through the guest event
// channel, keyed by the clone's id, which is what the list shows while it runs.
func (rt *computeRuntime) publishCloneProgress(vmID string, percent int, step string) {
	rt.publishVMEvent(vm.Event{
		VMID: vmID,
		Kind: "clone_progress",
		At:   time.Now().UTC(),
		Detail: map[string]string{
			"percent": strconv.Itoa(percent),
			"step":    step,
		},
	})
}
