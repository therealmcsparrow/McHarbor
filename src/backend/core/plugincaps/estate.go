// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
)

// maxRunningValues bounds running_values, so a manifest cannot make a card compare against an
// unbounded list on every row of every poll.
const maxRunningValues = 8

var reRunningValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// validateEstateSource holds an estate-source to the dashboard contract (metric, path,
// environment_field) and, when it names a label_key, to the environment-card contract: the
// card lists the route per environment, so the route must be gated by a key the plugin owns.
func validateEstateSource(doc *manifestDoc, c Capability) error {
	if err := token("metric", c.Metric); err != nil {
		return err
	}
	if c.EnvironmentField == "" && !c.EnvScoped {
		return errors.New("field environment_field is required")
	}
	if c.EnvironmentField != "" && !reField.MatchString(c.EnvironmentField) {
		return fmt.Errorf("environment_field %q is not a JSON field name", c.EnvironmentField)
	}
	route, err := findRoute(doc, http.MethodGet, c.Path, false)
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
	return validateEstateCard(doc, c, route)
}

func validateEstateCard(doc *manifestDoc, c Capability, route manifestRoute) error {
	if err := backupLabel(c); err != nil {
		return err
	}
	if err := searchIcon(c.Icon); err != nil {
		return err
	}
	if route.Public || route.Requires == "" {
		return fmt.Errorf("GET %s declares no requires_permission, so the environment card has nothing to gate on", c.Path)
	}
	if err := ownPermission(doc, route.Requires); err != nil {
		return err
	}
	if (c.RunningField == "") != (len(c.RunningValues) == 0) {
		return errors.New("running_field and running_values come together")
	}
	if c.RunningField != "" {
		if !reField.MatchString(c.RunningField) {
			return fmt.Errorf("running_field %q is not a JSON field name", c.RunningField)
		}
		if len(c.RunningValues) > maxRunningValues {
			return fmt.Errorf("running_values: at most %d may be declared", maxRunningValues)
		}
		for _, v := range c.RunningValues {
			if !reRunningValue.MatchString(v) {
				return fmt.Errorf("running_values %q must match %s", v, reRunningValue)
			}
		}
	}
	if c.Link == "" {
		return nil
	}
	if err := searchLink(doc.Slug, c.Link, true); err != nil {
		return err
	}
	for _, m := range rePlace.FindAllString(c.Link, -1) {
		if m != "{environmentId}" {
			return fmt.Errorf("link %q may only carry the {environmentId} placeholder", c.Link)
		}
	}
	return nil
}
