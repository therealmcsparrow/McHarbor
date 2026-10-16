// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"fmt"
	"os"
)

// netdevFDBase is the first descriptor a macvtap may be placed on. Below it are
// the standard streams, which carry everything QEMU says about why it failed.
const netdevFDBase = 3

// netdevExecCommand is the agent subcommand that places the descriptors.
const netdevExecCommand = "netdev-exec"

// AssignNetdevFDs gives every direct NIC the descriptor QEMU will be told to
// read, and reports whether any NIC needs one.
//
// The numbering is by position rather than by slot so a guest with a NIC in
// slot 3 alone still starts at fd 3, which keeps the wrapped argv readable.
func AssignNetdevFDs(spec *Spec) bool {
	next := netdevFDBase
	for index := range spec.NICs {
		if spec.NICs[index].DevicePath == "" {
			continue
		}
		spec.NICs[index].FD = next
		next++
	}
	return next > netdevFDBase
}

// NetdevExecArgv wraps a QEMU command line in the agent's own shim, which opens
// each macvtap character device, puts it on the expected descriptor and execs
// QEMU.
//
// The wrapping exists because QEMU is started as a detached transient unit, so
// there is no parent to inherit a descriptor from, and a macvtap cannot be
// reopened by name the way a bridged tap can. The shim is this binary rather
// than a shell: the whole argv stays one argument per value, which is what
// keeps a space in a volume path from becoming two arguments.
func NetdevExecArgv(spec Spec, argv []string) ([]string, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("vm: locating the agent binary for %s: %w", spec.Name, err)
	}

	wrapped := []string{self, netdevExecCommand}
	for _, nic := range spec.NICs {
		if nic.DevicePath == "" {
			continue
		}
		if nic.FD < netdevFDBase {
			return nil, fmt.Errorf("vm: NIC in slot %d has no descriptor assigned", nic.Slot)
		}
		wrapped = append(wrapped, "--fd", fmt.Sprintf("%d=%s", nic.FD, nic.DevicePath))
	}
	wrapped = append(wrapped, "--")
	return append(wrapped, argv...), nil
}
