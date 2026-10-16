// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/qmp"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// guestQueryTimeout bounds one guest-agent round trip.
//
// The guest is the untrusted party here: a wedged or hostile guest that never
// answers must not hold the agent's dispatch goroutine, and a freeze in
// particular has to fail fast so the thaw below it still runs.
const guestQueryTimeout = 15 * time.Second

// handleGuestQuery answers vm_guest_query.
//
// The query name is matched against a fixed set rather than forwarded: the
// guest agent implements guest-exec, which is an unaudited remote shell into
// the guest, and the only thing standing between it and a caller is that no
// branch here reaches it.
func (rt *computeRuntime) handleGuestQuery(ctx context.Context, req *VMPayload, reply *VMPayload) error {
	if req.VMID == "" {
		return fmt.Errorf("compute: %s carried no guest id", MsgVMGuestQuery)
	}

	ctx, cancel := context.WithTimeout(ctx, guestQueryTimeout)
	defer cancel()

	paths := vm.PathsFor(req.VMID)
	if _, err := os.Stat(paths.GuestAgent); err != nil {
		// A stopped guest has no socket at all, which is not the same as a
		// running guest with no agent installed — the UI says something
		// different for each, so the distinction has to survive to it.
		reply.Guest = &VMGuestInfo{VMID: req.VMID, AgentState: "absent"}
		reply.Success = true
		return nil
	}

	guest, err := qmp.DialGuest(ctx, paths.GuestAgent)
	if err != nil {
		reply.Guest = &VMGuestInfo{VMID: req.VMID, AgentState: "absent"}
		reply.Success = true
		return nil
	}
	defer func() { errlog.Close(guest) }()

	info := &VMGuestInfo{VMID: req.VMID, AgentState: "connected"}
	switch req.GuestQuery {
	case "", "network":
		info.IPs, err = guest.UsableIPs(ctx)
	case "osinfo":
		var osInfo qmp.GuestOSInfo
		if osInfo, err = guest.OSInfo(ctx); err == nil {
			applyOSInfo(info, osInfo)
		}
	case "hostname":
		info.Hostname, err = guest.Hostname(ctx)
	case "all":
		err = rt.fillGuestInfo(ctx, guest, info)
	case "health":
		err = fillGuestHealth(ctx, guest, info)
	case "fstrim":
		err = guest.FsTrim(ctx)
	case "fsfreeze":
		_, err = guest.FsFreeze(ctx)
		info.Frozen = err == nil
	case "fsthaw":
		_, err = guest.FsThaw(ctx)
	case "shutdown":
		err = guest.Shutdown(ctx)
	default:
		return fmt.Errorf("compute: %q is not a guest query this agent answers", req.GuestQuery)
	}
	if err != nil {
		return err
	}

	reply.Guest = info
	reply.Success = true
	return nil
}

// fillGuestInfo collects everything the VM detail shows in one connection.
//
// Each field is optional on its own: a guest agent that answers interfaces but
// not osinfo is old, not broken, so a failure on one field leaves the others
// populated rather than failing the whole query.
func (rt *computeRuntime) fillGuestInfo(ctx context.Context, guest *qmp.GuestAgent, info *VMGuestInfo) error {
	if err := guest.Ping(ctx); err != nil {
		return err
	}
	if ips, err := guest.UsableIPs(ctx); err == nil {
		info.IPs = ips
	}
	if osInfo, err := guest.OSInfo(ctx); err == nil {
		applyOSInfo(info, osInfo)
	}
	if hostname, err := guest.Hostname(ctx); err == nil {
		info.Hostname = hostname
	}
	if status, err := guest.FsFreezeStatus(ctx); err == nil {
		info.Frozen = status == "frozen"
	}
	return nil
}

// fillGuestHealth answers the incident scanner: a ping that fails means the guest
// is not answering, and each later command is optional because qemu-ga versions
// differ, so a missing one is recorded as unsupported instead of failing the query.
func fillGuestHealth(ctx context.Context, guest *qmp.GuestAgent, info *VMGuestInfo) error {
	if err := guest.Ping(ctx); err != nil {
		return err
	}
	if fss, err := guest.FsInfo(ctx); err == nil {
		for _, fs := range fss {
			info.Filesystems = append(info.Filesystems, VMGuestFilesystem{
				Mountpoint: fs.Mountpoint, Type: fs.Type, UsedBytes: *fs.UsedBytes, TotalBytes: *fs.TotalBytes,
			})
		}
	} else if qmp.IsUnsupportedCommand(err) {
		info.Unsupported = append(info.Unsupported, "guest-get-fsinfo")
	}
	if load, err := guest.Load(ctx); err == nil {
		info.Load1, info.Load5, info.Load15 = &load.Load1, &load.Load5, &load.Load15
	} else if qmp.IsUnsupportedCommand(err) {
		info.Unsupported = append(info.Unsupported, "guest-get-load")
	}
	before := time.Now()
	if ns, err := guest.Time(ctx); err == nil {
		mid := before.Add(time.Since(before) / 2)
		skew := (ns - mid.UnixNano()) / int64(time.Millisecond)
		info.ClockSkewMs = &skew
	} else if qmp.IsUnsupportedCommand(err) {
		info.Unsupported = append(info.Unsupported, "guest-get-time")
	}
	return nil
}

// applyOSInfo copies what the guest said about itself onto the reply.
//
// PrettyName wins where the guest offers it — "Debian GNU/Linux 12 (bookworm)"
// is what the operator recognises, and it is the field that distinguishes the
// guest's real identity from the distro McHarbor guessed at create time.
func applyOSInfo(info *VMGuestInfo, osInfo qmp.GuestOSInfo) {
	info.OSID = osInfo.ID
	info.OSName = osInfo.Name
	if osInfo.PrettyName != "" {
		info.OSName = osInfo.PrettyName
	}
	info.OSVersion = osInfo.Version
	if osInfo.Version == "" {
		info.OSVersion = osInfo.VersionID
	}
}

// thawStrandedGuests thaws any guest left frozen by an agent that died holding
// a freeze.
//
// QEMU's guest agent thaws on its own after its configured timeout, but that
// timeout is often disabled, so a crash between freeze and thaw can strand a
// guest's filesystems indefinitely. This runs at agent start for exactly that
// window.
func (rt *computeRuntime) thawStrandedGuests(ctx context.Context, vmIDs []string) {
	for _, vmID := range vmIDs {
		paths := vm.PathsFor(vmID)
		if _, err := os.Stat(paths.GuestAgent); err != nil {
			continue
		}
		dialCtx, cancel := context.WithTimeout(ctx, guestQueryTimeout)
		guest, err := qmp.DialGuest(dialCtx, paths.GuestAgent)
		if err != nil {
			cancel()
			continue
		}
		status, err := guest.FsFreezeStatus(dialCtx)
		if err == nil && status == "frozen" {
			if _, err := guest.FsThaw(dialCtx); err != nil && !errors.Is(err, context.Canceled) {
				rt.logger.Warn("compute: thawing a stranded guest failed",
					"vm", vmID, "error", err)
			} else {
				rt.logger.Info("compute: thawed a guest left frozen by a previous agent",
					"vm", vmID)
			}
		}
		errlog.Close(guest)
		cancel()
	}
}
