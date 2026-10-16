// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"github.com/ulikunitz/xz"
	"golang.org/x/sys/unix"
)

// unpackTimeout bounds laying one rootfs down. Generous because a template of
// tens of gigabytes on a slow disk is legitimate, finite so a wedged filesystem
// cannot hold the create forever.
const unpackTimeout = 2 * time.Hour

// xattrPrefix is how GNU tar and Go's archive/tar carry extended attributes.
const xattrPrefix = "SCHILY.xattr."

// UnpackRootfs lays a rootfs tarball down at target with its ownership shifted
// into the container's id range. It extracts beside target and renames into
// place, so a failure leaves no partial tree behind.
func UnpackRootfs(ctx context.Context, archive, target string, shift IDShift) error {
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("lxc: opening rootfs image %s: %w", archive, err)
	}
	defer func() { errlog.Close(file) }()

	stream, release, err := decompressedStream(file)
	if err != nil {
		return err
	}
	defer release()

	return populate(ctx, target, func(ctx context.Context, staging string) error {
		return extractTar(ctx, stream, staging, shift)
	})
}

// CopyTemplateRootfs copies another container's rootfs into target, re-owning
// every file from the template's id range into this container's. The template's
// base id is read from the owner of its root directory, which is the template
// container's own root.
func CopyTemplateRootfs(ctx context.Context, template, target string, shift IDShift) error {
	if err := CheckTemplatePath(template); err != nil {
		return err
	}
	info, err := os.Lstat(template)
	if err != nil {
		return fmt.Errorf("lxc: reading template rootfs %s: %w", template, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: template %s is not a directory", ErrUnsafeArchive, template)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("lxc: template %s carries no ownership", template)
	}
	base := templateBase{uid: int64(stat.Uid), gid: int64(stat.Gid)}

	return populate(ctx, target, func(ctx context.Context, staging string) error {
		reader, writer := io.Pipe()
		done := make(chan error, 1)
		go func() {
			walkErr := writeTreeAsTar(ctx, template, base, writer)
			_ = writer.CloseWithError(walkErr)
			done <- walkErr
		}()
		extractErr := extractTar(ctx, reader, staging, shift)
		_ = reader.CloseWithError(extractErr)
		walkErr := <-done
		if extractErr != nil {
			return extractErr
		}
		return walkErr
	})
}

// populate runs fill into a private staging tree beside target and moves the
// result into place. Any failure removes the staging tree and any parent
// directory this call created, which is the "no partial rootfs" guarantee.
func populate(ctx context.Context, target string, fill func(ctx context.Context, staging string) error) error {
	ctx, cancel := context.WithTimeout(ctx, unpackTimeout)
	defer cancel()

	if err := requireAbsentOrEmpty(target); err != nil {
		return err
	}

	parent := filepath.Dir(target)
	_, statErr := os.Lstat(parent)
	parentCreated := errors.Is(statErr, fs.ErrNotExist)
	// 0755 because the container's init runs as the mapped root, which is an
	// ordinary host user and needs search permission down to its rootfs.
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("lxc: preparing %s: %w", parent, err)
	}

	suffix, err := randomSuffix()
	if err != nil {
		return err
	}
	staging := target + ".unpack-" + suffix
	if err := os.Mkdir(staging, 0o755); err != nil {
		return fmt.Errorf("lxc: creating staging tree %s: %w", staging, err)
	}

	cleanup := func() {
		errlog.Log(os.RemoveAll(staging))
		if parentCreated {
			errlog.Log(os.Remove(parent))
		}
	}

	if err := fill(ctx, staging); err != nil {
		cleanup()
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		cleanup()
		return fmt.Errorf("lxc: clearing the empty rootfs directory %s: %w", target, err)
	}
	if err := os.Rename(staging, target); err != nil {
		cleanup()
		return fmt.Errorf("lxc: moving the rootfs into place at %s: %w", target, err)
	}
	return nil
}

// requireAbsentOrEmpty refuses to populate over an existing tree, so a create
// can never merge an image into data that is already there.
func requireAbsentOrEmpty(target string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lxc: reading %s: %w", target, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("lxc: %s already exists and is not a directory", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("lxc: reading %s: %w", target, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("lxc: a rootfs already exists at %s", target)
	}
	return nil
}

func randomSuffix() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("lxc: naming the staging tree: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// decompressedStream sniffs the compression from the file's magic bytes rather
// than its name, because a cached image is named after its id, not its format.
func decompressedStream(r io.Reader) (io.Reader, func(), error) {
	buffered := bufio.NewReader(r)
	magic := errlog.Value(buffered.Peek(6))
	switch {
	case bytes.HasPrefix(magic, []byte{0x1f, 0x8b}):
		reader, err := gzip.NewReader(buffered)
		if err != nil {
			return nil, nil, fmt.Errorf("lxc: opening gzip rootfs image: %w", err)
		}
		return reader, func() { errlog.Close(reader) }, nil
	case bytes.HasPrefix(magic, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		reader, err := xz.NewReader(buffered)
		if err != nil {
			return nil, nil, fmt.Errorf("lxc: opening xz rootfs image: %w", err)
		}
		return reader, func() {}, nil
	case bytes.HasPrefix(magic, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		decoder, err := zstd.NewReader(buffered)
		if err != nil {
			return nil, nil, fmt.Errorf("lxc: opening zstd rootfs image: %w", err)
		}
		return decoder, decoder.Close, nil
	case bytes.HasPrefix(magic, []byte{'B', 'Z', 'h'}):
		return bzip2.NewReader(buffered), func() {}, nil
	default:
		return buffered, func() {}, nil
	}
}

// dirMeta is a directory's final mode and time, applied after its children are
// written so a read-only directory does not block its own contents.
type dirMeta struct {
	path  string
	mode  os.FileMode
	mtime time.Time
}

// extractTar writes every member of stream under root. Nothing is ever written
// through a symlink: each parent is checked with Lstat, the final component is
// opened with O_NOFOLLOW, and hard links may only name members already inside
// the tree.
func extractTar(ctx context.Context, stream io.Reader, root string, shift IDShift) error {
	rootUID, err := shift.UID(0)
	if err != nil {
		return err
	}
	rootGID, err := shift.GID(0)
	if err != nil {
		return err
	}
	if err := os.Lchown(root, rootUID, rootGID); err != nil {
		return fmt.Errorf("lxc: owning the rootfs root: %w", err)
	}

	reader := tar.NewReader(stream)
	var dirs []dirMeta
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("lxc: unpacking the rootfs: %w", err)
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("lxc: reading the rootfs archive: %w", err)
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		rel, err := EntryPath(header.Name)
		if err != nil {
			return err
		}
		uid, err := shift.UID(int64(header.Uid))
		if err != nil {
			return fmt.Errorf("%s: %w", header.Name, err)
		}
		gid, err := shift.GID(int64(header.Gid))
		if err != nil {
			return fmt.Errorf("%s: %w", header.Name, err)
		}
		mode := header.FileInfo().Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)

		if rel == "" {
			if header.Typeflag == tar.TypeDir {
				dirs = append(dirs, dirMeta{path: root, mode: mode, mtime: header.ModTime})
			}
			continue
		}

		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := ensureParents(root, rel, rootUID, rootGID, true); err != nil {
			return err
		}

		written, err := writeMember(reader, header, root, rel, dst)
		if err != nil {
			return err
		}
		if !written {
			continue
		}

		if err := os.Lchown(dst, uid, gid); err != nil {
			return fmt.Errorf("lxc: owning %s: %w", rel, err)
		}
		applyXattrs(dst, header.PAXRecords)
		switch header.Typeflag {
		case tar.TypeDir:
			dirs = append(dirs, dirMeta{path: dst, mode: mode, mtime: header.ModTime})
		case tar.TypeSymlink:
		default:
			if err := os.Chmod(dst, mode); err != nil {
				return fmt.Errorf("lxc: setting the mode of %s: %w", rel, err)
			}
			errlog.Log(os.Chtimes(dst, header.ModTime, header.ModTime))
		}
	}

	for index := len(dirs) - 1; index >= 0; index-- {
		dir := dirs[index]
		if err := os.Chmod(dir.path, dir.mode); err != nil {
			return fmt.Errorf("lxc: setting the mode of %s: %w", dir.path, err)
		}
		errlog.Log(os.Chtimes(dir.path, dir.mtime, dir.mtime))
	}
	return nil
}

// ensureParents refuses a parent of rel that is a symlink, which is the path an
// archive would use to write onto the host, and creates missing ones when asked.
func ensureParents(root, rel string, uid, gid int, create bool) error {
	dir := path.Dir(rel)
	if dir == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(dir, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		switch {
		case errors.Is(err, fs.ErrNotExist) && !create:
			return fmt.Errorf("%w: %s names a path that is not in the rootfs", ErrUnsafeArchive, rel)
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(current, 0o755); err != nil {
				return fmt.Errorf("lxc: creating %s: %w", current, err)
			}
			if err := os.Lchown(current, uid, gid); err != nil {
				return fmt.Errorf("lxc: owning %s: %w", current, err)
			}
		case err != nil:
			return fmt.Errorf("lxc: reading %s: %w", current, err)
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%w: %s would be written through the symlink %s", ErrUnsafeArchive, rel, current)
		case !info.IsDir():
			return fmt.Errorf("%w: %s would be written beneath the non-directory %s", ErrUnsafeArchive, rel, current)
		}
	}
	return nil
}

// writeMember creates one member at dst. It reports false for a member that is
// deliberately not created — a device node, which libcontainer provides itself.
func writeMember(reader io.Reader, header *tar.Header, root, rel, dst string) (bool, error) {
	existing, statErr := os.Lstat(dst)
	if statErr == nil {
		if header.Typeflag == tar.TypeDir && existing.IsDir() {
			return true, nil
		}
		// A later member replaces an earlier one, as tar does. RemoveAll on a
		// symlink removes the link itself, never what it points at.
		if err := os.RemoveAll(dst); err != nil {
			return false, fmt.Errorf("lxc: replacing %s: %w", rel, err)
		}
	}

	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.Mkdir(dst, 0o700); err != nil {
			return false, fmt.Errorf("lxc: creating %s: %w", rel, err)
		}
	case tar.TypeReg:
		file, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
		if err != nil {
			return false, fmt.Errorf("lxc: creating %s: %w", rel, err)
		}
		_, copyErr := io.Copy(file, reader)
		closeErr := file.Close()
		if copyErr != nil {
			return false, fmt.Errorf("lxc: writing %s: %w", rel, copyErr)
		}
		if closeErr != nil {
			return false, fmt.Errorf("lxc: writing %s: %w", rel, closeErr)
		}
	case tar.TypeSymlink:
		if SymlinkEscapes(rel, header.Linkname) {
			return false, fmt.Errorf("%w: symlink %s -> %s points outside the rootfs", ErrUnsafeArchive, rel, header.Linkname)
		}
		if err := os.Symlink(header.Linkname, dst); err != nil {
			return false, fmt.Errorf("lxc: creating symlink %s: %w", rel, err)
		}
	case tar.TypeLink:
		linkRel, err := EntryPath(header.Linkname)
		if err != nil || linkRel == "" {
			return false, fmt.Errorf("%w: hard link %s -> %s", ErrUnsafeArchive, rel, header.Linkname)
		}
		if err := ensureParents(root, linkRel, 0, 0, false); err != nil {
			return false, err
		}
		source := filepath.Join(root, filepath.FromSlash(linkRel))
		info, err := os.Lstat(source)
		if err != nil {
			return false, fmt.Errorf("%w: hard link %s names %s, which is not in the rootfs", ErrUnsafeArchive, rel, header.Linkname)
		}
		if info.IsDir() {
			return false, fmt.Errorf("%w: hard link %s names a directory", ErrUnsafeArchive, rel)
		}
		// link(2) never follows a symlink source, so this cannot reach outside.
		if err := os.Link(source, dst); err != nil {
			return false, fmt.Errorf("lxc: creating hard link %s: %w", rel, err)
		}
		// The inode already carries its owner and mode from its first name.
		return false, nil
	case tar.TypeFifo:
		if err := unix.Mkfifo(dst, 0o600); err != nil {
			return false, fmt.Errorf("lxc: creating fifo %s: %w", rel, err)
		}
	case tar.TypeChar, tar.TypeBlock:
		return false, nil
	default:
		return false, nil
	}
	return true, nil
}

// applyXattrs restores extended attributes best-effort, as GNU tar does: a
// filesystem that cannot hold one still yields a bootable rootfs.
func applyXattrs(dst string, records map[string]string) {
	for key, value := range records {
		name, ok := strings.CutPrefix(key, xattrPrefix)
		if !ok || name == "" {
			continue
		}
		errlog.Log(unix.Lsetxattr(dst, name, []byte(value), 0))
	}
}

// templateBase is the host id that owns a template's root, which is the zero
// of its id range.
type templateBase struct {
	uid int64
	gid int64
}

// writeTreeAsTar streams a directory tree as tar with ids made relative to
// base, so the extractor can re-own them into a different range. Symlinks are
// written as links and never followed; hard links are preserved.
func writeTreeAsTar(ctx context.Context, source string, base templateBase, out io.Writer) error {
	writer := tar.NewWriter(out)
	seen := map[[2]uint64]string{}

	walkErr := filepath.WalkDir(source, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(current); err != nil {
				return err
			}
		}
		if info.Mode()&fs.ModeSocket != 0 {
			return nil
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = rel
		if rel == "." {
			header.Name = "./"
		} else if info.IsDir() {
			header.Name = rel + "/"
		}
		header.Uname, header.Gname = "", ""

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("lxc: %s carries no ownership", current)
		}
		uid, gid := int64(stat.Uid)-base.uid, int64(stat.Gid)-base.gid
		if uid < 0 || gid < 0 || uid >= defaultIDCount || gid >= defaultIDCount {
			return fmt.Errorf("%w: %s is owned outside the template's id range", ErrUnsafeArchive, rel)
		}
		header.Uid, header.Gid = int(uid), int(gid)

		if info.Mode().IsRegular() && stat.Nlink > 1 {
			key := [2]uint64{uint64(stat.Dev), stat.Ino}
			if first, linked := seen[key]; linked {
				header.Typeflag = tar.TypeLink
				header.Linkname = first
				header.Size = 0
			} else {
				seen[key] = rel
			}
		}
		header.PAXRecords = readXattrs(current)

		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			return nil
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		errlog.Close(file)
		return copyErr
	})
	if walkErr != nil {
		return fmt.Errorf("lxc: reading the template rootfs: %w", walkErr)
	}
	return writer.Close()
}

// readXattrs collects a file's extended attributes in archive/tar's PAX form.
func readXattrs(file string) map[string]string {
	size, err := unix.Llistxattr(file, nil)
	if err != nil || size <= 0 {
		return nil
	}
	names := make([]byte, size)
	size, err = unix.Llistxattr(file, names)
	if err != nil || size <= 0 {
		return nil
	}
	out := map[string]string{}
	for _, name := range strings.Split(string(names[:size]), "\x00") {
		if name == "" {
			continue
		}
		valueSize, err := unix.Lgetxattr(file, name, nil)
		if err != nil || valueSize < 0 {
			continue
		}
		value := make([]byte, valueSize)
		if valueSize > 0 {
			if valueSize, err = unix.Lgetxattr(file, name, value); err != nil {
				continue
			}
		}
		out[xattrPrefix+name] = string(value[:valueSize])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
