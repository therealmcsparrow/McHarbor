// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
)

// uploadBackupToSambaWithDestination writes the backup archive to
// a Samba / SMB share using the hirochachacha/go-smb2 client. The
// location row stores the server (host + port), share name,
// username, password, and the base directory inside the share.
//
// Unlike the local uploader, this path is fully usable in cluster
// mode: every McHarbor node that can reach the SMB server sees the
// same files, so container-backup archives uploaded by the leader
// are immediately visible to the followers and to any operator
// pulling archives off the share. The remote_path stored on the
// destination row is the relative path within the share, so the
// same destination can be retried from another node without
// re-computing the path.
//
// Progress updates follow the same pattern as the other
// uploaders: one call per chunk to updateRunProgress + a
// per-destination bytes counter when a destination id is
// available. The chunk size matches the OneDrive uploader's
// (8 MB) so the progress UI feels uniform across providers.
const sambaUploadChunkSize = 8 * 1024 * 1024

func (s *Service) uploadBackupToSambaWithDestination(
	ctx context.Context,
	runID, destinationID string,
	location backupStorageDestination,
	archivePath, remotePath string,
	size int64,
) (string, error) {
	cleanPath, fs, cleanup, err := s.sambaMount(ctx, location)
	if err != nil {
		return "", err
	}
	defer cleanup()

	cleanPath = cleanPath + "/" + normalizeSambaRemotePath(remotePath)
	if err := fs.MkdirAll(parentDirSamba(cleanPath), 0o755); err != nil {
		return "", fmt.Errorf("samba mkdir: %w", err)
	}

	source, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening backup archive for samba upload: %w", err)
	}
	defer source.Close()

	dst, err := fs.Create(cleanPath)
	if err != nil {
		return "", fmt.Errorf("creating samba remote file %s: %w", cleanPath, err)
	}
	// Track the cleanup so a partial write doesn't leave a
	// broken archive on the share. The operator can rerun the
	// upload (or wait for the next scheduled run) to overwrite.
	success := false
	defer func() {
		_ = dst.Close()
		if !success {
			_ = fs.Remove(cleanPath)
		}
	}()

	// Stream the archive up in chunks so a large backup doesn't
	// have to be held in memory and so the progress callback
	// fires regularly.
	buf := make([]byte, sambaUploadChunkSize)
	uploaded := int64(0)
	lastProgress := time.Now()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		n, readErr := source.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return "", fmt.Errorf("samba write %s: %w", cleanPath, writeErr)
			}
			uploaded += int64(n)
			if time.Since(lastProgress) >= 3*time.Second {
				s.updateRunProgress(ctx, runID, "uploading", backupUploadProgressMessage(location.Name, uploaded, size))
				if destinationID != "" {
					s.updateDestinationProgress(ctx, destinationID, uploaded, size)
				}
				lastProgress = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("reading backup archive for samba upload: %w", readErr)
		}
	}

	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("closing samba remote file: %w", err)
	}
	success = true

	s.updateRunProgress(ctx, runID, "uploading", backupUploadProgressMessage(location.Name, size, size))
	if destinationID != "" {
		s.updateDestinationProgress(ctx, destinationID, size, size)
	}

	if s.logger != nil {
		s.logger.Info(
			"container backup destination samba upload complete",
			"run", runID, "destination", destinationID,
			"storage", location.ID, "name", location.Name,
			"share", location.ShareName, "path", cleanPath, "size", size,
		)
	}
	return cleanPath, nil
}

// sambaMount dials the SMB server, negotiates a session, mounts
// the share, and returns the root filesystem + a cleanup function
// the caller defers. The cleanup logs out and closes the
// underlying TCP conn. Used by both the uploader and the delete
// path so the same connection lifecycle applies to both.
func (s *Service) sambaMount(
	ctx context.Context,
	location backupStorageDestination,
) (string, *smb2.Share, func(), error) {
	if strings.TrimSpace(location.ShareName) == "" {
		return "", nil, func() {}, fmt.Errorf("samba location is missing share name")
	}
	if strings.TrimSpace(location.Username) == "" {
		return "", nil, func() {}, fmt.Errorf("samba location is missing username")
	}

	host := location.Host
	port := location.Port
	if port == 0 {
		port = 445
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return "", nil, func() {}, fmt.Errorf("samba dial %s:%d: %w", host, port, err)
	}

	dial := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{User: location.Username, Password: location.Password},
	}
	type dialResult struct {
		session *smb2.Session
		err     error
	}
	dialDone := make(chan dialResult, 1)
	go func() {
		session, dialErr := dial.Dial(conn)
		dialDone <- dialResult{session: session, err: dialErr}
	}()
	var session *smb2.Session
	select {
	case <-ctx.Done():
		_ = conn.Close()
		return "", nil, func() {}, fmt.Errorf("samba handshake cancelled: %w", ctx.Err())
	case r := <-dialDone:
		if r.err != nil {
			_ = conn.Close()
			return "", nil, func() {}, fmt.Errorf("samba handshake: %w", r.err)
		}
		session = r.session.WithContext(ctx)
	}

	share := strings.TrimRight(strings.TrimSpace(location.ShareName), "/")
	if share == "" {
		_ = session.Logoff()
		return "", nil, func() {}, fmt.Errorf("samba share name is empty after normalization")
	}
	fs, err := session.Mount(share)
	if err != nil {
		_ = session.Logoff()
		return "", nil, func() {}, fmt.Errorf("samba mount share %q: %w", share, err)
	}
	cleanup := func() {
		_ = session.Logoff()
		_ = conn.Close()
	}
	return share, fs, cleanup, nil
}

// parentDirSamba returns the parent directory of a /-separated
// path. Empty for top-level files.
func parentDirSamba(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return ""
	}
	return p[:idx]
}

// normalizeSambaRemotePath strips the leading slash and resolves
// any parent-directory traversal. SMB shares are flat namespaces
// rooted at the share, so the path we hand the client must be a
// forward-slash-separated relative path with no leading slash
// and no `..` segments.
func normalizeSambaRemotePath(p string) string {
	p = strings.TrimSpace(p)
	for strings.HasPrefix(p, "/") {
		p = p[1:]
	}
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, segment := range parts {
		switch segment {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, segment)
		}
	}
	return strings.Join(out, "/")
}

// deleteSambaDestinationFile removes the backup archive at one
// Samba / SMB share. The path stored on the destination row is
// the relative path within the share (the same path the uploader
// returned); we mount the share, Remove the file, and log on
// failure. "File not found" is treated as success so a re-delete
// (for example, after a partial cleanup) doesn't fail again.
func (s *Service) deleteSambaDestinationFile(
	ctx context.Context,
	location backupStorageDestination,
	remotePath string,
	logger *slog.Logger,
) error {
	share, fs, cleanup, err := s.sambaMount(ctx, location)
	if err != nil {
		return err
	}
	defer cleanup()

	cleanPath := share + "/" + normalizeSambaRemotePath(remotePath)
	if err := fs.Remove(cleanPath); err != nil {
		if isSambaNotFound(err) {
			if logger != nil {
				logger.Info("samba destination file already gone", "share", share, "path", cleanPath)
			}
			return nil
		}
		return fmt.Errorf("samba remove %s: %w", cleanPath, err)
	}
	if logger != nil {
		logger.Info("samba destination file removed", "share", share, "path", cleanPath)
	}
	return nil
}

// isSambaNotFound reports whether an error returned by the SMB2
// client means "no such file". The exact error string varies by
// server, so we match a few common phrasings.
func isSambaNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "No such file") ||
		strings.Contains(s, "not found") ||
		strings.Contains(s, "OBJECT_NAME_NOT_FOUND") ||
		strings.Contains(s, "0xC0000034")
}

// Compile-time guards: make sure we don't accidentally drop
// imports if a refactor removes a call site.
var (
	_ = errors.New
	_ io.Reader
)
