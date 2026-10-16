// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugincaps

import (
	"fmt"
	"strings"
)

// RebootDrainRoutes fixes the two routes a reboot-drain names and the method of each: the
// host lists the running guests with the first and stops one gracefully with the second.
var RebootDrainRoutes = map[string]string{"running": "GET", "stop": "POST"}

// validateRebootDrain holds a drain to two of the plugin's own gated routes. The host calls
// them in-process as the reboot's requester, so a route with no key would let a reboot reach
// guests its requester may not touch, and a public one would reach them with no identity.
func validateRebootDrain(doc *manifestDoc, c Capability) error {
	if err := token("subject_type", c.SubjectType); err != nil {
		return err
	}
	keys := []string{"running", "stop"}
	if err := exactKeys("routes", c.Routes, keys); err != nil {
		return err
	}
	for _, k := range keys {
		path := c.Routes[k]
		if strings.Contains(path, "{") {
			return fmt.Errorf("routes.%s: path %q must not carry placeholders; the host passes the environment as ?env=", k, path)
		}
		route, err := findRoute(doc, RebootDrainRoutes[k], path, false)
		if err != nil {
			return fmt.Errorf("routes.%s: %w", k, err)
		}
		if route.Public {
			return fmt.Errorf("routes.%s: %s %s is public; a drain route must carry the caller's identity", k, route.Method, path)
		}
		if route.Requires == "" {
			return fmt.Errorf("routes.%s: %s %s declares no requires_permission, so nothing gates the drain", k, route.Method, path)
		}
		if err := ownPermission(doc, route.Requires); err != nil {
			return fmt.Errorf("routes.%s: %w", k, err)
		}
	}
	return nil
}

// RebootDrains returns every loaded plugin's reboot-drain declaration, one per subject type.
func (r *Registry) RebootDrains() []Capability {
	return r.OfKind(KindRebootDrain)
}
