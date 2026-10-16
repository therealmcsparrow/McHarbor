// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// TestLoginEndpointsExposeOnlyTheLoginSlice: the public route answers with the
// provider type and path of each login-endpoint capability and nothing else the
// registry holds, since it is reachable before anyone has signed in.
func TestLoginEndpointsExposeOnlyTheLoginSlice(t *testing.T) {
	const slug = "identity-captest"
	plugincaps.Default.Set(slug, []plugincaps.Capability{
		{Kind: plugincaps.KindLoginEndpoint, ProviderType: "ldap", Path: "/api/identity-captest/auth"},
		{Kind: plugincaps.KindRunbookProvider, Domain: "captest"},
	})
	t.Cleanup(func() { plugincaps.Default.Remove(slug) })

	rec := httptest.NewRecorder()
	(&Handler{}).handleLoginEndpoints(rec, httptest.NewRequest(http.MethodGet, "/api/plugins/loader/login-endpoints", nil))

	var body struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var mine []map[string]string
	for _, e := range body.Data {
		if e["path"] == "/api/identity-captest/auth" {
			mine = append(mine, e)
		}
	}
	if len(mine) != 1 || mine[0]["providerType"] != "ldap" || len(mine[0]) != 2 {
		t.Fatalf("want exactly {providerType, path} for the declared endpoint, got %v", body.Data)
	}
	for _, e := range body.Data {
		if e["path"] == "" {
			t.Fatalf("a non-login capability leaked into the public route: %v", body.Data)
		}
	}
}
