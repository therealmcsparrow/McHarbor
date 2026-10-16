// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

func pluginTarball(t *testing.T, slug, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	manifest := "schema: \"mcharbor.plugin/v1\"\nslug: \"" + slug + "\"\nname: \"Test\"\nversion: \"" + version + "\"\nmcharbor: \">=2026.09.01\"\n"
	padding := make([]byte, 100<<10)
	if _, err := rand.Read(padding); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"plugin.yaml": []byte(manifest), "backend/blob.bin": padding} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func migratedPlugins(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestStoresRoundTrip guards both self-hosted stores: a package larger than one chunk
// comes back byte for byte with its signature, and a missing one is reported as such.
func TestStoresRoundTrip(t *testing.T) {
	ctx := context.Background()
	tarball := pluginTarball(t, "comm-x", "2026.09.01")
	for _, store := range []PackageStore{NewDatabaseStore(migratedPlugins(t)), NewSharedFSStore(t.TempDir())} {
		if _, err := store.Get(ctx, "comm-x", "2026.09.01"); !errors.Is(err, ErrPackageMissing) {
			t.Fatalf("%s: empty store must report missing: %v", store.Kind(), err)
		}
		in := StoredPackage{Slug: "comm-x", Version: "2026.09.01", Build: "202609262300", Signature: "c2ln", Tarball: tarball}
		if err := store.Put(ctx, in); err != nil {
			t.Fatalf("%s put: %v", store.Kind(), err)
		}
		if err := store.Put(ctx, in); err != nil {
			t.Fatalf("%s re-put must replace: %v", store.Kind(), err)
		}
		out, err := store.Get(ctx, "comm-x", "2026.09.01")
		if err != nil || !bytes.Equal(out.Tarball, tarball) || out.Signature != "c2ln" || out.Build != "202609262300" {
			t.Fatalf("%s get: %v sig=%q build=%q same=%v", store.Kind(), err, out.Signature, out.Build, bytes.Equal(out.Tarball, tarball))
		}
		if err := store.Delete(ctx, "comm-x", "2026.09.01"); err != nil {
			t.Fatalf("%s delete: %v", store.Kind(), err)
		}
		if _, err := store.Get(ctx, "comm-x", "2026.09.01"); !errors.Is(err, ErrPackageMissing) {
			t.Fatalf("%s: deleted package still served: %v", store.Kind(), err)
		}
	}
	if err := NewSharedFSStore(t.TempDir()).Put(ctx, StoredPackage{Slug: "../etc", Version: "1", Tarball: tarball}); err == nil {
		t.Fatal("a path-traversing slug must be refused")
	}
}

// TestSyncPlacesVerifiedPackagesOnly guards the standby path: a node missing an
// installed plugin gets it from the store, and a package whose bytes no longer match
// the signature that verified it is refused rather than loaded.
func TestSyncPlacesVerifiedPackagesOnly(t *testing.T) {
	ctx := context.Background()
	database := migratedPlugins(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	svc := NewService(database, nil, "")
	store := NewDatabaseStore(database)
	inst := NewInstaller(dataDir, database, svc, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), []ed25519.PublicKey{pub}).WithPackageStore(store)

	good := pluginTarball(t, "comm-good", "2026.09.01")
	bad := pluginTarball(t, "comm-bad", "2026.09.01")
	sign := func(b []byte) string { return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, b)) }
	if err := store.Put(ctx, StoredPackage{Slug: "comm-good", Version: "2026.09.01", Signature: sign(good), Tarball: good}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, StoredPackage{Slug: "comm-bad", Version: "2026.09.01", Signature: sign(good), Tarball: bad}); err != nil {
		t.Fatal(err)
	}
	swapped := pluginTarball(t, "comm-other", "2026.09.01")
	if err := store.Put(ctx, StoredPackage{Slug: "comm-swap", Version: "2026.09.01", Signature: sign(swapped), Tarball: swapped}); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"comm-good", "comm-bad", "comm-gone", "comm-swap"} {
		if _, err := svc.InsertInstalledRow(ctx, slug, "2026.09.01", "T", "a", "d", true); err != nil {
			t.Fatal(err)
		}
	}
	res, err := inst.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Placed) != 1 || res.Placed[0] != "comm-good@2026.09.01" {
		t.Fatalf("placed = %v", res.Placed)
	}
	if _, ok := res.Failed["comm-bad@2026.09.01"]; !ok {
		t.Fatalf("a package that fails its signature must not be placed: %+v", res)
	}
	if _, ok := res.Failed["comm-swap@2026.09.01"]; !ok {
		t.Fatalf("a package that names another plugin must not be placed under this one: %+v", res)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "comm-gone@2026.09.01" {
		t.Fatalf("missing = %v", res.Missing)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "comm-good@2026.09.01", "plugin.yaml")); err != nil {
		t.Fatalf("placed package not on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "comm-bad@2026.09.01")); !os.IsNotExist(err) {
		t.Fatal("a refused package must leave nothing behind")
	}
	again, err := inst.Sync(ctx)
	if err != nil || len(again.Placed) != 0 || len(again.Failed) != 2 {
		t.Fatalf("a second sync must be a no-op: %+v %v", again, err)
	}
}

func TestParseStoreSetting(t *testing.T) {
	for in, want := range map[string]string{"": StoreLocal, "database": StoreDatabase, "shared-fs:/mnt/p": StoreSharedFS, "object:loc1": StoreObject} {
		got, err := ParseStoreSetting(in)
		if err != nil || got.Kind != want {
			t.Fatalf("%q -> %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"shared-fs:relative", "object:", "s3"} {
		if _, err := ParseStoreSetting(bad); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
}
