// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// PluginTarget is one plugin an instance restore wants present.
type PluginTarget struct {
	Slug    string
	Version string
	Enabled bool
	// AllowUnsigned carries consent to an unsigned artifact. A restore sets it,
	// because the backup records a plugin the operator already installed.
	AllowUnsigned bool
}

// ReconcileOutcome reports what reconciling one plugin did: kept, installed,
// updated or failed, the last with a reason the operator can act on.
type ReconcileOutcome struct {
	Slug    string
	Version string
	Action  string
	Error   string
}

// ReconcileInstalled makes each wanted plugin present at its exact version
// after an instance restore. It uses the same uninstall-then-install cycle as a
// repair, so the row, install directory, manifest and binary agree and role
// grants survive. A version the catalog no longer serves fails that plugin by
// name while the others continue; plugins absent from want are left alone.
func (h *Handler) ReconcileInstalled(ctx context.Context, want []PluginTarget) []ReconcileOutcome {
	out := make([]ReconcileOutcome, 0, len(want))
	for _, p := range want {
		action, err := h.reconcileTarget(ctx, p)
		o := ReconcileOutcome{Slug: p.Slug, Version: p.Version, Action: action}
		if err != nil {
			o.Action, o.Error = "failed", err.Error()
		}
		out = append(out, o)
	}
	return out
}

func (h *Handler) reconcileTarget(ctx context.Context, p PluginTarget) (string, error) {
	installed, err := h.svc.LookupVersion(ctx, p.Slug)
	if err != nil {
		return "", fmt.Errorf("reading the installed version: %w", err)
	}
	action := "kept"
	switch {
	case installed == p.Version:
	case installed != "":
		if err := h.repairOne(ctx, InstalledPlugin{Slug: p.Slug, Version: p.Version}, p.AllowUnsigned, false); err != nil {
			return "", err
		}
		action = "updated"
	default:
		src, err := h.catalogSourceFor(ctx, p.Slug, p.Version, false)
		if err != nil {
			return "", err
		}
		src.AllowUnsigned = p.AllowUnsigned
		if _, err := h.installer.Install(ctx, src, nil); err != nil {
			return "", fmt.Errorf("installing: %w", err)
		}
		h.notifyChange(p.Slug)
		action = "installed"
	}
	if err := h.svc.MarkEnabled(ctx, p.Slug, p.Enabled); err != nil {
		return "", fmt.Errorf("setting the enabled flag: %w", err)
	}
	return action, nil
}

// ReconcileRequest pins one plugin to an exact catalog version.
type ReconcileRequest struct {
	Version string `json:"version"`
	Enabled *bool  `json:"enabled,omitempty"`
	// AllowUnsigned is the same explicit consent InstallRequest carries.
	AllowUnsigned bool `json:"allow_unsigned,omitempty"`
}

// ReconcileResult is what pinning a plugin did.
type ReconcileResult struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Action  string `json:"action"`
}

// handleReconcile serves POST /api/appstore/plugins/{slug}/reconcile: make the
// plugin installed at exactly this catalog version, installing, moving or
// keeping it, with its role grants intact. It is the declarative counterpart
// of install and update, for tools that state a version rather than a step.
func (h *Handler) handleReconcile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var in ReconcileRequest
	if err := response.DecodeBody(r, &in); err != nil || slug == "" || strings.TrimSpace(in.Version) == "" {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	out := h.ReconcileInstalled(r.Context(), []PluginTarget{{Slug: slug, Version: in.Version, Enabled: enabled, AllowUnsigned: in.AllowUnsigned}})[0]
	if out.Action == "failed" {
		slog.Warn("plugin reconcile failed", "slug", slug, "version", in.Version, "error", out.Error)
		response.ErrCodeWithData(w, r, http.StatusConflict, i18n.ErrPluginUpdateFailed, map[string]string{"slug": slug})
		return
	}
	if h.audit != nil {
		h.audit.Log(r, audit.Entry{Action: "reconcile", EntityType: "plugin", EntityID: slug, EntityName: slug,
			Details: fmt.Sprintf("version=%s action=%s enabled=%t", in.Version, out.Action, enabled)})
	}
	response.OK(w, ReconcileResult{Slug: slug, Version: in.Version, Action: out.Action})
}
