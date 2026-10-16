// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Error(err)
	}
}

type fixedSources []PluginSource

func (f fixedSources) EnabledPluginSources(context.Context, string) ([]PluginSource, error) {
	return f, nil
}

// TestRemoteCatalogIconsResolveThroughTheHost guards the store cards of a remote catalog: an
// entry naming plugin.svg must get the published icons/<slug>/ URL, and the host must fetch,
// validate and cache it, so an uninstalled plugin shows its mark instead of a puzzle piece.
func TestRemoteCatalogIconsResolveThroughTheHost(t *testing.T) {
	var iconHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/catalog/plugins/index.json", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `[
			{"slug":"comm-slack","name":"Slack","icon":"plugin.svg","versions":[{"version":"1.0.0","download_url":"https://x/files/a.tar.gz"}]},
			{"slug":"comm-fake","name":"Fake","icon":"plugin.svg","versions":[{"version":"1.0.0","download_url":"https://x/files/b.tar.gz"}]},
			{"slug":"comm-tabler","name":"Tabler","icon":"IconBrandSlack","versions":[{"version":"1.0.0","download_url":"https://x/files/c.tar.gz"}]}
		]`)
	})
	mux.HandleFunc("/catalog/plugins/icons/comm-slack/plugin.svg", func(w http.ResponseWriter, _ *http.Request) {
		iconHits.Add(1)
		writeBody(t, w, `<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	})
	mux.HandleFunc("/catalog/plugins/icons/comm-fake/plugin.svg", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `<html>not an icon</html>`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	previous := remoteHTTPClient
	remoteHTTPClient = srv.Client()
	defer func() { remoteHTTPClient = previous }()

	ctx := context.Background()
	svc := NewService(nil, fixedSources{{Name: "remote", URL: srv.URL + "/catalog/plugins/index.json"}}, "")
	entries, err := svc.ListCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	urls := map[string]string{}
	for _, e := range entries {
		urls[e.Slug] = e.IconURL
	}
	if want := srv.URL + "/catalog/plugins/icons/comm-slack/plugin.svg"; urls["comm-slack"] != want {
		t.Errorf("icon_url = %q, want %q", urls["comm-slack"], want)
	}
	if urls["comm-tabler"] != "" {
		t.Errorf("a Tabler icon name got an icon_url: %q", urls["comm-tabler"])
	}

	cache := t.TempDir()
	file, err := svc.RemoteIcon(ctx, cache, "comm-slack", "dark")
	if err != nil {
		t.Fatalf("icon not served (dark falls back to the plain mark): %v", err)
	}
	body, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(body), "<svg") {
		t.Fatalf("cached icon = %q, %v", body, err)
	}
	if _, err := svc.RemoteIcon(ctx, cache, "comm-slack", ""); err != nil {
		t.Fatal(err)
	}
	if iconHits.Load() != 1 {
		t.Errorf("icon fetched %d times, want 1 (second request must come from the cache)", iconHits.Load())
	}

	if _, err := svc.RemoteIcon(ctx, cache, "comm-fake", ""); err == nil {
		t.Error("an HTML page was accepted as plugin.svg")
	}
	if _, err := svc.RemoteIcon(ctx, cache, "not-listed", ""); !errors.Is(err, ErrIconNotFound) {
		t.Errorf("a slug no catalog lists = %v, want ErrIconNotFound", err)
	}
}
