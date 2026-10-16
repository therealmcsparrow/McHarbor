// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package plugincaps reads the facts a plugin declares under
// contributes.capabilities, so host code asks "who provides X" instead of naming a slug.
// It is host-only on purpose: no plugin links it, so changing it never breaks a built .so.
package plugincaps

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind names one capability type. The set is closed: the host has a consumer for each.
type Kind string

// The capability kinds a plugin may declare. Each has exactly one host consumer.
const (
	// KindRunbookProvider claims a runbook domain (vm, ct, compute, swarm, k8s) and
	// describes the steps and selector lists it serves: routes, params and permissions.
	KindRunbookProvider Kind = "runbook-provider"
	// KindBackupRowActions supplies the pages, routes and keys behind the row
	// actions of a plugin-owned backup source in the host's backup tables.
	KindBackupRowActions Kind = "backup-row-actions"
	// KindEstateSource contributes one metric to the dashboard's estate summary.
	KindEstateSource Kind = "estate-source"
	// KindLifecycleSubject names the page a lifecycle event's subject links to.
	KindLifecycleSubject Kind = "lifecycle-subject"
	// KindDocumentExport says the plugin can render host documents in formats.
	KindDocumentExport Kind = "document-export"
	// KindLoginEndpoint names the public route a directory provider type signs in through.
	KindLoginEndpoint Kind = "login-endpoint"
	// KindPickerSource exposes one of the plugin's runbook-provider target lists to the
	// host's generic plugin-select field, so a workflow picker never names a plugin route.
	KindPickerSource Kind = "picker-source"
	// KindWorkflowNodeAlias claims a bare workflow action a stored canvas may still carry
	// (a node the host once shipped), resolving it to the plugin's own node, or to the
	// host's generic send node bound to the plugin's own channel type.
	KindWorkflowNodeAlias Kind = "workflow-node-alias"
	// KindEmailChannel says a channel type the plugin registers delivers addressed email,
	// so host email features list its channels without naming a provider.
	KindEmailChannel Kind = "email-channel"
	// KindConsoleSubject claims a guest kind the host's console gateway may open, with the
	// console kinds it serves and the plugin's own key that gates opening one.
	KindConsoleSubject Kind = "console-subject"
	// KindAgentRPC narrows the host's agent message ceiling to the types the plugin may send
	// and the unsolicited frames it may observe; the loader's agent-rpc view enforces it.
	KindAgentRPC Kind = "agent-rpc"
	// KindRebootDrain names the plugin's own two routes a host reboot drains its guests
	// through: one listing the running guests of a subject type, one stopping one gracefully.
	KindRebootDrain Kind = "reboot-drain"
	// KindSearchSource names the plugin's own list routes the Ctrl+K palette searches while it
	// is open, with the fields each item is labelled by and the plugin page it opens.
	KindSearchSource Kind = "search-source"
	// KindSearchAction adds navigation-only entries to the palette's Actions group, each
	// gated on a key the plugin owns or consumes and pointing at one of its own pages.
	KindSearchAction Kind = "search-action"
)

// Kinds lists every known kind, for validation messages and documentation.
func Kinds() []Kind {
	return []Kind{KindRunbookProvider, KindBackupRowActions, KindEstateSource,
		KindLifecycleSubject, KindDocumentExport, KindLoginEndpoint, KindPickerSource,
		KindWorkflowNodeAlias, KindEmailChannel, KindConsoleSubject, KindAgentRPC, KindRebootDrain,
		KindSearchSource, KindSearchAction}
}

// Capability is one declared fact. Only the fields its Kind uses are set; Validate
// refuses the rest, so a consumer can trust every field it reads.
type Capability struct {
	Plugin           string            `yaml:"-" json:"plugin"`
	Kind             Kind              `yaml:"kind" json:"kind"`
	Domain           string            `yaml:"domain,omitempty" json:"domain,omitempty"`
	SourceKind       string            `yaml:"source_kind,omitempty" json:"sourceKind,omitempty"`
	SubjectType      string            `yaml:"subject_type,omitempty" json:"subjectType,omitempty"`
	Metric           string            `yaml:"metric,omitempty" json:"metric,omitempty"`
	ProviderType     string            `yaml:"provider_type,omitempty" json:"providerType,omitempty"`
	Path             string            `yaml:"path,omitempty" json:"path,omitempty"`
	Page             string            `yaml:"page,omitempty" json:"page,omitempty"`
	EnvironmentField string            `yaml:"environment_field,omitempty" json:"environmentField,omitempty"`
	Formats          []string          `yaml:"formats,omitempty" json:"formats,omitempty"`
	Permission       string            `yaml:"permission,omitempty" json:"permission,omitempty"`
	GeneratePath     string            `yaml:"generate_path,omitempty" json:"generatePath,omitempty"`
	ExportPath       string            `yaml:"export_path,omitempty" json:"exportPath,omitempty"`
	Pages            map[string]string `yaml:"pages,omitempty" json:"pages,omitempty"`
	Routes           map[string]string `yaml:"routes,omitempty" json:"routes,omitempty"`
	Permissions      map[string]string `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Target           string            `yaml:"target,omitempty" json:"target,omitempty"`
	Alias            string            `yaml:"alias,omitempty" json:"alias,omitempty"`
	Node             string            `yaml:"node,omitempty" json:"node,omitempty"`
	ChannelType      string            `yaml:"channel_type,omitempty" json:"channelType,omitempty"`
	Consoles         []string          `yaml:"consoles,omitempty" json:"consoles,omitempty"`
	Send             []string          `yaml:"send,omitempty" json:"send,omitempty"`
	Observe          []string          `yaml:"observe,omitempty" json:"observe,omitempty"`

	// LabelKey titles a backup-row-actions or estate-source entry in the host's UI; LabelNS is the
	// capability's i18n_ns copied out by Parse, because I18nNS itself never reaches the JSON.
	LabelKey string `yaml:"label_key,omitempty" json:"labelKey,omitempty"`
	LabelNS  string `yaml:"-" json:"labelNs,omitempty"`

	// Icon, EnvScoped, RunningField, RunningValues and Link are an estate-source's optional
	// environment-card presentation; they only apply together with LabelKey.
	Icon          string   `yaml:"icon,omitempty" json:"icon,omitempty"`
	EnvScoped     bool     `yaml:"env_scoped,omitempty" json:"envScoped,omitempty"`
	RunningField  string   `yaml:"running_field,omitempty" json:"runningField,omitempty"`
	RunningValues []string `yaml:"running_values,omitempty" json:"runningValues,omitempty"`
	Link          string   `yaml:"link,omitempty" json:"link,omitempty"`

	// Sources and Actions are the search kinds' entries, each resolved by Parse with the
	// capability's namespace (and a source with its route's key), so the palette reads them whole.
	Sources []SearchSource `yaml:"sources,omitempty" json:"sources,omitempty"`
	Actions []SearchAction `yaml:"actions,omitempty" json:"actions,omitempty"`

	// Picker is a picker-source's target resolved from the same manifest by Parse, so a
	// host picker reads the list descriptor without seeing the runbook engine's own fields.
	Picker *PickerList `yaml:"-" json:"picker,omitempty"`

	// I18nNS, Targets and Steps are the runbook-provider descriptors. They stay out of
	// the JSON the loader endpoint serves, because only the host's runbook engine reads them.
	I18nNS  string          `yaml:"i18n_ns,omitempty" json:"-"`
	Targets []RunbookTarget `yaml:"targets,omitempty" json:"-"`
	Steps   []RunbookStep   `yaml:"steps,omitempty" json:"-"`
}

// ErrInvalid wraps every validation failure, so the loader can tell a bad
// declaration from an unreadable file.
var ErrInvalid = errors.New("plugincaps: invalid capability")

type manifestRoute struct {
	Method   string `yaml:"method"`
	Path     string `yaml:"path"`
	Public   bool   `yaml:"public"`
	Requires string `yaml:"requires_permission"`
}

type manifestDoc struct {
	Slug            string `yaml:"slug"`
	AddsPermissions []struct {
		Key string `yaml:"key"`
	} `yaml:"adds_permissions"`
	ConsumesPermissions []string `yaml:"consumes_permissions"`
	Contributes         struct {
		Routes       []manifestRoute `yaml:"routes"`
		Nodes        []string        `yaml:"nodes"`
		Capabilities []Capability    `yaml:"capabilities"`
		Forms        []manifestForm  `yaml:"forms"`
	} `yaml:"contributes"`
}

var (
	reToken    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	reField    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	rePerm     = regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)+$`)
	rePlace    = regexp.MustCompile(`\{[^}]*\}`)
	rePathTail = regexp.MustCompile(`^(/[A-Za-z0-9_.{}-]+)*$`)
)

// Parse reads contributes.capabilities out of raw plugin.yaml bytes and validates each
// against the rest of the same manifest. slug is the installed slug; a manifest naming
// another one is refused, since every page and route check is anchored on it.
func Parse(slug string, manifestYAML []byte) ([]Capability, error) {
	var doc manifestDoc
	if err := yaml.Unmarshal(manifestYAML, &doc); err != nil {
		return nil, fmt.Errorf("plugincaps: parsing manifest: %w", err)
	}
	if doc.Slug != slug {
		return nil, fmt.Errorf("%w: manifest slug %q is not %q", ErrInvalid, doc.Slug, slug)
	}
	caps := doc.Contributes.Capabilities
	seen := map[string]bool{}
	for i := range caps {
		caps[i].Plugin = slug
		if err := validate(&doc, caps[i]); err != nil {
			return nil, fmt.Errorf("%w: contributes.capabilities[%d] (%s): %w", ErrInvalid, i, caps[i].Kind, err)
		}
		if caps[i].Kind == KindPickerSource {
			caps[i].Picker = resolvePicker(&doc, caps[i].Target)
		}
		resolveSearch(&doc, &caps[i])
		if caps[i].Kind == KindBackupRowActions || caps[i].Kind == KindEstateSource {
			caps[i].LabelNS = caps[i].I18nNS
		}
		id := string(caps[i].Kind) + "/" + caps[i].discriminator()
		if seen[id] {
			return nil, fmt.Errorf("%w: contributes.capabilities[%d]: %s declared twice", ErrInvalid, i, id)
		}
		seen[id] = true
	}
	return caps, nil
}

// discriminator is the value a lookup of this kind is keyed by.
func (c Capability) discriminator() string {
	switch c.Kind {
	case KindRunbookProvider:
		return c.Domain
	case KindBackupRowActions:
		return c.SourceKind
	case KindEstateSource:
		return c.Metric
	case KindLifecycleSubject:
		return c.SubjectType
	case KindLoginEndpoint:
		return c.ProviderType
	case KindPickerSource:
		return c.Target
	case KindWorkflowNodeAlias:
		return c.Alias
	case KindEmailChannel:
		return c.ChannelType
	case KindConsoleSubject, KindRebootDrain:
		return c.SubjectType
	default:
		return ""
	}
}

// fieldSet names the scalar and map fields a capability actually carries, so a field
// its kind does not use can be refused rather than silently ignored.
func (c Capability) fieldSet() []string {
	var out []string
	add := func(name string, set bool) {
		if set {
			out = append(out, name)
		}
	}
	add("domain", c.Domain != "")
	add("source_kind", c.SourceKind != "")
	add("subject_type", c.SubjectType != "")
	add("metric", c.Metric != "")
	add("provider_type", c.ProviderType != "")
	add("path", c.Path != "")
	add("page", c.Page != "")
	add("environment_field", c.EnvironmentField != "")
	add("formats", len(c.Formats) > 0)
	add("permission", c.Permission != "")
	add("generate_path", c.GeneratePath != "")
	add("export_path", c.ExportPath != "")
	add("pages", len(c.Pages) > 0)
	add("routes", len(c.Routes) > 0)
	add("permissions", len(c.Permissions) > 0)
	add("i18n_ns", c.I18nNS != "")
	add("targets", len(c.Targets) > 0)
	add("steps", len(c.Steps) > 0)
	add("target", c.Target != "")
	add("alias", c.Alias != "")
	add("node", c.Node != "")
	add("channel_type", c.ChannelType != "")
	add("consoles", len(c.Consoles) > 0)
	add("send", len(c.Send) > 0)
	add("observe", len(c.Observe) > 0)
	add("sources", len(c.Sources) > 0)
	add("actions", len(c.Actions) > 0)
	add("label_key", c.LabelKey != "")
	add("icon", c.Icon != "")
	add("env_scoped", c.EnvScoped)
	add("running_field", c.RunningField != "")
	add("running_values", len(c.RunningValues) > 0)
	add("link", c.Link != "")
	return out
}

var kindFields = map[Kind][]string{
	KindRunbookProvider:   {"domain"},
	KindBackupRowActions:  {"source_kind", "pages", "routes", "permissions"},
	KindEstateSource:      {"metric", "path"},
	KindLifecycleSubject:  {"subject_type", "page"},
	KindDocumentExport:    {"formats", "permission", "generate_path", "export_path"},
	KindLoginEndpoint:     {"provider_type", "path"},
	KindPickerSource:      {"target"},
	KindWorkflowNodeAlias: {"alias"},
	KindEmailChannel:      {"channel_type"},
	KindConsoleSubject:    {"subject_type", "consoles", "permission"},
	KindAgentRPC:          {"send"},
	KindRebootDrain:       {"subject_type", "routes"},
	KindSearchSource:      {"i18n_ns", "sources"},
	KindSearchAction:      {"i18n_ns", "actions"},
}

// kindOptional names the fields a kind may carry without being required to.
var kindOptional = map[Kind][]string{
	KindRunbookProvider:   {"i18n_ns", "targets", "steps"},
	KindBackupRowActions:  {"label_key", "i18n_ns"},
	KindEstateSource:      {"environment_field", "label_key", "i18n_ns", "icon", "env_scoped", "running_field", "running_values", "link"},
	KindWorkflowNodeAlias: {"node", "channel_type"},
	KindAgentRPC:          {"observe"},
}

// backupRowActions fixes the keys of each map a backup-row-actions capability carries,
// and the method each route must be declared with.
var (
	backupPages       = []string{"open", "restore", "edit_plan"}
	backupRoutes      = map[string]string{"delete_run": "DELETE", "delete_plan": "DELETE"}
	backupPermissions = []string{"view", "manage", "delete", "restore"}
)

func validate(doc *manifestDoc, c Capability) error {
	want, ok := kindFields[c.Kind]
	if !ok {
		return fmt.Errorf("unknown kind %q (known: %v)", c.Kind, Kinds())
	}
	have := c.fieldSet()
	for _, f := range have {
		if !slices.Contains(want, f) && !slices.Contains(kindOptional[c.Kind], f) {
			return fmt.Errorf("field %s does not apply to this kind", f)
		}
	}
	for _, f := range want {
		if !slices.Contains(have, f) {
			return fmt.Errorf("field %s is required", f)
		}
	}
	switch c.Kind {
	case KindRunbookProvider:
		return validateRunbookProvider(doc, c)
	case KindBackupRowActions:
		return validateBackupRowActions(doc, c)
	case KindEstateSource:
		return validateEstateSource(doc, c)
	case KindLifecycleSubject:
		if err := token("subject_type", c.SubjectType); err != nil {
			return err
		}
		return ownPage(doc.Slug, "page", c.Page)
	case KindDocumentExport:
		for _, f := range c.Formats {
			if err := token("formats", f); err != nil {
				return err
			}
		}
		if err := ownPermission(doc, c.Permission); err != nil {
			return err
		}
		if err := declaredRoute(doc, "POST", c.GeneratePath, false); err != nil {
			return fmt.Errorf("generate_path: %w", err)
		}
		if err := declaredRoute(doc, "GET", c.ExportPath, false); err != nil {
			return fmt.Errorf("export_path: %w", err)
		}
		return nil
	case KindLoginEndpoint:
		if err := token("provider_type", c.ProviderType); err != nil {
			return err
		}
		return declaredRoute(doc, "POST", c.Path, true)
	case KindPickerSource:
		return validatePickerSource(doc, c)
	case KindWorkflowNodeAlias:
		return validateNodeAlias(doc, c)
	case KindEmailChannel:
		return ownChannelType(doc, c.ChannelType)
	case KindConsoleSubject:
		return validateConsoleSubject(doc, c)
	case KindAgentRPC:
		return validateAgentRPC(c)
	case KindRebootDrain:
		return validateRebootDrain(doc, c)
	case KindSearchSource:
		return validateSearchSource(doc, c)
	case KindSearchAction:
		return validateSearchAction(doc, c)
	}
	return nil
}

func validateBackupRowActions(doc *manifestDoc, c Capability) error {
	if err := token("source_kind", c.SourceKind); err != nil {
		return err
	}
	if err := backupLabel(c); err != nil {
		return err
	}
	if err := exactKeys("pages", c.Pages, backupPages); err != nil {
		return err
	}
	for _, k := range backupPages {
		if err := ownPage(doc.Slug, "pages."+k, c.Pages[k]); err != nil {
			return err
		}
	}
	routeKeys := make([]string, 0, len(backupRoutes))
	for k := range backupRoutes {
		routeKeys = append(routeKeys, k)
	}
	if err := exactKeys("routes", c.Routes, routeKeys); err != nil {
		return err
	}
	for k, method := range backupRoutes {
		if err := declaredRoute(doc, method, c.Routes[k], false); err != nil {
			return fmt.Errorf("routes.%s: %w", k, err)
		}
	}
	if err := exactKeys("permissions", c.Permissions, backupPermissions); err != nil {
		return err
	}
	for _, k := range backupPermissions {
		if err := ownPermission(doc, c.Permissions[k]); err != nil {
			return fmt.Errorf("permissions.%s: %w", k, err)
		}
	}
	return nil
}

// backupLabel holds a backup source's label to a key and the namespace it lives in, given
// together: a key with no namespace would be looked up in the host's bundles and miss.
func backupLabel(c Capability) error {
	if (c.LabelKey == "") != (c.I18nNS == "") {
		return errors.New("label_key and i18n_ns come together")
	}
	if c.LabelKey == "" {
		return nil
	}
	if !reI18nKey.MatchString(c.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", c.LabelKey)
	}
	if !reNS.MatchString(c.I18nNS) {
		return fmt.Errorf("i18n_ns %q must match %s", c.I18nNS, reNS)
	}
	return nil
}

func token(field, v string) error {
	if !reToken.MatchString(v) {
		return fmt.Errorf("%s %q must match %s", field, v, reToken)
	}
	return nil
}

func exactKeys(field string, m map[string]string, keys []string) error {
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

// ownPage holds a page link to the plugin's own /plugins/<slug> tree, so no plugin can
// point the host's links at another plugin or at a host page.
func ownPage(slug, field, page string) error {
	base := "/plugins/" + slug
	rest, ok := strings.CutPrefix(page, base)
	if !ok || !rePathTail.MatchString(rest) {
		return fmt.Errorf("%s %q must be %s or a path under it", field, page, base)
	}
	return nil
}

// declaredRoute requires path to be one of the plugin's own declared routes, compared
// with placeholders by position, so a capability can only name a route the plugin serves.
func declaredRoute(doc *manifestDoc, method, path string, public bool) error {
	_, err := findRoute(doc, method, path, public)
	return err
}

// findRoute is declaredRoute that also hands back the manifest's own declaration, so a
// caller can hold a capability to the permission the route itself enforces.
func findRoute(doc *manifestDoc, method, path string, public bool) (manifestRoute, error) {
	if !strings.HasPrefix(path, "/api/") {
		return manifestRoute{}, fmt.Errorf("path %q must start with /api/", path)
	}
	want := rePlace.ReplaceAllString(path, "{}")
	for _, r := range doc.Contributes.Routes {
		if !strings.EqualFold(r.Method, method) || rePlace.ReplaceAllString(r.Path, "{}") != want {
			continue
		}
		if public && !r.Public {
			return manifestRoute{}, fmt.Errorf("%s %s is declared but not public", method, path)
		}
		return r, nil
	}
	return manifestRoute{}, fmt.Errorf("%s %s is not a route this plugin declares", method, path)
}

// ownPermission requires key to be one the plugin adds or consumes, so a capability
// never names a key the plugin has no stake in.
func ownPermission(doc *manifestDoc, key string) error {
	if !rePerm.MatchString(key) {
		return fmt.Errorf("permission %q is not a permission key", key)
	}
	for _, p := range doc.AddsPermissions {
		if p.Key == key {
			return nil
		}
	}
	if slices.Contains(doc.ConsumesPermissions, key) {
		return nil
	}
	return fmt.Errorf("permission %q is neither in adds_permissions nor consumes_permissions", key)
}
