// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package vm

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"golang.org/x/sys/unix"
)

// RestoreLink is one downloaded piece of a chain, already on disk.
type RestoreLink struct {
	DiskID   string
	Kind     string
	Path     string
	Sequence int
}

// RestoreDisk is one disk to rebuild.
type RestoreDisk struct {
	DiskID     string
	VolumePath string
}

// RestoreRequest rebuilds a guest's disks from a downloaded chain.
type RestoreRequest struct {
	VMID       string
	RunID      string
	ScratchDir string
	Disks      []RestoreDisk
	Links      []RestoreLink
}

// ErrNotEnoughSpace means the host cannot hold the rebuilt disk.
//
// Reported before anything is downloaded rather than discovered an hour into
// a transfer, which is the difference between a clear refusal and a restore
// that fails after the operator has already taken the machine down.
var ErrNotEnoughSpace = fmt.Errorf("vm: not enough free space to rebuild this disk")

// RestoreSpaceNeeded is what a restore will require on the host.
type RestoreSpaceNeeded struct {
	// RequiredBytes is the rebuilt disk plus the chain that produces it.
	RequiredBytes int64
	AvailBytes    int64
}

// CheckRestoreSpace reports whether a restore can proceed.
//
// The assemble-beside-then-swap strategy needs room for the finished disk as
// well as the downloaded links. That is the price of the original staying
// untouched until the final move — a failure at any point before it leaves the
// machine exactly as it was, where applying in place would leave a disk that
// is half old and half new, unbootable, and not obviously so.
func CheckRestoreSpace(scratchDir string, diskBytes int64, chainBytes int64) (RestoreSpaceNeeded, error) {
	need := RestoreSpaceNeeded{RequiredBytes: diskBytes + chainBytes}

	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		return need, fmt.Errorf("vm: preparing the restore directory: %w", err)
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(scratchDir, &stat); err != nil {
		return need, fmt.Errorf("vm: reading free space on the restore directory: %w", err)
	}
	need.AvailBytes = int64(stat.Bavail) * int64(stat.Bsize)

	if need.AvailBytes < need.RequiredBytes {
		// Binary units, matching what `df -h` prints on the same host: the
		// operator's next move is to go and look, and a figure they cannot
		// line up against df is a figure they have to convert by hand.
		return need, fmt.Errorf("%w: %s free, %s needed, %s short",
			ErrNotEnoughSpace,
			HumanBytes(need.AvailBytes),
			HumanBytes(need.RequiredBytes),
			HumanBytes(need.RequiredBytes-need.AvailBytes))
	}
	return need, nil
}

// HumanBytes renders a byte count in binary units, as `df -h` does.
//
// Exported because every operator-facing size the agent reports should read
// the same way; a message mixing raw byte counts with formatted ones makes
// the reader check which is which.
func HumanBytes(value int64) string {
	if value <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}

// RestoreDisks rebuilds each disk from its chain and swaps it into place.
//
// The guest must already be stopped; the caller enforces that, because only
// it knows the guest's state. Each disk is assembled beside its target and
// moved over it only once it is complete.
func (m *Manager) RestoreDisks(ctx context.Context, req RestoreRequest) error {
	if len(req.Disks) == 0 {
		return fmt.Errorf("vm: restore %s named no disks", req.RunID)
	}

	lock := m.lockFor(req.VMID)
	lock.Lock()
	defer lock.Unlock()

	byDisk := map[string][]RestoreLink{}
	for _, link := range req.Links {
		byDisk[link.DiskID] = append(byDisk[link.DiskID], link)
	}

	for _, disk := range req.Disks {
		links := byDisk[disk.DiskID]
		if len(links) == 0 {
			return fmt.Errorf("vm: the backup carried nothing for disk %s", disk.DiskID)
		}
		if err := restoreOneDisk(ctx, req.ScratchDir, disk, links); err != nil {
			return err
		}
	}
	return nil
}

// restoreOneDisk assembles one disk and moves it into place.
func restoreOneDisk(ctx context.Context, scratchDir string, disk RestoreDisk, links []RestoreLink) error {
	sort.Slice(links, func(i, j int) bool { return links[i].Sequence < links[j].Sequence })

	// A gap means a link is missing, and applying what is present anyway
	// produces a disk that looks intact and is not. The server checks this
	// too; it is checked again here because this is the side that would
	// actually write the wrong bytes.
	if links[0].Kind != BackupKindFull {
		return fmt.Errorf("vm: the chain for disk %s does not start with a full copy", disk.DiskID)
	}
	for index, link := range links {
		if link.Sequence != links[0].Sequence+index {
			return fmt.Errorf("vm: the chain for disk %s is missing a link", disk.DiskID)
		}
	}

	// Each incremental is a qcow2 holding only its dirty clusters, so the
	// chain is assembled by pointing each link at the one below it and then
	// flattening the top into a single image.
	for index := 1; index < len(links); index++ {
		if _, err := runImgTool(ctx, "rebase", "-u", "-F", "qcow2",
			"-b", links[index-1].Path, links[index].Path); err != nil {
			return fmt.Errorf("vm: assembling the chain for disk %s: %w", disk.DiskID, err)
		}
	}

	assembled := filepath.Join(scratchDir, "restored-"+disk.DiskID+".qcow2")
	errlog.Log(os.Remove(assembled))
	top := links[len(links)-1].Path
	if _, err := runImgTool(ctx, "convert", "-O", "qcow2", top, assembled); err != nil {
		return fmt.Errorf("vm: rebuilding disk %s: %w", disk.DiskID, err)
	}

	// The move is the only destructive step, and it is last. Everything above
	// it can fail without touching the machine's current disk.
	if err := os.Rename(assembled, disk.VolumePath); err != nil {
		// Rename across filesystems fails with EXDEV; the scratch directory is
		// normally beside the pool, but it does not have to be.
		if err := copyOver(assembled, disk.VolumePath); err != nil {
			return fmt.Errorf("vm: putting the rebuilt disk %s in place: %w", disk.DiskID, err)
		}
		errlog.Log(os.Remove(assembled))
	}
	return nil
}

// copyOver replaces a file's contents from another file.
//
// Used only when a rename cannot cross the filesystem boundary. It writes in
// place rather than to a temp file and renaming, because the reason it is
// reached at all is that renaming does not work here.
func copyOver(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { errlog.Close(in) }()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		errlog.Close(out)
		return err
	}
	return out.Close()
}
