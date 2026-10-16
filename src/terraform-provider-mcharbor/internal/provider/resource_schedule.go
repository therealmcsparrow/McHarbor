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

type scheduleResource struct{ c *apiClient }

type scheduleModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Cron          types.String `tfsdk:"cron"`
	Action        types.String `tfsdk:"action"`
	Target        types.String `tfsdk:"target"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	NextRunAt     types.String `tfsdk:"next_run_at"`
}

var (
	_ resource.ResourceWithConfigure   = &scheduleResource{}
	_ resource.ResourceWithImportState = &scheduleResource{}
)

func newScheduleResource() resource.Resource { return &scheduleResource{} }

func (r *scheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (r *scheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A scheduled container action. The cron expression is validated by McHarbor, which rejects an " +
			"unparseable one. Requires schedules.view to read, schedules.manage to change and schedules.delete to remove.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":           schema.StringAttribute{Required: true},
			"description":    schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"cron":           schema.StringAttribute{Required: true, Description: "Five-field cron expression, for example \"0 3 * * *\"."},
			"action":         schema.StringAttribute{Required: true, Description: "start, stop, restart or exec."},
			"target":         schema.StringAttribute{Required: true, Description: "Container name or id."},
			"environment_id": schema.StringAttribute{Required: true},
			"enabled":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"next_run_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *scheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *scheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.SchedulesCreateJSONRequestBody{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Cron:        plan.Cron.ValueString(),
		Action:      plan.Action.ValueString(),
		Target:      plan.Target.ValueString(),
		EnvId:       plan.EnvironmentID.ValueString(),
	}
	var s client.SchedulesSchedule
	if err := decode(apiCall(r.c.api.SchedulesCreate(ctx, body)), "schedulesCreate", &s); err != nil {
		addError(&resp.Diagnostics, "Creating schedule", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, scheduleFromAPI(s))...)
	if s.Enabled != plan.Enabled.ValueBool() {
		enabled := plan.Enabled.ValueBool()
		if err := decode(apiCall(r.c.api.SchedulesUpdate(ctx, s.Id, client.SchedulesUpdateJSONRequestBody{Enabled: &enabled})), "schedulesUpdate", &s); err != nil {
			addError(&resp.Diagnostics, "Setting schedule enabled", err)
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, scheduleFromAPI(s))...)
	}
}

func (r *scheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var s client.SchedulesSchedule
	err := decode(apiCall(r.c.api.SchedulesGet(ctx, state.ID.ValueString())), "schedulesGet", &s)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading schedule", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, scheduleFromAPI(s))...)
}

func (r *scheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.SchedulesUpdateJSONRequestBody{
		Name:        strPtr(plan.Name),
		Description: strPtr(plan.Description),
		Cron:        strPtr(plan.Cron),
		Action:      strPtr(plan.Action),
		Target:      strPtr(plan.Target),
		EnvId:       strPtr(plan.EnvironmentID),
		Enabled:     boolPtr(plan.Enabled),
	}
	var s client.SchedulesSchedule
	if err := decode(apiCall(r.c.api.SchedulesUpdate(ctx, state.ID.ValueString(), body)), "schedulesUpdate", &s); err != nil {
		addError(&resp.Diagnostics, "Updating schedule", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, scheduleFromAPI(s))...)
}

func (r *scheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.SchedulesDelete(ctx, state.ID.ValueString())), "schedulesDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting schedule", err)
	}
}

func (r *scheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func scheduleFromAPI(s client.SchedulesSchedule) scheduleModel {
	return scheduleModel{
		ID:            types.StringValue(s.Id),
		Name:          types.StringValue(s.Name),
		Description:   types.StringValue(s.Description),
		Cron:          types.StringValue(s.Cron),
		Action:        types.StringValue(s.Action),
		Target:        types.StringValue(s.Target),
		EnvironmentID: types.StringValue(s.EnvId),
		Enabled:       types.BoolValue(s.Enabled),
		NextRunAt:     types.StringValue(s.NextRunAt),
	}
}
