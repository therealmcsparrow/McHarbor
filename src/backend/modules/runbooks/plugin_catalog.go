// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package runbooks

// pluginCall is one plugin route a catalog entry invokes. Path is the route
// exactly as the plugin's manifest declares it, with the placeholders renamed to
// the step's parameters: {vm} is the vm parameter, {$env} is the run's
// environment, and {pod.namespace} / {pod.name} split a "namespace/name" value.
type pluginCall struct {
	Method string
	Path   string

	// Query maps a query parameter to its source: "$env" for the run's
	// environment, "=value" for a literal, anything else for a step parameter.
	Query map[string]string
	Body  []bodyField

	// PickID selects one item out of a list response by its "id", using the
	// named parameter, for a plugin with a list route and no item route.
	PickID string

	// Text prints one string field of the response verbatim, for a log.
	Text string
}

// bodyField is one JSON body field and the typed step parameter it comes from.
// An empty parameter is left out, so the plugin's own default applies.
type bodyField struct {
	JSON  string
	Param string
	Type  string
}
