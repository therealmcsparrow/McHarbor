// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/gpu"
	"github.com/therealmcsparrow/mcharbor/core/hardware"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// HardwareResponse is the body of GET /metrics/hardware.
//
// Scope carries the warning: these are the devices of
// the machine running McHarbor, not of the selected environment. A
// multi-host operator switching environments must not read the local
// machine's adapters as the remote host's.
type HardwareResponse struct {
	hardware.Inventory
	Scope string `json:"scope"`
}

// HandleHardware returns the local host's device inventory.
//
// Always 200. An unreadable sysfs (non-Linux dev host, a masked `/sys`) is
// reported as `available: false` and explained in the UI, because a 500
// here would put an error state on a page whose other twelve sections
// rendered correctly, for a machine that is working fine.
func (h *Handler) HandleHardware(w http.ResponseWriter, r *http.Request) {
	inventory := h.hardware.Inventory(r.Context())
	h.enrichGPUs(r, &inventory)
	response.OK(w, HardwareResponse{Inventory: inventory, Scope: "host"})
}

// enrichGPUs folds nvidia-smi telemetry into the sysfs GPU list.
//
// sysfs knows a card is there and which driver is bound to it; only the
// driver knows the model name, how much memory it has, and how busy it
// is. The two are matched positionally among NVIDIA cards, which is sound
// because both enumerate in PCI bus order — nvidia-smi's `index` is
// assigned that way, and readPCIDevices sorts by address.
//
// When sysfs read nothing but nvidia-smi answered — a masked `/sys` with
// the NVIDIA runtime still injected — the cards are synthesised from the
// telemetry alone, so the panel is not empty for the one case where we
// demonstrably do know what is installed.
func (h *Handler) enrichGPUs(r *http.Request, inventory *hardware.Inventory) {
	if h.gpu == nil || !h.gpu.Available() {
		return
	}

	devices, err := h.gpu.Devices(r.Context())
	if err != nil {
		// A present-but-failing nvidia-smi usually means driver/library
		// skew after a host driver upgrade. Worth a log line; not worth
		// dropping the sysfs inventory we already have.
		h.app.Logger.Error("metrics: reading GPU telemetry for hardware inventory failed", "error", err)
		return
	}
	if len(devices) == 0 {
		return
	}

	next := 0
	for i := range inventory.GPUs {
		if next >= len(devices) {
			break
		}
		if !isNVIDIA(inventory.GPUs[i]) {
			continue
		}
		applyTelemetry(&inventory.GPUs[i], devices[next])
		next++
	}

	for ; next < len(devices); next++ {
		inventory.GPUs = append(inventory.GPUs, synthesiseGPU(devices[next]))
		inventory.Available = true
	}
}

// isNVIDIA reports whether a PCI row is an NVIDIA card, by vendor id
// rather than by the bound driver — a card sitting on `nouveau`, or on no
// driver at all, is still the card nvidia-smi would be reporting if the
// proprietary stack were loaded.
func isNVIDIA(device hardware.Device) bool {
	return strings.HasPrefix(device.Model, "10de")
}

func applyTelemetry(device *hardware.Device, telemetry gpu.Device) {
	if telemetry.Name != "" {
		device.Name = telemetry.Name
	}
	device.MemoryTotal = telemetry.MemoryTotal
	utilization := telemetry.Utilization
	device.Utilization = &utilization
	if telemetry.Temperature > 0 {
		temperature := telemetry.Temperature
		device.Temperature = &temperature
	}
}

// synthesiseGPU builds a row for a card nvidia-smi saw and sysfs did not.
// The PCI address is unknown in that case, so the driver's own index is
// the identifier.
func synthesiseGPU(telemetry gpu.Device) hardware.Device {
	device := hardware.Device{
		ID:     "nvidia-" + strconv.Itoa(telemetry.Index),
		Kind:   hardware.KindGPU,
		Bus:    hardware.BusPCI,
		Name:   telemetry.Name,
		Driver: "nvidia",
	}
	applyTelemetry(&device, telemetry)
	return device
}
