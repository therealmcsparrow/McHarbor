// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type alertRuleResource struct{ c *apiClient }

type alertConditionModel struct {
	Operator      types.String  `tfsdk:"operator"`
	Threshold     types.Float64 `tfsdk:"threshold"`
	Unit          types.String  `tfsdk:"unit"`
	ForSeconds    types.Int64   `tfsdk:"for_seconds"`
	WindowSeconds types.Int64   `tfsdk:"window_seconds"`
	Params        types.Map     `tfsdk:"params"`
}

type alertRuleModel struct {
	ID               types.String         `tfsdk:"id"`
	Name             types.String         `tfsdk:"name"`
	Severity         types.String         `tfsdk:"severity"`
	SourceType       types.String         `tfsdk:"source_type"`
	ConditionKey     types.String         `tfsdk:"condition_key"`
	Condition        *alertConditionModel `tfsdk:"condition"`
	Target           types.String         `tfsdk:"target"`
	EnvironmentIDs   types.Set            `tfsdk:"environment_ids"`
	Destinations     types.List           `tfsdk:"destinations"`
	SendInApp        types.Bool           `tfsdk:"send_in_app"`
	CooldownSeconds  types.Int64          `tfsdk:"cooldown_seconds"`
	NotifyOnRecovery types.Bool           `tfsdk:"notify_on_recovery"`
	MessageTemplate  types.String         `tfsdk:"message_template"`
	Enabled          types.Bool           `tfsdk:"enabled"`
}

var destinationAttrTypes = map[string]attr.Type{
	"channel_id":   types.StringType,
	"min_severity": types.StringType,
}

var (
	_ resource.ResourceWithConfigure   = &alertRuleResource{}
	_ resource.ResourceWithImportState = &alertRuleResource{}
)

func newAlertRuleResource() resource.Resource { return &alertRuleResource{} }

func (r *alertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rule"
}

func (r *alertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "An alert rule: a condition from a source (see GET /api/alerts/conditions), where it applies, " +
			"and who is notified. McHarbor fills condition defaults from the condition's declaration. Requires " +
			"alerts.view to read, alerts.manage to create and change, and alerts.delete to remove.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"name":          schema.StringAttribute{Required: true},
			"severity":      schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS, Description: "info, warning or critical."},
			"source_type":   schema.StringAttribute{Required: true, Description: "For example container, host or environment."},
			"condition_key": schema.StringAttribute{Required: true, Description: "For example cpu, memory, down."},
			"condition": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"operator": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS},
					"threshold": schema.Float64Attribute{
						Optional: true, Computed: true,
						PlanModifiers: []planmodifier.Float64{float64planmodifier.UseStateForUnknown()},
					},
					"unit":           schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS},
					"for_seconds":    schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI},
					"window_seconds": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI},
					"params": schema.MapAttribute{
						Optional: true, Computed: true, ElementType: types.StringType,
						PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
					},
				},
			},
			"target": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS, Description: "Name or id pattern, or * for all."},
			"environment_ids": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
				Description: "Empty means every active environment.",
			},
			"destinations": schema.ListNestedAttribute{
				Optional: true, Computed: true,
				Default: listdefault.StaticValue(types.ListValueMust(types.ObjectType{AttrTypes: destinationAttrTypes}, nil)),
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"channel_id":   schema.StringAttribute{Required: true},
					"min_severity": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS},
				}},
				Description: "Communication channels to notify. A rule needs at least one destination or send_in_app.",
			},
			"send_in_app":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
			"cooldown_seconds": schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI},
			"notify_on_recovery": schema.BoolAttribute{
				Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"message_template": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS},
			"enabled":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		},
	}
}

func (r *alertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func alertConditionBody(ctx context.Context, m *alertConditionModel, diags *diag.Diagnostics) client.AlertsConditionConfig {
	out := client.AlertsConditionConfig{}
	if m == nil {
		return out
	}
	out.Operator = strPtr(m.Operator)
	out.Unit = strPtr(m.Unit)
	out.ForSeconds = intPtr(m.ForSeconds)
	out.WindowSeconds = intPtr(m.WindowSeconds)
	if !m.Threshold.IsNull() && !m.Threshold.IsUnknown() {
		v := m.Threshold.ValueFloat64()
		out.Threshold = &v
	}
	if params := stringMap(ctx, m.Params, diags); params != nil {
		out.Params = &params
	}
	return out
}

func alertDestinationsBody(ctx context.Context, l types.List, diags *diag.Diagnostics) []client.AlertsDestinationInput {
	out := []client.AlertsDestinationInput{}
	if l.IsNull() || l.IsUnknown() {
		return out
	}
	var items []struct {
		ChannelID   types.String `tfsdk:"channel_id"`
		MinSeverity types.String `tfsdk:"min_severity"`
	}
	diags.Append(l.ElementsAs(ctx, &items, false)...)
	for _, it := range items {
		out = append(out, client.AlertsDestinationInput{ChannelId: it.ChannelID.ValueString(), MinSeverity: it.MinSeverity.ValueString()})
	}
	return out
}

func (r *alertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.AlertsCreateJSONRequestBody{
		Name:             plan.Name.ValueString(),
		Severity:         plan.Severity.ValueString(),
		SourceType:       plan.SourceType.ValueString(),
		ConditionKey:     plan.ConditionKey.ValueString(),
		Condition:        alertConditionBody(ctx, plan.Condition, &resp.Diagnostics),
		Target:           plan.Target.ValueString(),
		EnvironmentIds:   setStrings(ctx, plan.EnvironmentIDs, &resp.Diagnostics),
		Destinations:     alertDestinationsBody(ctx, plan.Destinations, &resp.Diagnostics),
		SendInApp:        plan.SendInApp.ValueBool(),
		NotifyOnRecovery: boolPtr(plan.NotifyOnRecovery),
		MessageTemplate:  plan.MessageTemplate.ValueString(),
	}
	if n := intPtr(plan.CooldownSeconds); n != nil {
		body.CooldownSeconds = *n
	}
	if resp.Diagnostics.HasError() {
		return
	}
	var a client.AlertsAlert
	if err := decode(apiCall(r.c.api.AlertsCreate(ctx, body)), "alertsCreate", &a); err != nil {
		addError(&resp.Diagnostics, "Creating alert rule", err)
		return
	}
	if a.Enabled != plan.Enabled.ValueBool() {
		enabled := plan.Enabled.ValueBool()
		if err := decode(apiCall(r.c.api.AlertsUpdate(ctx, a.Id, client.AlertsUpdateJSONRequestBody{Enabled: &enabled})), "alertsUpdate", &a); err != nil {
			addError(&resp.Diagnostics, "Setting alert rule enabled", err)
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, alertRuleFromAPI(a))...)
}

// findAlert reads one rule through the paginated list, the only read the API offers.
func (r *alertRuleResource) findAlert(ctx context.Context, id string) (*client.AlertsAlert, error) {
	all, err := listAll[client.AlertsAlert]("alertsList", func(page, perPage string) call {
		return apiCall(r.c.api.AlertsList(ctx, &client.AlertsListParams{Page: &page, PerPage: &perPage}))
	})
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Id == id {
			return &all[i], nil
		}
	}
	return nil, nil
}

func (r *alertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.findAlert(ctx, state.ID.ValueString())
	if err != nil {
		addError(&resp.Diagnostics, "Reading alert rule", err)
		return
	}
	if a == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, alertRuleFromAPI(*a))...)
}

func (r *alertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	condition := alertConditionBody(ctx, plan.Condition, &resp.Diagnostics)
	envs := setStrings(ctx, plan.EnvironmentIDs, &resp.Diagnostics)
	dests := alertDestinationsBody(ctx, plan.Destinations, &resp.Diagnostics)
	body := client.AlertsUpdateJSONRequestBody{
		Name:             strPtr(plan.Name),
		Severity:         strPtr(plan.Severity),
		SourceType:       strPtr(plan.SourceType),
		ConditionKey:     strPtr(plan.ConditionKey),
		Condition:        &condition,
		Target:           strPtr(plan.Target),
		EnvironmentIds:   &envs,
		Destinations:     &dests,
		SendInApp:        boolPtr(plan.SendInApp),
		CooldownSeconds:  intPtr(plan.CooldownSeconds),
		NotifyOnRecovery: boolPtr(plan.NotifyOnRecovery),
		MessageTemplate:  strPtr(plan.MessageTemplate),
		Enabled:          boolPtr(plan.Enabled),
	}
	if resp.Diagnostics.HasError() {
		return
	}
	var a client.AlertsAlert
	if err := decode(apiCall(r.c.api.AlertsUpdate(ctx, state.ID.ValueString(), body)), "alertsUpdate", &a); err != nil {
		addError(&resp.Diagnostics, "Updating alert rule", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, alertRuleFromAPI(a))...)
}

func (r *alertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.AlertsDelete(ctx, state.ID.ValueString())), "alertsDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting alert rule", err)
	}
}

func (r *alertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func alertRuleFromAPI(a client.AlertsAlert) alertRuleModel {
	params := types.MapValueMust(types.StringType, map[string]attr.Value{})
	if a.Condition.Params != nil {
		elems := map[string]attr.Value{}
		for k, v := range *a.Condition.Params {
			elems[k] = types.StringValue(v)
		}
		params = types.MapValueMust(types.StringType, elems)
	}
	threshold := types.Float64Value(0)
	if a.Condition.Threshold != nil {
		threshold = types.Float64Value(*a.Condition.Threshold)
	}
	dests := make([]attr.Value, 0, len(a.Destinations))
	for _, d := range a.Destinations {
		dests = append(dests, types.ObjectValueMust(destinationAttrTypes, map[string]attr.Value{
			"channel_id":   types.StringValue(d.ChannelId),
			"min_severity": types.StringValue(deref(d.MinSeverity)),
		}))
	}
	return alertRuleModel{
		ID:           types.StringValue(a.Id),
		Name:         types.StringValue(a.Name),
		Severity:     types.StringValue(a.Severity),
		SourceType:   types.StringValue(a.SourceType),
		ConditionKey: types.StringValue(a.ConditionKey),
		Condition: &alertConditionModel{
			Operator:      types.StringValue(deref(a.Condition.Operator)),
			Threshold:     threshold,
			Unit:          types.StringValue(deref(a.Condition.Unit)),
			ForSeconds:    int64Of(a.Condition.ForSeconds),
			WindowSeconds: int64Of(a.Condition.WindowSeconds),
			Params:        params,
		},
		Target:           types.StringValue(a.Target),
		EnvironmentIDs:   stringSet(a.EnvironmentIds),
		Destinations:     types.ListValueMust(types.ObjectType{AttrTypes: destinationAttrTypes}, dests),
		SendInApp:        types.BoolValue(a.SendInApp),
		CooldownSeconds:  types.Int64Value(int64(a.CooldownSeconds)),
		NotifyOnRecovery: types.BoolValue(a.NotifyOnRecovery),
		MessageTemplate:  types.StringValue(a.MessageTemplate),
		Enabled:          types.BoolValue(a.Enabled),
	}
}

// int64Of reads an omitempty integer as zero when absent, matching what the
// server stores, so a plan does not flip between null and 0.
func int64Of(v *int) types.Int64 {
	if v == nil {
		return types.Int64Value(0)
	}
	return types.Int64Value(int64(*v))
}
