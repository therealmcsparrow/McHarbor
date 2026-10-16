// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestProviderSchemaIsValid serves the provider over the real protocol and
// asks for its schema, which fails on any malformed resource or data source.
func TestProviderSchemaIsValid(t *testing.T) {
	srv, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	for _, name := range []string{"mcharbor_environment", "mcharbor_stack", "mcharbor_role", "mcharbor_role_assignment", "mcharbor_api_key"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %s missing", name)
		}
	}
	for _, name := range []string{"mcharbor_environments", "mcharbor_roles", "mcharbor_permission_catalog"} {
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("data source %s missing", name)
		}
	}
}

// TestSchemasImplementCleanly runs the framework's own implementation checks,
// which catch defaults on non-computed attributes and similar mistakes.
func TestSchemasImplementCleanly(t *testing.T) {
	ctx := context.Background()
	p := New("test")()
	for _, factory := range p.Resources(ctx) {
		r := factory()
		var meta resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "mcharbor"}, &meta)
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		resp.Diagnostics.Append(resp.Schema.ValidateImplementation(ctx)...)
		for _, d := range resp.Diagnostics {
			t.Errorf("%s: %s: %s", meta.TypeName, d.Summary(), d.Detail())
		}
		if _, ok := r.(resource.ResourceWithImportState); !ok {
			t.Errorf("%s does not support import", meta.TypeName)
		}
	}
	for _, factory := range p.DataSources(ctx) {
		d := factory()
		var meta datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "mcharbor"}, &meta)
		var resp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &resp)
		resp.Diagnostics.Append(resp.Schema.ValidateImplementation(ctx)...)
		for _, diag := range resp.Diagnostics {
			t.Errorf("%s: %s: %s", meta.TypeName, diag.Summary(), diag.Detail())
		}
	}
}

// TestEnvironmentRejectsUnknownConnectionType checks validation runs at plan
// time through the protocol, before any request is made.
func TestEnvironmentRejectsUnknownConnectionType(t *testing.T) {
	ctx := context.Background()
	srv, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	schemaResp, err := srv.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	objType := schemaResp.ResourceSchemas["mcharbor_environment"].ValueType().(tftypes.Object)

	values := map[string]tftypes.Value{}
	for name, typ := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(typ, nil)
	}
	for name, v := range map[string]string{"name": "edge", "connection_type": "carrier-pigeon"} {
		values[name] = tftypes.NewValue(tftypes.String, v)
	}
	config, err := tfprotov6.NewDynamicValue(objType, tftypes.NewValue(objType, values))
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	resp, err := srv.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: "mcharbor_environment", Config: &config})
	if err != nil {
		t.Fatalf("ValidateResourceConfig: %v", err)
	}
	if len(resp.Diagnostics) == 0 {
		t.Fatal("an unknown connection_type passed validation")
	}
}
