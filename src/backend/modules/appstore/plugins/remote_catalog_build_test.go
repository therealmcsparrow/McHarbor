// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRemoteCatalogCarriesTheBuild is the missing Update button: a rebuild keeps
// its version, so the Store can only see it through the build, and a catalog
// read over HTTPS used to drop the build on the way to the entry the Store reads.
func TestRemoteCatalogCarriesTheBuild(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/catalog/plugins/index.json", func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `[
			{"slug":"addon-kvm","name":"Virtual Machines","versions":[
				{"version":"2026.10.01","build":"202610060901","built_for":"2026.10.01","download_url":"https://x/files/kvm.tar.gz"}]},
			{"slug":"comm-slack","name":"Slack","versions":[
				{"version":"2026.10.01","build":"202610010800","download_url":"https://x/files/slack-a.tar.gz"}]},
			{"slug":"comm-slack","name":"Slack","versions":[
				{"version":"2026.10.02","build":"202610050900","download_url":"https://x/files/slack-b.tar.gz"}]}
		]`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	previous := remoteHTTPClient
	remoteHTTPClient = srv.Client()
	defer func() { remoteHTTPClient = previous }()

	svc := NewService(nil, fixedSources{{Name: "remote", URL: srv.URL + "/catalog/plugins/index.json"}}, "")
	entries, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PluginEntry{}
	for _, e := range entries {
		got[e.Slug] = e
	}
	if kvm := got["addon-kvm"]; kvm.Build != "202610060901" || kvm.BuiltFor != "2026.10.01" {
		t.Fatalf("addon-kvm build = %q built_for = %q; the Store cannot offer a same-version update without it", kvm.Build, kvm.BuiltFor)
	}
	// Listed twice: the merged entry must carry the newest release's build,
	// not the first file's and not none.
	if slack := got["comm-slack"]; slack.Version != "2026.10.02" || slack.Build != "202610050900" {
		t.Fatalf("merged comm-slack = %s / %s, want 2026.10.02 / 202610050900", slack.Version, slack.Build)
	}
}
