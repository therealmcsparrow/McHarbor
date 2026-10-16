// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"io"
	"log/slog"
	"path"
	"strings"
)

// filteredTarReader wraps a Docker tar stream so only the entries matching
// paths reach the archive.
//
// The same tarPathMatch / filterTarSubset pair the restore side uses is
// reused verbatim, so "choose /config" means precisely the same set of
// entries whether it is being captured or applied. Filtering happens in a
// goroutine feeding an io.Pipe rather than through a temp file because
// writeStream already owns spooling and sizing — adding a second spool
// here would double the disk traffic on a large export for no gain.
//
// A selection that matches nothing yields an empty tar and a warning
// rather than an error. An emptied directory is a legitimate state, and
// failing a nightly run over it would be worse than recording the truth:
// the run's item list shows what was actually captured.
func filteredTarReader(
	src io.ReadCloser,
	paths []string,
	label string,
	logger *slog.Logger,
) io.ReadCloser {
	reader, writer := io.Pipe()
	// goroutine lifecycle: ends when the tar stream is exhausted, or earlier
	// when the consumer closes the read half — every write then fails and the
	// deferred src.Close() runs. No path leaves it parked on the pipe.
	go func() {
		defer func() {
			// The Docker stream must be drained/closed even when the
			// consumer stops early, or the client connection leaks.
			closeLogged(src, "container tar stream")
		}()
		tw := tar.NewWriter(writer)
		matched, err := filterTarSubset(tw, tar.NewReader(src), paths)
		if err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		if matched == 0 && logger != nil {
			logger.Warn(
				"container backup: path selection matched no entries",
				"entry", label, "paths", paths,
			)
		}
		if err := tw.Close(); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		_ = writer.CloseWithError(nil)
	}()
	return reader
}

// Selections are stored and shown as absolute container paths, because
// that is what the user picked in the file browser and what reads
// sensibly back out of the database. Tar entry names are not absolute:
// ContainerExport roots at "/" without the leading slash, and Docker
// names a CopyFromContainer archive after the basename of the copied
// path, so /var/www/html arrives as "html/...".
//
// The two helpers below translate at capture time, which keeps the API,
// the UI, and the stored plan speaking one obvious language and confines
// the tar-relative form to the one place that needs it.

// captureFilesystemPaths maps absolute container paths onto the entry
// names of a ContainerExport tar: /etc/nginx becomes "etc/nginx".
func captureFilesystemPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, entry := range paths {
		clean := normalizeTarPath(entry)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	return out
}

// captureMountPaths maps absolute container paths onto the entry names of
// the CopyFromContainer tar for one mount. With target /var/www/html,
// /var/www/html/config becomes "html/config", and the mount root itself
// becomes "html".
//
// A path that is not inside the mount is dropped rather than reinterpreted
// — a selection that has drifted must narrow to nothing, never widen to
// something else.
func captureMountPaths(target string, paths []string) []string {
	cleanTarget := "/" + normalizeTarPath(target)
	root := path.Base(cleanTarget)
	if root == "" || root == "." || root == "/" {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, entry := range paths {
		clean := "/" + normalizeTarPath(entry)
		if clean == cleanTarget {
			out = append(out, root)
			continue
		}
		if !strings.HasPrefix(clean, cleanTarget+"/") {
			continue
		}
		inner := strings.TrimPrefix(clean, cleanTarget+"/")
		out = append(out, path.Join(root, inner))
	}
	return out
}
