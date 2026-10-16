// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fakeagent

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

const demoRefusal = "This operation is not available in demo mode."

// handleHostRPC answers the host-level request/response messages. It returns
// false for a type it does not own so extensions get their turn.
func (s *session) handleHostRPC(msg *coreagent.WSMessage) bool {
	h := s.agent.cfg.Host
	now := time.Now()
	switch msg.Type {
	case coreagent.MsgHostStatsRequest:
		hs := h.SampleHost(now)
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostStatsResponse, ID: msg.ID, HostStats: &coreagent.HostStatsPayload{
			CPUPercent: hs.CPUPercent, MemUsed: hs.MemUsed, Load1: hs.Load1, Load5: hs.Load5, Load15: hs.Load15,
			Uptime: hs.Uptime, FSTotal: h.DiskTotal, FSUsed: h.DiskUsed, Supported: true,
		}})
	case coreagent.MsgHostOSInfoRequest:
		manager := map[string]string{"ubuntu": "apt", "debian": "apt", "rocky": "dnf", "fedora": "dnf", "alpine": "apk"}[h.Distro]
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostOSInfoResponse, ID: msg.ID, HostOSInfo: &coreagent.HostOSInfoPayload{
			PrettyName: prettyName(h), Distro: h.Distro, Version: h.OSVersion, Kernel: h.Kernel, Arch: h.Arch,
			Hostname: h.Hostname, UptimeSeconds: int64(now.Sub(h.BootedAt).Seconds()), Manager: manager,
			Virtualization: map[bool]string{true: "none", false: "kvm"}[h.Compute], RebootRequired: estate.StableHash(h.Key)%5 == 0,
		}})
	case coreagent.MsgHostLogRequest:
		go s.streamHostLog(msg)
	case coreagent.MsgPruneRequest:
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgPruneResponse, ID: msg.ID, Prune: prune(h, msg.Prune)})
	case coreagent.MsgHostUpdateCheck:
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostUpdateResult, ID: msg.ID, HostUpdate: packageUpdates(h)})
	case coreagent.MsgHostUpdateApply:
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostUpdateChunk, ID: msg.ID, HostUpdateChunk: &coreagent.HostUpdateChunkPayload{Line: demoRefusal}})
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostUpdateEnd, ID: msg.ID, HostUpdate: &coreagent.HostUpdatePayload{ExitCode: 1, Success: false, Error: demoRefusal}})
	case coreagent.MsgHostServiceRequest:
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostServiceResponse, ID: msg.ID, HostService: services(h).handle(msg.HostService)})
	case coreagent.MsgAgentUpdateRequest:
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgAgentUpdateResponse, ID: msg.ID, AgentUpdate: &coreagent.AgentUpdatePayload{Success: false, Error: demoRefusal}})
	case coreagent.MsgSnapshotDetectRequest, coreagent.MsgSnapshotCapabilityRequest, coreagent.MsgSnapshotCreateRequest, coreagent.MsgSnapshotRemoveRequest:
		s.reply(&coreagent.WSMessage{Type: snapshotReply(msg.Type), ID: msg.ID, Snapshot: snapshot(msg.Snapshot)})
	case coreagent.MsgTransferPrepare, coreagent.MsgTransferImage, coreagent.MsgTransferArchive, coreagent.MsgTransferProbe, coreagent.MsgTransferRestore:
		id := ""
		if msg.Transfer != nil {
			id = msg.Transfer.TransferID
		}
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgTransferResult, ID: msg.ID, Transfer: &coreagent.TransferPayload{TransferID: id, Success: false, Error: demoRefusal}})
	case coreagent.MsgBackupRun, coreagent.MsgBackupRestore:
		id := ""
		if msg.Backup != nil {
			id = msg.Backup.TransferID
		}
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgTransferResult, ID: msg.ID, Backup: &coreagent.BackupPayload{TransferID: id, Success: false, Stage: "failed"}, Transfer: &coreagent.TransferPayload{TransferID: id, Success: false, Error: demoRefusal}})
	case coreagent.MsgTransferCancel:
	default:
		return false
	}
	return true
}

func snapshotReply(req string) string {
	switch req {
	case coreagent.MsgSnapshotDetectRequest:
		return coreagent.MsgSnapshotDetectResponse
	case coreagent.MsgSnapshotCapabilityRequest:
		return coreagent.MsgSnapshotCapabilityResponse
	case coreagent.MsgSnapshotCreateRequest:
		return coreagent.MsgSnapshotCreateResponse
	}
	return coreagent.MsgSnapshotRemoveResponse
}

func snapshot(req *coreagent.SnapshotPayload) *coreagent.SnapshotPayload {
	out := &coreagent.SnapshotPayload{}
	if req != nil {
		out.Operation, out.EnvID, out.Driver = req.Operation, req.EnvID, req.Driver
	}
	if out.Operation == "detect" || out.Operation == "" {
		out.Success = true
		for _, d := range []string{"zfs", "btrfs", "lvm"} {
			out.Detected = append(out.Detected, coreagent.SnapshotDriverOut{Driver: d, Available: false, Error: "not installed on this demo host"})
		}
		return out
	}
	out.Error = demoRefusal
	return out
}

func prune(h *estate.Host, req *coreagent.PrunePayload) *coreagent.PrunePayload {
	out := &coreagent.PrunePayload{Success: true}
	if req == nil {
		out.Success = false
		out.Error = "invalid prune request"
		return out
	}
	out.Type = req.Type
	switch req.Type {
	case "containers":
		ids, n := h.PruneContainers()
		out.ItemsDeleted, out.SpaceReclaimed = int64(len(ids)), n
	case "images":
		imgs, n := h.PruneImages(false)
		out.ItemsDeleted, out.SpaceReclaimed = int64(len(imgs)), n
	case "volumes":
		vols, n := h.PruneVolumes()
		out.ItemsDeleted, out.SpaceReclaimed = int64(len(vols)), n
	case "networks":
		out.ItemsDeleted = int64(len(h.PruneNetworks()))
	case "system":
		ids, a := h.PruneContainers()
		imgs, b := h.PruneImages(false)
		nets := h.PruneNetworks()
		out.ItemsDeleted = int64(len(ids) + len(imgs) + len(nets))
		out.SpaceReclaimed = a + b
		if req.Volumes {
			vols, c := h.PruneVolumes()
			out.ItemsDeleted += int64(len(vols))
			out.SpaceReclaimed += c
		}
	case "buildcache":
		out.ItemsDeleted, out.SpaceReclaimed = 0, 0
	default:
		out.Success = false
		out.Error = "unknown prune type " + strconv.Quote(req.Type)
	}
	return out
}

func packageUpdates(h *estate.Host) *coreagent.HostUpdatePayload {
	manager := map[string]string{"ubuntu": "apt", "debian": "apt", "rocky": "dnf", "fedora": "dnf", "alpine": "apk"}[h.Distro]
	all := []string{
		"openssl 3.0.13-0ubuntu3.4 -> 3.0.13-0ubuntu3.5",
		"libssl3 3.0.13-0ubuntu3.4 -> 3.0.13-0ubuntu3.5",
		"docker-ce 5:27.3.0-1 -> 5:27.3.1-1",
		"containerd.io 1.7.21-1 -> 1.7.22-1",
		"linux-image-generic 6.8.0-44 -> 6.8.0-45",
		"curl 8.5.0-2ubuntu10.3 -> 8.5.0-2ubuntu10.4",
		"tzdata 2024a-3ubuntu1.1 -> 2024b-0ubuntu0.24.04",
		"systemd 255.4-1ubuntu8.3 -> 255.4-1ubuntu8.4",
		"python3.12 3.12.3-1ubuntu0.1 -> 3.12.3-1ubuntu0.2",
	}
	n := int(estate.StableHash(h.Key) % uint64(len(all)+1))
	return &coreagent.HostUpdatePayload{Manager: manager, Available: n > 0, Updates: all[:n], Success: true}
}

var hostLogTemplates = map[string][]string{
	"system": {"systemd[1]: Started Daily apt download activities.", "systemd[1]: Starting Cleanup of Temporary Directories...", "CRON[%d]: (root) CMD (command -v debian-sa1 > /dev/null && debian-sa1 1 1)", "systemd-timesyncd[%d]: Contacted time server 162.159.200.1:123.", "systemd[1]: logrotate.service: Deactivated successfully."},
	"kernel": {"kernel: [%d.412312] br-%x: port %d(veth%x) entered forwarding state", "kernel: [%d.004120] EXT4-fs (nvme0n1p2): resizing filesystem not needed", "kernel: [%d.918810] IPv6: ADDRCONF(NETDEV_CHANGE): veth%x: link becomes ready"},
	"auth":   {"sshd[%d]: Accepted publickey for admin from 10.0.%d.%d port %d ssh2", "sshd[%d]: pam_unix(sshd:session): session opened for user admin(uid=1000)", "sudo:    admin : TTY=pts/0 ; PWD=/home/admin ; USER=root ; COMMAND=/usr/bin/systemctl status docker"},
	"docker": {"dockerd[%d]: time=\"now\" level=info msg=\"Container failed to exit within 10s of signal 15 - using the force\" container=%x", "dockerd[%d]: time=\"now\" level=info msg=\"ignoring event\" module=libcontainerd namespace=moby topic=/tasks/delete", "containerd[%d]: time=\"now\" level=info msg=\"shim disconnected\" id=%x"},
	"agent":  {"mcharbor-agent[%d]: level=info msg=\"connected to McHarbor\" tls=pinned", "mcharbor-agent[%d]: level=info msg=\"stats batch accepted\" seq=%d", "mcharbor-agent[%d]: level=debug msg=\"proxy request\" method=GET path=/containers/json"},
}

func (s *session) streamHostLog(msg *coreagent.WSMessage) {
	req := msg.HostLogRequest
	source, tail := "system", 200
	if req != nil {
		if req.Source != "" {
			source = req.Source
		}
		if req.Tail > 0 {
			tail = min(req.Tail, 2000)
		}
	}
	tpls, ok := hostLogTemplates[source]
	if !ok {
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostLogEnd, ID: msg.ID, HostLogEnd: &coreagent.HostLogEndPayload{Notices: []string{"no_supported_log_source"}}})
		return
	}
	h := s.agent.cfg.Host
	now := time.Now()
	for i := tail; i > 0; i-- {
		t := now.Add(-time.Duration(i*37) * time.Second)
		seed := estate.StableHash(h.Key + source + strconv.FormatInt(t.Unix(), 10))
		tpl := tpls[seed%uint64(len(tpls))]
		line := t.Format("Jan _2 15:04:05") + " " + h.Hostname + " " + fillArgs(tpl, seed)
		if err := s.send(&coreagent.WSMessage{Type: coreagent.MsgHostLogChunk, ID: msg.ID, HostLogChunk: &coreagent.HostLogChunkPayload{Line: line}}); err != nil {
			return
		}
	}
	s.reply(&coreagent.WSMessage{Type: coreagent.MsgHostLogEnd, ID: msg.ID, HostLogEnd: &coreagent.HostLogEndPayload{}})
}

func fillArgs(tpl string, seed uint64) string {
	var args []any
	for i := 0; i+1 < len(tpl); i++ {
		if tpl[i] == '%' && (tpl[i+1] == 'd' || tpl[i+1] == 'x') {
			seed = seed*6364136223846793005 + 1442695040888963407
			args = append(args, int((seed>>33)%60000)+100)
		}
	}
	return fmt.Sprintf(tpl, args...)
}

type serviceStore struct {
	mu        sync.Mutex
	units     []coreagent.HostServiceUnit
	overrides map[string]string
}

var (
	servicesMu sync.Mutex
	servicesBy = map[string]*serviceStore{}
)

// services returns the host's simulated systemd units, kept per host so a
// change survives the agent reconnecting.
func services(h *estate.Host) *serviceStore {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	if st, ok := servicesBy[h.Key]; ok {
		return st
	}
	st := &serviceStore{overrides: map[string]string{}}
	base := []struct{ name, desc, active, file string }{
		{"mcharbor-agent.service", "McHarbor remote agent", "active", "enabled"},
		{"docker.service", "Docker Application Container Engine", "active", "enabled"},
		{"containerd.service", "containerd container runtime", "active", "enabled"},
		{"ssh.service", "OpenBSD Secure Shell server", "active", "enabled"},
		{"systemd-journald.service", "Journal Service", "active", "static"},
		{"systemd-timesyncd.service", "Network Time Synchronization", "active", "enabled"},
		{"cron.service", "Regular background program processing daemon", "active", "enabled"},
		{"rsyslog.service", "System Logging Service", "active", "enabled"},
		{"unattended-upgrades.service", "Unattended Upgrades Shutdown", "active", "enabled"},
		{"node-exporter.service", "Prometheus node exporter", "active", "enabled"},
		{"fail2ban.service", "Fail2Ban Service", "active", "enabled"},
		{"nfs-client.target", "NFS client services", "active", "enabled"},
		{"smartd.service", "Self Monitoring and Reporting Technology (SMART) Daemon", "active", "enabled"},
		{"backup-sync.service", "Nightly offsite backup sync", "inactive", "disabled"},
		{"wg-quick@wg0.service", "WireGuard via wg-quick(8) for wg0", "active", "enabled"},
		{"apt-daily.service", "Daily apt download activities", "inactive", "static"},
		{"certbot.service", "Certbot renewal", "failed", "enabled"},
	}
	if h.Compute {
		base = append(base, struct{ name, desc, active, file string }{"libvirtd.service", "Virtualization daemon", "active", "enabled"}, struct{ name, desc, active, file string }{"lxc.service", "LXC Container Initialization and Autoboot Code", "active", "enabled"})
	}
	for i, b := range base {
		u := coreagent.HostServiceUnit{Name: b.name, Description: b.desc, LoadState: "loaded", ActiveState: b.active, UnitFileState: b.file, Result: "success"}
		switch b.active {
		case "active":
			u.SubState = "running"
			u.MainPID = 600 + i*37
		case "failed":
			u.SubState = "failed"
			u.Result = "exit-code"
			u.FailureReason = "certbot: Failed to renew certificate shop.acme.example with error: Timeout during connect (likely firewall problem)"
		default:
			u.SubState = "dead"
		}
		st.units = append(st.units, u)
	}
	servicesBy[h.Key] = st
	return st
}

var protectedUnits = []string{"mcharbor-agent", "ssh", "sshd", "systemd-", "dbus", "docker", "containerd"}

func (st *serviceStore) handle(req *coreagent.HostServicePayload) *coreagent.HostServicePayload {
	out := &coreagent.HostServicePayload{Success: true}
	if req == nil {
		return &coreagent.HostServicePayload{ErrorCode: "failed", Error: "invalid request"}
	}
	out.Op, out.Unit, out.Action = req.Op, req.Unit, req.Action
	st.mu.Lock()
	defer st.mu.Unlock()
	find := func() *coreagent.HostServiceUnit {
		for i := range st.units {
			if st.units[i].Name == req.Unit || st.units[i].Name == req.Unit+".service" {
				return &st.units[i]
			}
		}
		return nil
	}
	switch req.Op {
	case coreagent.HostServiceOpList:
		out.Units = append([]coreagent.HostServiceUnit(nil), st.units...)
	case coreagent.HostServiceOpAction:
		u := find()
		if u == nil {
			return &coreagent.HostServicePayload{Op: req.Op, ErrorCode: "invalid_unit", Error: "Unit " + req.Unit + " not found."}
		}
		if (req.Action == coreagent.HostServiceActionStop || req.Action == coreagent.HostServiceActionDisable) && isProtected(u.Name) {
			return &coreagent.HostServicePayload{Op: req.Op, ErrorCode: "protected", Error: u.Name + " is protected"}
		}
		switch req.Action {
		case coreagent.HostServiceActionStart, coreagent.HostServiceActionRestart:
			u.ActiveState, u.SubState, u.Result, u.FailureReason = "active", "running", "success", ""
			u.MainPID = int(time.Now().Unix()%30000) + 1000
			if req.Action == coreagent.HostServiceActionRestart && u.Name == "mcharbor-agent.service" {
				out.Warning = coreagent.HostServiceWarningAgentRestart
			}
		case coreagent.HostServiceActionStop:
			u.ActiveState, u.SubState, u.MainPID = "inactive", "dead", 0
		case coreagent.HostServiceActionEnable:
			u.UnitFileState = "enabled"
		case coreagent.HostServiceActionDisable:
			u.UnitFileState = "disabled"
		}
		out.Units = []coreagent.HostServiceUnit{*u}
	case coreagent.HostServiceOpOverrideGet:
		u := find()
		if u == nil {
			return &coreagent.HostServicePayload{Op: req.Op, ErrorCode: "invalid_unit", Error: "Unit " + req.Unit + " not found."}
		}
		out.Effective = "# /lib/systemd/system/" + u.Name + "\n[Unit]\nDescription=" + u.Description + "\n\n[Service]\nExecStart=/usr/bin/" + trimUnit(u.Name) + "\nRestart=on-failure\n\n[Install]\nWantedBy=multi-user.target\n"
		out.Override, out.OverrideExists = st.overrides[u.Name]
		out.OverridePath = "/etc/systemd/system/" + u.Name + ".d/override.conf"
	case coreagent.HostServiceOpOverridePut:
		u := find()
		if u == nil {
			return &coreagent.HostServicePayload{Op: req.Op, ErrorCode: "invalid_unit", Error: "Unit " + req.Unit + " not found."}
		}
		st.overrides[u.Name] = req.Content
		out.OverrideExists = true
		out.OverridePath = "/etc/systemd/system/" + u.Name + ".d/override.conf"
		out.Reloaded = req.Reload
		if req.Verify {
			out.VerifyOutput = ""
		}
	case coreagent.HostServiceOpOverrideDel:
		if u := find(); u != nil {
			delete(st.overrides, u.Name)
		}
		out.Reloaded = true
	case coreagent.HostServiceOpDaemonReload:
		out.Reloaded = true
	default:
		return &coreagent.HostServicePayload{Op: req.Op, ErrorCode: "unsupported", Error: "unsupported operation"}
	}
	return out
}

func trimUnit(name string) string {
	for _, suffix := range []string{".service", ".target", ".timer"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

func isProtected(name string) bool {
	for _, p := range protectedUnits {
		if len(name) >= len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}
