// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

// TestValidateSecretSource pins the rule that a value comes from exactly one
// place, and that only a new secret must name one (an import adopts the value).
func TestValidateSecretSource(t *testing.T) {
	cases := []struct {
		name                       string
		valueSet, generate, create bool
		wantErr                    bool
	}{
		{"value on create", true, false, true, false},
		{"generate on create", false, true, true, false},
		{"both", true, true, false, true},
		{"neither on create", false, false, true, true},
		{"neither on an adopted secret", false, false, false, false},
	}
	for _, tc := range cases {
		diags := ValidateSecretSource(tc.valueSet, tc.generate, tc.create)
		if diags.HasError() != tc.wantErr {
			t.Errorf("%s: HasError = %v, want %v (%v)", tc.name, diags.HasError(), tc.wantErr, diags)
		}
	}
}

// TestSecretValueNeverReachesState is the promise of the design: whatever the
// server sends and whatever was configured, state holds metadata only.
func TestSecretValueNeverReachesState(t *testing.T) {
	raw := rawData(`{"secret":{"id":"s1","name":"db","kind":"password","scope":"global","version":1,"generated":true},"value":"TOP-SECRET-VALUE"}`)
	sec, err := secretFromResponse(raw)
	if err != nil {
		t.Fatalf("secretFromResponse: %v", err)
	}
	state := secretState(sec, secretModel{ValueWO: types.StringValue("configured"), Generate: types.BoolValue(true)})
	if !state.ValueWO.IsNull() {
		t.Fatalf("value_wo reached state: %v", state.ValueWO)
	}
	encoded, _ := json.Marshal(state)
	if strings.Contains(string(encoded), "TOP-SECRET-VALUE") || strings.Contains(string(encoded), "configured") {
		t.Fatalf("a secret value leaked into the state model: %s", encoded)
	}
	if state.ID.ValueString() != "s1" || state.Version.ValueInt64() != 1 {
		t.Fatalf("metadata not mapped: %+v", state)
	}
}

// TestSecretFromResponseAcceptsTheBareSecret covers create without generate,
// where the payload is the secret itself rather than {secret, value}.
func TestSecretFromResponseAcceptsTheBareSecret(t *testing.T) {
	sec, err := secretFromResponse(rawData(`{"id":"s2","name":"api","kind":"token","scope":"global","version":3}`))
	if err != nil || sec.Id != "s2" || sec.Version != 3 {
		t.Fatalf("got %+v, %v", sec, err)
	}
}

// TestWriteOnlyAttributesAreDeclaredSo guards the schema: a credential that
// loses WriteOnly would start landing in state on the next apply.
func TestWriteOnlyAttributesAreDeclaredSo(t *testing.T) {
	check := func(r resource.Resource, names ...string) {
		resp := &resource.SchemaResponse{}
		r.Schema(context.Background(), resource.SchemaRequest{}, resp)
		for _, n := range names {
			a, ok := resp.Schema.Attributes[n]
			if !ok {
				t.Errorf("attribute %s missing", n)
				continue
			}
			if !a.IsWriteOnly() || !a.IsSensitive() {
				t.Errorf("attribute %s must be write-only and sensitive", n)
			}
		}
	}
	check(newSecretResource(), "value_wo")
	check(newStorageLocationResource(), append([]string{"config_wo"}, storageCredentialAttrs...)...)
}

// TestStorageLocationStateHasNoCredentials mirrors the secret test for the
// storage credentials.
func TestStorageLocationStateHasNoCredentials(t *testing.T) {
	state := storageLocationState(client.StorageLocationsStorageLocation{Id: "l1", Name: "x", LocationType: "local"},
		storageLocationModel{Password: types.StringValue("hunter2")})
	if !state.Password.IsNull() || !state.Config.IsNull() {
		t.Fatalf("credentials reached state: %+v", state)
	}
}
