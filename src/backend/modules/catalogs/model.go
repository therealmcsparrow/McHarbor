// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package catalogs

import "time"

// Kind enumerates the catalog surfaces admin can manage.
//
// Dashboard widgets and workflow nodes used to have their own kinds here.
// They ship as plugin bundles (`widget-xx`, `nodes-xx`) instead, so they
// arrive through KindPlugin and no longer need a catalog source of their
// own. Enabling and disabling an individual widget or node stays where it
// was, in the App Store.
type Kind string

const (
	// KindDockerApp sources Docker application templates for the App Store.
	KindDockerApp Kind = "docker_app"
	// KindPlugin sources installable McHarbor plugin bundles.
	KindPlugin Kind = "plugin"
	// KindTheme sources installable UI theme packs.
	KindTheme Kind = "theme"
	// KindVMImage sources the guest images addon-compute builds VMs and
	// system containers from. The kind is host-known rather than plugin-
	// invented so ValidKind, the probe's payload matcher and the six i18n
	// bundles stay authoritative here; a plugin contributes only the rows.
	KindVMImage Kind = "vm_image"
)

// AllKinds is the canonical kind list used by the API/UI to render
// per-kind sections. Order matches the UI's tab order.
var AllKinds = []Kind{KindDockerApp, KindPlugin, KindVMImage, KindTheme}

// ValidKind reports whether k is one of the recognized kinds.
func ValidKind(k string) bool {
	switch Kind(k) {
	case KindDockerApp, KindPlugin, KindTheme, KindVMImage:
		return true
	}
	return false
}

// Catalog is one row in settings_catalogs. Read-only rows have immutable
// metadata and cannot be deleted, but administrators may enable or disable
// them.
type Catalog struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Kind     Kind   `json:"kind"`
	Enabled  bool   `json:"enabled"`
	ReadOnly bool   `json:"readonly"`
	// Origin names the plugin whose manifest declared this row, empty for
	// a host built-in or an operator-added one. It is what lets the seed
	// reap a contributed row once its plugin is gone without touching
	// anything an operator chose.
	Origin    string    `json:"origin,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRequest is the body of POST /api/catalogs.
type CreateRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

// UpdateRequest is the body of PUT /api/catalogs/{id}. All fields
// optional; only non-nil fields are written.
type UpdateRequest struct {
	Name    *string `json:"name,omitempty"`
	URL     *string `json:"url,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}
