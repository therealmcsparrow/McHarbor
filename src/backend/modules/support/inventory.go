// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

// stateCategory describes one known DATA_DIR entry. Backup is filled in from the instance
// backup's own FileRoots where the entry is one of them, so the two cannot disagree.
type stateCategory struct {
	id      string
	path    string
	backup  string
	cleanup string
}

// stateCategories are the DATA_DIR entries McHarbor itself creates.
var stateCategories = []stateCategory{
	{"encryption_key", ".encryption_key", BackupIncluded, CleanupNo},
	{"server_secret", ".secret", BackupIncluded, CleanupNo},
	{"tls", "tls", BackupPartial, CleanupNo},
	{"plugins", "plugins", BackupExcluded, CleanupNo},
	{"plugin_catalog", "plugin-catalog", BackupExcluded, CleanupRegenerable},
	{"stacks", "stacks", "", CleanupNo},
	{"files", "files", "", CleanupNo},
	{"custom_nodes", "custom-nodes", "", CleanupNo},
	{"widgets", "widgets", "", CleanupNo},
	{"widgets_state", "widgets-state.json", "", CleanupNo},
	{"workflow_nodes", "workflow-nodes", "", CleanupNo},
	{"git", "git", BackupExcluded, CleanupRegenerable},
	{"container_backups", "backups/containers", BackupExcluded, CleanupManaged},
	{"backup_staging", "backups/tmp", BackupExcluded, CleanupSafe},
	{"instance_backups", "instance-backups", BackupExcluded, CleanupManaged},
	{"restore_staging", ".restore", BackupExcluded, CleanupManaged},
	{"restore_ledger", coreinstancebackup.LedgerFile, "", CleanupNo},
	{"restore_status", "restore-status.json", BackupExcluded, CleanupSafe},
	{"support_bundles", "support-bundles", BackupExcluded, CleanupSafe},
}

// maxWalkFiles caps how many files one category's size walk visits, so a plugin directory
// holding a million cache files cannot turn an inventory request into a minute of disk I/O.
const maxWalkFiles = 200000

// backupFor resolves an entry's backup inclusion against the instance-backup FileRoots.
func backupFor(c stateCategory) string {
	for _, root := range coreinstancebackup.FileRoots {
		if root == c.path {
			return BackupIncluded
		}
	}
	if c.backup == "" {
		return BackupExcluded
	}
	return c.backup
}

// StateInventory lists every DATA_DIR category with its size, writability, instance-backup
// inclusion and cleanup eligibility. It never changes anything.
func (s *Service) StateInventory(ctx context.Context) (Inventory, error) {
	inv := Inventory{DataDir: s.d.DataDir, DatabaseDriver: s.d.DatabaseDriver}
	if total, free, err := localDiskUsage(s.d.DataDir); err == nil {
		inv.DiskBytes, inv.FreeBytes = total, free
	}
	claimed := map[string]bool{"backups": true}
	if s.d.DatabaseDriver == "sqlite" || s.d.DatabaseDriver == "" {
		inv.Entries = append(inv.Entries, s.databaseEntry(claimed))
	} else {
		inv.Entries = append(inv.Entries, InventoryEntry{Category: "database", Path: s.d.DatabaseDriver, Kind: "external", Exists: true, Writable: true, Backup: BackupDatabase, Cleanup: CleanupNo})
	}
	for _, c := range stateCategories {
		if err := ctx.Err(); err != nil {
			return Inventory{}, err
		}
		claimed[strings.SplitN(c.path, "/", 2)[0]] = true
		e := measure(filepath.Join(s.d.DataDir, filepath.FromSlash(c.path)))
		e.Category, e.Path, e.Backup, e.Cleanup = c.id, c.path, backupFor(c), c.cleanup
		inv.Entries = append(inv.Entries, e)
	}
	others, err := os.ReadDir(s.d.DataDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Inventory{}, err
	}
	for _, o := range others {
		if claimed[o.Name()] || strings.HasPrefix(o.Name(), ".mcharbor-write-probe-") {
			continue
		}
		e := measure(filepath.Join(s.d.DataDir, o.Name()))
		e.Category, e.Path, e.Backup, e.Cleanup = "other", o.Name(), BackupExcluded, CleanupUnknown
		inv.Entries = append(inv.Entries, e)
	}
	for _, e := range inv.Entries {
		inv.TotalBytes += e.SizeBytes
	}
	return inv, nil
}

func (s *Service) databaseEntry(claimed map[string]bool) InventoryEntry {
	path := s.d.DatabasePath
	e := measure(path)
	for _, suffix := range []string{"-wal", "-shm"} {
		if side := measure(path + suffix); side.Exists {
			e.SizeBytes += side.SizeBytes
			e.Files += side.Files
		}
	}
	rel, err := filepath.Rel(s.d.DataDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(path)
	} else {
		base := filepath.Base(rel)
		claimed[base], claimed[base+"-wal"], claimed[base+"-shm"] = true, true, true
	}
	e.Category, e.Path, e.Backup, e.Cleanup = "database", filepath.ToSlash(rel), BackupDatabase, CleanupNo
	return e
}

// measure sizes a file or directory and probes whether McHarbor can write to it.
func measure(path string) InventoryEntry {
	st, err := os.Stat(path)
	if err != nil {
		return InventoryEntry{Kind: "missing"}
	}
	if !st.IsDir() {
		return InventoryEntry{Kind: "file", Exists: true, SizeBytes: st.Size(), Files: 1, Writable: fileWritable(path)}
	}
	e := InventoryEntry{Kind: "dir", Exists: true, Writable: dirWritable(path)}
	walkErr := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped so the size is a best-effort total
		}
		if d.IsDir() {
			return nil
		}
		if e.Files >= maxWalkFiles {
			e.Truncated = true
			return fs.SkipAll
		}
		if info, err := d.Info(); err == nil {
			e.SizeBytes += info.Size()
		}
		e.Files++
		return nil
	})
	if walkErr != nil {
		e.Truncated = true
	}
	return e
}

// fileWritable opens the file for writing without truncating or changing it.
func fileWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	return f.Close() == nil
}
