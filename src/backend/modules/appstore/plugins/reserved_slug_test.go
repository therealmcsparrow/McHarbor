// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/pluginslug"
)

// completeTarball is a package whose manifest passes the schema, so a refusal can only come
// from the slug.
func completeTarball(t *testing.T, slug string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	manifest := []byte("schema: \"mcharbor.plugin/v1\"\nslug: \"" + slug + "\"\nname: \"Test\"\nversion: \"2026.09.01\"\nauthor: \"McSparrow\"\nmcharbor: \">=2026.09.01\"\n")
	if err := tw.WriteHeader(&tar.Header{Name: "plugin.yaml", Mode: 0o644, Size: int64(len(manifest)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestInstallRefusesAReservedSlug: every install path runs the manifest through the same
// check, so a package slugged "host" is refused before anything is written.
func TestInstallRefusesAReservedSlug(t *testing.T) {
	inst, svc, _ := unsignedInstaller(t, nil)
	_, err := inst.Install(context.Background(), Source{Tarball: completeTarball(t, "host"), AllowUnsigned: true}, nil)
	if !errors.Is(err, pluginslug.ErrReserved) || !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("a package slugged host must be refused as an invalid manifest, got %v", err)
	}
	if v, err := svc.LookupVersion(context.Background(), "host"); err != nil || v != "" {
		t.Fatalf("a refused install wrote a row at %q (%v)", v, err)
	}
}

// TestSyncRefusesAReservedSlug: a stored package slugged "host" is never placed on a node
// that syncs it, even unsigned-allowed; the shared store is no proof of origin.
func TestSyncRefusesAReservedSlug(t *testing.T) {
	inst, _, dataDir := unsignedInstaller(t, nil)
	target := filepath.Join(dataDir, "plugins", "host@2026.09.01")
	err := inst.place(context.Background(), StoredPackage{Slug: "host", Version: "2026.09.01", Tarball: completeTarball(t, "host")}, target)
	if !errors.Is(err, pluginslug.ErrReserved) {
		t.Fatalf("placing a package slugged host must be refused as reserved, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("a refused package was placed at %s (%v)", target, statErr)
	}
}
