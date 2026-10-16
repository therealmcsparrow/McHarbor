// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/lxc"
	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// maxInlineFileBytes is the largest file the browser will read or write over
// the WebSocket transport.
//
// A pool holds disk images, so the common large object here is measured in
// gigabytes and must never sit in one frame: the transport has a single writer
// and a multi-gigabyte message would wedge it for every other guest on the
// host. Anything above this is a download, not a read.
const maxInlineFileBytes = 2 << 20

// maxPoolDirEntries bounds one listing. A pool root with a runaway process
// writing into it can hold far more entries than a browser can render, and the
// agent is the only side that can refuse to build the slice at all.
const maxPoolDirEntries = 5000

// errPoolEscape is returned for any path that resolves outside its pool.
//
// It is deliberately one error for every way out — absolute path, "..",
// symlink, bind mount — so no caller can learn which of them it found.
var errPoolEscape = errors.New("storage: path is outside the pool")

// poolRoot resolves the directory a pool is browsed through, with its symlinks
// already followed, so every later comparison is against a real path.
func (rt *computeRuntime) poolRoot(ctx context.Context, req *ComputePoolPayload) (string, error) {
	driver, pool, err := rt.resolvePool(req)
	if err != nil {
		return "", err
	}
	root, err := driver.Root(ctx, pool)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("storage: pool root %q is not absolute", root)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("storage: resolve pool root: %w", err)
	}
	return resolved, nil
}

// resolveInPool turns a pool-relative request path into an absolute one that is
// proven to be inside root.
//
// mustExist separates a read from a create: a file being written does not exist
// yet, so its parent is what gets resolved and the leaf is appended afterwards.
// Resolving the leaf in that case would either fail or, worse, follow a symlink
// an attacker planted at exactly that name.
func resolveInPool(root, rel string, mustExist bool) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", errPoolEscape
	}
	// path.Clean on an absolute form collapses ".." before it can be joined, so
	// "../../etc" and "/etc" both land at "etc" and are then judged on where
	// they actually resolve rather than on how they were spelled.
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(rel)), "/")
	if clean == "." {
		clean = ""
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errPoolEscape
	}

	target := filepath.Join(root, filepath.FromSlash(clean))
	if mustExist {
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			if os.IsNotExist(err) {
				return "", storage.ErrNotFound
			}
			return "", err
		}
		if !withinRoot(root, resolved) {
			return "", errPoolEscape
		}
		return resolved, nil
	}

	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		if os.IsNotExist(err) {
			return "", storage.ErrNotFound
		}
		return "", err
	}
	if !withinRoot(root, realParent) {
		return "", errPoolEscape
	}
	leaf := filepath.Base(target)
	if leaf == "." || leaf == string(filepath.Separator) {
		return "", errPoolEscape
	}
	return filepath.Join(realParent, leaf), nil
}

// withinRoot reports whether candidate is root or sits beneath it. The
// separator on the prefix is what stops "/pool-backup" matching "/pool".
func withinRoot(root, candidate string) bool {
	if candidate == root {
		return true
	}
	return strings.HasPrefix(candidate, root+string(filepath.Separator))
}

// validateLeafName rejects a name that would traverse rather than name.
func validateLeafName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("storage: %q is not a usable name", name)
	}
	if strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0) {
		return errors.New("storage: a name cannot contain a path separator")
	}
	return nil
}

// parseFileMode reads an octal permission string, falling back to def when it
// is empty. Only the permission bits are honoured: setuid on a root agent is
// not something a file browser hands out.
func parseFileMode(mode string, def os.FileMode) (os.FileMode, error) {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return def, nil
	}
	parsed, err := strconv.ParseUint(trimmed, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("storage: %q is not an octal mode", mode)
	}
	return os.FileMode(parsed) & os.ModePerm, nil
}

// handlePoolBrowse lists one directory inside a pool.
func (rt *computeRuntime) handlePoolBrowse(ctx context.Context, msg WSMessage) {
	req := msg.Pool
	if req == nil {
		rt.replyPoolFiles(msg, MsgPoolBrowseResponse, nil, fmt.Errorf("compute: %q carried no pool payload", msg.Type))
		return
	}

	root, err := rt.poolRoot(ctx, req)
	if err != nil {
		rt.replyPoolFiles(msg, MsgPoolBrowseResponse, nil, err)
		return
	}
	dir, err := resolveInPool(root, req.Path, true)
	if err != nil {
		rt.replyPoolFiles(msg, MsgPoolBrowseResponse, nil, err)
		return
	}
	info, err := os.Stat(dir)
	if err != nil {
		rt.replyPoolFiles(msg, MsgPoolBrowseResponse, nil, err)
		return
	}
	if !info.IsDir() {
		rt.replyPoolFiles(msg, MsgPoolBrowseResponse, nil, fmt.Errorf("storage: %q is not a directory", req.Path))
		return
	}

	names, err := os.ReadDir(dir)
	if err != nil {
		rt.replyPoolFiles(msg, MsgPoolBrowseResponse, nil, err)
		return
	}

	base := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(req.Path)), "/")
	if base == "." {
		base = ""
	}

	entries := make([]ComputePoolFile, 0, len(names))
	for _, entry := range names {
		if len(entries) >= maxPoolDirEntries {
			break
		}
		// Lstat rather than entry.Info(): a symlink must be reported as a
		// symlink, with its own metadata, not as whatever it points at.
		full := filepath.Join(dir, entry.Name())
		stat, err := os.Lstat(full)
		if err != nil {
			// A file removed between the readdir and the stat is normal on a
			// live pool and is not worth failing the whole listing over.
			continue
		}
		entries = append(entries, poolFileEntry(base, stat, full))
	}

	// Directories first, then by name: a pool root holds a handful of
	// directories among hundreds of disk images, and alphabetical order alone
	// buries them.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	rt.replyPoolFiles(msg, MsgPoolBrowseResponse, entries, nil)
}

// poolFileEntry converts one stat into its protocol shape.
func poolFileEntry(base string, stat os.FileInfo, full string) ComputePoolFile {
	entry := ComputePoolFile{
		Name:       stat.Name(),
		Path:       path.Join(base, stat.Name()),
		IsDir:      stat.IsDir(),
		SizeBytes:  stat.Size(),
		Mode:       fmt.Sprintf("%04o", stat.Mode().Perm()),
		ModifiedAt: stat.ModTime().UTC().Format(time.RFC3339),
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		entry.IsSymlink = true
		if dest, err := os.Readlink(full); err == nil {
			entry.LinkTarget = dest
		}
	}
	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		entry.OwnerUID = int(sys.Uid)
		entry.OwnerGID = int(sys.Gid)
	}
	return entry
}

// handlePoolFile performs one mutating or reading file operation in a pool.
func (rt *computeRuntime) handlePoolFile(ctx context.Context, msg WSMessage) {
	req := msg.Pool
	if req == nil {
		rt.replyPoolFiles(msg, MsgPoolFileResponse, nil, fmt.Errorf("compute: %q carried no pool payload", msg.Type))
		return
	}

	root, err := rt.poolRoot(ctx, req)
	if err != nil {
		rt.replyPoolFiles(msg, MsgPoolFileResponse, nil, err)
		return
	}

	var content []byte
	switch req.Operation {
	case "read":
		content, err = poolReadFile(root, req)
	case "write":
		err = poolWriteFile(root, req)
	case "mkdir":
		err = poolMkdir(root, req)
	case "delete":
		err = poolDelete(root, req)
	case "rename":
		err = poolRename(root, req)
	default:
		err = fmt.Errorf("storage: %q is not a pool file operation", req.Operation)
	}
	if err != nil {
		rt.replyPoolFiles(msg, MsgPoolFileResponse, nil, err)
		return
	}

	errlog.Log(rt.send(WSMessage{
		Type: MsgPoolFileResponse,
		ID:   msg.ID,
		Pool: &ComputePoolPayload{
			Operation: req.Operation,
			PoolID:    req.PoolID,
			Path:      req.Path,
			Content:   content,
			Success:   true,
		},
	}))
}

// poolReadFile returns a whole small file. Anything larger is refused with a
// message naming the limit, because silently truncating a config file the
// operator is about to edit would write the truncation back.
func poolReadFile(root string, req *ComputePoolPayload) ([]byte, error) {
	target, err := resolveInPool(root, req.Path, true)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("storage: %q is a directory", req.Path)
	}
	if stat.Size() > maxInlineFileBytes {
		return nil, fmt.Errorf("storage: %q is %d bytes, over the %d byte inline limit", req.Path, stat.Size(), maxInlineFileBytes)
	}
	return os.ReadFile(target)
}

// poolWriteFile replaces a file's contents.
//
// The write goes to a temporary file in the same directory and is renamed over
// the target, so a failure partway leaves the original intact rather than a
// half-written cloud-init seed or disk image.
func poolWriteFile(root string, req *ComputePoolPayload) error {
	if len(req.Content) > maxInlineFileBytes {
		return fmt.Errorf("storage: content is over the %d byte inline limit", maxInlineFileBytes)
	}
	target, err := resolveInPool(root, req.Path, false)
	if err != nil {
		return err
	}
	mode, err := parseFileMode(req.Mode, 0o644)
	if err != nil {
		return err
	}
	if existing, statErr := os.Stat(target); statErr == nil {
		if existing.IsDir() {
			return fmt.Errorf("storage: %q is a directory", req.Path)
		}
		if strings.TrimSpace(req.Mode) == "" {
			mode = existing.Mode().Perm()
		}
	}
	owner, inRootfs, err := rootfsOwnerFor(root, target)
	if err != nil {
		return err
	}
	uid, gid := owner.UID, owner.GID
	// Lstat: the rename replaces a symlink at target rather than its target, so
	// the entry being replaced is the one whose owner may be kept.
	if replaced, lstatErr := os.Lstat(target); inRootfs && lstatErr == nil {
		uid, gid = keptOwner(owner, replaced)
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), ".mcharbor-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best effort: the rename below consumes it on the success path, so
		// this only removes something when the write already failed.
		errlog.Log(os.Remove(tmpName))
	}()

	if _, err := tmp.Write(req.Content); err != nil {
		errlog.Close(tmp)
		return err
	}
	if err := tmp.Sync(); err != nil {
		errlog.Close(tmp)
		return err
	}
	// Before the chmod, because chown clears setuid and setgid. Fchown on the
	// descriptor this call created cannot be redirected through a symlink.
	if inRootfs {
		if err := tmp.Chown(uid, gid); err != nil {
			errlog.Close(tmp)
			return fmt.Errorf("storage: give %q to the container's owner: %w", req.Path, err)
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

// poolMkdir creates one directory. Parents are not created: a typo in a deep
// path would otherwise build the whole mistaken tree.
func poolMkdir(root string, req *ComputePoolPayload) error {
	target, err := resolveInPool(root, req.Path, false)
	if err != nil {
		return err
	}
	mode, err := parseFileMode(req.Mode, 0o755)
	if err != nil {
		return err
	}
	owner, inRootfs, err := rootfsOwnerFor(root, target)
	if err != nil {
		return err
	}
	if err := os.Mkdir(target, mode); err != nil {
		if os.IsExist(err) {
			return storage.ErrExists
		}
		return err
	}
	if !inRootfs {
		return nil
	}
	// Lchown, so a symlink swapped in after the mkdir is re-owned itself and
	// its target is never touched.
	if err := os.Lchown(target, owner.UID, owner.GID); err != nil {
		errlog.Log(os.Remove(target))
		return fmt.Errorf("storage: give %q to the container's owner: %w", req.Path, err)
	}
	return nil
}

// rootfsOwnerFor returns who a new entry at target must belong to when target
// lies inside a system container's rootfs in this pool, and false elsewhere.
//
// A host-root file inside an unprivileged rootfs shows as nobody to the guest
// and makes every later backup of it unrestorable into another id range.
func rootfsOwnerFor(root, target string) (lxc.RootfsOwner, bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return lxc.RootfsOwner{}, false, nil //nolint:nilerr // a target outside root cannot be inside a rootfs, so no owner applies rather than an error
	}
	ctID, rootfsRel, ok := lxc.RootfsOf(filepath.ToSlash(rel))
	if !ok {
		return lxc.RootfsOwner{}, false, nil
	}
	rootfs := filepath.Join(root, filepath.FromSlash(rootfsRel))
	owner, err := lxc.RootfsOwnerFor(rootfs, lxc.PathsFor(ctID).ConfigJSON)
	if err != nil {
		return lxc.RootfsOwner{}, false, fmt.Errorf("storage: read the owner of container %s's rootfs: %w", ctID, err)
	}
	return owner, true, nil
}

// keptOwner is the owner an overwrite leaves on a file: its current one when
// the container can see it, else the container's root, which repairs a file an
// older agent wrote as host root.
func keptOwner(owner lxc.RootfsOwner, existing os.FileInfo) (int, int) {
	stat, ok := existing.Sys().(*syscall.Stat_t)
	if !ok || !owner.Owns(int64(stat.Uid), int64(stat.Gid)) {
		return owner.UID, owner.GID
	}
	return int(stat.Uid), int(stat.Gid)
}

// poolDelete removes a file, or a directory when Recursive is set.
func poolDelete(root string, req *ComputePoolPayload) error {
	target, err := resolveInPool(root, req.Path, true)
	if err != nil {
		return err
	}
	if target == root {
		return errors.New("storage: refusing to delete the pool root")
	}
	stat, err := os.Lstat(target)
	if err != nil {
		return err
	}
	// A guest keeps a deleted disk alive only until it exits, so a delete that
	// succeeds here would destroy the disk later and silently.
	holders, err := holdersUnder(linuxProc{root: "/proc"}, target)
	if err != nil {
		return err
	}
	if len(holders) > 0 {
		return inUseError(req.Path, holders)
	}
	if stat.IsDir() && req.Recursive {
		return os.RemoveAll(target)
	}
	return os.Remove(target)
}

// poolRename moves an entry within its own directory. NewName is a leaf, so
// there is no second path to confine.
func poolRename(root string, req *ComputePoolPayload) error {
	if err := validateLeafName(req.NewName); err != nil {
		return err
	}
	target, err := resolveInPool(root, req.Path, true)
	if err != nil {
		return err
	}
	if target == root {
		return errors.New("storage: refusing to rename the pool root")
	}
	dest := filepath.Join(filepath.Dir(target), req.NewName)
	if !withinRoot(root, dest) {
		return errPoolEscape
	}
	if _, err := os.Lstat(dest); err == nil {
		return storage.ErrExists
	}
	return os.Rename(target, dest)
}

// replyPoolFiles sends one pool-file reply, carrying the error as text when
// there is one. The agent is the side that knows what actually failed; the host
// turns this into an i18n code before any of it reaches a browser.
func (rt *computeRuntime) replyPoolFiles(msg WSMessage, replyType string, entries []ComputePoolFile, err error) {
	reply := &ComputePoolPayload{Entries: entries}
	if msg.Pool != nil {
		reply.Operation = msg.Pool.Operation
		reply.PoolID = msg.Pool.PoolID
		reply.Path = msg.Pool.Path
	}
	if err != nil {
		reply.Error = err.Error()
	} else {
		reply.Success = true
	}
	errlog.Log(rt.send(WSMessage{Type: replyType, ID: msg.ID, Pool: reply}))
}
