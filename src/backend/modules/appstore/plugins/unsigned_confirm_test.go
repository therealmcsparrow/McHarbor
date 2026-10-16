// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func unsignedInstaller(t *testing.T, keys []ed25519.PublicKey) (*DefaultInstaller, *Service, string) {
	t.Helper()
	database := migratedPlugins(t)
	dataDir := t.TempDir()
	svc := NewService(database, nil, "")
	inst := NewInstaller(dataDir, database, svc, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), keys)
	inst.platformOK = true
	inst.requireSignature = false
	return inst, svc, dataDir
}

func progressLog(lines *[]string) ProgressFunc {
	return func(_ string, _ int, msg string) { *lines = append(*lines, msg) }
}

// TestInstallRefusesUnsignedWithoutConsent guards TOFIX-07: the dialog is not the
// control, the installer is. An unsigned tarball with no AllowUnsigned installs nothing.
func TestInstallRefusesUnsignedWithoutConsent(t *testing.T) {
	inst, svc, dataDir := unsignedInstaller(t, nil)
	var lines []string
	_, err := inst.Install(context.Background(), Source{Tarball: pluginTarball(t, "comm-x", "2026.09.01")}, progressLog(&lines))
	if !errors.Is(err, ErrUnsignedNotConfirmed) {
		t.Fatalf("unsigned install without consent: err = %v, want ErrUnsignedNotConfirmed", err)
	}
	if v, err := svc.LookupVersion(context.Background(), "comm-x"); err != nil || v != "" {
		t.Fatalf("a refused install wrote a row at %q", v)
	}
	entries, readErr := os.ReadDir(filepath.Join(dataDir, "plugins"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("reading plugins dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused install left %d entries on disk", len(entries))
	}
}

// TestInstallProceedsWithConsentOrSignature guards the two paths that must stay open:
// an operator who confirmed, and a tarball a trust key verifies, which needs no consent.
func TestInstallProceedsWithConsentOrSignature(t *testing.T) {
	inst, _, _ := unsignedInstaller(t, nil)
	var lines []string
	_, err := inst.Install(context.Background(), Source{Tarball: pluginTarball(t, "comm-x", "2026.09.01"), AllowUnsigned: true}, progressLog(&lines))
	if errors.Is(err, ErrUnsignedNotConfirmed) {
		t.Fatal("a confirmed unsigned install was refused")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "installing at your own risk") {
		t.Fatalf("a confirmed unsigned install must still say so: %v", lines)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signedInst, _, _ := unsignedInstaller(t, []ed25519.PublicKey{pub})
	tarball := pluginTarball(t, "comm-y", "2026.09.01")
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, tarball))
	_, err = signedInst.Install(context.Background(), Source{Tarball: tarball, Signature: sig}, nil)
	if errors.Is(err, ErrUnsignedNotConfirmed) || errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("a signed install must not need consent: %v", err)
	}
}

// TestInstallRouteRefusesUnsignedWithItsCode guards the API shape scripts and the
// CLI depend on: the install stream (the only install route) ends with an error
// event carrying err.plugin.unsigned_confirmation_required, not a generic failure.
func TestInstallRouteRefusesUnsignedWithItsCode(t *testing.T) {
	inst, svc, _ := unsignedInstaller(t, nil)
	h := NewHandler(inst, svc, nil)
	body, err := json.Marshal(InstallRequest{Tarball: pluginTarball(t, "comm-x", "2026.09.01")})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.handleInstallStream(w, httptest.NewRequest(http.MethodPost, "/appstore/plugins/install/stream", strings.NewReader(string(body))))
	if !strings.Contains(w.Body.String(), "err.plugin.unsigned_confirmation_required") {
		t.Fatalf("install stream: %d %s", w.Code, w.Body.String())
	}
}

// TestUpdateRefusesUnsignedBeforeUninstalling guards the Store Update path: it tears
// the install down before reinstalling, so a refusal after that would delete the plugin.
func TestUpdateRefusesUnsignedBeforeUninstalling(t *testing.T) {
	tarball := pluginTarball(t, "comm-x", "2026.09.02")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/comm-x.tar.gz" {
			http.NotFound(w, r)
			return
		}
		if _, err := w.Write(tarball); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	catalogDir := t.TempDir()
	entry := `{"slug":"comm-x","name":"X","version":"2026.09.02","download_url":"` + srv.URL + `/comm-x.tar.gz","versions":[{"version":"2026.09.02","download_url":"` + srv.URL + `/comm-x.tar.gz"}]}`
	if err := os.WriteFile(filepath.Join(catalogDir, "comm-x.json"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}

	inst, _, _ := unsignedInstaller(t, nil)
	svc := NewService(inst.db, nil, catalogDir)
	inst.svc = svc
	if _, err := svc.InsertInstalledRow(context.Background(), "comm-x", "2026.09.01", "X", "", "", true); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(inst, svc, nil)
	r := chi.NewRouter()
	r.Post("/appstore/plugins/{slug}/update/stream", h.handleUpdateStream)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/appstore/plugins/comm-x/update/stream", http.NoBody))

	if !strings.Contains(w.Body.String(), `"error_code":"err.plugin.unsigned_confirmation_required"`) {
		t.Fatalf("update stream did not ask for consent:\n%s", w.Body.String())
	}
	if v, err := svc.LookupVersion(context.Background(), "comm-x"); err != nil || v != "2026.09.01" {
		t.Fatalf("a refused update touched the install: version now %q", v)
	}
}
