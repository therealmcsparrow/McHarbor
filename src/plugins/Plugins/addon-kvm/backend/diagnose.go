// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// AI-assisted diagnosis of a guest that will not start, or that crashed.
//
// This is the one place a plugin can add real value with a model: the failure
// modes here are numerous, the symptoms are often remote from the cause, and
// the evidence — a definition plus an exit reason — is small, structured and
// safe to send.
//
// # What crosses the boundary, and what does not
//
// Only the guest's own definition and its last error. Never a console stream,
// never a disk, never a cloud-init seed (which carries SSH keys), never a
// password hash, never an address. The prompt below is the complete list of
// what leaves this process, and it is built field by field rather than by
// serializing the model — so a field added to VM later cannot silently start
// being sent.

// maxDiagnosisTokens keeps the answer to a paragraph or two. An operator
// reading a diagnosis wants the likely cause and the next step, not an essay.
const maxDiagnosisTokens = 500

// diagnose asks the router what is likely wrong with a guest.
//
// Returns empty when no AI provider is configured or the call fails. That is
// not an error the operator needs to see: this feature is additive, and the
// detail panel simply does not offer it.
func (h *Handler) diagnose(ctx context.Context, vm VM) string {
	if h.ai == nil || !h.ai.Available() {
		return ""
	}

	facts := buildDiagnosisFacts(vm)
	maxTokens := maxDiagnosisTokens

	resp, err := h.ai.Complete(ctx, plugin.AIRequest{
		MaxTokens: &maxTokens,
		Messages: []plugin.AIMessage{
			{
				Role: "system",
				Content: "You help an operator diagnose a KVM virtual machine that will not " +
					"start or that stopped unexpectedly. You are given the guest's " +
					"definition and its last error. Give the most likely cause and the " +
					"single next thing to check or try. Be specific about QEMU, KVM, " +
					"cloud-init and storage behaviour. If the evidence does not support a " +
					"conclusion, say what additional information would settle it rather " +
					"than guessing. Two short paragraphs at most, plain prose, no headings.",
			},
			{Role: "user", Content: facts},
		},
	})
	if err != nil {
		h.logger.Debug("virtual machine diagnosis unavailable", "vm", vm.ID, "error", err)
		return ""
	}
	return strings.TrimSpace(resp.Content)
}

// buildDiagnosisFacts assembles the prompt, field by field.
//
// Built explicitly rather than by marshalling the model: that way a field added
// to VM later — a guest-reported hostname, a stored credential — cannot start
// being sent to a third party without somebody deciding to send it.
func buildDiagnosisFacts(vm VM) string {
	var b strings.Builder

	b.WriteString("Virtual machine that is not behaving as expected.\n\n")
	fmt.Fprintf(&b, "State: %s\n", vm.State)
	if vm.LastError != "" {
		// The single most useful line: the agent reads this from the host's
		// journal, so an OOM kill says so rather than presenting as a generic
		// failure.
		fmt.Fprintf(&b, "Last error: %s\n", vm.LastError)
	}

	fmt.Fprintf(&b, "\nHardware:\n")
	fmt.Fprintf(&b, "- %d vCPU (%s), %d MB memory\n", vm.VCPUs, vm.CPUModel, vm.MemoryMB)
	fmt.Fprintf(&b, "- Machine type %s, %s firmware", vm.MachineType, vm.Firmware)
	if vm.SecureBoot {
		b.WriteString(", Secure Boot on")
	}
	if vm.TPMEnabled {
		b.WriteString(", vTPM on")
	}
	fmt.Fprintf(&b, "\n- Display %s\n", vm.DisplayType)
	if vm.GuestOSFamily != "" {
		fmt.Fprintf(&b, "- Guest OS family: %s %s\n", vm.GuestOSFamily, vm.GuestOSVariant)
	}

	if len(vm.Disks) > 0 {
		b.WriteString("\nDisks:\n")
		for _, disk := range vm.Disks {
			fmt.Fprintf(&b, "- slot %d, %s bus, %s format on a %s pool, %s",
				disk.Slot, disk.Bus, disk.Format, disk.PoolBackend, humanBytes(disk.SizeBytes))
			if disk.BackingPath != "" {
				// Whether a disk is copy-on-write over an image matters: a
				// missing or evicted backing file is a common cause of a guest
				// that will not boot.
				b.WriteString(", copy-on-write over a backing image")
			}
			if disk.IsBoot {
				b.WriteString(", boot disk")
			}
			if disk.IsCDROM {
				b.WriteString(", CD-ROM")
			}
			b.WriteString("\n")
		}
	}

	if len(vm.NICs) > 0 {
		b.WriteString("\nNetwork:\n")
		for _, nic := range vm.NICs {
			fmt.Fprintf(&b, "- slot %d, %s on bridge %s", nic.Slot, nic.Model, nic.Bridge)
			if nic.VLANTag != 0 {
				fmt.Fprintf(&b, ", VLAN %d", nic.VLANTag)
			}
			b.WriteString("\n")
		}
	}

	if len(vm.Devices) > 0 {
		b.WriteString("\nPassed-through host devices:\n")
		for _, device := range vm.Devices {
			fmt.Fprintf(&b, "- %s at %s\n", device.Kind, device.HostAddress)
		}
	}

	// Whether cloud-init is configured, and how the network is meant to come
	// up — but never the SSH keys and never the password hash.
	if vm.CloudInit != nil {
		fmt.Fprintf(&b, "\ncloud-init is configured: %s networking, %d SSH keys, password %s.\n",
			vm.CloudInit.NetworkMode, len(vm.CloudInit.SSHKeys),
			yesNo(vm.CloudInit.HasPassword))
	} else {
		b.WriteString("\ncloud-init is not configured; this guest boots from an installer or an existing disk.\n")
	}

	if vm.GuestAgentState != "" {
		fmt.Fprintf(&b, "QEMU guest agent: %s\n", vm.GuestAgentState)
	}
	if len(vm.Snapshots) > 0 {
		fmt.Fprintf(&b, "Snapshots: %d, deepest chain %d.\n",
			len(vm.Snapshots), vm.Snapshots[len(vm.Snapshots)-1].ChainDepth)
	}

	return b.String()
}

func yesNo(value bool) string {
	if value {
		return "set"
	}
	return "not set"
}
