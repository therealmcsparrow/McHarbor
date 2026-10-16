// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/modules/widgets"
)

// TestSignWidgetsProducesWhatTheHostInstalls is the round trip that matters: an entry
// signed by -widget installs through the widgets module with the same trust key, and
// editing the signed label afterwards is refused.
func TestSignWidgetsProducesWhatTheHostInstalls(t *testing.T) {
	root := t.TempDir()
	catalog := filepath.Join(root, "widgets")
	if err := os.MkdirAll(filepath.Join(catalog, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := `window.McharborWidget_clock={mount:function(el){el.textContent="<tick>"}};`
	if err := os.WriteFile(filepath.Join(catalog, "files", "clock.umd.js"), []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := `{"key":"clock","version":"2026.09.01","label":"Clock & co","icon":"clock","bundle":"files/clock.umd.js",
		"defaultSize":{"w":3,"h":2},"minSize":{"w":2,"h":1},"translations":{"en":{"label":"Clock","n":1.5}}}`
	entryPath := filepath.Join(catalog, "clock.json")
	if err := os.WriteFile(entryPath, []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(root, "k")
	if err := generateKeyPair(prefix); err != nil {
		t.Fatal(err)
	}
	if err := signWidgets(catalog, prefix+".key", prefix+".pub"); err != nil {
		t.Fatalf("signWidgets: %v", err)
	}

	pubPEM, err := os.ReadFile(prefix + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := host.ParsePEMPublicKeys(pubPEM)
	if err != nil || len(keys) != 1 {
		t.Fatalf("trust key: %v", err)
	}
	install := func() error {
		svc := widgets.NewService(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil))).WithCatalog([]string{root}, []ed25519.PublicKey(keys))
		_, err := svc.Install("clock", widgets.InstallOptions{})
		return err
	}
	if err := install(); err != nil {
		t.Fatalf("host refused a -widget signed entry: %v", err)
	}

	raw, err := os.ReadFile(entryPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["label"] = "Clock (official)"
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := install(); !errors.Is(err, widgets.ErrSignatureInvalid) {
		t.Fatalf("relabelled entry: Install = %v, want ErrSignatureInvalid", err)
	}
}
