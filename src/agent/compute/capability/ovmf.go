// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package capability

import "os"

// ovmfCodePaths are where distributions put the plain UEFI firmware. There is
// no standard location, which is exactly why this is probed rather than assumed.
var ovmfCodePaths = []string{
	"/usr/share/OVMF/OVMF_CODE_4M.fd",
	"/usr/share/OVMF/OVMF_CODE.fd",
	"/usr/share/edk2/ovmf/OVMF_CODE.fd",
	"/usr/share/edk2/x64/OVMF_CODE.4m.fd",
	"/usr/share/qemu/ovmf-x86_64-code.bin",
}

// ovmfVarsPaths are the writable-variable templates that pair with the plain
// firmware. They carry no Secure Boot keys, which is correct for a guest that
// is not using Secure Boot.
var ovmfVarsPaths = []string{
	"/usr/share/OVMF/OVMF_VARS_4M.fd",
	"/usr/share/OVMF/OVMF_VARS.fd",
	"/usr/share/edk2/ovmf/OVMF_VARS.fd",
	"/usr/share/edk2/x64/OVMF_VARS.4m.fd",
	"/usr/share/qemu/ovmf-x86_64-vars.bin",
}

// ovmfSecureCodePaths are the SMM-enabled builds. Only these enforce Secure
// Boot; the plain build ignores the keys entirely, which is how a guest ends up
// reporting Secure Boot as off while McHarbor believed it was on.
var ovmfSecureCodePaths = []string{
	"/usr/share/OVMF/OVMF_CODE_4M.secboot.fd",
	"/usr/share/OVMF/OVMF_CODE.secboot.fd",
	"/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd",
	"/usr/share/edk2/x64/OVMF_CODE.secboot.4m.fd",
	"/usr/share/qemu/ovmf-x86_64-smm-code.bin",
}

// ovmfEnrolledVarsPaths carry Microsoft's KEK and db pre-enrolled. Secure Boot
// with an empty key store is Secure Boot in setup mode, which Windows 11 counts
// as disabled — so an enrolled template is a requirement, not a nicety.
var ovmfEnrolledVarsPaths = []string{
	"/usr/share/OVMF/OVMF_VARS_4M.ms.fd",
	"/usr/share/OVMF/OVMF_VARS.ms.fd",
	"/usr/share/edk2/ovmf/OVMF_VARS.secboot.fd",
	"/usr/share/edk2/x64/OVMF_VARS.4m.fd",
	"/usr/share/qemu/ovmf-x86_64-ms-vars.bin",
}

// OVMFPaths returns the firmware code and the variable template to seed a guest
// from. Either is empty when this host has nothing suitable, and for a Secure
// Boot guest both must be present: the SMM build without enrolled keys boots
// with Secure Boot off.
func OVMFPaths(secureBoot bool) (code string, vars string) {
	if secureBoot {
		return firstExisting(ovmfSecureCodePaths), firstExisting(ovmfEnrolledVarsPaths)
	}
	return firstExisting(ovmfCodePaths), firstExisting(ovmfVarsPaths)
}

func firstExisting(paths []string) string {
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}
