// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hardware

// pciVendors maps the PCI vendor ids an operator is realistically holding
// to their names.
//
// This is deliberately a short list, not a copy of `pci.ids`. The full
// table is ~1.5 MB of data that goes stale, has to be shipped in the
// image, and would buy us marketing names for hardware nobody running a
// container host owns. What actually matters is that the three or four
// adapters in a given machine read as "Intel Corporation" instead of
// "0x8086"; anything outside the list still renders as `vendor:device`
// next to its kernel driver, which is enough to search for.
//
// Keys are lowercase hex without the `0x` prefix, matching what sysfs
// writes after trimming.
var pciVendors = map[string]string{
	"1002": "AMD (ATI)",
	"1013": "Cirrus Logic",
	"1022": "AMD",
	"1028": "Dell",
	"103c": "HP",
	"1043": "ASUSTeK",
	"106b": "Apple",
	"1077": "QLogic",
	"10de": "NVIDIA Corporation",
	"10ec": "Realtek Semiconductor",
	"1106": "VIA Technologies",
	"1179": "Toshiba",
	"11ab": "Marvell",
	"1234": "QEMU",
	"1274": "Ensoniq",
	"12d8": "Pericom",
	"1344": "Micron",
	"1414": "Microsoft",
	"144d": "Samsung Electronics",
	"14e4": "Broadcom",
	"15ad": "VMware",
	"15b3": "Mellanox (NVIDIA Networking)",
	"1682": "XFX",
	"16c3": "ASMedia",
	"1849": "ASRock",
	"1912": "Renesas",
	"1969": "Qualcomm Atheros",
	"197b": "JMicron",
	"1af4": "Red Hat (virtio)",
	"1b21": "ASMedia Technology",
	"1b36": "Red Hat (QEMU)",
	"1b4b": "Marvell Technology",
	"1b73": "Fresco Logic",
	"1c5c": "SK hynix",
	"1cc1": "ADATA",
	"1d6b": "Linux Foundation",
	"1de1": "Tekram",
	"1e0f": "KIOXIA",
	"2646": "Kingston",
	"5333": "S3 Graphics",
	"8086": "Intel Corporation",
	"80ee": "VirtualBox",
	"9005": "Adaptec",
	"c0a9": "Micron/Crucial",
}

// pciVendorName resolves a vendor id, or returns "" when it is unmapped.
func pciVendorName(id string) string {
	return pciVendors[id]
}

// nvidiaVendorID identifies the cards the nvidia-smi telemetry can be
// merged onto. An AMD or Intel GPU is listed all the same, it just has no
// utilisation figure to show.
const nvidiaVendorID = "10de"
