// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// pluginCapDecl is one entry of contributes.capabilities. It mirrors the host's
// plugincaps.Capability field for field, because the shared fixtures in
// src/contracts/plugincaps are what keep the two rule sets in step.
type pluginCapDecl struct {
	Kind             string            `yaml:"kind"`
	Domain           string            `yaml:"domain"`
	SourceKind       string            `yaml:"source_kind"`
	SubjectType      string            `yaml:"subject_type"`
	Metric           string            `yaml:"metric"`
	ProviderType     string            `yaml:"provider_type"`
	Path             string            `yaml:"path"`
	Page             string            `yaml:"page"`
	EnvironmentField string            `yaml:"environment_field"`
	Formats          []string          `yaml:"formats"`
	Permission       string            `yaml:"permission"`
	GeneratePath     string            `yaml:"generate_path"`
	ExportPath       string            `yaml:"export_path"`
	Pages            map[string]string `yaml:"pages"`
	Routes           map[string]string `yaml:"routes"`
	Permissions      map[string]string `yaml:"permissions"`
	Target           string            `yaml:"target"`
	Alias            string            `yaml:"alias"`
	Node             string            `yaml:"node"`
	ChannelType      string            `yaml:"channel_type"`
	Consoles         []string          `yaml:"consoles"`
	Send             []string          `yaml:"send"`
	Observe          []string          `yaml:"observe"`
	LabelKey         string            `yaml:"label_key"`
	Icon             string            `yaml:"icon"`
	EnvScoped        bool              `yaml:"env_scoped"`
	RunningField     string            `yaml:"running_field"`
	RunningValues    []string          `yaml:"running_values"`
	Link             string            `yaml:"link"`

	I18nNS  string                `yaml:"i18n_ns"`
	Targets []pluginRunbookTarget `yaml:"targets"`
	Steps   []pluginRunbookStep   `yaml:"steps"`
	Sources []pluginSearchSource  `yaml:"sources"`
	Actions []pluginSearchAction  `yaml:"actions"`
}

var (
	rePluginCapToken    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	rePluginCapField    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	rePluginCapPlace    = regexp.MustCompile(`\{[^}]*\}`)
	rePluginCapPathTail = regexp.MustCompile(`^(/[A-Za-z0-9_.{}-]+)*$`)
)

// pluginCapKindFields is the closed set of kinds and the fields each one uses,
// copied from the host's plugincaps.kindFields.
var pluginCapKindFields = map[string][]string{
	"runbook-provider":    {"domain"},
	"backup-row-actions":  {"source_kind", "pages", "routes", "permissions"},
	"estate-source":       {"metric", "path"},
	"lifecycle-subject":   {"subject_type", "page"},
	"document-export":     {"formats", "permission", "generate_path", "export_path"},
	"login-endpoint":      {"provider_type", "path"},
	"picker-source":       {"target"},
	"workflow-node-alias": {"alias"},
	"email-channel":       {"channel_type"},
	"console-subject":     {"subject_type", "consoles", "permission"},
	"agent-rpc":           {"send"},
	"reboot-drain":        {"subject_type", "routes"},
	"search-source":       {"i18n_ns", "sources"},
	"search-action":       {"i18n_ns", "actions"},
}

// pluginCapKindOptional names the fields a kind may carry without needing them, copied
// from the host's plugincaps.kindOptional.
var pluginCapKindOptional = map[string][]string{
	"runbook-provider":    {"i18n_ns", "targets", "steps"},
	"backup-row-actions":  {"label_key", "i18n_ns"},
	"estate-source":       {"environment_field", "label_key", "i18n_ns", "icon", "env_scoped", "running_field", "running_values", "link"},
	"workflow-node-alias": {"node", "channel_type"},
	"agent-rpc":           {"observe"},
}

var (
	pluginCapBackupPages       = []string{"open", "restore", "edit_plan"}
	pluginCapBackupRoutes      = map[string]string{"delete_run": "DELETE", "delete_plan": "DELETE"}
	pluginCapBackupPermissions = []string{"view", "manage", "delete", "restore"}
)

// validateCapabilities applies the host's contributes.capabilities rules to a
// manifest, so an entry the loader would drop is an error here instead of a
// log line after install.
func validateCapabilities(slug string, m pluginManifest, add findingSink) {
	seen := map[string]bool{}
	for i, c := range m.Contribs.Capabilities {
		if err := validateCapDecl(slug, m, c); err != nil {
			add("error", "capabilities", fmt.Sprintf("contributes.capabilities[%d] (%s): %v; the host drops the declaration whole", i, c.Kind, err))
			continue
		}
		id := c.Kind + "/" + c.discriminator()
		if seen[id] {
			add("error", "capabilities", fmt.Sprintf("contributes.capabilities[%d]: %s declared twice", i, id))
		}
		seen[id] = true
	}
}

func (c pluginCapDecl) discriminator() string {
	switch c.Kind {
	case "runbook-provider":
		return c.Domain
	case "backup-row-actions":
		return c.SourceKind
	case "estate-source":
		return c.Metric
	case "lifecycle-subject":
		return c.SubjectType
	case "login-endpoint":
		return c.ProviderType
	case "picker-source":
		return c.Target
	case "workflow-node-alias":
		return c.Alias
	case "email-channel":
		return c.ChannelType
	case "console-subject", "reboot-drain":
		return c.SubjectType
	}
	return ""
}

func (c pluginCapDecl) fieldSet() []string {
	var out []string
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"domain", c.Domain != ""}, {"source_kind", c.SourceKind != ""},
		{"subject_type", c.SubjectType != ""}, {"metric", c.Metric != ""},
		{"provider_type", c.ProviderType != ""}, {"path", c.Path != ""},
		{"page", c.Page != ""}, {"environment_field", c.EnvironmentField != ""},
		{"formats", len(c.Formats) > 0}, {"permission", c.Permission != ""},
		{"generate_path", c.GeneratePath != ""}, {"export_path", c.ExportPath != ""},
		{"pages", len(c.Pages) > 0}, {"routes", len(c.Routes) > 0},
		{"permissions", len(c.Permissions) > 0}, {"i18n_ns", c.I18nNS != ""},
		{"targets", len(c.Targets) > 0}, {"steps", len(c.Steps) > 0},
		{"target", c.Target != ""}, {"alias", c.Alias != ""}, {"node", c.Node != ""},
		{"channel_type", c.ChannelType != ""}, {"consoles", len(c.Consoles) > 0},
		{"send", len(c.Send) > 0}, {"observe", len(c.Observe) > 0},
		{"sources", len(c.Sources) > 0}, {"actions", len(c.Actions) > 0},
		{"label_key", c.LabelKey != ""},
		{"icon", c.Icon != ""}, {"env_scoped", c.EnvScoped}, {"running_field", c.RunningField != ""},
		{"running_values", len(c.RunningValues) > 0}, {"link", c.Link != ""},
	} {
		if f.set {
			out = append(out, f.name)
		}
	}
	return out
}

func validateCapDecl(slug string, m pluginManifest, c pluginCapDecl) error {
	want, ok := pluginCapKindFields[c.Kind]
	if !ok {
		known := make([]string, 0, len(pluginCapKindFields))
		for k := range pluginCapKindFields {
			known = append(known, k)
		}
		sort.Strings(known)
		return fmt.Errorf("unknown kind %q (known: %v)", c.Kind, known)
	}
	have := c.fieldSet()
	for _, f := range have {
		if !slices.Contains(want, f) && !slices.Contains(pluginCapKindOptional[c.Kind], f) {
			return fmt.Errorf("field %s does not apply to this kind", f)
		}
	}
	for _, f := range want {
		if !slices.Contains(have, f) {
			return fmt.Errorf("field %s is required", f)
		}
	}
	switch c.Kind {
	case "runbook-provider":
		return validateCapRunbook(m, c)
	case "backup-row-actions":
		return validateCapBackup(slug, m, c)
	case "estate-source":
		return validateCapEstate(slug, m, c)
	case "lifecycle-subject":
		if err := capToken("subject_type", c.SubjectType); err != nil {
			return err
		}
		return capOwnPage(slug, "page", c.Page)
	case "document-export":
		for _, f := range c.Formats {
			if err := capToken("formats", f); err != nil {
				return err
			}
		}
		if err := capOwnPermission(m, c.Permission); err != nil {
			return err
		}
		if err := capDeclaredRoute(m, "POST", c.GeneratePath, false); err != nil {
			return fmt.Errorf("generate_path: %w", err)
		}
		if err := capDeclaredRoute(m, "GET", c.ExportPath, false); err != nil {
			return fmt.Errorf("export_path: %w", err)
		}
		return nil
	case "login-endpoint":
		if err := capToken("provider_type", c.ProviderType); err != nil {
			return err
		}
		return capDeclaredRoute(m, "POST", c.Path, true)
	case "picker-source":
		return validateCapPicker(m, c)
	case "workflow-node-alias":
		return validateCapNodeAlias(m, c)
	case "email-channel":
		return capOwnChannelType(m, c.ChannelType)
	case "console-subject":
		return validateCapConsole(m, c)
	case "agent-rpc":
		return validateCapAgentRPC(c)
	case "reboot-drain":
		return validateCapRebootDrain(m, c)
	case "search-source":
		return validateCapSearchSource(slug, m, c)
	case "search-action":
		return validateCapSearchAction(slug, m, c)
	}
	return nil
}

// pluginRebootDrainRoutes mirrors the host's plugincaps.RebootDrainRoutes.
var pluginRebootDrainRoutes = map[string]string{"running": "GET", "stop": "POST"}

// validateCapRebootDrain mirrors the host's plugincaps.validateRebootDrain.
func validateCapRebootDrain(m pluginManifest, c pluginCapDecl) error {
	if err := capToken("subject_type", c.SubjectType); err != nil {
		return err
	}
	keys := []string{"running", "stop"}
	if err := capExactKeys("routes", c.Routes, keys); err != nil {
		return err
	}
	for _, k := range keys {
		path := c.Routes[k]
		if strings.Contains(path, "{") {
			return fmt.Errorf("routes.%s: path %q must not carry placeholders; the host passes the environment as ?env=", k, path)
		}
		route, err := capFindRoute(m, pluginRebootDrainRoutes[k], path, false)
		if err != nil {
			return fmt.Errorf("routes.%s: %w", k, err)
		}
		if route.Public {
			return fmt.Errorf("routes.%s: %s %s is public; a drain route must carry the caller's identity", k, route.Method, path)
		}
		if route.Requires == "" {
			return fmt.Errorf("routes.%s: %s %s declares no requires_permission, so nothing gates the drain", k, route.Method, path)
		}
		if err := capOwnPermission(m, route.Requires); err != nil {
			return fmt.Errorf("routes.%s: %w", k, err)
		}
	}
	return nil
}

// pluginConsoleKinds mirrors the host's plugincaps.ConsoleKinds, the agent's console streams.
var pluginConsoleKinds = []string{"vnc", "spice", "serial", "tty", "exec"}

// pluginTerminalConsoles marks the agent console kinds that are a byte stream a terminal can
// attach to; vnc and spice are framebuffers. It is a fact about the agent, not a plugin.
var pluginTerminalConsoles = map[string]bool{"serial": true, "tty": true, "exec": true}

// validateCapConsole mirrors the host's plugincaps.validateConsoleSubject.
func validateCapConsole(m pluginManifest, c pluginCapDecl) error {
	if err := capToken("subject_type", c.SubjectType); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, kind := range c.Consoles {
		if !slices.Contains(pluginConsoleKinds, kind) {
			return fmt.Errorf("consoles: %q is not an agent console kind (known: %v)", kind, pluginConsoleKinds)
		}
		if seen[kind] {
			return fmt.Errorf("consoles: %q is listed twice", kind)
		}
		seen[kind] = true
	}
	if !rePermissionKey.MatchString(c.Permission) {
		return fmt.Errorf("permission %q is not a permission key", c.Permission)
	}
	for _, p := range m.AddsPerms {
		if p.Key == c.Permission {
			return nil
		}
	}
	return fmt.Errorf("permission %q is not in adds_permissions", c.Permission)
}

// rePluginNodeKey mirrors the host's plugincaps.reNodeKey, the node catalog's alphabet.
var rePluginNodeKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// validateCapPicker mirrors the host's plugincaps.validatePickerSource.
func validateCapPicker(m pluginManifest, c pluginCapDecl) error {
	if err := capToken("target", c.Target); err != nil {
		return err
	}
	var target *pluginRunbookTarget
	for _, decl := range m.Contribs.Capabilities {
		if decl.Kind != "runbook-provider" {
			continue
		}
		for i := range decl.Targets {
			if decl.Targets[i].Kind == c.Target {
				target = &decl.Targets[i]
			}
		}
	}
	if target == nil {
		return fmt.Errorf("target %q is not a targets[].kind of this plugin's runbook-provider", c.Target)
	}
	route, err := capFindRoute(m, "GET", target.Path, false)
	if err != nil {
		return err
	}
	if route.Requires == "" {
		return fmt.Errorf("GET %s declares no requires_permission, so the picker has nothing to gate on", target.Path)
	}
	return capOwnPermission(m, route.Requires)
}

// validateCapNodeAlias mirrors the host's plugincaps.validateNodeAlias.
func validateCapNodeAlias(m pluginManifest, c pluginCapDecl) error {
	if !rePluginNodeKey.MatchString(c.Alias) {
		return fmt.Errorf("alias %q must match %s", c.Alias, rePluginNodeKey)
	}
	if (c.Node == "") == (c.ChannelType == "") {
		return errors.New("exactly one of node or channel_type is required")
	}
	if c.ChannelType != "" {
		return capOwnChannelType(m, c.ChannelType)
	}
	if !slices.Contains(m.Contribs.Nodes, c.Node) {
		return fmt.Errorf("node %q is not listed under contributes.nodes", c.Node)
	}
	return nil
}

// capOwnChannelType mirrors the host's plugincaps.ownChannelType.
func capOwnChannelType(m pluginManifest, channelType string) error {
	if err := capToken("channel_type", channelType); err != nil {
		return err
	}
	for _, f := range m.Contribs.Forms {
		if f.Resource == "channel" && f.Key == channelType {
			return nil
		}
	}
	return fmt.Errorf("channel_type %q is not the key of a contributes.forms entry with resource channel", channelType)
}

func validateCapBackup(slug string, m pluginManifest, c pluginCapDecl) error {
	if err := capToken("source_kind", c.SourceKind); err != nil {
		return err
	}
	if err := capBackupLabel(c); err != nil {
		return err
	}
	if err := capExactKeys("pages", c.Pages, pluginCapBackupPages); err != nil {
		return err
	}
	for _, k := range pluginCapBackupPages {
		if err := capOwnPage(slug, "pages."+k, c.Pages[k]); err != nil {
			return err
		}
	}
	routeKeys := make([]string, 0, len(pluginCapBackupRoutes))
	for k := range pluginCapBackupRoutes {
		routeKeys = append(routeKeys, k)
	}
	sort.Strings(routeKeys)
	if err := capExactKeys("routes", c.Routes, routeKeys); err != nil {
		return err
	}
	for _, k := range routeKeys {
		if err := capDeclaredRoute(m, pluginCapBackupRoutes[k], c.Routes[k], false); err != nil {
			return fmt.Errorf("routes.%s: %w", k, err)
		}
	}
	if err := capExactKeys("permissions", c.Permissions, pluginCapBackupPermissions); err != nil {
		return err
	}
	for _, k := range pluginCapBackupPermissions {
		if err := capOwnPermission(m, c.Permissions[k]); err != nil {
			return fmt.Errorf("permissions.%s: %w", k, err)
		}
	}
	return nil
}

// capBackupLabel mirrors the host's plugincaps.backupLabel.
func capBackupLabel(c pluginCapDecl) error {
	if (c.LabelKey == "") != (c.I18nNS == "") {
		return errors.New("label_key and i18n_ns come together")
	}
	if c.LabelKey == "" {
		return nil
	}
	if !rePluginRunbookI18nKey.MatchString(c.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", c.LabelKey)
	}
	if !rePluginRunbookNS.MatchString(c.I18nNS) {
		return fmt.Errorf("i18n_ns %q must match %s", c.I18nNS, rePluginRunbookNS)
	}
	return nil
}

func capToken(field, v string) error {
	if !rePluginCapToken.MatchString(v) {
		return fmt.Errorf("%s %q must match %s", field, v, rePluginCapToken)
	}
	return nil
}

func capExactKeys(field string, m map[string]string, keys []string) error {
	for k := range m {
		if !slices.Contains(keys, k) {
			return fmt.Errorf("%s.%s is not a known key (known: %v)", field, k, keys)
		}
	}
	for _, k := range keys {
		if strings.TrimSpace(m[k]) == "" {
			return fmt.Errorf("%s.%s is required", field, k)
		}
	}
	return nil
}

func capOwnPage(slug, field, page string) error {
	base := "/plugins/" + slug
	rest, ok := strings.CutPrefix(page, base)
	if !ok || !rePluginCapPathTail.MatchString(rest) {
		return fmt.Errorf("%s %q must be %s or a path under it", field, page, base)
	}
	return nil
}

func capDeclaredRoute(m pluginManifest, method, path string, public bool) error {
	_, err := capFindRoute(m, method, path, public)
	return err
}

// capFindRoute is capDeclaredRoute that also returns the manifest's own declaration, so a
// step can be held to the permission its route enforces.
func capFindRoute(m pluginManifest, method, path string, public bool) (pluginRouteDecl, error) {
	if !strings.HasPrefix(path, "/api/") {
		return pluginRouteDecl{}, fmt.Errorf("path %q must start with /api/", path)
	}
	want := rePluginCapPlace.ReplaceAllString(path, "{}")
	for _, r := range m.Contribs.Routes {
		if !strings.EqualFold(r.Method, method) || rePluginCapPlace.ReplaceAllString(r.Path, "{}") != want {
			continue
		}
		if public && !r.Public {
			return pluginRouteDecl{}, fmt.Errorf("%s %s is declared but not public", method, path)
		}
		return r, nil
	}
	return pluginRouteDecl{}, fmt.Errorf("%s %s is not a route this plugin declares", method, path)
}

func capOwnPermission(m pluginManifest, key string) error {
	if !rePermissionKey.MatchString(key) {
		return fmt.Errorf("permission %q is not a permission key", key)
	}
	for _, p := range m.AddsPerms {
		if p.Key == key {
			return nil
		}
	}
	if slices.Contains(m.ConsumesPerms, key) {
		return nil
	}
	return fmt.Errorf("permission %q is neither in adds_permissions nor consumes_permissions", key)
}
