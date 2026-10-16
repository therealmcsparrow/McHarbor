// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// ErrNoGNUTar means the host's tar is not GNU tar.
//
// A hard requirement rather than a preference: --listed-incremental is what
// makes an incremental chain possible at all, and --xattrs plus --acls are what
// make a restored rootfs actually work — file capabilities on ping and dumpcap
// live in xattrs, and a tar that drops them produces a container that boots and
// then fails in ways nobody traces back here.
var ErrNoGNUTar = errors.New("lxc: system container backups need GNU tar on the host")

// backupTimeout bounds producing or applying one artifact.
//
// Hours, because a rootfs of tens of gigabytes legitimately takes them. It is
// bounded at all so a wedged filesystem cannot hold the agent's goroutine for
// the life of the process.
const backupTimeout = 12 * time.Hour

// BackupRequest is one container backup as the runtime hands it to the manager.
type BackupRequest struct {
	CTID     string
	RunID    string
	Kind     string
	Manifest []byte

	// RootfsPath is the live tree. It is read through Snapshot's path instead
	// whenever a snapshot is taken, so this is never the source for a running
	// container on a snapshot-capable pool.
	RootfsPath string

	// Snapshot takes a point-in-time copy of the rootfs and returns a
	// directory the copy can be read through, plus the call that releases it.
	// nil, or an error, means this pool cannot produce one and the cgroup
	// freezer is used instead.
	Snapshot func(ctx context.Context) (path string, release func(), err error)

	// SnapshotIsCopy marks a Snapshot that copies rather than snapshots — a dir
	// pool's tree copy. It is taken without freezing and recorded as a live copy.
	SnapshotIsCopy bool

	// ListingName is the file this chain accumulates its incremental listing
	// in. GNU tar rewrites it on every level, so it is the direct analogue of
	// a VM chain's dirty bitmap — and it is lost the same ways.
	ListingName string

	ScratchDir string

	OnPhase    func(phase, detail string)
	OnProgress func(copied int64)
}

// BackupResult is what one backup actually produced.
type BackupResult struct {
	Kind           string
	Consistency    string
	ListingName    string
	ListingPresent bool
	FellBackToFull bool
	FallbackReason string

	RootfsArchive string
	ManifestFile  string
	BytesWritten  int64
}

// RestoreRequest rebuilds a rootfs from an ordered chain of artifacts.
type RestoreRequest struct {
	CTID       string
	RunID      string
	TargetPath string
	// Links are the chain's archives, already ordered oldest first. Applying
	// them out of order, or skipping one, produces a tree that looks complete
	// and is not, so the caller sorts and the manager trusts that.
	Links      []string
	ScratchDir string
	OnPhase    func(phase, detail string)

	// SourceIDMap is the range the backup was taken in, as McHarbor recorded
	// it; nil means it was not recorded and is read off the restored tree.
	SourceIDMap *IDMap
	// TargetIDMap is the range of the container restored into. nil comes from a
	// McHarbor that predates re-owning, and leaves the owners as archived.
	TargetIDMap *IDMap
}

// RestoreResult is what a restore did about ownership, which McHarbor needs to
// know before it calls a restore into another id range finished.
type RestoreResult struct {
	IDShift      string
	EntriesMoved int
}

// Backup produces a compressed tar of a container's rootfs plus its manifest.
//
// The quiesce decision is made here rather than by the caller because only this
// layer can see both the container's live state and whether the pool answered
// with a readable snapshot — and the refusal when neither is available has to
// happen before a single byte is written.
func (m *Manager) Backup(ctx context.Context, req BackupRequest) (BackupResult, error) {
	if err := assertGNUTar(ctx); err != nil {
		return BackupResult{}, err
	}
	if strings.TrimSpace(req.RootfsPath) == "" {
		return BackupResult{}, fmt.Errorf("lxc: backup of %s carried no rootfs path", req.CTID)
	}
	if err := os.MkdirAll(req.ScratchDir, 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("lxc: preparing the backup scratch directory: %w", err)
	}

	unlock := m.lockFor(req.CTID)
	unlock.Lock()
	defer unlock.Unlock()

	ctx, cancel := context.WithTimeout(ctx, backupTimeout)
	defer cancel()

	source, consistency, isCopy, release, err := m.quiesce(ctx, req)
	if err != nil {
		return BackupResult{}, err
	}
	defer release()

	result := BackupResult{
		Kind:        req.Kind,
		Consistency: consistency,
		ListingName: req.ListingName,
	}

	listing, working, err := m.prepareListing(req)
	if err != nil {
		return BackupResult{}, err
	}
	result.ListingPresent = working != ""
	if req.Kind == "incremental" {
		// Without a listing tar produces a level-0 archive and calls it whatever
		// it was asked to; against a listing from another inode numbering it
		// produces one that cannot be restored. Either way the downgrade is
		// reported rather than hidden.
		if reason := incrementalBlocker(result.ListingPresent, readListingOrigin(listing), isCopy); reason != "" {
			result.Kind = "full"
			result.FellBackToFull = true
			result.FallbackReason = reason
		}
	}

	scratchListing := filepath.Join(req.ScratchDir, req.RunID+".snar")
	if result.Kind == "full" {
		errlog.Log(os.Remove(scratchListing))
	} else if err := copyFile(working, scratchListing); err != nil {
		return BackupResult{}, err
	}

	phase(req.OnPhase, "manifesting", "writing the container's definition into the artifact")
	result.ManifestFile = filepath.Join(req.ScratchDir, req.RunID+"-manifest.json")
	if err := os.WriteFile(result.ManifestFile, req.Manifest, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("lxc: writing the backup manifest: %w", err)
	}

	phase(req.OnPhase, "archiving", "reading the rootfs through the "+consistency+" copy")
	result.RootfsArchive = filepath.Join(req.ScratchDir, req.RunID+"-rootfs.tar.gz")
	if err := runTarCreate(ctx, source, result.RootfsArchive, scratchListing); err != nil {
		errlog.Log(os.Remove(result.RootfsArchive))
		return BackupResult{}, err
	}

	// The listing advances only now. GNU tar rewrites it in place, so running
	// against the live file would have advanced this chain's baseline even on a
	// run that failed — and the next incremental would then be a delta against
	// data that never reached a destination.
	// The old origin goes first: a listing whose origin is missing is refused as
	// a base, so a crash between the two writes costs one full copy, never a bad
	// incremental.
	if err := os.Remove(listingOriginPath(listing)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return BackupResult{}, fmt.Errorf("lxc: clearing the incremental listing's origin: %w", err)
	}
	if err := os.Rename(scratchListing, listing); err != nil {
		return BackupResult{}, fmt.Errorf("lxc: recording the incremental listing: %w", err)
	}
	if err := writeListingOrigin(listing, listingOriginFor(isCopy)); err != nil {
		m.logBackup("lxc: recording where the incremental listing was read from failed", req.CTID, err)
	}

	for _, path := range []string{result.RootfsArchive, result.ManifestFile} {
		if info, statErr := os.Stat(path); statErr == nil {
			result.BytesWritten += info.Size()
		}
	}
	if req.OnProgress != nil {
		req.OnProgress(result.BytesWritten)
	}
	return result, nil
}

// quiesce establishes the facts decideQuiesce needs, performs whichever
// mechanism it picks, and returns the tree the archive reads from.
//
// The effects are here and the policy is in decideQuiesce, so the one case that
// matters — executing, no snapshot, no freeze — is refused by a function that can
// be read and tested without a container in front of it.
func (m *Manager) quiesce(
	ctx context.Context,
	req BackupRequest,
) (source string, consistency string, isCopy bool, release func(), err error) {
	state, err := m.Status(ctx, req.CTID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", "", false, func() {}, err
	}

	facts := quiesceFacts{
		Live:          state.State == StateRunning || state.State == StateFrozen,
		AlreadyFrozen: state.State == StateFrozen,
		RootfsPath:    req.RootfsPath,
	}
	release = func() {}

	if planSnapshot(facts.Live, req.Snapshot != nil).Take {
		if req.SnapshotIsCopy {
			phase(req.OnPhase, "quiescing", "copying the rootfs while the container keeps running")
		} else {
			phase(req.OnPhase, "quiescing", "taking a pool snapshot of the rootfs")
		}
		path, releaseSnap, snapErr := req.Snapshot(ctx)
		switch {
		case snapErr == nil && path != "":
			facts.SnapshotPath = path
			facts.SnapshotIsCopy = req.SnapshotIsCopy
			release = releaseSnap
		case snapErr != nil:
			phase(req.OnPhase, "quiescing",
				"this pool could not snapshot the rootfs, so the container is being frozen instead")
		}
	}

	needFreeze := facts.Live && facts.SnapshotPath == "" && !facts.AlreadyFrozen
	if needFreeze {
		phase(req.OnPhase, "quiescing", "freezing the container's cgroup for the copy")
		freezeErr := m.Freeze(ctx, req.CTID)
		facts.FreezeOK = freezeErr == nil
		if facts.FreezeOK {
			release = func() {
				// Thawed on every path out. A container left frozen by a failed
				// backup is indistinguishable from a hung one from the outside.
				thawCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				if thawErr := m.Unfreeze(thawCtx, req.CTID); thawErr != nil {
					m.logBackup("lxc: thawing after a backup failed", req.CTID, thawErr)
				}
			}
		} else {
			m.logBackup("lxc: freezing for a backup failed", req.CTID, freezeErr)
		}
	}

	source, consistency, err = decideQuiesce(facts)
	if err != nil {
		release()
		return "", "", false, func() {}, err
	}
	return source, consistency, readsCopy(facts, source), release, nil
}

// prepareListing returns where this chain's listing lives and a readable copy
// of it, or an empty working path when there is none yet.
func (m *Manager) prepareListing(req BackupRequest) (listing string, working string, err error) {
	if strings.TrimSpace(req.ListingName) == "" {
		return "", "", fmt.Errorf("lxc: backup of %s named no incremental listing", req.CTID)
	}
	dir := filepath.Join(RuntimeRoot(), ".backup-listings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("lxc: preparing the incremental listing directory: %w", err)
	}
	listing = filepath.Join(dir, filepath.Base(req.ListingName)+".snar")
	if _, statErr := os.Stat(listing); statErr != nil {
		return listing, "", nil //nolint:nilerr // a missing listing file means no prior level exists, so the run starts as a full backup
	}
	return listing, listing, nil
}

// RestoreRootfs rebuilds a rootfs from a chain, beside the target and then in
// one move.
//
// Never in place. An extract straight over the live tree leaves a half-restored
// rootfs behind if anything fails, and a half-restored rootfs boots.
//
// The archives hold host ids, so a restore into a container with another id
// range re-owns the rebuilt tree into that range: once, after the whole chain
// is applied, and before the swap, so a refusal leaves the target untouched.
func (m *Manager) RestoreRootfs(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if err := assertGNUTar(ctx); err != nil {
		return RestoreResult{}, err
	}
	if len(req.Links) == 0 {
		return RestoreResult{}, fmt.Errorf("lxc: the restore of %s carried no archives", req.CTID)
	}
	if strings.TrimSpace(req.TargetPath) == "" {
		return RestoreResult{}, fmt.Errorf("lxc: the restore of %s carried no target path", req.CTID)
	}
	for _, check := range []struct {
		name string
		idm  *IDMap
	}{{"source", req.SourceIDMap}, {"target", req.TargetIDMap}} {
		if check.idm == nil {
			continue
		}
		if err := check.idm.Validate(); err != nil {
			return RestoreResult{}, fmt.Errorf("lxc: the restore of %s named an unusable %s range: %w",
				req.CTID, check.name, err)
		}
	}

	unlock := m.lockFor(req.CTID)
	unlock.Lock()
	defer unlock.Unlock()

	ctx, cancel := context.WithTimeout(ctx, backupTimeout)
	defer cancel()

	state, err := m.Status(ctx, req.CTID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return RestoreResult{}, err
	}
	if state.State == StateRunning || state.State == StateFrozen {
		// Replacing the rootfs under a running init leaves its processes
		// holding inodes that no longer exist, and it does not fail cleanly.
		return RestoreResult{}, fmt.Errorf("lxc: %s is still running, so its rootfs cannot be replaced", req.CTID)
	}

	staging := req.TargetPath + ".restore-" + req.RunID
	if err := os.RemoveAll(staging); err != nil {
		return RestoreResult{}, fmt.Errorf("lxc: clearing the restore staging tree: %w", err)
	}
	// A recreate restores into a container that has no directory yet. What this
	// call creates for the staging tree it removes again on failure, so a failed
	// recreate leaves no empty containers/<id>/ behind for the operator to find.
	created := missingAncestors(filepath.Dir(staging))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return RestoreResult{}, fmt.Errorf("lxc: creating the restore staging tree: %w", err)
	}
	restored := false
	defer func() {
		errlog.Log(os.RemoveAll(staging))
		if !restored {
			for _, dir := range created {
				errlog.Log(os.Remove(dir))
			}
		}
	}()

	for index, archive := range req.Links {
		phase(req.OnPhase, "rebuilding",
			fmt.Sprintf("applying archive %d of %d", index+1, len(req.Links)))
		if err := runTarExtract(ctx, staging, archive); err != nil {
			return RestoreResult{}, fmt.Errorf("lxc: archive %d of %d could not be applied, and the target "+
				"rootfs was left untouched: %w", index+1, len(req.Links), err)
		}
	}

	result, err := m.reownStaging(ctx, req, staging)
	if err != nil {
		return RestoreResult{}, err
	}

	retired := req.TargetPath + ".replaced-" + req.RunID
	if _, statErr := os.Stat(req.TargetPath); statErr == nil {
		if err := os.Rename(req.TargetPath, retired); err != nil {
			return RestoreResult{}, fmt.Errorf("lxc: moving the old rootfs aside: %w", err)
		}
	}
	if err := os.Rename(staging, req.TargetPath); err != nil {
		// Put the original back rather than leaving the container with no
		// rootfs at all: the restore failed, the container should not have.
		errlog.Log(os.Rename(retired, req.TargetPath))
		return RestoreResult{}, fmt.Errorf("lxc: moving the restored rootfs into place: %w", err)
	}
	restored = true
	if err := os.RemoveAll(retired); err != nil {
		m.logBackup("lxc: removing the replaced rootfs failed", req.CTID, err)
	}
	return result, nil
}

// reownStaging gives the rebuilt tree the target container's owners, or
// reports that it was asked to leave them as archived.
func (m *Manager) reownStaging(ctx context.Context, req RestoreRequest, staging string) (RestoreResult, error) {
	if req.TargetIDMap == nil {
		return RestoreResult{IDShift: IDShiftSkipped}, nil
	}
	remap, err := ResolveRestoreRemap(ctx, staging, req.SourceIDMap, *req.TargetIDMap)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("lxc: the restored rootfs could not be given this container's owners, "+
			"and the target rootfs was left untouched: %w", err)
	}
	if remap.Identity() {
		return RestoreResult{IDShift: IDShiftNone}, nil
	}
	phase(req.OnPhase, "reowning", fmt.Sprintf("moving file owners from %s to %s", remap.From, remap.To))
	moved, err := ReownTree(ctx, staging, remap)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("lxc: the restored rootfs could not be given this container's owners, "+
			"and the target rootfs was left untouched: %w", err)
	}
	return RestoreResult{IDShift: IDShiftApplied, EntriesMoved: moved}, nil
}

// missingAncestors lists dir and each parent above it that does not exist yet,
// innermost first, stopping at the first one that does.
func missingAncestors(dir string) []string {
	var out []string
	for current := filepath.Clean(dir); ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			return out
		}
		parent := filepath.Dir(current)
		if parent == current {
			return out
		}
		out = append(out, current)
	}
}

// tarFlags are shared by create and extract, and every one of them earns its
// place: numeric owner because the container's own /etc/passwd is not the
// host's, xattrs and acls because file capabilities and posix ACLs live there
// and a restored rootfs without them is subtly broken, selinux because a
// labelled distribution will not boot relabelled.
var tarFlags = []string{
	"--numeric-owner", "--xattrs", "--xattrs-include=*", "--acls", "--selinux",
}

// runTarCreate writes one archive, incrementally when a listing is supplied.
//
// --no-check-device because each run may read the same tree through a different
// mount — a snapshot, the live rootfs — and a changed device number would make
// tar treat every directory as new, so every incremental would carry everything.
func runTarCreate(ctx context.Context, source, archive, listing string) error {
	args := append([]string{"--create", "--gzip", "--sparse"}, tarFlags...)
	args = append(args,
		"--no-check-device",
		"--listed-incremental="+listing,
		"--file="+archive,
		"--directory="+source,
		".")
	return runTar(ctx, "producing the backup archive", args)
}

// runTarExtract applies one archive over a tree.
//
// --listed-incremental=/dev/null is not decoration: without it tar ignores the
// deletion records an incremental level carries, so a file removed inside the
// container comes back on restore and the tree does not match what was backed
// up.
func runTarExtract(ctx context.Context, target, archive string) error {
	args := append([]string{"--extract", "--gzip"}, tarFlags...)
	args = append(args,
		"--listed-incremental=/dev/null",
		"--file="+archive,
		"--directory="+target)
	return runTar(ctx, "applying the backup archive", args)
}

// runTar runs one tar invocation and keeps its stderr for the message.
func runTar(ctx context.Context, what string, args []string) error {
	binary, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGNUTar, err)
	}
	out, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		if detail := firstLine(string(out)); detail != "" {
			return fmt.Errorf("lxc: %s: %w: %s", what, err, detail)
		}
		return fmt.Errorf("lxc: %s: %w", what, err)
	}
	return nil
}

// assertGNUTar refuses a host whose tar cannot do the job.
//
// Checked up front rather than discovered from a failed invocation: bsdtar and
// busybox tar both accept --create and neither understands
// --listed-incremental, so the failure would otherwise arrive as an opaque
// usage error hours into an operator's first backup.
func assertGNUTar(ctx context.Context) error {
	binary, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGNUTar, err)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, binary, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoGNUTar, err)
	}
	if !strings.Contains(string(out), "GNU tar") {
		return fmt.Errorf("%w: this host has %s", ErrNoGNUTar, firstLine(string(out)))
	}
	return nil
}

// copyFile duplicates the incremental listing so a failed run cannot advance a
// chain's baseline.
func copyFile(source, target string) error {
	if source == "" {
		return nil
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("lxc: reading the incremental listing: %w", err)
	}
	if err := os.WriteFile(target, body, 0o600); err != nil {
		return fmt.Errorf("lxc: staging the incremental listing: %w", err)
	}
	return nil
}

// phase reports a step, tolerating a caller that wired no reporter.
func phase(report func(string, string), stage, detail string) {
	if report != nil {
		report(stage, detail)
	}
}

// firstLine trims a command's output down to something a message can carry.
func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// logBackup records a best-effort failure without a logger of the manager's
// own, which it does not have.
func (m *Manager) logBackup(message, ctID string, err error) {
	fmt.Fprintf(os.Stderr, "%s container=%s error=%v\n", message, ctID, err)
}
