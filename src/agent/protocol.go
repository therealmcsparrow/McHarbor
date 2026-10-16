// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SYNC: This file must match src/backend/core/agent/protocol.go.
// Run `make check-protocol` to verify sync.

package main

import "encoding/json"

// WebSocket message types exchanged between server and agent.
const (
	// Auth handshake
	MsgAuth       = "auth"        // Agent->Server: token + hostname + Docker version
	MsgAuthResult = "auth_result" // Server->Agent: accept/reject with envID

	// Keepalive
	MsgPing = "ping" // Bidirectional
	MsgPong = "pong" // Bidirectional

	// HTTP proxy (Docker API calls)
	MsgHTTPRequest      = "http_request"       // Server->Agent: proxied Docker API call
	MsgHTTPRequestStart = "http_request_start" // Server->Agent: proxied Docker API call with streamed request body
	MsgHTTPRequestChunk = "http_request_chunk" // Server->Agent: request body chunk
	MsgHTTPRequestEnd   = "http_request_end"   // Server->Agent: end of request body stream
	MsgHTTPResponse     = "http_response"      // Agent->Server: full response

	// Streaming responses (logs, stats, exec)
	MsgHTTPResponseStart = "http_response_start" // Agent->Server: streaming header
	MsgHTTPResponseChunk = "http_response_chunk" // Agent->Server: binary data chunk
	MsgHTTPResponseEnd   = "http_response_end"   // Agent->Server: end of stream

	// Cancellation
	MsgHTTPCancel = "http_cancel" // Server->Agent: cancel in-flight request

	// Exec session (terminal over agent)
	MsgExecStart  = "exec_start"  // Server->Agent: start exec attach
	MsgExecInput  = "exec_input"  // Server->Agent: stdin data
	MsgExecOutput = "exec_output" // Agent->Server: stdout data
	MsgExecResize = "exec_resize" // Server->Agent: terminal resize
	MsgExecEnd    = "exec_end"    // Bidirectional: exec session ended

	// Compose stack commands
	MsgComposeRun    = "compose_run"    // Server->Agent: run docker compose in a staged project
	MsgComposeResult = "compose_result" // Agent->Server: compose command result
	MsgComposeCancel = "compose_cancel" // Server->Agent: cancel compose command

	// Direct agent-to-agent transfers
	MsgTransferPrepare  = "transfer_prepare"  // Server->Target agent: prepare upload receiver
	MsgTransferReady    = "transfer_ready"    // Target agent->Server: receiver status
	MsgTransferImage    = "transfer_image"    // Server->Source agent: stream Docker image to target
	MsgTransferArchive  = "transfer_archive"  // Server->Source agent: stream container archive to target
	MsgTransferProbe    = "transfer_probe"    // Server->Source agent: test direct reachability to target
	MsgTransferRestore  = "transfer_restore"  // Server->Agent: pull restore archive from McHarbor and apply locally
	MsgTransferProgress = "transfer_progress" // Source agent->Server: bytes sent directly
	MsgTransferResult   = "transfer_result"   // Agent->Server: direct transfer result
	MsgTransferCancel   = "transfer_cancel"   // Server->Agent: cancel direct transfer

	// Agent-side container backup and restore
	MsgBackupRun     = "backup_run"     // Server->Agent: create archive locally and upload it
	MsgBackupRestore = "backup_restore" // Server->Agent: download archive locally and restore entries

	// Host metrics and prune (agent reports /proc data and runs docker prune locally)
	MsgHostStatsRequest  = "host_stats_request"  // Server->Agent: collect host stats
	MsgHostStatsResponse = "host_stats_response" // Agent->Server: host stats snapshot
	MsgPruneRequest      = "prune_request"       // Server->Agent: run docker prune
	MsgPruneResponse     = "prune_response"      // Agent->Server: prune result

	// Host terminal (interactive shell on the remote host, streams over WS)
	MsgHostTerminalStart  = "host_terminal_start"  // Server->Agent: start a shell session
	MsgHostTerminalInput  = "host_terminal_input"  // Server->Agent: stdin chunk
	MsgHostTerminalOutput = "host_terminal_output" // Agent->Server: stdout chunk
	MsgHostTerminalResize = "host_terminal_resize" // Server->Agent: TTY resize
	MsgHostTerminalEnd    = "host_terminal_end"    // Bidirectional: session ended

	// Host log streaming (bounded journal/file snapshot over WS)
	MsgHostLogRequest = "host_log_request" // Server->Agent: read host logs
	MsgHostLogChunk   = "host_log_chunk"   // Agent->Server: log line chunk
	MsgHostLogEnd     = "host_log_end"     // Agent->Server: log stream complete

	// Host OS inventory and package updates. The check is one-shot, the apply
	// streams because a distribution upgrade runs for minutes and the operator
	// needs to see it move.
	MsgHostOSInfoRequest  = "host_os_info_request"  // Server->Agent: describe the host OS
	MsgHostOSInfoResponse = "host_os_info_response" // Agent->Server: host OS description
	MsgHostUpdateCheck    = "host_update_check"     // Server->Agent: list pending packages
	MsgHostUpdateResult   = "host_update_result"    // Agent->Server: pending package list
	MsgHostUpdateApply    = "host_update_apply"     // Server->Agent: run the upgrade
	MsgHostUpdateChunk    = "host_update_chunk"     // Agent->Server: one line of upgrade output
	MsgHostUpdateEnd      = "host_update_end"       // Agent->Server: upgrade finished

	// Host systemd services. One request/reply pair carries every operation —
	// list, lifecycle action, cat, drop-in read/write/revert, daemon-reload —
	// selected by HostServicePayload.Op, so the transport gains one waiter map
	// rather than seven. Native Linux agents only.
	MsgHostServiceRequest  = "host_service_request"  // Server->Agent: one systemd operation
	MsgHostServiceResponse = "host_service_response" // Agent->Server: its result

	// Host links: an operator-named macvlan child on the host itself, so the
	// host can reach guests and containers on a macvlan network. One pair
	// carries ensure and remove, selected by HostLinkPayload.Op.
	MsgHostLinkRequest  = "host_link_request"  // Server->Agent: ensure or remove one host link
	MsgHostLinkResponse = "host_link_response" // Agent->Server: its result

	// Agent self-update (agent replaces its own container or binary)
	MsgAgentUpdateRequest  = "agent_update_request"  // Server->Agent: update to the given version
	MsgAgentUpdateResponse = "agent_update_response" // Agent->Server: accepted / failed

	// Offline buffer replay (agent stored data locally while disconnected)
	MsgOfflineBatch    = "offline_batch"     // Agent->Server: one bounded batch of buffered rows
	MsgOfflineBatchAck = "offline_batch_ack" // Server->Agent: batch persisted (or rejected)
	MsgTraceSpans      = "trace_spans"       // Agent->Server: spans for a traced proxied request

	// Push-based container stats. The server subscribes a capable agent; the
	// agent streams Docker's own stats locally and pushes one batch per
	// interval, one in flight at a time, and the ack carries the next interval.
	MsgStatsSubscribe = "stats_subscribe" // Server->Agent: start (or retune) pushing
	MsgStatsPush      = "stats_push"      // Agent->Server: one interval's samples
	MsgStatsPushAck   = "stats_push_ack"  // Server->Agent: applied/duplicate + next interval, or stop

	// Snapshot family: filesystem-level snapshots (zfs, btrfs, lvm on
	// Linux; VSS on Windows) so a container backup can read a frozen
	// view of its mounts instead of pausing the container. The agent
	// owns the CLI calls because only it can see the host's PATH.
	MsgSnapshotDetectRequest      = "snapshot_detect_request"      // Server->Agent: which snapshot drivers exist here?
	MsgSnapshotDetectResponse     = "snapshot_detect_response"     // Agent->Server: one entry per usable driver
	MsgSnapshotCapabilityRequest  = "snapshot_capability_request"  // Server->Agent: list targets for one driver
	MsgSnapshotCapabilityResponse = "snapshot_capability_response" // Agent->Server: targets + tool version
	MsgSnapshotCreateRequest      = "snapshot_create_request"      // Server->Agent: freeze the given targets
	MsgSnapshotCreateResponse     = "snapshot_create_response"     // Agent->Server: snapshot name + mount path
	MsgSnapshotRemoveRequest      = "snapshot_remove_request"      // Server->Agent: destroy a snapshot
	MsgSnapshotRemoveResponse     = "snapshot_remove_response"     // Agent->Server: removal result

	// Compute node capability: what this host can actually run. Probed on
	// agent connect, on demand from the UI, and every six hours. Every
	// reported item carries its own remediation string so the UI renders a
	// checklist with copyable install commands rather than a generic
	// "capability check failed".
	MsgComputeProbeRequest    = "compute_probe_request"    // Server->Agent: what can this host run?
	MsgComputeProbeResponse   = "compute_probe_response"   // Agent->Server: capability report
	MsgComputeInstallRequest  = "compute_install_request"  // Server->Agent: install the packages for one capability item
	MsgComputeInstallResponse = "compute_install_response" // Agent->Server: what was installed, and the fresh capability

	// Compute locations: where this host keeps VM state, container rootfs
	// trees and cached images. The agent ships defaults under
	// /var/lib/mcharbor/compute, and McHarbor overrides them so an operator
	// can put guests on a SAN or NAS and have them reachable from more than
	// one host. Sent on connect and whenever the operator changes a location.
	//
	// Applied only when the host is running no guests: the paths are baked
	// into live QEMU command lines and libcontainer state, so moving them
	// underneath a running guest would strand it. The agent refuses and names
	// what is in the way rather than half-applying.
	MsgComputeConfigRequest  = "compute_config_request"  // Server->Agent: use these locations
	MsgComputeConfigResponse = "compute_config_response" // Agent->Server: the locations now in effect

	MsgComputeMigrateRequest  = "compute_migrate_request"  // Server->Agent: move what is already here
	MsgComputeMigrateProgress = "compute_migrate_progress" // Agent->Server: how far the move has got
	MsgComputeMigrateResponse = "compute_migrate_response" // Agent->Server: the plan, or every path moved

	// Console family: guest byte streams — KVM VNC/SPICE/serial and LXC
	// PID 1 tty / exec. Shaped exactly like exec_* and host_terminal_*
	// because hijacked byte streams already proved they cannot ride the
	// http_* proxy path.
	//
	// ORDERING HAZARD: the StreamReader for a session must exist before the
	// first console_output frame is dispatched. The http_response_start bug
	// was exactly this, and here it manifests as an RFB handshake that
	// hangs forever rather than a short response that stalls.
	MsgConsoleStart  = "console_start"  // Server->Agent: open a guest console session
	MsgConsoleInput  = "console_input"  // Server->Agent: client bytes
	MsgConsoleOutput = "console_output" // Agent->Server: guest bytes
	MsgConsoleResize = "console_resize" // Server->Agent: TTY resize (text consoles only)
	MsgConsoleEnd    = "console_end"    // Bidirectional: session ended

	// Storage pools: a named place on one host where guest volumes live.
	// Detection reuses the snapshot driver detection wholesale and adds
	// capacity reporting.
	MsgPoolDetectRequest  = "pool_detect_request"  // Server->Agent: enumerate usable pool backends
	MsgPoolDetectResponse = "pool_detect_response" // Agent->Server: one entry per backend
	MsgPoolCreateRequest  = "pool_create_request"  // Server->Agent: initialize a pool
	MsgPoolCreateResponse = "pool_create_response" // Agent->Server: result + usable capacity
	MsgPoolStatsRequest   = "pool_stats_request"   // Server->Agent: capacity sample
	MsgPoolStatsResponse  = "pool_stats_response"  // Agent->Server: used/free/allocated
	MsgPoolRemoveRequest  = "pool_remove_request"  // Server->Agent: detach a pool
	MsgPoolRemoveResponse = "pool_remove_response" // Agent->Server: removal result

	// Pool file access. A pool is a directory tree on the host, and the
	// operator who can create guests on it is the operator who needs to see
	// what is already there — an orphaned disk left by a failed import, a
	// cloud-init seed to correct, an ISO dropped in by hand over scp.
	//
	// CONFINEMENT: the agent runs as root, so every path in these messages is
	// relative to the pool's own Target and is resolved against it on the
	// agent. An absolute path, a `..` segment, or a symlink whose destination
	// leaves the pool is refused there — never merely validated here, because
	// the server is the untrusted side of this boundary.
	MsgPoolBrowseRequest  = "pool_browse_request"  // Server->Agent: list one directory in a pool
	MsgPoolBrowseResponse = "pool_browse_response" // Agent->Server: directory entries
	MsgPoolFileRequest    = "pool_file_request"    // Server->Agent: read/write/mkdir/delete/rename
	MsgPoolFileResponse   = "pool_file_response"   // Agent->Server: outcome, and content on a read

	// Guest networking. Bridged mode attaches a tap or veth to an existing
	// host bridge and never touches the physical link; managed mode creates
	// mcharbor-br0 and owns its subnet, NAT and DHCP.
	MsgNetDetectRequest  = "net_detect_request"  // Server->Agent: enumerate host links and bridges
	MsgNetDetectResponse = "net_detect_response" // Agent->Server: link inventory
	MsgNetEnsureRequest  = "net_ensure_request"  // Server->Agent: create/verify a managed network
	MsgNetEnsureResponse = "net_ensure_response" // Agent->Server: result
	MsgNetRemoveRequest  = "net_remove_request"  // Server->Agent: tear down a managed network
	MsgNetRemoveResponse = "net_remove_response" // Agent->Server: removal result
	MsgNetLeaseEvent     = "net_lease_event"     // Agent->Server: DHCP lease granted/expired
	// External-switch confirmation. Adopting the host's own adapter arms a
	// rollback on the agent; this is what cancels it, and McHarbor only sends
	// it once it has seen the agent still answering after the change. Silence
	// is therefore the safe case rather than the ambiguous one.
	MsgNetAdoptConfirm   = "net_adopt_confirm"   // Server->Agent: keep the external switch
	MsgNetAdoptConfirmed = "net_adopt_confirmed" // Agent->Server: rollback cancelled

	// Image catalog. The agent downloads straight from the source to the
	// host; images never transit the backend, because a 700 MB qcow2 over
	// the WS transport would compete with console traffic for no gain.
	MsgImageFetchRequest  = "compute_image_fetch"    // Server->Agent: download + verify an image
	MsgImageFetchProgress = "compute_image_progress" // Agent->Server: bytes/percent
	MsgImageFetchResult   = "compute_image_result"   // Agent->Server: local path + checksum
	MsgImageListRequest   = "compute_image_list"     // Server->Agent: what is cached locally?
	MsgImageListResponse  = "compute_image_listed"   // Agent->Server: cached image inventory
	MsgImageRemoveRequest = "compute_image_remove"   // Server->Agent: evict a cached image
	MsgImageRemoveResult  = "compute_image_removed"  // Agent->Server: removal result
	MsgImageCancelRequest = "compute_image_cancel"   // Server->Agent: abort an in-flight fetch

	// KVM guests. Every VM runs as a systemd transient unit rather than an
	// agent child, so guests survive an agent restart, an agent crash, and
	// the agent self-update path that replaces the running binary.
	MsgVMDefine   = "vm_define"   // Server->Agent: materialize disks + unit, do not start
	MsgVMStart    = "vm_start"    // Server->Agent: start the transient unit
	MsgVMShutdown = "vm_shutdown" // Server->Agent: ACPI powerdown with grace period
	MsgVMStop     = "vm_stop"     // Server->Agent: immediate quit
	MsgVMReset    = "vm_reset"    // Server->Agent: hard reset
	MsgVMPause    = "vm_pause"    // Server->Agent: stop vCPUs
	MsgVMResume   = "vm_resume"   // Server->Agent: cont
	MsgVMDelete   = "vm_delete"   // Server->Agent: remove unit + optionally volumes
	MsgVMResult   = "vm_result"   // Agent->Server: outcome of any of the above

	MsgVMQuery = "vm_query" // Server->Agent: reconcile all VM state on this host
	MsgVMState = "vm_state" // Agent->Server: per-VM state snapshot (also unsolicited)
	MsgVMEvent = "vm_event" // Agent->Server: QMP async event (SHUTDOWN, panic, io error)
	MsgVMStats = "vm_stats" // Agent->Server: sampled cpu/mem/disk/net
	// MsgHostDeviceList asks a host what passthrough devices it has, so the
	// attach dialog can offer a list instead of asking for a PCI address typed
	// from memory.
	MsgHostDeviceList = "host_device_list" // Server->Agent: enumerate attachable devices

	MsgVMLog        = "vm_log"         // Server->Agent: tail a guest's own journal units
	MsgVMDiskList   = "vm_disk_list"   // Server->Agent: enumerate the volumes in a pool
	MsgVMDiskCreate = "vm_disk_create" // Server->Agent: allocate a volume in a pool
	MsgVMDiskResize = "vm_disk_resize" // Server->Agent: grow a volume (never shrink)
	MsgVMDiskDelete = "vm_disk_delete" // Server->Agent: remove a volume
	MsgVMDiskResult = "vm_disk_result" // Agent->Server: outcome + actual size

	MsgVMSnapshotCreate = "vm_snapshot_create" // Server->Agent: snapshot per pool mechanism
	MsgVMSnapshotRevert = "vm_snapshot_revert" // Server->Agent: revert (destructive)
	MsgVMSnapshotDelete = "vm_snapshot_delete" // Server->Agent: drop a snapshot
	MsgVMSnapshotResult = "vm_snapshot_result" // Agent->Server: outcome

	// Guest import. A disk written by VMware, Hyper-V, VirtualBox or Parallels
	// is a format qemu-img already reads, so importing one is an inspection
	// and a conversion rather than an emulation problem. An OVA is a tar of a
	// descriptor and those disks, so the descriptor also supplies the guest's
	// shape — processors, memory, firmware — instead of the operator retyping
	// it from the old hypervisor.
	//
	// Both run on the agent, never on McHarbor: a 200 GB VMDK would otherwise
	// cross the transport twice, and qemu-img is a dependency the compute host
	// already has and the McHarbor container does not.
	MsgVMImportInspect  = "vm_import_inspect"  // Server->Agent: what is in this file?
	MsgVMImportConvert  = "vm_import_convert"  // Server->Agent: convert it into a pool volume
	MsgVMImportStage    = "vm_import_stage"    // Server->Agent: pull the upload and keep it
	MsgVMImportDiscard  = "vm_import_discard"  // Server->Agent: drop the staged copy
	MsgVMImportProgress = "vm_import_progress" // Agent->Server: percent
	MsgVMImportResult   = "vm_import_result"   // Agent->Server: volumes + the guest's shape

	MsgVMCloneRequest  = "vm_clone_request"  // Server->Agent: linked or full clone
	MsgVMCloneProgress = "vm_clone_progress" // Agent->Server: percent for full clones
	MsgVMCloneResult   = "vm_clone_result"   // Agent->Server: new volume paths

	// Guest-agent reads only. guest-exec is deliberately never exposed: it
	// is arbitrary code execution inside the guest with no audit trail the
	// guest can be trusted to keep, and it would need a permission stronger
	// than vm.console to be honest about what it does.
	MsgVMGuestQuery = "vm_guest_query" // Server->Agent: guest-agent query (NOT guest-exec)
	// MsgVMGuestAgentDisc asks the host to build the guest-agent installer
	// disc for a guest and answer with its path, so the server can attach it
	// as a CD-ROM without knowing anything about the host's filesystem.
	MsgVMGuestAgentDisc = "vm_guest_agent_disc" // Server->Agent: build the guest-agent disc
	MsgVMGuestInfo      = "vm_guest_info"       // Agent->Server: IPs, osinfo, agent state

	// MsgVMQEMUCapability asks what the host's QEMU can actually do.
	//
	// It answers from the cached capability probe rather than starting
	// anything, and it exists as its own message because the host's shared
	// compute spec is ABI-linked into every plugin: carrying the QEMU feature
	// set as a typed field there would take every installed plugin offline.
	MsgVMQEMUCapability = "vm_qemu_capability" // Server->Agent: QEMU feature gate

	// Live resizes. Each one is a request the guest may decline, so the reply
	// carries what actually happened rather than an acknowledgement.
	MsgVMMemorySet   = "vm_memory_set"   // Server->Agent: virtio-mem resize
	MsgVMCPUSet      = "vm_cpu_set"      // Server->Agent: plug/unplug vCPUs
	MsgVMThrottleSet = "vm_throttle_set" // Server->Agent: live I/O limits on one disk

	// Live hardware changes. Each one is QMP hot-plug on a running guest, and a
	// refusal the server can act on (restart once, wrong bus, no empty drive)
	// comes back as Success with an ErrorCode rather than as a failure.
	MsgVMDiskHotplug = "vm_disk_hotplug" // Server->Agent: attach, detach or grow a disk on a running guest
	MsgVMMediaChange = "vm_media_change" // Server->Agent: insert or eject a disc in a running guest's drive
	MsgVMNICHotplug  = "vm_nic_hotplug"  // Server->Agent: attach, detach or relink a NIC on a running guest

	// MsgVMImageInspect runs one read-only qemu-img subcommand from a fixed
	// allow-list. Never a free command string: this runs as root on the host.
	MsgVMImageInspect = "vm_image_inspect" // Server->Agent: qemu-img check/info/map/measure/compare

	// Backup. The artifact is produced and staged on the host, then streamed
	// to McHarbor, which encrypts it in flight and passes it to the
	// destination — so neither the agent nor the plugin ever holds the
	// backup master key, and McHarbor never stages a guest-sized file.
	MsgVMBackupRun      = "vm_backup_run"      // Server->Agent: snapshot, produce the artifact, stream it back
	MsgVMBackupRestore  = "vm_backup_restore"  // Server->Agent: pull a chain and rebuild the disks
	MsgVMBackupProgress = "vm_backup_progress" // Agent->Server: stage and bytes
	MsgVMBackupResult   = "vm_backup_result"   // Agent->Server: per-disk outcome

	MsgVMMigrate         = "vm_migrate"          // Server->Agent: cold or live migration to a peer host
	MsgVMMigrateProgress = "vm_migrate_progress" // Agent->Server: percent + downtime estimate
	MsgVMMigrateResult   = "vm_migrate_result"   // Agent->Server: outcome
	MsgVMCancel          = "vm_cancel"           // Server->Agent: cancel clone / disk allocate / migrate

	// MsgVMSpiceTicket opens a short-lived, password-guarded TCP entrance to a
	// guest's SPICE server for a native remote-viewer. The browser client does
	// not need one: it rides the console_* stream exactly as VNC does.
	MsgVMSpiceTicket = "vm_spice_ticket" // Server->Agent: mint a remote-viewer ticket

	// LXC system containers. Driven through libcontainer in-process — no
	// daemon, no CLI, no second database that can drift from McHarbor's.
	MsgCTCreate   = "ct_create"   // Server->Agent: materialize rootfs + config, do not start
	MsgCTStart    = "ct_start"    // Server->Agent: start the transient unit
	MsgCTShutdown = "ct_shutdown" // Server->Agent: SIGRTMIN+3 (systemd) with grace period
	MsgCTStop     = "ct_stop"     // Server->Agent: SIGKILL the container cgroup
	MsgCTRestart  = "ct_restart"  // Server->Agent: shutdown then start
	MsgCTFreeze   = "ct_freeze"   // Server->Agent: cgroup freezer
	MsgCTUnfreeze = "ct_unfreeze" // Server->Agent: thaw
	MsgCTDelete   = "ct_delete"   // Server->Agent: remove unit + rootfs
	MsgCTResult   = "ct_result"   // Agent->Server: outcome of any of the above

	MsgCTQuery     = "ct_query"     // Server->Agent: reconcile all container state on this host
	MsgCTState     = "ct_state"     // Agent->Server: per-container state (also unsolicited)
	MsgCTStats     = "ct_stats"     // Agent->Server: sampled cgroup cpu/mem/pids/io + net
	MsgCTProcesses = "ct_processes" // Server->Agent: process list from the container cgroup
	MsgCTProcList  = "ct_proc_list" // Agent->Server: the process list

	// Live cgroup limit changes are a genuine advantage over VMs: CPU and
	// memory change on a running container with no restart.
	MsgCTUpdateLimits = "ct_update_limits" // Server->Agent: live cgroup limit change
	MsgCTLimitsResult = "ct_limits_result" // Agent->Server: applied values

	MsgCTSnapshotCreate = "ct_snapshot_create" // Server->Agent: snapshot per pool mechanism
	MsgCTSnapshotRevert = "ct_snapshot_revert" // Server->Agent: revert (destructive)
	MsgCTSnapshotDelete = "ct_snapshot_delete" // Server->Agent: drop a snapshot
	MsgCTSnapshotResult = "ct_snapshot_result" // Agent->Server: outcome

	MsgCTCloneRequest  = "ct_clone_request"  // Server->Agent: clone from template
	MsgCTCloneProgress = "ct_clone_progress" // Agent->Server: percent for dir-pool copies
	MsgCTCloneResult   = "ct_clone_result"   // Agent->Server: new rootfs path

	MsgCTCancel = "ct_cancel" // Server->Agent: cancel a clone or a backup

	// Container backup. The artifact is a compressed tar of the rootfs plus a
	// JSON manifest of the container's definition, staged on the host and then
	// streamed to McHarbor, which encrypts it in flight and hands it to the
	// destination — so neither the agent nor the plugin ever holds the backup
	// master key, and McHarbor never stages a rootfs-sized file.
	//
	// A tar rather than a pool-native send stream on purpose: a send stream
	// pins the restore to the backend it came from, and the common case is
	// restoring onto a different host whose pool is not the same mechanism.
	MsgCTBackupRun      = "ct_backup_run"      // Server->Agent: quiesce, produce the artifact, stream it back
	MsgCTBackupRestore  = "ct_backup_restore"  // Server->Agent: pull a chain and rebuild the rootfs
	MsgCTBackupProgress = "ct_backup_progress" // Agent->Server: stage and bytes
	MsgCTBackupResult   = "ct_backup_result"   // Agent->Server: outcome, and what was actually taken

	// Host power actions removed for stability; see commit removing host power.
)

// OfflineBatchPayload carries rows the agent buffered in its local store while
// it had no connection to the server. Exactly one of Logs / Events / Metrics is
// populated per batch, selected by Kind. The server answers with the same
// BatchID and Accepted=true once the rows are durable, and only then does the
// agent delete them locally — a batch that is never acked is replayed on the
// next connection, so ingest must be idempotent (dedupe on entry IDs).
type OfflineBatchPayload struct {
	BatchID   string               `json:"batchId"`
	Kind      string               `json:"kind"` // "logs" | "events" | "metrics"
	Logs      []OfflineLogEntry    `json:"logs,omitempty"`
	Events    []OfflineEventEntry  `json:"events,omitempty"`
	Metrics   []OfflineMetricEntry `json:"metrics,omitempty"`
	Remaining int64                `json:"remaining,omitempty"` // rows still queued after this batch
	Accepted  bool                 `json:"accepted,omitempty"`
	Error     string               `json:"error,omitempty"`
}

// TraceSpan is one span the agent measured for a request that arrived with a
// sampled traceparent. The agent has no exporter of its own, because it usually
// sits behind NAT; the server relays these to its collector. Timestamps are the
// agent host's clock.
type TraceSpan struct {
	TraceID       string            `json:"traceId"`
	SpanID        string            `json:"spanId"`
	ParentSpanID  string            `json:"parentSpanId,omitempty"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"` // server | client
	StartUnixNano int64             `json:"start"`
	EndUnixNano   int64             `json:"end"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// OfflineLogEntry is one line of the agent's own log captured while offline.
type OfflineLogEntry struct {
	ID       string `json:"id"`
	Level    string `json:"level"`
	Message  string `json:"message"`
	Attrs    string `json:"attrs,omitempty"` // JSON object of slog attributes
	LoggedAt string `json:"loggedAt"`        // RFC3339
}

// OfflineEventEntry is one Docker daemon event captured while offline.
type OfflineEventEntry struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`   // container | image | volume | network
	Action     string            `json:"action"` // start | die | ...
	ActorID    string            `json:"actorId"`
	ActorName  string            `json:"actorName,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Timestamp  string            `json:"timestamp"` // RFC3339
}

// OfflineMetricEntry is one per-container stats sample captured while offline.
// Field names mirror the server's container_metrics columns.
type OfflineMetricEntry struct {
	ID            string  `json:"id"`
	ContainerID   string  `json:"containerId"`
	ContainerName string  `json:"containerName,omitempty"`
	ContainerKey  string  `json:"containerKey,omitempty"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemUsed       int64   `json:"memUsed"`
	MemLimit      int64   `json:"memLimit"`
	NetRx         int64   `json:"netRx"`
	NetTx         int64   `json:"netTx"`
	BlockRead     int64   `json:"blockRead"`
	BlockWrite    int64   `json:"blockWrite"`
	PIDs          int64   `json:"pids"`
	SampledAt     string  `json:"sampledAt"` // RFC3339, shared by every container of one tick
}

// StatsPushPayload carries push-based container stats in both directions.
// Session is fixed for one agent connection and Seq increases within it, so the
// server can order batches and recognise a resend. On the subscribe and ack
// the server sets IntervalSeconds, which is also its backpressure signal, and
// Stop to end pushing. Samples reuse the offline metric shape so a batch whose
// ack never arrives can go straight into the offline buffer for replay.
type StatsPushPayload struct {
	Session         string               `json:"session,omitempty"`
	Seq             uint64               `json:"seq,omitempty"`
	SampledAt       string               `json:"sampledAt,omitempty"` // RFC3339
	Samples         []OfflineMetricEntry `json:"samples,omitempty"`
	IntervalSeconds int                  `json:"intervalSeconds,omitempty"`
	Accepted        bool                 `json:"accepted,omitempty"`
	Duplicate       bool                 `json:"duplicate,omitempty"`
	Stop            bool                 `json:"stop,omitempty"`
	Error           string               `json:"error,omitempty"`
}

// AgentUpdatePayload carries an agent self-update request and its result.
// Version is the target agent version. Image optionally overrides the
// container image to pull (empty: the agent reuses its own image reference,
// swapping a pinned semver tag for Version). BinaryURL optionally overrides
// the release download URL for binary installs.
type AgentUpdatePayload struct {
	Version string `json:"version"`
	// Build is the target artifact's yyyyMMddHHmm stamp. A rebuild does not
	// move the version, so an agent that compares only the version concludes it
	// is already current and replies success without downloading anything —
	// which reads as a replacement that never happens, on every sweep.
	Build     string `json:"build,omitempty"`
	Image     string `json:"image,omitempty"`
	BinaryURL string `json:"binaryUrl,omitempty"`
	// BinarySHA256 is the digest of the file at BinaryURL; an agent that knows
	// the field refuses a download that does not match it.
	BinarySHA256 string `json:"binarySha256,omitempty"`
	// ImageURL is where a container agent downloads its replacement as a
	// `docker save` tarball from McHarbor, ImageRef the reference it loads as and
	// ImageSHA256 the tarball's digest. Agents older than AgentImageLoadMinBuild
	// ignore all three and keep pulling from a registry themselves.
	ImageURL    string `json:"imageUrl,omitempty"`
	ImageRef    string `json:"imageRef,omitempty"`
	ImageSHA256 string `json:"imageSha256,omitempty"`
	// Response fields
	Success bool   `json:"success,omitempty"`
	Mode    string `json:"mode,omitempty"` // "container" or "binary"
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WSMessage is the envelope for all WebSocket messages.
type WSMessage struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"` // Request ID for multiplexing
	// Payload fields (only populated per message type)
	Auth            *AuthPayload               `json:"auth,omitempty"`
	AuthResult      *AuthResultPayload         `json:"authResult,omitempty"`
	HTTPRequest     *WSHTTPRequest             `json:"httpRequest,omitempty"`
	HTTPResponse    *WSHTTPResponse            `json:"httpResponse,omitempty"`
	StreamStart     *WSStreamStart             `json:"streamStart,omitempty"`
	StreamChunk     *WSStreamChunk             `json:"streamChunk,omitempty"`
	ExecStart       *ExecStartPayload          `json:"execStart,omitempty"`
	ExecResize      *ExecResizePayload         `json:"execResize,omitempty"`
	Compose         *ComposePayload            `json:"compose,omitempty"`
	Transfer        *TransferPayload           `json:"transfer,omitempty"`
	Backup          *BackupPayload             `json:"backup,omitempty"`
	HostStats       *HostStatsPayload          `json:"hostStats,omitempty"`
	Prune           *PrunePayload              `json:"prune,omitempty"`
	HostTerminal    *HostTerminalStartPayload  `json:"hostTerminal,omitempty"`
	HostTerminalRes *HostTerminalResizePayload `json:"hostTerminalResize,omitempty"`
	HostTerminalEnd *HostTerminalEndPayload    `json:"hostTerminalEnd,omitempty"`
	HostLogRequest  *HostLogRequestPayload     `json:"hostLogRequest,omitempty"`
	HostLogChunk    *HostLogChunkPayload       `json:"hostLogChunk,omitempty"`
	HostLogEnd      *HostLogEndPayload         `json:"hostLogEnd,omitempty"`
	HostOSInfo      *HostOSInfoPayload         `json:"hostOsInfo,omitempty"`
	HostUpdate      *HostUpdatePayload         `json:"hostUpdate,omitempty"`
	HostUpdateChunk *HostUpdateChunkPayload    `json:"hostUpdateChunk,omitempty"`
	HostService     *HostServicePayload        `json:"hostService,omitempty"`
	HostLink        *HostLinkPayload           `json:"hostLink,omitempty"`
	AgentUpdate     *AgentUpdatePayload        `json:"agentUpdate,omitempty"`
	Offline         *OfflineBatchPayload       `json:"offline,omitempty"`
	Spans           []TraceSpan                `json:"spans,omitempty"`
	StatsPush       *StatsPushPayload          `json:"statsPush,omitempty"`
	Snapshot        *SnapshotPayload           `json:"snapshot,omitempty"`
	Compute         *ComputePayload            `json:"compute,omitempty"`
	Console         *ConsolePayload            `json:"console,omitempty"`
	Pool            *ComputePoolPayload        `json:"pool,omitempty"`
	Net             *ComputeNetPayload         `json:"net,omitempty"`
	ComputeImage    *ComputeImagePayload       `json:"computeImage,omitempty"`
	VM              *VMPayload                 `json:"vm,omitempty"`
	CT              *CTPayload                 `json:"ct,omitempty"`
}

// ComposePayload carries an agent-side docker compose command.
type ComposePayload struct {
	ProjectName string            `json:"projectName,omitempty"`
	Files       map[string]string `json:"files,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Success     bool              `json:"success,omitempty"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// TransferPayload carries direct agent-to-agent transfer control metadata.
type TransferPayload struct {
	TransferID           string                  `json:"transferId"`
	Kind                 string                  `json:"kind,omitempty"`
	Token                string                  `json:"token,omitempty"`
	URL                  string                  `json:"url,omitempty"`
	ImageRef             string                  `json:"imageRef,omitempty"`
	ContainerID          string                  `json:"containerId,omitempty"`
	SourcePath           string                  `json:"sourcePath,omitempty"`
	TargetPath           string                  `json:"targetPath,omitempty"`
	Stage                string                  `json:"stage,omitempty"`
	Size                 int64                   `json:"size,omitempty"`
	Bytes                int64                   `json:"bytes,omitempty"`
	StatusCode           int                     `json:"statusCode,omitempty"`
	Success              bool                    `json:"success,omitempty"`
	Error                string                  `json:"error,omitempty"`
	Receiver             *TransferReceiverMarker `json:"receiver,omitempty"`
	ResponderAgentMarker string                  `json:"responderAgentMarker,omitempty"`
	Diagnostic           *TransferAuthDiagnostic `json:"diagnostic,omitempty"`
}

// BackupPayload carries agent-side backup and restore metadata.
type BackupPayload struct {
	TransferID        string   `json:"transferId"`
	ContainerID       string   `json:"containerId,omitempty"`
	ContainerName     string   `json:"containerName,omitempty"`
	IncludeConfig     bool     `json:"includeConfig,omitempty"`
	IncludeLogs       bool     `json:"includeLogs,omitempty"`
	IncludeFilesystem bool     `json:"includeFilesystem,omitempty"`
	IncludeImage      bool     `json:"includeImage,omitempty"`
	SelectedMounts    []string `json:"selectedMounts,omitempty"`
	// SnapshotMounts maps a container mount destination to the frozen
	// host path the agent should archive instead of copying the live
	// path out of the running container. Populated only when the plan
	// runs in snapshot consistency mode and the driver returned a
	// mountable view; an empty map means the legacy pause path.
	SnapshotMounts map[string]string `json:"snapshotMounts,omitempty"`
	// ConsistencyMode mirrors the plan's consistency mode ("pause",
	// "snapshot" or "live"). The agent reads it to decide whether it may
	// pause the container: "live" means the operator accepted a possibly
	// torn copy in exchange for zero downtime, so the agent must not
	// pause. Empty means "pause", which is what every host built before
	// this field sent.
	ConsistencyMode     string                     `json:"consistencyMode,omitempty"`
	RestoreItems        []string                   `json:"restoreItems,omitempty"`
	EncryptionKey       string                     `json:"encryptionKey,omitempty"`
	StorageDestinations []BackupStorageDestination `json:"storageDestinations,omitempty"`
	ArchiveURL          string                     `json:"archiveUrl,omitempty"`
	ArchiveToken        string                     `json:"archiveToken,omitempty"`
	ArchiveSize         int64                      `json:"archiveSize,omitempty"`
	Stage               string                     `json:"stage,omitempty"`
	StorageLocationID   string                     `json:"storageLocationId,omitempty"`
	Bytes               int64                      `json:"bytes,omitempty"`
	Size                int64                      `json:"size,omitempty"`
	Success             bool                       `json:"success,omitempty"`
	Error               string                     `json:"error,omitempty"`
	Restored            []string                   `json:"restored,omitempty"`
}

// BackupStorageDestination describes an operation-scoped upload target.
type BackupStorageDestination struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	LocationType string `json:"locationType"`
	UploadURL    string `json:"uploadUrl"`
	Token        string `json:"token"`
	RemotePath   string `json:"remotePath"`
}

// TransferReceiverMarker identifies a prepared receiver without exposing tokens.
type TransferReceiverMarker struct {
	TransferID       string `json:"transferId"`
	Kind             string `json:"kind"`
	ExpiresAt        string `json:"expiresAt"`
	TokenFingerprint string `json:"tokenFingerprint"`
	AgentMarker      string `json:"agentMarker,omitempty"`
}

// TransferAuthDiagnostic carries token-safe transfer receiver auth diagnostics.
type TransferAuthDiagnostic struct {
	ReceiverExists       bool   `json:"receiverExists"`
	ReceiverExpired      bool   `json:"receiverExpired"`
	ReceiverKind         string `json:"receiverKind,omitempty"`
	KindMatched          bool   `json:"kindMatched"`
	BearerPresent        bool   `json:"bearerPresent"`
	TokenMatched         bool   `json:"tokenMatched"`
	RemoteAddr           string `json:"remoteAddr,omitempty"`
	ResponderAgentMarker string `json:"responderAgentMarker,omitempty"`
}

// ExecStartPayload is sent by the server to start an exec attach on the agent.
type ExecStartPayload struct {
	ExecID string `json:"execId"`
}

// ExecResizePayload is sent by the server to resize the exec terminal.
type ExecResizePayload struct {
	ExecID string `json:"execId"`
	Cols   uint   `json:"cols"`
	Rows   uint   `json:"rows"`
}

// HostStatsPayload carries a host-stats snapshot from the agent. Fields
// mirror the Linux readHostStats + readRootFSUsage outputs in the server.
type HostStatsPayload struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemUsed    int64   `json:"memUsed"`
	Load1      float64 `json:"load1"`
	Load5      float64 `json:"load5"`
	Load15     float64 `json:"load15"`
	Uptime     int64   `json:"uptime"`
	FSTotal    int64   `json:"fsTotal"`
	FSUsed     int64   `json:"fsUsed"`
	Supported  bool    `json:"supported"`
	Error      string  `json:"error,omitempty"`
	// IO is nil from an agent that predates host I/O counters, and when the host
	// exposes neither a physical NIC nor a physical disk to read.
	IO *HostIOCounters `json:"io,omitempty"`
}

// HostIOCounters are the host's cumulative network and disk byte counters, summed
// over physical devices only so bridged, tapped and container traffic is counted once.
type HostIOCounters struct {
	NetRxBytes     int64 `json:"netRxBytes"`
	NetTxBytes     int64 `json:"netTxBytes"`
	DiskReadBytes  int64 `json:"diskReadBytes"`
	DiskWriteBytes int64 `json:"diskWriteBytes"`
}

// PrunePayload carries a prune request (Server->Agent) or result (Agent->Server).
type PrunePayload struct {
	Type           string `json:"type"`
	Volumes        bool   `json:"volumes,omitempty"`
	ItemsDeleted   int64  `json:"itemsDeleted,omitempty"`
	SpaceReclaimed int64  `json:"spaceReclaimed,omitempty"`
	Success        bool   `json:"success,omitempty"`
	Error          string `json:"error,omitempty"`
}

// HostTerminalStartPayload requests the agent to start an interactive shell.
// Cols/Rows default to 80x24 when omitted.
//
// Shell, User and WorkingDir are all optional and were added after the
// message shipped. An agent that predates them ignores the unknown JSON
// fields and starts its detected shell as root in its own working
// directory - which is exactly the behaviour every agent had before, so
// the server can send them unconditionally.
type HostTerminalStartPayload struct {
	Cols uint `json:"cols,omitempty"`
	Rows uint `json:"rows,omitempty"`
	// Shell is an absolute path the server has already allowlisted. Empty
	// means "detect one".
	Shell string `json:"shell,omitempty"`
	// User is a host account name. Empty means "stay as the agent's own
	// user", which is root in every supported install.
	User string `json:"user,omitempty"`
	// WorkingDir is an absolute path to start the shell in. Empty means the
	// agent's working directory.
	WorkingDir string `json:"workingDir,omitempty"`
}

// HostTerminalResizePayload updates the TTY size for an active shell session.
type HostTerminalResizePayload struct {
	ExecID string `json:"execId"`
	Cols   uint   `json:"cols"`
	Rows   uint   `json:"rows"`
}

// HostTerminalEndPayload notifies the agent that the session is over.
type HostTerminalEndPayload struct {
	ExecID string `json:"execId"`
}

// HostLogRequestPayload describes a host-log read request.
type HostLogRequestPayload struct {
	Source string `json:"source"`
	Tail   int    `json:"tail"`
	// Target names one guest for the qemu source. Empty asks which guests have
	// a log at all, rather than reading one.
	Target string `json:"target,omitempty"`
}

// HostLogChunkPayload carries a single line of host-log output streamed from
// the agent to the server.
type HostLogChunkPayload struct {
	Line string `json:"line"`
}

// HostLogEndPayload signals that the host-log stream is complete and carries
// any non-fatal notices collected during the read.
type HostLogEndPayload struct {
	Notices []string `json:"notices,omitempty"`
	// AvailableFiles answers a target-less qemu request, so the UI can offer a
	// guest picker without the agent exposing a guest API.
	AvailableFiles []string `json:"availableFiles,omitempty"`
	// Targets answers the same request with the guests McHarbor itself runs,
	// named, so the picker can show a VM's name rather than a unit id.
	Targets []HostLogTarget `json:"targets,omitempty"`
}

// HostLogTarget is one guest a qemu log request can name: ID is what goes back
// as the request's target, Label is the name the guest was started under.
type HostLogTarget struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// HostOSInfoPayload describes the operating system the agent runs on, so the
// environment page can show what it is without shelling into it.
type HostOSInfoPayload struct {
	PrettyName     string `json:"prettyName,omitempty"`
	Distro         string `json:"distro,omitempty"`
	Version        string `json:"version,omitempty"`
	Kernel         string `json:"kernel,omitempty"`
	Arch           string `json:"arch,omitempty"`
	Hostname       string `json:"hostname,omitempty"`
	UptimeSeconds  int64  `json:"uptimeSeconds,omitempty"`
	Manager        string `json:"manager,omitempty"`
	Virtualization string `json:"virtualization,omitempty"`
	RebootRequired bool   `json:"rebootRequired,omitempty"`
	Error          string `json:"error,omitempty"`
}

// HostUpdatePayload carries the result of a package check or of a finished
// upgrade. Updates is populated by a check, ExitCode and Success by an apply.
//
// WithNewPackages, SkipUnavailable and RemoveUnused travel the other way: they
// are the only fields the server sets, on the apply request, and an agent that
// predates one ignores it and upgrades conservatively as before.
type HostUpdatePayload struct {
	WithNewPackages bool     `json:"withNewPackages,omitempty"`
	SkipUnavailable bool     `json:"skipUnavailable,omitempty"`
	RemoveUnused    bool     `json:"removeUnused,omitempty"`
	Manager         string   `json:"manager,omitempty"`
	Available       bool     `json:"available,omitempty"`
	Updates         []string `json:"updates,omitempty"`
	Output          string   `json:"output,omitempty"`
	ExitCode        int      `json:"exitCode,omitempty"`
	Success         bool     `json:"success,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// HostUpdateChunkPayload carries one line of upgrade output as it is produced.
type HostUpdateChunkPayload struct {
	Line string `json:"line"`
}

// Host service operations carried in HostServicePayload.Op.
const (
	HostServiceOpList         = "list"
	HostServiceOpAction       = "action"
	HostServiceOpOverrideGet  = "override_get"
	HostServiceOpOverridePut  = "override_put"
	HostServiceOpOverrideDel  = "override_delete"
	HostServiceOpDaemonReload = "daemon_reload"
)

// Host service lifecycle actions carried in HostServicePayload.Action.
const (
	HostServiceActionStart   = "start"
	HostServiceActionStop    = "stop"
	HostServiceActionRestart = "restart"
	HostServiceActionEnable  = "enable"
	HostServiceActionDisable = "disable"
)

// HostServiceCapability is the AuthPayload.Capabilities entry a native Linux
// agent with a working systemctl reports.
const HostServiceCapability = "host_services"

// HostServiceWarningAgentRestart tells the server the agent is restarting its
// own unit, so the connection is about to drop and reconnect.
const HostServiceWarningAgentRestart = "agent_restart"

// HostServicePayload is one systemd operation and its answer. The request sets
// Op, Unit, Action, Content, Reload and Verify; the agent answers in the rest.
//
// ErrorCode is a stable machine value ("invalid_unit", "protected",
// "unsupported", "failed") so the server can choose an i18n code without
// parsing the human Error text.
type HostServicePayload struct {
	Op      string `json:"op"`
	Unit    string `json:"unit,omitempty"`
	Action  string `json:"action,omitempty"`
	Content string `json:"content,omitempty"`
	Reload  bool   `json:"reload,omitempty"`
	Verify  bool   `json:"verify,omitempty"`

	Units          []HostServiceUnit `json:"units,omitempty"`
	Effective      string            `json:"effective,omitempty"`
	Override       string            `json:"override,omitempty"`
	OverrideExists bool              `json:"overrideExists,omitempty"`
	OverridePath   string            `json:"overridePath,omitempty"`
	VerifyOutput   string            `json:"verifyOutput,omitempty"`
	Reloaded       bool              `json:"reloaded,omitempty"`
	Warning        string            `json:"warning,omitempty"`
	Truncated      bool              `json:"truncated,omitempty"`
	Success        bool              `json:"success,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	Error          string            `json:"error,omitempty"`
}

// HostServiceUnit is one systemd service as the Services tab lists it: the
// loaded state from list-units merged with the enablement from list-unit-files.
type HostServiceUnit struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	LoadState     string `json:"loadState,omitempty"`
	ActiveState   string `json:"activeState,omitempty"`
	SubState      string `json:"subState,omitempty"`
	UnitFileState string `json:"unitFileState,omitempty"`
	MainPID       int    `json:"mainPid,omitempty"`
	Result        string `json:"result,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
	Template      bool   `json:"template,omitempty"`
}

// Host link operations carried in HostLinkPayload.Op.
const (
	HostLinkOpEnsure = "ensure"
	HostLinkOpRemove = "remove"
)

// Host link owners carried in HostLinkPayload.Owner. The owner and its id form
// the link's ifalias, which is the only thing that proves a link is McHarbor's.
const (
	HostLinkOwnerCompute = "compute"
	HostLinkOwnerDocker  = "docker"
)

// Host link error codes carried in HostLinkPayload.ErrorCode, so the server can
// pick an i18n code without parsing the human Error text.
const (
	HostLinkErrNameTaken        = "name_taken"
	HostLinkErrParentNotFound   = "parent_not_found"
	HostLinkErrNotLinux         = "not_linux"
	HostLinkErrPermissionDenied = "permission_denied"
	HostLinkErrInvalid          = "invalid"
)

// HostLinkPayload is one host link operation and its answer. The request sets
// Op through IPv6CIDR; the agent answers in Success, Error and ErrorCode.
//
// IPv6Enabled with an empty IPv6CIDR asks for SLAAC; IPv6Enabled false leaves
// the kernel's IPv6 defaults on the link alone.
type HostLinkPayload struct {
	Op          string `json:"op"`
	Owner       string `json:"owner"`
	OwnerID     string `json:"ownerId"`
	Name        string `json:"name,omitempty"`
	Parent      string `json:"parent,omitempty"`
	IPv4CIDR    string `json:"ipv4Cidr,omitempty"`
	IPv6Enabled bool   `json:"ipv6Enabled,omitempty"`
	IPv6CIDR    string `json:"ipv6Cidr,omitempty"`

	Success   bool   `json:"success,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// AuthPayload is sent by the agent during the handshake.
type AuthPayload struct {
	Token        string `json:"token"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	AgentVersion string `json:"agentVersion"`
	// AgentBuild is the yyyyMMddHHmm stamp of the artifact. The version says
	// which release this is; the build says which build of it, which is what
	// distinguishes two agents that report the same version.
	AgentBuild string `json:"agentBuild,omitempty"`
	// HostIP is the source address this host would use to reach the outside,
	// read from the routing table rather than from interface enumeration.
	// Interface order is arbitrary and, on a host-networked agent, includes
	// every Docker bridge — so enumeration alone reports a container gateway
	// as readily as the address an operator would recognise.
	HostIP        string `json:"hostIp,omitempty"`
	DockerVersion string `json:"dockerVersion"`
	// InstallKind is "native" or "container". Guests are supervised as systemd
	// transient units and need QEMU and /dev/kvm on the host, none of which a
	// container agent has — so this is the single field that answers "why can
	// this host not run a VM?" before anyone opens the capability panel.
	// Empty from an agent older than 3.5.0.
	InstallKind string `json:"installKind,omitempty"`
	// Capabilities names optional features this agent can serve on this host,
	// such as HostServiceCapability. A build gate says the code exists; this
	// says the host can actually run it. Empty from older agents.
	Capabilities []string `json:"capabilities,omitempty"`
	// TLSMode is how this agent secured its control channel: "pinned",
	// "verified", "insecure" (TLS with verification disabled) or "plaintext".
	// Only the agent can know it — the server sees a terminated connection.
	TLSMode     string `json:"tlsMode,omitempty"`
	TransferURL string `json:"transferUrl,omitempty"`
	// TransferCandidates lists every base URL a peer agent may try to reach
	// this agent's direct transfer receiver on, in preference order. The
	// server appends a reflexive candidate built from the address it saw the
	// agent connect from, which is the one that survives a dynamic IP.
	TransferCandidates []string `json:"transferCandidates,omitempty"`
	// TransferPort is the receiver's listening port, needed by the server to
	// build that reflexive candidate. Zero disables it.
	TransferPort int `json:"transferPort,omitempty"`
}

// AuthResultPayload is sent by the server after validating the auth.
type AuthResultPayload struct {
	Success bool   `json:"success"`
	EnvID   string `json:"envId,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WSHTTPRequest represents a Docker API HTTP request sent over WebSocket.
type WSHTTPRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   string            `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"` // raw bytes
}

// WSHTTPResponse represents a full HTTP response sent back by the agent.
type WSHTTPResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"` // raw bytes
}

// WSStreamStart signals the beginning of a streaming response.
type WSStreamStart struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers,omitempty"`
}

// WSStreamChunk carries a chunk of streaming response data.
type WSStreamChunk struct {
	Data []byte `json:"data"`
}

// SnapshotPayload is the request and the reply body for every
// snapshot-family message. Operation disambiguates which of the four
// the agent should run; the agent echoes Operation, EnvID and Driver
// back so the server can route the reply without re-reading its own
// request.
type SnapshotPayload struct {
	Operation    string   `json:"operation,omitempty"` // detect | capability | create | remove
	EnvID        string   `json:"envId,omitempty"`
	Driver       string   `json:"driver,omitempty"` // zfs | btrfs | lvm | vss
	Targets      []string `json:"targets,omitempty"`
	SnapshotID   string   `json:"snapshotId,omitempty"`   // correlation token, normally the backup run id
	SnapshotName string   `json:"snapshotName,omitempty"` // driver-native handle, set on create, read on remove
	MountPath    string   `json:"mountPath,omitempty"`    // read-only view the archive can be built from
	ReadOnly     bool     `json:"readOnly,omitempty"`
	TimeoutSec   int      `json:"timeoutSec,omitempty"`
	CreatedAt    string   `json:"createdAt,omitempty"`

	// Reply-only fields.
	ToolPath   string              `json:"toolPath,omitempty"`
	Version    string              `json:"version,omitempty"`
	Available  bool                `json:"available,omitempty"`
	TargetsOut []SnapshotTargetOut `json:"targetsOut,omitempty"`
	Detected   []SnapshotDriverOut `json:"detected,omitempty"`
	MountPaths map[string]string   `json:"mountPaths,omitempty"` // target -> frozen path
	Success    bool                `json:"success,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// SnapshotTargetOut is one snapshot-capable unit the driver exposes:
// a ZFS dataset, a btrfs subvolume, an LVM logical volume, or a
// Windows shadow-copy volume.
type SnapshotTargetOut struct {
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint,omitempty"`
	UsedBytes  int64  `json:"usedBytes,omitempty"`
	Driver     string `json:"driver,omitempty"`
}

// SnapshotDriverOut is one driver the agent found on its own host,
// returned by the detect operation. Available=false with a populated
// Error is the normal answer for a driver whose CLI is absent; the
// server shows it as a greyed-out row rather than hiding it, so the
// operator learns what to install.
type SnapshotDriverOut struct {
	Driver    string              `json:"driver"`
	ToolPath  string              `json:"toolPath,omitempty"`
	Version   string              `json:"version,omitempty"`
	Available bool                `json:"available"`
	Error     string              `json:"error,omitempty"`
	Targets   []SnapshotTargetOut `json:"targets,omitempty"`
}

// ---------------------------------------------------------------------------
// Compute: KVM virtual machines and LXC system containers.
//
// The backend is a control plane — desired state in SQLite, RBAC, audit, UI —
// and never touches QEMU, netlink or a rootfs. The agent is the node executor.
// Everything below is the vocabulary between the two.
//
// Compute commands are deliberately NOT buffered by the offline store. A
// queued "start this VM" replayed twenty minutes after a network partition is
// a correctness hazard; these fail fast with a reconnect prompt instead. Only
// stats and lease events take the offline path.
// ---------------------------------------------------------------------------

// ComputePayload is the request and the reply body for the capability probe.
// It is the first thing the UI renders for an environment, so an absent item
// must explain itself: Remediation is a copyable command, not prose.
type ComputePayload struct {
	Operation string `json:"operation,omitempty"` // probe
	EnvID     string `json:"envId,omitempty"`

	// Roots carries the locations on a compute_config_request, and the
	// locations actually in effect on the response — which differ whenever the
	// agent declined one.
	Roots *ComputeRoots `json:"roots,omitempty"`

	// Migrate carries a relocation request, and its plan or its result on the
	// way back.
	Migrate *ComputeMigrate `json:"migrate,omitempty"`

	// Reply-only fields.
	Capability *ComputeCapability `json:"capability,omitempty"`
	// Install names the capability item to install on a compute_install_request, and carries
	// the packages the agent chose and the package manager's output on the reply.
	Install *ComputeInstall `json:"install,omitempty"`
	Success bool            `json:"success,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ComputeMigrate asks a host to move what it already holds to new locations.
//
// Changing a root only decides where the *next* guest is written; everything
// already on disk stays where it is, and the rows that name it keep pointing at
// the old path. This is the other half: the files move, and the host reports
// every path it changed so each plugin can rewrite the rows it owns.
type ComputeMigrate struct {
	// Plan asks what the move would involve without moving anything.
	Plan bool `json:"plan,omitempty"`
	// Repair moves nothing and fixes what a move left pointing at the old
	// location: a qcow2 whose backing file is named by absolute path, and a
	// snapshot manifest full of them.
	Repair bool `json:"repair,omitempty"`
	// Roots are the destinations. An empty field is a location not changing.
	Roots *ComputeRoots `json:"roots,omitempty"`
	// Volumes are explicit file moves, which is how a pool volume outside the
	// disk root travels: the agent does not know which files belong to which
	// pool, so the server names them.
	Volumes []ComputeMove `json:"volumes,omitempty"`
	// VMFrom, CTFrom and ImageFrom are where the data is *now*.
	//
	// Sent rather than inferred, because by the time a migration runs the host
	// has usually already adopted the new roots: McHarbor pushes them as soon
	// as the operator saves, so asking the agent where it keeps things answers
	// with the empty destination. The server remembers what they were.
	VMFrom    string `json:"vmFrom,omitempty"`
	CTFrom    string `json:"ctFrom,omitempty"`
	ImageFrom string `json:"imageFrom,omitempty"`
	// DiskFrom and DiskTo move a dir pool's target wholesale. Every file in it
	// named `<guestId>-<slot>` is announced as a guest event on the way, so
	// the plugin that owns the disk row rewrites it without anyone holding a
	// cross-plugin table.
	DiskFrom string `json:"diskFrom,omitempty"`
	DiskTo   string `json:"diskTo,omitempty"`

	// Reply-only.
	Entries  int             `json:"entries,omitempty"`
	Bytes    int64           `json:"bytes,omitempty"`
	Moved    []ComputeMove   `json:"moved,omitempty"`
	Failures []ComputeFailed `json:"failures,omitempty"`
	// Notes say what was looked at. A repair that reports "0 repaired" is
	// otherwise indistinguishable from one that looked in the wrong place,
	// which is exactly the ambiguity that wasted a debugging round.
	Notes     []string `json:"notes,omitempty"`
	Completed bool     `json:"completed,omitempty"`
}

// ComputeMove is one path before and after.
type ComputeMove struct {
	Kind string `json:"kind"` // vm | ct | image | volume
	From string `json:"from"`
	To   string `json:"to"`
	// GuestID and Slot identify a volume's owner, so the server can rewrite
	// the disk row without matching on a path it may have already changed.
	GuestID string `json:"guestId,omitempty"`
	Slot    int    `json:"slot,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
}

// ComputeFailed is one path that could not be moved, and why.
//
// Reported rather than aborting the whole run: a migration that stops at the
// first unreadable file leaves the host half-moved with no account of it.
type ComputeFailed struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ComputeRoots names where a host keeps its compute state.
//
// An empty field means "keep whatever you are using", which is what lets
// McHarbor send only the locations an operator actually set and leave the rest
// at the agent's defaults. Drives are deliberately absent: a disk lives in its
// storage pool, and a pool already carries its own target.
type ComputeRoots struct {
	// VMRoot is where per-VM sockets, firmware vars and args.json live.
	VMRoot string `json:"vmRoot,omitempty"`
	// CTRoot is where container rootfs trees and config.json live.
	CTRoot string `json:"ctRoot,omitempty"`
	// ImageRoot is the image cache. On a host that mounts McHarbor's shared
	// image library, this is the mount point — an uploaded image is then
	// simply already present rather than transferred.
	ImageRoot string `json:"imageRoot,omitempty"`
}

// ComputeCapability is one host's answer to "what can you run?". Supported is
// false — with a populated UnsupportedWhy — on Windows and macOS agents rather
// than the capability simply being absent, so the UI can explain instead of
// staying silent about a section the operator expected to see.
type ComputeCapability struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	KernelVersion  string `json:"kernelVersion,omitempty"`
	AgentVersion   string `json:"agentVersion,omitempty"`
	Supported      bool   `json:"supported"`
	UnsupportedWhy string `json:"unsupportedWhy,omitempty"`
	// CanRunVMs and CanRunContainers are the two conclusions the UI acts on;
	// Items is the evidence behind them.
	CanRunVMs        bool `json:"canRunVms"`
	CanRunContainers bool `json:"canRunContainers"`
	// SeccompBuiltIn reports whether this agent binary was compiled with the
	// seccomp build tag. Built without it, libcontainer applies no syscall
	// filter at all — a posture that must never be invisible.
	SeccompBuiltIn bool `json:"seccompBuiltIn"`
	// SupervisorMode is "dbus" when the agent reached the system bus, or
	// "systemd-run" when it fell back to the CLI.
	SupervisorMode string `json:"supervisorMode,omitempty"`

	// HostCPUs and HostMemoryBytes size the guest sliders in the UI.
	HostCPUs        int   `json:"hostCpus,omitempty"`
	HostMemoryBytes int64 `json:"hostMemoryBytes,omitempty"`
	// HostSockets and HostPhysicalCores describe the topology behind HostCPUs,
	// which counts threads. Overcommit ratios are judged against real cores.
	HostSockets       int `json:"hostSockets,omitempty"`
	HostPhysicalCores int `json:"hostPhysicalCores,omitempty"`

	// HugePages and NUMANodes are absent on a host that has neither, which is
	// the ordinary case. The UI disables the matching VM toggles rather than
	// offering a setting whose guest start would then fail.
	HugePages *ComputeHugePages `json:"hugePages,omitempty"`
	NUMANodes []ComputeNUMANode `json:"numaNodes,omitempty"`

	Items          []ComputeCapabilityItem `json:"items,omitempty"`
	QEMUBinaries   []ComputeQEMUBinary     `json:"qemuBinaries,omitempty"`
	Bridges        []ComputeHostLink       `json:"bridges,omitempty"`
	SubuidRanges   []ComputeIDRange        `json:"subuidRanges,omitempty"`
	SubgidRanges   []ComputeIDRange        `json:"subgidRanges,omitempty"`
	StorageDrivers []SnapshotDriverOut     `json:"storageDrivers,omitempty"`
	ProbedAt       string                  `json:"probedAt,omitempty"` // RFC3339
}

// ComputeCapabilityItem is one line of the dependency-floor checklist.
// Critical marks an item whose absence blocks the capability outright, so the
// UI can separate "you cannot do this" from "this option is unavailable".
type ComputeCapabilityItem struct {
	Key         string `json:"key"` // kvm_device | cgroup2 | userns | subuid | systemd | dbus | qemu_img | ovmf | swtpm | idmapped_mounts | iommu | seccomp
	Present     bool   `json:"present"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	Critical    bool   `json:"critical,omitempty"`
	Blocks      string `json:"blocks,omitempty"` // vm | ct | both
	// Installable marks a missing item the agent can install itself (compute_install_request).
	Installable bool `json:"installable,omitempty"`
}

// ComputeInstall is one package install for a capability item. The server sends only Item;
// the agent alone decides which packages that means, so a request cannot name software.
type ComputeInstall struct {
	Item     string   `json:"item"`
	Packages []string `json:"packages,omitempty"`
	Output   string   `json:"output,omitempty"`
}

// ComputeQEMUBinary is one qemu-system-* the agent found on PATH.
type ComputeQEMUBinary struct {
	Path         string   `json:"path"`
	Arch         string   `json:"arch"`    // x86_64 | aarch64
	Version      string   `json:"version"` // as reported by --version
	MachineTypes []string `json:"machineTypes,omitempty"`
	KVMAccel     bool     `json:"kvmAccel"` // TCG-only binaries are refused, not offered

	// Devices, CPUModels and QMPCommands are the capability gate. Every VM
	// feature McHarbor offers is a device to emit or a QMP verb to call, so a
	// host whose QEMU has neither must show the feature disabled with the
	// missing capability named, instead of failing at use time with QEMU's own
	// wording.
	Devices     []string `json:"devices,omitempty"`
	CPUModels   []string `json:"cpuModels,omitempty"`
	QMPCommands []string `json:"qmpCommands,omitempty"`
}

// ComputeHugePages is the host's hugepage pool.
//
// McHarbor never reserves hugepages on the operator's behalf — that takes
// memory from everything else on the box — so this gates the per-VM toggle
// rather than being acted on.
type ComputeHugePages struct {
	Total    int64 `json:"total"`
	Free     int64 `json:"free"`
	SizeKB   int64 `json:"sizeKb,omitempty"`
	Surplus  int64 `json:"surplus,omitempty"`
	Reserved int64 `json:"reserved,omitempty"`
}

// ComputeNUMANode is one host NUMA node and what it has free right now.
//
// FreeBytes is what a pinning pre-flight checks: a guest pinned to a node with
// no memory left fails to start with an error naming neither the node nor the
// reason.
type ComputeNUMANode struct {
	ID          int   `json:"id"`
	CPUs        []int `json:"cpus,omitempty"`
	MemoryBytes int64 `json:"memoryBytes,omitempty"`
	FreeBytes   int64 `json:"freeBytes,omitempty"`
}

// ComputeIDRange is one /etc/subuid or /etc/subgid entry for the agent's user.
// It establishes the envelope the container id allocator may carve up; it is
// never used to derive which sub-ranges are already free — that allocation
// lives in the McHarbor database so it stays auditable across agent restarts.
type ComputeIDRange struct {
	Start int64 `json:"start"`
	Count int64 `json:"count"`
}

// ConsolePayload carries one frame of a guest console session. Data is passed
// through unmodified for RFB and SPICE; serial and tty use the same byte-stream
// shape host_terminal_* uses.
//
// Nothing a guest sends is trusted. The backend escapes before rendering and
// never turns a guest-reported value into a link or a command argument.
type ConsolePayload struct {
	SessionID string   `json:"sessionId"`
	Kind      string   `json:"kind,omitempty"`      // vnc | spice | serial | tty | exec
	GuestKind string   `json:"guestKind,omitempty"` // vm | ct
	TargetID  string   `json:"targetId,omitempty"`
	Command   []string `json:"command,omitempty"` // exec only; shell detection picks bash then sh
	Cols      int      `json:"cols,omitempty"`
	Rows      int      `json:"rows,omitempty"`
	Data      []byte   `json:"data,omitempty"`

	// Reply-only fields.
	Success  bool   `json:"success,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ComputePoolPayload is the request and the reply body for every pool-family
// message, following the SnapshotPayload shape: Operation disambiguates, and
// the agent echoes EnvID and PoolID so the server can route the reply.
type ComputePoolPayload struct {
	Operation string `json:"operation,omitempty"` // detect | create | stats | remove | browse | read | write | mkdir | delete | rename
	EnvID     string `json:"envId,omitempty"`
	PoolID    string `json:"poolId,omitempty"`
	Name      string `json:"name,omitempty"`
	Backend   string `json:"backend,omitempty"` // dir | zfs | lvm | btrfs
	Target    string `json:"target,omitempty"`  // path, dataset, vg/thinpool, subvolume
	// Force removes a pool the agent still finds volumes on. The backend
	// refuses this by default and gates the override behind compute.delete.
	Force bool `json:"force,omitempty"`

	// Path is always relative to the pool's Target, with `/` separators and no
	// leading slash; the empty string is the pool root. The agent resolves it
	// and refuses anything that escapes, so a server-side check here would be
	// reassurance rather than a control.
	Path string `json:"path,omitempty"`
	// NewName is the last path segment a rename moves to, never a path: moving
	// a file to another directory is not something the browser offers, and
	// accepting one would be a second way to leave the pool.
	NewName string `json:"newName,omitempty"`
	// Mode is the octal permission string a write or mkdir applies, e.g. 0644.
	// Empty means the agent's default for that kind of entry.
	Mode string `json:"mode,omitempty"`
	// Content carries a whole small file. Anything the browser considers large
	// is downloaded over the HTTP proxy instead, because a multi-gigabyte disk
	// image must not sit in one WebSocket frame.
	Content []byte `json:"content,omitempty"`
	// Recursive lets a delete remove a non-empty directory. Absent, the agent
	// refuses one, so an accidental click cannot take a pool's whole tree.
	Recursive bool `json:"recursive,omitempty"`

	// Reply-only fields.
	Detected []ComputePoolBackend `json:"detected,omitempty"`
	Stats    *ComputePoolStats    `json:"stats,omitempty"`
	Entries  []ComputePoolFile    `json:"entries,omitempty"`
	Success  bool                 `json:"success,omitempty"`
	Error    string               `json:"error,omitempty"`
	// ErrorCode classifies a refusal the server branches on: "pool_in_use" when
	// guest volumes remain on a pool being removed.
	ErrorCode string `json:"errorCode,omitempty"`
}

// ComputePoolFile is one entry in a pool directory listing.
//
// LinkTarget is reported for a symlink so the browser can show where it points,
// but following one is the agent's decision and it declines any that leaves the
// pool — the field is for display, not for a second resolution attempt.
type ComputePoolFile struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"isDir,omitempty"`
	IsSymlink  bool   `json:"isSymlink,omitempty"`
	LinkTarget string `json:"linkTarget,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Mode       string `json:"mode,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"` // RFC3339
	OwnerUID   int    `json:"ownerUid,omitempty"`
	OwnerGID   int    `json:"ownerGid,omitempty"`
}

// ComputePoolBackend is one pool backend on the host. Available=false with a
// populated Remediation is the normal answer for a backend whose tooling is
// absent; the UI greys the row rather than hiding it.
type ComputePoolBackend struct {
	Backend     string                 `json:"backend"`
	Available   bool                   `json:"available"`
	Detail      string                 `json:"detail,omitempty"`
	Remediation string                 `json:"remediation,omitempty"`
	Candidates  []ComputePoolCandidate `json:"candidates,omitempty"`
	// Instant reports whether this backend clones and snapshots as a metadata
	// operation. It drives the setup wizard's recommendation, because picking
	// zfs or btrfs is what makes guest snapshots and clones reuse code that
	// already exists rather than a slow copy.
	Instant bool `json:"instant,omitempty"`
}

// ComputePoolCandidate is one place on the host a pool of this backend could
// live: an existing dataset, a volume group with a thin pool, a subvolume, or
// a writable directory.
type ComputePoolCandidate struct {
	Target        string `json:"target"`
	Mountpoint    string `json:"mountpoint,omitempty"`
	CapacityBytes int64  `json:"capacityBytes,omitempty"`
	FreeBytes     int64  `json:"freeBytes,omitempty"`
}

// ComputePoolStats is one capacity sample. It piggybacks the existing host
// stats interval rather than adding a second timer.
type ComputePoolStats struct {
	CapacityBytes  int64  `json:"capacityBytes"`
	UsedBytes      int64  `json:"usedBytes"`
	FreeBytes      int64  `json:"freeBytes"`
	AllocatedBytes int64  `json:"allocatedBytes,omitempty"` // thin provisioning: sum of volume sizes
	VolumeCount    int    `json:"volumeCount,omitempty"`
	SampledAt      string `json:"sampledAt,omitempty"` // RFC3339
}

// ComputeNetPayload is the request and the reply body for every net-family
// message.
//
// Bridged mode never reconfigures a physical link, which is what keeps a
// misconfiguration from stranding the operator's own SSH session. Managed mode
// does own its bridge, its NAT rules and its DHCP responder, and every managed
// change is preceded by a connectivity self-test with automatic rollback.
type ComputeNetPayload struct {
	Operation      string `json:"operation,omitempty"` // detect | ensure | remove | lease
	EnvID          string `json:"envId,omitempty"`
	NetworkID      string `json:"networkId,omitempty"`
	Name           string `json:"name,omitempty"`
	Mode           string `json:"mode,omitempty"`   // bridged | managed | external | macvlan
	Bridge         string `json:"bridge,omitempty"` // br0 | mcharbor-br0
	VLANTag        int    `json:"vlanTag,omitempty"`
	SubnetCIDR     string `json:"subnetCidr,omitempty"`
	GatewayIP      string `json:"gatewayIp,omitempty"`
	DHCPEnabled    bool   `json:"dhcpEnabled,omitempty"`
	DHCPRangeStart string `json:"dhcpRangeStart,omitempty"`
	DHCPRangeEnd   string `json:"dhcpRangeEnd,omitempty"`
	NATEnabled     bool   `json:"natEnabled,omitempty"`
	// UplinkLink is the masquerade egress interface in managed mode, the
	// adapter being adopted in external mode, and the parent interface the
	// children stack on in macvlan mode. Managed mode never
	// enslaves it and never touches the host's own addressing; external mode
	// does both, which is why only external mode carries a rollback. The
	// direct modes enslave nothing either — a macvlan child leaves its parent
	// and the host's addressing on it completely alone.
	UplinkLink string `json:"uplinkLink,omitempty"`
	// LinkMode is the direct-mode sub-mode: bridge | vepa | private |
	// passthru. Empty on every other mode.
	LinkMode string `json:"linkMode,omitempty"`
	// HostAccess asks for a shim link on the host itself. A macvlan child
	// cannot reach its own parent, so without it a guest can talk to the
	// whole LAN except the machine it is running on.
	HostAccess bool `json:"hostAccess,omitempty"`
	// HostAccessIP is the shim's address in CIDR form, empty to leave the host
	// to claim one itself.
	HostAccessIP string `json:"hostAccessIp,omitempty"`
	// HostLinkName is the operator's name for the shim, empty for the
	// generated mcharbor-h<id>. Ownership is the link's ifalias, not its name.
	HostLinkName string `json:"hostLinkName,omitempty"`
	// IPv6Enabled turns IPv6 on for the shim; with HostAccessIPv6 empty the
	// shim takes a SLAAC address from the segment's router advertisements.
	IPv6Enabled    bool   `json:"ipv6Enabled,omitempty"`
	HostAccessIPv6 string `json:"hostAccessIpv6,omitempty"`

	// Reply-only fields.
	Links   []ComputeHostLink `json:"links,omitempty"`
	Lease   *ComputeLease     `json:"lease,omitempty"`
	Success bool              `json:"success,omitempty"`
	Error   string            `json:"error,omitempty"`
	// ErrorCode classifies a failure the server has to act on rather than only
	// log — currently "uplink_busy", the one failure whose fix belongs to the
	// operator and needs to reach them as a message rather than a 502.
	ErrorCode string `json:"errorCode,omitempty"`
}

// ComputeHostLink is one network interface on the host. Managed reports
// whether McHarbor created it — every object McHarbor creates carries an
// mcharbor- prefix and is removable from the UI, so nothing is hidden host
// mutation.
type ComputeHostLink struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // bridge | physical | bond | vlan | veth | tap
	MAC       string   `json:"mac,omitempty"`
	MTU       int      `json:"mtu,omitempty"`
	Up        bool     `json:"up"`
	Addresses []string `json:"addresses,omitempty"`
	Master    string   `json:"master,omitempty"` // bridge this link is enslaved to
	Managed   bool     `json:"managed,omitempty"`
	// HasDefaultRoute marks the link the host's default route leaves by. The
	// UI warns hard before any managed-mode change touches it.
	HasDefaultRoute bool `json:"hasDefaultRoute,omitempty"`
	// Upper names the interfaces stacked on this link. A link with any of them
	// cannot be adopted as an uplink, so the picker can refuse it instead of
	// the kernel refusing it with EBUSY mid-adoption.
	Upper []string `json:"upper,omitempty"`
}

// ComputeLease is one DHCP lease on a managed network. Leases take the offline
// path, unlike compute commands: a lease replayed late is stale information,
// not a destructive action.
type ComputeLease struct {
	NetworkID string `json:"networkId"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname,omitempty"` // guest-supplied, untrusted, display only
	GuestKind string `json:"guestKind,omitempty"`
	GuestID   string `json:"guestId,omitempty"`
	Action    string `json:"action,omitempty"` // granted | renewed | expired | released
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// ComputeImagePayload is the request and the reply body for every image-family
// message.
//
// Checksum verification is mandatory and non-overridable for built-in catalog
// entries. AllowUnverified applies only to an operator-supplied URL, and the
// backend records the acknowledgement in the audit row.
type ComputeImagePayload struct {
	Operation string `json:"operation,omitempty"` // fetch | list | remove | import | cancel
	EnvID     string `json:"envId,omitempty"`
	ImageID   string `json:"imageId,omitempty"`
	Kind      string `json:"kind,omitempty"` // cloud_image | rootfs | iso

	SourceURL       string `json:"sourceUrl,omitempty"`
	ChecksumURL     string `json:"checksumUrl,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	ChecksumAlgo    string `json:"checksumAlgo,omitempty"` // sha256 | sha512
	SignatureURL    string `json:"signatureUrl,omitempty"` // detached distro signature, verified against a pinned key
	AllowUnverified bool   `json:"allowUnverified,omitempty"`

	// FromLibrary marks an image the operator uploaded into the shared image
	// library rather than one the agent downloads, so the fetch adopts the file
	// already sitting in the cache root instead of dialling a mirror.
	FromLibrary bool `json:"fromLibrary,omitempty"`
	// Filename names that file. A library image has no source URL to derive an
	// extension from, and a boot disk's extension is what QEMU reads its format
	// from.
	Filename string `json:"filename,omitempty"`
	// LibraryURL and LibraryToken let an agent pull an uploaded image from the
	// server when the library directory is not shared with this host. Empty
	// for a host that mounts the same storage, which adopts the file in place
	// and transfers nothing.
	LibraryURL   string `json:"libraryUrl,omitempty"`
	LibraryToken string `json:"libraryToken,omitempty"`

	// Import (rootfs -> pool template) fields.
	PoolID     string `json:"poolId,omitempty"`
	LocalPath  string `json:"localPath,omitempty"`
	TargetPath string `json:"targetPath,omitempty"`

	// Progress and reply fields.
	BytesDone  int64 `json:"bytesDone,omitempty"`
	BytesTotal int64 `json:"bytesTotal,omitempty"`
	Percent    int   `json:"percent,omitempty"`
	// Phase is downloading | verifying | decompressing | installing. The
	// percentage counts bytes off the wire and stops moving once they are all
	// here, which is not the same as being finished.
	Phase     string               `json:"phase,omitempty"`
	SizeBytes int64                `json:"sizeBytes,omitempty"`
	Verified  bool                 `json:"verified,omitempty"`
	Signed    bool                 `json:"signed,omitempty"`
	Cached    []ComputeCachedImage `json:"cached,omitempty"`
	Success   bool                 `json:"success,omitempty"`
	Error     string               `json:"error,omitempty"`
}

// ComputeCachedImage is one image already on the host. InUse is set when a
// live guest disk backs onto it — those are never evicted by the LRU.
type ComputeCachedImage struct {
	ImageID    string `json:"imageId"`
	Kind       string `json:"kind,omitempty"`
	LocalPath  string `json:"localPath"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Verified   bool   `json:"verified,omitempty"`
	InUse      bool   `json:"inUse,omitempty"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
}

// VMPayload is the request and the reply body for every vm-family message.
type VMPayload struct {
	Operation string `json:"operation,omitempty"` // mirrors the message type, echoed for routing
	EnvID     string `json:"envId,omitempty"`
	VMID      string `json:"vmId,omitempty"`

	Spec *VMSpec `json:"spec,omitempty"`
	// GraceSeconds is the ACPI powerdown window before escalating to quit.
	// The escalation is visible in the UI and recorded in the audit row.
	GraceSeconds  int  `json:"graceSeconds,omitempty"`
	DeleteVolumes bool `json:"deleteVolumes,omitempty"`

	Disk         *VMDiskSpec `json:"disk,omitempty"`
	SnapshotID   string      `json:"snapshotId,omitempty"`
	SnapshotName string      `json:"snapshotName,omitempty"`
	// ChildSnapshot names the snapshot directly below the one being deleted, so
	// the agent folds the deleted layer into that branch's file rather than
	// into whatever the guest happens to be writing to now.
	ChildSnapshot string `json:"childSnapshot,omitempty"`
	WithMemory    bool   `json:"withMemory,omitempty"`
	Consistency   string `json:"consistency,omitempty"` // crash | filesystem | application
	// PauseGuest stops the guest's vCPUs for the duration of the snapshot.
	// Without a guest agent there is nothing to quiesce the filesystems, and a
	// paused guest at least cannot be writing while its disks are frozen.
	PauseGuest bool           `json:"pauseGuest,omitempty"`
	Clone      *VMCloneSpec   `json:"clone,omitempty"`
	Import     *VMImportSpec  `json:"import,omitempty"`
	Migrate    *VMMigrateSpec `json:"migrate,omitempty"`
	GuestQuery string         `json:"guestQuery,omitempty"` // network | osinfo | hostname | fstrim | fsfreeze | fsthaw | health
	// GuestFamily selects which guest-agent disc vm_guest_agent_disc builds.
	// The agent takes the family from the caller rather than asking the guest,
	// because the disc is most needed by a guest that cannot answer yet.
	GuestFamily string         `json:"guestFamily,omitempty"`
	Devices     []VMDeviceSpec `json:"devices,omitempty"`

	// TargetMemoryMB and TargetVCPUs carry a live resize request. Named for
	// the target rather than reusing the spec's fields, because a resize is
	// not a redefinition: the boot size stays what it was.
	TargetMemoryMB int64 `json:"targetMemoryMb,omitempty"`
	TargetVCPUs    int   `json:"targetVcpus,omitempty"`
	// ImageOp is one read-only qemu-img subcommand plus its target.
	ImageOp *VMImageOpSpec `json:"imageOp,omitempty"`
	// Backup asks for a backup run; Restore asks for one to be applied.
	Backup  *VMBackupSpec  `json:"backup,omitempty"`
	Restore *VMRestoreSpec `json:"restore,omitempty"`
	// Hotplug carries one live hardware change for vm_disk_hotplug,
	// vm_media_change and vm_nic_hotplug. Spec travels beside it, because the
	// agent needs the machine type and the NIC's network, not only the device.
	Hotplug *VMHotplugSpec `json:"hotplug,omitempty"`

	// Reply-only fields.
	//
	// Disks carries a disk whose active file moved, which happens on a live
	// qcow2 snapshot: `blockdev-snapshot-sync` freezes the file the guest was
	// writing to and hands it a new overlay, so the path McHarbor has on record
	// is no longer the one QEMU has open. The server persists what comes back
	// here; ignoring it would leave the next start pointing at a frozen
	// snapshot rather than the guest's live disk.
	Disks  []VMDiskSpec `json:"disks,omitempty"`
	States []VMState    `json:"states,omitempty"` // vm_query reconciliation
	State  *VMState     `json:"state,omitempty"`
	Event  *VMEvent     `json:"event,omitempty"`
	Stats  *VMStats     `json:"stats,omitempty"`
	Guest  *VMGuestInfo `json:"guest,omitempty"`
	// QEMUCapability answers vm_qemu_capability: what this host's QEMU can
	// emit and which QMP verbs it implements, so a feature the host cannot do
	// renders disabled instead of failing at use time.
	QEMUCapability *VMQEMUCapability `json:"qemuCapability,omitempty"`
	// Resize answers vm_memory_set and vm_cpu_set with what the guest actually
	// did, which on an unplug is regularly not what was asked.
	Resize *VMResizeResult `json:"resize,omitempty"`
	// ImageInfo answers vm_image_inspect with qemu-img's own JSON, passed
	// through rather than re-modelled: the shape differs per subcommand and
	// per QEMU version, and the consumer is a diagnostic panel.
	ImageInfo json.RawMessage `json:"imageInfo,omitempty"`
	// BackupResult answers vm_backup_run, one entry per disk.
	BackupResult []VMBackupDiskResult `json:"backupResult,omitempty"`
	// BackupStage and BackupBytes carry progress while a run is in flight.
	BackupStage string `json:"backupStage,omitempty"` // quiescing | copying | uploading | cleaning
	BackupBytes int64  `json:"backupBytes,omitempty"`
	Percent     int    `json:"percent,omitempty"`
	// ImportResult answers vm_import_inspect and vm_import_convert.
	ImportResult *VMImportResult `json:"importResult,omitempty"`
	// SpiceTicket answers vm_spice_ticket. Its password is a one-time secret
	// for the .vv file McHarbor hands back, and is never logged or stored.
	SpiceTicket *VMSpiceTicket `json:"spiceTicket,omitempty"`
	// HostDevices answers host_device_list.
	HostDevices []HostDevice `json:"hostDevices,omitempty"`
	// Volumes answers vm_disk_list: every disk image present in the pool,
	// including ones McHarbor has no record of. Deciding which of those are
	// free is the server's job — the agent cannot know what is attached.
	Volumes []VMVolumeInfo `json:"volumes,omitempty"`
	// LogSource selects which of a guest's units vm_log reads: "vm" for QEMU
	// itself, "tpm" for its software TPM. The agent builds the unit name from
	// the guest id rather than taking one, so no caller can name an arbitrary
	// unit on a root-privileged host.
	LogSource string `json:"logSource,omitempty"`
	LogTail   int    `json:"logTail,omitempty"`
	// LogLines answers vm_log, oldest first.
	LogLines []string `json:"logLines,omitempty"`
	// LogNotices carries why a read was partial — journalctl missing, or the
	// unit having no entries — so the panel can say that instead of rendering
	// an empty box that looks like a healthy guest.
	LogNotices []string `json:"logNotices,omitempty"`
	// Orphans are units matching mcharbor-vm-* that the backend has no row
	// for. They are reported, never auto-killed.
	Orphans []string `json:"orphans,omitempty"`
	// MediaPath answers vm_guest_agent_disc with the disc the agent built, and
	// MediaLabel names it for the UI. The server records the path verbatim as
	// a CD-ROM row; only the host knows where its image cache lives.
	MediaPath  string `json:"mediaPath,omitempty"`
	MediaLabel string `json:"mediaLabel,omitempty"`
	// NICAddrs answers vm_start with the PCI slot each NIC actually got, keyed
	// by nic id. The server records them so every later start can pin them.
	NICAddrs  map[string]string `json:"nicAddrs,omitempty"`
	Success   bool              `json:"success,omitempty"`
	Error     string            `json:"error,omitempty"`
	ErrorCode string            `json:"errorCode,omitempty"` // mapped QMP error class, for i18n on the server
	// Notice qualifies a successful reply: "hotplug_unclaimed" when the guest never
	// picked up a hot-added device. ErrorCode cannot carry it, because a server reads
	// any ErrorCode on a hot-plug reply as a refusal.
	Notice string `json:"notice,omitempty"`
}

// VMSpec is everything the agent needs to materialize and boot one guest. It
// is sent whole on define and on start, so a VM can be re-materialized on a
// host that lost its runtime directory without the agent holding state.
type VMSpec struct {
	Name           string `json:"name"`
	UUID           string `json:"uuid"` // SMBIOS machine UUID; xid is not a UUID
	GuestOSFamily  string `json:"guestOsFamily,omitempty"`
	GuestOSVariant string `json:"guestOsVariant,omitempty"`

	VCPUs       int    `json:"vcpus"`
	CPUModel    string `json:"cpuModel,omitempty"` // host, or a named baseline when migration is intended
	CPUSockets  int    `json:"cpuSockets,omitempty"`
	CPUCores    int    `json:"cpuCores,omitempty"`
	CPUThreads  int    `json:"cpuThreads,omitempty"`
	MemoryMB    int64  `json:"memoryMb"`
	MemoryMaxMB int64  `json:"memoryMaxMb,omitempty"` // balloon ceiling

	// VCPUsMax is the hotplug ceiling, fixed at boot. Raising it needs a
	// restart, which is why the UI separates "change now" from "at next boot".
	VCPUsMax int `json:"vcpusMax,omitempty"`
	// VirtioMem replaces the balloon with a resizable memory device. QEMU
	// documents the two as incompatible, so exactly one is emitted.
	VirtioMem bool `json:"virtioMem,omitempty"`
	// HugePages backs guest RAM with hugepages. Not swappable, so an
	// over-committed host with hugepage guests OOMs rather than degrading.
	HugePages bool `json:"hugePages,omitempty"`
	// NUMAPolicy is none | pin | mirror, and NUMAHostNodes are the host nodes
	// the backend resolved it to.
	NUMAPolicy    string `json:"numaPolicy,omitempty"`
	NUMAHostNodes []int  `json:"numaHostNodes,omitempty"`
	// VSockCID is the guest's AF_VSOCK context id; below 3 means none, since
	// the kernel reserves 0, 1 and 2.
	VSockCID int `json:"vsockCid,omitempty"`
	// IOThread moves block I/O off QEMU's main loop. One per guest.
	IOThread bool `json:"ioThread,omitempty"`
	// Shares are virtiofs directories, each backed by its own virtiofsd that
	// the agent starts before QEMU and stops with it.
	Shares []VMShareSpec `json:"shares,omitempty"`

	// MachineType is pinned at creation (pc-q35-8.2, not q35) and never
	// silently changed by a QEMU upgrade — an unpinned machine type is how
	// guests break on a host package update.
	MachineType string `json:"machineType"`
	Firmware    string `json:"firmware,omitempty"` // uefi | bios
	SecureBoot  bool   `json:"secureBoot,omitempty"`
	TPMEnabled  bool   `json:"tpmEnabled,omitempty"`
	DisplayType string `json:"displayType,omitempty"` // virtio-gpu | qxl | std | none
	// DisplayWidth and DisplayHeight are the resolution the adapter offers the
	// guest at boot; zero leaves QEMU's default. An older agent ignores them.
	DisplayWidth  int `json:"displayWidth,omitempty"`
	DisplayHeight int `json:"displayHeight,omitempty"`
	// SpiceOptions tune the SPICE server when the agent starts one. A guest
	// without SPICE ignores them, and so does an agent that predates them.
	SpiceOptions *VMSpiceOptions `json:"spiceOptions,omitempty"`

	Disks   []VMDiskSpec   `json:"disks,omitempty"`
	NICs    []VMNICSpec    `json:"nics,omitempty"`
	Devices []VMDeviceSpec `json:"devices,omitempty"`

	// CloudInit, when set, has the agent build a NoCloud seed image in
	// process and attach it as a second CD-ROM. Never carries a plaintext
	// password: the hash is computed on the backend before it gets here.
	CloudInit *VMCloudInitSpec `json:"cloudInit,omitempty"`
	// ISOPaths are attached as CD-ROMs for the manual-install path. A Windows
	// guest gets the virtio driver ISO as a second one, because Windows
	// cannot see a virtio disk without it.
	ISOPaths []string `json:"isoPaths,omitempty"`

	UnitName string `json:"unitName,omitempty"` // mcharbor-vm-<id>
	// ExtraArgs is reserved and unused. args.json in the runtime directory
	// already gives support the full picture read-only, which covers the
	// diagnostic value without the unbounded support surface.
	ExtraArgs []string `json:"extraArgs,omitempty"`
}

// VMDiskSpec is one block device. Shrink is never expressed here: the backend
// refuses it outright, because shrinking a live filesystem from outside
// destroys data and a warning is not enough.
type VMDiskSpec struct {
	DiskID      string `json:"diskId,omitempty"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	Slot        int    `json:"slot"`
	Bus         string `json:"bus,omitempty"` // virtio-blk | virtio-scsi | sata
	VolumePath  string `json:"volumePath,omitempty"`
	Format      string `json:"format,omitempty"` // qcow2 | raw
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	// BackingPath is the cached cloud image or the template disk a linked
	// clone copies on write from.
	BackingPath string `json:"backingPath,omitempty"`
	CacheMode   string `json:"cacheMode,omitempty"`
	Discard     bool   `json:"discard,omitempty"` // on by default so deletes return capacity to thin pools
	ReadOnly    bool   `json:"readonly,omitempty"`
	IsBoot      bool   `json:"isBoot,omitempty"`
	IsCDROM     bool   `json:"isCdrom,omitempty"`

	// BootIndex places this device explicitly in the boot order. Zero keeps
	// the old derivation from IsBoot and IsCDROM, which every guest created
	// before the order was selectable still depends on.
	BootIndex int `json:"bootIndex,omitempty"`
	// ShareRW lets a second guest open this disk at the same time. Required by
	// a cluster filesystem, catastrophic without one.
	ShareRW bool `json:"shareRw,omitempty"`
	// I/O limits, zero meaning unlimited. Read and write are separate because
	// one number for both throttles the wrong half of a restore.
	ReadIOPS  int64 `json:"readIops,omitempty"`
	WriteIOPS int64 `json:"writeIops,omitempty"`
	ReadBPS   int64 `json:"readBps,omitempty"`
	WriteBPS  int64 `json:"writeBps,omitempty"`
}

// VMShareSpec is one virtiofs directory handed to a guest.
//
// HostPath is validated against the operator's configured roots before it gets
// here: a share of / or /etc hands the host to the guest, which is why the
// whole feature sits behind vm.privileged rather than vm.manage.
type VMShareSpec struct {
	ShareID  string `json:"shareId"`
	Tag      string `json:"tag"`
	HostPath string `json:"hostPath"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// VMSpiceOptions are the SPICE server's image, stream and audio settings, from
// the guest's display profile. Each is optional: empty, or a value the agent
// does not recognise, leaves QEMU's own default in place.
type VMSpiceOptions struct {
	ImageCompression      string `json:"imageCompression,omitempty"`      // auto_glz | auto_lz | quic | glz | lz | off
	JPEGWANCompression    string `json:"jpegWanCompression,omitempty"`    // auto | never | always
	ZlibGLZWANCompression string `json:"zlibGlzWanCompression,omitempty"` // auto | never | always
	StreamingVideo        string `json:"streamingVideo,omitempty"`        // off | all | filter
	PlaybackCompression   string `json:"playbackCompression,omitempty"`   // on | off
	AgentMouse            string `json:"agentMouse,omitempty"`            // on | off
}

// VMVolumeInfo is one volume found in a pool by vm_disk_list.
//
// It describes the host, not McHarbor's records: a volume left behind by a
// guest that was deleted without its disks looks exactly like one in use, and
// the server is what tells them apart.
type VMVolumeInfo struct {
	Name       string `json:"name"`
	VolumePath string `json:"volumePath"`
	Format     string `json:"format,omitempty"` // qcow2 | raw
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	IsBlock    bool   `json:"isBlock,omitempty"`
}

// VMNICSpec is one guest network interface.
type VMNICSpec struct {
	NICID     string `json:"nicId,omitempty"`
	NetworkID string `json:"networkId,omitempty"`
	Bridge    string `json:"bridge,omitempty"`
	VLANTag   int    `json:"vlanTag,omitempty"`
	Slot      int    `json:"slot"`
	Model     string `json:"model,omitempty"` // virtio-net | e1000e
	MAC       string `json:"mac"`
	TapName   string `json:"tapName,omitempty"`
	// PCIAddr pins the NIC to the slot it was first given, as 0x04. Empty
	// until McHarbor has observed one, and QEMU then allocates by
	// command-line order — which is what lets a newly attached disc rename
	// the guest's interface and cost it its address.
	PCIAddr string `json:"pciAddr,omitempty"`
	// AttachKind selects how the NIC reaches its network: empty or "bridge"
	// for a tap or veth on a bridge, "macvlan" for a direct child of
	// ParentLink. Denormalised from the network exactly as Bridge is, so the
	// agent never has to resolve a network id.
	AttachKind string `json:"attachKind,omitempty"`
	// ParentLink is the host interface a direct NIC stacks on, and is the
	// network's uplink rather than its VLAN sub-interface: only the agent can
	// tell the operator's own eth0.100 from one McHarbor made, so it resolves
	// VLANTag itself at guest start.
	ParentLink string `json:"parentLink,omitempty"`
	// LinkMode is the direct sub-mode — bridge | vepa | private | passthru.
	LinkMode string `json:"linkMode,omitempty"`
	// LinkDown starts the NIC with its link cut, which is the persisted half of
	// a live set_link: without it a restart would quietly plug the cable back in.
	LinkDown bool `json:"linkDown,omitempty"`
}

// VMHotplugSpec is one live hardware change on a running guest.
//
// Action is attach | detach | resize for a disk, insert | eject for a disc and
// attach | detach | link for a NIC. Exactly one of Disk and NIC is set.
type VMHotplugSpec struct {
	Action string      `json:"action"`
	Disk   *VMDiskSpec `json:"disk,omitempty"`
	NIC    *VMNICSpec  `json:"nic,omitempty"`
	// LinkUp is the state a link action sets.
	LinkUp bool `json:"linkUp,omitempty"`
}

// VMDeviceSpec is a host device passed through to the guest. Every one of
// these can compromise the host rather than just the guest, which is why they
// sit behind vm.privileged and never behind vm.manage.
type VMDeviceSpec struct {
	DeviceID    string `json:"deviceId,omitempty"`
	Kind        string `json:"kind"`        // pci | usb | block
	HostAddress string `json:"hostAddress"` // PCI BDF, USB vendor:product, /dev path
	Description string `json:"description,omitempty"`
}

// HostDevice is one device on a host that could be passed through to a guest.
//
// Assignable is the field that matters in the UI. A device bound to a host
// driver is listed but not offered, because attaching it would take it away
// from the host mid-flight — and a PCI device shares its IOMMU group, so the
// whole group moves together or not at all.
type HostDevice struct {
	Kind        string `json:"kind"` // pci | usb
	Address     string `json:"address"`
	Description string `json:"description,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Product     string `json:"product,omitempty"`
	Driver      string `json:"driver,omitempty"`
	Class       string `json:"class,omitempty"`
	IOMMUGroup  string `json:"iommuGroup,omitempty"`
	Assignable  bool   `json:"assignable"`
	Reason      string `json:"reason,omitempty"`
}

// VMCloudInitSpec is the NoCloud seed the agent writes as an ISO9660 volume
// labelled CIDATA.
//
// SECURITY: the seed is readable by anything that can read the pool. It must
// never carry a plaintext password, an API token or a private key.
// PasswordHash is a SHA-512 crypt(3) string produced on the backend; the
// plaintext exists only for the duration of the create request and is never
// persisted.
type VMCloudInitSpec struct {
	InstanceID    string   `json:"instanceId"` // regenerated on clone so cloud-init re-runs
	Hostname      string   `json:"hostname,omitempty"`
	FQDN          string   `json:"fqdn,omitempty"`
	Timezone      string   `json:"timezone,omitempty"`
	SSHKeys       []string `json:"sshKeys,omitempty"`
	DefaultUser   string   `json:"defaultUser,omitempty"`
	PasswordHash  string   `json:"passwordHash,omitempty"`
	NetworkMode   string   `json:"networkMode,omitempty"` // dhcp | static
	StaticIP      string   `json:"staticIp,omitempty"`
	StaticGateway string   `json:"staticGateway,omitempty"`
	StaticDNS     []string `json:"staticDns,omitempty"`
	RunCmd        []string `json:"runCmd,omitempty"`
	UserDataExtra string   `json:"userDataExtra,omitempty"` // raw override, merged last
}

// VMImportSpec asks the agent to inspect or convert a foreign guest image.
//
// Exactly one source is used. SourcePath names a file the host can already see
// — the shared image library, or something the operator copied there
// themselves. SourceURL is McHarbor serving a staged upload, redeemed with
// SourceToken; the agent streams it to disk rather than holding it in memory,
// because these are disk images and there is no upper bound worth assuming.
type VMImportSpec struct {
	ImportID   string `json:"importId"`
	SourcePath string `json:"sourcePath,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
	// SourceToken authenticates the pull. Never logged.
	SourceToken string `json:"sourceToken,omitempty"`
	// FileName carries the extension, which is what says whether the source is
	// an archive to unpack or a disk to convert.
	FileName string `json:"fileName,omitempty"`

	// Conversion targets. Empty on an inspect.
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	// VMID names the guest the converted volumes belong to, which is what
	// their filenames are built from.
	VMID string `json:"vmId,omitempty"`
	// Format is what to convert to. Empty means qcow2, which is the only
	// format that supports the snapshots and thin allocation the rest of the
	// product assumes.
	Format string `json:"format,omitempty"`
}

// VMImportDisk is one disk found in a source.
type VMImportDisk struct {
	// Index orders the disks as the descriptor listed them, which is the order
	// the original guest booted in.
	Index int `json:"index"`
	// SourceFile is the name inside the archive, or the file itself.
	SourceFile string `json:"sourceFile"`
	// SourceFormat is what qemu-img detected: vmdk, vhdx, vpc, vdi,
	// parallels, qcow2, raw.
	SourceFormat string `json:"sourceFormat"`
	VirtualSize  int64  `json:"virtualSize"`
	ActualSize   int64  `json:"actualSize,omitempty"`
	// VolumePath is where the converted disk landed. Set on a convert only.
	VolumePath string `json:"volumePath,omitempty"`
}

// VMImportResult is what the agent found, and what it produced.
//
// The guest fields come from an OVA's descriptor and are absent for a bare
// disk, where there is nothing to read them from — McHarbor then falls back to
// its own defaults rather than inventing numbers that look authoritative.
type VMImportResult struct {
	ImportID string         `json:"importId"`
	Kind     string         `json:"kind"` // disk | ova
	Disks    []VMImportDisk `json:"disks,omitempty"`

	// StagedPath is where vm_import_stage left the source on this host. It is
	// what makes a later inspect or convert need no transfer and no grant, so
	// neither depends on McHarbor still holding the upload.
	StagedPath string `json:"stagedPath,omitempty"`

	Name           string `json:"name,omitempty"`
	VCPUs          int    `json:"vcpus,omitempty"`
	MemoryMB       int64  `json:"memoryMb,omitempty"`
	Firmware       string `json:"firmware,omitempty"` // uefi | bios
	GuestOSFamily  string `json:"guestOsFamily,omitempty"`
	GuestOSVariant string `json:"guestOsVariant,omitempty"`
	NICCount       int    `json:"nicCount,omitempty"`
	// Warnings are things the source asked for that McHarbor will not carry
	// over — a paravirtual SCSI controller, a vendor guest-tools device. Shown
	// rather than dropped, because they change how the guest boots.
	Warnings []string `json:"warnings,omitempty"`
}

// VMCloneSpec describes a linked or full clone. The agent regenerates nothing
// itself: new MACs, the new machine UUID and the new cloud-init instance-id
// arrive in Spec, so the classic clone footgun — a guest inheriting the
// template's hostname and SSH host keys — cannot happen by omission.
type VMCloneSpec struct {
	TargetVMID string  `json:"targetVmId"`
	Linked     bool    `json:"linked,omitempty"`
	Spec       *VMSpec `json:"spec,omitempty"`
}

// VMMigrateSpec describes a move to a peer agent host. Cold migration —
// stop, transfer, start — ships first and covers most homelab needs.
type VMMigrateSpec struct {
	TargetHost     string `json:"targetHost"`
	TargetPort     int    `json:"targetPort,omitempty"`
	Live           bool   `json:"live,omitempty"`
	SharedStorage  bool   `json:"sharedStorage,omitempty"`
	MaxBandwidthMB int64  `json:"maxBandwidthMb,omitempty"`
	MaxDowntimeMS  int64  `json:"maxDowntimeMs,omitempty"`

	// Phase names which half of a migration this message drives; one vm_migrate
	// is always one host's half, and McHarbor sequences the two. VMMigratePhase*.
	Phase string `json:"phase,omitempty"`
	// MigrationID correlates both halves and names the target's scratch state.
	MigrationID string `json:"migrationId,omitempty"`
	// Token is the one-time secret a target's prepare records and its finish or
	// abort must present again, so no other caller can finish its migration.
	Token string          `json:"token,omitempty"`
	Disks []VMMigrateDisk `json:"disks,omitempty"`
	// TLS carries this host's half of the per-migration mutual-TLS material for
	// a live move. Minted by McHarbor, valid for minutes, and never echoed back.
	TLS *VMMigrateTLS `json:"tls,omitempty"`
	// NBDPort is where a live target serves the disks the source mirrors into.
	NBDPort int `json:"nbdPort,omitempty"`
	// SPICE is whether the source guest runs a SPICE server. A live target must
	// build the identical device set or QEMU refuses the incoming state, so the
	// source reports it on live_check and McHarbor carries it to live_prepare.
	SPICE bool `json:"spice,omitempty"`
	// HotplugLayout is whether the source guest was booted with its PCIe
	// hot-plug ports. A guest started before they existed has none, and its
	// target must be built the same way or the incoming state is refused.
	HotplugLayout bool `json:"hotplugLayout,omitempty"`

	// Reply-only: the addresses a live target listens on, and how much a cold
	// half moved.
	ListenAddrs []string `json:"listenAddrs,omitempty"`
	BytesMoved  int64    `json:"bytesMoved,omitempty"`
}

// VMMigrateDisk is one disk's part in a migration. Pool fields describe the
// target's pool; SourcePath and Format the source's file.
type VMMigrateDisk struct {
	DiskID      string `json:"diskId"`
	Slot        int    `json:"slot"`
	SourcePath  string `json:"sourcePath,omitempty"`
	Format      string `json:"format,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	// URL and Token are the relay grant a cold half streams through: a
	// server-relative path the agent resolves, and the bearer that opens it.
	URL   string `json:"url,omitempty"`
	Token string `json:"token,omitempty"`
	// Reply-only: where the disk landed on the target.
	VolumePath   string `json:"volumePath,omitempty"`
	TargetFormat string `json:"targetFormat,omitempty"`
}

// VMMigrateTLS is one host's mutual-TLS material for a live migration, PEM.
// ServerName is what the source verifies the target's certificate against.
type VMMigrateTLS struct {
	CACert     string `json:"caCert"`
	Cert       string `json:"cert"`
	Key        string `json:"key"`
	ServerName string `json:"serverName,omitempty"`
}

// VMMigratePhase values. Send and receive are the cold halves; the live_*
// phases run QMP migration with NBD disk mirroring; abort undoes a target.
const (
	VMMigratePhaseSend        = "send"
	VMMigratePhaseReceive     = "receive"
	VMMigratePhaseLiveCheck   = "live_check"
	VMMigratePhaseLivePrepare = "live_prepare"
	VMMigratePhaseLiveSend    = "live_send"
	VMMigratePhaseLiveFinish  = "live_finish"
	VMMigratePhaseAbort       = "abort"
)

// VMSpiceTicket is a remote-viewer's way in: a TCP port on the guest's host
// that relays to the SPICE socket once the one-time password is presented.
type VMSpiceTicket struct {
	Port      int      `json:"port"`
	Password  string   `json:"password"`
	Addresses []string `json:"addresses,omitempty"`
	ExpiresAt string   `json:"expiresAt,omitempty"` // RFC3339
}

// VMState is one guest's confirmed state. The backend never writes a state it
// has not been told: "unknown" with AgentOffline is the honest answer when the
// owning agent is gone, and guessing a VM is still running because it was ten
// minutes ago is how a management UI lies to its operator.
type VMState struct {
	VMID         string `json:"vmId"`
	State        string `json:"state"` // defined | running | paused | stopping | stopped | crashed | migrating | unknown
	UnitName     string `json:"unitName,omitempty"`
	UnitActive   string `json:"unitActive,omitempty"` // systemd ActiveState
	UnitSub      string `json:"unitSub,omitempty"`    // systemd SubState
	PID          int    `json:"pid,omitempty"`
	QMPReachable bool   `json:"qmpReachable,omitempty"`
	StartedAt    string `json:"startedAt,omitempty"`
	// ExitReason is read from the journal on an unexpected exit, so an OOM
	// kill surfaces as an OOM kill rather than a generic failure.
	ExitReason string `json:"exitReason,omitempty"`
	LastError  string `json:"lastError,omitempty"`
	// RunState is QEMU's own run state, unmapped. State collapses a dozen of
	// them into the handful McHarbor models, which hides the two that matter
	// most: an operator reading "Paused" cannot otherwise tell a deliberate
	// pause from a guest stopped dead because its disk backend failed.
	RunState string `json:"runState,omitempty"` // running | paused | io-error | guest-panicked | postmigrate | …
	// BlockJobs are the mirrors, commits and backups QEMU is running for this
	// guest, so a snapshot still flattening does not read as a hung guest.
	BlockJobs []VMBlockJob `json:"blockJobs,omitempty"`
}

// VMBlockJob is one long-running block operation QEMU is driving.
//
// Offset against Len is the only progress there is for a commit or a backup,
// and Error is why one stopped — a failed job leaves the guest running on a
// chain that did not finish collapsing, which is a state worth naming.
type VMBlockJob struct {
	Device string `json:"device"`
	Type   string `json:"type"` // commit | stream | mirror | backup
	Len    int64  `json:"len,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	Status string `json:"status,omitempty"` // created | running | ready | pending | concluded
	Error  string `json:"error,omitempty"`
}

// VMEvent is one QMP asynchronous event, translated. BLOCK_IO_ERROR carries
// the pool so a full pool surfaces as a pool alert rather than a mystery
// guest pause.
type VMEvent struct {
	VMID      string            `json:"vmId"`
	Kind      string            `json:"kind"` // shutdown | reset | stop | resume | powerdown | guest_panic | block_io_error | rtc_change
	PoolID    string            `json:"poolId,omitempty"`
	Device    string            `json:"device,omitempty"`
	Detail    map[string]string `json:"detail,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
}

// VMStats is one sample, pushed on the agent's own interval rather than
// polled. The background metrics collector skips agent environments for
// exactly this reason: polling per guest through the transport reproduces the
// per-call cost problem the Docker stats path already hit.
type VMStats struct {
	VMID           string  `json:"vmId"`
	CPUPercent     float64 `json:"cpuPercent,omitempty"`
	CPUUsageNSec   int64   `json:"cpuUsageNsec,omitempty"` // systemd unit property, no cgroup path building
	MemoryBytes    int64   `json:"memoryBytes,omitempty"`  // systemd MemoryCurrent
	MemoryMaxBytes int64   `json:"memoryMaxBytes,omitempty"`
	BalloonBytes   int64   `json:"balloonBytes,omitempty"`
	DiskReadBytes  int64   `json:"diskReadBytes,omitempty"`
	DiskWriteBytes int64   `json:"diskWriteBytes,omitempty"`
	NetRxBytes     int64   `json:"netRxBytes,omitempty"`
	NetTxBytes     int64   `json:"netTxBytes,omitempty"`
	SampledAt      string  `json:"sampledAt,omitempty"`
}

// VMGuestInfo is what the QEMU guest agent reported. Every field here is
// display-only and untrusted: a guest reporting a hostname of
// "<img src=x onerror=...>" must render as text, and a guest-reported IP is
// never used to build a link the operator clicks.
type VMGuestInfo struct {
	VMID       string   `json:"vmId"`
	AgentState string   `json:"agentState"` // unknown | connected | absent
	Hostname   string   `json:"hostname,omitempty"`
	OSID       string   `json:"osId,omitempty"`
	OSName     string   `json:"osName,omitempty"`
	OSVersion  string   `json:"osVersion,omitempty"`
	IPs        []string `json:"ips,omitempty"`
	Frozen     bool     `json:"frozen,omitempty"`
	// Filesystems, the load averages and ClockSkewMs are filled by the "health"
	// query; Unsupported names qemu-ga commands this guest's agent lacks.
	Filesystems []VMGuestFilesystem `json:"filesystems,omitempty"`
	Load1       *float64            `json:"load1,omitempty"`
	Load5       *float64            `json:"load5,omitempty"`
	Load15      *float64            `json:"load15,omitempty"`
	ClockSkewMs *int64              `json:"clockSkewMs,omitempty"`
	Unsupported []string            `json:"unsupported,omitempty"`
}

// VMGuestFilesystem is one guest filesystem with usage, as qemu-ga reported it.
type VMGuestFilesystem struct {
	Mountpoint string `json:"mountpoint"`
	Type       string `json:"type,omitempty"`
	UsedBytes  int64  `json:"usedBytes"`
	TotalBytes int64  `json:"totalBytes"`
}

// VMQEMUCapability is what one host's QEMU can actually do.
//
// Every feature in McHarbor's VM surface is a device to emit or a QMP verb to
// call, and hosts run different QEMU builds. Without this the UI offers
// everything and each unsupported feature fails at use time with QEMU's own
// wording; with it, the control renders disabled naming what is missing.
type VMQEMUCapability struct {
	Binary  string `json:"binary,omitempty"`
	Arch    string `json:"arch,omitempty"`
	Version string `json:"version,omitempty"`
	// MachineTypes is what a pinned machine type is checked against. A guest
	// pinned to a type a QEMU upgrade removed will not start, and the warning
	// belongs before the start rather than after it.
	MachineTypes []string `json:"machineTypes,omitempty"`
	Devices      []string `json:"devices,omitempty"`
	CPUModels    []string `json:"cpuModels,omitempty"`
	QMPCommands  []string `json:"qmpCommands,omitempty"`

	// Features collapses the lists above into the answers the UI actually
	// asks, so a frontend never has to know that virtio-mem means both a
	// device and a qom-set verb. Keys are the VMFeature* constants.
	Features map[string]bool `json:"features,omitempty"`

	HugePages *ComputeHugePages `json:"hugePages,omitempty"`
	NUMANodes []ComputeNUMANode `json:"numaNodes,omitempty"`
}

// VMFeature keys carried in VMQEMUCapability.Features. They are stable
// identifiers the frontend gates controls on, mirrored in the plugin.
const (
	VMFeatureVirtioMem   = "virtio_mem"
	VMFeatureCPUHotplug  = "cpu_hotplug"
	VMFeatureHugePages   = "hugepages"
	VMFeatureNUMA        = "numa"
	VMFeatureVirtioFS    = "virtiofs"
	VMFeatureVSock       = "vsock"
	VMFeatureIOThrottle  = "io_throttle"
	VMFeatureIOThread    = "iothread"
	VMFeatureDirtyBitmap = "dirty_bitmap"
	VMFeatureBlockdevBkp = "blockdev_backup"
	VMFeatureVirtioGPU   = "virtio_gpu"
	VMFeatureVirtioSnd   = "virtio_snd"
	VMFeatureUSBU2F      = "usb_u2f"

	// Agent features rather than QEMU ones. Q18 keeps the version at
	// 2026.09.01, so an agent that can do these says so here instead of a
	// new minimum-version gate.
	VMFeatureSPICE        = "spice"
	VMFeatureClone        = "clone"
	VMFeatureMigrateCold  = "migrate_cold"
	VMFeatureMigrateLive  = "migrate_live"
	VMFeatureSerialResize = "serial_resize"
	// VMFeatureHotplug is live disk, disc and NIC changes on a running guest.
	VMFeatureHotplug = "hotplug"
)

// VMResizeResult is what a live memory or vCPU change achieved.
//
// Requested and Actual are both carried because they differ, routinely: a
// memory request is rounded to the host's block size, and an unplug is a
// request the guest may decline outright. Reporting only what was asked for is
// how a refused resize looks successful.
type VMResizeResult struct {
	RequestedMemoryMB int64 `json:"requestedMemoryMb,omitempty"`
	ActualMemoryMB    int64 `json:"actualMemoryMb,omitempty"`
	BlockSizeMB       int64 `json:"blockSizeMb,omitempty"`
	RequestedVCPUs    int   `json:"requestedVcpus,omitempty"`
	ActualVCPUs       int   `json:"actualVcpus,omitempty"`
	// Complied is false when the guest did not reach the requested figure. It
	// is not an error: the operation was accepted and the guest declined.
	Complied bool `json:"complied"`
}

// VMImageOpSpec is one read-only qemu-img subcommand.
//
// Operation is matched against a fixed set on the agent rather than being
// passed to a shell: this runs as root on the host, and the difference between
// an allow-list and a free string is the difference between a diagnostic and a
// remote command execution.
type VMImageOpSpec struct {
	Operation string `json:"operation"` // info | check | map | measure | compare
	// Path is the image being inspected. ComparePath is the second image, used
	// only by compare.
	Path        string `json:"path"`
	ComparePath string `json:"comparePath,omitempty"`
	// Repair asks check to fix what it finds. Refused by the agent unless the
	// guest is stopped, because `check -r` against a disk an active guest has
	// open corrupts it — and only the host can tell whether it is open.
	Repair bool `json:"repair,omitempty"`
	// SizeBytes and Format let measure answer for an image that does not exist
	// yet, which is what the create-disk dialog asks.
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Format    string `json:"format,omitempty"`
}

// VMBackupSpec is one backup run, as the agent receives it.
//
// Kind is what McHarbor asked for, not what the agent will necessarily do:
// only the agent can see whether a disk's dirty bitmap still exists and is
// consistent, so an incremental request may come back as a full copy. That
// decision belongs on the side that can check.
type VMBackupSpec struct {
	RunID   string `json:"runId"`
	ChainID string `json:"chainId,omitempty"`
	Kind    string `json:"kind"` // full | incremental
	// Quiesce freezes the guest's filesystems through its agent for the
	// duration of the copy. Without it the backup is crash-consistent, and
	// the caller records which it got rather than claiming the better one.
	Quiesce bool                `json:"quiesce,omitempty"`
	Disks   []VMBackupDiskSpec  `json:"disks,omitempty"`
	Upload  *VMBackupUploadSpec `json:"upload,omitempty"`
	// ScratchDir is where the artifact is staged before it is streamed.
	// A full run needs free space here of roughly the guest's used size;
	// an incremental needs only the delta.
	ScratchDir string `json:"scratchDir,omitempty"`
}

// VMBackupDiskSpec is one disk to copy.
type VMBackupDiskSpec struct {
	DiskID     string `json:"diskId"`
	VolumePath string `json:"volumePath"`
	// NodeName is the blockdev node QEMU knows this disk by, which is what
	// blockdev-backup addresses rather than the path.
	NodeName string `json:"nodeName,omitempty"`
	// BitmapName is the persistent dirty bitmap backing this disk's chain.
	// Empty on a full run that starts a new chain.
	BitmapName string `json:"bitmapName,omitempty"`
}

// VMBackupUploadSpec is where the agent streams the artifact.
//
// It points back at McHarbor rather than at the real destination: the agent
// never holds a storage credential, and McHarbor fans one stream out to
// every configured destination.
type VMBackupUploadSpec struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// VMBackupDiskResult is what actually happened to one disk.
//
// FellBackToFull is the field the safety of the whole feature rests on. An
// incremental taken against a lost or inconsistent bitmap restores to a
// corrupt disk, so the agent reports the downgrade and McHarbor records a
// full run — never an incremental it did not take.
type VMBackupDiskResult struct {
	DiskID         string `json:"diskId"`
	Kind           string `json:"kind"` // what was actually taken
	BytesWritten   int64  `json:"bytesWritten"`
	BitmapName     string `json:"bitmapName,omitempty"`
	BitmapPresent  bool   `json:"bitmapPresent"`
	FellBackToFull bool   `json:"fellBackToFull,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	// Consistency is crash or filesystem, decided by whether the guest
	// agent answered the freeze — reported rather than assumed.
	Consistency string `json:"consistency,omitempty"`
	Error       string `json:"error,omitempty"`
}

// VMRestoreSpec rebuilds a guest's disks from a chain.
//
// Artifacts arrive newest-last: the full copy first, then each incremental
// in the order it was taken. Applying them out of order silently produces a
// disk that looks intact and is not, so the order is the caller's
// responsibility and the agent applies exactly what it is given.
type VMRestoreSpec struct {
	RunID      string                 `json:"runId"`
	Disks      []VMRestoreDiskSpec    `json:"disks,omitempty"`
	ScratchDir string                 `json:"scratchDir,omitempty"`
	Artifacts  []VMRestoreArtifactRef `json:"artifacts,omitempty"`
}

// VMRestoreDiskSpec is one disk to rebuild.
type VMRestoreDiskSpec struct {
	DiskID     string `json:"diskId"`
	VolumePath string `json:"volumePath"`
}

// VMRestoreArtifactRef is one downloadable piece of a chain.
type VMRestoreArtifactRef struct {
	DiskID string `json:"diskId"`
	Kind   string `json:"kind"` // full | incremental
	URL    string `json:"url"`
	Token  string `json:"token"`
	// Sequence orders the chain. The agent applies ascending and refuses a
	// set with a gap rather than producing a plausible-looking disk.
	Sequence int `json:"sequence"`
	// SizeBytes is what this link actually occupies, which is what the
	// restore's space check needs. Zero from a host too old to send it, and
	// the agent then falls back to the disk's virtual size — over-cautious,
	// but a restore that runs out of room half way is worse.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// CTPayload is the request and the reply body for every ct-family message.
type CTPayload struct {
	Operation   string `json:"operation,omitempty"`
	EnvID       string `json:"envId,omitempty"`
	ContainerID string `json:"containerId,omitempty"`

	Spec         *CTSpec   `json:"spec,omitempty"`
	GraceSeconds int       `json:"graceSeconds,omitempty"`
	DeleteRootfs bool      `json:"deleteRootfs,omitempty"`
	Limits       *CTLimits `json:"limits,omitempty"`

	SnapshotID   string       `json:"snapshotId,omitempty"`
	SnapshotName string       `json:"snapshotName,omitempty"`
	Clone        *CTCloneSpec `json:"clone,omitempty"`

	// Backup and restore. Both carry their own spec rather than reusing the
	// snapshot fields: a snapshot stays on the pool, a backup leaves the host.
	Backup  *CTBackupSpec  `json:"backup,omitempty"`
	Restore *CTRestoreSpec `json:"restore,omitempty"`

	// ImageID names a rootfs image in the host's image cache that ct_create
	// unpacks into the spec's RootfsPath before writing the container state.
	ImageID string `json:"imageId,omitempty"`
	PoolID  string `json:"poolId,omitempty"`

	// Reply-only fields.
	States    []CTState   `json:"states,omitempty"`
	State     *CTState    `json:"state,omitempty"`
	Stats     *CTStats    `json:"stats,omitempty"`
	Processes []CTProcess `json:"processes,omitempty"`
	Applied   *CTLimits   `json:"applied,omitempty"`
	Percent   int         `json:"percent,omitempty"`
	SizeBytes int64       `json:"sizeBytes,omitempty"`
	// BackupResult answers ct_backup_run. Kind is what was actually taken,
	// never what was asked for — only the agent can see whether the previous
	// run's incremental listing survived.
	BackupResult *CTBackupResult `json:"backupResult,omitempty"`
	BackupBytes  int64           `json:"backupBytes,omitempty"`
	// Progress carries an unsolicited ct_backup_progress frame's detail. A map
	// rather than typed fields because the stages a backup reports are the
	// plugin's vocabulary, not the transport's, and widening the transport
	// every time one is added is what the opaque payload exists to avoid.
	Progress  map[string]string `json:"progress,omitempty"`
	Orphans   []string          `json:"orphans,omitempty"`
	Success   bool              `json:"success,omitempty"`
	Error     string            `json:"error,omitempty"`
	ErrorCode string            `json:"errorCode,omitempty"`
}

// CTBackupSpec is one container backup as the agent receives it.
//
// Declared here rather than in the host's shared compute spec: that package is
// ABI-linked into every plugin binary, so a typed field there would take every
// installed plugin offline for the sake of a struct two components share.
type CTBackupSpec struct {
	RunID   string `json:"runId"`
	ChainID string `json:"chainId,omitempty"`
	Kind    string `json:"kind"`

	// RootfsPath, PoolID and PoolBackend are what the quiesce decision turns
	// on: a backend whose snapshot is a readable directory is snapshotted, and
	// anything else freezes the cgroup instead.
	RootfsPath  string `json:"rootfsPath"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`

	// Manifest is the container's definition as McHarbor holds it, written
	// into the artifact verbatim so a restore can rebuild a container McHarbor
	// no longer has a row for.
	Manifest json.RawMessage `json:"manifest,omitempty"`

	// ListingName is the incremental listing this chain accumulates in, the
	// direct analogue of a VM chain's dirty bitmap. Lost, the agent reports a
	// fallback to a full copy rather than producing a delta against nothing.
	ListingName string `json:"listingName,omitempty"`

	Upload     *CTBackupUploadSpec `json:"upload,omitempty"`
	ScratchDir string              `json:"scratchDir,omitempty"`
}

// CTBackupUploadSpec points the agent back at McHarbor rather than at the real
// destination, so no storage credential ever reaches a remote host.
type CTBackupUploadSpec struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// CTBackupResult is what the agent actually did.
//
// FellBackToFull is the field this whole feature's correctness rests on: an
// incremental taken against a lost listing restores to a rootfs missing every
// file changed since the listing went, so a downgrade is reported and
// McHarbor records a full run.
type CTBackupResult struct {
	Kind           string `json:"kind"`
	BytesWritten   int64  `json:"bytesWritten"`
	ListingName    string `json:"listingName,omitempty"`
	ListingPresent bool   `json:"listingPresent"`
	FellBackToFull bool   `json:"fellBackToFull,omitempty"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	// Consistency is "snapshot", "frozen" or "filesystem". There is no
	// "crash" value: a copy of a live rootfs with neither a snapshot nor a
	// freeze is refused rather than taken and labelled.
	Consistency string `json:"consistency,omitempty"`
	// IDShift answers a restore: "none" (the ranges matched), "shifted" (the
	// tree was re-owned into the target range) or "skipped" (no target range
	// was sent). Empty means an agent that predates re-owning.
	IDShift string `json:"idShift,omitempty"`
	Error   string `json:"error,omitempty"`
}

// CTRestoreSpec rebuilds a container's rootfs from a chain.
type CTRestoreSpec struct {
	RunID string `json:"runId"`
	// TargetPath is where the rootfs is rebuilt, which is not always the
	// source container's own path: a restore may be asked for into a new
	// container beside the one that was backed up.
	TargetPath  string `json:"targetPath"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`

	Artifacts  []CTRestoreArtifactRef `json:"artifacts,omitempty"`
	ScratchDir string                 `json:"scratchDir,omitempty"`

	// SourceIDMap is the id range the backup was taken in; nil means it was
	// never recorded and the agent reads it off the restored tree, refusing
	// when the tree is ambiguous. The archives hold host ids either way.
	SourceIDMap *CTIDMap `json:"sourceIdMap,omitempty"`
	// TargetIDMap is the range of the container restored into. The rebuilt
	// tree is re-owned into it before it replaces the rootfs.
	TargetIDMap *CTIDMap `json:"targetIdMap,omitempty"`
}

// CTIDMap is one container's id range as its rootfs holds it on the host.
// Privileged means no user namespace, so the starts and counts are unused.
type CTIDMap struct {
	Privileged bool  `json:"privileged,omitempty"`
	UIDStart   int64 `json:"uidStart,omitempty"`
	UIDCount   int64 `json:"uidCount,omitempty"`
	GIDStart   int64 `json:"gidStart,omitempty"`
	GIDCount   int64 `json:"gidCount,omitempty"`
}

// CTRestoreArtifactRef is one downloadable link of a chain.
//
// Sequence orders it. Applying an incremental before the full it builds on, or
// skipping a link, produces a rootfs that looks intact and is not, so the
// order is explicit rather than implied by the slice.
type CTRestoreArtifactRef struct {
	Part      string `json:"part"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	Sequence  int    `json:"sequence"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
}

// CTSpec is everything the agent needs to materialize and start one system
// container.
type CTSpec struct {
	Name        string `json:"name"`
	RootfsPath  string `json:"rootfsPath"`
	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	// PoolTarget is the pool's root. The agent refuses to populate or remove
	// a rootfs that is not strictly inside it, so a create or a delete can
	// never reach a host path or the pool root itself.
	PoolTarget string `json:"poolTarget,omitempty"`
	// TemplatePath is the rootfs of the template container a create copies
	// from, re-owned into the new container's id range.
	TemplatePath string `json:"templatePath,omitempty"`

	// InitSystem selects the container profile. systemd needs a delegated
	// cgroup v2 subtree, tmpfs at /run and /tmp, a populated /dev, and
	// SIGRTMIN+3 as its stop signal — SIGTERM to systemd's PID 1 is what
	// produces the "container takes 90 seconds to stop" complaint.
	InitSystem string   `json:"initSystem,omitempty"` // systemd | openrc | none
	InitArgv   []string `json:"initArgv,omitempty"`

	// Privileged removes the user namespace. Root inside a privileged
	// container is root on the host in every way that matters, so this is
	// gated behind ct.privileged and never granted by a role preset.
	Privileged bool `json:"privileged,omitempty"`
	Nesting    bool `json:"nesting,omitempty"`

	// Id mapping. Ranges are distinct and non-overlapping per container —
	// overlapping ranges would let one container's root touch another's
	// files, which defeats the point of unprivileged mode.
	UIDMapStart int64  `json:"uidMapStart,omitempty"`
	UIDMapCount int64  `json:"uidMapCount,omitempty"`
	GIDMapStart int64  `json:"gidMapStart,omitempty"`
	GIDMapCount int64  `json:"gidMapCount,omitempty"`
	IDMapMech   string `json:"idmapMechanism,omitempty"` // idmapped | chown

	Limits *CTLimits     `json:"limits,omitempty"`
	NICs   []CTNICSpec   `json:"nics,omitempty"`
	Mounts []CTMountSpec `json:"mounts,omitempty"`

	// Provisioning applied to the rootfs before first boot: hostname, SSH
	// keys, a hashed root password, and the distro-specific network config
	// for a static address.
	Hostname     string   `json:"hostname,omitempty"`
	SSHKeys      []string `json:"sshKeys,omitempty"`
	PasswordHash string   `json:"passwordHash,omitempty"`
	NetConfigFmt string   `json:"netConfigFormat,omitempty"` // systemd-networkd | interfaces | none

	UnitName string `json:"unitName,omitempty"` // mcharbor-ct-<id>
}

// CTLimits are cgroup v2 values. Unified hierarchy only — a cgroup v1 host is
// refused with a clear message rather than half-supported.
type CTLimits struct {
	CPUMax          string `json:"cpuMax,omitempty"` // cgroup cpu.max, e.g. "200000 100000"
	CPUWeight       int    `json:"cpuWeight,omitempty"`
	CPUSet          string `json:"cpuset,omitempty"`
	MemoryMaxBytes  int64  `json:"memoryMaxBytes,omitempty"`
	MemoryHighBytes int64  `json:"memoryHighBytes,omitempty"` // soft throttle before the hard kill
	SwapMaxBytes    int64  `json:"swapMaxBytes,omitempty"`    // 0 by default; swap-in-container surprises people
	PidsMax         int64  `json:"pidsMax,omitempty"`         // fork-bomb protection, genuinely necessary
	IOMax           string `json:"ioMax,omitempty"`
	DiskQuotaBytes  int64  `json:"diskQuotaBytes,omitempty"` // pool quota; dir pools cannot enforce this
}

// CTNICSpec is one veth pair: host end enslaved to the bridge, peer moved into
// the container netns and renamed.
type CTNICSpec struct {
	NICID       string   `json:"nicId,omitempty"`
	NetworkID   string   `json:"networkId,omitempty"`
	Bridge      string   `json:"bridge,omitempty"`
	VLANTag     int      `json:"vlanTag,omitempty"`
	Slot        int      `json:"slot"`
	GuestName   string   `json:"guestName,omitempty"` // eth0
	MAC         string   `json:"mac"`
	VethHost    string   `json:"vethHost,omitempty"`
	AddressMode string   `json:"addressMode,omitempty"` // dhcp | static
	StaticIP    string   `json:"staticIp,omitempty"`
	StaticGW    string   `json:"staticGw,omitempty"`
	StaticDNS   []string `json:"staticDns,omitempty"`
	// StaticIPv6 and StaticGW6 are a second, IPv6 address and default route,
	// applied from the host side beside the IPv4 pair whatever AddressMode says.
	StaticIPv6 string `json:"staticIpv6,omitempty"`
	StaticGW6  string `json:"staticGw6,omitempty"`
	// AttachKind selects how the NIC reaches its network: empty or "bridge"
	// for a tap or veth on a bridge, "macvlan" for a direct child of
	// ParentLink. Denormalised from the network exactly as Bridge is, so the
	// agent never has to resolve a network id.
	AttachKind string `json:"attachKind,omitempty"`
	// ParentLink is the host interface a direct NIC stacks on, and is the
	// network's uplink rather than its VLAN sub-interface: only the agent can
	// tell the operator's own eth0.100 from one McHarbor made, so it resolves
	// VLANTag itself at guest start.
	ParentLink string `json:"parentLink,omitempty"`
	// LinkMode is the direct sub-mode — bridge | vepa | private | passthru.
	LinkMode string `json:"linkMode,omitempty"`
}

// CTMountSpec is one mount into the container. A bind of an arbitrary host
// path is a host-compromise vector and takes ct.privileged; a volume inside
// the pool takes only ct.manage. The agent re-validates the source
// independently — the backend check is UX, the agent check is security.
type CTMountSpec struct {
	MountID     string `json:"mountId,omitempty"`
	Kind        string `json:"kind"` // bind | volume | device
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	ShiftIDs    bool   `json:"shiftIds,omitempty"`
}

// CTCloneSpec describes a clone from a template. On a CoW pool this is a
// metadata operation; on a dir pool it is a copy and reports progress.
type CTCloneSpec struct {
	TargetContainerID string  `json:"targetContainerId"`
	Spec              *CTSpec `json:"spec,omitempty"`
	// Linked asks for a clone that shares the source's data. The reply echoes
	// it with what was actually made, because a rootfs re-owned into a new id
	// range cannot share blocks and the caller must not believe it does.
	Linked bool `json:"linked,omitempty"`
}

// CTState is one container's confirmed state.
type CTState struct {
	ContainerID string   `json:"containerId"`
	State       string   `json:"state"` // defined | running | frozen | stopping | stopped | crashed | unknown
	UnitName    string   `json:"unitName,omitempty"`
	UnitActive  string   `json:"unitActive,omitempty"`
	UnitSub     string   `json:"unitSub,omitempty"`
	InitPID     int      `json:"initPid,omitempty"`
	StartedAt   string   `json:"startedAt,omitempty"`
	IPs         []string `json:"ips,omitempty"` // read from the netns, not from the guest
	ExitReason  string   `json:"exitReason,omitempty"`
	LastError   string   `json:"lastError,omitempty"`
}

// CTStats is one cgroup sample.
//
// lxcfs is deliberately not required, so `free` and /proc/cpuinfo inside the
// container show host values. McHarbor's own numbers read the cgroup and are
// correct; the UI says so in the resources panel rather than requiring a
// second daemon to make `free` pretty.
type CTStats struct {
	ContainerID     string  `json:"containerId"`
	CPUPercent      float64 `json:"cpuPercent,omitempty"`
	CPUUsageNSec    int64   `json:"cpuUsageNsec,omitempty"`
	MemoryBytes     int64   `json:"memoryBytes,omitempty"`
	MemoryMaxBytes  int64   `json:"memoryMaxBytes,omitempty"`
	MemoryHighBytes int64   `json:"memoryHighBytes,omitempty"`
	SwapBytes       int64   `json:"swapBytes,omitempty"`
	PidsCurrent     int64   `json:"pidsCurrent,omitempty"`
	PidsMax         int64   `json:"pidsMax,omitempty"`
	IOReadBytes     int64   `json:"ioReadBytes,omitempty"`
	IOWriteBytes    int64   `json:"ioWriteBytes,omitempty"`
	NetRxBytes      int64   `json:"netRxBytes,omitempty"`
	NetTxBytes      int64   `json:"netTxBytes,omitempty"`
	RootfsUsedBytes int64   `json:"rootfsUsedBytes,omitempty"`
	SampledAt       string  `json:"sampledAt,omitempty"`
}

// CTProcess is one process in the container's cgroup — a genuinely useful
// thing a system container has and a VM does not.
type CTProcess struct {
	PID         int     `json:"pid"` // as seen inside the container
	HostPID     int     `json:"hostPid,omitempty"`
	PPID        int     `json:"ppid,omitempty"`
	User        string  `json:"user,omitempty"`
	Command     string  `json:"command,omitempty"`
	CPUPercent  float64 `json:"cpuPercent,omitempty"`
	MemoryBytes int64   `json:"memoryBytes,omitempty"`
}
