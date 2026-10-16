// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type secretResource struct {
	c *apiClient
}

type secretModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Kind           types.String `tfsdk:"kind"`
	Group          types.String `tfsdk:"group"`
	Scope          types.String `tfsdk:"scope"`
	EnvironmentID  types.String `tfsdk:"environment_id"`
	StackName      types.String `tfsdk:"stack_name"`
	AutoApply      types.Bool   `tfsdk:"auto_apply"`
	ValueWO        types.String `tfsdk:"value_wo"`
	ValueWOVersion types.Int64  `tfsdk:"value_wo_version"`
	Generate       types.Bool   `tfsdk:"generate"`
	GenerateLength types.Int64  `tfsdk:"generate_length"`
	Version        types.Int64  `tfsdk:"version"`
	Generated      types.Bool   `tfsdk:"generated"`
}

var (
	_ resource.ResourceWithConfigure      = &secretResource{}
	_ resource.ResourceWithImportState    = &secretResource{}
	_ resource.ResourceWithValidateConfig = &secretResource{}
	_ resource.ResourceWithModifyPlan     = &secretResource{}
)

func newSecretResource() resource.Resource { return &secretResource{} }

func (r *secretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *secretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A secret in the McHarbor vault. The value never enters Terraform state or plan output: " +
			"supply it write-only through value_wo, let McHarbor generate it, or import a secret whose value was " +
			"set in McHarbor. Requires secrets.view to read, secrets.manage to create, rotate and change, and " +
			"secrets.delete to remove; a global or application scope also needs secrets.manage_global. " +
			"secrets.reveal is never used.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				PlanModifiers: replace,
				Description:   "Referenced as ${secret:NAME}. Changing it replaces the secret.",
			},
			"description": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("")},
			"kind": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("env"),
				Description: "env, password, token, cert, private_key, kubeconfig, ssh_key or json.",
			},
			"group": schema.StringAttribute{Optional: true, Description: "Free-text folder label in the vault listing."},
			"scope": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("global"),
				PlanModifiers: replace,
				Description:   "global, environment, stack or application. Changing it replaces the secret.",
			},
			"environment_id": schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"stack_name":     schema.StringAttribute{Optional: true, PlanModifiers: replace},
			"auto_apply": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Propagate a rotation to every referencing container and stack immediately.",
			},
			"value_wo": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
				Description: "Option A: the value, write-only (Terraform 1.11+). Sent on create, and on update only when " +
					"value_wo_version changes. Never stored in state.",
			},
			"value_wo_version": schema.Int64Attribute{
				Optional:    true,
				Description: "Bump to rotate: McHarbor stores a new secret version (from value_wo, or freshly generated when generate is true).",
			},
			"generate": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
				Description:   "Option B: McHarbor generates the value with crypto/rand. The one-time value in the response is discarded.",
			},
			"generate_length": schema.Int64Attribute{
				Optional:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Description:   "Byte length of a generated value, 16 to 128.",
			},
			"version": schema.Int64Attribute{
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Description:   "The current secret version; it increases on every rotation.",
			},
			"generated": schema.BoolAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *secretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

// ValidateSecretSource enforces that a secret's value comes from exactly one
// place; it is exported to the tests as the plan-time contract.
func ValidateSecretSource(valueSet, generate, creating bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if valueSet && generate {
		diags.AddAttributeError(path.Root("value_wo"), "Conflicting secret sources",
			"Set either value_wo (option A) or generate = true (option B), not both.")
	}
	if creating && !valueSet && !generate {
		diags.AddAttributeError(path.Root("value_wo"), "Missing secret value",
			"A new secret needs value_wo or generate = true. To manage an existing secret whose value was set in "+
				"McHarbor, import it: terraform import mcharbor_secret.<name> <id>.")
	}
	return diags
}

func (r *secretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg secretModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(ValidateSecretSource(!cfg.ValueWO.IsNull() && !cfg.ValueWO.IsUnknown(),
		cfg.Generate.ValueBool(), false)...)
}

func (r *secretResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var cfg, plan secretModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if req.State.Raw.IsNull() {
		resp.Diagnostics.Append(ValidateSecretSource(!cfg.ValueWO.IsNull(), cfg.Generate.ValueBool(), true)...)
		return
	}
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.ValueWOVersion.Equal(state.ValueWOVersion) && !plan.ValueWOVersion.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("version"), types.Int64Unknown())...)
	}
}

func (r *secretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.SecretsCreateJSONRequestBody{
		Name:          plan.Name.ValueString(),
		Description:   plan.Description.ValueString(),
		Kind:          plan.Kind.ValueString(),
		Group:         plan.Group.ValueString(),
		Scope:         plan.Scope.ValueString(),
		EnvironmentId: strPtr(plan.EnvironmentID),
		StackName:     strPtr(plan.StackName),
		AutoApply:     plan.AutoApply.ValueBool(),
		Generate:      plan.Generate.ValueBool(),
		Value:         cfg.ValueWO.ValueString(),
	}
	if n := intPtr(plan.GenerateLength); n != nil {
		body.GenerateLength = *n
	}
	params := &client.SecretsCreateParams{Env: strPtr(plan.EnvironmentID)}
	var raw rawData
	if err := decode(apiCall(r.c.api.SecretsCreate(ctx, params, body)), "secretsCreate", &raw); err != nil {
		addError(&resp.Diagnostics, "Creating secret", err)
		return
	}
	sec, err := secretFromResponse(raw)
	if err != nil {
		addError(&resp.Diagnostics, "Reading created secret", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, secretState(sec, plan))...)
}

func (r *secretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var sec client.SecretsSecret
	err := decode(apiCall(r.c.api.SecretsGet(ctx, state.ID.ValueString())), "secretsGet", &sec)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading secret", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, secretState(sec, state))...)
}

func (r *secretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	env := strPtr(plan.EnvironmentID)

	var sec client.SecretsSecret
	patch := client.SecretsPatchJSONRequestBody{
		Description: strPtr(plan.Description),
		Kind:        strPtr(plan.Kind),
		AutoApply:   boolPtr(plan.AutoApply),
	}
	group := plan.Group.ValueString()
	patch.Group = &group
	if err := decode(apiCall(r.c.api.SecretsPatch(ctx, id, &client.SecretsPatchParams{Env: env}, patch)), "secretsPatch", &sec); err != nil {
		addError(&resp.Diagnostics, "Updating secret", err)
		return
	}

	if !plan.ValueWOVersion.Equal(state.ValueWOVersion) && !plan.ValueWOVersion.IsNull() {
		rotate := client.SecretsRotateJSONRequestBody{Generate: plan.Generate.ValueBool()}
		if rotate.Generate {
			if n := intPtr(plan.GenerateLength); n != nil {
				rotate.GenerateLength = *n
			}
		} else {
			if cfg.ValueWO.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root("value_wo"), "Missing secret value",
					"value_wo_version changed, which rotates the secret, but value_wo is not set.")
				return
			}
			rotate.Value = cfg.ValueWO.ValueString()
		}
		var raw rawData
		if err := decode(apiCall(r.c.api.SecretsRotate(ctx, id, &client.SecretsRotateParams{Env: env}, rotate)), "secretsRotate", &raw); err != nil {
			addError(&resp.Diagnostics, "Rotating secret", err)
			return
		}
		rotated, err := secretFromResponse(raw)
		if err != nil {
			addError(&resp.Diagnostics, "Reading rotated secret", err)
			return
		}
		sec = rotated
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, secretState(sec, plan))...)
}

func (r *secretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.SecretsDelete(ctx, state.ID.ValueString(), &client.SecretsDeleteParams{})), "secretsDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting secret", err)
	}
}

func (r *secretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// secretFromResponse reads a create or rotate payload, which is the bare secret
// or, when McHarbor generated the value, {secret, value}. The value is dropped here.
func secretFromResponse(raw rawData) (client.SecretsSecret, error) {
	var wrapped struct {
		Secret *client.SecretsSecret `json:"secret"`
	}
	if err := raw.into(&wrapped); err != nil {
		return client.SecretsSecret{}, err
	}
	if wrapped.Secret != nil {
		return *wrapped.Secret, nil
	}
	var sec client.SecretsSecret
	err := raw.into(&sec)
	return sec, err
}

// secretState maps the server's metadata onto state. It is the only place a
// secret model is built from the API, and it never carries a value.
func secretState(sec client.SecretsSecret, prior secretModel) secretModel {
	description := ""
	if sec.Description != nil {
		description = *sec.Description
	}
	generate := prior.Generate
	if generate.IsNull() || generate.IsUnknown() {
		generate = types.BoolValue(false)
	}
	return secretModel{
		ID:             types.StringValue(sec.Id),
		Name:           types.StringValue(sec.Name),
		Description:    types.StringValue(description),
		Kind:           types.StringValue(sec.Kind),
		Group:          optString(sec.Group),
		Scope:          types.StringValue(sec.Scope),
		EnvironmentID:  optString(sec.EnvironmentId),
		StackName:      optString(sec.StackName),
		AutoApply:      types.BoolValue(sec.AutoApply),
		ValueWO:        types.StringNull(),
		ValueWOVersion: prior.ValueWOVersion,
		Generate:       generate,
		GenerateLength: prior.GenerateLength,
		Version:        types.Int64Value(int64(sec.Version)),
		Generated:      types.BoolValue(sec.Generated),
	}
}
