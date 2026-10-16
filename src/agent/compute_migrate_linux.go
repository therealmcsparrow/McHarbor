// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/vm"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Moving compute state between locations.
//
// Changing a root decides where the next guest is written and nothing else:
// the files already on the host stay, and every row naming them keeps pointing
// at the old path. This file is the other half. It moves what is there, reports
// progress while it does, and hands back every path it changed so the server
// and each plugin can rewrite the rows they own.
//
// Two mechanisms, chosen per item: a rename when both sides are on one
// filesystem, which is instant and atomic, and a copy-verify-unlink otherwise.
// The original is removed only after the copy is on disk and its size matches,
// because the alternative is a half-copied disk image that looks like a disk
// image.

// migrateProgressInterval bounds how often a move reports itself. A directory
// of ten thousand small files would otherwise spend the migration sending
// progress messages.
const migrateProgressInterval = 2 * time.Second

// migrateCopyBuffer is the copy block size. Disk images are the common case
// here, so it is sized for throughput rather than for small files.
const migrateCopyBuffer = 4 << 20

// handleComputeMigrate answers compute_migrate_request.
//
// A plan run answers with what it found and touches nothing. An apply run moves
// everything it can and reports the failures rather than stopping at the first
// one, because a migration that aborts half way leaves the host in a state
// nobody has a record of.
func (rt *computeRuntime) handleComputeMigrate(ctx context.Context, a *Agent, msg WSMessage) {
	reply := &ComputePayload{Operation: msg.Type}
	if msg.Compute != nil {
		reply.EnvID = msg.Compute.EnvID
	}

	result, err := rt.migrate(ctx, a, msg)
	if err != nil {
		reply.Error = err.Error()
	} else {
		reply.Success = true
		reply.Migrate = result
	}
	roots := effectiveRoots(rt)
	reply.Roots = &roots
	errlog.Log(rt.send(WSMessage{Type: MsgComputeMigrateResponse, ID: msg.ID, Compute: reply}))
}

// migrate plans or performs one relocation.
func (rt *computeRuntime) migrate(ctx context.Context, a *Agent, msg WSMessage) (*ComputeMigrate, error) {
	if msg.Compute == nil || msg.Compute.Migrate == nil {
		return nil, errors.New("compute: compute_migrate_request carried no request")
	}
	req := *msg.Compute.Migrate
	have := effectiveRoots(rt)

	jobs, err := rt.migrationJobs(req, have)
	if err != nil {
		return nil, err
	}

	if req.Repair {
		return rt.repairLinks(ctx, req, have)
	}

	out := &ComputeMigrate{}
	for i := range jobs {
		jobs[i].bytes, jobs[i].entries = treeFootprint(jobs[i].from)
		out.Entries += jobs[i].entries
		out.Bytes += jobs[i].bytes
	}
	if req.Plan {
		return out, nil
	}

	// A guest whose disk is being moved out from under it is a corrupted
	// guest. The server stops them first and says so; this is the check that
	// makes that promise enforceable rather than advisory.
	if busy, err := rt.runningVMs(ctx); err != nil {
		return nil, err
	} else if len(busy) > 0 {
		return nil, fmt.Errorf("compute: cannot move compute state while %d guest(s) are running: %s",
			len(busy), strings.Join(busy, ", "))
	}

	reporter := rt.newMigrateReporter(msg.Compute.EnvID, out.Bytes)
	for _, job := range jobs {
		moved, failures := rt.runMigration(job, reporter)
		out.Moved = append(out.Moved, moved...)
		out.Failures = append(out.Failures, failures...)
	}
	reporter.finish()

	// A moved qcow2 still names its backing file by the path it had before,
	// and a manifest still names every frozen disk the same way. Repairing is
	// part of moving: a guest that will not open is not a completed move.
	if repaired, err := rt.repairLinks(ctx, req, have); err == nil && repaired != nil {
		out.Moved = append(out.Moved, repaired.Moved...)
		out.Failures = append(out.Failures, repaired.Failures...)
	}

	// The roots follow the data, so a restart comes back to where the files
	// actually are rather than to where they used to be.
	if err := rt.adoptRoots(a, req.Roots); err != nil {
		return out, err
	}
	out.Completed = true
	return out, nil
}

// repairLinks fixes what a move leaves behind inside the files themselves.
//
// Moving a qcow2 does not move what is written in it: an overlay names its
// backing file by absolute path, and a snapshot manifest names every frozen
// disk the same way. Both still point at the directory the data left, so the
// guest that was fine before the move will not open at all after it — which is
// exactly what happened to a host whose disks moved to a new array.
//
// Nothing is copied here. A backing reference whose file is missing, but whose
// basename exists in the directory the image now lives in, is rewritten to
// point there.
func (rt *computeRuntime) repairLinks(ctx context.Context, req ComputeMigrate, have ComputeRoots) (*ComputeMigrate, error) {
	out := &ComputeMigrate{Completed: true}

	roots := []string{req.DiskTo, req.Roots.VMRoot, have.VMRoot}
	seen := map[string]bool{}
	for _, root := range roots {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		fixed, failures, note := rt.repairChainsIn(ctx, root)
		out.Moved = append(out.Moved, fixed...)
		out.Failures = append(out.Failures, failures...)
		out.Notes = append(out.Notes, note)
	}

	vmRoot := firstNonEmpty(req.Roots.VMRoot, have.VMRoot)
	manifests := rt.repairManifestsIn(vmRoot, req.DiskTo)
	out.Moved = append(out.Moved, manifests...)
	out.Notes = append(out.Notes, fmt.Sprintf("%s: %d guest directory(ies), %d manifest(s) repaired",
		vmRoot, countDirs(vmRoot), len(manifests)))
	out.Entries = len(out.Moved)
	return out, nil
}

// repairChainsIn rewrites the dangling backing reference of every image in one
// directory.
func (rt *computeRuntime) repairChainsIn(ctx context.Context, dir string) ([]ComputeMove, []ComputeFailed, string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Sprintf("%s: cannot be read (%v)", dir, err)
	}
	var fixed []ComputeMove
	var failures []ComputeFailed
	images, backed := 0, 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		images++
		backing, format := imageBacking(ctx, path)
		if backing == "" {
			continue
		}
		backed++
		if _, err := os.Stat(backing); err == nil {
			if filepath.IsAbs(backing) && filepath.Dir(backing) == dir {
				// It resolves, but only from here. Rewriting it as the
				// sibling's name is what makes the directory movable at all.
				if rt.rebaseTo(ctx, path, filepath.Base(backing), format) {
					fixed = append(fixed, ComputeMove{
						Kind: "relative", From: backing, To: filepath.Base(backing),
					})
				}
			}
			continue
		}
		candidate := filepath.Join(dir, filepath.Base(backing))
		if _, err := os.Stat(candidate); err != nil {
			failures = append(failures, ComputeFailed{
				Path:   path,
				Reason: "its backing file " + backing + " is missing and no file of that name is here",
			})
			continue
		}
		// The sibling's name, not its absolute path: a repaired chain should
		// also be a movable one.
		if !rt.rebaseTo(ctx, path, filepath.Base(candidate), format) {
			failures = append(failures, ComputeFailed{
				Path: path, Reason: "rebasing onto " + candidate + " failed",
			})
			continue
		}
		fixed = append(fixed, ComputeMove{Kind: "backing", From: backing, To: candidate})
	}
	return fixed, failures, fmt.Sprintf(
		"%s: %d file(s), %d with a backing file, %d repaired",
		dir, images, backed, len(fixed))
}

// rebaseTo rewrites an image's backing reference and copies nothing.
//
// -u is the whole point: the data is already correct, only the path written
// inside the image is wrong.
func (rt *computeRuntime) rebaseTo(ctx context.Context, image, backing, format string) bool {
	args := []string{"rebase", "-u", "-b", backing, "-F", defaultString(format, "qcow2"), image}
	if out, err := exec.CommandContext(ctx, "qemu-img", args...).CombinedOutput(); err != nil {
		rt.logger.Warn("compute: rebasing an image failed",
			"image", image, "backing", backing, "error", strings.TrimSpace(string(out)))
		return false
	}
	rt.logger.Info("compute: repointed a backing file", "image", image, "backing", backing)
	return true
}

// repairManifestsIn rewrites every path in every snapshot manifest whose file
// now lives somewhere else.
func (rt *computeRuntime) repairManifestsIn(vmRoot, diskRoot string) []ComputeMove {
	if vmRoot == "" || diskRoot == "" {
		return nil
	}
	var fixed []ComputeMove
	guests, err := os.ReadDir(vmRoot)
	if err != nil {
		return nil
	}
	for _, guest := range guests {
		if !guest.IsDir() {
			continue
		}
		dir := filepath.Join(vmRoot, guest.Name(), snapshotDirName)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			manifest, err := readManifest(guest.Name(), strings.TrimSuffix(file.Name(), ".json"))
			if err != nil {
				continue
			}
			changed := false
			for i := range manifest.Disks {
				for _, field := range []*string{
					&manifest.Disks[i].FrozenPath,
					&manifest.Disks[i].ActivePath,
					&manifest.Disks[i].Handle,
				} {
					moved, ok := relocatedInto(*field, diskRoot)
					if !ok {
						continue
					}
					*field = moved
					changed = true
				}
			}
			if !changed {
				continue
			}
			if err := writeManifest(manifest); err != nil {
				rt.logger.Warn("compute: repairing a snapshot manifest failed",
					"file", path, "error", err)
				continue
			}
			fixed = append(fixed, ComputeMove{Kind: "manifest", From: path, To: path})
		}
	}
	return fixed
}

// countDirs is how many guest directories a root holds, which is the fastest
// way to say "this location is empty" in a report.
func countDirs(root string) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return -1
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

// relocatedInto reports where a missing file now is, when a file of that name
// is in root.
func relocatedInto(path, root string) (string, bool) {
	if path == "" || filepath.Dir(path) == root {
		return "", false
	}
	if _, err := os.Stat(path); err == nil {
		return "", false
	}
	candidate := filepath.Join(root, filepath.Base(path))
	if _, err := os.Stat(candidate); err != nil {
		return "", false
	}
	return candidate, true
}

// imageBacking reads an image's backing file and format.
func imageBacking(ctx context.Context, path string) (string, string) {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// -U (force-share) because a running guest holds its disk's write lock, and
	// the images-in-use check has to read exactly those disks.
	out, err := exec.CommandContext(probeCtx, "qemu-img", "info", "-U", "--output=json", path).Output()
	if err != nil {
		return "", ""
	}
	var info struct {
		BackingFile       string `json:"backing-filename"`
		BackingFileFormat string `json:"backing-filename-format"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", ""
	}
	return info.BackingFile, info.BackingFileFormat
}

// defaultString is the first value that is set.
func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// migrationJob is one directory or file to relocate.
type migrationJob struct {
	kind    string
	from    string
	to      string
	guestID string
	slot    int
	// tree is true for a directory move, false for a single file.
	tree    bool
	bytes   int64
	entries int
}

// migrationJobs turns a request into the list of moves it implies.
//
// The roots are moved wholesale — everything under the old VM root belongs to
// McHarbor by construction — while volumes are named one by one, because a
// pool's target may hold files this agent did not put there.
func (rt *computeRuntime) migrationJobs(req ComputeMigrate, have ComputeRoots) ([]migrationJob, error) {
	jobs := make([]migrationJob, 0, 4)

	if req.Roots != nil {
		// The server's own record of where the data was wins over what this
		// host currently reports: the roots may already have been repointed at
		// the empty destination.
		roots := []struct {
			kind string
			from string
			to   string
		}{
			{"vm", firstNonEmpty(req.VMFrom, have.VMRoot), req.Roots.VMRoot},
			{"ct", firstNonEmpty(req.CTFrom, have.CTRoot), req.Roots.CTRoot},
			{"image", firstNonEmpty(req.ImageFrom, have.ImageRoot), req.Roots.ImageRoot},
		}
		for _, root := range roots {
			if root.to == "" || root.to == root.from || root.from == "" {
				continue
			}
			if err := validateRoot(root.kind, root.to); err != nil {
				return nil, err
			}
			jobs = append(jobs, migrationJob{kind: root.kind, from: root.from, to: root.to, tree: true})
		}
	}

	if req.DiskTo != "" && req.DiskFrom != "" && req.DiskTo != req.DiskFrom {
		if err := validateRoot("disk", req.DiskTo); err != nil {
			return nil, err
		}
		jobs = append(jobs, migrationJob{kind: "disk", from: req.DiskFrom, to: req.DiskTo, tree: true})
	}

	for _, volume := range req.Volumes {
		if volume.From == "" || volume.To == "" || volume.From == volume.To {
			continue
		}
		jobs = append(jobs, migrationJob{
			kind: "volume", from: volume.From, to: volume.To,
			guestID: volume.GuestID, slot: volume.Slot,
		})
	}
	return jobs, nil
}

// runMigration performs one job and reports what moved.
func (rt *computeRuntime) runMigration(job migrationJob, reporter *migrateReporter) ([]ComputeMove, []ComputeFailed) {
	if !job.tree {
		if err := moveFile(job.from, job.to, reporter); err != nil {
			return nil, []ComputeFailed{{Path: job.from, Reason: err.Error()}}
		}
		move := ComputeMove{
			Kind: job.kind, From: job.from, To: job.to,
			GuestID: job.guestID, Slot: job.slot, Bytes: job.bytes,
		}
		rt.announceVolumeMove(move)
		return []ComputeMove{move}, nil
	}

	entries, err := os.ReadDir(job.from)
	if err != nil {
		return nil, []ComputeFailed{{Path: job.from, Reason: err.Error()}}
	}
	if err := os.MkdirAll(job.to, 0o700); err != nil {
		return nil, []ComputeFailed{{Path: job.to, Reason: err.Error()}}
	}

	moved := make([]ComputeMove, 0, len(entries))
	var failures []ComputeFailed
	for _, entry := range entries {
		from := filepath.Join(job.from, entry.Name())
		to := filepath.Join(job.to, entry.Name())
		reporter.item(from)

		var moveErr error
		if entry.IsDir() {
			moveErr = moveTree(from, to, reporter)
		} else {
			moveErr = moveFile(from, to, reporter)
		}
		if moveErr != nil {
			failures = append(failures, ComputeFailed{Path: from, Reason: moveErr.Error()})
			continue
		}
		move := ComputeMove{Kind: job.kind, From: from, To: to}
		switch job.kind {
		case "disk":
			if entry.IsDir() {
				// The guest's own directory: every disk in it moved with it,
				// and the directory's name is the guest.
				rt.announceGuestDirMove(entry.Name(), from, to)
				break
			}
			move.GuestID, move.Slot = guestFromVolumeName(entry.Name())
			rt.announceVolumeMove(move)
		case "image":
			// An ISO in the image root is attached to guests as a CD-ROM, so a
			// disk row names it. Announced by path because the file belongs to
			// no single guest — several may have it mounted.
			rt.announcePathMove(move)
		}
		moved = append(moved, move)
	}

	if job.kind == "vm" {
		rt.repointSnapshotManifests(job.from, job.to)
	}
	return moved, failures
}

// firstNonEmpty returns the first value that was actually set.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// guestFromVolumeName recovers the owner of a volume from its filename.
//
// volumeNameFor writes `<guestId>-<slot>`, which is the only link between a
// file in a pool and the guest whose disk it is — the agent holds no table of
// its own, and the server's is in a plugin the mover cannot read.
func guestFromVolumeName(name string) (string, int) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	cut := strings.LastIndex(base, "-")
	if cut <= 0 {
		return "", 0
	}
	slot, err := strconv.Atoi(base[cut+1:])
	if err != nil {
		return "", 0
	}
	return base[:cut], slot
}

// announceVolumeMove tells the server a guest's disk changed path.
//
// Sent as a guest event because that is the one channel every plugin already
// subscribes to: the plugin that owns the disk row rewrites it, and nothing
// else has to know the relocation happened.
func (rt *computeRuntime) announceVolumeMove(move ComputeMove) {
	if move.GuestID == "" {
		return
	}
	errlog.Log(rt.send(WSMessage{Type: MsgVMEvent, VM: &VMPayload{
		VMID: move.GuestID,
		Event: &VMEvent{
			VMID: move.GuestID,
			Kind: "volume_moved",
			Detail: map[string]string{
				"from": move.From,
				"to":   move.To,
				"slot": strconv.Itoa(move.Slot),
			},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}}))
}

// announceGuestDirMove reports every disk inside a guest directory that moved.
func (rt *computeRuntime) announceGuestDirMove(guestID, from, to string) {
	entries, err := os.ReadDir(to)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		_, slot, ok := strings.Cut(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), "disk-")
		if !ok {
			continue
		}
		number, err := strconv.Atoi(slot)
		if err != nil {
			continue
		}
		rt.announceVolumeMove(ComputeMove{
			Kind: "disk", GuestID: guestID, Slot: number,
			From: filepath.Join(from, entry.Name()),
			To:   filepath.Join(to, entry.Name()),
		})
	}
}

// announcePathMove tells the server a file moved, without naming an owner.
//
// The disk row that points at it is found by path: an image can be attached to
// any number of guests, and the mover has no table saying which.
func (rt *computeRuntime) announcePathMove(move ComputeMove) {
	errlog.Log(rt.send(WSMessage{Type: MsgVMEvent, VM: &VMPayload{
		Event: &VMEvent{
			Kind: "path_moved",
			Detail: map[string]string{
				"from": move.From,
				"to":   move.To,
			},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}}))
}

// repointSnapshotManifests rewrites the paths inside every manifest a moved
// guest carries.
//
// A manifest records absolute paths to frozen disks and memory state. Moving
// the directory without rewriting them leaves a revert pointing at files that
// are no longer there — the failure would surface hours later, on the one
// operation the operator needed to work.
func (rt *computeRuntime) repointSnapshotManifests(oldRoot, newRoot string) {
	guests, err := os.ReadDir(newRoot)
	if err != nil {
		return
	}
	for _, guest := range guests {
		if !guest.IsDir() {
			continue
		}
		dir := filepath.Join(newRoot, guest.Name(), snapshotDirName)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			rewritten := strings.ReplaceAll(string(data), oldRoot, newRoot)
			if rewritten == string(data) {
				continue
			}
			var manifest snapshotManifest
			if err := json.Unmarshal([]byte(rewritten), &manifest); err != nil {
				rt.logger.Warn("compute: a snapshot manifest could not be repointed after a move",
					"file", path, "error", err)
				continue
			}
			if err := os.WriteFile(path, []byte(rewritten), 0o600); err != nil {
				rt.logger.Warn("compute: rewriting a snapshot manifest after a move failed",
					"file", path, "error", err)
			}
		}
	}
}

// adoptRoots points the runtime at the locations the data now lives in.
func (rt *computeRuntime) adoptRoots(a *Agent, roots *ComputeRoots) error {
	if roots == nil {
		return nil
	}
	if roots.VMRoot != "" {
		vm.SetRuntimeRoot(roots.VMRoot)
	}
	if roots.CTRoot != "" {
		lxc.SetRuntimeRoot(roots.CTRoot)
	}
	if roots.ImageRoot != "" {
		if err := rt.images.SetRoot(roots.ImageRoot); err != nil {
			return err
		}
	}
	return a.saveComputeRoots(effectiveRoots(rt))
}

// treeFootprint totals what a path holds, for the plan and the progress bar.
func treeFootprint(root string) (int64, int) {
	var bytes int64
	entries := 0
	errlog.Log(filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable entries are skipped: the footprint is an estimate
		}
		if entry.IsDir() {
			return nil
		}
		entries++
		if info, statErr := entry.Info(); statErr == nil {
			bytes += info.Size()
		}
		return nil
	}))
	return bytes, entries
}

// moveTree relocates a directory, recursing only when it has to.
//
// The rename is tried first for the whole tree: on one filesystem it moves a
// 200 GB guest instantly, and falling back per file would copy every byte for
// nothing.
func moveTree(from, to string, reporter *migrateReporter) error {
	if err := os.Rename(from, to); err == nil {
		reporter.renamed(from)
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		// Not a cross-device rename: the destination may already exist, which
		// the per-entry walk below resolves.
		if _, statErr := os.Stat(to); statErr != nil {
			return err
		}
	}

	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, info.Mode().Perm()); err != nil {
		return err
	}
	for _, entry := range entries {
		childFrom := filepath.Join(from, entry.Name())
		childTo := filepath.Join(to, entry.Name())
		if entry.IsDir() {
			if err := moveTree(childFrom, childTo, reporter); err != nil {
				return err
			}
			continue
		}
		if err := moveFile(childFrom, childTo, reporter); err != nil {
			return err
		}
	}
	return os.Remove(from)
}

// moveFile relocates one file, verifying the copy before the original goes.
func moveFile(from, to string, reporter *migrateReporter) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	if err := os.Rename(from, to); err == nil {
		reporter.renamed(from)
		return nil
	}

	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { errlog.Close(source) }()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	target, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}

	copied, copyErr := io.CopyBuffer(&progressWriter{to: target, reporter: reporter},
		source, make([]byte, migrateCopyBuffer))
	if copyErr == nil {
		copyErr = target.Sync()
	}
	closeErr := target.Close()
	if copyErr != nil {
		errlog.Log(os.Remove(to))
		return copyErr
	}
	if closeErr != nil {
		errlog.Log(os.Remove(to))
		return closeErr
	}
	if copied != info.Size() {
		errlog.Log(os.Remove(to))
		return fmt.Errorf("compute: %s copied %d of %d bytes", from, copied, info.Size())
	}
	return os.Remove(from)
}

// progressWriter counts bytes as they land, so a single 200 GB image still
// moves the bar.
type progressWriter struct {
	to       io.Writer
	reporter *migrateReporter
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.to.Write(p)
	w.reporter.bytes(int64(n))
	return n, err
}

// migrateReporter throttles progress back to McHarbor.
type migrateReporter struct {
	rt       *computeRuntime
	envID    string
	total    int64
	done     int64
	current  string
	lastSent time.Time
}

func (rt *computeRuntime) newMigrateReporter(envID string, total int64) *migrateReporter {
	return &migrateReporter{rt: rt, envID: envID, total: total}
}

// item records which path the move has reached.
func (r *migrateReporter) item(path string) {
	r.current = path
	r.send(false)
}

// renamed credits a whole tree at once: a rename moves every byte in it without
// any of them passing through this process.
func (r *migrateReporter) renamed(path string) {
	bytes, _ := treeFootprint(path)
	if bytes == 0 {
		if info, err := os.Stat(path); err == nil {
			bytes = info.Size()
		}
	}
	r.done += bytes
	r.send(false)
}

// bytes credits a copy in flight.
func (r *migrateReporter) bytes(n int64) {
	r.done += n
	r.send(false)
}

// finish reports the last state whatever the throttle says.
func (r *migrateReporter) finish() { r.send(true) }

func (r *migrateReporter) send(force bool) {
	if !force && time.Since(r.lastSent) < migrateProgressInterval {
		return
	}
	r.lastSent = time.Now()
	percent := 100
	if r.total > 0 {
		percent = int(float64(r.done) / float64(r.total) * 100)
		if percent > 100 {
			percent = 100
		}
	}
	errlog.Log(r.rt.send(WSMessage{Type: MsgComputeMigrateProgress, Compute: &ComputePayload{
		Operation: MsgComputeMigrateRequest,
		EnvID:     r.envID,
		Migrate:   &ComputeMigrate{Bytes: r.done, Entries: percent},
	}}))
}
