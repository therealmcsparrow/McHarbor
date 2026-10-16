// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package host

import (
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/contract"
)

//go:embed manifest-v1.schema.json
var manifestSchemaJSON []byte

// ManifestSchemaError is returned by ValidateManifestSchema when a
// manifest does not conform to the v1 grammar. Field is a path-style
// locator (e.g. "adds_permissions[2].key"); Reason is human-readable.
type ManifestSchemaError struct {
	Field  string
	Reason string
}

func (e *ManifestSchemaError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("manifest schema: %s", e.Reason)
	}
	return fmt.Sprintf("manifest %s: %s", e.Field, e.Reason)
}

// ManifestError aliases ManifestSchemaError. The existing plugins
// package already references plugin.ManifestError, so we keep one
// symbol exported.
type ManifestError = ManifestSchemaError

// SchemaLenient reports whether the relaxed-schema mode is enabled.
// Settable from main.go via the MCHARBOR_PLUGIN_RELAXED_SCHEMA env var.
// When true, ValidateManifestSchema accepts unknown root fields and
// unknown permission / scope keys so legacy plugins keep loading
// across schema bumps.
var SchemaLenient = false

// pre-compiled patterns, derived from the JSON schema.
var (
	reSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)
	rePerm = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,62}[a-z0-9]$`)
	// A scope token in `permissions:` may carry a colon-separated argument —
	// `i18n.namespace.add:kubernetes` names the namespace the plugin is asking
	// for. rePerm has no colon and never did, so every such manifest failed
	// validation; the three shipped plugins that use the form (kubernetes,
	// reporting, swarm) are the evidence that the form, not the grammar, is
	// what was intended. The argument is stripped before the known-token
	// lookup below. Kept separate from rePerm because an `adds_permissions`
	// key is a plain RBAC key and must not gain a colon.
	reScope     = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,62}[a-z0-9](:[a-z0-9][a-z0-9_-]{0,62})?$`)
	reVer       = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-+][a-zA-Z0-9.]+)?$`)
	reRange     = regexp.MustCompile(`^[><=~^]*[0-9]+\.[0-9]+\.[0-9]+(\s*[><=~^]*[0-9]+\.[0-9]+\.[0-9]+)?$`)
	reChecksum  = regexp.MustCompile(`^[a-f0-9]{64}$`)
	reAPIPath   = regexp.MustCompile(`^/api/[a-z0-9-]+(/.*)?$`)
	reRoutePath = regexp.MustCompile(`^/[a-z0-9-]+(/.*)?$`)
	reRuleID    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	reThemeID   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)
	reCatalogID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)
)

// knownScopeTokens is the canonical permission scope list. Lenient
// mode skips this check (legacy plugins may declare scopes not yet in
// the schema).
var knownScopeTokens = map[string]bool{
	// host capabilities a plugin may declare to use a surface.
	"docker.read":               true,
	"docker.write":              true,
	"docker.socket.read":        true,
	"docker.socket.write":       true,
	"k8s.read":                  true,
	"k8s.write":                 true,
	"audit.read":                true,
	"audit.write":               true,
	"audit.emit":                true,
	"secrets.read":              true,
	"secrets.write":             true,
	"identity.read":             true,
	"identity.write":            true,
	"identity.issue_session":    true,
	"identity.adapter.register": true,
	"env.read":                  true,
	"env.write":                 true,
	"ai.invoke":                 true,
	// plugin-side surface registrations.
	"rbac.permission.register":      true,
	"menu.sidebar.add":              true,
	"i18n.namespace.add":            true,
	"dashboard.widget.add":          true,
	"db.migrations.run":             true,
	"notification.adapter.register": true,
	"snapshot.adapter.register":     true,
	// The remaining adapter families. They were reachable through
	// lenient mode only, so a strict-mode host would have rejected
	// every storage, AI, email and logging plugin already shipping —
	// the canonical list is meant to name every family that exists.
	"storage.adapter.register": true,
	"ai.adapter.register":      true,
	"email.adapter.register":   true,
	"logging.adapter.register": true,
	// A plugin that contributes report types to the Reporting plugin's
	// Report Studio. Registering costs the host nothing on its own —
	// the type only becomes reachable once Reporting is installed too.
	"reporting.adapter.register": true,
	// A plugin that contributes alert conditions to the host's alert
	// engine. It exposes no route of its own — the engine polls the
	// registered source on its own leader-elected ticker — so the token
	// buys evaluation, not reachable surface.
	"alerting.adapter.register": true,
	// A plugin that contributes an image vulnerability scanner. Like
	// alerting it exposes no route of its own — the host's /api/scans
	// subtree is the only admin surface — so the token buys the
	// ability to be dispatched a scan, not reachable surface.
	"scanner.adapter.register": true,
	// The agent-rpc bridge: a plugin that sends agent messages (guest,
	// pool, network and image commands) declares this token and an agent-rpc
	// capability listing the types, each inside the host's ceiling. The
	// loader hands HostContext.AgentRPC only to a plugin carrying both.
	"agent.rpc": true,
	// A plugin that brings its own catalog sources — an index of guest
	// images, say — and reads back the enabled ones. It mints no category
	// of its own: the kind must already be one catalogs.ValidKind knows, so
	// the token buys rows in an existing registry, not a new surface.
	"catalog.source.register": true,
	"encryption.handle.read":  true,
	"auth.user.read":          true,
	"settings.manage":         true,
	"rbac.environment.read":   true,
	// host-side RBAC permission keys that plugins may consume or reference.
	"containers.read":      true,
	"containers.write":     true,
	"communications.read":  true,
	"communications.write": true,
}

// knownCatalogKinds mirrors catalogs.ValidKind. It is duplicated rather than
// imported because core/plugin must not depend on a feature module — the two
// lists are small, closed, and change together.
var knownCatalogKinds = map[string]bool{
	"docker_app": true,
	"plugin":     true,
	"theme":      true,
	"vm_image":   true,
}

// KnownScopeToken reports whether s is a recognised scope token,
// stripping the optional colon-separated argument first so
// `i18n.namespace.add:kubernetes` resolves to `i18n.namespace.add`.
//
// Exported because the installer used to carry its own hand-copied
// allow-list, which drifted: it was missing menu.sidebar.add,
// i18n.namespace.add, rbac.permission.register, db.migrations.run,
// scanner.adapter.register and others, and it matched exactly rather
// than stripping the argument. Every addon, scanner, logging and LDAP
// plugin failed install on a token this list has always accepted.
// One list, one answer.
func KnownScopeToken(s string) bool {
	token := s
	if colon := strings.IndexByte(token, ':'); colon >= 0 {
		token = token[:colon]
	}
	return knownScopeTokens[token]
}

// KnownAuditSeverity reports whether s is a severity the host audit log stores, spelled exactly;
// a route declaring anything else would have its rows folded into an inferred severity.
func KnownAuditSeverity(s string) bool {
	switch s {
	case "info", "success", "warning", "error":
		return true
	}
	return false
}

// ValidateManifestSchema enforces the v1 grammar. The argument is the
// already-parsed Manifest (so the JSON syntax check is implicit). When
// SchemaLenient is true, unknown permission scope tokens and unknown
// additionalProperties are accepted — the rest of the grammar still
// applies.
func ValidateManifestSchema(m *contract.Manifest) error {
	if m == nil {
		return &ManifestSchemaError{Field: "", Reason: "manifest is nil"}
	}
	if m.Schema != "" && m.Schema != "mcharbor.plugin/v1" {
		return &ManifestSchemaError{Field: "schema", Reason: "must be \"mcharbor.plugin/v1\" (got \"" + m.Schema + "\")"}
	}

	if !reSlug.MatchString(m.Slug) {
		return &ManifestSchemaError{Field: "slug", Reason: "must match ^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$"}
	}
	if m.Name == "" {
		return &ManifestSchemaError{Field: "name", Reason: "required"}
	}
	if !reVer.MatchString(m.Version) {
		return &ManifestSchemaError{Field: "version", Reason: "must be semver (e.g. 1.0.0, 1.0.0-rc1)"}
	}
	if m.Author == "" {
		return &ManifestSchemaError{Field: "author", Reason: "required"}
	}
	if m.Mcharbor != "" && !reRange.MatchString(m.Mcharbor) {
		return &ManifestSchemaError{Field: "mcharbor", Reason: "must be a semver range (e.g. >=2.1.0 <3.0.0)"}
	}

	for i, p := range m.Perms {
		if !reScope.MatchString(p) {
			return &ManifestSchemaError{Field: fmt.Sprintf("permissions[%d]", i), Reason: "invalid permission key \"" + p + "\""}
		}
		// The token is what is looked up; the argument after the colon is the
		// plugin's own (a namespace name, say) and is not part of the grammar
		// of known tokens.
		token := p
		if colon := strings.IndexByte(token, ':'); colon >= 0 {
			token = token[:colon]
		}
		if !SchemaLenient && !knownScopeTokens[token] {
			return &ManifestSchemaError{Field: fmt.Sprintf("permissions[%d]", i), Reason: "unknown scope token \"" + p + "\""}
		}
	}

	seenKeys := make(map[string]bool)
	for i, p := range m.AddsPerms {
		if !rePerm.MatchString(p.Key) {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_permissions[%d].key", i), Reason: "invalid permission key \"" + p.Key + "\""}
		}
		if seenKeys[p.Key] {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_permissions[%d].key", i), Reason: "duplicate permission key \"" + p.Key + "\""}
		}
		seenKeys[p.Key] = true
		if p.Scope != "" && p.Scope != "global" && p.Scope != "env" && p.Scope != "stack" {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_permissions[%d].scope", i), Reason: "must be global|env|stack"}
		}
	}

	for i, p := range m.Consumes {
		if !rePerm.MatchString(p) {
			return &ManifestSchemaError{Field: fmt.Sprintf("consumes_permissions[%d]", i), Reason: "invalid permission key \"" + p + "\""}
		}
	}

	seenRuleIDs := make(map[string]bool)
	for i, r := range m.ABACRules {
		if !reRuleID.MatchString(r.ID) {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_abac_rules[%d].id", i), Reason: "invalid rule id \"" + r.ID + "\""}
		}
		if seenRuleIDs[r.ID] {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_abac_rules[%d].id", i), Reason: "duplicate rule id \"" + r.ID + "\""}
		}
		seenRuleIDs[r.ID] = true
		if r.Then != "" && r.Then != "allow" && r.Then != "deny" {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_abac_rules[%d].then", i), Reason: "must be allow|deny"}
		}
		if r.When == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("adds_abac_rules[%d].when", i), Reason: "required"}
		}
	}

	seenCfgKeys := make(map[string]bool)
	for i, c := range m.Config {
		if !rePerm.MatchString(c.Key) {
			return &ManifestSchemaError{Field: fmt.Sprintf("config_schema[%d].key", i), Reason: "invalid config key \"" + c.Key + "\""}
		}
		if seenCfgKeys[c.Key] {
			return &ManifestSchemaError{Field: fmt.Sprintf("config_schema[%d].key", i), Reason: "duplicate config key \"" + c.Key + "\""}
		}
		seenCfgKeys[c.Key] = true
		switch c.Type {
		case "string", "int", "bool", "secret":
		case "":
			return &ManifestSchemaError{Field: fmt.Sprintf("config_schema[%d].type", i), Reason: "required"}
		default:
			return &ManifestSchemaError{Field: fmt.Sprintf("config_schema[%d].type", i), Reason: "must be string|int|bool|secret"}
		}
		if c.Label == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("config_schema[%d].label", i), Reason: "required"}
		}
	}

	for i, r := range m.Contribs.Routes {
		if r.Method == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.routes[%d].method", i), Reason: "required"}
		}
		if !reAPIPath.MatchString(r.Path) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.routes[%d].path", i), Reason: "must start with /api/<slug>"}
		}
		if r.AuditAction != "" && !rePerm.MatchString(r.AuditAction) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.routes[%d].audit_action", i), Reason: "invalid audit action \"" + r.AuditAction + "\""}
		}
		if r.AuditEntityType != "" && !rePerm.MatchString(r.AuditEntityType) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.routes[%d].audit_entity_type", i), Reason: "invalid audit entity type \"" + r.AuditEntityType + "\""}
		}
		if r.AuditSeverity != "" && !KnownAuditSeverity(r.AuditSeverity) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.routes[%d].audit_severity", i), Reason: "must be info|success|warning|error, got \"" + r.AuditSeverity + "\""}
		}
	}

	for i, m := range m.Contribs.Menu {
		if !reRoutePath.MatchString(m.Path) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.menu[%d].path", i), Reason: "must start with /<slug>"}
		}
		if m.LabelKey == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.menu[%d].label_key", i), Reason: "required"}
		}
	}

	for i, s := range m.Contribs.SCIMSchemas {
		if s.URI == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.scim_schemas[%d].uri", i), Reason: "required"}
		}
		if s.Name == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.scim_schemas[%d].name", i), Reason: "required"}
		}
	}

	seenThemes := make(map[string]bool)
	for i, t := range m.Contribs.Themes {
		if !reThemeID.MatchString(t.ID) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.themes[%d].id", i), Reason: "invalid theme id \"" + t.ID + "\""}
		}
		if seenThemes[t.ID] {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.themes[%d].id", i), Reason: "duplicate theme id \"" + t.ID + "\""}
		}
		seenThemes[t.ID] = true
		if t.Name == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.themes[%d].name", i), Reason: "required"}
		}
		if t.CSSPath == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.themes[%d].css_path", i), Reason: "required"}
		}
		if strings.Contains(t.CSSPath, "..") || strings.HasPrefix(t.CSSPath, "/") {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.themes[%d].css_path", i), Reason: "must be plugin-relative"}
		}
		if t.DefaultVariant != "" && t.DefaultVariant != "dark" && t.DefaultVariant != "light" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.themes[%d].default_variant", i), Reason: "must be dark|light"}
		}
	}

	seenCatalogs := make(map[string]bool)
	for i, c := range m.Contribs.Catalogs {
		if !reCatalogID.MatchString(c.ID) {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.catalogs[%d].id", i), Reason: "invalid catalog id \"" + c.ID + "\""}
		}
		if seenCatalogs[c.ID] {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.catalogs[%d].id", i), Reason: "duplicate catalog id \"" + c.ID + "\""}
		}
		seenCatalogs[c.ID] = true
		if !knownCatalogKinds[c.Kind] {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.catalogs[%d].kind", i), Reason: "unknown catalog kind \"" + c.Kind + "\""}
		}
		if c.Name == "" {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.catalogs[%d].name", i), Reason: "required"}
		}
		// https or an absolute mount path, matching what the catalogs
		// service will accept from an operator typing the same field. Plain
		// http is refused here even though loopback is tolerated elsewhere:
		// a manifest ships to every install, so it cannot be talking about
		// somebody's local dev server.
		if !strings.HasPrefix(c.URL, "https://") && !strings.HasPrefix(c.URL, "/") {
			return &ManifestSchemaError{Field: fmt.Sprintf("contributes.catalogs[%d].url", i), Reason: "must be an https:// URL or an absolute path"}
		}
	}

	if m.Checksum != "" && !reChecksum.MatchString(m.Checksum) {
		return &ManifestSchemaError{Field: "checksum", Reason: "must be a 64-char hex SHA-256 digest"}
	}

	return nil
}

// ManifestSchemaJSON returns the embedded schema as JSON bytes. Used
// by the diagnostics route to publish the schema for admin download.
func ManifestSchemaJSON() []byte {
	out := make([]byte, len(manifestSchemaJSON))
	copy(out, manifestSchemaJSON)
	return out
}

// --- diagnostic helpers (informational, not enforced) -------------

// SchemaReport summarizes a manifest against the v1 schema. Returned
// for the /api/plugins/loader/schema endpoint so admins can debug
// plugin build issues without reading the source.
func SchemaReport(m *contract.Manifest) SchemaReportT {
	err := ValidateManifestSchema(m)
	if err == nil {
		return SchemaReportT{Valid: true, Lenient: SchemaLenient}
	}
	var f, r string
	var ms *ManifestSchemaError
	if errors.As(err, &ms) {
		f = ms.Field
		r = ms.Reason
	} else {
		r = err.Error()
	}
	return SchemaReportT{Valid: false, Lenient: SchemaLenient, Field: f, Reason: r}
}

// SchemaReportT is the wire shape returned by SchemaReport.
type SchemaReportT struct {
	Valid   bool   `json:"valid"`
	Lenient bool   `json:"lenient"`
	Field   string `json:"field,omitempty"`
	Reason  string `json:"reason,omitempty"`
}
