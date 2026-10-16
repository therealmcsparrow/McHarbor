// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package openapi serves the generated OpenAPI document, merged with plugin-contributed routes.
package openapi

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// Handler serves the OpenAPI document for this instance.
type Handler struct {
	app     *router.AppDeps
	service *Service
}

// NewHandler creates a new openapi handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app, service: NewService(app.DB)}
}

// HandleDocs returns the OpenAPI specification as JSON. It is generated from
// the route registry this instance actually mounted, plus the routes of every
// installed plugin; ?source=core returns the core document CI commits.
func (h *Handler) HandleDocs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("source") == "core" {
		response.OK(w, apispec.Document(h.app.Spec, appversion.Current()))
		return
	}

	pluginOps, err := h.service.PluginOperations(r.Context())
	if err != nil {
		h.app.Logger.Error("openapi: loading plugin routes failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}

	live := apispec.NewRegistry()
	for _, op := range h.app.Spec.Operations() {
		live.Add(op)
	}
	for _, op := range pluginOps {
		live.Add(op)
	}
	response.OK(w, apispec.Document(live, appversion.Current()))
}
