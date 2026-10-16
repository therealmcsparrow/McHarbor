// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// releaseArchive builds a tar.gz shaped like the upstream release: the binary at the root
// beside files that must not be extracted.
func releaseArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string][]byte{"LICENSE": []byte("license"), "trivy": binary} {
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

// serveRelease points the installer at a local server and pins archive's checksum (or a
// wrong one) for amd64, restoring both afterwards.
func serveRelease(t *testing.T, archive []byte, pinned string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) != trivyArchives["amd64"].name {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(archive); err != nil {
			t.Errorf("serving the archive: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	prevURL, prevPin := trivyReleaseURL, trivyArchives["amd64"]
	trivyReleaseURL = srv.URL + "/"
	trivyArchives["amd64"] = struct{ name, sha256 string }{prevPin.name, pinned}
	t.Cleanup(func() { trivyReleaseURL, trivyArchives["amd64"] = prevURL, prevPin })
	t.Setenv("PATH", "")
}

func TestInstallPutsTheVerifiedBinaryInThePluginFolder(t *testing.T) {
	archive := releaseArchive(t, []byte("trivy binary"))
	sum := sha256.Sum256(archive)
	serveRelease(t, archive, hex.EncodeToString(sum[:]))

	dir := filepath.Join(t.TempDir(), "bin")
	b := newBinaryInstaller(dir)
	if b.path() != "" {
		t.Fatal("nothing should be found before the install")
	}
	if err := b.install(t.Context(), "amd64"); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := b.path(); got != filepath.Join(dir, "trivy") {
		t.Fatalf("path = %q, want the plugin's own copy", got)
	}
	body, err := os.ReadFile(filepath.Join(dir, "trivy"))
	if err != nil || string(body) != "trivy binary" {
		t.Fatalf("installed binary = %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err == nil {
		t.Fatal("only the binary is extracted")
	}
}

func TestInstallRefusesAnArchiveThatDoesNotMatchThePin(t *testing.T) {
	serveRelease(t, releaseArchive(t, []byte("tampered")), "0000000000000000000000000000000000000000000000000000000000000000")

	dir := filepath.Join(t.TempDir(), "bin")
	b := newBinaryInstaller(dir)
	if err := b.install(t.Context(), "amd64"); err == nil {
		t.Fatal("an archive that does not match the pinned checksum must be refused")
	}
	if b.path() != "" {
		t.Fatal("a refused archive must leave no binary behind")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("the plugin folder should be empty, holds %d entries", len(entries))
	}
}
