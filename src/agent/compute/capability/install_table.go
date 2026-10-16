// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capability

// installPackages is the only list of packages the agent will ever install, keyed by
// capability item. McHarbor names the item, never a package, so a request cannot widen it.
func installPackages(item, family, osID, arch string) []string {
	arm := arch == "arm64"
	fedora := osID == "fedora"
	switch family {
	case "debian":
		switch item {
		case ItemQEMU:
			if arm {
				return []string{"qemu-system-arm"}
			}
			return []string{"qemu-system-x86"}
		case ItemQEMUImg:
			return []string{"qemu-utils"}
		case ItemOVMF, ItemOVMFSecure:
			if arm {
				return []string{"qemu-efi-aarch64"}
			}
			return []string{"ovmf"}
		case ItemSwtpm:
			return []string{"swtpm", "swtpm-tools"}
		}
	case "rhel":
		switch item {
		case ItemQEMU:
			switch {
			case !fedora:
				return []string{"qemu-kvm"}
			case arm:
				return []string{"qemu-system-aarch64"}
			}
			return []string{"qemu-system-x86"}
		case ItemQEMUImg:
			return []string{"qemu-img"}
		case ItemOVMF, ItemOVMFSecure:
			if arm {
				return []string{"edk2-aarch64"}
			}
			return []string{"edk2-ovmf"}
		case ItemSwtpm:
			return []string{"swtpm", "swtpm-tools"}
		}
	}
	return nil
}
