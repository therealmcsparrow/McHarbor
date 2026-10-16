// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The shared image library.
//
// A catalog entry names a mirror and each host fetches from it. That shape has
// nothing to offer an image nobody publishes — a golden qcow2, a vendor
// appliance, an installer ISO — so those get a directory instead: McHarbor
// writes the upload into it, and every agent bind-mounts the same directory
// onto its own image cache root, where the file is then simply already present.
//
// No bytes cross the agent transport, which is the same reason catalog
// downloads never did: a multi-gigabyte disk image would compete with console
// traffic for nothing, when both ends can see one filesystem.

// libraryDirEnv overrides where McHarbor writes the library. It exists because
// this must be the *same* directory the agents mount, and only the operator
// knows where that is on their machine.
const libraryDirEnv = "MCHARBOR_COMPUTE_IMAGE_LIBRARY"

// agentLibraryMount is the path every agent mounts the library on: the image
// cache root compiled into the agent (`compute_runtime_linux.go`). It is shown
// to the operator rather than configured, because the agent side is a constant.
const agentLibraryMount = "/var/lib/mcharbor/compute/images"

// maxUploadBytes bounds one upload at 64 GiB — larger than any real disk image
// and small enough that a runaway client cannot quietly fill a disk.
const maxUploadBytes int64 = 64 << 30

// libraryExtensions are the extensions an uploaded file may carry.
//
// Not a format check: it is what the agent's cache path is allowed to end in,
// and the extension is what QEMU reads a disk's format from. Anything else
// would be stored without one and probed instead — safe, but slower and less
// predictable, so it is refused at the door.
var libraryExtensions = map[string]bool{
	".qcow2": true, ".img": true, ".iso": true, ".raw": true,
	".vmdk": true, ".vhd": true, ".vhdx": true,
	".gz": true, ".xz": true, ".zst": true, ".tar": true, ".bz2": true,
}

// Errors the upload path raises, mapped onto i18n codes by the handler.
var (
	ErrLibraryUnavailable = errors.New("the image library directory is not writable")
	ErrUploadEmpty        = errors.New("no file was uploaded")
	ErrUploadTooLarge     = errors.New("the uploaded file exceeds the size limit")
	ErrUploadExtension    = errors.New("unsupported image file extension")
)

// Library reports where uploads are stored, so the UI can name the directory
// an operator's hosts have to mount.
type Library struct {
	// Path is the directory as McHarbor sees it.
	Path string `json:"path"`
	// AgentMount is where each agent expects to find it. The two differ
	// whenever McHarbor runs in a container, which is the normal case.
	AgentMount string `json:"agentMount"`
	Writable   bool   `json:"writable"`
	// LastError says why it is not writable. Safe to show: this is McHarbor's
	// own directory, not a remote host's filesystem.
	LastError    string `json:"lastError,omitempty"`
	MaxSizeBytes int64  `json:"maxSizeBytes"`
	UsedBytes    int64  `json:"usedBytes"`
	FileCount    int    `json:"fileCount"`
}

// libraryRoot resolves the library directory, preferring the operator's
// override over the plugin's subtree of the host data directory.
func (s *Service) libraryRoot() string {
	if override := strings.TrimSpace(os.Getenv(libraryDirEnv)); override != "" {
		return filepath.Clean(override)
	}
	return filepath.Join(s.workDir, "compute", "images")
}

// ensureLibrary creates the directory on first use.
//
// 0o700 matches the agent's own cache root: these are boot volumes, and
// anything that can rewrite one owns every guest started from it.
func (s *Service) ensureLibrary() (string, error) {
	root := s.libraryRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrLibraryUnavailable, root, err)
	}
	return root, nil
}

// LibraryInfo describes the library for the images page.
func (s *Service) LibraryInfo() Library {
	info := Library{
		Path:         s.libraryRoot(),
		AgentMount:   agentLibraryMount,
		MaxSizeBytes: maxUploadBytes,
	}

	root, err := s.ensureLibrary()
	if err != nil {
		info.LastError = err.Error()
		return info
	}
	info.Writable = true

	entries, err := os.ReadDir(root)
	if err != nil {
		info.Writable = false
		info.LastError = err.Error()
		return info
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".partial-") {
			continue
		}
		stat, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		info.FileCount++
		info.UsedBytes += stat.Size()
	}
	return info
}

// libraryFileName is the name an image takes inside the library.
//
// It is the name the agent's own cache path computes for the same id, so an
// uploaded file lands exactly where the agent will later look for it. The
// operator's filename survives only as the display name — using it here would
// let one upload overwrite another.
func libraryFileName(imageID, originalName string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filepath.Base(originalName)))
	if !libraryExtensions[ext] {
		return "", fmt.Errorf("%w: %q", ErrUploadExtension, ext)
	}
	return imageID + ext, nil
}

// receiveUpload streams one upload into the library, hashing as it goes.
//
// The hash is taken here, from the bytes McHarbor actually received, and never
// from the client: it is what the agent re-checks before adopting the file, so
// a checksum the uploader supplied would be verifying itself.
func (s *Service) receiveUpload(ctx context.Context, imageID, fileName string, src io.Reader) (string, int64, string, error) {
	root, err := s.ensureLibrary()
	if err != nil {
		return "", 0, "", err
	}

	name, err := libraryFileName(imageID, fileName)
	if err != nil {
		return "", 0, "", err
	}

	// Written under a partial name and renamed only once complete, so an
	// interrupted upload is never picked up as a boot disk. The agent's cache
	// skips `.partial-` for that same reason, and across the mount this is
	// that same directory.
	tmp, err := os.CreateTemp(root, ".partial-*")
	if err != nil {
		return "", 0, "", fmt.Errorf("%w: %v", ErrLibraryUnavailable, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	hasher := sha256.New()
	written, err := copyLibraryBytes(ctx, io.MultiWriter(tmp, hasher), src)
	if err != nil {
		return "", 0, "", err
	}
	if written == 0 {
		return "", 0, "", ErrUploadEmpty
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, "", fmt.Errorf("%w: %v", ErrLibraryUnavailable, err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, "", fmt.Errorf("%w: %v", ErrLibraryUnavailable, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return "", 0, "", fmt.Errorf("%w: %v", ErrLibraryUnavailable, err)
	}
	if err := os.Rename(tmpName, filepath.Join(root, name)); err != nil {
		return "", 0, "", fmt.Errorf("%w: %v", ErrLibraryUnavailable, err)
	}
	committed = true

	return name, written, hex.EncodeToString(hasher.Sum(nil)), nil
}

// copyLibraryBytes streams src into dst under the size ceiling, honouring
// cancellation so a browser that goes away does not leave a goroutine writing
// gigabytes to disk.
func copyLibraryBytes(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 1<<20)
	var written int64

	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if written+int64(n) > maxUploadBytes {
				return written, ErrUploadTooLarge
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return written, fmt.Errorf("%w: %v", ErrLibraryUnavailable, err)
			}
			written += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, fmt.Errorf("reading upload: %w", readErr)
		}
	}
}

// libraryFilePath resolves one uploaded image's file inside the library.
//
// filepath.Base defends the library against a stored name carrying a path:
// the name reaches here from a database column, and a row that says
// "../../etc/shadow" must resolve inside the library or nowhere.
func (s *Service) libraryFilePath(name string) string {
	if name == "" {
		return ""
	}
	return filepath.Join(s.libraryRoot(), filepath.Base(name))
}

// removeLibraryFile deletes an uploaded image's bytes.
//
// Called only when the catalog entry goes: an upload has no mirror to be
// downloaded from again, so the row and the file live and die together.
func (s *Service) removeLibraryFile(name string) {
	if name == "" {
		return
	}
	// Defends the directory against a stored name carrying a path, which
	// nothing writes today and a future migration might.
	if name != filepath.Base(name) {
		s.logger.Warn("compute: refusing to remove a library name with a path in it", "name", name)
		return
	}
	if err := os.Remove(filepath.Join(s.libraryRoot(), name)); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("compute: removing library file failed", "name", name, "error", err)
	}
}
