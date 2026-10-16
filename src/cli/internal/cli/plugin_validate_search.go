// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// pluginSearchIcons mirrors the host's plugincaps.SearchIcons. Both copies are held to
// src/contracts/plugincaps/search_icons.yaml.
var pluginSearchIcons = []string{
	"apps", "box", "box-multiple", "cloud-network", "cpu", "database", "device-desktop",
	"download", "file-upload", "key", "network", "photo", "plus", "rocket", "route",
	"server", "server-2", "server-bolt", "settings", "stack-2", "template", "topology-star-3",
}

// pluginMaxSearchEntries mirrors the host's maxSearchEntries.
const pluginMaxSearchEntries = 8

// pluginSearchSource mirrors the host's plugincaps.SearchSource as the manifest writes it.
type pluginSearchSource struct {
	Key       string              `yaml:"key"`
	LabelKey  string              `yaml:"label_key"`
	Path      string              `yaml:"path"`
	EnvScoped bool                `yaml:"env_scoped"`
	Items     string              `yaml:"items"`
	ID        string              `yaml:"id"`
	Name      []string            `yaml:"name"`
	Detail    []string            `yaml:"detail"`
	Link      string              `yaml:"link"`
	Icon      string              `yaml:"icon"`
	FanOut    *pluginSearchFanOut `yaml:"fan_out"`
}

// pluginSearchFanOut mirrors the host's plugincaps.SearchFanOut.
type pluginSearchFanOut struct {
	Path  string `yaml:"path"`
	Items string `yaml:"items"`
	ID    string `yaml:"id"`
	Param string `yaml:"param"`
	Label string `yaml:"label"`
}

// pluginSearchAction mirrors the host's plugincaps.SearchAction.
type pluginSearchAction struct {
	Key         string `yaml:"key"`
	LabelKey    string `yaml:"label_key"`
	KeywordsKey string `yaml:"keywords_key"`
	Icon        string `yaml:"icon"`
	Link        string `yaml:"link"`
	Permission  string `yaml:"permission"`
}

var (
	rePluginSearchKey    = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	rePluginItemsPath    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}(\.[A-Za-z][A-Za-z0-9_]{0,62})*$`)
	rePluginLinkSegment  = regexp.MustCompile(`^([A-Za-z0-9_-]|\{[A-Za-z][A-Za-z0-9_]{0,62}\})+$`)
	rePluginLinkQueryArg = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}=([A-Za-z0-9_-]*|\{[A-Za-z][A-Za-z0-9_]{0,62}\})$`)
	rePluginFanOutParam  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)
)

// validateCapSearchSource mirrors the host's plugincaps.validateSearchSource.
func validateCapSearchSource(slug string, m pluginManifest, c pluginCapDecl) error {
	if !rePluginRunbookNS.MatchString(c.I18nNS) {
		return fmt.Errorf("i18n_ns %q must match %s", c.I18nNS, rePluginRunbookNS)
	}
	if len(c.Sources) > pluginMaxSearchEntries {
		return fmt.Errorf("sources: at most %d may be declared", pluginMaxSearchEntries)
	}
	seen := map[string]bool{}
	for i, s := range c.Sources {
		if err := validateCapOneSource(slug, m, s); err != nil {
			return fmt.Errorf("sources[%d] (%s): %w", i, s.Key, err)
		}
		if seen[s.Key] {
			return fmt.Errorf("sources[%d]: key %s declared twice", i, s.Key)
		}
		seen[s.Key] = true
	}
	return nil
}

func validateCapOneSource(slug string, m pluginManifest, s pluginSearchSource) error {
	if err := capToken("key", s.Key); err != nil {
		return err
	}
	if !rePluginRunbookI18nKey.MatchString(s.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", s.LabelKey)
	}
	if strings.Contains(s.Path, "{") {
		return fmt.Errorf("path %q must not carry placeholders; the palette lists without an object to fill them", s.Path)
	}
	route, err := capFindRoute(m, "GET", s.Path, false)
	if err != nil {
		return err
	}
	if route.Public || route.Requires == "" {
		return fmt.Errorf("GET %s declares no requires_permission, so the palette has nothing to gate on", s.Path)
	}
	if err := capOwnPermission(m, route.Requires); err != nil {
		return err
	}
	if s.Items != "" && !rePluginItemsPath.MatchString(s.Items) {
		return fmt.Errorf("items %q is not a dotted JSON field path", s.Items)
	}
	if err := capSearchFields("id", []string{s.ID}, 1); err != nil {
		return err
	}
	if err := capSearchFields("name", s.Name, 1); err != nil {
		return err
	}
	if err := capSearchFields("detail", s.Detail, 0); err != nil {
		return err
	}
	if err := capSearchIcon(s.Icon); err != nil {
		return err
	}
	if s.FanOut != nil {
		if err := validateCapFanOut(m, s); err != nil {
			return err
		}
	}
	return capSearchLink(slug, s.Link, true)
}

// validateCapFanOut mirrors the host's plugincaps.validateFanOut.
func validateCapFanOut(m pluginManifest, s pluginSearchSource) error {
	f := s.FanOut
	if s.EnvScoped {
		return errors.New("fan_out and env_scoped are exclusive; a source is repeated per environment or per item, not both")
	}
	if strings.Contains(f.Path, "{") {
		return fmt.Errorf("fan_out path %q must not carry placeholders", f.Path)
	}
	route, err := capFindRoute(m, "GET", f.Path, false)
	if err != nil {
		return fmt.Errorf("fan_out: %w", err)
	}
	if route.Public || route.Requires == "" {
		return fmt.Errorf("fan_out: GET %s declares no requires_permission, so the palette has nothing to gate on", f.Path)
	}
	if err := capOwnPermission(m, route.Requires); err != nil {
		return fmt.Errorf("fan_out: %w", err)
	}
	if f.Items != "" && !rePluginItemsPath.MatchString(f.Items) {
		return fmt.Errorf("fan_out items %q is not a dotted JSON field path", f.Items)
	}
	if err := capSearchFields("fan_out id", []string{f.ID}, 1); err != nil {
		return err
	}
	if f.Label != "" {
		if err := capSearchFields("fan_out label", []string{f.Label}, 1); err != nil {
			return err
		}
	}
	if !rePluginFanOutParam.MatchString(f.Param) {
		return fmt.Errorf("fan_out param %q must match %s", f.Param, rePluginFanOutParam)
	}
	if f.Param == "env" {
		return errors.New("fan_out param env is the palette's own environment parameter")
	}
	return nil
}

// validateCapSearchAction mirrors the host's plugincaps.validateSearchAction.
func validateCapSearchAction(slug string, m pluginManifest, c pluginCapDecl) error {
	if !rePluginRunbookNS.MatchString(c.I18nNS) {
		return fmt.Errorf("i18n_ns %q must match %s", c.I18nNS, rePluginRunbookNS)
	}
	if len(c.Actions) > pluginMaxSearchEntries {
		return fmt.Errorf("actions: at most %d may be declared", pluginMaxSearchEntries)
	}
	seen := map[string]bool{}
	for i, a := range c.Actions {
		if err := validateCapOneAction(slug, m, a); err != nil {
			return fmt.Errorf("actions[%d] (%s): %w", i, a.Key, err)
		}
		if seen[a.Key] {
			return fmt.Errorf("actions[%d]: key %s declared twice", i, a.Key)
		}
		seen[a.Key] = true
	}
	return nil
}

func validateCapOneAction(slug string, m pluginManifest, a pluginSearchAction) error {
	if !rePluginSearchKey.MatchString(a.Key) {
		return fmt.Errorf("key %q must match %s", a.Key, rePluginSearchKey)
	}
	if !rePluginRunbookI18nKey.MatchString(a.LabelKey) {
		return fmt.Errorf("label_key %q is not an i18n key", a.LabelKey)
	}
	if a.KeywordsKey != "" && !rePluginRunbookI18nKey.MatchString(a.KeywordsKey) {
		return fmt.Errorf("keywords_key %q is not an i18n key", a.KeywordsKey)
	}
	if err := capSearchIcon(a.Icon); err != nil {
		return err
	}
	if err := capOwnPermission(m, a.Permission); err != nil {
		return err
	}
	return capSearchLink(slug, a.Link, false)
}

func capSearchFields(field string, names []string, minimum int) error {
	if len(names) < minimum || len(names) > 4 {
		return fmt.Errorf("%s must name between %d and 4 fields", field, minimum)
	}
	for _, n := range names {
		if !rePluginCapField.MatchString(n) {
			return fmt.Errorf("%s %q is not a JSON field name", field, n)
		}
	}
	return nil
}

func capSearchIcon(icon string) error {
	if !slices.Contains(pluginSearchIcons, icon) {
		return fmt.Errorf("icon %q is not in the palette's icon list (known: %v)", icon, pluginSearchIcons)
	}
	return nil
}

// capSearchLink mirrors the host's plugincaps.searchLink.
func capSearchLink(slug, link string, placeholders bool) error {
	path, query, hasQuery := strings.Cut(link, "?")
	base := "/plugins/" + slug
	rest, ok := strings.CutPrefix(path, base)
	if !ok || (rest != "" && !strings.HasPrefix(rest, "/")) {
		return fmt.Errorf("link %q must be %s or a path under it", link, base)
	}
	if rest != "" {
		for _, seg := range strings.Split(rest[1:], "/") {
			if !rePluginLinkSegment.MatchString(seg) {
				return fmt.Errorf("link %q has a segment %q that is not a page name or a {field} placeholder", link, seg)
			}
		}
	}
	if !placeholders && strings.Contains(path, "{") {
		return errors.New("link must not carry placeholders; an action has no item to fill them from")
	}
	if hasQuery {
		for _, arg := range strings.Split(query, "&") {
			if !rePluginLinkQueryArg.MatchString(arg) {
				return fmt.Errorf("link %q has a query argument %q that is not name=value", link, arg)
			}
		}
	}
	return nil
}
