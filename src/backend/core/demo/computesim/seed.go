// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// Tables the seeder writes. They are the plugins' own names; nothing here is
// ever built from input, which is why they can be joined into SQL at all.
const (
	tblComputeEnvs = "plugin_addon_compute_environments"
	tblPools       = "plugin_addon_compute_pools"
	tblNetworks    = "plugin_addon_compute_networks"
	tblLeases      = "plugin_addon_compute_leases"
	tblImages      = "plugin_addon_compute_images"
	tblImageCache  = "plugin_addon_compute_image_cache"
	tblVMs         = "plugin_addon_kvm_vms"
	tblVMDisks     = "plugin_addon_kvm_disks"
	tblVMNICs      = "plugin_addon_kvm_nics"
	tblVMSnapshots = "plugin_addon_kvm_snapshots"
	tblCTs         = "plugin_addon_lxc_containers"
	tblCTNICs      = "plugin_addon_lxc_nics"
	tblCTSnapshots = "plugin_addon_lxc_snapshots"
)

// SeedDB writes the simulated compute estate into the addon-compute, addon-kvm
// and addon-lxc tables for every compute host named in envIDs (estate host key
// to environments.id). It is idempotent: a row that already exists is left
// alone, so a second call inserts nothing and never overwrites operator edits.
func (s *Sim) SeedDB(ctx context.Context, db *sql.DB, envIDs map[string]string) error {
	_, err := s.seedRows(ctx, db, envIDs)
	return err
}

// seedRows is SeedDB returning how many rows it wrote, which is what the
// idempotency test needs to see.
func (s *Sim) seedRows(ctx context.Context, db *sql.DB, envIDs map[string]string) (int64, error) {
	if db == nil {
		return 0, errors.New("computesim: no database")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("computesim: begin seed: %w", err)
	}
	w := &seedWriter{ctx: ctx, tx: tx, columns: map[string]map[string]bool{}}
	for _, key := range s.order {
		envID, ok := envIDs[key]
		if !ok || envID == "" {
			continue
		}
		if err := w.host(s.hosts[key], envID, s.builtAt); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return 0, fmt.Errorf("computesim: seeding %s: %w (rollback: %w)", key, err, rbErr)
			}
			return 0, fmt.Errorf("computesim: seeding %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("computesim: commit seed: %w", err)
	}
	return w.inserted, nil
}

type seedWriter struct {
	ctx      context.Context //nolint:containedctx // seedWriter lives for one seed transaction and carries that transaction's cancellation
	tx       *sql.Tx
	columns  map[string]map[string]bool
	inserted int64
}

// tableColumns reads a table's columns once; only columns that exist are
// written, so a column a later plugin build adds in code is filled when present
// and skipped, not failed on, when this runs against an older schema.
func (w *seedWriter) tableColumns(table string) (cols map[string]bool, err error) {
	if cached, ok := w.columns[table]; ok {
		return cached, nil
	}
	rows, err := w.tx.QueryContext(w.ctx, "PRAGMA table_info("+table+")") // SQL identifier: table is one of the tbl* constants
	if err != nil {
		return nil, fmt.Errorf("reading %s columns: %w", table, err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			cols, err = nil, fmt.Errorf("reading %s columns: %w", table, cerr)
		}
	}()
	cols = map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("reading %s columns: %w", table, err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading %s columns: %w", table, err)
	}
	w.columns[table] = cols
	return cols, nil
}

func (w *seedWriter) insert(table string, row map[string]any) error {
	cols, err := w.tableColumns(table)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return fmt.Errorf("table %s does not exist; the plugin's migrations must run first", table)
	}
	names := make([]string, 0, len(row))
	for name := range row {
		if cols[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	args := make([]any, 0, len(names))
	for _, name := range names {
		args = append(args, row[name])
	}
	query := "INSERT OR IGNORE INTO " + table + " (" + strings.Join(names, ", ") + ") VALUES (" + // SQL identifier: table is a tbl* constant, names are filtered to PRAGMA columns
		strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ") + ")"
	res, err := w.tx.ExecContext(w.ctx, query, args...)
	if err != nil {
		return fmt.Errorf("inserting into %s: %w", table, err)
	}
	w.inserted += db.RowsAffected(res)
	return nil
}

func ts(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func nullIf(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func jsonText(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (w *seedWriter) host(hs *hostSim, envID string, builtAt time.Time) error {
	if err := w.environment(hs, envID); err != nil {
		return err
	}
	for _, p := range hs.pools {
		capacity, used, allocated, volumes := poolStats(hs, p)
		if err := w.insert(tblPools, map[string]any{
			"id": p.ID, "environment_id": envID, "name": p.Name, "backend": p.Backend, "target": p.Target,
			"is_default": boolInt(p.IsDefault), "capacity_bytes": capacity, "used_bytes": used,
			"allocated_bytes": allocated, "volume_count": volumes, "sampled_at": ts(builtAt), "state": "ready",
			"created_at": ts(p.CreatedAt), "updated_at": ts(builtAt),
		}); err != nil {
			return err
		}
	}
	for _, n := range hs.networks {
		if err := w.insert(tblNetworks, map[string]any{
			"id": n.ID, "environment_id": envID, "name": n.Name, "mode": n.Mode, "bridge": n.Bridge,
			"vlan_tag": nullIfZero(n.VLAN), "subnet_cidr": nullIf(n.Subnet), "gateway_ip": nullIf(n.Gateway),
			"dhcp_enabled": boolInt(n.DHCP), "dhcp_range_start": nullIf(n.RangeStart), "dhcp_range_end": nullIf(n.RangeEnd),
			"nat_enabled": boolInt(n.NAT), "uplink_link": nullIf(n.Uplink), "host_access": 0,
			"is_default": boolInt(n.IsDefault), "state": "ready",
			"created_at": ts(n.CreatedAt), "updated_at": ts(builtAt),
		}); err != nil {
			return err
		}
	}
	if err := w.imageCache(hs, envID); err != nil {
		return err
	}
	for _, id := range hs.vmOrder {
		if err := w.vm(hs, envID, hs.vms[id], builtAt); err != nil {
			return err
		}
	}
	for _, id := range hs.ctOrder {
		if err := w.ct(hs, envID, hs.cts[id], builtAt); err != nil {
			return err
		}
	}
	return nil
}

func (w *seedWriter) environment(hs *hostSim, envID string) error {
	caps, err := jsonText(capabilityFor(hs.host, hs, hs.probedAt))
	if err != nil {
		return fmt.Errorf("encoding capability: %w", err)
	}
	var defPool, defNet string
	if len(hs.pools) > 0 {
		defPool = hs.pools[0].ID
	}
	if len(hs.networks) > 0 {
		defNet = hs.networks[0].ID
	}
	created := hs.probedAt.Add(-200 * 24 * time.Hour)
	if err := w.insert(tblComputeEnvs, map[string]any{
		"id": stableID(0, created, "compute-env", hs.host.Key), "environment_id": envID, "compute_enabled": 1,
		"capabilities": caps, "probed_at": ts(hs.probedAt), "default_pool_id": nullIf(defPool),
		"default_network_id": nullIf(defNet), "created_at": ts(created), "updated_at": ts(hs.probedAt),
	}); err != nil {
		return err
	}
	// A row the plugin created lazily before the seed ran is blank, and a blank
	// row is compute switched off; only such a row is completed, never one an
	// operator has configured.
	res, err := w.tx.ExecContext(w.ctx, `
		UPDATE `+tblComputeEnvs+`
		   SET compute_enabled = 1, capabilities = ?, probed_at = ?, default_pool_id = ?, default_network_id = ?, updated_at = ?
		 WHERE environment_id = ? AND capabilities IS NULL AND compute_enabled = 0`,
		caps, ts(hs.probedAt), nullIf(defPool), nullIf(defNet), ts(hs.probedAt), envID)
	if err != nil {
		return fmt.Errorf("completing the compute environment row: %w", err)
	}
	w.inserted += db.RowsAffected(res)
	return nil
}

func (w *seedWriter) imageCache(hs *hostSim, envID string) error {
	cols, err := w.tableColumns(tblImageCache)
	if err != nil || len(cols) == 0 {
		return err
	}
	// bounded by the images of one simulated host.
	for _, img := range hs.images {
		var one int
		err := w.tx.QueryRowContext(w.ctx, "SELECT 1 FROM "+tblImages+" WHERE id = ?", img.ImageID).Scan(&one) // SQL identifier: tblImages is a constant
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking image %s: %w", img.ImageID, err)
		}
		if err := w.insert(tblImageCache, map[string]any{
			"id": stableID(0, img.LastUsed, "image-cache", hs.host.Key, img.ImageID), "image_id": img.ImageID,
			"environment_id": envID, "local_path": img.LocalPath, "size_bytes": img.SizeBytes, "verified": 1,
			"signed": 0, "state": "ready", "progress_pct": 100, "last_used_at": ts(img.LastUsed),
			"created_at": ts(img.LastUsed.Add(-240 * time.Hour)), "updated_at": ts(img.LastUsed),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *seedWriter) lease(hs *hostSim, n *nic, guestKind, guestID, hostname string, started, builtAt time.Time) error {
	for _, net := range hs.networks {
		if net.ID != n.NetworkID || !net.DHCP || n.IP == "" {
			continue
		}
		return w.insert(tblLeases, map[string]any{
			"id": stableID(0, started, "lease", net.ID, n.MAC), "network_id": net.ID, "mac": n.MAC, "ip": n.IP,
			"hostname": hostname, "guest_kind": guestKind, "guest_id": guestID,
			"expires_at": ts(builtAt.Add(12 * time.Hour)), "created_at": ts(started), "updated_at": ts(builtAt),
		})
	}
	return nil
}

func poolOf(hs *hostSim, id string) *pool {
	for _, p := range hs.pools {
		if p.ID == id {
			return p
		}
	}
	return &pool{ID: id, Backend: "dir", Target: "/var/lib/mcharbor/pools/local"}
}

func (w *seedWriter) vm(hs *hostSim, envID string, g *vm, builtAt time.Time) error {
	live := g.State == stateRunning || g.State == statePaused
	row := map[string]any{
		"id": g.ID, "environment_id": envID, "name": g.Name, "description": nullIf(g.Description), "state": g.State,
		"guest_os_family": g.OS.family, "guest_os_variant": g.OS.variant, "vcpus": g.VCPUs, "cpu_model": "host",
		"cpu_sockets": 1, "cpu_cores": g.VCPUs, "cpu_threads": 1, "memory_mb": g.MemoryMB,
		"machine_type": g.MachineType, "firmware": stateOr(g.OS.firmware, "uefi"), "secure_boot": boolInt(g.OS.secureBoot),
		"tpm_enabled": boolInt(g.OS.tpm), "display_type": stateOr(g.OS.display, "virtio-gpu"),
		"autostart": boolInt(g.Autostart), "autostart_order": 0, "autostart_delay_s": 0,
		"is_template": boolInt(g.IsTemplate), "uuid": g.UUID, "unit_name": "mcharbor-vm-" + g.ID,
		"last_started_at": ts(g.StartedAt), "last_error": nullIf(g.LastError),
		"created_at": ts(g.CreatedAt), "updated_at": ts(builtAt),
		"current_snapshot_id": nullIf(g.Current), "run_state": nullIf(g.RunState),
		"virtio_mem": 0, "hugepages": 0, "iothread": 0,
	}
	if live {
		row["memory_current_mb"] = g.MemoryMB
	}
	if g.State == stateRunning {
		if g.OS.guestAgent {
			info := guestInfo(g, builtAt)
			ips, err := jsonText(info.IPs)
			if err != nil {
				return err
			}
			row["guest_agent_state"], row["guest_reported_ips"] = "connected", ips
			row["guest_hostname"], row["guest_os_name"], row["guest_os_version"] = info.Hostname, info.OSName, info.OSVersion
		} else {
			row["guest_agent_state"] = "absent"
		}
	}
	if err := w.insert(tblVMs, row); err != nil {
		return err
	}
	for _, d := range g.Disks {
		p := poolOf(hs, d.PoolID)
		disk := map[string]any{
			"id": d.ID, "vm_id": g.ID, "pool_id": p.ID, "pool_backend": p.Backend, "pool_target": p.Target,
			"slot": d.Slot, "bus": d.Bus, "volume_path": d.VolumePath, "format": d.Format, "size_bytes": d.SizeBytes,
			"cache_mode": "none", "discard": 1, "readonly": 0, "is_boot": boolInt(d.IsBoot), "is_cdrom": boolInt(d.IsCDROM),
			"share_rw": 0, "created_at": ts(g.CreatedAt), "updated_at": ts(builtAt),
		}
		if d.IsBoot {
			disk["boot_index"] = 1
		}
		if err := w.insert(tblVMDisks, disk); err != nil {
			return err
		}
	}
	for _, n := range g.NICs {
		if err := w.insert(tblVMNICs, map[string]any{
			"id": n.ID, "vm_id": g.ID, "network_id": n.NetworkID, "bridge": n.Bridge, "slot": n.Slot,
			"model": "virtio-net", "mac": n.MAC, "tap_name": fmt.Sprintf("mhv%s%d", tail(g.ID, 8), n.Slot),
			"created_at": ts(g.CreatedAt), "updated_at": ts(builtAt),
		}); err != nil {
			return err
		}
		if g.State == stateRunning {
			if err := w.lease(hs, n, "vm", g.ID, g.Name, g.StartedAt, builtAt); err != nil {
				return err
			}
		}
	}
	mechanism := "dir"
	if len(g.Disks) > 0 {
		mechanism = poolOf(hs, g.Disks[0].PoolID).Backend
	}
	for i, snap := range g.Snapshots {
		if err := w.insert(tblVMSnapshots, map[string]any{
			"id": snap.ID, "vm_id": g.ID, "parent_id": nullIf(snap.ParentID), "name": snap.Name,
			"mechanism": mechanism, "consistency": snap.Consistency, "with_memory": boolInt(snap.WithMemory),
			"size_bytes": snap.SizeBytes, "chain_depth": i + 1, "handle": snap.Handle,
			"created_at": ts(snap.CreatedAt), "updated_at": ts(snap.CreatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func ctMechanism(backend string) string {
	switch backend {
	case "zfs", "btrfs", "lvm":
		return backend
	}
	return "rsync"
}

func (w *seedWriter) ct(hs *hostSim, envID string, c *ct, builtAt time.Time) error {
	p := poolOf(hs, c.PoolID)
	limits := limitsOf(c)
	applied, err := jsonText(limits)
	if err != nil {
		return err
	}
	live := c.State == stateRunning || c.State == stateFrozen
	row := map[string]any{
		"id": c.ID, "environment_id": envID, "name": c.Name, "description": nullIf(c.Description), "state": c.State,
		"os_family": c.OS.family, "os_version": c.OS.version, "pool_id": p.ID, "pool_backend": p.Backend,
		"pool_target": p.Target, "rootfs_path": c.RootfsPath, "init_system": stateOr(c.OS.init, "systemd"),
		"init_argv": "[]", "privileged": boolInt(c.Privileged), "nesting": boolInt(c.Nesting),
		"uid_map_start": 100000, "uid_map_count": 65536, "gid_map_start": 100000, "gid_map_count": 65536,
		"idmap_mechanism": "idmapped", "cpu_max": limits.CPUMax, "cpu_weight": limits.CPUWeight,
		"memory_max_bytes": limits.MemoryMaxBytes, "memory_high_bytes": limits.MemoryHighBytes, "swap_max_bytes": 0,
		"pids_max": limits.PidsMax, "disk_quota_bytes": limits.DiskQuotaBytes, "applied_limits": applied,
		"autostart": boolInt(c.Autostart), "autostart_order": 0, "autostart_delay_s": 0,
		"is_template": boolInt(c.IsTemplate), "unit_name": "mcharbor-ct-" + c.ID,
		"last_started_at": ts(c.StartedAt), "exit_reason": nullIf(c.ExitReason), "last_error": nullIf(c.LastError),
		"created_at": ts(c.CreatedAt), "updated_at": ts(builtAt),
	}
	if live {
		st := ctState(c)
		ips, err := jsonText(st.IPs)
		if err != nil {
			return err
		}
		row["observed_ips"], row["init_pid"], row["limits_applied_at"] = ips, c.InitPID, ts(c.StartedAt)
	}
	if err := w.insert(tblCTs, row); err != nil {
		return err
	}
	for _, n := range c.NICs {
		if err := w.insert(tblCTNICs, map[string]any{
			"id": n.ID, "container_id": c.ID, "network_id": n.NetworkID, "bridge": n.Bridge, "slot": n.Slot,
			"guest_name": fmt.Sprintf("eth%d", n.Slot), "mac": n.MAC, "veth_host": fmt.Sprintf("mhc%s%d", tail(c.ID, 8), n.Slot),
			"address_mode": "dhcp", "created_at": ts(c.CreatedAt), "updated_at": ts(builtAt),
		}); err != nil {
			return err
		}
		if c.State == stateRunning {
			if err := w.lease(hs, n, "ct", c.ID, c.Name, c.StartedAt, builtAt); err != nil {
				return err
			}
		}
	}
	for i, snap := range c.Snapshots {
		if err := w.insert(tblCTSnapshots, map[string]any{
			"id": snap.ID, "container_id": c.ID, "parent_id": nullIf(snap.ParentID), "name": snap.Name,
			"mechanism": ctMechanism(p.Backend), "was_frozen": 0, "size_bytes": snap.SizeBytes, "chain_depth": i + 1,
			"handle": snap.Handle, "created_at": ts(snap.CreatedAt), "updated_at": ts(snap.CreatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}
