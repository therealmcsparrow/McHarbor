// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capability

import (
	"slices"
	"testing"
)

func TestInstallPackagesNamesTheDistributionsOwnPackage(t *testing.T) {
	cases := []struct {
		item, family, osID, arch string
		want                     []string
	}{
		{ItemQEMUImg, "debian", "ubuntu", "amd64", []string{"qemu-utils"}},
		{ItemQEMUImg, "rhel", "fedora", "amd64", []string{"qemu-img"}},
		{ItemQEMU, "debian", "debian", "amd64", []string{"qemu-system-x86"}},
		{ItemQEMU, "debian", "debian", "arm64", []string{"qemu-system-arm"}},
		{ItemQEMU, "rhel", "fedora", "amd64", []string{"qemu-system-x86"}},
		{ItemQEMU, "rhel", "rocky", "amd64", []string{"qemu-kvm"}},
		{ItemOVMF, "debian", "debian", "amd64", []string{"ovmf"}},
		{ItemOVMFSecure, "rhel", "fedora", "amd64", []string{"edk2-ovmf"}},
		{ItemOVMF, "debian", "debian", "arm64", []string{"qemu-efi-aarch64"}},
		{ItemSwtpm, "debian", "ubuntu", "amd64", []string{"swtpm", "swtpm-tools"}},
	}
	for _, c := range cases {
		if got := installPackages(c.item, c.family, c.osID, c.arch); !slices.Equal(got, c.want) {
			t.Errorf("installPackages(%s, %s, %s, %s) = %v, want %v", c.item, c.family, c.osID, c.arch, got, c.want)
		}
	}
}

func TestInstallPackagesOffersNothingOutsideTheTable(t *testing.T) {
	// Kernel features and host settings are not packages, and an unknown family gets no guess.
	for _, item := range []string{ItemKVMDevice, ItemCgroupV2, ItemIOMMU, ItemSubuid, "anything; rm -rf /"} {
		if got := installPackages(item, "debian", "debian", "amd64"); got != nil {
			t.Errorf("installPackages(%q) = %v, want nothing", item, got)
		}
	}
	if got := installPackages(ItemQEMUImg, "arch", "arch", "amd64"); got != nil {
		t.Errorf("an unsupported family must get no packages, got %v", got)
	}
}
