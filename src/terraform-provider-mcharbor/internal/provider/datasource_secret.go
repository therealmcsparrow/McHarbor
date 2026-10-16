// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type secretDataSource struct{ c *apiClient }

type secretDataModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Scope         types.String `tfsdk:"scope"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	StackName     types.String `tfsdk:"stack_name"`
	Kind          types.String `tfsdk:"kind"`
	Description   types.String `tfsdk:"description"`
	Group         types.String `tfsdk:"group"`
	Version       types.Int64  `tfsdk:"version"`
	Generated     types.Bool   `tfsdk:"generated"`
	AutoApply     types.Bool   `tfsdk:"auto_apply"`
}

func newSecretDataSource() datasource.DataSource { return &secretDataSource{} }

func (d *secretDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (d *secretDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up one secret's metadata by name and scope, for modules that only reference it " +
			"(${secret:NAME}). Never returns the value. Requires secrets.view.",
		Attributes: map[string]schema.Attribute{
			"name":           schema.StringAttribute{Required: true},
			"scope":          schema.StringAttribute{Optional: true, Computed: true, Description: "Narrows the match when a name exists in more than one scope."},
			"environment_id": schema.StringAttribute{Optional: true, Computed: true},
			"stack_name":     schema.StringAttribute{Optional: true, Computed: true},
			"id":             schema.StringAttribute{Computed: true},
			"kind":           schema.StringAttribute{Computed: true},
			"description":    schema.StringAttribute{Computed: true},
			"group":          schema.StringAttribute{Computed: true},
			"version":        schema.Int64Attribute{Computed: true},
			"generated":      schema.BoolAttribute{Computed: true},
			"auto_apply":     schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *secretDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	d.c = configureDataSource(req)
}

func (d *secretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg secretDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := cfg.Name.ValueString()
	all, err := listAll[client.SecretsSecret]("secretsList", func(page, perPage string) call {
		params := &client.SecretsListParams{Page: &page, PerPage: &perPage, Env: strPtr(cfg.EnvironmentID)}
		return apiCall(d.c.api.SecretsList(ctx, params, withQuery("search", name)))
	})
	if err != nil {
		addError(&resp.Diagnostics, "Listing secrets", err)
		return
	}
	matches := matchSecrets(all, name, cfg.Scope.ValueString(), cfg.EnvironmentID.ValueString(), cfg.StackName.ValueString())
	switch len(matches) {
	case 0:
		resp.Diagnostics.AddError("Secret not found", fmt.Sprintf("No secret named %q matches the given scope.", name))
		return
	case 1:
	default:
		resp.Diagnostics.AddError("Secret is ambiguous", fmt.Sprintf(
			"%d secrets are named %q; set scope, environment_id or stack_name to pick one.", len(matches), name))
		return
	}
	s := matches[0]
	description := ""
	if s.Description != nil {
		description = *s.Description
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, secretDataModel{
		ID:            types.StringValue(s.Id),
		Name:          types.StringValue(s.Name),
		Scope:         types.StringValue(s.Scope),
		EnvironmentID: optString(s.EnvironmentId),
		StackName:     optString(s.StackName),
		Kind:          types.StringValue(s.Kind),
		Description:   types.StringValue(description),
		Group:         optString(s.Group),
		Version:       types.Int64Value(int64(s.Version)),
		Generated:     types.BoolValue(s.Generated),
		AutoApply:     types.BoolValue(s.AutoApply),
	})...)
}

// matchSecrets keeps exact-name hits, since the list's search is a substring match.
func matchSecrets(all []client.SecretsSecret, name, scope, envID, stack string) []client.SecretsSecret {
	var out []client.SecretsSecret
	for _, s := range all {
		if s.Name != name || (scope != "" && s.Scope != scope) {
			continue
		}
		if envID != "" && deref(s.EnvironmentId) != envID {
			continue
		}
		if stack != "" && deref(s.StackName) != stack {
			continue
		}
		out = append(out, s)
	}
	return out
}
