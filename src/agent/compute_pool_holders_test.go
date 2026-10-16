// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeProc struct {
	fds     map[int][]string
	cgroups map[int]string
	comms   map[int]string
}

func (f fakeProc) PIDs() ([]int, error) {
	out := make([]int, 0, len(f.fds))
	for pid := range f.fds {
		out = append(out, pid)
	}
	return out, nil
}

func (f fakeProc) FDTargets(pid int) ([]string, error) { return f.fds[pid], nil }
func (f fakeProc) Cgroup(pid int) (string, error)      { return f.cgroups[pid], nil }
func (f fakeProc) Comm(pid int) (string, error)        { return f.comms[pid], nil }

func TestHoldersUnderFindsARunningGuestsDisk(t *testing.T) {
	proc := fakeProc{
		fds: map[int][]string{
			101: {"/dev/null", "/mcharbor/compute/dapdjfdju16c72upg6lg/disk-0 (deleted)"},
			202: {"/mcharbor/compute/dapdjfdju16c72upg6lg-copy/disk-0"},
			303: {"/var/log/syslog"},
		},
		cgroups: map[int]string{101: "0::/system.slice/mcharbor-vm-dapdjfdju16c72upg6lg.service\n"},
		comms:   map[int]string{101: "qemu-system-x86", 202: "rsync", 303: "rsyslogd"},
	}

	holders, err := holdersUnder(proc, "/mcharbor/compute/dapdjfdju16c72upg6lg")
	if err != nil {
		t.Fatal(err)
	}
	if len(holders) != 1 || holders[0].PID != 101 || holders[0].VMID != "dapdjfdju16c72upg6lg" {
		t.Fatalf("holders = %+v, want only the guest pid 101 (a sibling with the same prefix is not inside)", holders)
	}

	err = inUseError("dapdjfdju16c72upg6lg", holders)
	if !errors.Is(err, errPathInUse) {
		t.Fatalf("error does not wrap errPathInUse: %v", err)
	}
	if !strings.Contains(err.Error(), "running guest dapdjfdju16c72upg6lg") {
		t.Fatalf("the refusal does not name the guest: %v", err)
	}
}

func TestHoldersUnderMatchesTheFileItselfAndNamesPlainProcesses(t *testing.T) {
	proc := fakeProc{
		fds:   map[int][]string{7: {"/pool/images/a.iso (deleted)"}},
		comms: map[int]string{7: "qemu-img"},
	}
	holders, err := holdersUnder(proc, "/pool/images/a.iso")
	if err != nil || len(holders) != 1 {
		t.Fatalf("holders = %+v, %v", holders, err)
	}
	if msg := inUseError("images/a.iso", holders).Error(); !strings.Contains(msg, "qemu-img (pid 7)") {
		t.Fatalf("a non-guest holder is not named: %s", msg)
	}
}

func TestHoldersUnderIsEmptyWhenNothingIsOpen(t *testing.T) {
	proc := fakeProc{fds: map[int][]string{1: {"/etc/hosts"}}}
	holders, err := holdersUnder(proc, "/mcharbor/compute/old")
	if err != nil || len(holders) != 0 {
		t.Fatalf("holders = %+v, %v", holders, err)
	}
}
