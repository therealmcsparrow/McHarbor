// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// ErrNoBackupEncryptionKey means an encrypted upload was asked for on a host
// with no master key installed.
//
// Refused rather than written in the clear: a backup that silently stops
// being encrypted is the one failure nobody notices until the archive is
// already somewhere it should not be readable.
var ErrNoBackupEncryptionKey = errors.New("storage stream: no backup encryption key is installed on this host")

// UploadStream writes a reader to key under the given location.
//
// The body is read exactly once and never retried here. A stream has no
// second read, so a failure is returned to the caller, which owns the source
// and is the only side that can decide whether to produce the bytes again.
func (b *HostBridge) UploadStream(
	ctx context.Context,
	pluginSlug string,
	req pluginruntime.StorageStreamDispatch,
) (result pluginruntime.StorageStreamDispatchResult, err error) {
	var loc *StorageLocation
	var written int64
	defer func() {
		b.auditUpload(ctx, pluginSlug, req.LocationID, req.Key, int(written), loc, err)
	}()

	loc, err = b.service.ByID(ctx, req.LocationID)
	if err != nil {
		return result, fmt.Errorf("resolving storage location: %w", err)
	}
	if loc == nil {
		return result, fmt.Errorf("storage location %s not found", req.LocationID)
	}
	if !loc.Enabled {
		return result, fmt.Errorf("storage location %s is disabled", loc.Name)
	}

	body := req.Body
	if body == nil {
		return result, errors.New("storage stream: no body")
	}

	// Counting sits below the encryption wrap on purpose: what the operator
	// wants to see move is the guest's own bytes, not the envelope's
	// per-chunk overhead.
	counted := &countingReader{inner: body, progress: req.Progress}
	body = counted

	if req.Encrypt {
		if b.app == nil || b.app.BackupCrypto == nil {
			return result, ErrNoBackupEncryptionKey
		}
		encrypted, done := encryptStream(b.app.BackupCrypto, body)
		defer func() {
			// The encrypting goroutine's error wins over a nil return: an
			// adapter that read a truncated stream may well report success.
			if encErr := done(); encErr != nil && err == nil {
				err = encErr
			}
		}()
		body = encrypted
		result.KeyID = b.app.BackupCrypto.KeyID()
	}

	if adapter, ok := b.resolveAdapter(loc.LocationType); ok {
		cfg, cfgErr := b.buildConfig(ctx, loc)
		if cfgErr != nil {
			return result, cfgErr
		}
		if upErr := adapter.Upload(ctx, cfg, req.Key, body); upErr != nil {
			return result, fmt.Errorf("storage stream (%s): %w", loc.LocationType, upErr)
		}
	} else if loc.LocationType == "local" {
		if localErr := streamLocal(loc, req.Key, body); localErr != nil {
			return result, localErr
		}
	} else {
		return result, fmt.Errorf(
			"no storage adapter registered for %s — install the matching plugin", loc.LocationType)
	}

	written = counted.count.Load()
	result.BytesWritten = written
	result.RemotePath = remotePathFor(loc, req.Key)
	return result, nil
}

// countingReader reports progress as the source is drained.
type countingReader struct {
	inner    io.Reader
	count    atomic.Int64
	progress func(int64)
}

// Read passes bytes through and reports the running total.
func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		total := r.count.Add(int64(n))
		if r.progress != nil {
			r.progress(total)
		}
	}
	return n, err
}

// encryptStream turns the write-shaped envelope into a reader the adapters
// can consume.
//
// backupcrypto seals into an io.Writer and every storage adapter wants an
// io.Reader, so a pipe bridges them and the sealing runs in its own
// goroutine. The returned function blocks until that goroutine has finished
// and reports what it hit — without it, a failure mid-stream would surface
// only as a short upload the adapter happily accepted.
func encryptStream(crypto *backupcrypto.Service, src io.Reader) (io.Reader, func() error) {
	reader, writer := io.Pipe()
	errCh := make(chan error, 1)

	// goroutine lifecycle: it ends when src is drained or a write fails. The
	// returned wait function closes the reader first, so an adapter that stopped
	// reading turns the next pipe write into an error instead of a block; the
	// buffered errCh is then drained by that same function.
	go func() {
		sealed, _, err := crypto.EncryptWriter(writer)
		if err != nil {
			_ = writer.CloseWithError(err)
			errCh <- err
			return
		}
		if _, err := io.Copy(sealed, src); err != nil {
			closeutil.Report(sealed.Close(), "sealed")
			_ = writer.CloseWithError(err)
			errCh <- err
			return
		}
		// Close flushes the final chunk. Closing the pipe before it would
		// truncate the envelope and produce an archive that fails to
		// decrypt with no explanation.
		if err := sealed.Close(); err != nil {
			_ = writer.CloseWithError(err)
			errCh <- err
			return
		}
		errCh <- writer.Close()
	}()

	return reader, func() error {
		// A drained stream has already finished, so this close changes nothing
		// for it. An abandoned one would otherwise hold the upload open forever.
		_ = reader.CloseWithError(errStreamAbandoned) //nolint:errcheck // PipeReader.CloseWithError always returns nil
		return <-errCh
	}
}

// errStreamAbandoned is what the sealing goroutine sees when the adapter
// returned before reading the whole envelope, which also exposes a short read
// the adapter reported as a successful upload.
var errStreamAbandoned = errors.New("storage stream: the destination stopped reading before the end of the stream")

// streamLocal writes a stream to the host filesystem under the location's
// base path.
//
// Containment and atomicity are handled exactly as uploadLocal does them:
// the key comes from a plugin, so the resolved path is checked against the
// base, and the file is written to a temp name and renamed so nothing
// watching the directory opens a half-written artifact.
func streamLocal(loc *StorageLocation, key string, body io.Reader) error {
	base := strings.TrimSpace(loc.BasePath)
	if base == "" {
		return fmt.Errorf("storage location %s has no base path", loc.Name)
	}
	base = filepath.Clean(base)

	target := filepath.Clean(filepath.Join(base, filepath.FromSlash(key)))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("storage key escapes the base path of location %s", loc.Name)
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating local storage directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".mcharbor-stream-*")
	if err != nil {
		return fmt.Errorf("creating local storage temp file: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			closeutil.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, body); err != nil {
		closeutil.Report(tmp.Close(), "tmp")
		return fmt.Errorf("writing local storage file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing local storage file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return fmt.Errorf("setting local storage file mode: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("finalising local storage file: %w", err)
	}
	committed = true
	return nil
}

// remotePathFor is what the run record shows the operator.
//
// For a local destination it is the real path; for an adapter it is the key
// under the location, because only the adapter knows how its provider spells
// a full address.
func remotePathFor(loc *StorageLocation, key string) string {
	if loc.LocationType == "local" && strings.TrimSpace(loc.BasePath) != "" {
		return filepath.Join(filepath.Clean(loc.BasePath), filepath.FromSlash(key))
	}
	return key
}

// DownloadStream opens a stored object, decrypting it in flight when asked.
//
// The reader is handed to the caller unbuffered: this interface exists
// precisely so a guest-sized artifact never has to be held in memory, and
// materialising it here would defeat that at the last step.
func (b *HostBridge) DownloadStream(
	ctx context.Context,
	pluginSlug string,
	req pluginruntime.StorageDownloadDispatch,
) (io.ReadCloser, error) {
	// Delegates rather than resolving again: the adapter-or-local order and
	// its refusals already exist on the service, and a second copy here would
	// be one more place for them to disagree.
	body, err := b.service.OpenObject(ctx, req.LocationID, req.Key)
	if err != nil {
		return nil, err
	}

	if !req.Decrypt {
		return body, nil
	}
	if b.app == nil || b.app.BackupCrypto == nil {
		closeutil.Report(body.Close(), "body")
		return nil, ErrNoBackupEncryptionKey
	}

	plain, _, err := b.app.BackupCrypto.DecryptReader(body)
	if err != nil {
		closeutil.Report(body.Close(), "body")
		// Names the likely cause rather than the library's wording: the
		// common reason an envelope will not open is that this McHarbor holds
		// a different master key than the one that sealed it.
		return nil, fmt.Errorf("opening the encrypted object — this McHarbor may hold a different backup key than the one it was written with: %w", err)
	}
	return &closePair{Reader: plain, also: body}, nil
}

// DeleteObject removes a stored object.
//
// An object that is already gone is a success. A retention pass that ran
// twice, or an operator who tidied a destination by hand, must not leave the
// sweep behind it failing forever on a key nobody will ever see again.
func (b *HostBridge) DeleteObject(ctx context.Context, pluginSlug, locationID, key string) error {
	if err := b.service.DeleteObject(ctx, locationID, key); err != nil {
		return fmt.Errorf("removing the stored object: %w", err)
	}
	return nil
}

// closePair closes the decrypting reader and the object underneath it.
//
// DecryptReader wraps the source but does not own it, so closing only the
// outer reader would leak the adapter's connection or the open file.
type closePair struct {
	io.Reader
	also io.Closer
}

// Close closes both layers, reporting the first failure.
func (c *closePair) Close() error {
	var firstErr error
	if closer, ok := c.Reader.(io.Closer); ok {
		firstErr = closer.Close()
	}
	if err := c.also.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
