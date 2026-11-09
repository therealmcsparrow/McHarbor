// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// BackupTempLocationInput is the PUT /container-backups/temp body; an empty path resets to the default.
type BackupTempLocationInput struct {
	Path string `json:"path"`
}

// mountTempRoutes registers the backup temp location routes on the /container-backups group. The location is a
// host-wide setting, so every route takes the global backups.manage grant, never an environment-scoped one.
func mountTempRoutes(r *apispec.Router, app *router.AppDeps, h *Handler) {
	gate := rbac.RequireGlobalPermission(app.RBACService, rbac.PermBackupsManage)
	r.With(gate).Get("/temp", h.HandleGetTemp, apispec.Response(BackupTempStatus{}))
	r.With(gate).Put("/temp", h.HandleSaveTemp, apispec.Request(BackupTempLocationInput{}), apispec.Response(BackupTempStatus{}))
	r.With(gate).Post("/temp/cleanup", h.HandleCleanupTemp, apispec.Response(BackupTempCleanupResult{}))
}
