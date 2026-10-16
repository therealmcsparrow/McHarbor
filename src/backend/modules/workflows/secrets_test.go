// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// stubResolver stands in for the vault. The point of these tests is the walk,
// not the lookup — the lookup has its own table-driven tests in the secrets
// module.
type stubResolver struct {
	values map[string]string
	calls  []string
}

func (r *stubResolver) ResolveValue(_ context.Context, value string, _ secrets.ResolveScope, field string) (string, error) {
	if !secrets.ContainsReference(value) {
		return value, nil
	}
	r.calls = append(r.calls, field)
	out := value
	matched := false
	for name, plaintext := range r.values {
		reference := "${secret:" + name + "}"
		if strings.Contains(out, reference) {
			out = strings.ReplaceAll(out, reference, plaintext)
			matched = true
		}
	}
	if !matched {
		return "", errors.New("no secret named " + value)
	}
	return out, nil
}

func TestResolveNodeSecretsWalksNestedConfig(t *testing.T) {
	resolver := &stubResolver{values: map[string]string{"API_TOKEN": "tok_live", "PW": "s3cr3t"}}
	svc := &Service{secretResolver: resolver}

	node := &CanvasNode{
		ID:     "node-1",
		Action: "http-request",
		Config: map[string]interface{}{
			"url":     "https://example.test",
			"timeout": 30,
			"auth": map[string]interface{}{
				"token": "${secret:API_TOKEN}",
			},
			"headers": []interface{}{
				"X-Plain: yes",
				"X-Auth: ${secret:PW}",
			},
		},
	}

	resolved, err := svc.resolveNodeSecrets(context.Background(), node, "env-1")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}

	auth, _ := resolved.Config["auth"].(map[string]interface{})
	if auth["token"] != "tok_live" {
		t.Errorf("nested map value not resolved: %v", auth["token"])
	}
	headers, _ := resolved.Config["headers"].([]interface{})
	if headers[1] != "X-Auth: s3cr3t" {
		t.Errorf("slice element not resolved: %v", headers[1])
	}
	if headers[0] != "X-Plain: yes" {
		t.Errorf("a plain slice element should pass through: %v", headers[0])
	}
	if resolved.Config["timeout"] != 30 {
		t.Errorf("a non-string value should pass through untouched: %v", resolved.Config["timeout"])
	}

	// The stored node must be untouched: ExecuteNode is handed a node decoded
	// from the saved canvas, and plaintext written into it could be persisted
	// on the next save.
	originalAuth, _ := node.Config["auth"].(map[string]interface{})
	if originalAuth["token"] != "${secret:API_TOKEN}" {
		t.Fatal("the original node was mutated; plaintext could reach the stored canvas")
	}
}

func TestResolveNodeSecretsSkipsNodesWithoutReferences(t *testing.T) {
	resolver := &stubResolver{values: map[string]string{}}
	svc := &Service{secretResolver: resolver}

	node := &CanvasNode{
		ID:     "node-1",
		Config: map[string]interface{}{"url": "https://example.test", "retries": 3},
	}

	resolved, err := svc.resolveNodeSecrets(context.Background(), node, "env-1")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if resolved != node {
		t.Error("a node with no references should be returned as-is, not copied")
	}
	if len(resolver.calls) != 0 {
		t.Errorf("a node with no references should cost no lookups, got %v", resolver.calls)
	}
}

func TestResolveNodeSecretsFailsOnAMiss(t *testing.T) {
	resolver := &stubResolver{values: map[string]string{}}
	svc := &Service{secretResolver: resolver}

	node := &CanvasNode{
		ID:     "node-1",
		Config: map[string]interface{}{"token": "${secret:NOT_THERE}"},
	}

	if _, err := svc.resolveNodeSecrets(context.Background(), node, "env-1"); err == nil {
		t.Fatal("a missing reference must fail the node rather than run it with an empty credential")
	}
}

func TestResolveNodeSecretsIsANoOpWithoutAResolver(t *testing.T) {
	svc := &Service{}
	node := &CanvasNode{ID: "node-1", Config: map[string]interface{}{"token": "${secret:X}"}}

	resolved, err := svc.resolveNodeSecrets(context.Background(), node, "env-1")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if resolved != node {
		t.Error("without a resolver the node should pass through unchanged")
	}
}
