// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type updatePolicyResource struct{ c *apiClient }

type updatePolicyModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ContainerMatch types.String `tfsdk:"container_match"`
	ImageMatch     types.String `tfsdk:"image_match"`
	Schedule       types.String `tfsdk:"schedule"`
	Strategy       types.String `tfsdk:"strategy"`
	AutoRestart    types.Bool   `tfsdk:"auto_restart"`
	Enabled        types.Bool   `tfsdk:"enabled"`
}

var (
	_ resource.ResourceWithConfigure   = &updatePolicyResource{}
	_ resource.ResourceWithImportState = &updatePolicyResource{}
)

func newUpdatePolicyResource() resource.Resource { return &updatePolicyResource{} }

func (r *updatePolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_update_policy"
}

func (r *updatePolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An image update policy: which containers and images to check, when, and how. Requires " +
			"updates.view to read and updates.manage to create, change and remove.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":            schema.StringAttribute{Required: true},
			"container_match": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Glob or regex over container names."},
			"image_match":     schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), Description: "Glob or regex over image names."},
			"schedule":        schema.StringAttribute{Required: true, Description: "Five-field cron expression."},
			"strategy":        schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("latest"), Description: "latest, semver or digest."},
			"auto_restart":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"enabled":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		},
	}
}

func (r *updatePolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *updatePolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan updatePolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.UpdatesCreateJSONRequestBody{
		Name:           plan.Name.ValueString(),
		ContainerMatch: plan.ContainerMatch.ValueString(),
		ImageMatch:     plan.ImageMatch.ValueString(),
		Schedule:       plan.Schedule.ValueString(),
		Strategy:       plan.Strategy.ValueString(),
		AutoRestart:    plan.AutoRestart.ValueBool(),
	}
	var p client.UpdatesPolicy
	if err := decode(apiCall(r.c.api.UpdatesCreate(ctx, body)), "updatesCreate", &p); err != nil {
		addError(&resp.Diagnostics, "Creating update policy", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, updatePolicyFromAPI(p))...)
	if p.Enabled != plan.Enabled.ValueBool() {
		enabled := plan.Enabled.ValueBool()
		if err := decode(apiCall(r.c.api.UpdatesUpdate(ctx, p.Id, client.UpdatesUpdateJSONRequestBody{Enabled: &enabled})), "updatesUpdate", &p); err != nil {
			addError(&resp.Diagnostics, "Setting update policy enabled", err)
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, updatePolicyFromAPI(p))...)
	}
}

func (r *updatePolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state updatePolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var p client.UpdatesPolicy
	err := decode(apiCall(r.c.api.UpdatesGet(ctx, state.ID.ValueString())), "updatesGet", &p)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading update policy", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, updatePolicyFromAPI(p))...)
}

func (r *updatePolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state updatePolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.UpdatesUpdateJSONRequestBody{
		Name:           strPtr(plan.Name),
		ContainerMatch: strPtr(plan.ContainerMatch),
		ImageMatch:     strPtr(plan.ImageMatch),
		Schedule:       strPtr(plan.Schedule),
		Strategy:       strPtr(plan.Strategy),
		AutoRestart:    boolPtr(plan.AutoRestart),
		Enabled:        boolPtr(plan.Enabled),
	}
	var p client.UpdatesPolicy
	if err := decode(apiCall(r.c.api.UpdatesUpdate(ctx, state.ID.ValueString(), body)), "updatesUpdate", &p); err != nil {
		addError(&resp.Diagnostics, "Updating update policy", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, updatePolicyFromAPI(p))...)
}

func (r *updatePolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state updatePolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.UpdatesDelete(ctx, state.ID.ValueString())), "updatesDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting update policy", err)
	}
}

func (r *updatePolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func updatePolicyFromAPI(p client.UpdatesPolicy) updatePolicyModel {
	return updatePolicyModel{
		ID:             types.StringValue(p.Id),
		Name:           types.StringValue(p.Name),
		ContainerMatch: types.StringValue(p.ContainerMatch),
		ImageMatch:     types.StringValue(p.ImageMatch),
		Schedule:       types.StringValue(p.Schedule),
		Strategy:       types.StringValue(p.Strategy),
		AutoRestart:    types.BoolValue(p.AutoRestart),
		Enabled:        types.BoolValue(p.Enabled),
	}
}
