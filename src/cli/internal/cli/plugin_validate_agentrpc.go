// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"fmt"
	"slices"
)

// pluginAgentRPCSend mirrors the host's core/agent plugin route ceiling, the agent messages
// a plugin may send. Both copies are held to src/contracts/plugincaps/agent_rpc_ceiling.yaml.
var pluginAgentRPCSend = []string{
	"compute_config_request", "compute_image_fetch", "compute_image_list", "compute_image_remove", "compute_install_request",
	"compute_migrate_request", "compute_probe_request",
	"ct_backup_restore", "ct_backup_run", "ct_cancel", "ct_clone_request", "ct_create",
	"ct_delete", "ct_freeze", "ct_processes", "ct_query", "ct_restart", "ct_shutdown",
	"ct_snapshot_create", "ct_snapshot_delete", "ct_snapshot_revert", "ct_start", "ct_stop",
	"ct_unfreeze", "ct_update_limits", "host_device_list", "net_detect_request", "net_ensure_request",
	"net_remove_request", "pool_browse_request", "pool_create_request", "pool_detect_request",
	"pool_file_request", "pool_remove_request", "pool_stats_request", "vm_backup_restore", "vm_backup_run", "vm_cancel", "vm_clone_request",
	"vm_cpu_set", "vm_define", "vm_delete", "vm_disk_create", "vm_disk_delete", "vm_disk_hotplug", "vm_disk_list",
	"vm_disk_resize", "vm_guest_agent_disc", "vm_guest_query", "vm_image_inspect",
	"vm_import_convert", "vm_import_discard", "vm_import_inspect", "vm_import_stage", "vm_log",
	"vm_media_change", "vm_memory_set", "vm_migrate", "vm_nic_hotplug", "vm_pause", "vm_qemu_capability", "vm_query", "vm_reset",
	"vm_resume", "vm_shutdown", "vm_snapshot_create", "vm_snapshot_delete", "vm_snapshot_revert",
	"vm_spice_ticket", "vm_start", "vm_stats", "vm_stop", "vm_throttle_set",
}

// pluginAgentRPCObserve mirrors the host's observe table, the unsolicited agent frames a
// plugin may subscribe to.
var pluginAgentRPCObserve = []string{
	"compute_migrate_progress", "ct_backup_progress", "net_lease_event", "vm_event",
	"vm_import_progress",
}

// validateCapAgentRPC mirrors the host's plugincaps.validateAgentRPC.
func validateCapAgentRPC(c pluginCapDecl) error {
	seen := map[string]bool{}
	for _, msgType := range c.Send {
		if !slices.Contains(pluginAgentRPCSend, msgType) {
			return fmt.Errorf("send: %q is outside the agent-rpc ceiling; a plugin may send only the agent messages the host lists", msgType)
		}
		if seen[msgType] {
			return fmt.Errorf("send: %q is listed twice", msgType)
		}
		seen[msgType] = true
	}
	seen = map[string]bool{}
	for _, msgType := range c.Observe {
		if !slices.Contains(pluginAgentRPCObserve, msgType) {
			return fmt.Errorf("observe: %q is not an observable agent frame", msgType)
		}
		if seen[msgType] {
			return fmt.Errorf("observe: %q is listed twice", msgType)
		}
		seen[msgType] = true
	}
	return nil
}
