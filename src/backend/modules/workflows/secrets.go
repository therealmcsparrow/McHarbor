// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// SecretResolver substitutes ${secret:NAME} in a workflow node's configuration
// immediately before the node runs. Held behind an interface so this module
// does not depend on the vault to compile.
type SecretResolver interface {
	ResolveValue(ctx context.Context, value string, scope secrets.ResolveScope, field string) (string, error)
}

// SetSecretResolver overrides the resolver. Present for tests; ordinary wiring
// builds one inside NewService from the database and encryption service it
// already receives.
func (s *Service) SetSecretResolver(r SecretResolver) { s.secretResolver = r }

// resolveNodeSecrets returns a copy of the node whose configuration has every
// `${secret:NAME}` substituted.
//
// It walks the config rather than naming specific keys, because a workflow
// node's credential field is called `password` in one node type, `token` in
// another, `apiKey` in a third, and lives inside a nested `auth` object in a
// fourth. There are eighty-odd node actions; enumerating their credential
// fields would be wrong within a release.
//
// The node itself is never mutated. `ExecuteNode` is handed a node decoded
// from the stored canvas, and writing plaintext into it risks that plaintext
// being saved back the next time the canvas is persisted.
func (s *Service) resolveNodeSecrets(ctx context.Context, node *CanvasNode, envID string) (*CanvasNode, error) {
	if s.secretResolver == nil || node == nil || len(node.Config) == 0 {
		return node, nil
	}
	if !configHasReference(node.Config) {
		return node, nil
	}

	scope := secrets.ResolveScope{
		EnvironmentID: envID,
		ResourceType:  "workflow",
		ResourceID:    node.ID,
	}

	resolved, err := s.resolveConfigValue(ctx, node.Config, scope, "")
	if err != nil {
		return nil, err
	}

	clone := *node
	clone.Config, _ = resolved.(map[string]interface{})
	return &clone, nil
}

// configHasReference is a cheap pre-pass so the common case — a node with no
// references at all — costs one walk and no database queries.
func configHasReference(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		return secrets.ContainsReference(typed)
	case map[string]interface{}:
		for _, nested := range typed {
			if configHasReference(nested) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if configHasReference(nested) {
				return true
			}
		}
	}
	return false
}

// resolveConfigValue rebuilds a config tree with every string resolved. Maps
// and slices are copied rather than edited in place, for the same
// don't-mutate-the-stored-node reason.
func (s *Service) resolveConfigValue(
	ctx context.Context, value interface{}, scope secrets.ResolveScope, field string,
) (interface{}, error) {
	switch typed := value.(type) {
	case string:
		return s.secretResolver.ResolveValue(ctx, typed, scope, field)

	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			nestedField := key
			if field != "" {
				nestedField = field + "." + key
			}
			resolved, err := s.resolveConfigValue(ctx, nested, scope, nestedField)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil

	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, nested := range typed {
			resolved, err := s.resolveConfigValue(ctx, nested, scope, field)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil

	default:
		// Numbers, booleans and nulls cannot carry a reference.
		return value, nil
	}
}
