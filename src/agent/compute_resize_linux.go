// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
)

// handleMemorySet answers vm_memory_set.
//
// The request carries the guest's target total; the boot size comes from the
// spec, because only the difference is plugged and the agent must not have to
// remember what the guest was defined with.
func (rt *computeRuntime) handleMemorySet(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.Spec == nil {
		return fmt.Errorf("compute: %s carried no spec", MsgVMMemorySet)
	}
	if !req.Spec.VirtioMem {
		return fmt.Errorf("compute: %s has no resizable memory device; "+
			"it has to be restarted with one before its memory can change", req.Spec.Name)
	}

	result, err := rt.vms.ResizeMemory(ctx, req.VMID, req.Spec.MemoryMB, req.TargetMemoryMB)
	if err != nil {
		return err
	}
	reply.Resize = &VMResizeResult{
		RequestedMemoryMB: result.RequestedMB,
		ActualMemoryMB:    result.PluggedMB,
		BlockSizeMB:       result.BlockSizeMB,
		Complied:          result.Complied,
	}
	reply.Success = true
	return nil
}

// handleCPUSet answers vm_cpu_set.
func (rt *computeRuntime) handleCPUSet(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.TargetVCPUs < 1 {
		return fmt.Errorf("compute: %s needs at least one vcpu", MsgVMCPUSet)
	}

	result, err := rt.vms.SetVCPUs(ctx, req.VMID, req.TargetVCPUs)
	if err != nil {
		return err
	}
	reply.Resize = &VMResizeResult{
		RequestedVCPUs: result.RequestedVCPUs,
		ActualVCPUs:    result.ActualVCPUs,
		Complied:       result.Complied,
	}
	reply.Success = true
	return nil
}

// handleThrottleSet answers vm_throttle_set.
//
// The limits reach a running guest here; the same values also go onto the
// command line at next start, so a restart does not quietly drop them.
func (rt *computeRuntime) handleThrottleSet(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.Disk == nil {
		return fmt.Errorf("compute: %s carried no disk", MsgVMThrottleSet)
	}

	if err := rt.vms.SetIOThrottle(ctx, req.VMID, vm.Disk{
		Slot:      req.Disk.Slot,
		ReadIOPS:  req.Disk.ReadIOPS,
		WriteIOPS: req.Disk.WriteIOPS,
		ReadBPS:   req.Disk.ReadBPS,
		WriteBPS:  req.Disk.WriteBPS,
	}); err != nil {
		return err
	}
	reply.Success = true
	return nil
}

// imageOpTimeout bounds one qemu-img run.
//
// `check` on a large image reads every cluster, so this is generous. It is
// still bounded: an image on a wedged NFS mount would otherwise hold the
// dispatch goroutine indefinitely.
const imageOpTimeout = 10 * time.Minute

// handleImageInspect answers vm_image_inspect.
//
// The operation is matched against a fixed set and the arguments are built
// here, never taken from the request. This runs as root on the host, so the
// difference between an allow-list and a forwarded command string is the
// difference between a diagnostic and a remote shell.
func (rt *computeRuntime) handleImageInspect(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.ImageOp == nil {
		return fmt.Errorf("compute: %s carried no operation", MsgVMImageInspect)
	}
	op := req.ImageOp

	binary, err := exec.LookPath("qemu-img")
	if err != nil {
		return fmt.Errorf("compute: qemu-img is not installed on this host: %w", err)
	}

	var args []string
	switch op.Operation {
	case "info":
		// The whole backing chain, not just the top: the snapshot view infers
		// its tree from McHarbor's own rows today, and this is what lets it be
		// checked against the image.
		args = []string{"info", "--output=json", "--backing-chain", op.Path}
	case "check":
		args = []string{"check", "--output=json"}
		if op.Repair {
			running, err := rt.vmIsRunning(ctx, req.VMID)
			if err != nil {
				return err
			}
			if running {
				// The authoritative refusal, on the side that knows. `check -r`
				// against a disk a live guest has open corrupts it, and a UI
				// check alone can be raced or bypassed.
				return fmt.Errorf("compute: this disk belongs to a running guest; " +
					"repairing it would corrupt it, so the guest has to be stopped first")
			}
			args = append(args, "-r", "all")
		}
		args = append(args, op.Path)
	case "map":
		args = []string{"map", "--output=json", op.Path}
	case "measure":
		args = []string{"measure", "--output=json"}
		if op.Format != "" {
			args = append(args, "-O", op.Format)
		}
		if op.SizeBytes > 0 {
			// Measuring a size rather than an existing file, which is what the
			// create-disk dialog asks: what will "10 GB" actually take here.
			args = append(args, "--size", strconv.FormatInt(op.SizeBytes, 10))
		} else {
			args = append(args, op.Path)
		}
	case "compare":
		if op.ComparePath == "" {
			return fmt.Errorf("compute: compare needs two images")
		}
		args = []string{"compare", op.Path, op.ComparePath}
	default:
		// Deliberately not a pass-through. dd, bench and amend are all reachable
		// from the same binary, and bench against a live guest's disk or
		// `amend --force` on an encrypted image are destructive.
		return fmt.Errorf("compute: %q is not an image operation this agent runs", op.Operation)
	}

	// -U on every read-only operation. A running guest holds a write lock on
	// its own disk, and without --force-share qemu-img refuses to open it --
	// which is a refusal about locking, not about the image, and the operator
	// asked a question that is perfectly safe to answer while it runs. The
	// repair path above is the one that must not share, and it already refuses
	// on a running guest instead.
	if !op.Repair {
		args = append([]string{args[0], "-U"}, args[1:]...)
	}

	runCtx, cancel := context.WithTimeout(ctx, imageOpTimeout)
	defer cancel()

	out, runErr := exec.CommandContext(runCtx, binary, args...).Output()
	// A non-zero exit is the answer for check and compare rather than a
	// failure: check exits 2 on corruption and 3 on leaks, compare exits 1 when
	// the images differ, and all three are results the operator asked for.
	if runErr != nil && len(out) == 0 {
		return fmt.Errorf("compute: qemu-img %s: %w", op.Operation, runErr)
	}

	if json.Valid(out) {
		reply.ImageInfo = out
	} else {
		// compare has no JSON mode, so its verdict is wrapped rather than
		// dropped — the consumer gets one shape from every operation.
		wrapped, err := json.Marshal(map[string]string{"output": string(out)})
		if err != nil {
			return fmt.Errorf("compute: encoding the qemu-img result: %w", err)
		}
		reply.ImageInfo = wrapped
	}
	reply.Success = true
	return nil
}
