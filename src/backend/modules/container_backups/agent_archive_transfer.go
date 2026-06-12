// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"
)

const agentArchiveTransferTTL = 2 * time.Hour

var agentArchiveTransfers = newAgentArchiveTransferStore()

type agentArchiveTransferEntry struct {
	ID         string
	Token      string
	RunID      string
	TargetPath string
	SourcePath string
	ExpiresAt  time.Time
}

type agentArchiveTransferStore struct {
	mu      sync.Mutex
	entries map[string]agentArchiveTransferEntry
}

func newAgentArchiveTransferStore() *agentArchiveTransferStore {
	return &agentArchiveTransferStore{entries: make(map[string]agentArchiveTransferEntry)}
}

func (s *agentArchiveTransferStore) createUpload(runID, targetPath string) (agentArchiveTransferEntry, error) {
	token, err := randomRestoreTransferToken()
	if err != nil {
		return agentArchiveTransferEntry{}, err
	}
	entry := agentArchiveTransferEntry{
		ID:         xid.New().String(),
		Token:      token,
		RunID:      strings.TrimSpace(runID),
		TargetPath: strings.TrimSpace(targetPath),
		ExpiresAt:  time.Now().UTC().Add(agentArchiveTransferTTL),
	}
	if entry.RunID == "" || entry.TargetPath == "" {
		return agentArchiveTransferEntry{}, fmt.Errorf("invalid agent archive upload transfer")
	}
	s.store(entry)
	return entry, nil
}

func (s *agentArchiveTransferStore) createDownload(runID, sourcePath string) (agentArchiveTransferEntry, error) {
	token, err := randomRestoreTransferToken()
	if err != nil {
		return agentArchiveTransferEntry{}, err
	}
	entry := agentArchiveTransferEntry{
		ID:         xid.New().String(),
		Token:      token,
		RunID:      strings.TrimSpace(runID),
		SourcePath: strings.TrimSpace(sourcePath),
		ExpiresAt:  time.Now().UTC().Add(agentArchiveTransferTTL),
	}
	if entry.RunID == "" || entry.SourcePath == "" {
		return agentArchiveTransferEntry{}, fmt.Errorf("invalid agent archive download transfer")
	}
	s.store(entry)
	return entry, nil
}

func (s *agentArchiveTransferStore) store(entry agentArchiveTransferEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(time.Now().UTC())
	s.entries[entry.ID] = entry
}

func (s *agentArchiveTransferStore) consume(id, authHeader string) (agentArchiveTransferEntry, int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return agentArchiveTransferEntry{}, http.StatusNotFound, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.cleanupExpiredLocked(now)

	entry, ok := s.entries[id]
	if !ok {
		return agentArchiveTransferEntry{}, http.StatusNotFound, false
	}
	if now.After(entry.ExpiresAt) {
		delete(s.entries, id)
		return agentArchiveTransferEntry{}, http.StatusGone, false
	}
	token := bearerToken(authHeader)
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(entry.Token)) != 1 {
		return agentArchiveTransferEntry{}, http.StatusUnauthorized, false
	}

	delete(s.entries, id)
	return entry, http.StatusOK, true
}

func (s *agentArchiveTransferStore) cancel(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.entries, id)
	s.mu.Unlock()
}

func (s *agentArchiveTransferStore) cleanupExpiredLocked(now time.Time) {
	for id, entry := range s.entries {
		if now.After(entry.ExpiresAt) {
			delete(s.entries, id)
		}
	}
}

func (s *Service) receiveAgentArchive(ctx context.Context, entry agentArchiveTransferEntry, reader io.Reader) error {
	targetPath, err := cleanLocalBackupPath(entry.TargetPath)
	if err != nil {
		return err
	}
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return fmt.Errorf("creating local backup destination directory: %w", err)
	}
	tmp, err := os.CreateTemp(targetDir, ".mcharbor-agent-backup-*")
	if err != nil {
		return fmt.Errorf("creating agent backup temp file: %w", err)
	}
	tmpName := tmp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := copyWithContext(ctx, tmp, reader); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("receiving agent backup archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing agent backup temp file: %w", err)
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("moving agent backup archive into place: %w", err)
	}
	keepTemp = true
	return nil
}

func (s *Service) streamAgentArchive(ctx context.Context, writer io.Writer, sourcePath string) error {
	archivePath, err := s.validatedArchivePath(sourcePath)
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening agent restore archive: %w", err)
	}
	defer file.Close()
	_, err = copyWithContext(ctx, writer, file)
	return err
}
