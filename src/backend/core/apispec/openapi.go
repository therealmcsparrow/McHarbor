// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"regexp"
	"sort"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// SpecVersion is the OpenAPI dialect emitted. 3.0.3 rather than 3.1 because
// oapi-codegen, which generates the CLI's client, handles it far better.
const SpecVersion = "3.0.3"

var pathParamRe = regexp.MustCompile(`\{([^}/]+)\}`)

// Document renders the whole OpenAPI document for everything the registry
// recorded. It is pure: the same registry always produces the same bytes, so CI
// can regenerate and fail on a diff.
func Document(reg *Registry, version string) map[string]any {
	b := newSchemaBuilder()
	ops := reg.Operations()

	paths := map[string]any{}
	tagsSeen := map[string]bool{}
	idsSeen := map[string]bool{}

	for _, op := range ops {
		if op.Method == MethodAny {
			// OpenAPI has no wildcard verb. Recording it keeps the drift
			// report honest; emitting it would invent methods the route may
			// not actually answer.
			continue
		}

		item, _ := paths[op.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			if params := pathParameters(op.Path); len(params) > 0 {
				item["parameters"] = params
			}
			paths[op.Path] = item
		}

		tag := op.tag()
		tagsSeen[tag] = true
		obj := operationObject(b, op, tag)
		// One handler mounted on two verbs would otherwise yield one operationId
		// twice, which generated clients reject as a duplicate method.
		if id, _ := obj["operationId"].(string); idsSeen[id] {
			obj["operationId"] = id + camel(strings.ToLower(op.Method))
		}
		idsSeen[obj["operationId"].(string)] = true
		item[strings.ToLower(op.Method)] = obj
	}

	tags := make([]map[string]any, 0, len(tagsSeen))
	for t := range tagsSeen {
		tags = append(tags, map[string]any{"name": t})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i]["name"].(string) < tags[j]["name"].(string) })

	components := map[string]any{
		"securitySchemes": securitySchemes(),
		"schemas":         b.components,
		"responses":       sharedResponses(),
	}

	return map[string]any{
		"openapi": SpecVersion,
		"info": map[string]any{
			"title":       "McHarbor API",
			"version":     version,
			"description": apiDescription,
			"contact":     map[string]string{"name": "McHarbor"},
		},
		"servers":    []map[string]string{{"url": "/", "description": "Current McHarbor instance; every path carries its own /api prefix"}},
		"tags":       tags,
		"paths":      paths,
		"components": components,
	}
}

const apiDescription = "McHarbor exposes a self-hosted control plane API for Docker, remote agents, " +
	"workflows, notifications, store content, and operational telemetry. Most routes return the shared " +
	"{ success, data?, error?, message?, code? } envelope and require either the session cookie or a " +
	"Bearer API key. Environment-scoped routes select the target runtime with ?env=<environmentId>.\n\n" +
	"This document is generated from the running router. Every operation carries the permission it " +
	"enforces as x-mcharbor-permission, or x-mcharbor-exempt naming why it enforces none."

func (o Operation) tag() string {
	if len(o.Tags) > 0 {
		return o.Tags[0]
	}
	if o.Module != "" {
		return o.Module
	}
	return "other"
}

func operationObject(b *schemaBuilder, op Operation, tag string) map[string]any {
	obj := map[string]any{
		"tags":        []string{tag},
		"operationId": operationID(op),
		"summary":     op.Summary,
		"responses":   responses(b, op),
	}
	if op.Description != "" {
		obj["description"] = op.Description
	}
	if op.Deprecated {
		obj["deprecated"] = true
	}
	if params := queryParameters(op); len(params) > 0 {
		obj["parameters"] = params
	}
	if op.Request != nil {
		obj["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{"schema": b.schemaFor(op.Request)},
			},
		}
	}

	switch op.Group {
	case GroupPublic, GroupAuth:
		obj["security"] = []map[string]any{}
	default:
		obj["security"] = []map[string]any{{"cookieAuth": []string{}}, {"bearerAuth": []string{}}}
	}

	if len(op.Perms) > 0 {
		obj["x-mcharbor-permission"] = permissionExtension(op.Perms)
	} else if op.Exempt != "" {
		obj["x-mcharbor-exempt"] = op.Exempt
	}
	if op.Stream != StreamNone {
		obj["x-mcharbor-stream"] = string(op.Stream)
	}
	return obj
}

func permissionExtension(reqs []rbac.PermRequirement) []map[string]any {
	out := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		keys := make([]string, 0, len(req.Keys))
		for _, k := range req.Keys {
			keys = append(keys, string(k))
		}
		entry := map[string]any{"scope": string(req.Scope)}
		if len(keys) > 0 {
			entry["keys"] = keys
		}
		if req.EnvParam != "" {
			entry["env_param"] = req.EnvParam
		}
		if req.Any {
			entry["match"] = "any"
		} else {
			entry["match"] = "all"
		}
		out = append(out, entry)
	}
	return out
}

func pathParameters(path string) []map[string]any {
	matches := pathParamRe.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		// chi allows {id:[0-9]+}; the regex is not part of the contract.
		if i := strings.Index(name, ":"); i >= 0 {
			name = name[:i]
		}
		out = append(out, map[string]any{
			"name":     name,
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string"},
		})
	}
	return out
}

func queryParameters(op Operation) []map[string]any {
	if len(op.Query) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(op.Query))
	for _, p := range op.Query {
		param := map[string]any{
			"name":   p.Name,
			"in":     "query",
			"schema": map[string]any{"type": "string"},
		}
		if p.Description != "" {
			param["description"] = p.Description
		}
		if p.Required {
			param["required"] = true
		}
		if p.Example != "" {
			param["example"] = p.Example
		}
		out = append(out, param)
	}
	return out
}

// responses builds the success and failure set. The envelope is described once
// here rather than repeated per operation, with the operation's own payload
// substituted into data when it declared one.
func responses(b *schemaBuilder, op Operation) map[string]any {
	success := map[string]any{"description": "Success"}

	switch op.Stream {
	case StreamSSE:
		success["description"] = "Event stream; the connection stays open until the client closes it."
		success["content"] = map[string]any{"text/event-stream": map[string]any{
			"schema": map[string]any{"type": "string"},
		}}
	case StreamWebSocket:
		success = map[string]any{"description": "Protocol upgrade; the exchange that follows is not HTTP."}
	case StreamBinary:
		success["content"] = map[string]any{"application/octet-stream": map[string]any{
			"schema": map[string]any{"type": "string", "format": "binary"},
		}}
	case StreamText:
		success["content"] = map[string]any{"text/plain": map[string]any{
			"schema": map[string]any{"type": "string"},
		}}
	default:
		envelope := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"success": map[string]any{"type": "boolean"},
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"success"},
		}
		if op.Response != nil {
			envelope["properties"].(map[string]any)["data"] = b.schemaFor(op.Response)
		}
		success["content"] = map[string]any{
			"application/json": map[string]any{"schema": envelope},
		}
	}

	out := map[string]any{successCode(op): success}

	if op.Stream == StreamWebSocket {
		out["101"] = out[successCode(op)]
		delete(out, successCode(op))
	}

	out["400"] = responseRef("BadRequest")
	if op.Group == GroupProtected {
		out["401"] = responseRef("Unauthorized")
		if op.Gated() {
			out["403"] = responseRef("Forbidden")
		}
	}
	out["500"] = responseRef("ServerError")
	return out
}

func successCode(op Operation) string {
	if op.Method == "POST" {
		return "201"
	}
	return "200"
}

// sharedResponses holds the failure bodies once, because every operation shares
// them and inlining them per operation made the committed document megabytes.
func sharedResponses() map[string]any {
	return map[string]any{
		"BadRequest":   errorResponse("Validation failed."),
		"Unauthorized": errorResponse("Authentication required."),
		"Forbidden":    errorResponse("The caller lacks the required permission."),
		"ServerError":  errorResponse("Server error; the cause is logged server-side."),
	}
}

func responseRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/responses/" + name}
}

func errorResponse(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"success": map[string]any{"type": "boolean"},
						"error":   map[string]any{"type": "string"},
						"code":    map[string]any{"type": "string"},
						"detail": map[string]any{
							"type":        "string",
							"description": "Redacted cause behind the coded failure; only on authenticated requests.",
						},
						"requestId": map[string]any{"type": "string"},
					},
					"required": []string{"success"},
				},
			},
		},
	}
}

func securitySchemes() map[string]any {
	return map[string]any{
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
	}
}
