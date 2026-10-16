// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package contract

import "github.com/therealmcsparrow/mcharbor/core/plugin/types"

// Manifest is the parsed plugin.yaml file. Plugins read it once at
// Init() time and surface it via Manifest().
type Manifest struct {
	Schema   string `json:"schema" yaml:"schema"`
	Slug     string `json:"slug" yaml:"slug"`
	Name     string `json:"name" yaml:"name"`
	Version  string `json:"version" yaml:"version"`
	Author   string `json:"author" yaml:"author"`
	License  string `json:"license" yaml:"license"`
	Homepage string `json:"homepage" yaml:"homepage"`
	Desc     string `json:"description" yaml:"description"`
	Mcharbor string `json:"mcharbor" yaml:"mcharbor"`
	// TablePrefix names the prefix this plugin's migrations actually
	// used, for plugins whose table names predate their current slug.
	// Uninstall drops `plugin_<slug>_*` by default; a plugin that was
	// renamed keeps its original table names and declares them here so
	// the sweep still finds them. Empty means the derived default.
	TablePrefix string `json:"table_prefix,omitempty" yaml:"table_prefix,omitempty"`
	Go          string `json:"go" yaml:"go"`
	// Requires names other plugins this one needs installed to work.
	//
	// The installer pulls each missing one in alongside this plugin and
	// removes it again when the last plugin requiring it is uninstalled — a
	// dependency it installed itself, never one the operator chose. Slugs
	// only: there is no version constraint here, because a plugin family
	// shipped together moves together, and a resolver is a great deal of
	// machinery for a problem this host does not have.
	Requires  []string                `json:"requires,omitempty" yaml:"requires,omitempty"`
	Perms     []string                `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	AddsPerms []types.PermissionDecl  `json:"adds_permissions,omitempty" yaml:"adds_permissions,omitempty"`
	Consumes  []string                `json:"consumes_permissions,omitempty" yaml:"consumes_permissions,omitempty"`
	ABACRules []types.ABACRuleDecl    `json:"adds_abac_rules,omitempty" yaml:"adds_abac_rules,omitempty"`
	Config    []types.ConfigFieldDecl `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`
	Contribs  Contributes             `json:"contributes" yaml:"contributes"`
	// Caps declares the privileged capabilities the plugin needs.
	// net.bind and fs.write are hard-vetoed by the loader (they
	// imply the host would let the plugin break out of its data
	// dir). Other capabilities are advisory for v1.
	Caps      Capabilities `json:"capabilities" yaml:"capabilities"`
	Signature string       `json:"signature,omitempty" yaml:"signature,omitempty"`
	Checksum  string       `json:"checksum,omitempty" yaml:"checksum,omitempty"`
}

// Capabilities expresses the privileged surfaces a plugin needs. The
// loader enforces these at Boot time (architecture §9).
type Capabilities struct {
	NetBind      bool `json:"net_bind,omitempty" yaml:"net_bind,omitempty"`           // ALWAYS denied
	FSWrite      bool `json:"fs_write,omitempty" yaml:"fs_write,omitempty"`           // ALWAYS denied if outside plugin data dir
	OutboundHTTP bool `json:"outbound_http,omitempty" yaml:"outbound_http,omitempty"` // informational; allowed
	OutboundTCP  bool `json:"outbound_tcp,omitempty" yaml:"outbound_tcp,omitempty"`   // informational; allowed
	Subprocess   bool `json:"subprocess,omitempty" yaml:"subprocess,omitempty"`       // informational; allowed
}

// Contributes lists everything a plugin adds to the host, so the loader can register it in one pass.
type Contributes struct {
	Routes      []RouteDecl        `json:"routes,omitempty" yaml:"routes,omitempty"`
	Menu        []MenuItem         `json:"menu,omitempty" yaml:"menu,omitempty"`
	Widgets     []string           `json:"widgets,omitempty" yaml:"widgets,omitempty"`
	Nodes       []string           `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Themes      []ThemeDecl        `json:"themes,omitempty" yaml:"themes,omitempty"`
	Catalogs    []CatalogDecl      `json:"catalogs,omitempty" yaml:"catalogs,omitempty"`
	Migrations  []string           `json:"migrations,omitempty" yaml:"migrations,omitempty"`
	I18nNS      []string           `json:"i18n_namespaces,omitempty" yaml:"i18n_namespaces,omitempty"`
	SCIMSchemas []types.SCIMSchema `json:"scim_schemas,omitempty" yaml:"scim_schemas,omitempty"`
	// Forms is the per-resource extension-point list. Each entry
	// names a resource (channel, email_server, identity_provider,
	// storage_location, ai_provider) and the kind of UI to mount
	// (config_form | setup_guide | selector). The host's edit
	// dialogs query this list at render time and mount the plugin's
	// React component into a stable slot.
	Forms []FormDecl `json:"forms,omitempty" yaml:"forms,omitempty"`
}

// ThemeDecl declares a CSS theme contributed by a plugin. CSSPath is
// plugin-relative and served through the plugin asset endpoint.
type ThemeDecl struct {
	ID             string `json:"id" yaml:"id"`
	Name           string `json:"name" yaml:"name"`
	Description    string `json:"description,omitempty" yaml:"description,omitempty"`
	CSSPath        string `json:"css_path" yaml:"css_path"`
	SwatchHue      int    `json:"swatch_hue,omitempty" yaml:"swatch_hue,omitempty"`
	DefaultVariant string `json:"default_variant,omitempty" yaml:"default_variant,omitempty"`
	HasDark        bool   `json:"has_dark,omitempty" yaml:"has_dark,omitempty"`
	HasLight       bool   `json:"has_light,omitempty" yaml:"has_light,omitempty"`
}

// CatalogDecl declares a catalog source the plugin brings with it, so its
// category appears in Settings the moment the plugin is installed and is
// reaped when it is removed.
//
// Kind must be one the host already knows (catalogs.ValidKind): a plugin
// supplies the rows, never a new category, because a category the six locale
// bundles cannot label would render as a raw i18n key.
type CatalogDecl struct {
	// ID is plugin-local; the host namespaces it by slug, so two plugins may
	// both declare "images" without colliding.
	ID   string `json:"id" yaml:"id"`
	Kind string `json:"kind" yaml:"kind"`
	Name string `json:"name" yaml:"name"`
	// URL is an https index document or an absolute path to a mounted
	// directory, matching what an operator may type into the same field.
	URL string `json:"url" yaml:"url"`
}

// FormDecl declares a single plugin-owned UI extension point. The
// host loads the plugin's UMD bundle (resolved via the existing
// /plugins/loader/forms endpoint) and calls the registered mount
// function for the matching kind into the host's edit dialog.
type FormDecl struct {
	Resource string `json:"resource" yaml:"resource"`           // channel | email_server | identity_provider | storage_location | ai_provider
	Kind     string `json:"kind" yaml:"kind"`                   // config_form | setup_guide | selector
	Key      string `json:"key,omitempty" yaml:"key,omitempty"` // selector key (defaults to Resource)
}

// RouteDecl declares one HTTP route a plugin serves, with the permission and audit metadata the host enforces.
type RouteDecl struct {
	Method          string `json:"method" yaml:"method"`                                           // "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
	Path            string `json:"path" yaml:"path"`                                               // "/api/<slug>/..."
	Requires        string `json:"requires_permission" yaml:"requires_permission"`                 // optional override
	Public          bool   `json:"public,omitempty" yaml:"public,omitempty"`                       // true => no auth required
	AuditRequired   *bool  `json:"audit_required,omitempty" yaml:"audit_required,omitempty"`       // nil => inferred by method
	AuditAction     string `json:"audit_action,omitempty" yaml:"audit_action,omitempty"`           // canonical action for plugin mutations
	AuditEntityType string `json:"audit_entity_type,omitempty" yaml:"audit_entity_type,omitempty"` // entity type for fallback audit rows
	AuditSeverity   string `json:"audit_severity,omitempty" yaml:"audit_severity,omitempty"`       // info | success | warning | error; empty => inferred
}

// MenuItem is one sidebar entry a plugin contributes.
//
// Parent doubles as the sidebar *section id*: migration 104 declares
// plugin_menu.parent as "optional section id". Entries that share a
// non-empty Parent render as their own named section in the sidebar,
// labelled `<LabelNS>:nav.section.<Parent>`. Entries with an empty
// Parent collapse into the generic "Plugins" section, which is the
// behaviour every plugin had before sections existed.
type MenuItem struct {
	Parent string `json:"parent" yaml:"parent"` // "" | sidebar section id owned by this plugin
	// LabelNS is the i18n namespace LabelKey resolves against. Empty
	// means "the plugin's slug", which is the convention every
	// bundled plugin follows.
	LabelNS  string `json:"label_ns,omitempty" yaml:"label_ns,omitempty"`
	LabelKey string `json:"label_key" yaml:"label_key"`           // i18n key into LabelNS
	Path     string `json:"path" yaml:"path"`                     // frontend route
	Icon     string `json:"icon,omitempty" yaml:"icon,omitempty"` // SVG filename in icons/
	Required string `json:"required_permission,omitempty" yaml:"required_permission,omitempty"`
	// Priority orders entries within a section, higher first. Zero — the
	// default — leaves an entry where the old rule put it: after every plugin
	// whose slug sorts earlier.
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`
}
