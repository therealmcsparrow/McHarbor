// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package openapi

type operationSpec struct {
	Method      string
	Summary     string
	Description string
	OperationID string
	Tags        []string
	Parameters  []any
	RequestBody any
	Responses   map[string]any
	Security    []map[string]any
	Extensions  map[string]any
}

func buildSpec() map[string]any {
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "McHarbor API",
			"version": "1.2.1",
			"summary": "REST, SSE, and WebSocket surface for McHarbor",
			"description": "McHarbor exposes a self-hosted control plane API for Docker, Kubernetes, remote agents, workflows, notifications, store content, and operational telemetry. " +
				"Most routes return the shared { success, data?, error?, message?, code? } envelope and require either the session cookie or a Bearer API key. " +
				"Environment-scoped routes select the target runtime with ?env=<environmentId>.",
			"contact": map[string]string{
				"name": "McHarbor",
			},
		},
		"servers": []map[string]string{
			{
				"url":         "/api",
				"description": "Current McHarbor instance",
			},
		},
		"tags":  buildTags(),
		"paths": buildPaths(),
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"cookieAuth": map[string]any{
					"type": "apiKey",
					"in":   "cookie",
					"name": "mcharbor_session",
				},
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "API key",
				},
			},
			"parameters": map[string]any{
				"EnvID": map[string]any{
					"name":        "env",
					"in":          "query",
					"description": "Environment identifier. Required on runtime routes that target a Docker, agent, or Kubernetes connection.",
					"required":    false,
					"schema": map[string]any{
						"type": "string",
					},
				},
				"Page": map[string]any{
					"name":        "page",
					"in":          "query",
					"description": "1-based page number for paginated list endpoints.",
					"required":    false,
					"schema": map[string]any{
						"type":    "integer",
						"minimum": 1,
						"default": 1,
					},
				},
				"PerPage": map[string]any{
					"name":        "per_page",
					"in":          "query",
					"description": "Items per page. McHarbor caps this value at 100.",
					"required":    false,
					"schema": map[string]any{
						"type":    "integer",
						"minimum": 1,
						"maximum": 100,
						"default": 25,
					},
				},
			},
			"schemas": buildSchemas(),
		},
		"security": protectedSecurity(),
	}
}

func buildTags() []map[string]any {
	return []map[string]any{
		{"name": "system", "description": "Instance discovery, health, machine docs, and API conventions."},
		{"name": "auth", "description": "Session bootstrap, login, logout, and current-session inspection."},
		{"name": "identity", "description": "External identity provider discovery, authorization, and CRUD."},
		{"name": "api-keys", "description": "Bearer API key lifecycle for automation clients."},
		{"name": "environments", "description": "Saved Docker, agent, and Kubernetes connection records."},
		{"name": "agents", "description": "Remote agent bootstrap and agent environment administration."},
		{"name": "docker", "description": "Docker daemon system info and runtime operations."},
		{"name": "containers", "description": "Container lifecycle, inspection, files, logs, and metrics."},
		{"name": "images", "description": "Image listing, pull/import, prune, inspect, and tagging."},
		{"name": "volumes", "description": "Volume inventory and lifecycle management."},
		{"name": "networks", "description": "Network inventory and container attachment operations."},
		{"name": "stacks", "description": "Compose stack deployment, updates, logs, and stack-scoped webhooks."},
		{"name": "kubernetes", "description": "Pods, deployments, services, and namespaces."},
		{"name": "workflows", "description": "Workflow definitions, live execution, webhook triggers, and node availability."},
		{"name": "custom-nodes", "description": "Sandboxed JavaScript workflow node definitions."},
		{"name": "app-store", "description": "Bundled and synced app catalog, installs, and progress streaming."},
		{"name": "widgets", "description": "Dashboard widget definition registry and installation."},
		{"name": "notifications", "description": "Notification rules, communication channels, and in-app notifications."},
		{"name": "telemetry", "description": "Logs, events, metrics, and realtime transports."},
	}
}

func buildPaths() map[string]any {
	return map[string]any{
		"/health": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read backend health",
				Description: "Public health probe used by Docker, reverse proxies, and external uptime checks.",
				OperationID: "healthRead",
				Tags:        []string{"system"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"200": okResponse("Backend is healthy", schemaRef("HealthStatus")),
				},
			}),
		),
		"/about": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read instance metadata",
				Description: "Returns McHarbor version information, Go runtime details, platform, and selected direct dependency versions.",
				OperationID: "aboutRead",
				Tags:        []string{"system"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"200": okResponse("Instance metadata", schemaRef("AboutInfo")),
				},
			}),
		),
		"/auth/status": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read authentication bootstrap status",
				Description: "Used by the frontend to determine whether the instance still needs first-run setup and whether local sign-in is available.",
				OperationID: "authStatusRead",
				Tags:        []string{"auth"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"200": okResponse("Authentication bootstrap state", schemaRef("AuthStatus")),
				},
			}),
		),
		"/auth/login": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a session",
				Description: "Authenticates a local account and issues the McHarbor session cookie for browser-style clients.",
				OperationID: "authLogin",
				Tags:        []string{"auth"},
				Security:    publicSecurity(),
				RequestBody: jsonRequest("Credentials for a local account", schemaRef("LoginRequest"), true),
				Responses: map[string]any{
					"200": okResponse("Session established", schemaRef("SessionUser")),
					"400": errorResponse("Malformed login request"),
					"401": errorResponse("Invalid credentials"),
				},
			}),
		),
		"/auth/logout": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Destroy the current session",
				Description: "Clears the current session cookie. McHarbor exposes this route publicly so the browser can always sign out cleanly.",
				OperationID: "authLogout",
				Tags:        []string{"auth"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"200": okResponse("Session cleared", messageSchema("Logged out")),
				},
			}),
		),
		"/auth/setup": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create the first local administrator",
				Description: "Bootstrap route used exactly once on a fresh instance before any local user exists.",
				OperationID: "authSetup",
				Tags:        []string{"auth"},
				Security:    publicSecurity(),
				RequestBody: jsonRequest("Initial administrator credentials", schemaRef("SetupRequest"), true),
				Responses: map[string]any{
					"201": createdResponse("Administrator created", schemaRef("SessionUser")),
					"400": errorResponse("Malformed setup request"),
					"409": errorResponse("Instance is already initialized"),
				},
			}),
		),
		"/auth/session": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read current session context",
				Description: "Returns the authenticated user, effective permissions, and session context for the active browser or API-key principal.",
				OperationID: "authSessionRead",
				Tags:        []string{"auth"},
				Responses: map[string]any{
					"200": okResponse("Session context", schemaRef("SessionUser")),
					"401": errorResponse("Authentication required"),
				},
			}),
		),
		"/docs/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read authenticated machine docs",
				Description: "Returns this OpenAPI JSON document. Use the human-readable static API guide for concepts, examples, and transport notes.",
				OperationID: "docsRead",
				Tags:        []string{"system"},
				Responses: map[string]any{
					"200": okResponse("OpenAPI specification", map[string]any{"type": "object", "additionalProperties": true}),
					"401": errorResponse("Authentication required"),
				},
			}),
		),
		"/api-keys/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List API keys",
				Description: "Lists API keys visible to the current principal. Plaintext token values are never returned after creation.",
				OperationID: "apiKeysList",
				Tags:        []string{"api-keys"},
				Responses: map[string]any{
					"200": okResponse("API key metadata", arrayOf(schemaRef("APIKey"))),
					"401": errorResponse("Authentication required"),
					"403": errorResponse("Permission denied"),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create an API key",
				Description: "Creates a new API key for automation clients. The raw token is returned once in the response payload.",
				OperationID: "apiKeysCreate",
				Tags:        []string{"api-keys"},
				RequestBody: jsonRequest("API key creation payload", schemaRef("APIKeyCreateRequest"), true),
				Responses: map[string]any{
					"201": createdResponse("API key created", schemaRef("APIKeyCreated")),
					"400": errorResponse("Malformed API key request"),
					"403": errorResponse("Permission denied"),
				},
			}),
		),
		"/api-keys/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one API key",
				Description: "Returns metadata for a single API key record.",
				OperationID: "apiKeysRead",
				Tags:        []string{"api-keys"},
				Parameters:  []any{pathParam("id", "API key identifier")},
				Responses: map[string]any{
					"200": okResponse("API key metadata", schemaRef("APIKey")),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("API key not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Revoke an API key",
				Description: "Deletes the API key record so the token can no longer be used.",
				OperationID: "apiKeysDelete",
				Tags:        []string{"api-keys"},
				Parameters:  []any{pathParam("id", "API key identifier")},
				Responses: map[string]any{
					"204": noContentResponse("API key revoked"),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("API key not found"),
				},
			}),
		),
		"/identity-providers/enabled": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List enabled identity providers",
				Description: "Public helper endpoint for the login page so it can present available external identity providers.",
				OperationID: "identityEnabledList",
				Tags:        []string{"identity"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"200": okResponse("Enabled identity providers", arrayOf(schemaRef("IdentityProvider"))),
				},
			}),
		),
		"/identity-providers/{id}/authorize": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Start external provider authorization",
				Description: "Begins the provider-specific authorization flow, typically by redirecting the client to the upstream identity provider.",
				OperationID: "identityAuthorize",
				Tags:        []string{"identity"},
				Security:    publicSecurity(),
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				Responses: map[string]any{
					"302": redirectResponse("Redirect to the upstream identity provider"),
					"404": errorResponse("Identity provider not found"),
				},
			}),
		),
		"/identity-providers/callback": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Finish external provider sign-in",
				Description: "Completes the OIDC or external identity callback flow and establishes the McHarbor session.",
				OperationID: "identityCallback",
				Tags:        []string{"identity"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"302": redirectResponse("Redirect back into the McHarbor UI"),
					"400": errorResponse("Invalid callback parameters"),
				},
			}),
		),
		"/identity-providers/{id}/metadata": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read service-provider metadata",
				Description: "Returns the generated SAML 2.0 service-provider metadata XML for a configured SAML provider.",
				OperationID: "identityMetadataRead",
				Tags:        []string{"identity"},
				Security:    publicSecurity(),
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				Responses: map[string]any{
					"200": textResponse("SAML metadata XML"),
					"404": errorResponse("Identity provider not found"),
				},
			}),
		),
		"/identity-providers/{id}/acs": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Finish SAML 2.0 sign-in",
				Description: "Consumes a SAML assertion and establishes the McHarbor session.",
				OperationID: "identitySamlACSRead",
				Tags:        []string{"identity"},
				Security:    publicSecurity(),
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				Responses: map[string]any{
					"302": redirectResponse("Redirect back into the McHarbor UI"),
					"400": errorResponse("Invalid SAML response"),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Finish SAML 2.0 sign-in",
				Description: "Consumes a SAML assertion and establishes the McHarbor session.",
				OperationID: "identitySamlACSCreate",
				Tags:        []string{"identity"},
				Security:    publicSecurity(),
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				Responses: map[string]any{
					"302": redirectResponse("Redirect back into the McHarbor UI"),
					"400": errorResponse("Invalid SAML response"),
				},
			}),
		),
		"/identity-providers/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List identity providers",
				Description: "Administrative route for viewing configured external identity providers.",
				OperationID: "identityList",
				Tags:        []string{"identity"},
				Responses: map[string]any{
					"200": okResponse("Configured identity providers", arrayOf(schemaRef("IdentityProvider"))),
					"403": errorResponse("Permission denied"),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create an identity provider",
				Description: "Creates an external identity provider configuration used for OIDC or future federated login flows.",
				OperationID: "identityCreate",
				Tags:        []string{"identity"},
				RequestBody: jsonRequest("Identity provider definition", schemaRef("IdentityProviderWrite"), true),
				Responses: map[string]any{
					"201": createdResponse("Identity provider created", schemaRef("IdentityProvider")),
					"400": errorResponse("Malformed provider payload"),
					"403": errorResponse("Permission denied"),
				},
			}),
		),
		"/identity-providers/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one identity provider",
				Description: "Returns the stored configuration for an identity provider.",
				OperationID: "identityRead",
				Tags:        []string{"identity"},
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				Responses: map[string]any{
					"200": okResponse("Identity provider", schemaRef("IdentityProvider")),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Identity provider not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update an identity provider",
				Description: "Updates the stored provider configuration.",
				OperationID: "identityUpdate",
				Tags:        []string{"identity"},
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				RequestBody: jsonRequest("Updated provider definition", schemaRef("IdentityProviderWrite"), true),
				Responses: map[string]any{
					"200": okResponse("Identity provider updated", schemaRef("IdentityProvider")),
					"400": errorResponse("Malformed provider payload"),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Identity provider not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete an identity provider",
				Description: "Removes the provider configuration from McHarbor.",
				OperationID: "identityDelete",
				Tags:        []string{"identity"},
				Parameters:  []any{pathParam("id", "Identity provider identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Identity provider deleted"),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Identity provider not found"),
				},
			}),
		),
		"/environments/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List environments",
				Description: "Lists all saved Docker, agent, and Kubernetes environments available to the current principal.",
				OperationID: "environmentsList",
				Tags:        []string{"environments"},
				Responses: map[string]any{
					"200": okResponse("Environment records", arrayOf(schemaRef("Environment"))),
					"403": errorResponse("Permission denied"),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create an environment",
				Description: "Creates a connection record for Docker, remote agent, or Kubernetes access.",
				OperationID: "environmentsCreate",
				Tags:        []string{"environments"},
				RequestBody: jsonRequest("Environment definition", schemaRef("EnvironmentWrite"), true),
				Responses: map[string]any{
					"201": createdResponse("Environment created", schemaRef("Environment")),
					"400": errorResponse("Malformed environment payload"),
					"403": errorResponse("Permission denied"),
				},
			}),
		),
		"/environments/detect-socket": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Detect local runtime sockets",
				Description: "Returns candidate Docker or Podman sockets that can be used when creating local environments.",
				OperationID: "environmentsDetectSocket",
				Tags:        []string{"environments"},
				Responses: map[string]any{
					"200": okResponse("Detected socket candidates", arrayOf(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string"},
							"kind": map[string]any{"type": "string"},
						},
					})),
					"403": errorResponse("Permission denied"),
				},
			}),
		),
		"/environments/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one environment",
				Description: "Returns a single environment definition.",
				OperationID: "environmentsRead",
				Tags:        []string{"environments"},
				Parameters:  []any{pathParam("id", "Environment identifier")},
				Responses: map[string]any{
					"200": okResponse("Environment definition", schemaRef("Environment")),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Environment not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update an environment",
				Description: "Updates connection settings, default selection, metadata, and runtime-specific connection fields.",
				OperationID: "environmentsUpdate",
				Tags:        []string{"environments"},
				Parameters:  []any{pathParam("id", "Environment identifier")},
				RequestBody: jsonRequest("Environment definition", schemaRef("EnvironmentWrite"), true),
				Responses: map[string]any{
					"200": okResponse("Environment updated", schemaRef("Environment")),
					"400": errorResponse("Malformed environment payload"),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Environment not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete an environment",
				Description: "Removes the environment definition from McHarbor.",
				OperationID: "environmentsDelete",
				Tags:        []string{"environments"},
				Parameters:  []any{pathParam("id", "Environment identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Environment deleted"),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Environment not found"),
				},
			}),
		),
		"/environments/{id}/test": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Test environment connectivity",
				Description: "Validates the connection settings and returns runtime metadata such as Docker or Kubernetes version details.",
				OperationID: "environmentsTest",
				Tags:        []string{"environments"},
				Parameters:  []any{pathParam("id", "Environment identifier")},
				Responses: map[string]any{
					"200": okResponse("Connectivity test result", schemaRef("EnvironmentTestResult")),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Environment not found"),
				},
			}),
		),
		"/agent/ws": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Open the remote agent transport socket",
				Description: "WebSocket used by the outbound McHarbor agent. The agent authenticates with the token query parameter instead of the session cookie.",
				OperationID: "agentsWebsocket",
				Tags:        []string{"agents", "telemetry"},
				Security:    publicSecurity(),
				Parameters:  []any{queryParam("token", "Agent token used during outbound websocket bootstrap", true)},
				Responses: map[string]any{
					"101": switchingProtocolsResponse("WebSocket upgraded"),
					"401": errorResponse("Invalid or expired agent token"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "websocket",
				},
			}),
		),
		"/agent/install/{token}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read an agent install script",
				Description: "Returns a bootstrap install script for the remote agent using a short-lived install token.",
				OperationID: "agentsInstallScript",
				Tags:        []string{"agents"},
				Security:    publicSecurity(),
				Parameters:  []any{pathParam("token", "Install token")},
				Responses: map[string]any{
					"200": textResponse("Install script"),
					"404": errorResponse("Install token not found"),
				},
			}),
		),
		"/agents/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List agent-backed environments",
				Description: "Lists environments that use the outbound agent transport.",
				OperationID: "agentsList",
				Tags:        []string{"agents"},
				Responses: map[string]any{
					"200": okResponse("Agent-backed environments", arrayOf(schemaRef("AgentStatus"))),
					"403": errorResponse("Permission denied"),
				},
			}),
		),
		"/agents/{envId}/status": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one agent status",
				Description: "Returns connection status, agent version, and last-seen metadata for an agent-backed environment.",
				OperationID: "agentsStatusRead",
				Tags:        []string{"agents"},
				Parameters:  []any{pathParam("envId", "Environment identifier")},
				Responses: map[string]any{
					"200": okResponse("Agent status", schemaRef("AgentStatus")),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Environment not found"),
				},
			}),
		),
		"/agents/{envId}/regenerate-token": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Rotate an agent token",
				Description: "Generates a new long-lived agent token and invalidates the previous one.",
				OperationID: "agentsRegenerateToken",
				Tags:        []string{"agents"},
				Parameters:  []any{pathParam("envId", "Environment identifier")},
				Responses: map[string]any{
					"200": okResponse("New agent token", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"token": map[string]any{"type": "string"},
						},
						"required": []string{"token"},
					}),
					"403": errorResponse("Permission denied"),
					"404": errorResponse("Environment not found"),
				},
			}),
		),
		"/docker/info": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read Docker daemon info",
				Description: "Returns runtime information about the selected Docker environment.",
				OperationID: "dockerInfoRead",
				Tags:        []string{"docker"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Docker daemon info", schemaRef("DockerInfoSummary")),
					"400": errorResponse("Missing environment"),
					"404": errorResponse("Environment not found"),
				},
			}),
		),
		"/containers/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List containers",
				Description: "Lists containers for the selected Docker environment.",
				OperationID: "containersList",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Container summaries", arrayOf(schemaRef("ContainerSummary"))),
					"400": errorResponse("Missing environment"),
					"403": errorResponse("Permission denied"),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a container",
				Description: "Creates a new container in the selected environment from a Docker-style create payload.",
				OperationID: "containersCreate",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID")},
				RequestBody: jsonRequest("Container create payload", schemaRef("ContainerCreateRequest"), true),
				Responses: map[string]any{
					"201": createdResponse("Container created", schemaRef("ContainerSummary")),
					"400": errorResponse("Malformed create payload"),
					"403": errorResponse("Permission denied"),
				},
			}),
		),
		"/containers/stats/summary": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read summarized container stats",
				Description: "Returns compact CPU, memory, and network summaries across containers for dashboard-style views.",
				OperationID: "containersStatsSummary",
				Tags:        []string{"containers", "telemetry"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Summarized container stats", arrayOf(schemaRef("ContainerStats"))),
					"400": errorResponse("Missing environment"),
				},
			}),
		),
		"/containers/check-updates": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Check container image updates",
				Description: "Checks whether running containers in the selected environment have newer images available upstream.",
				OperationID: "containersCheckUpdates",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Update check results", arrayOf(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":        map[string]any{"type": "string"},
							"image":     map[string]any{"type": "string"},
							"hasUpdate": map[string]any{"type": "boolean"},
						},
					})),
				},
			}),
		),
		"/containers/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Inspect a container",
				Description: "Returns the full inspection payload for a single container.",
				OperationID: "containersRead",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": okResponse("Container inspection", schemaRef("ContainerSummary")),
					"404": errorResponse("Container not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a container",
				Description: "Removes the container from the selected environment.",
				OperationID: "containersDelete",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Container deleted"),
					"404": errorResponse("Container not found"),
				},
			}),
		),
		"/containers/{id}/start": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Start a container",
				Description: "Starts a stopped container in the selected environment.",
				OperationID: "containersStart",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": okResponse("Container started", schemaRef("ContainerSummary")),
					"404": errorResponse("Container not found"),
				},
			}),
		),
		"/containers/{id}/stop": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Stop a container",
				Description: "Stops a running container in the selected environment.",
				OperationID: "containersStop",
				Tags:        []string{"containers"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": okResponse("Container stopped", schemaRef("ContainerSummary")),
					"404": errorResponse("Container not found"),
				},
			}),
		),
		"/containers/{id}/logs": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read container logs",
				Description: "Returns log data for a single container using the container module route rather than the dedicated logs stream endpoint.",
				OperationID: "containersLogsRead",
				Tags:        []string{"containers", "telemetry"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": textResponse("Container logs"),
					"404": errorResponse("Container not found"),
				},
			}),
		),
		"/containers/{id}/stats": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read live container stats snapshot",
				Description: "Returns a current metrics snapshot for one container.",
				OperationID: "containersStatsRead",
				Tags:        []string{"containers", "telemetry"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": okResponse("Container stats", schemaRef("ContainerStats")),
					"404": errorResponse("Container not found"),
				},
			}),
		),
		"/images/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List images",
				Description: "Lists Docker images for the selected environment.",
				OperationID: "imagesList",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Image summaries", arrayOf(schemaRef("ImageSummary"))),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Pull an image",
				Description: "Pulls an image into the selected environment.",
				OperationID: "imagesPull",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID")},
				RequestBody: jsonRequest("Image pull request", schemaRef("ImagePullRequest"), true),
				Responses: map[string]any{
					"200": okResponse("Pull result", schemaRef("ImageSummary")),
					"400": errorResponse("Malformed pull payload"),
				},
			}),
		),
		"/images/prune": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Prune unused images",
				Description: "Deletes unused images in the selected environment.",
				OperationID: "imagesPrune",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Prune result", messageSchema("Images pruned")),
				},
			}),
		),
		"/images/import": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Import an image archive",
				Description: "Imports an image archive into the selected environment.",
				OperationID: "imagesImport",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID")},
				RequestBody: multipartRequest("Image archive upload"),
				Responses: map[string]any{
					"200": okResponse("Import result", schemaRef("ImageSummary")),
				},
			}),
		),
		"/images/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Inspect an image",
				Description: "Returns image metadata and inspection details.",
				OperationID: "imagesRead",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Image identifier")},
				Responses: map[string]any{
					"200": okResponse("Image inspection", schemaRef("ImageSummary")),
					"404": errorResponse("Image not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete an image",
				Description: "Removes an image from the selected environment.",
				OperationID: "imagesDelete",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Image identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Image deleted"),
					"404": errorResponse("Image not found"),
				},
			}),
		),
		"/images/{id}/tag": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Tag an image",
				Description: "Creates an additional tag for an existing image.",
				OperationID: "imagesTag",
				Tags:        []string{"images"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Image identifier")},
				RequestBody: jsonRequest("Tag payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"repository": map[string]any{"type": "string"},
						"tag":        map[string]any{"type": "string"},
					},
				}, true),
				Responses: map[string]any{
					"200": okResponse("Image tagged", schemaRef("ImageSummary")),
				},
			}),
		),
		"/volumes/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List volumes",
				Description: "Lists Docker volumes for the selected environment.",
				OperationID: "volumesList",
				Tags:        []string{"volumes"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Volume summaries", arrayOf(schemaRef("Volume"))),
				},
			}),
		),
		"/volumes/{name}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Inspect a volume",
				Description: "Returns details for a single Docker volume.",
				OperationID: "volumesRead",
				Tags:        []string{"volumes"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Volume name")},
				Responses: map[string]any{
					"200": okResponse("Volume details", schemaRef("Volume")),
					"404": errorResponse("Volume not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a volume",
				Description: "Deletes a Docker volume from the selected environment.",
				OperationID: "volumesDelete",
				Tags:        []string{"volumes"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Volume name")},
				Responses: map[string]any{
					"204": noContentResponse("Volume deleted"),
					"404": errorResponse("Volume not found"),
				},
			}),
		),
		"/networks/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List networks",
				Description: "Lists Docker networks for the selected environment.",
				OperationID: "networksList",
				Tags:        []string{"networks"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Network summaries", arrayOf(schemaRef("Network"))),
				},
			}),
		),
		"/networks/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Inspect a network",
				Description: "Returns details for a single Docker network.",
				OperationID: "networksRead",
				Tags:        []string{"networks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Network identifier")},
				Responses: map[string]any{
					"200": okResponse("Network details", schemaRef("Network")),
					"404": errorResponse("Network not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a network",
				Description: "Deletes a Docker network from the selected environment.",
				OperationID: "networksDelete",
				Tags:        []string{"networks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Network identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Network deleted"),
					"404": errorResponse("Network not found"),
				},
			}),
		),
		"/stacks/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List Compose stacks",
				Description: "Lists managed stacks for the selected environment.",
				OperationID: "stacksList",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Stack summaries", arrayOf(schemaRef("StackSummary"))),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a stack",
				Description: "Creates and deploys a new stack from Compose content and optional environment variables.",
				OperationID: "stacksCreate",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID")},
				RequestBody: jsonRequest("Stack deployment payload", schemaRef("StackDeployRequest"), true),
				Responses: map[string]any{
					"201": createdResponse("Stack created", schemaRef("StackSummary")),
					"400": errorResponse("Malformed stack payload"),
				},
			}),
		),
		"/stacks/links": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read a container stack link",
				Description: "Returns the persisted McHarbor link that associates a container with a Compose stack.",
				OperationID: "stacksContainerLinkRead",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), queryParam("containerId", "Container identifier", true)},
				Responses: map[string]any{
					"200": okResponse("Container stack link", nullable(schemaRef("ContainerStackLink"))),
					"400": errorResponse("Container identifier is required"),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Link a container to a stack",
				Description: "Creates or replaces the persisted McHarbor link between a container and a Compose stack.",
				OperationID: "stacksContainerLinkCreate",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID")},
				RequestBody: jsonRequest("Container stack link payload", schemaRef("LinkContainerRequest"), true),
				Responses: map[string]any{
					"200": okResponse("Container stack link", schemaRef("ContainerStackLink")),
					"400": errorResponse("Malformed container stack link payload"),
					"500": errorResponse("Container stack link failed"),
				},
			}),
		),
		"/stacks/links/{containerId}": path(
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Unlink a container from a stack",
				Description: "Removes the persisted McHarbor link between a container and a Compose stack.",
				OperationID: "stacksContainerLinkDelete",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("containerId", "Container identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Container stack link removed"),
				},
			}),
		),
		"/stacks/{name}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one stack",
				Description: "Returns the stack detail view for a single Compose project.",
				OperationID: "stacksRead",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Stack name")},
				Responses: map[string]any{
					"200": okResponse("Stack details", schemaRef("StackSummary")),
					"404": errorResponse("Stack not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update a stack",
				Description: "Updates Compose content, environment variables, or other stack settings.",
				OperationID: "stacksUpdate",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Stack name")},
				RequestBody: jsonRequest("Stack deployment payload", schemaRef("StackDeployRequest"), true),
				Responses: map[string]any{
					"200": okResponse("Stack updated", schemaRef("StackSummary")),
					"404": errorResponse("Stack not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a stack",
				Description: "Deletes the managed stack record and removes the stack from the target environment.",
				OperationID: "stacksDelete",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Stack name")},
				Responses: map[string]any{
					"204": noContentResponse("Stack deleted"),
					"404": errorResponse("Stack not found"),
				},
			}),
		),
		"/stacks/{name}/up": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Bring a stack up",
				Description: "Runs the stack startup action for an existing managed stack.",
				OperationID: "stacksUp",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Stack name")},
				Responses: map[string]any{
					"200": okResponse("Stack started", schemaRef("StackSummary")),
				},
			}),
		),
		"/stacks/{name}/down": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Bring a stack down",
				Description: "Stops stack resources and tears down the Compose project from the target environment.",
				OperationID: "stacksDown",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Stack name")},
				Responses: map[string]any{
					"200": okResponse("Stack stopped", schemaRef("StackSummary")),
				},
			}),
		),
		"/stacks/{name}/compose": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read stack Compose source",
				Description: "Returns the Compose file currently stored for the stack.",
				OperationID: "stacksComposeRead",
				Tags:        []string{"stacks"},
				Parameters:  []any{paramRef("EnvID"), pathParam("name", "Stack name")},
				Responses: map[string]any{
					"200": textResponse("Compose source"),
					"404": errorResponse("Stack not found"),
				},
			}),
		),
		"/pods/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List pods",
				Description: "Lists Kubernetes pods in the selected environment and namespace scope.",
				OperationID: "podsList",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Pod summaries", arrayOf(schemaRef("KubernetesObject"))),
				},
			}),
		),
		"/pods/{namespace}/{name}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one pod",
				Description: "Returns details for a Kubernetes pod.",
				OperationID: "podsRead",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID"), pathParam("namespace", "Kubernetes namespace"), pathParam("name", "Pod name")},
				Responses: map[string]any{
					"200": okResponse("Pod details", schemaRef("KubernetesObject")),
					"404": errorResponse("Pod not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a pod",
				Description: "Deletes a Kubernetes pod from the selected namespace.",
				OperationID: "podsDelete",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID"), pathParam("namespace", "Kubernetes namespace"), pathParam("name", "Pod name")},
				Responses: map[string]any{
					"204": noContentResponse("Pod deleted"),
					"404": errorResponse("Pod not found"),
				},
			}),
		),
		"/pods/{namespace}/{name}/logs": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read pod logs",
				Description: "Returns logs for a Kubernetes pod.",
				OperationID: "podsLogsRead",
				Tags:        []string{"kubernetes", "telemetry"},
				Parameters:  []any{paramRef("EnvID"), pathParam("namespace", "Kubernetes namespace"), pathParam("name", "Pod name")},
				Responses: map[string]any{
					"200": textResponse("Pod logs"),
					"404": errorResponse("Pod not found"),
				},
			}),
		),
		"/deployments/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List deployments",
				Description: "Lists Kubernetes deployments for the selected environment.",
				OperationID: "deploymentsList",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Deployment summaries", arrayOf(schemaRef("KubernetesObject"))),
				},
			}),
		),
		"/deployments/{namespace}/{name}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one deployment",
				Description: "Returns details for a Kubernetes deployment.",
				OperationID: "deploymentsRead",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID"), pathParam("namespace", "Kubernetes namespace"), pathParam("name", "Deployment name")},
				Responses: map[string]any{
					"200": okResponse("Deployment details", schemaRef("KubernetesObject")),
					"404": errorResponse("Deployment not found"),
				},
			}),
		),
		"/deployments/{namespace}/{name}/scale": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Scale a deployment",
				Description: "Adjusts the replica count for a Kubernetes deployment.",
				OperationID: "deploymentsScale",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID"), pathParam("namespace", "Kubernetes namespace"), pathParam("name", "Deployment name")},
				RequestBody: jsonRequest("Replica update payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"replicas": map[string]any{"type": "integer", "minimum": 0},
					},
					"required": []string{"replicas"},
				}, true),
				Responses: map[string]any{
					"200": okResponse("Deployment scaled", schemaRef("KubernetesObject")),
					"404": errorResponse("Deployment not found"),
				},
			}),
		),
		"/k8s-services/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List Kubernetes services",
				Description: "Lists Kubernetes services in the selected environment.",
				OperationID: "k8sServicesList",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Service summaries", arrayOf(schemaRef("KubernetesObject"))),
				},
			}),
		),
		"/namespaces/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List namespaces",
				Description: "Lists Kubernetes namespaces in the selected environment.",
				OperationID: "namespacesList",
				Tags:        []string{"kubernetes"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Namespaces", arrayOf(schemaRef("KubernetesObject"))),
				},
			}),
		),
		"/workflows/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List workflows",
				Description: "Lists workflow definitions available to the current principal.",
				OperationID: "workflowsList",
				Tags:        []string{"workflows"},
				Responses: map[string]any{
					"200": okResponse("Workflow definitions", arrayOf(schemaRef("Workflow"))),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a workflow",
				Description: "Creates a new workflow definition with canvas data, metadata, and optional trigger configuration.",
				OperationID: "workflowsCreate",
				Tags:        []string{"workflows"},
				RequestBody: jsonRequest("Workflow definition", schemaRef("WorkflowWrite"), true),
				Responses: map[string]any{
					"201": createdResponse("Workflow created", schemaRef("Workflow")),
					"400": errorResponse("Malformed workflow payload"),
				},
			}),
		),
		"/workflows/runs": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List workflow runs",
				Description: "Returns historical workflow execution runs with pagination.",
				OperationID: "workflowRunsList",
				Tags:        []string{"workflows"},
				Parameters:  []any{paramRef("Page"), paramRef("PerPage")},
				Responses: map[string]any{
					"200": paginatedResponse("Workflow runs", schemaRef("WorkflowRun")),
				},
			}),
		),
		"/workflows/link-outputs": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List workflow link outputs",
				Description: "Returns workflow link-out nodes that can be targeted by link-in style automation flows.",
				OperationID: "workflowLinkOutputsList",
				Tags:        []string{"workflows"},
				Responses: map[string]any{
					"200": okResponse("Link outputs", arrayOf(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"workflowId": map[string]any{"type": "string"},
							"nodeId":     map[string]any{"type": "string"},
							"label":      map[string]any{"type": "string"},
						},
					})),
				},
			}),
		),
		"/workflows/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one workflow",
				Description: "Returns the full workflow definition including canvas data and metadata.",
				OperationID: "workflowsRead",
				Tags:        []string{"workflows"},
				Parameters:  []any{pathParam("id", "Workflow identifier")},
				Responses: map[string]any{
					"200": okResponse("Workflow definition", schemaRef("Workflow")),
					"404": errorResponse("Workflow not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update a workflow",
				Description: "Updates a workflow definition, including node canvas data and metadata.",
				OperationID: "workflowsUpdate",
				Tags:        []string{"workflows"},
				Parameters:  []any{pathParam("id", "Workflow identifier")},
				RequestBody: jsonRequest("Workflow definition", schemaRef("WorkflowWrite"), true),
				Responses: map[string]any{
					"200": okResponse("Workflow updated", schemaRef("Workflow")),
					"404": errorResponse("Workflow not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a workflow",
				Description: "Deletes the workflow definition and its related runtime metadata.",
				OperationID: "workflowsDelete",
				Tags:        []string{"workflows"},
				Parameters:  []any{pathParam("id", "Workflow identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Workflow deleted"),
					"404": errorResponse("Workflow not found"),
				},
			}),
		),
		"/workflows/{id}/execute": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Execute a workflow manually",
				Description: "Starts a manual workflow run. The response streams execution events with Server-Sent Events for live node and debug updates.",
				OperationID: "workflowsExecute",
				Tags:        []string{"workflows", "telemetry"},
				Parameters:  []any{pathParam("id", "Workflow identifier")},
				RequestBody: jsonRequest("Optional manual execution payload", schemaRef("WorkflowExecuteRequest"), false),
				Responses: map[string]any{
					"200": sseResponse("Execution event stream"),
					"404": errorResponse("Workflow not found"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "sse",
				},
			}),
		),
		"/workflows/{id}/live": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Subscribe to workflow live events",
				Description: "Streams workflow execution updates, including node progress, edge traversal, and debug events.",
				OperationID: "workflowsLive",
				Tags:        []string{"workflows", "telemetry"},
				Parameters:  []any{pathParam("id", "Workflow identifier")},
				Responses: map[string]any{
					"200": sseResponse("Workflow live event stream"),
					"404": errorResponse("Workflow not found"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "sse",
				},
			}),
		),
		"/workflows/webhooks/*": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Receive an inbound workflow webhook",
				Description: "Public inbound workflow trigger endpoint. The wildcard path is resolved internally against workflow webhook definitions.",
				OperationID: "workflowsWebhookGet",
				Tags:        []string{"workflows"},
				Security:    publicSecurity(),
				Responses: map[string]any{
					"200": okResponse("Webhook trigger result", schemaRef("WorkflowMessage")),
					"404": errorResponse("Workflow webhook not found"),
				},
			}),
		),
		"/workflow-nodes/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List built-in node availability",
				Description: "Returns availability and gating state for built-in workflow nodes used in the editor and Store.",
				OperationID: "workflowNodesAvailabilityList",
				Tags:        []string{"workflows"},
				Responses: map[string]any{
					"200": okResponse("Node availability", arrayOf(schemaRef("WorkflowNodeAvailability"))),
				},
			}),
		),
		"/workflow-nodes/{key}": path(
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update built-in node availability",
				Description: "Enables or disables a built-in node definition in the Store and workflow editor.",
				OperationID: "workflowNodesAvailabilityUpdate",
				Tags:        []string{"workflows"},
				Parameters:  []any{pathParam("key", "Built-in node key")},
				RequestBody: jsonRequest("Availability toggle payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"enabled": map[string]any{"type": "boolean"},
					},
					"required": []string{"enabled"},
				}, true),
				Responses: map[string]any{
					"200": okResponse("Node availability updated", schemaRef("WorkflowNodeAvailability")),
					"404": errorResponse("Node not found"),
				},
			}),
		),
		"/custom-nodes/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List custom workflow nodes",
				Description: "Returns all custom JavaScript nodes, including their definition metadata and translations.",
				OperationID: "customNodesList",
				Tags:        []string{"custom-nodes"},
				Responses: map[string]any{
					"200": okResponse("Custom nodes", arrayOf(schemaRef("CustomNode"))),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a custom node",
				Description: "Creates a new custom workflow node with JavaScript execution logic and node metadata.",
				OperationID: "customNodesCreate",
				Tags:        []string{"custom-nodes"},
				RequestBody: jsonRequest("Custom node definition", schemaRef("CustomNodeWrite"), true),
				Responses: map[string]any{
					"201": createdResponse("Custom node created", schemaRef("CustomNode")),
				},
			}),
		),
		"/custom-nodes/test": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Test a custom node script",
				Description: "Runs a custom node script in the sandbox without saving it.",
				OperationID: "customNodesTest",
				Tags:        []string{"custom-nodes"},
				RequestBody: jsonRequest("Inline test payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":   map[string]any{"type": "string"},
						"config": map[string]any{"type": "object", "additionalProperties": true},
						"msg":    schemaRef("WorkflowMessage"),
					},
				}, true),
				Responses: map[string]any{
					"200": okResponse("Sandbox execution result", schemaRef("WorkflowMessage")),
				},
			}),
		),
		"/custom-nodes/{key}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one custom node",
				Description: "Returns the complete custom node definition and script.",
				OperationID: "customNodesRead",
				Tags:        []string{"custom-nodes"},
				Parameters:  []any{pathParam("key", "Custom node key")},
				Responses: map[string]any{
					"200": okResponse("Custom node", schemaRef("CustomNode")),
					"404": errorResponse("Custom node not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update a custom node",
				Description: "Updates the stored custom node definition and JavaScript code.",
				OperationID: "customNodesUpdate",
				Tags:        []string{"custom-nodes"},
				Parameters:  []any{pathParam("key", "Custom node key")},
				RequestBody: jsonRequest("Custom node definition", schemaRef("CustomNodeWrite"), true),
				Responses: map[string]any{
					"200": okResponse("Custom node updated", schemaRef("CustomNode")),
					"404": errorResponse("Custom node not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a custom node",
				Description: "Deletes the custom node definition from disk.",
				OperationID: "customNodesDelete",
				Tags:        []string{"custom-nodes"},
				Parameters:  []any{pathParam("key", "Custom node key")},
				Responses: map[string]any{
					"204": noContentResponse("Custom node deleted"),
					"404": errorResponse("Custom node not found"),
				},
			}),
		),
		"/app-store/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List app catalog entries",
				Description: "Lists installable app catalog entries in the Store.",
				OperationID: "appStoreList",
				Tags:        []string{"app-store"},
				Responses: map[string]any{
					"200": okResponse("App catalog entries", arrayOf(schemaRef("AppStoreItem"))),
				},
			}),
		),
		"/app-store/categories": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List app categories",
				Description: "Returns Store categories for app grouping and filtering.",
				OperationID: "appStoreCategoriesList",
				Tags:        []string{"app-store"},
				Responses: map[string]any{
					"200": okResponse("App categories", arrayOf(map[string]any{"type": "string"})),
				},
			}),
		),
		"/app-store/install": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Install an app",
				Description: "Installs a Store app into a target environment. The environment and runtime-specific values come from the request payload.",
				OperationID: "appStoreInstall",
				Tags:        []string{"app-store"},
				RequestBody: jsonRequest("App installation payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"slug":          map[string]any{"type": "string"},
						"environmentId": map[string]any{"type": "string"},
						"name":          map[string]any{"type": "string"},
						"values":        map[string]any{"type": "object", "additionalProperties": true},
					},
					"required": []string{"slug", "environmentId", "name"},
				}, true),
				Responses: map[string]any{
					"200": okResponse("App installation result", schemaRef("AppStoreItem")),
				},
			}),
		),
		"/app-store/install/stream": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Install an app with streaming progress",
				Description: "Starts an app installation and streams progress events over Server-Sent Events.",
				OperationID: "appStoreInstallStream",
				Tags:        []string{"app-store", "telemetry"},
				RequestBody: jsonRequest("App installation payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"slug":          map[string]any{"type": "string"},
						"environmentId": map[string]any{"type": "string"},
						"name":          map[string]any{"type": "string"},
						"values":        map[string]any{"type": "object", "additionalProperties": true},
					},
					"required": []string{"slug", "environmentId", "name"},
				}, true),
				Responses: map[string]any{
					"200": sseResponse("App installation progress stream"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "sse",
				},
			}),
		),
		"/widgets/definitions": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List widget definitions",
				Description: "Returns built-in and installed dashboard widget definitions.",
				OperationID: "widgetsDefinitionsList",
				Tags:        []string{"widgets"},
				Responses: map[string]any{
					"200": okResponse("Widget definitions", arrayOf(schemaRef("WidgetDefinition"))),
				},
			}),
		),
		"/widgets/": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Install a widget definition",
				Description: "Installs a dashboard widget definition into the local widget registry.",
				OperationID: "widgetsCreate",
				Tags:        []string{"widgets"},
				RequestBody: jsonRequest("Widget definition", schemaRef("WidgetDefinition"), true),
				Responses: map[string]any{
					"201": createdResponse("Widget installed", schemaRef("WidgetDefinition")),
				},
			}),
		),
		"/widgets/{key}": path(
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update a widget definition",
				Description: "Updates a previously installed widget definition.",
				OperationID: "widgetsUpdate",
				Tags:        []string{"widgets"},
				Parameters:  []any{pathParam("key", "Widget key")},
				RequestBody: jsonRequest("Widget definition", schemaRef("WidgetDefinition"), true),
				Responses: map[string]any{
					"200": okResponse("Widget updated", schemaRef("WidgetDefinition")),
					"404": errorResponse("Widget not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Uninstall a widget definition",
				Description: "Deletes an installed widget definition from the local registry.",
				OperationID: "widgetsDelete",
				Tags:        []string{"widgets"},
				Parameters:  []any{pathParam("key", "Widget key")},
				Responses: map[string]any{
					"204": noContentResponse("Widget removed"),
					"404": errorResponse("Widget not found"),
				},
			}),
		),
		"/communication-channels/capabilities": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List communication channel capabilities",
				Description: "Returns the supported outbound communication channel types and capability metadata.",
				OperationID: "communicationCapabilitiesList",
				Tags:        []string{"notifications"},
				Responses: map[string]any{
					"200": okResponse("Channel capabilities", arrayOf(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type":        map[string]any{"type": "string"},
							"displayName": map[string]any{"type": "string"},
						},
					})),
				},
			}),
		),
		"/communication-channels/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List communication channels",
				Description: "Returns configured outbound communication channels such as Slack, Discord, Teams, Gotify, ntfy, or internal routing.",
				OperationID: "communicationChannelsList",
				Tags:        []string{"notifications"},
				Responses: map[string]any{
					"200": okResponse("Communication channels", arrayOf(schemaRef("CommunicationChannel"))),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a communication channel",
				Description: "Creates a new outbound communication channel configuration.",
				OperationID: "communicationChannelsCreate",
				Tags:        []string{"notifications"},
				RequestBody: jsonRequest("Communication channel", schemaRef("CommunicationChannelWrite"), true),
				Responses: map[string]any{
					"201": createdResponse("Communication channel created", schemaRef("CommunicationChannel")),
				},
			}),
		),
		"/communication-channels/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one communication channel",
				Description: "Returns a single communication channel configuration.",
				OperationID: "communicationChannelsRead",
				Tags:        []string{"notifications"},
				Parameters:  []any{pathParam("id", "Communication channel identifier")},
				Responses: map[string]any{
					"200": okResponse("Communication channel", schemaRef("CommunicationChannel")),
					"404": errorResponse("Communication channel not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update a communication channel",
				Description: "Updates an existing communication channel configuration.",
				OperationID: "communicationChannelsUpdate",
				Tags:        []string{"notifications"},
				Parameters:  []any{pathParam("id", "Communication channel identifier")},
				RequestBody: jsonRequest("Communication channel", schemaRef("CommunicationChannelWrite"), true),
				Responses: map[string]any{
					"200": okResponse("Communication channel updated", schemaRef("CommunicationChannel")),
					"404": errorResponse("Communication channel not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a communication channel",
				Description: "Removes a communication channel from the outbound notification configuration.",
				OperationID: "communicationChannelsDelete",
				Tags:        []string{"notifications"},
				Parameters:  []any{pathParam("id", "Communication channel identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Communication channel deleted"),
					"404": errorResponse("Communication channel not found"),
				},
			}),
		),
		"/notifications/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List notification rules",
				Description: "Lists configured notification rules or routes.",
				OperationID: "notificationsList",
				Tags:        []string{"notifications"},
				Responses: map[string]any{
					"200": okResponse("Notification rules", arrayOf(schemaRef("NotificationRule"))),
				},
			}),
			op(operationSpec{
				Method:      "POST",
				Summary:     "Create a notification rule",
				Description: "Creates a new notification rule that targets channels or downstream transports.",
				OperationID: "notificationsCreate",
				Tags:        []string{"notifications"},
				RequestBody: jsonRequest("Notification rule", schemaRef("NotificationRuleWrite"), true),
				Responses: map[string]any{
					"201": createdResponse("Notification rule created", schemaRef("NotificationRule")),
				},
			}),
		),
		"/notifications/configured-types": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List configured notification types",
				Description: "Returns which notification transport types are currently configured and available.",
				OperationID: "notificationsConfiguredTypes",
				Tags:        []string{"notifications"},
				Responses: map[string]any{
					"200": okResponse("Configured notification types", arrayOf(map[string]any{"type": "string"})),
				},
			}),
		),
		"/notifications/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read one notification rule",
				Description: "Returns a single notification rule or route definition.",
				OperationID: "notificationsRead",
				Tags:        []string{"notifications"},
				Parameters:  []any{pathParam("id", "Notification rule identifier")},
				Responses: map[string]any{
					"200": okResponse("Notification rule", schemaRef("NotificationRule")),
					"404": errorResponse("Notification rule not found"),
				},
			}),
			op(operationSpec{
				Method:      "PUT",
				Summary:     "Update a notification rule",
				Description: "Updates a stored notification rule.",
				OperationID: "notificationsUpdate",
				Tags:        []string{"notifications"},
				Parameters:  []any{pathParam("id", "Notification rule identifier")},
				RequestBody: jsonRequest("Notification rule", schemaRef("NotificationRuleWrite"), true),
				Responses: map[string]any{
					"200": okResponse("Notification rule updated", schemaRef("NotificationRule")),
					"404": errorResponse("Notification rule not found"),
				},
			}),
			op(operationSpec{
				Method:      "DELETE",
				Summary:     "Delete a notification rule",
				Description: "Deletes a notification rule or route definition.",
				OperationID: "notificationsDelete",
				Tags:        []string{"notifications"},
				Parameters:  []any{pathParam("id", "Notification rule identifier")},
				Responses: map[string]any{
					"204": noContentResponse("Notification rule deleted"),
					"404": errorResponse("Notification rule not found"),
				},
			}),
		),
		"/in-app-notifications/": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "List in-app notifications",
				Description: "Returns notifications visible in the McHarbor UI notification center for the current user.",
				OperationID: "inAppNotificationsList",
				Tags:        []string{"notifications"},
				Parameters:  []any{paramRef("Page"), paramRef("PerPage")},
				Responses: map[string]any{
					"200": paginatedResponse("In-app notifications", schemaRef("InAppNotification")),
				},
			}),
		),
		"/in-app-notifications/unread-count": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read unread in-app notification count",
				Description: "Returns the current unread notification count for the signed-in user.",
				OperationID: "inAppNotificationsUnreadCount",
				Tags:        []string{"notifications"},
				Responses: map[string]any{
					"200": okResponse("Unread notification count", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"count": map[string]any{"type": "integer", "minimum": 0},
						},
						"required": []string{"count"},
					}),
				},
			}),
		),
		"/in-app-notifications/read-all": path(
			op(operationSpec{
				Method:      "POST",
				Summary:     "Mark all in-app notifications as read",
				Description: "Marks all visible notifications as read for the current user.",
				OperationID: "inAppNotificationsReadAll",
				Tags:        []string{"notifications"},
				Responses: map[string]any{
					"200": okResponse("Notifications marked as read", messageSchema("Notifications updated")),
				},
			}),
		),
		"/events/stream": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Subscribe to environment events",
				Description: "Server-Sent Events stream for runtime events from the selected environment.",
				OperationID: "eventsStream",
				Tags:        []string{"telemetry"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": sseResponse("Runtime event stream"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "sse",
				},
			}),
		),
		"/logs/{id}": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Stream logs",
				Description: "Dedicated Server-Sent Events endpoint for container logs.",
				OperationID: "logsStream",
				Tags:        []string{"telemetry"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": sseResponse("Container log stream"),
					"404": errorResponse("Container not found"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "sse",
				},
			}),
		),
		"/metrics/host": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read host metrics snapshot",
				Description: "Returns a point-in-time metrics snapshot for the selected environment host.",
				OperationID: "metricsHostRead",
				Tags:        []string{"telemetry"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Host metrics", schemaRef("MetricsSnapshot")),
				},
			}),
		),
		"/metrics/containers": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Read container metrics snapshot",
				Description: "Returns current metrics across containers in the selected environment.",
				OperationID: "metricsContainersRead",
				Tags:        []string{"telemetry"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"200": okResponse("Container metrics", arrayOf(schemaRef("ContainerStats"))),
				},
			}),
		),
		"/metrics/containers/{id}/stream": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Subscribe to container metrics",
				Description: "Server-Sent Events stream for live container metrics.",
				OperationID: "metricsContainersStream",
				Tags:        []string{"telemetry"},
				Parameters:  []any{paramRef("EnvID"), pathParam("id", "Container identifier")},
				Responses: map[string]any{
					"200": sseResponse("Container metrics stream"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "sse",
				},
			}),
		),
		"/terminal/ws": path(
			op(operationSpec{
				Method:      "GET",
				Summary:     "Open the browser terminal websocket",
				Description: "Interactive exec terminal transport used by the McHarbor browser terminal.",
				OperationID: "terminalWebsocket",
				Tags:        []string{"telemetry"},
				Parameters:  []any{paramRef("EnvID")},
				Responses: map[string]any{
					"101": switchingProtocolsResponse("WebSocket upgraded"),
					"400": errorResponse("Missing environment or container context"),
				},
				Extensions: map[string]any{
					"x-mcharbor-transport": "websocket",
				},
			}),
		),
	}
}

func buildSchemas() map[string]any {
	return map[string]any{
		"HealthStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string", "example": "ok"},
			},
			"required": []string{"status"},
		},
		"AboutInfo": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version":   map[string]any{"type": "string"},
				"goVersion": map[string]any{"type": "string"},
				"platform":  map[string]any{"type": "string"},
				"dependencies": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":    map[string]any{"type": "string"},
							"version": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
		"AuthStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"configured":   map[string]any{"type": "boolean"},
				"requiresAuth": map[string]any{"type": "boolean"},
				"setupRequired": map[string]any{
					"type": "boolean",
				},
			},
		},
		"SessionUser": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":          map[string]any{"type": "string"},
				"username":    map[string]any{"type": "string"},
				"email":       map[string]any{"type": "string"},
				"displayName": map[string]any{"type": "string"},
				"role":        map[string]any{"type": "string"},
				"permissions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		"LoginRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{"type": "string"},
				"password": map[string]any{"type": "string", "format": "password"},
			},
			"required": []string{"username", "password"},
		},
		"SetupRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username":        map[string]any{"type": "string"},
				"password":        map[string]any{"type": "string", "format": "password"},
				"confirmPassword": map[string]any{"type": "string", "format": "password"},
				"email":           map[string]any{"type": "string"},
			},
			"required": []string{"username", "password"},
		},
		"APIKey": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
				"prefix":     map[string]any{"type": "string"},
				"scopes":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"createdAt":  map[string]any{"type": "string", "format": "date-time"},
				"lastUsedAt": map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
				"expiresAt":  map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
			},
		},
		"APIKeyCreateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string"},
				"scopes":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"expiresAt": map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
			},
			"required": []string{"name"},
		},
		"APIKeyCreated": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"token": map[string]any{"type": "string"},
				"key":   schemaRef("APIKey"),
			},
		},
		"IdentityProvider": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string"},
				"name":         map[string]any{"type": "string"},
				"providerType": map[string]any{"type": "string", "enum": []string{"entra_id", "google", "generic_oidc", "saml_2_0"}},
				"enabled":      map[string]any{"type": "boolean"},
				"tenantId":     map[string]any{"type": []string{"string", "null"}},
				"domain":       map[string]any{"type": []string{"string", "null"}},
				"issuerUrl":    map[string]any{"type": []string{"string", "null"}, "format": "uri"},
				"metadataUrl":  map[string]any{"type": []string{"string", "null"}, "format": "uri"},
				"entityId":     map[string]any{"type": []string{"string", "null"}},
				"clientId":     map[string]any{"type": "string"},
				"scopes":       map[string]any{"type": "string"},
			},
		},
		"IdentityProviderWrite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"providerType": map[string]any{"type": "string", "enum": []string{"entra_id", "google", "generic_oidc", "saml_2_0"}},
				"enabled":      map[string]any{"type": "boolean"},
				"tenantId":     map[string]any{"type": "string"},
				"domain":       map[string]any{"type": "string"},
				"issuerUrl":    map[string]any{"type": "string", "format": "uri"},
				"metadataUrl":  map[string]any{"type": "string", "format": "uri"},
				"entityId":     map[string]any{"type": "string"},
				"clientId":     map[string]any{"type": "string"},
				"clientSecret": map[string]any{"type": "string", "format": "password"},
				"scopes":       map[string]any{"type": "string"},
			},
			"required": []string{"name", "providerType"},
		},
		"Environment": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":               map[string]any{"type": "string"},
				"name":             map[string]any{"type": "string"},
				"orchestratorType": map[string]any{"type": "string", "enum": []string{"docker", "kubernetes"}},
				"connectionType":   map[string]any{"type": "string"},
				"isDefault":        map[string]any{"type": "boolean"},
				"status":           map[string]any{"type": "string"},
			},
		},
		"EnvironmentWrite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":             map[string]any{"type": "string"},
				"orchestratorType": map[string]any{"type": "string", "enum": []string{"docker", "kubernetes"}},
				"connectionType":   map[string]any{"type": "string"},
				"socketPath":       map[string]any{"type": "string"},
				"host":             map[string]any{"type": "string"},
				"tls":              map[string]any{"type": "boolean"},
				"kubeconfig":       map[string]any{"type": "string"},
				"k8sNamespace":     map[string]any{"type": "string"},
				"isDefault":        map[string]any{"type": "boolean"},
			},
			"required": []string{"name", "orchestratorType", "connectionType"},
		},
		"EnvironmentTestResult": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ok":      map[string]any{"type": "boolean"},
				"message": map[string]any{"type": "string"},
				"version": map[string]any{"type": "string"},
			},
		},
		"AgentStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"environmentId": map[string]any{"type": "string"},
				"connected":     map[string]any{"type": "boolean"},
				"version":       map[string]any{"type": "string"},
				"lastSeenAt":    map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
			},
		},
		"DockerInfoSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string"},
				"name":            map[string]any{"type": "string"},
				"serverVersion":   map[string]any{"type": "string"},
				"operatingSystem": map[string]any{"type": "string"},
				"containers":      map[string]any{"type": "integer"},
			},
		},
		"ContainerSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string"},
				"name":          map[string]any{"type": "string"},
				"image":         map[string]any{"type": "string"},
				"state":         map[string]any{"type": "string"},
				"status":        map[string]any{"type": "string"},
				"environmentId": map[string]any{"type": "string"},
			},
		},
		"ContainerCreateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string"},
				"image": map[string]any{"type": "string"},
				"cmd":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"env":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
			},
			"required": []string{"image"},
		},
		"ContainerStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cpuPercent":  map[string]any{"type": "number"},
				"memoryUsage": map[string]any{"type": "integer"},
				"memoryLimit": map[string]any{"type": "integer"},
				"rxBytes":     map[string]any{"type": "integer"},
				"txBytes":     map[string]any{"type": "integer"},
			},
		},
		"ImageSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":       map[string]any{"type": "string"},
				"repoTags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"size":     map[string]any{"type": "integer"},
				"createdAt": map[string]any{
					"type":   "string",
					"format": "date-time",
				},
			},
		},
		"ImagePullRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image":      map[string]any{"type": "string"},
				"tag":        map[string]any{"type": "string"},
				"registryId": map[string]any{"type": "string"},
			},
			"required": []string{"image"},
		},
		"Volume": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string"},
				"driver":     map[string]any{"type": "string"},
				"mountpoint": map[string]any{"type": "string"},
				"scope":      map[string]any{"type": "string"},
			},
		},
		"Network": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":     map[string]any{"type": "string"},
				"name":   map[string]any{"type": "string"},
				"driver": map[string]any{"type": "string"},
				"scope":  map[string]any{"type": "string"},
			},
		},
		"StackSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":          map[string]any{"type": "string"},
				"environmentId": map[string]any{"type": "string"},
				"status":        map[string]any{"type": "string"},
				"services":      map[string]any{"type": "integer"},
				"updatedAt":     map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"StackDeployRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":             map[string]any{"type": "string"},
				"compose":          map[string]any{"type": "string"},
				"env":              map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				"projectDirectory": map[string]any{"type": "string"},
			},
			"required": []string{"name", "compose"},
		},
		"ContainerStackLink": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string"},
				"environmentId": map[string]any{"type": "string"},
				"containerId":   map[string]any{"type": "string"},
				"stackName":     map[string]any{"type": "string"},
				"serviceName":   map[string]any{"type": "string"},
				"createdAt":     map[string]any{"type": "string", "format": "date-time"},
				"updatedAt":     map[string]any{"type": "string", "format": "date-time"},
			},
			"required": []string{"id", "environmentId", "containerId", "stackName", "createdAt", "updatedAt"},
		},
		"LinkContainerRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"containerId": map[string]any{"type": "string"},
				"stackName":   map[string]any{"type": "string"},
				"serviceName": map[string]any{"type": "string"},
			},
			"required": []string{"containerId", "stackName"},
		},
		"KubernetesObject": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      map[string]any{"type": "string"},
				"namespace": map[string]any{"type": "string"},
				"kind":      map[string]any{"type": "string"},
				"status":    map[string]any{"type": "string"},
			},
		},
		"Workflow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"name":       map[string]any{"type": "string"},
				"enabled":    map[string]any{"type": "boolean"},
				"canvasData": map[string]any{"type": "object", "additionalProperties": true},
				"updatedAt":  map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"WorkflowWrite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string"},
				"enabled":    map[string]any{"type": "boolean"},
				"canvasData": map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"name"},
		},
		"WorkflowRun": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string"},
				"workflowId": map[string]any{"type": "string"},
				"status":     map[string]any{"type": "string"},
				"startedAt":  map[string]any{"type": "string", "format": "date-time"},
				"finishedAt": map[string]any{"type": []string{"string", "null"}, "format": "date-time"},
			},
		},
		"WorkflowExecuteRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"triggerNodeId": map[string]any{"type": "string"},
				"input":         map[string]any{"type": "object", "additionalProperties": true},
			},
		},
		"WorkflowMessage": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"_msgid":       map[string]any{"type": "string"},
				"topic":        map[string]any{"type": "string"},
				"payload":      map[string]any{},
				"req":          map[string]any{"type": "object", "additionalProperties": true},
				"statusCode":   map[string]any{"type": "integer"},
				"headers":      map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				"parts":        map[string]any{"type": "object", "additionalProperties": true},
				"_performance": map[string]any{"type": "object", "additionalProperties": true},
			},
		},
		"WorkflowNodeAvailability": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":     map[string]any{"type": "string"},
				"enabled": map[string]any{"type": "boolean"},
				"reason":  map[string]any{"type": "string"},
			},
		},
		"CustomNode": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":          map[string]any{"type": "string"},
				"label":        map[string]any{"type": "string"},
				"category":     map[string]any{"type": "string"},
				"execute":      map[string]any{"type": "string"},
				"configSchema": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": true}},
			},
		},
		"CustomNodeWrite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":          map[string]any{"type": "string"},
				"label":        map[string]any{"type": "string"},
				"category":     map[string]any{"type": "string"},
				"execute":      map[string]any{"type": "string"},
				"configSchema": map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": true}},
				"translations": map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"key", "label", "execute"},
		},
		"AppStoreItem": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug":          map[string]any{"type": "string"},
				"name":          map[string]any{"type": "string"},
				"category":      map[string]any{"type": "string"},
				"description":   map[string]any{"type": "string"},
				"latestVersion": map[string]any{"type": "string"},
				"installed":     map[string]any{"type": "boolean"},
			},
		},
		"WidgetDefinition": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":      map[string]any{"type": "string"},
				"label":    map[string]any{"type": "string"},
				"category": map[string]any{"type": "string"},
				"version":  map[string]any{"type": "string"},
			},
		},
		"CommunicationChannel": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":        map[string]any{"type": "string"},
				"name":      map[string]any{"type": "string"},
				"type":      map[string]any{"type": "string"},
				"enabled":   map[string]any{"type": "boolean"},
				"isDefault": map[string]any{"type": "boolean"},
			},
		},
		"CommunicationChannelWrite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":     map[string]any{"type": "string"},
				"type":     map[string]any{"type": "string"},
				"enabled":  map[string]any{"type": "boolean"},
				"settings": map[string]any{"type": "object", "additionalProperties": true},
			},
			"required": []string{"name", "type"},
		},
		"NotificationRule": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "string"},
				"name":    map[string]any{"type": "string"},
				"event":   map[string]any{"type": "string"},
				"enabled": map[string]any{"type": "boolean"},
				"target":  map[string]any{"type": "string"},
			},
		},
		"NotificationRuleWrite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string"},
				"event":   map[string]any{"type": "string"},
				"enabled": map[string]any{"type": "boolean"},
				"conditions": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
				"target": map[string]any{"type": "string"},
			},
			"required": []string{"name", "event"},
		},
		"InAppNotification": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":        map[string]any{"type": "string"},
				"title":     map[string]any{"type": "string"},
				"message":   map[string]any{"type": "string"},
				"read":      map[string]any{"type": "boolean"},
				"createdAt": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"MetricsSnapshot": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cpuPercent":  map[string]any{"type": "number"},
				"memoryUsed":  map[string]any{"type": "integer"},
				"memoryTotal": map[string]any{"type": "integer"},
				"diskUsed":    map[string]any{"type": "integer"},
				"diskTotal":   map[string]any{"type": "integer"},
			},
		},
	}
}

func path(ops ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, item := range ops {
		for key, value := range item {
			out[key] = value
		}
	}
	return out
}

func op(spec operationSpec) map[string]any {
	body := map[string]any{
		"summary":     spec.Summary,
		"description": spec.Description,
		"operationId": spec.OperationID,
		"tags":        spec.Tags,
		"responses":   spec.Responses,
	}
	if len(spec.Parameters) > 0 {
		body["parameters"] = spec.Parameters
	}
	if spec.RequestBody != nil {
		body["requestBody"] = spec.RequestBody
	}
	if spec.Security != nil {
		body["security"] = spec.Security
	}
	for key, value := range spec.Extensions {
		body[key] = value
	}
	return map[string]any{
		methodKey(spec.Method): body,
	}
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func nullable(schema any) map[string]any {
	return map[string]any{
		"oneOf": []any{
			schema,
			map[string]any{"type": "null"},
		},
	}
}

func paramRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/parameters/" + name}
}

func pathParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"description": description,
		"required":    true,
		"schema": map[string]any{
			"type": "string",
		},
	}
}

func queryParam(name, description string, required bool) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"description": description,
		"required":    required,
		"schema": map[string]any{
			"type": "string",
		},
	}
}

func jsonRequest(description string, schema any, required bool) map[string]any {
	return map[string]any{
		"description": description,
		"required":    required,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func multipartRequest(description string) map[string]any {
	return map[string]any{
		"description": description,
		"required":    true,
		"content": map[string]any{
			"multipart/form-data": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file": map[string]any{
							"type":   "string",
							"format": "binary",
						},
					},
				},
			},
		},
	}
}

func arrayOf(schema any) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": schema,
	}
}

func messageSchema(message string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string", "example": message},
		},
	}
}

func apiEnvelope(dataSchema any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"success": map[string]any{"type": "boolean", "example": true},
			"data":    dataSchema,
			"message": map[string]any{"type": "string"},
			"code":    map[string]any{"type": "string"},
		},
		"required": []string{"success"},
	}
}

func errorEnvelope() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"success": map[string]any{"type": "boolean", "example": false},
			"error":   map[string]any{"type": "string"},
			"code":    map[string]any{"type": "string"},
		},
		"required": []string{"success", "error"},
	}
}

func paginatedEnvelope(itemSchema any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"success": map[string]any{"type": "boolean", "example": true},
			"data": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items":       arrayOf(itemSchema),
					"total":       map[string]any{"type": "integer"},
					"page":        map[string]any{"type": "integer"},
					"per_page":    map[string]any{"type": "integer"},
					"total_pages": map[string]any{"type": "integer"},
				},
			},
		},
		"required": []string{"success", "data"},
	}
}

func okResponse(description string, dataSchema any) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": apiEnvelope(dataSchema),
			},
		},
	}
}

func createdResponse(description string, dataSchema any) map[string]any {
	return okResponse(description, dataSchema)
}

func paginatedResponse(description string, itemSchema any) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": paginatedEnvelope(itemSchema),
			},
		},
	}
}

func errorResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": errorEnvelope(),
			},
		},
	}
}

func noContentResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
	}
}

func redirectResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"headers": map[string]any{
			"Location": map[string]any{
				"description": "Redirect target",
				"schema": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

func textResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"text/plain": map[string]any{
				"schema": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

func sseResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"text/event-stream": map[string]any{
				"schema": map[string]any{
					"type":        "string",
					"description": "SSE stream carrying event and data lines.",
				},
			},
		},
	}
}

func switchingProtocolsResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
	}
}

func protectedSecurity() []map[string]any {
	return []map[string]any{
		{"cookieAuth": []string{}},
		{"bearerAuth": []string{}},
	}
}

func publicSecurity() []map[string]any {
	return []map[string]any{}
}
