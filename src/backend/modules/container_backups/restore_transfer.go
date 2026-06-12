// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
)

const restoreTransferTTL = 1 * time.Hour

var restoreTransfers = newRestoreTransferStore()

type restoreTransferEntry struct {
	ID        string
	Token     string
	RunID     string
	SecretKey string
	EntryName string
	Size      int64
	ExpiresAt time.Time
}

type restoreTransferStore struct {
	mu      sync.Mutex
	entries map[string]restoreTransferEntry
}

func newRestoreTransferStore() *restoreTransferStore {
	return &restoreTransferStore{entries: make(map[string]restoreTransferEntry)}
}

func (s *restoreTransferStore) create(runID, secretKey, entryName string, size int64) (restoreTransferEntry, error) {
	token, err := randomRestoreTransferToken()
	if err != nil {
		return restoreTransferEntry{}, err
	}
	entry := restoreTransferEntry{
		ID:        xid.New().String(),
		Token:     token,
		RunID:     strings.TrimSpace(runID),
		SecretKey: secretKey,
		EntryName: strings.TrimSpace(entryName),
		Size:      size,
		ExpiresAt: time.Now().UTC().Add(restoreTransferTTL),
	}
	if entry.RunID == "" || entry.EntryName == "" || entry.Size < 0 {
		return restoreTransferEntry{}, fmt.Errorf("invalid restore transfer entry")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(time.Now().UTC())
	s.entries[entry.ID] = entry
	return entry, nil
}

func (s *restoreTransferStore) consume(id, authHeader string) (restoreTransferEntry, int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return restoreTransferEntry{}, http.StatusNotFound, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.cleanupExpiredLocked(now)

	entry, ok := s.entries[id]
	if !ok {
		return restoreTransferEntry{}, http.StatusNotFound, false
	}
	if now.After(entry.ExpiresAt) {
		delete(s.entries, id)
		return restoreTransferEntry{}, http.StatusGone, false
	}
	token := bearerToken(authHeader)
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(entry.Token)) != 1 {
		return restoreTransferEntry{}, http.StatusUnauthorized, false
	}

	delete(s.entries, id)
	return entry, http.StatusOK, true
}

func (s *restoreTransferStore) cancel(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.entries, id)
	s.mu.Unlock()
}

func (s *restoreTransferStore) cleanupExpiredLocked(now time.Time) {
	for id, entry := range s.entries {
		if now.After(entry.ExpiresAt) {
			delete(s.entries, id)
		}
	}
}

func randomRestoreTransferToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating restore transfer token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func copyWithContext(ctx context.Context, writer io.Writer, reader io.Reader) (int64, error) {
	buf := make([]byte, 128*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			w, writeErr := writer.Write(buf[:n])
			written += int64(w)
			if writeErr != nil {
				return written, writeErr
			}
			if w != n {
				return written, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

func (s *Service) writeRestoreTransferEntry(ctx context.Context, w http.ResponseWriter, entry restoreTransferEntry) error {
	run, err := s.runByID(ctx, entry.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBackupRunNotDownloadable
	}
	if err != nil {
		return err
	}
	if run == nil || run.Operation != "backup" || run.Status != "success" {
		return ErrBackupRunNotDownloadable
	}

	file, decrypted, err := s.openRestoreArchive(run, entry.SecretKey)
	if err != nil {
		return err
	}
	defer file.Close()
	defer decrypted.Close()

	tr := tar.NewReader(decrypted)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return ErrBackupRunNotDownloadable
		}
		if err != nil {
			return fmt.Errorf("reading restore transfer archive: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg || header.Name != entry.EntryName {
			continue
		}
		if header.Size >= 0 && entry.Size >= 0 && header.Size != entry.Size {
			return fmt.Errorf("restore transfer entry size changed")
		}

		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", header.Size))
		w.Header().Set("Cache-Control", "no-store")
		written, err := copyWithContext(ctx, w, tr)
		if err != nil {
			return fmt.Errorf("streaming restore transfer entry: %w", err)
		}
		if header.Size >= 0 && written != header.Size {
			return fmt.Errorf("restore transfer size mismatch")
		}
		return nil
	}
}
