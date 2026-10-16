// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package clidownload

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
)

func writeArchive(t *testing.T, dir, name string, fill byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), bytes.Repeat([]byte{fill}, minArchiveSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalBuildShadowsTheImageAndChecksumsAreRead(t *testing.T) {
	local, image := t.TempDir(), t.TempDir()
	writeArchive(t, image, "mcharbor_2026.09.01_linux_amd64.tar.gz", 'i')
	writeArchive(t, image, "mcharbor_2026.09.01_windows_amd64.zip", 'i')
	writeArchive(t, local, "mcharbor_2026.09.02_linux_amd64.tar.gz", 'l')
	sum := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(local, "mcharbor_2026.09.02_checksums.txt"),
		[]byte(sum+" *mcharbor_2026.09.02_linux_amd64.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(local, image)
	list := svc.List()
	if len(list) != 2 {
		t.Fatalf("list = %+v, want one entry per platform", list)
	}
	linux, err := svc.Find("linux", "amd64")
	if err != nil || linux.Version != "2026.09.02" || linux.SHA256 != sum {
		t.Fatalf("linux = %+v, %v; the local build must win and carry its checksum", linux, err)
	}
}

func TestOnlyWellFormedFullSizeArchivesAreOffered(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mcharbor_2026.09.01_linux_arm64.tar.gz"), []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArchive(t, dir, "mcharbor_2026.09.01_plan9_amd64.tar.gz", 'x')
	writeArchive(t, dir, "notes.txt", 'x')
	if list := NewService(dir).List(); len(list) != 0 {
		t.Fatalf("offered %+v; a truncated file, an unknown OS and a stray file must all be skipped", list)
	}
	if _, err := NewService(dir).Find("../../etc", "passwd"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a path-like platform resolved: %v", err)
	}
}

func TestDownloadStreamsTheArchiveUnderItsName(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "mcharbor_2026.09.01_windows_amd64.zip", 'z')
	h := &Handler{svc: NewService(dir)}

	serve := func(osName, arch string, signedIn bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/cli/download/"+osName+"/"+arch, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("os", osName)
		rctx.URLParams.Add("arch", arch)
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		if signedIn {
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.User{ID: "u1", Username: "u1"})
		}
		rec := httptest.NewRecorder()
		h.HandleDownload(rec, req.WithContext(ctx))
		return rec
	}

	rec := serve("windows", "amd64", true)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "mcharbor_2026.09.01_windows_amd64.zip") {
		t.Fatalf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	if rec.Body.Len() != minArchiveSize+1 {
		t.Fatalf("served %d bytes", rec.Body.Len())
	}
	if rec := serve("linux", "arm64", true); rec.Code != http.StatusNotFound {
		t.Fatalf("missing platform returned %d", rec.Code)
	}
	if rec := serve("windows", "amd64", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous download returned %d", rec.Code)
	}
}
