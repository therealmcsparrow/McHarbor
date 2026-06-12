// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const moveTransferTTL = 1 * time.Hour

var moveTransfers = newMoveTransferStore()

type moveTransferKind string

const (
	moveTransferKindImage   moveTransferKind = "image"
	moveTransferKindArchive moveTransferKind = "archive"
)

type moveTransferEntry struct {
	ID          string
	Token       string
	Kind        moveTransferKind
	SourceEnvID string
	ImageRef    string
	ContainerID string
	SourcePath  string
	ExpiresAt   time.Time
}

type moveTransferStore struct {
	mu      sync.Mutex
	entries map[string]moveTransferEntry
}

func newMoveTransferStore() *moveTransferStore {
	return &moveTransferStore{entries: make(map[string]moveTransferEntry)}
}

func (s *moveTransferStore) create(entry moveTransferEntry) (moveTransferEntry, error) {
	token, err := randomMoveTransferToken()
	if err != nil {
		return moveTransferEntry{}, err
	}
	if strings.TrimSpace(entry.ID) == "" {
		id, err := newMoveTransferID()
		if err != nil {
			return moveTransferEntry{}, err
		}
		entry.ID = id
	}
	entry.Token = token
	entry.ExpiresAt = time.Now().UTC().Add(moveTransferTTL)
	if strings.TrimSpace(entry.SourceEnvID) == "" || strings.TrimSpace(string(entry.Kind)) == "" {
		return moveTransferEntry{}, fmt.Errorf("invalid move transfer entry")
	}
	switch entry.Kind {
	case moveTransferKindImage:
		if strings.TrimSpace(entry.ImageRef) == "" {
			return moveTransferEntry{}, fmt.Errorf("invalid move image transfer entry")
		}
	case moveTransferKindArchive:
		if strings.TrimSpace(entry.ContainerID) == "" || strings.TrimSpace(entry.SourcePath) == "" {
			return moveTransferEntry{}, fmt.Errorf("invalid move archive transfer entry")
		}
	default:
		return moveTransferEntry{}, fmt.Errorf("unsupported move transfer kind")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(time.Now().UTC())
	s.entries[entry.ID] = entry
	return entry, nil
}

func (s *moveTransferStore) consume(id, authHeader string) (moveTransferEntry, int, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return moveTransferEntry{}, http.StatusNotFound, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.cleanupExpiredLocked(now)

	entry, ok := s.entries[id]
	if !ok {
		return moveTransferEntry{}, http.StatusNotFound, false
	}
	if now.After(entry.ExpiresAt) {
		delete(s.entries, id)
		return moveTransferEntry{}, http.StatusGone, false
	}
	token := moveBearerToken(authHeader)
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(entry.Token)) != 1 {
		return moveTransferEntry{}, http.StatusUnauthorized, false
	}

	delete(s.entries, id)
	return entry, http.StatusOK, true
}

func (s *moveTransferStore) cancel(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.entries, id)
	s.mu.Unlock()
}

func (s *moveTransferStore) cleanupExpiredLocked(now time.Time) {
	for id, entry := range s.entries {
		if now.After(entry.ExpiresAt) {
			delete(s.entries, id)
		}
	}
}

func randomMoveTransferToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating move transfer token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func moveBearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func (s *Service) writeMoveTransfer(ctx context.Context, w http.ResponseWriter, entry moveTransferEntry) error {
	cli, err := s.getClient(entry.SourceEnvID)
	if err != nil {
		return err
	}

	var reader io.ReadCloser
	switch entry.Kind {
	case moveTransferKindImage:
		reader, err = cli.ImageSave(ctx, []string{entry.ImageRef})
		if err != nil {
			return fmt.Errorf("exporting move image archive: %w", err)
		}
	case moveTransferKindArchive:
		reader, _, err = cli.CopyFromContainer(ctx, entry.ContainerID, entry.SourcePath)
		if err != nil {
			return fmt.Errorf("exporting move container archive: %w", err)
		}
	default:
		return fmt.Errorf("unsupported move transfer kind")
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := copyMoveTransfer(ctx, w, reader); err != nil {
		return err
	}
	return nil
}

func copyMoveTransfer(ctx context.Context, writer io.Writer, reader io.Reader) (int64, error) {
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
