// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type backupPlanResource struct{ c *apiClient }

type backupPlanModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	EnvironmentID      types.String `tfsdk:"environment_id"`
	ContainerID        types.String `tfsdk:"container_id"`
	ContainerName      types.String `tfsdk:"container_name"`
	StorageLocationIDs types.Set    `tfsdk:"storage_location_ids"`
	IncludeConfig      types.Bool   `tfsdk:"include_config"`
	IncludeLogs        types.Bool   `tfsdk:"include_logs"`
	IncludeFilesystem  types.Bool   `tfsdk:"include_filesystem"`
	IncludeImage       types.Bool   `tfsdk:"include_image"`
	SelectedMounts     types.Set    `tfsdk:"selected_mounts"`
	LogTailLines       types.Int64  `tfsdk:"log_tail_lines"`
	Cron               types.String `tfsdk:"cron"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	RetentionCount     types.Int64  `tfsdk:"retention_count"`
	RetentionDays      types.Int64  `tfsdk:"retention_days"`
	ConsistencyMode    types.String `tfsdk:"consistency_mode"`
	SnapshotConfigID   types.String `tfsdk:"snapshot_config_id"`
	NextRunAt          types.String `tfsdk:"next_run_at"`
}

var (
	_ resource.ResourceWithConfigure   = &backupPlanResource{}
	_ resource.ResourceWithImportState = &backupPlanResource{}
)

func newBackupPlanResource() resource.Resource { return &backupPlanResource{} }

func (r *backupPlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_plan"
}

func (r *backupPlanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keepS := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keepB := []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
	keepI := []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A container backup plan: what to archive, where to, when, and how long to keep it. Per-mount " +
			"selections are managed in the UI and left untouched. Requires backups.view to read, backups.manage " +
			"to create and change, and backups.delete to remove.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"name":           schema.StringAttribute{Required: true},
			"environment_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"container_id":   schema.StringAttribute{Required: true, PlanModifiers: replace},
			"container_name": schema.StringAttribute{Computed: true, PlanModifiers: keepS},
			"storage_location_ids": schema.SetAttribute{
				Required: true, ElementType: types.StringType,
				Description: "See the mcharbor_storage_location resource.",
			},
			"include_config":     schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB},
			"include_logs":       schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB},
			"include_filesystem": schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB},
			"include_image":      schema.BoolAttribute{Optional: true, Computed: true, PlanModifiers: keepB},
			"selected_mounts": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default: setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
			},
			"log_tail_lines":     schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI},
			"cron":               schema.StringAttribute{Optional: true, Description: "Five-field cron expression; omit for manual runs only."},
			"enabled":            schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"retention_count":    schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI},
			"retention_days":     schema.Int64Attribute{Optional: true, Computed: true, PlanModifiers: keepI},
			"consistency_mode":   schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: keepS, Description: "pause, snapshot or live."},
			"snapshot_config_id": schema.StringAttribute{Optional: true},
			"next_run_at":        schema.StringAttribute{Computed: true, PlanModifiers: keepS},
		},
	}
}

func (r *backupPlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *backupPlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan backupPlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.ContainerBackupsCreatePlanJSONRequestBody{
		Name:               plan.Name.ValueString(),
		ContainerId:        plan.ContainerID.ValueString(),
		StorageLocationIds: setStrings(ctx, plan.StorageLocationIDs, &resp.Diagnostics),
		IncludeConfig:      plan.IncludeConfig.ValueBool(),
		IncludeLogs:        plan.IncludeLogs.ValueBool(),
		IncludeFilesystem:  plan.IncludeFilesystem.ValueBool(),
		IncludeImage:       plan.IncludeImage.ValueBool(),
		SelectedMounts:     setStrings(ctx, plan.SelectedMounts, &resp.Diagnostics),
		Cron:               plan.Cron.ValueString(),
		Enabled:            plan.Enabled.ValueBool(),
		ConsistencyMode:    plan.ConsistencyMode.ValueString(),
		SnapshotConfigId:   plan.SnapshotConfigID.ValueString(),
	}
	if n := intPtr(plan.LogTailLines); n != nil {
		body.LogTailLines = *n
	}
	if n := intPtr(plan.RetentionCount); n != nil {
		body.RetentionCount = *n
	}
	if n := intPtr(plan.RetentionDays); n != nil {
		body.RetentionDays = *n
	}
	if resp.Diagnostics.HasError() {
		return
	}
	env := plan.EnvironmentID.ValueString()
	var p client.ContainerBackupsBackupPlan
	if err := decode(apiCall(r.c.api.ContainerBackupsCreatePlan(ctx, &client.ContainerBackupsCreatePlanParams{Env: &env}, body)), "containerBackupsCreatePlan", &p); err != nil {
		addError(&resp.Diagnostics, "Creating backup plan", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, backupPlanState(p, plan))...)
}

// findPlan reads one plan through the environment's plan list, the only read the API offers.
func (r *backupPlanResource) findPlan(ctx context.Context, envID, id string) (*client.ContainerBackupsBackupPlan, error) {
	var plans []client.ContainerBackupsBackupPlan
	err := decode(apiCall(r.c.api.ContainerBackupsListPlans(ctx, &client.ContainerBackupsListPlansParams{Env: &envID})), "containerBackupsListPlans", &plans)
	if err != nil {
		return nil, err
	}
	for i := range plans {
		if plans[i].Id == id {
			return &plans[i], nil
		}
	}
	return nil, nil
}

func (r *backupPlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state backupPlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.findPlan(ctx, state.EnvironmentID.ValueString(), state.ID.ValueString())
	if err != nil {
		addError(&resp.Diagnostics, "Reading backup plan", err)
		return
	}
	if p == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, backupPlanState(*p, state))...)
}

func (r *backupPlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state backupPlanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	locations := setStrings(ctx, plan.StorageLocationIDs, &resp.Diagnostics)
	mounts := setStrings(ctx, plan.SelectedMounts, &resp.Diagnostics)
	cron := plan.Cron.ValueString()
	snapshot := plan.SnapshotConfigID.ValueString()
	body := client.ContainerBackupsUpdatePlanJSONRequestBody{
		Name:               strPtr(plan.Name),
		StorageLocationIds: &locations,
		IncludeConfig:      boolPtr(plan.IncludeConfig),
		IncludeLogs:        boolPtr(plan.IncludeLogs),
		IncludeFilesystem:  boolPtr(plan.IncludeFilesystem),
		IncludeImage:       boolPtr(plan.IncludeImage),
		SelectedMounts:     &mounts,
		LogTailLines:       intPtr(plan.LogTailLines),
		Cron:               &cron,
		Enabled:            boolPtr(plan.Enabled),
		RetentionCount:     intPtr(plan.RetentionCount),
		RetentionDays:      intPtr(plan.RetentionDays),
		ConsistencyMode:    strPtr(plan.ConsistencyMode),
		SnapshotConfigId:   &snapshot,
	}
	if resp.Diagnostics.HasError() {
		return
	}
	var p client.ContainerBackupsBackupPlan
	if err := decode(apiCall(r.c.api.ContainerBackupsUpdatePlan(ctx, state.ID.ValueString(), body)), "containerBackupsUpdatePlan", &p); err != nil {
		addError(&resp.Diagnostics, "Updating backup plan", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, backupPlanState(p, plan))...)
}

func (r *backupPlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state backupPlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.ContainerBackupsDeletePlan(ctx, state.ID.ValueString())), "containerBackupsDeletePlan", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting backup plan", err)
	}
}

func (r *backupPlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	envID, id, ok := strings.Cut(req.ID, "/")
	if !ok || envID == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <environment_id>/<plan_id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathRoot("environment_id"), envID)...)
}

// backupPlanState maps the server's plan onto state. When Docker recreated the
// container, McHarbor refreshes its id; the configured id is kept while the
// container name is unchanged, so a recreate does not force a replacement.
func backupPlanState(p client.ContainerBackupsBackupPlan, prior backupPlanModel) backupPlanModel {
	containerID := types.StringValue(p.ContainerId)
	if !prior.ContainerID.IsNull() && !prior.ContainerID.IsUnknown() && prior.ContainerName.ValueString() == p.ContainerName &&
		prior.ContainerID.ValueString() != p.ContainerId && p.ContainerIdStale != nil && *p.ContainerIdStale {
		containerID = prior.ContainerID
	}
	return backupPlanModel{
		ID:                 types.StringValue(p.Id),
		Name:               types.StringValue(p.Name),
		EnvironmentID:      types.StringValue(p.EnvironmentId),
		ContainerID:        containerID,
		ContainerName:      types.StringValue(p.ContainerName),
		StorageLocationIDs: stringSet(p.StorageLocationIds),
		IncludeConfig:      types.BoolValue(p.IncludeConfig),
		IncludeLogs:        types.BoolValue(p.IncludeLogs),
		IncludeFilesystem:  types.BoolValue(p.IncludeFilesystem),
		IncludeImage:       types.BoolValue(p.IncludeImage),
		SelectedMounts:     stringSet(p.SelectedMounts),
		LogTailLines:       types.Int64Value(int64(p.LogTailLines)),
		Cron:               optString(p.Cron),
		Enabled:            types.BoolValue(p.Enabled),
		RetentionCount:     types.Int64Value(int64(p.RetentionCount)),
		RetentionDays:      types.Int64Value(int64(p.RetentionDays)),
		ConsistencyMode:    types.StringValue(deref(p.ConsistencyMode)),
		SnapshotConfigID:   optString(p.SnapshotConfigId),
		NextRunAt:          types.StringValue(deref(p.NextRunAt)),
	}
}
