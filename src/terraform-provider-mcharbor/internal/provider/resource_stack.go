// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

// envMaskSentinel is what McHarbor returns in place of a secret stack variable.
// A read that sees it keeps the configured value instead of reporting drift.
const envMaskSentinel = "••••••••"

type stackResource struct {
	c *apiClient
}

type stackModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	EnvironmentID  types.String `tfsdk:"environment_id"`
	Compose        types.String `tfsdk:"compose"`
	Description    types.String `tfsdk:"description"`
	EnvVars        types.Map    `tfsdk:"env_vars"`
	AutoStart      types.Bool   `tfsdk:"auto_start"`
	SecretFileMode types.Bool   `tfsdk:"secret_file_mode"`
}

type composeContent struct {
	Content string `json:"content"`
}

var (
	_ resource.ResourceWithConfigure   = &stackResource{}
	_ resource.ResourceWithImportState = &stackResource{}
)

func newStackResource() resource.Resource { return &stackResource{} }

func (r *stackResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stack"
}

func (r *stackResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Compose stack on one environment. Requires stacks.view to read, stacks.manage to create and change, " +
			"and stacks.delete to destroy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "McHarbor's stack id. Import with <environment_id>/<name>.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true},
			"environment_id": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"compose": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				Description: "The compose file content, stored in Terraform state. It is read back with reveal, which needs " +
					"stacks.manage on the stack; without it, or where McHarbor withholds its own keys, the read carries masked " +
					"values and the next plan shows a change.",
			},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString(""),
			},
			"env_vars": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "Stack environment variables, stored in Terraform state. McHarbor masks secret ones on read; a masked value keeps the configured one.",
			},
			"auto_start": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Deploy the stack as soon as it is created. Only read at creation.",
			},
			"secret_file_mode": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Mount secret references as files rather than environment variables.",
			},
		},
	}
}

func (r *stackResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *stackResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan stackModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.StacksCreateJSONRequestBody{
		Name:           plan.Name.ValueString(),
		Compose:        plan.Compose.ValueString(),
		AutoStart:      plan.AutoStart.ValueBool(),
		Description:    strPtr(plan.Description),
		EnvironmentId:  strPtr(plan.EnvironmentID),
		SecretFileMode: boolPtr(plan.SecretFileMode),
	}
	if vars := stringMap(ctx, plan.EnvVars, &resp.Diagnostics); vars != nil {
		body.EnvVars = &vars
	}
	var st client.StacksStack
	if err := decode(apiCall(r.c.api.StacksCreate(ctx, body)), "stacksCreate", &st); err != nil {
		addError(&resp.Diagnostics, "Creating stack", err)
		return
	}
	plan.ID = types.StringValue(st.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *stackResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state stackModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := state.Name.ValueString()
	envID := state.EnvironmentID.ValueString()

	var st client.StacksStack
	err := decode(apiCall(r.c.api.StacksGetDetail(ctx, name, &client.StacksGetDetailParams{Env: &envID})), "stacksGetDetail", &st)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading stack", err)
		return
	}
	var compose composeContent
	// The masked view would read back as drift from the configured file on every plan; the
	// provider manages the stack, so it holds stacks.manage and reads the values it wrote.
	reveal := "true"
	if err := decode(apiCall(r.c.api.StacksGetCompose(ctx, name, &client.StacksGetComposeParams{Reveal: &reveal}, withQuery("env", envID))), "stacksGetCompose", &compose); err != nil {
		addError(&resp.Diagnostics, "Reading stack compose", err)
		return
	}
	var vars map[string]string
	if err := decode(apiCall(r.c.api.StacksGetEnvVars(ctx, name, &client.StacksGetEnvVarsParams{}, withQuery("env", envID))), "stacksGetEnvVars", &vars); err != nil {
		addError(&resp.Diagnostics, "Reading stack environment variables", err)
		return
	}

	state.ID = types.StringValue(st.Id)
	state.Name = types.StringValue(st.Name)
	if st.EnvironmentId != nil && *st.EnvironmentId != "" {
		state.EnvironmentID = types.StringValue(*st.EnvironmentId)
	}
	state.Compose = types.StringValue(compose.Content)
	state.Description = types.StringValue(deref(st.Description))
	state.SecretFileMode = types.BoolValue(st.SecretFileMode)
	state.EnvVars = mergeEnvVars(ctx, state.EnvVars, vars, &resp.Diagnostics)
	if state.AutoStart.IsNull() || state.AutoStart.IsUnknown() {
		state.AutoStart = types.BoolValue(true)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// mergeEnvVars takes the server's variables, but keeps the configured value
// wherever the server returned the mask, since the real value is unreadable.
func mergeEnvVars(ctx context.Context, prior types.Map, server map[string]string, diags *diag.Diagnostics) types.Map {
	configured := stringMap(ctx, prior, diags)
	if len(server) == 0 && configured == nil {
		return types.MapNull(types.StringType)
	}
	out := make(map[string]attr.Value, len(server))
	for key, value := range server {
		if value == envMaskSentinel {
			if mine, ok := configured[key]; ok {
				value = mine
			}
		}
		out[key] = types.StringValue(value)
	}
	return types.MapValueMust(types.StringType, out)
}

func (r *stackResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state stackModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.StacksUpdateJSONRequestBody{
		Name:           strPtr(plan.Name),
		Compose:        strPtr(plan.Compose),
		Description:    strPtr(plan.Description),
		SecretFileMode: boolPtr(plan.SecretFileMode),
	}
	vars := stringMap(ctx, plan.EnvVars, &resp.Diagnostics)
	if vars == nil && !state.EnvVars.IsNull() {
		vars = map[string]string{}
	}
	if vars != nil {
		body.EnvVars = &vars
	}
	var st client.StacksStack
	if err := decode(apiCall(r.c.api.StacksUpdate(ctx, state.Name.ValueString(), body, withQuery("env", state.EnvironmentID.ValueString()))), "stacksUpdate", &st); err != nil {
		addError(&resp.Diagnostics, "Updating stack", err)
		return
	}
	plan.ID = types.StringValue(st.Id)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *stackResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state stackModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.StacksDelete(ctx, state.Name.ValueString(), &client.StacksDeleteParams{Env: state.EnvironmentID.ValueStringPointer()})), "stacksDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting stack", err)
	}
}

func (r *stackResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	envID, name, ok := strings.Cut(req.ID, "/")
	if !ok || envID == "" || name == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <environment_id>/<name>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, stackModel{
		ID:             types.StringNull(),
		Name:           types.StringValue(name),
		EnvironmentID:  types.StringValue(envID),
		Compose:        types.StringNull(),
		Description:    types.StringNull(),
		EnvVars:        types.MapNull(types.StringType),
		AutoStart:      types.BoolValue(true),
		SecretFileMode: types.BoolNull(),
	})...)
}
