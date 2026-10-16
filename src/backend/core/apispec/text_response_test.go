// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import "testing"

// TestTextOperationIsNotDescribedAsTheJSONEnvelope guards the scrape endpoint's
// contract: a generated client that expects {success} from a Prometheus
// exposition would fail on every call.
func TestTextOperationIsNotDescribedAsTheJSONEnvelope(t *testing.T) {
	op := Operation{Method: "GET", Path: "/api/metrics"}
	Text()(&op)

	success := responses(newSchemaBuilder(), op)[successCode(op)].(map[string]any)
	content := success["content"].(map[string]any)
	if _, ok := content["text/plain"]; !ok {
		t.Fatalf("text operation content = %v, want text/plain", content)
	}
	if _, ok := content["application/json"]; ok {
		t.Fatal("text operation still advertises the JSON envelope")
	}
}
