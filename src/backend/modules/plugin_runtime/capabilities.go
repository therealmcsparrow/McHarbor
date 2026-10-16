// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugin_runtime

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// LoginEndpoint is the one fact the sign-in screen needs before a session exists: which
// route a directory provider type posts credentials to.
type LoginEndpoint struct {
	ProviderType string `json:"providerType"`
	Path         string `json:"path"`
}

// handleCapabilities returns every capability the loaded plugins declare. It is
// authenticated and ungated like the rest of the loader surface, because host pages
// read it for every signed-in user; each capability's own permission gates its use.
func (h *Handler) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	response.OK(w, plugincaps.Default.All())
}

// handleLoginEndpoints is public because the login screen runs before a session
// exists. It returns only provider types and public route paths, which the login
// form would reveal the moment it posted anyway.
func (h *Handler) handleLoginEndpoints(w http.ResponseWriter, _ *http.Request) {
	out := []LoginEndpoint{}
	for _, c := range plugincaps.Default.OfKind(plugincaps.KindLoginEndpoint) {
		out = append(out, LoginEndpoint{ProviderType: c.ProviderType, Path: c.Path})
	}
	response.OK(w, out)
}
