// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// writeAgentFrozenMount archives one mount from the snapshot the host
// took, rather than copying it out of the running container.
//
// A mount is stored as a nested tar inside the backup archive, holding
// exactly what `docker cp` would have produced, so the restore path
// does not change. The frozen walk builds that inner tar to a temp file
// and hands it to the same writer the live path uses, which keeps a
// large mount off the heap.
func writeAgentFrozenMount(ctx context.Context, tw *agentBackupProgressWriter, entryName, frozenPath, mountDestination string) error {
	info, err := os.Stat(frozenPath)
	if err != nil {
		return fmt.Errorf("reading frozen mount %s: %w", frozenPath, err)
	}
	spool, err := os.CreateTemp("", "mcharbor-agent-frozen-*.tar")
	if err != nil {
		return fmt.Errorf("creating frozen spool file: %w", err)
	}
	spoolPath := spool.Name()
	defer func() {
		if removeErr := os.Remove(spoolPath); removeErr != nil && !os.IsNotExist(removeErr) {
			// Nothing actionable here: the archive is already written
			// and the file lands in the OS temp sweep.
			_ = removeErr
		}
	}()

	// `docker cp` names entries after the source's last path element,
	// and the restore splits on that, so the frozen walk matches it.
	prefix := filepath.Base(strings.TrimRight(mountDestination, "/"))
	if prefix == "" || prefix == "/" || prefix == "." {
		prefix = "root"
	}

	inner := tar.NewWriter(spool)
	writeErr := writeAgentFrozenTree(ctx, inner, frozenPath, prefix, info)
	if closeErr := inner.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if closeErr := spool.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return fmt.Errorf("building frozen archive for %s: %w", mountDestination, writeErr)
	}

	reader, err := os.Open(spoolPath) // #nosec G304 -- the temp file created just above
	if err != nil {
		return fmt.Errorf("reading frozen spool file: %w", err)
	}
	return writeAgentBackupStream(ctx, tw, entryName, reader)
}

func writeAgentFrozenTree(ctx context.Context, tw *tar.Writer, frozenPath, prefix string, root os.FileInfo) error {
	if !root.IsDir() {
		return writeAgentFrozenFile(tw, frozenPath, prefix, root)
	}
	return filepath.Walk(frozenPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, relErr := filepath.Rel(frozenPath, path)
		if relErr != nil {
			return relErr
		}
		name := prefix
		if relative != "." {
			name = prefix + "/" + filepath.ToSlash(relative)
		}
		return writeAgentFrozenFile(tw, path, name, info)
	})
}

// writeAgentFrozenFile writes one entry. Symlinks are stored as links
// rather than followed, so a link pointing outside the snapshot cannot
// pull unrelated host files into the archive.
func writeAgentFrozenFile(tw *tar.Writer, path, name string, info os.FileInfo) error {
	link := ""
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", path, err)
		}
		link = target
	}
	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return fmt.Errorf("building header for %s: %w", path, err)
	}
	header.Name = name
	if info.IsDir() {
		header.Name = name + "/"
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("writing header for %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(path) // #nosec G304 -- path comes from walking the snapshot the host created
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer errlog.Close(file)
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("copying %s: %w", path, err)
	}
	return nil
}
