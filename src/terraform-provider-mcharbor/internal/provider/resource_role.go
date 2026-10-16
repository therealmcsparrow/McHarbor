// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type roleResource struct {
	c *apiClient
}

type roleModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Permissions types.Set    `tfsdk:"permissions"`
	IsSystem    types.Bool   `tfsdk:"is_system"`
}

var (
	_ resource.ResourceWithConfigure   = &roleResource{}
	_ resource.ResourceWithImportState = &roleResource{}
)

func newRoleResource() resource.Resource { return &roleResource{} }

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A role: a named set of permission keys. Requires roles.view to read and roles.manage to change.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(""),
			},
			"permissions": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Permission keys, for example containers.view. See the mcharbor_permission_catalog data source.",
			},
			"is_system": schema.BoolAttribute{Computed: true, Description: "System roles cannot be changed or deleted."},
		},
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.RolesCreateJSONRequestBody{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Permissions: setStrings(ctx, plan.Permissions, &resp.Diagnostics),
	}
	var role client.RolesRole
	if err := decode(apiCall(r.c.api.RolesCreate(ctx, body)), "rolesCreate", &role); err != nil {
		addError(&resp.Diagnostics, "Creating role", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, roleFromAPI(role))...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var role client.RolesRole
	err := decode(apiCall(r.c.api.RolesGet(ctx, state.ID.ValueString())), "rolesGet", &role)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading role", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, roleFromAPI(role))...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.RolesUpdateJSONRequestBody{
		Name:        strPtr(plan.Name),
		Description: strPtr(plan.Description),
		Permissions: setStrings(ctx, plan.Permissions, &resp.Diagnostics),
	}
	var role client.RolesRole
	if err := decode(apiCall(r.c.api.RolesUpdate(ctx, state.ID.ValueString(), body)), "rolesUpdate", &role); err != nil {
		addError(&resp.Diagnostics, "Updating role", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, roleFromAPI(role))...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.RolesDelete(ctx, state.ID.ValueString())), "rolesDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting role", err)
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func roleFromAPI(role client.RolesRole) roleModel {
	return roleModel{
		ID:          types.StringValue(role.Id),
		Name:        types.StringValue(role.Name),
		Description: types.StringValue(role.Description),
		Permissions: stringSet(role.Permissions),
		IsSystem:    types.BoolValue(role.IsSystem),
	}
}
