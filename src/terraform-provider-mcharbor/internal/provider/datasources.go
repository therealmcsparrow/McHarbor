// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

// configureDataSource is shared by every data source, which all only need the client.
func configureDataSource(req datasource.ConfigureRequest) *apiClient {
	c, _ := req.ProviderData.(*apiClient)
	return c
}

type environmentsDataSource struct{ c *apiClient }

func newEnvironmentsDataSource() datasource.DataSource { return &environmentsDataSource{} }

var environmentAttrTypes = map[string]attr.Type{
	"id":              types.StringType,
	"name":            types.StringType,
	"connection_type": types.StringType,
	"is_default":      types.BoolType,
	"agent_status":    types.StringType,
}

func (d *environmentsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environments"
}

func (d *environmentsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every environment the API key can see. Requires environments.view.",
		Attributes: map[string]schema.Attribute{
			"environments": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":              schema.StringAttribute{Computed: true},
					"name":            schema.StringAttribute{Computed: true},
					"connection_type": schema.StringAttribute{Computed: true},
					"is_default":      schema.BoolAttribute{Computed: true},
					"agent_status":    schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *environmentsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	d.c = configureDataSource(req)
}

func (d *environmentsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var envs []client.EnvironmentsEnvironment
	if err := decode(apiCall(d.c.api.EnvironmentsList(ctx)), "environmentsList", &envs); err != nil {
		addError(&resp.Diagnostics, "Listing environments", err)
		return
	}
	objType := types.ObjectType{AttrTypes: environmentAttrTypes}
	items := make([]attr.Value, 0, len(envs))
	for _, e := range envs {
		items = append(items, types.ObjectValueMust(environmentAttrTypes, map[string]attr.Value{
			"id":              types.StringValue(e.Id),
			"name":            types.StringValue(e.Name),
			"connection_type": types.StringValue(e.ConnectionType),
			"is_default":      types.BoolValue(e.IsDefault),
			"agent_status":    types.StringValue(deref(e.AgentStatus)),
		}))
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot("environments"), types.ListValueMust(objType, items))...)
}

type rolesDataSource struct{ c *apiClient }

func newRolesDataSource() datasource.DataSource { return &rolesDataSource{} }

var roleAttrTypes = map[string]attr.Type{
	"id":          types.StringType,
	"name":        types.StringType,
	"description": types.StringType,
	"permissions": types.ListType{ElemType: types.StringType},
	"is_system":   types.BoolType,
}

func (d *rolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

func (d *rolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every role, including the system presets. Requires roles.view.",
		Attributes: map[string]schema.Attribute{
			"roles": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":          schema.StringAttribute{Computed: true},
					"name":        schema.StringAttribute{Computed: true},
					"description": schema.StringAttribute{Computed: true},
					"permissions": schema.ListAttribute{Computed: true, ElementType: types.StringType},
					"is_system":   schema.BoolAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *rolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	d.c = configureDataSource(req)
}

func (d *rolesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var roles []client.RolesRole
	if err := decode(apiCall(d.c.api.RolesList(ctx)), "rolesList", &roles); err != nil {
		addError(&resp.Diagnostics, "Listing roles", err)
		return
	}
	objType := types.ObjectType{AttrTypes: roleAttrTypes}
	items := make([]attr.Value, 0, len(roles))
	for _, r := range roles {
		items = append(items, types.ObjectValueMust(roleAttrTypes, map[string]attr.Value{
			"id":          types.StringValue(r.Id),
			"name":        types.StringValue(r.Name),
			"description": types.StringValue(r.Description),
			"permissions": stringList(r.Permissions),
			"is_system":   types.BoolValue(r.IsSystem),
		}))
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot("roles"), types.ListValueMust(objType, items))...)
}

type permissionCatalogDataSource struct{ c *apiClient }

func newPermissionCatalogDataSource() datasource.DataSource { return &permissionCatalogDataSource{} }

// catalogEntry mirrors GET /api/auth/permissions/catalog, which the spec does
// not yet type.
type catalogEntry struct {
	Key         string `json:"key"`
	Category    string `json:"category"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
}

var catalogAttrTypes = map[string]attr.Type{
	"key":         types.StringType,
	"category":    types.StringType,
	"action":      types.StringType,
	"description": types.StringType,
	"scope":       types.StringType,
}

func (d *permissionCatalogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_catalog"
}

func (d *permissionCatalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Every permission key McHarbor and its installed plugins define. Requires roles.view or permissions.view.",
		Attributes: map[string]schema.Attribute{
			"permissions": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"key":         schema.StringAttribute{Computed: true},
					"category":    schema.StringAttribute{Computed: true},
					"action":      schema.StringAttribute{Computed: true},
					"description": schema.StringAttribute{Computed: true},
					"scope":       schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *permissionCatalogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	d.c = configureDataSource(req)
}

func (d *permissionCatalogDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var entries []catalogEntry
	if err := decode(apiCall(d.c.api.AuthPermissionCatalog(ctx)), "authPermissionCatalog", &entries); err != nil {
		addError(&resp.Diagnostics, "Reading the permission catalog", err)
		return
	}
	objType := types.ObjectType{AttrTypes: catalogAttrTypes}
	items := make([]attr.Value, 0, len(entries))
	for _, e := range entries {
		items = append(items, types.ObjectValueMust(catalogAttrTypes, map[string]attr.Value{
			"key":         types.StringValue(e.Key),
			"category":    types.StringValue(e.Category),
			"action":      types.StringValue(e.Action),
			"description": types.StringValue(e.Description),
			"scope":       types.StringValue(e.Scope),
		}))
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot("permissions"), types.ListValueMust(objType, items))...)
}
