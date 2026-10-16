// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type workflowResource struct{ c *apiClient }

type workflowModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	Canvas      types.String `tfsdk:"canvas_json"`
	Variables   types.String `tfsdk:"variables_json"`
}

var (
	_ resource.ResourceWithConfigure   = &workflowResource{}
	_ resource.ResourceWithImportState = &workflowResource{}
)

func newWorkflowResource() resource.Resource { return &workflowResource{} }

func (r *workflowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *workflowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A workflow. Its graph is JSON, not HCL: keep the canvas exported from the workflow editor in a file " +
			"and pass it with `file(\"workflow.json\")`. JSON that differs only in formatting is not a change. Requires " +
			"workflows.manage to create and change, workflows.delete to remove.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":        schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"status": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("draft"),
				Description: "draft or active. Only an active workflow's triggers fire.",
				Validators:  []validator.String{oneOf("draft", "active")},
			},
			"canvas_json": schema.StringAttribute{
				Required:      true,
				Description:   "The workflow graph as JSON, usually file(\"workflow.json\").",
				Validators:    []validator.String{jsonValidator{}},
				PlanModifiers: []planmodifier.String{jsonEquivalent{}},
			},
			"variables_json": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("{}"),
				Description:   "Workflow variables as a JSON object.",
				Validators:    []validator.String{jsonValidator{}},
				PlanModifiers: []planmodifier.String{jsonEquivalent{}},
			},
		},
	}
}

func (r *workflowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *workflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workflowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var w client.WorkflowsWorkflow
	if err := decode(apiCall(r.c.api.WorkflowsCreate(ctx, client.WorkflowsCreateJSONRequestBody{
		Name: plan.Name.ValueString(), Description: plan.Description.ValueString(),
	})), "workflowsCreate", &w); err != nil {
		addError(&resp.Diagnostics, "Creating workflow", err)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), w.Id)...)
	if err := r.put(ctx, w.Id, plan, &w); err != nil {
		addError(&resp.Diagnostics, "Setting the workflow graph", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workflowFromAPI(w, plan))...)
}

func (r *workflowResource) put(ctx context.Context, id string, plan workflowModel, out *client.WorkflowsWorkflow) error {
	return decode(apiCall(r.c.api.WorkflowsUpdate(ctx, id, client.WorkflowsUpdateJSONRequestBody{
		Name: strPtr(plan.Name), Description: strPtr(plan.Description), Status: strPtr(plan.Status),
		CanvasData: strPtr(plan.Canvas), Variables: strPtr(plan.Variables),
	})), "workflowsUpdate", out)
}

func (r *workflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workflowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var w client.WorkflowsWorkflow
	err := decode(apiCall(r.c.api.WorkflowsGet(ctx, state.ID.ValueString())), "workflowsGet", &w)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading workflow", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workflowFromAPI(w, state))...)
}

func (r *workflowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state workflowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var w client.WorkflowsWorkflow
	if err := r.put(ctx, state.ID.ValueString(), plan, &w); err != nil {
		addError(&resp.Diagnostics, "Updating workflow", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workflowFromAPI(w, plan))...)
}

func (r *workflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state workflowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.WorkflowsDelete(ctx, state.ID.ValueString())), "workflowsDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting workflow", err)
	}
}

func (r *workflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// workflowFromAPI keeps the configured JSON text when the server's is the
// same document, so formatting never shows up as drift.
func workflowFromAPI(w client.WorkflowsWorkflow, prior workflowModel) workflowModel {
	return workflowModel{
		ID:          types.StringValue(w.Id),
		Name:        types.StringValue(w.Name),
		Description: types.StringValue(w.Description),
		Status:      types.StringValue(w.Status),
		Canvas:      types.StringValue(sameJSONOr(prior.Canvas, w.CanvasData)),
		Variables:   types.StringValue(sameJSONOr(prior.Variables, w.Variables)),
	}
}

func sameJSONOr(prior types.String, server string) string {
	if !prior.IsNull() && !prior.IsUnknown() && jsonEqual(prior.ValueString(), server) {
		return prior.ValueString()
	}
	return server
}

func jsonEqual(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return a == b
	}
	xa, errA := json.Marshal(x)
	ya, errB := json.Marshal(y)
	return errA == nil && errB == nil && bytes.Equal(xa, ya)
}

// jsonEquivalent plans no change when the configured JSON is the stored
// document in another format.
type jsonEquivalent struct{}

func (jsonEquivalent) Description(context.Context) string {
	return "Formatting-only differences in JSON are not changes."
}
func (m jsonEquivalent) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

func (jsonEquivalent) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		return
	}
	if jsonEqual(req.StateValue.ValueString(), req.PlanValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

// jsonValidator rejects text that is not JSON at plan time.
type jsonValidator struct{}

func (jsonValidator) Description(context.Context) string { return "must be valid JSON" }
func (v jsonValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (jsonValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !json.Valid([]byte(req.ConfigValue.ValueString())) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON", "The value is not valid JSON.")
	}
}
