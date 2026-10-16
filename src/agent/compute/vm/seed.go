// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package vm

import (
	"errors"
	"fmt"
	"os"
)

// SeedSlot is the disk slot reserved for the cloud-init seed CD-ROM. It sits
// outside the 0–15 hot-plug range so its node names can never collide with a
// disk the operator attaches.
const SeedSlot = 99

// AttachSeed adds the guest's cloud-init seed as a read-only CD-ROM, and
// reports whether it did.
//
// The seed is only attached until the guest has booted with it once: cloud-init
// needs it on the first boot, and a CD-ROM left on afterwards would block live
// migration, whose target drops CD-ROMs. It is never attached to an incoming
// migration for the same reason. No boot index, so it can never take the boot
// from the disk or an installer disc.
func AttachSeed(spec *Spec, paths Paths) bool {
	if spec.IncomingURI != "" {
		return false
	}
	info, err := os.Stat(paths.SeedISO)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if _, err := os.Stat(paths.SeedMarker); err == nil {
		return false
	}
	spec.Disks = append(spec.Disks, Disk{
		ID:        "cidata",
		Slot:      SeedSlot,
		Bus:       "sata",
		Path:      paths.SeedISO,
		Format:    "raw",
		ReadOnly:  true,
		IsCDROM:   true,
		BootIndex: -1,
	})
	return true
}

// MarkSeedDelivered records that the guest has booted with its seed, so later
// starts leave it off.
func MarkSeedDelivered(paths Paths) error {
	if err := os.WriteFile(paths.SeedMarker, nil, 0o600); err != nil {
		return fmt.Errorf("vm: record seed delivery: %w", err)
	}
	return nil
}

// ResetSeedDelivery forgets an earlier delivery, so a newly written seed is
// attached again on the next start.
func ResetSeedDelivery(paths Paths) error {
	if err := os.Remove(paths.SeedMarker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vm: reset seed delivery: %w", err)
	}
	return nil
}
