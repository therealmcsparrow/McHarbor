// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// SearchIcons is the closed set of icon names a search source or action may name. The
// palette maps each to a component it ships, so a plugin can never make the host load one.
// Both validators and the palette's map are held to src/contracts/plugincaps/search_icons.yaml.
var SearchIcons = []string{
	"apps", "box", "box-multiple", "cloud-network", "cpu", "database", "device-desktop",
	"download", "file-upload", "key", "network", "photo", "plus", "rocket", "route",
	"server", "server-2", "server-bolt", "settings", "stack-2", "template", "topology-star-3",
}

// maxSearchEntries bounds one declaration, so a manifest cannot make the palette fan out
// to an unbounded number of list requests every time it opens.
const maxSearchEntries = 8

// MaxFanOutTargets is how many fan-out items the palette requests a source for. The
// palette enforces it; it is exported so the docs and the palette's test quote one number.
const MaxFanOutTargets = 16

// SearchFanOut makes a source one request per item of another list the plugin owns —
// one per Kubernetes cluster, say — with the item's id passed as Param. Permission is
// filled by Parse from the list route's own gate.
type SearchFanOut struct {
	Path       string `yaml:"path" json:"path"`
	Items      string `yaml:"items,omitempty" json:"items,omitempty"`
	ID         string `yaml:"id" json:"id"`
	Param      string `yaml:"param" json:"param"`
	Label      string `yaml:"label,omitempty" json:"label,omitempty"`
	Permission string `yaml:"-" json:"permission"`
}

// SearchSource is one list the palette searches while it is open. Permission and I18nNS are
// filled by Parse from the route's own gate and the capability, never read from the manifest.
type SearchSource struct {
	Key       string   `yaml:"key" json:"key"`
	LabelKey  string   `yaml:"label_key" json:"labelKey"`
	Path      string   `yaml:"path" json:"path"`
	EnvScoped bool     `yaml:"env_scoped,omitempty" json:"envScoped"`
	Items     string   `yaml:"items,omitempty" json:"items,omitempty"`
	ID        string   `yaml:"id" json:"id"`
	Name      []string `yaml:"name" json:"name"`
	Detail    []string `yaml:"detail,omitempty" json:"detail,omitempty"`
	Link      string   `yaml:"link" json:"link"`
	Icon      string   `yaml:"icon" json:"icon"`
	// FanOut, when set, repeats the request per item of another list; see SearchFanOut.
	FanOut     *SearchFanOut `yaml:"fan_out,omitempty" json:"fanOut,omitempty"`
	Permission string        `yaml:"-" json:"permission"`
	I18nNS     string        `yaml:"-" json:"i18nNs"`
}

// SearchAction is one entry in the palette's Actions group. It only navigates to the
// plugin's own page, so no plugin code runs in the host when an operator picks it.
type SearchAction struct {
	Key         string `yaml:"key" json:"key"`
	LabelKey    string `yaml:"label_key" json:"labelKey"`
	KeywordsKey string `yaml:"keywords_key,omitempty" json:"keywordsKey,omitempty"`
	Icon        string `yaml:"icon" json:"icon"`
	Link        string `yaml:"link" json:"link"`
	Permission  string `yaml:"permission" json:"permission"`
	I18nNS      string `yaml:"-" json:"i18nNs"`
}

var (
	reSearchKey    = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	reItemsPath    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}(\.[A-Za-z][A-Za-z0-9_]{0,62})*$`)
	reLinkSegment  = regexp.MustCompile(`^([A-Za-z0-9_-]|\{[A-Za-z][A-Za-z0-9_]{0,62}\})+$`)
	reLinkQueryArg = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}=([A-Za-z0-9_-]*|\{[A-Za-z][A-Za-z0-9_]{0,62}\})$`)
	reFanOutParam  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)
)

func validateSearchSource(doc *manifestDoc, c Capability) error {
	if !reNS.MatchString(c.I18nNS) {
		return fmt.Errorf("i18n_ns %q must match %s", c.I18nNS, reNS)
	}
	if len(c.Sources) > maxSearchEntries {
		return fmt.Errorf("sources: at most %d may be declared", maxSearchEntries)
	}
	seen := map[string]bool{}
	for i, s := range c.Sources {
		if err := validateOneSource(doc, s); err != nil {
			return fmt.Errorf("sources[%d] (%s): %w", i, s.Key, err)
		}
		if seen[s.Key] {
			return fmt.Errorf("sources[%d]: key %s declared twice", i, s.Key)
		}
		seen[s.Key] = true
	}
	return nil
}

func validateOneSource(doc *manifestDoc, s SearchSource) error {
	if err := token("key", s.Key); err != nil {
		return err
	}
	if !reI18nKey.MatchString(s.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", s.LabelKey)
	}
	if strings.Contains(s.Path, "{") {
		return fmt.Errorf("path %q must not carry placeholders; the palette lists without an object to fill them", s.Path)
	}
	route, err := findRoute(doc, http.MethodGet, s.Path, false)
	if err != nil {
		return err
	}
	if route.Public || route.Requires == "" {
		return fmt.Errorf("GET %s declares no requires_permission, so the palette has nothing to gate on", s.Path)
	}
	if err := ownPermission(doc, route.Requires); err != nil {
		return err
	}
	if s.Items != "" && !reItemsPath.MatchString(s.Items) {
		return fmt.Errorf("items %q is not a dotted JSON field path", s.Items)
	}
	if err := searchFields("id", []string{s.ID}, 1); err != nil {
		return err
	}
	if err := searchFields("name", s.Name, 1); err != nil {
		return err
	}
	if err := searchFields("detail", s.Detail, 0); err != nil {
		return err
	}
	if err := searchIcon(s.Icon); err != nil {
		return err
	}
	if s.FanOut != nil {
		if err := validateFanOut(doc, s); err != nil {
			return err
		}
	}
	return searchLink(doc.Slug, s.Link, true)
}

// validateFanOut holds a fan-out to the source rules: a GET list the plugin declares and
// gates with its own key, a JSON id field, and a query parameter name for that id.
func validateFanOut(doc *manifestDoc, s SearchSource) error {
	f := s.FanOut
	if s.EnvScoped {
		return errors.New("fan_out and env_scoped are exclusive; a source is repeated per environment or per item, not both")
	}
	if strings.Contains(f.Path, "{") {
		return fmt.Errorf("fan_out path %q must not carry placeholders", f.Path)
	}
	route, err := findRoute(doc, http.MethodGet, f.Path, false)
	if err != nil {
		return fmt.Errorf("fan_out: %w", err)
	}
	if route.Public || route.Requires == "" {
		return fmt.Errorf("fan_out: GET %s declares no requires_permission, so the palette has nothing to gate on", f.Path)
	}
	if err := ownPermission(doc, route.Requires); err != nil {
		return fmt.Errorf("fan_out: %w", err)
	}
	if f.Items != "" && !reItemsPath.MatchString(f.Items) {
		return fmt.Errorf("fan_out items %q is not a dotted JSON field path", f.Items)
	}
	if !reField.MatchString(f.ID) {
		return fmt.Errorf("fan_out id %q is not a JSON field name", f.ID)
	}
	if f.Label != "" && !reField.MatchString(f.Label) {
		return fmt.Errorf("fan_out label %q is not a JSON field name", f.Label)
	}
	if !reFanOutParam.MatchString(f.Param) {
		return fmt.Errorf("fan_out param %q must match %s", f.Param, reFanOutParam)
	}
	if f.Param == "env" {
		return errors.New("fan_out param env is the palette's own environment parameter")
	}
	return nil
}

func validateSearchAction(doc *manifestDoc, c Capability) error {
	if !reNS.MatchString(c.I18nNS) {
		return fmt.Errorf("i18n_ns %q must match %s", c.I18nNS, reNS)
	}
	if len(c.Actions) > maxSearchEntries {
		return fmt.Errorf("actions: at most %d may be declared", maxSearchEntries)
	}
	seen := map[string]bool{}
	for i, a := range c.Actions {
		if err := validateOneAction(doc, a); err != nil {
			return fmt.Errorf("actions[%d] (%s): %w", i, a.Key, err)
		}
		if seen[a.Key] {
			return fmt.Errorf("actions[%d]: key %s declared twice", i, a.Key)
		}
		seen[a.Key] = true
	}
	return nil
}

func validateOneAction(doc *manifestDoc, a SearchAction) error {
	if !reSearchKey.MatchString(a.Key) {
		return fmt.Errorf("key %q must match %s", a.Key, reSearchKey)
	}
	if !reI18nKey.MatchString(a.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", a.LabelKey)
	}
	if a.KeywordsKey != "" && !reI18nKey.MatchString(a.KeywordsKey) {
		return fmt.Errorf("keywords_key %q is not an i18n key", a.KeywordsKey)
	}
	if err := searchIcon(a.Icon); err != nil {
		return err
	}
	if err := ownPermission(doc, a.Permission); err != nil {
		return err
	}
	return searchLink(doc.Slug, a.Link, false)
}

// searchFields holds a list of item fields to JSON field names, between min and four.
func searchFields(field string, names []string, minimum int) error {
	if len(names) < minimum || len(names) > 4 {
		return fmt.Errorf("%s must name between %d and 4 fields", field, minimum)
	}
	for _, n := range names {
		if !reField.MatchString(n) {
			return fmt.Errorf("%s %q is not a JSON field name", field, n)
		}
	}
	return nil
}

func searchIcon(icon string) error {
	if !slices.Contains(SearchIcons, icon) {
		return fmt.Errorf("icon %q is not in the palette's icon list (known: %v)", icon, SearchIcons)
	}
	return nil
}

// searchLink holds a link to the plugin's own /plugins/<slug> page tree: no scheme, no host,
// no dot segment and no fragment. Placeholders are item fields, so only a source may use them.
func searchLink(slug, link string, placeholders bool) error {
	path, query, hasQuery := strings.Cut(link, "?")
	base := "/plugins/" + slug
	rest, ok := strings.CutPrefix(path, base)
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return fmt.Errorf("link %q must be %s or a path under it", link, base)
	}
	if rest != "" {
		for _, seg := range strings.Split(rest[1:], "/") {
			if !reLinkSegment.MatchString(seg) {
				return fmt.Errorf("link %q has a segment %q that is not a page name or a {field} placeholder", link, seg)
			}
		}
	}
	if !placeholders && strings.Contains(path, "{") {
		return errors.New("link must not carry placeholders; an action has no item to fill them from")
	}
	if hasQuery {
		for _, arg := range strings.Split(query, "&") {
			if !reLinkQueryArg.MatchString(arg) {
				return fmt.Errorf("link %q has a query argument %q that is not name=value", link, arg)
			}
		}
	}
	return nil
}

// resolveSearch stamps each source with the permission its route enforces and each entry
// with the capability's namespace, so the palette gates and translates without the manifest.
func resolveSearch(doc *manifestDoc, c *Capability) {
	for i := range c.Sources {
		if route, err := findRoute(doc, http.MethodGet, c.Sources[i].Path, false); err == nil {
			c.Sources[i].Permission = route.Requires
		}
		if f := c.Sources[i].FanOut; f != nil {
			if route, err := findRoute(doc, http.MethodGet, f.Path, false); err == nil {
				f.Permission = route.Requires
			}
		}
		c.Sources[i].I18nNS = c.I18nNS
	}
	for i := range c.Actions {
		c.Actions[i].I18nNS = c.I18nNS
	}
}
