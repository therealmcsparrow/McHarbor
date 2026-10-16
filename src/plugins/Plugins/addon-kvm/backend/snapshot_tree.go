// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/xid"
)

// Snapshots form a tree, not a chain.
//
// A guest has a *current* node — the snapshot its disks descend from. A new
// snapshot hangs off that node, and reverting moves the pointer instead of
// destroying everything below it. That is what Hyper-V and VirtualBox do, and
// it is the only shape in which "go back, try again, come back" is expressible.
//
// The storage underneath is what makes it possible: every backend's snapshot
// leaves the snapshot's own file intact and gives the guest a fresh layer on
// top, so two siblings can be backed by the same parent without either seeing
// the other's writes.

// maxSnapshotTotal caps how many snapshots one guest may hold across all of its
// branches. maxSnapshotChain still caps the depth of any single branch, which
// is the number that actually costs read performance.
const maxSnapshotTotal = 64

// snapshotSlug reduces a display name to the identifier the agent uses on disk.
//
// The two are deliberately different things. A name is for the operator and may
// contain anything they can type; a slug becomes a path component, a ZFS
// snapshot name and an LVM logical volume name, all of which are far stricter
// than a sentence. The xid suffix makes it unique without the operator having
// to care, which also means reverting to a snapshot twice never collides.
func snapshotSlug(name string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		// A name made entirely of characters a filesystem will not take — CJK,
		// emoji — still deserves a snapshot. The suffix alone identifies it.
		slug = "snap"
	}
	return slug + "-" + xid.New().String()
}

// validateSnapshotName checks what an operator typed.
//
// Free text, because the slug carries the filesystem's rules. Only the two
// things that would break something downstream are refused: nothing at all, and
// a name too long to display.
func validateSnapshotName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNameRequired
	}
	if len([]rune(name)) > 120 {
		return "", ErrNameTooLong
	}
	if strings.ContainsAny(name, "\n\r\t") {
		return "", ErrNameInvalid
	}
	return name, nil
}

// ensureNICSchema adds the recorded PCI address and how the NIC attaches.
//
// Same reason as the columns below: a plugin's migrations run once, at install,
// so a numbered file never reaches an installation that already has this
// plugin, and SQLite has no ADD COLUMN IF NOT EXISTS.
func (s *Service) ensureNICSchema(ctx context.Context) error {
	columns := []struct {
		definition string
		what       string
	}{
		{"pci_addr TEXT", "PCI address"},
		{"attach_kind TEXT", "attachment kind"},
		{"parent_link TEXT", "parent interface"},
		{"link_mode TEXT", "link mode"},
		{"link_down INTEGER NOT NULL DEFAULT 0", "link state"},
	}
	for _, wanted := range columns {
		if _, err := s.db.Exec(ctx,
			`ALTER TABLE plugin_addon_kvm_nics ADD COLUMN `+wanted.definition); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("adding the NIC %s column: %w", wanted.what, err)
		}
	}
	return nil
}

// ensureSnapshotTreeSchema adds the current-snapshot pointer.
//
// A plugin's migrations run once, at install, so a new numbered file never
// reaches an installation that already has this plugin. SQLite has no
// ADD COLUMN IF NOT EXISTS, so a second run reports a duplicate column and that
// is the expected state rather than a failure.
func (s *Service) ensureSnapshotTreeSchema(ctx context.Context) error {
	if _, err := s.db.Exec(ctx,
		`ALTER TABLE plugin_addon_kvm_vms ADD COLUMN current_snapshot_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("adding the current snapshot column: %w", err)
	}
	if _, err := s.db.Exec(ctx,
		`ALTER TABLE plugin_addon_kvm_snapshot_schedules ADD COLUMN pause_guest INTEGER NOT NULL DEFAULT 1`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("adding the schedule pause column: %w", err)
	}
	// Installations that predate the tree have a linear chain whose parents are
	// already correct, so the tip of that chain is the right starting pointer.
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_vms
		   SET current_snapshot_id = (
		       SELECT id FROM plugin_addon_kvm_snapshots
		        WHERE vm_id = plugin_addon_kvm_vms.id
		        ORDER BY chain_depth DESC, created_at DESC LIMIT 1)
		 WHERE current_snapshot_id IS NULL
		   AND EXISTS (SELECT 1 FROM plugin_addon_kvm_snapshots
		                WHERE vm_id = plugin_addon_kvm_vms.id)`); err != nil {
		return fmt.Errorf("seeding the current snapshot pointer: %w", err)
	}
	return nil
}

// setCurrentSnapshot moves a guest's pointer to the node its disks now descend
// from. An empty id means the guest is back on its original disks.
func (s *Service) setCurrentSnapshot(ctx context.Context, vmID, snapshotID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE plugin_addon_kvm_vms SET current_snapshot_id = ? WHERE id = ?`,
		nullIfEmpty(snapshotID), vmID)
	if err != nil {
		return fmt.Errorf("recording the current snapshot: %w", err)
	}
	return nil
}

// countChildren fills in ChildCount for every snapshot in the list.
//
// Computed here rather than queried per row: the whole set is already in hand,
// and the UI needs the count on every node to decide which deletes it may
// offer at all.
func countChildren(snapshots []Snapshot) {
	children := make(map[string]int, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.ParentID != "" {
			children[snapshot.ParentID]++
		}
	}
	for i := range snapshots {
		snapshots[i].ChildCount = children[snapshots[i].ID]
	}
}

// branchDepth is how deep the branch ending at parentID runs.
//
// The total number of snapshots does not matter to read performance; the length
// of one backing chain does, and with a tree those are no longer the same
// number.
func branchDepth(snapshots []Snapshot, parentID string) int {
	byID := make(map[string]Snapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byID[snapshot.ID] = snapshot
	}
	depth := 0
	for parentID != "" {
		parent, ok := byID[parentID]
		if !ok {
			break
		}
		depth++
		parentID = parent.ParentID
		if depth > maxSnapshotTotal {
			// A cycle can only exist if the rows were edited by hand, but an
			// infinite loop here would hang the request rather than fail it.
			break
		}
	}
	return depth
}

// childOf returns the one snapshot hanging directly off parentID, when there is
// exactly one. The agent needs it to know which branch absorbs a deleted layer.
func childOf(snapshots []Snapshot, parentID string) *Snapshot {
	var found *Snapshot
	for i := range snapshots {
		if snapshots[i].ParentID != parentID {
			continue
		}
		if found != nil {
			return nil
		}
		found = &snapshots[i]
	}
	return found
}
