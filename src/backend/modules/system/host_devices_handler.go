// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// hostDeviceLister is the one Service method the route needs, so the route can
// be tested without a Docker daemon.
type hostDeviceLister interface {
	HostDevices(ctx context.Context, envID string) (*HostDeviceList, error)
}

type hostDevicesHandler struct {
	app    *router.AppDeps
	lister hostDeviceLister
}

// mountHostDeviceRoutes registers the device listing under /system. It reads
// the host, so it takes host.view rather than a container key.
func mountHostDeviceRoutes(r *apispec.Router, app *router.AppDeps, h *hostDevicesHandler) {
	r.With(rbac.RequirePermission(app.RBACService, rbac.PermHostView)).Get("/host-devices", h.HandleHostDevices, apispec.Scoped(), apispec.Response(HostDeviceList{}))
}

// HandleHostDevices lists the device nodes on the environment's host for the
// container device picker; failures are logged, never echoed.
func (h *hostDevicesHandler) HandleHostDevices(w http.ResponseWriter, r *http.Request) {
	if user := auth.RequireAuth(r); user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	envID := response.ParseEnvID(r)
	list, err := h.lister.HostDevices(r.Context(), envID)
	if err != nil {
		h.app.Logger.Error("system: listing host devices failed", "env", envID, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrHostDevicesFailed, err)
		return
	}
	response.OK(w, list)
}
