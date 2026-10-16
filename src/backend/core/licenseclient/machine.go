// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package licenseclient

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"sync"
)

// A seat is one machine running McHarbor code, but an instance id
// identifies a *process*: a server and an agent on the same host mint one
// each and claim two seats for one machine. That is not an exotic
// configuration — enabling compute on the McHarbor host requires exactly
// it, because the container cannot be the execution host for QEMU or LXC.
//
// MachineID is the fingerprint that lets the licence server collapse the
// two back into one seat. It is a hint, not a credential: it grants
// nothing on its own, the licence key is still what authenticates, and a
// host that will not identify itself simply does not get deduplicated.
//
// The raw source is hashed rather than sent as-is. `/etc/machine-id` is
// used elsewhere as a seed for host-local secrets, and a licence check-in
// has no business carrying it in the clear.

const (
	// machineIDPath is systemd's host identifier: stable across reboots
	// and upgrades, regenerated only when the host is re-imaged.
	machineIDPath = "/etc/machine-id"
	// dbusMachineIDPath is the same value on hosts that predate systemd
	// or do not mount /etc/machine-id.
	dbusMachineIDPath = "/var/lib/dbus/machine-id"
	// productUUIDPath is the DMI serial, readable by root only. It is the
	// fallback for a host with no machine-id at all.
	productUUIDPath = "/sys/class/dmi/id/product_uuid"
	// machineIDLength is how much of the hash is sent. 128 bits is far
	// past collision risk for a population of licensed hosts, and a short
	// value stays readable in a licence dashboard.
	machineIDLength = 32
)

var (
	machineIDOnce  sync.Once
	machineIDValue string
)

// MachineID returns this host's fingerprint, or "" when nothing on the
// host identifies it. Resolved once — the sources are files that do not
// change under a running process, and a check-in should not pay for the
// reads every day.
func MachineID() string {
	machineIDOnce.Do(func() {
		machineIDValue = resolveMachineID()
	})
	return machineIDValue
}

// resolveMachineID walks the sources in descending order of stability.
//
// Hostname is last and deliberate: it is the weakest of the three,
// because two hosts can share one and a rename changes it. It still beats
// nothing, since the failure mode of a wrong fingerprint is a seat that
// does not collapse — the same place the product is today.
func resolveMachineID() string {
	for _, path := range []string{machineIDPath, dbusMachineIDPath, productUUIDPath} {
		if id := readMachineIDFile(path); id != "" {
			return hashMachineID("id:" + id)
		}
	}

	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	if name = strings.TrimSpace(name); name != "" {
		return hashMachineID("host:" + name)
	}
	return ""
}

// readMachineIDFile reads one candidate, treating an unreadable or empty
// file as simply absent. None of these paths exist on every host, and a
// missing one is the normal case rather than an error worth surfacing.
func readMachineIDFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// hashMachineID hashes a source value, prefixed so that a hostname and a
// machine-id which happen to hold the same string cannot collide.
func hashMachineID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:machineIDLength]
}
