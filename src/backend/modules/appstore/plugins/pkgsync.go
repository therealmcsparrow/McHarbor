// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor/core/pluginslug"
)

// WithPackageStore makes every install also land in store, and lets Sync place
// packages a node is missing; nil keeps the single-node behaviour.
func (d *DefaultInstaller) WithPackageStore(store PackageStore) *DefaultInstaller {
	d.store = store
	return d
}

// PackageStore returns the configured store, or nil when packages stay local.
func (d *DefaultInstaller) PackageStore() PackageStore { return d.store }

// verifiedSignature returns the base64 signature that proves tarball, so a node that
// syncs the package later verifies exactly what this node verified.
func (d *DefaultInstaller) verifiedSignature(ctx context.Context, src Source, tarball []byte) string {
	if sig := decodeSignature(src.Signature); sig != nil && d.verifySignature(tarball, sig) {
		return src.Signature
	}
	if raw := d.fetchSidecar(ctx, src.URL); raw != "" {
		if sig := decodeSignature(raw); sig != nil && d.verifySignature(tarball, sig) {
			return raw
		}
	}
	return ""
}

func (d *DefaultInstaller) storePackage(ctx context.Context, src Source, tarball []byte, slug, version string) {
	if d.store == nil {
		return
	}
	p := StoredPackage{Slug: slug, Version: version, Build: src.Build, Signature: d.verifiedSignature(ctx, src, tarball), Tarball: tarball}
	if err := d.store.Put(ctx, p); err != nil && d.logger != nil {
		d.logger.Error("plugins: package not stored; other nodes cannot load this plugin", "slug", slug, "store", d.store.Kind(), "error", err)
	}
}

func (d *DefaultInstaller) forgetPackage(ctx context.Context, slug, version string) {
	if d.store == nil {
		return
	}
	if err := d.store.Delete(ctx, slug, version); err != nil && d.logger != nil {
		d.logger.Warn("plugins: stored package not removed", "slug", slug, "store", d.store.Kind(), "error", err)
	}
}

// SyncResult reports what Sync did, so boot logs and the HA page can say why a node
// cannot load a plugin the database says is installed.
type SyncResult struct {
	Placed  []string
	Missing []string
	Failed  map[string]string
}

// Sync places every installed plugin whose directory this node lacks, from the store.
// It writes no database rows and runs no migrations: the install already did both
// on the shared database, and repeating them here would be a second install.
func (d *DefaultInstaller) Sync(ctx context.Context) (SyncResult, error) {
	res := SyncResult{Failed: map[string]string{}}
	if d.store == nil {
		return res, nil
	}
	installed, err := d.svc.ListInstalled(ctx)
	if err != nil {
		return res, err
	}
	for _, row := range installed {
		ref := row.Slug + "@" + row.Version
		target := filepath.Join(d.dataDir, "plugins", ref)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		p, err := d.store.Get(ctx, row.Slug, row.Version)
		if errors.Is(err, ErrPackageMissing) {
			res.Missing = append(res.Missing, ref)
			continue
		}
		if err != nil {
			res.Failed[ref] = err.Error()
			continue
		}
		if err := d.place(ctx, p, target); err != nil {
			res.Failed[ref] = err.Error()
			continue
		}
		res.Placed = append(res.Placed, ref)
	}
	return res, nil
}

// place verifies a stored package with this node's trust keys before it touches the
// plugins directory: the database is shared, so a row there is not proof of origin. A
// reserved slug is refused first, as every install and manifest load refuses it.
func (d *DefaultInstaller) place(ctx context.Context, p StoredPackage, target string) error {
	if err := pluginslug.Check(p.Slug); err != nil {
		return fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	signed, err := d.verifyTarball(ctx, Source{Signature: p.Signature}, p.Tarball)
	if err != nil {
		return err
	}
	if !signed && p.Signature != "" {
		return fmt.Errorf("%w: stored signature does not match", ErrSignatureInvalid)
	}
	staging, manifest, err := d.extractAndReadManifest(p.Tarball)
	if err != nil {
		return err
	}
	if manifest.Slug != p.Slug || manifest.Version != p.Version {
		if rmErr := os.RemoveAll(staging); rmErr != nil && d.logger != nil {
			d.logger.Warn("plugins: removing staging dir failed", "error", rmErr)
		}
		return fmt.Errorf("%w: stored package is %s@%s, expected %s@%s", ErrManifestInvalid, manifest.Slug, manifest.Version, p.Slug, p.Version)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating plugins dir: %w", err)
	}
	if err := os.Rename(staging, target); err != nil {
		if rmErr := os.RemoveAll(staging); rmErr != nil && d.logger != nil {
			d.logger.Warn("plugins: removing staging dir failed", "error", rmErr)
		}
		return fmt.Errorf("placing package: %w", err)
	}
	return nil
}
