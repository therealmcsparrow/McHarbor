// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Object-level operations against a storage location, exposed for
// workflow nodes. Every method resolves the row, decrypts its
// credentials and dispatches to the adapter that owns the location
// type — the same order container_backups and the plugin HostBridge
// use. `local` is host-owned and served from base_path directly.
//
// No credential ever leaves this package: callers pass a location id
// and an object key, nothing else.

// StorageObjectInfo is one entry returned by ListObjects.
type StorageObjectInfo struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified,omitempty"`
}

// ErrStorageLocationNotFound is returned when the id resolves to no row.
var ErrStorageLocationNotFound = errors.New("storage location not found")

// ErrStorageLocationDisabled is returned when the row exists but is disabled.
var ErrStorageLocationDisabled = errors.New("storage location is disabled")

// ErrStorageOperationUnsupported is returned when the owning adapter did
// not declare the requested capability (mirrors plugin.ErrUnsupported).
var ErrStorageOperationUnsupported = errors.New("storage operation not supported by this location type")

// ErrStorageAdapterMissing is returned for a non-local location type
// whose plugin is not installed.
var ErrStorageAdapterMissing = errors.New("no storage adapter registered for this location type")

// ErrStorageKeyEscapesBase is returned when a key would resolve outside
// a local location's base path.
var ErrStorageKeyEscapesBase = errors.New("storage key escapes the base path")

// maxStorageObjectListEntries bounds a listing so a workflow node cannot
// pull an unbounded bucket into a msg payload.
const maxStorageObjectListEntries = 1000

// maxStorageDownloadBytes bounds an in-memory download surfaced into a
// workflow msg. Larger objects belong in a backup run, not a msg.
const maxStorageDownloadBytes = 64 << 20

type pluginStorageObjectClient interface {
	Upload(ctx context.Context, config map[string]string, key string, r io.Reader) error
	Download(ctx context.Context, config map[string]string, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, config map[string]string, key string) error
	List(ctx context.Context, config map[string]string, prefix string) ([]plugin.StorageObject, error)
}

// ListObjects returns objects under prefix for the given location.
func (s *Service) ListObjects(ctx context.Context, locationID, prefix string) ([]StorageObjectInfo, error) {
	loc, cfg, err := s.resolveObjectTarget(ctx, locationID)
	if err != nil {
		return nil, err
	}
	prefix = strings.TrimSpace(prefix)

	if loc.LocationType == "local" {
		return listLocalObjects(loc, prefix)
	}
	adapter, ok := s.lookupStorageObjectAdapter(loc.LocationType)
	if !ok {
		return nil, ErrStorageAdapterMissing
	}
	if prefix == "" {
		// The contract requires a non-empty prefix; "/" means the root
		// for every adapter shipped so far.
		prefix = "/"
	}
	objects, err := adapter.List(ctx, cfg, prefix)
	if err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			return nil, ErrStorageOperationUnsupported
		}
		return nil, fmt.Errorf("storage list (%s): %w", loc.LocationType, err)
	}
	out := make([]StorageObjectInfo, 0, len(objects))
	for i, obj := range objects {
		if i >= maxStorageObjectListEntries {
			break
		}
		info := StorageObjectInfo{Key: obj.Key, Size: obj.Size}
		if !obj.LastModified.IsZero() {
			info.LastModified = obj.LastModified.UTC().Format(time.RFC3339)
		}
		out = append(out, info)
	}
	return out, nil
}

// UploadObject writes data to key under the given location.
func (s *Service) UploadObject(ctx context.Context, locationID, key string, data []byte) error {
	loc, cfg, err := s.resolveObjectTarget(ctx, locationID)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("storage key is required")
	}

	if loc.LocationType == "local" {
		target, err := localObjectPath(loc, key)
		if err != nil {
			return err
		}
		return writeLocalObject(target, data)
	}
	adapter, ok := s.lookupStorageObjectAdapter(loc.LocationType)
	if !ok {
		return ErrStorageAdapterMissing
	}
	if err := adapter.Upload(ctx, cfg, key, bytes.NewReader(data)); err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			return ErrStorageOperationUnsupported
		}
		return fmt.Errorf("storage upload (%s): %w", loc.LocationType, err)
	}
	return nil
}

// DownloadObject reads key from the given location into memory. The
// returned byte count is capped at maxStorageDownloadBytes.
func (s *Service) DownloadObject(ctx context.Context, locationID, key string) ([]byte, error) {
	loc, cfg, err := s.resolveObjectTarget(ctx, locationID)
	if err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("storage key is required")
	}

	reader, err := openStorageObject(ctx, s, loc, cfg, key)
	if err != nil {
		return nil, err
	}
	defer func() { closeutil.Report(reader.Close(), "reader") }()

	limited := io.LimitReader(reader, maxStorageDownloadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading storage object: %w", err)
	}
	if len(data) > maxStorageDownloadBytes {
		return nil, fmt.Errorf("storage object exceeds the %d byte workflow download limit", maxStorageDownloadBytes)
	}
	return data, nil
}

// DeleteObject removes key from the given location. Missing keys are a
// no-op, matching the adapter contract.
func (s *Service) DeleteObject(ctx context.Context, locationID, key string) error {
	loc, cfg, err := s.resolveObjectTarget(ctx, locationID)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("storage key is required")
	}

	if loc.LocationType == "local" {
		target, err := localObjectPath(loc, key)
		if err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing local storage object: %w", err)
		}
		return nil
	}
	adapter, ok := s.lookupStorageObjectAdapter(loc.LocationType)
	if !ok {
		return ErrStorageAdapterMissing
	}
	if err := adapter.Delete(ctx, cfg, key); err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			return ErrStorageOperationUnsupported
		}
		return fmt.Errorf("storage delete (%s): %w", loc.LocationType, err)
	}
	return nil
}

// resolveObjectTarget loads the row with decrypted credentials and
// builds the plaintext config map the adapter expects.
func (s *Service) resolveObjectTarget(ctx context.Context, locationID string) (*testLocationRow, map[string]string, error) {
	locationID = strings.TrimSpace(locationID)
	if locationID == "" {
		return nil, nil, fmt.Errorf("storage location id is required")
	}
	if s.enc == nil {
		return nil, nil, fmt.Errorf("storage credential encryption service is not configured")
	}
	loc, err := s.loadLocationForTest(ctx, locationID)
	if err != nil {
		return nil, nil, err
	}
	if loc == nil {
		return nil, nil, ErrStorageLocationNotFound
	}
	if !loc.Enabled {
		return nil, nil, ErrStorageLocationDisabled
	}
	return loc, s.buildLocationConfigForPlugin(loc), nil
}

func (s *Service) lookupStorageObjectAdapter(locationType string) (pluginStorageObjectClient, bool) {
	if s.pluginRuntime == nil {
		return nil, false
	}
	source, ok := s.pluginRuntime.(storageAdapterSource)
	if !ok {
		return nil, false
	}
	raw, _, ok := source.StorageAdapter(locationType)
	if !ok || raw == nil {
		return nil, false
	}
	client, ok := raw.(pluginStorageObjectClient)
	if !ok {
		return nil, false
	}
	return client, true
}

// localObjectPath resolves key under the location's base path and
// refuses anything that lands outside it. The key comes from a workflow
// config field, which is operator-authored but still untrusted input.
func localObjectPath(loc *testLocationRow, key string) (string, error) {
	base := strings.TrimSpace(loc.BasePath)
	if base == "" {
		return "", fmt.Errorf("storage location %s has no base path", loc.Name)
	}
	base = filepath.Clean(base)
	// UploadStream reports a local object as base_path/key, and callers hand
	// that path back to read or delete it. Joined again it pointed at a file
	// that never existed, so every delete "succeeded" and left the object.
	target := filepath.Clean(filepath.Join(base, filepath.FromSlash(key)))
	if keyPath := filepath.Clean(filepath.FromSlash(key)); filepath.IsAbs(keyPath) {
		if rel, err := filepath.Rel(base, keyPath); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			target = keyPath
		}
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrStorageKeyEscapesBase
	}
	return target, nil
}

// writeLocalObject writes atomically: temp file in the destination
// directory, renamed into place, so a sync client watching the folder
// never sees a half-written file.
func writeLocalObject(target string, data []byte) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating local storage directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".mcharbor-upload-*")
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
	if _, err := tmp.Write(data); err != nil {
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

// listLocalObjects walks base_path/prefix and returns file entries with
// keys relative to base_path (forward slashes, like object stores).
func listLocalObjects(loc *testLocationRow, prefix string) ([]StorageObjectInfo, error) {
	base := strings.TrimSpace(loc.BasePath)
	if base == "" {
		return nil, fmt.Errorf("storage location %s has no base path", loc.Name)
	}
	base = filepath.Clean(base)
	root := base
	if prefix != "" && prefix != "/" {
		resolved, err := localObjectPath(loc, prefix)
		if err != nil {
			return nil, err
		}
		root = resolved
	}

	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []StorageObjectInfo{}, nil
		}
		return nil, fmt.Errorf("reading local storage prefix: %w", err)
	}

	out := make([]StorageObjectInfo, 0, 32)
	appendEntry := func(path string, fi os.FileInfo) {
		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			return
		}
		out = append(out, StorageObjectInfo{
			Key:          filepath.ToSlash(rel),
			Size:         fi.Size(),
			LastModified: fi.ModTime().UTC().Format(time.RFC3339),
		})
	}

	if !info.IsDir() {
		appendEntry(root, info)
		return out, nil
	}

	walkErr := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries rather than failing the listing
		}
		if len(out) >= maxStorageObjectListEntries {
			return filepath.SkipDir
		}
		if fi.IsDir() {
			return nil
		}
		if strings.HasPrefix(fi.Name(), ".mcharbor-upload-") {
			return nil
		}
		appendEntry(path, fi)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking local storage prefix: %w", walkErr)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// OpenObject opens key for streaming, without the size ceiling DownloadObject
// applies.
//
// The two are deliberately different: DownloadObject serves a workflow that
// wants the bytes in hand and is capped so one cannot exhaust memory, while
// this serves a backup restore whose whole point is an artifact far larger
// than memory. The caller closes the reader.
func (s *Service) OpenObject(ctx context.Context, locationID, key string) (io.ReadCloser, error) {
	loc, cfg, err := s.resolveObjectTarget(ctx, locationID)
	if err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("storage key is required")
	}
	return openStorageObject(ctx, s, loc, cfg, key)
}

// openStorageObject resolves one key to a reader, adapter or local.
//
// Shared by DownloadObject and OpenObject so the resolution order — local
// path, then registered adapter, then a named refusal — exists once.
func openStorageObject(
	ctx context.Context,
	s *Service,
	loc *testLocationRow,
	cfg map[string]string,
	key string,
) (io.ReadCloser, error) {
	if loc.LocationType == "local" {
		target, err := localObjectPath(loc, key)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(target)
		if err != nil {
			return nil, fmt.Errorf("opening local storage object: %w", err)
		}
		return file, nil
	}

	adapter, ok := s.lookupStorageObjectAdapter(loc.LocationType)
	if !ok {
		return nil, ErrStorageAdapterMissing
	}
	reader, err := adapter.Download(ctx, cfg, key)
	if err != nil {
		if errors.Is(err, plugin.ErrUnsupported) {
			return nil, ErrStorageOperationUnsupported
		}
		return nil, fmt.Errorf("storage download (%s): %w", loc.LocationType, err)
	}
	return reader, nil
}
