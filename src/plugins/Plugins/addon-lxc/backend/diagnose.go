// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// AI-assisted diagnosis of a container that will not start, or that exited.
//
// This is the one place a plugin can add real value with a model: the failure
// modes here are numerous and unusually opaque. A container that exits
// immediately might have the wrong init for its rootfs, a missing delegated
// cgroup subtree, an id range that does not cover the uids inside its image, a
// bind mount whose source does not exist, or a memory.max below what its init
// needs to reach a shell — and all five present identically as "the container
// stopped".
//
// # What crosses the boundary, and what does not
//
// Only the container's own definition, its limits and its exit reason. Never a
// console stream, never a directory listing of the rootfs, never the contents
// of a mount, never a password hash, never an SSH key, never an address. The
// prompt below is the complete list of what leaves this process, and it is
// built field by field rather than by serializing the model — so a field added
// to Container later cannot silently start being sent.

// maxDiagnosisTokens keeps the answer to a paragraph or two. An operator
// reading a diagnosis wants the likely cause and the next step, not an essay.
const maxDiagnosisTokens = 500

// HandleDiagnose serves POST /api/system-containers/{id}/diagnose.
func (h *Handler) HandleDiagnose(w http.ResponseWriter, r *http.Request) {
	ct, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "diagnose container", err)
		return
	}

	answer := h.diagnose(r.Context(), ct)
	if answer == "" {
		// Not an error: an installation with no AI provider simply does not
		// have this, and saying so is better than a failure the operator
		// cannot act on.
		ok(w, map[string]any{"available": false})
		return
	}
	ok(w, map[string]any{"available": true, "diagnosis": answer})
}

// diagnose asks the router what is likely wrong with a container.
//
// Returns empty when no AI provider is configured or the call fails. That is
// not an error the operator needs to see: this feature is additive, and the
// detail panel simply does not offer it.
func (h *Handler) diagnose(ctx context.Context, ct Container) string {
	if h.ai == nil || !h.ai.Available() {
		return ""
	}

	facts := buildDiagnosisFacts(ct)
	maxTokens := maxDiagnosisTokens

	resp, err := h.ai.Complete(ctx, plugin.AIRequest{
		MaxTokens: &maxTokens,
		Messages: []plugin.AIMessage{
			{
				Role: "system",
				Content: "You help an operator diagnose an LXC system container that will " +
					"not start or that exited unexpectedly. You are given the container's " +
					"definition, its cgroup v2 limits and its exit reason. Give the most " +
					"likely cause and the single next thing to check or try. Be specific " +
					"about user namespaces and id mapping, cgroup v2 limits and the OOM " +
					"killer, systemd inside a container (delegated cgroup subtree, tmpfs " +
					"on /run, SIGRTMIN+3 as the stop signal), idmapped versus chowned " +
					"mounts, and rootfs layout. If the evidence does not support a " +
					"conclusion, say what additional information would settle it rather " +
					"than guessing. Two short paragraphs at most, plain prose, no headings.",
			},
			{Role: "user", Content: facts},
		},
	})
	if err != nil {
		h.logger.Debug("container diagnosis unavailable", "container", ct.ID, "error", err)
		return ""
	}
	return strings.TrimSpace(resp.Content)
}

// buildDiagnosisFacts assembles the prompt, field by field.
//
// Built explicitly rather than by marshalling the model: that way a field added
// to Container later — an observed hostname, a stored credential — cannot start
// being sent to a third party without somebody deciding to send it.
func buildDiagnosisFacts(ct Container) string {
	var b strings.Builder

	b.WriteString("System container that is not behaving as expected.\n\n")
	fmt.Fprintf(&b, "State: %s\n", ct.State)
	if ct.ExitReason != "" {
		// The most useful single line. The agent reads this from the unit's
		// result, so an OOM kill or a signal says so rather than presenting as
		// a generic failure.
		fmt.Fprintf(&b, "Exit reason: %s\n", ct.ExitReason)
	}
	if ct.LastError != "" {
		fmt.Fprintf(&b, "Last error: %s\n", ct.LastError)
	}

	b.WriteString("\nProfile:\n")
	fmt.Fprintf(&b, "- init system: %s\n", ct.InitSystem)
	if ct.OSFamily != "" {
		fmt.Fprintf(&b, "- OS family: %s %s\n", ct.OSFamily, ct.OSVersion)
	}
	if ct.Privileged {
		b.WriteString("- privileged: no user namespace\n")
	} else {
		// The id range is the single most common cause of a container that
		// unpacks and then cannot execute anything: a rootfs whose files are
		// owned by uids outside the mapped range appears to belong to nobody.
		fmt.Fprintf(&b, "- unprivileged, uid/gid range %d..%d, %s mapping\n",
			ct.UIDMapStart, ct.UIDMapStart+ct.UIDMapCount, ct.IDMapMechanism)
	}
	if ct.Nesting {
		b.WriteString("- nesting enabled\n")
	}
	fmt.Fprintf(&b, "- rootfs on a %s pool\n", ct.PoolBackend)
	if ct.TemplatePath != "" {
		b.WriteString("- copy-on-write from a template rootfs\n")
	}

	b.WriteString("\nLimits (cgroup v2):\n")
	writeLimits(&b, "requested", ct.Limits)
	if ct.Applied != nil {
		// Both, because the gap between them is itself diagnostic: a memory.max
		// the kernel clamped to something far smaller explains an init that
		// never reaches a shell.
		writeLimits(&b, "actually applied", *ct.Applied)
	}

	if len(ct.NICs) > 0 {
		b.WriteString("\nNetwork:\n")
		for _, nic := range ct.NICs {
			fmt.Fprintf(&b, "- %s on bridge %s, %s", nic.GuestName, nic.Bridge, nic.AddressMode)
			if nic.VLANTag != 0 {
				fmt.Fprintf(&b, ", VLAN %d", nic.VLANTag)
			}
			b.WriteString("\n")
		}
	}

	if len(ct.Mounts) > 0 {
		b.WriteString("\nMounts:\n")
		for _, mount := range ct.Mounts {
			// The destination and the kind, never the source. A bind source is
			// a host path, and host paths are not something to hand to a third
			// party to help debug a container.
			fmt.Fprintf(&b, "- %s at %s", mount.Kind, mount.Destination)
			if mount.ReadOnly {
				b.WriteString(", read-only")
			}
			if !mount.ShiftIDs {
				// Worth calling out: an unshifted mount in an unprivileged
				// container is owned by nobody from the inside, and that is a
				// frequent and confusing failure.
				b.WriteString(", ids not shifted")
			}
			b.WriteString("\n")
		}
	}

	if len(ct.Snapshots) > 0 {
		fmt.Fprintf(&b, "\nSnapshots: %d, deepest chain %d.\n",
			len(ct.Snapshots), ct.Snapshots[0].ChainDepth)
	}

	return b.String()
}

func writeLimits(b *strings.Builder, label string, limits Limits) {
	fmt.Fprintf(b, "- %s: memory max %s", label, humanBytes(limits.MemoryMaxBytes))
	if limits.MemoryHighBytes > 0 {
		fmt.Fprintf(b, " (throttle at %s)", humanBytes(limits.MemoryHighBytes))
	}
	if limits.SwapMaxBytes > 0 {
		fmt.Fprintf(b, ", swap %s", humanBytes(limits.SwapMaxBytes))
	} else {
		b.WriteString(", swap disabled")
	}
	if limits.CPUMax != "" {
		fmt.Fprintf(b, ", cpu.max %q", limits.CPUMax)
	}
	if limits.CPUSet != "" {
		fmt.Fprintf(b, ", cpuset %s", limits.CPUSet)
	}
	fmt.Fprintf(b, ", pids max %d\n", limits.PidsMax)
}

func humanBytes(bytes int64) string {
	if bytes <= 0 {
		return "unset"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
