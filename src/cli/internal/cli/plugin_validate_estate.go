// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// pluginMaxRunningValues mirrors the host's plugincaps.maxRunningValues.
const pluginMaxRunningValues = 8

var rePluginRunningValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// validateCapEstate mirrors the host's plugincaps.validateEstateSource.
func validateCapEstate(slug string, m pluginManifest, c pluginCapDecl) error {
	if err := capToken("metric", c.Metric); err != nil {
		return err
	}
	if c.EnvironmentField == "" && !c.EnvScoped {
		return errors.New("field environment_field is required")
	}
	if c.EnvironmentField != "" && !rePluginCapField.MatchString(c.EnvironmentField) {
		return fmt.Errorf("environment_field %q is not a JSON field name", c.EnvironmentField)
	}
	route, err := capFindRoute(m, "GET", c.Path, false)
	if err != nil {
		return err
	}
	if c.LabelKey == "" {
		for _, f := range []string{"icon", "env_scoped", "running_field", "running_values", "link"} {
			if slices.Contains(c.fieldSet(), f) {
				return fmt.Errorf("%s applies only to a source that names a label_key", f)
			}
		}
		if c.I18nNS != "" {
			return errors.New("label_key and i18n_ns come together")
		}
		return nil
	}
	return validateCapEstateCard(slug, m, c, route)
}

// validateCapEstateCard mirrors the host's plugincaps.validateEstateCard.
func validateCapEstateCard(slug string, m pluginManifest, c pluginCapDecl, route pluginRouteDecl) error {
	if err := capBackupLabel(c); err != nil {
		return err
	}
	if err := capSearchIcon(c.Icon); err != nil {
		return err
	}
	if route.Public || route.Requires == "" {
		return fmt.Errorf("GET %s declares no requires_permission, so the environment card has nothing to gate on", c.Path)
	}
	if err := capOwnPermission(m, route.Requires); err != nil {
		return err
	}
	if (c.RunningField == "") != (len(c.RunningValues) == 0) {
		return errors.New("running_field and running_values come together")
	}
	if c.RunningField != "" {
		if !rePluginCapField.MatchString(c.RunningField) {
			return fmt.Errorf("running_field %q is not a JSON field name", c.RunningField)
		}
		if len(c.RunningValues) > pluginMaxRunningValues {
			return fmt.Errorf("running_values: at most %d may be declared", pluginMaxRunningValues)
		}
		for _, v := range c.RunningValues {
			if !rePluginRunningValue.MatchString(v) {
				return fmt.Errorf("running_values %q must match %s", v, rePluginRunningValue)
			}
		}
	}
	if c.Link == "" {
		return nil
	}
	if err := capSearchLink(slug, c.Link, true); err != nil {
		return err
	}
	for _, p := range rePluginCapPlace.FindAllString(c.Link, -1) {
		if p != "{environmentId}" {
			return fmt.Errorf("link %q may only carry the {environmentId} placeholder", c.Link)
		}
	}
	return nil
}
