// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"
)

// Package store kinds. Local is today's behaviour: the package exists only in this
// node's DATA_DIR, which is fine for one node and fatal for a standby.
const (
	StoreLocal    = "local"
	StoreDatabase = "database"
	StoreSharedFS = "shared-fs"
	StoreObject   = "object"
)

// chunkBytes keeps each base64 chunk under MySQL's 64 KiB TEXT limit, so the same
// schema holds a package on all four backends without a BLOB type.
const chunkBytes = 32 << 10

// ErrPackageMissing means the store has no copy of that version.
var ErrPackageMissing = errors.New("plugins: package not in the store")

// StoredPackage is a verified tarball as the store keeps it; Signature is the
// base64 ed25519 signature that verified it at install, empty for unsigned installs.
type StoredPackage struct {
	Slug      string
	Version   string
	Build     string
	Signature string
	Tarball   []byte
}

// PackageStore keeps installed plugin packages somewhere every node can reach, so a
// standby that never ran the install can still load the plugin when it is promoted.
type PackageStore interface {
	Kind() string
	Put(ctx context.Context, p StoredPackage) error
	Get(ctx context.Context, slug, version string) (StoredPackage, error)
	Delete(ctx context.Context, slug, version string) error
}

// ObjectOperator is the slice of the storage-location runtime the object store uses.
type ObjectOperator interface {
	UploadObject(ctx context.Context, locationID, key string, data []byte) error
	DownloadObject(ctx context.Context, locationID, key string) ([]byte, error)
	DeleteObject(ctx context.Context, locationID, key string) error
}

var slugVersionRE = regexp.MustCompile(`^[a-z0-9_][a-z0-9-]{0,62}[a-z0-9]$|^[0-9][0-9A-Za-z.+-]{0,63}$`)

func objectKey(slug, version string) string {
	return "mcharbor-plugins/" + slug + "@" + version + ".tar.gz"
}

// StoreSetting is MCHARBOR_PLUGIN_STORE parsed: the kind plus, for shared-fs, the
// mounted path and, for object, the storage location id.
type StoreSetting struct {
	Kind     string
	Path     string
	Location string
}

// ParseStoreSetting reads "database", "shared-fs:/mnt/plugins" or
// "object:<storage-location-id>"; empty means local.
func ParseStoreSetting(v string) (StoreSetting, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || v == StoreLocal:
		return StoreSetting{Kind: StoreLocal}, nil
	case v == StoreDatabase:
		return StoreSetting{Kind: StoreDatabase}, nil
	case strings.HasPrefix(v, StoreSharedFS+":"):
		path := strings.TrimPrefix(v, StoreSharedFS+":")
		if !filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
			return StoreSetting{}, fmt.Errorf("plugins: %s needs an absolute path", StoreSharedFS)
		}
		return StoreSetting{Kind: StoreSharedFS, Path: path}, nil
	case strings.HasPrefix(v, StoreObject+":"):
		loc := strings.TrimPrefix(v, StoreObject+":")
		if loc == "" {
			return StoreSetting{}, fmt.Errorf("plugins: %s needs a storage location id", StoreObject)
		}
		return StoreSetting{Kind: StoreObject, Location: loc}, nil
	}
	return StoreSetting{}, fmt.Errorf("plugins: unknown MCHARBOR_PLUGIN_STORE %q", v)
}

// dbStore keeps packages in the shared database as base64 chunks.
type dbStore struct{ db *sql.DB }

// NewDatabaseStore returns the store every external-database deployment can use
// without any extra infrastructure.
func NewDatabaseStore(db *sql.DB) PackageStore { return &dbStore{db: db} }

func (s *dbStore) Kind() string { return StoreDatabase }

func (s *dbStore) Put(ctx context.Context, p StoredPackage) error {
	sum := sha256.Sum256(p.Tarball)
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("plugins: storing package: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Warn("plugins: rolling back package store failed", "error", err)
		}
	}()
	var oldID string
	switch err := tx.QueryRowContext(ctx, `SELECT id FROM plugin_packages WHERE slug = ? AND version = ?`, p.Slug, p.Version).Scan(&oldID); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fmt.Errorf("plugins: storing package: %w", err)
	default:
		if _, err := tx.ExecContext(ctx, `DELETE FROM plugin_package_chunks WHERE package_id = ?`, oldID); err != nil {
			return fmt.Errorf("plugins: replacing package: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM plugin_packages WHERE id = ?`, oldID); err != nil {
			return fmt.Errorf("plugins: replacing package: %w", err)
		}
	}
	id := xid.New().String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO plugin_packages (id, slug, version, build, sha256, size, signature, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, p.Slug, p.Version, p.Build, hex.EncodeToString(sum[:]), len(p.Tarball), p.Signature, now, now); err != nil {
		return fmt.Errorf("plugins: storing package: %w", err)
	}
	for seq, off := 0, 0; off < len(p.Tarball); seq, off = seq+1, off+chunkBytes {
		end := min(off+chunkBytes, len(p.Tarball))
		if _, err := tx.ExecContext(ctx, `INSERT INTO plugin_package_chunks (id, package_id, seq, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			xid.New().String(), id, seq, base64.StdEncoding.EncodeToString(p.Tarball[off:end]), now, now); err != nil {
			return fmt.Errorf("plugins: storing package chunk: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("plugins: storing package: %w", err)
	}
	return nil
}

func (s *dbStore) Get(ctx context.Context, slug, version string) (StoredPackage, error) {
	var id, build, sum, sig string
	err := s.db.QueryRowContext(ctx, `SELECT id, build, sha256, signature FROM plugin_packages WHERE slug = ? AND version = ?`, slug, version).
		Scan(&id, &build, &sum, &sig)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredPackage{}, ErrPackageMissing
	}
	if err != nil {
		return StoredPackage{}, fmt.Errorf("plugins: reading package: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM plugin_package_chunks WHERE package_id = ? ORDER BY seq LIMIT 100000`, id)
	if err != nil {
		return StoredPackage{}, fmt.Errorf("plugins: reading package: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugins: closing package chunk rows failed", "error", err)
		}
	}()
	var tarball []byte
	for rows.Next() {
		var chunk string
		if err := rows.Scan(&chunk); err != nil {
			return StoredPackage{}, fmt.Errorf("plugins: reading package chunk: %w", err)
		}
		raw, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			return StoredPackage{}, fmt.Errorf("plugins: decoding package chunk: %w", err)
		}
		tarball = append(tarball, raw...)
	}
	if err := rows.Err(); err != nil {
		return StoredPackage{}, fmt.Errorf("plugins: reading package: %w", err)
	}
	got := sha256.Sum256(tarball)
	if hex.EncodeToString(got[:]) != sum {
		return StoredPackage{}, fmt.Errorf("plugins: stored package %s@%s is corrupt", slug, version)
	}
	return StoredPackage{Slug: slug, Version: version, Build: build, Signature: sig, Tarball: tarball}, nil
}

func (s *dbStore) Delete(ctx context.Context, slug, version string) error {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM plugin_packages WHERE slug = ? AND version = ?`, slug, version).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("plugins: deleting package: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM plugin_package_chunks WHERE package_id = ?`, id); err != nil {
		return fmt.Errorf("plugins: deleting package: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM plugin_packages WHERE id = ?`, id); err != nil {
		return fmt.Errorf("plugins: deleting package: %w", err)
	}
	return nil
}

// fsStore keeps packages as files on a path every node mounts (NFS, SMB, a cluster
// filesystem). The signature travels in a sidecar beside the tarball.
type fsStore struct{ root string }

// NewSharedFSStore returns a store rooted at a shared mount.
func NewSharedFSStore(root string) PackageStore { return &fsStore{root: root} }

func (s *fsStore) Kind() string { return StoreSharedFS }

func (s *fsStore) path(slug, version string) (string, error) {
	if !slugVersionRE.MatchString(slug) || !slugVersionRE.MatchString(version) {
		return "", fmt.Errorf("plugins: refusing package name %q@%q", slug, version)
	}
	return filepath.Join(s.root, slug+"@"+version+".tar.gz"), nil
}

func (s *fsStore) Put(_ context.Context, p StoredPackage) error {
	path, err := s.path(p.Slug, p.Version)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("plugins: creating package store: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, p.Tarball, 0o644); err != nil {
		return fmt.Errorf("plugins: writing package: %w", err)
	}
	if err := os.WriteFile(path+".sig", []byte(p.Signature+"\n"+p.Build+"\n"), 0o644); err != nil {
		return fmt.Errorf("plugins: writing package signature: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("plugins: placing package: %w", err)
	}
	return nil
}

func (s *fsStore) Get(_ context.Context, slug, version string) (StoredPackage, error) {
	path, err := s.path(slug, version)
	if err != nil {
		return StoredPackage{}, err
	}
	tarball, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return StoredPackage{}, ErrPackageMissing
	}
	if err != nil {
		return StoredPackage{}, fmt.Errorf("plugins: reading package: %w", err)
	}
	meta, err := os.ReadFile(path + ".sig")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return StoredPackage{}, fmt.Errorf("plugins: reading package signature: %w", err)
	}
	lines := strings.SplitN(string(meta), "\n", 3)
	p := StoredPackage{Slug: slug, Version: version, Tarball: tarball}
	if len(lines) > 0 {
		p.Signature = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		p.Build = strings.TrimSpace(lines[1])
	}
	return p, nil
}

func (s *fsStore) Delete(_ context.Context, slug, version string) error {
	path, err := s.path(slug, version)
	if err != nil {
		return err
	}
	for _, f := range []string{path, path + ".sig"} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("plugins: deleting package: %w", err)
		}
	}
	return nil
}

// objectStore keeps packages in a storage location (S3, SFTP, SMB …) through the
// storage-* plugin that serves it; the signature rides in a sidecar object.
type objectStore struct {
	op       ObjectOperator
	location string
}

// NewObjectStore returns a store backed by a configured storage location.
func NewObjectStore(op ObjectOperator, location string) PackageStore {
	return &objectStore{op: op, location: location}
}

func (s *objectStore) Kind() string { return StoreObject }

func (s *objectStore) Put(ctx context.Context, p StoredPackage) error {
	key := objectKey(p.Slug, p.Version)
	if err := s.op.UploadObject(ctx, s.location, key, p.Tarball); err != nil {
		return fmt.Errorf("plugins: uploading package: %w", err)
	}
	if err := s.op.UploadObject(ctx, s.location, key+".sig", []byte(p.Signature+"\n"+p.Build+"\n")); err != nil {
		return fmt.Errorf("plugins: uploading package signature: %w", err)
	}
	return nil
}

func (s *objectStore) Get(ctx context.Context, slug, version string) (StoredPackage, error) {
	key := objectKey(slug, version)
	tarball, err := s.op.DownloadObject(ctx, s.location, key)
	if err != nil {
		return StoredPackage{}, fmt.Errorf("%w: %w", ErrPackageMissing, err)
	}
	p := StoredPackage{Slug: slug, Version: version, Tarball: tarball}
	if meta, err := s.op.DownloadObject(ctx, s.location, key+".sig"); err == nil {
		lines := strings.SplitN(string(meta), "\n", 3)
		p.Signature = strings.TrimSpace(lines[0])
		if len(lines) > 1 {
			p.Build = strings.TrimSpace(lines[1])
		}
	}
	return p, nil
}

func (s *objectStore) Delete(ctx context.Context, slug, version string) error {
	key := objectKey(slug, version)
	if err := s.op.DeleteObject(ctx, s.location, key); err != nil {
		return fmt.Errorf("plugins: deleting package: %w", err)
	}
	if err := s.op.DeleteObject(ctx, s.location, key+".sig"); err != nil {
		return fmt.Errorf("plugins: deleting package signature: %w", err)
	}
	return nil
}
