// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package openapi

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// Handler holds dependencies for the OpenAPI docs handler.
type Handler struct {
	app *router.AppDeps
}

// NewHandler creates a new openapi handler.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{app: app}
}

// HandleDocs returns the OpenAPI specification as JSON.
func (h *Handler) HandleDocs(w http.ResponseWriter, r *http.Request) {
	response.OK(w, buildSpec())
}

func methodKey(m string) string {
	switch m {
	case "GET":
		return "get"
	case "POST":
		return "post"
	case "PUT":
		return "put"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return "get"
	}
}
