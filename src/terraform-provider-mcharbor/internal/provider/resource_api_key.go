// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type apiKeyResource struct {
	c *apiClient
}

type apiKeyModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	RoleIDs        types.Set    `tfsdk:"role_ids"`
	Scopes         types.Set    `tfsdk:"scopes"`
	EnvironmentIDs types.Set    `tfsdk:"environment_ids"`
	ExpiresAt      types.String `tfsdk:"expires_at"`
	Key            types.String `tfsdk:"key"`
	KeyPrefix      types.String `tfsdk:"key_prefix"`
	UserID         types.String `tfsdk:"user_id"`
}

type apiKeySecret struct {
	Key string `json:"key"`
}

var (
	_ resource.ResourceWithConfigure   = &apiKeyResource{}
	_ resource.ResourceWithImportState = &apiKeyResource{}
)

func newAPIKeyResource() resource.Resource { return &apiKeyResource{} }

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	emptySet := setdefault.StaticValue(stringSet(nil))
	resp.Schema = schema.Schema{
		Description: "An API key owned by the provider's own user, scoped by roles, permission keys and environments. " +
			"Destroy revokes the key; McHarbor keeps the revoked row for audit. Requires api_keys.manage.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true},
			"role_ids": schema.SetAttribute{
				Required: true, ElementType: types.StringType,
				Description: "Roles the key acts with. The caller must hold everything it grants.",
			},
			"scopes": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, Default: emptySet,
				Description: "Permission keys that further narrow the roles; empty means the roles apply in full.",
			},
			"environment_ids": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType, Default: emptySet,
				Description: "Environments the key may touch; empty means all the roles allow.",
			},
			"expires_at": schema.StringAttribute{
				Optional:      true,
				Description:   "RFC 3339 expiry. McHarbor cannot change it after creation, so a change replaces the key.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"key": schema.StringAttribute{
				Computed:      true,
				Sensitive:     true,
				Description:   "The secret. McHarbor returns it once, at creation, and it is stored in Terraform state. Null after import.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key_prefix": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.ApiKeysCreateJSONRequestBody{
		Name:           plan.Name.ValueString(),
		RoleIds:        setStrings(ctx, plan.RoleIDs, &resp.Diagnostics),
		Scopes:         setStrings(ctx, plan.Scopes, &resp.Diagnostics),
		EnvironmentIds: setStrings(ctx, plan.EnvironmentIDs, &resp.Diagnostics),
		ExpiresAt:      strPtr(plan.ExpiresAt),
	}
	c := apiCall(r.c.api.ApiKeysCreate(ctx, body))
	var raw rawData
	if err := decode(c, "apiKeysCreate", &raw); err != nil {
		addError(&resp.Diagnostics, "Creating API key", err)
		return
	}
	// The create result embeds the key record and adds the one-time secret
	// beside it, flat in one object.
	var key client.ApiKeysAPIKey
	var secret apiKeySecret
	if err := raw.into(&key, &secret); err != nil {
		addError(&resp.Diagnostics, "Decoding the created API key", err)
		return
	}
	state := apiKeyFromAPI(key, plan)
	state.Key = types.StringValue(secret.Key)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var key client.ApiKeysAPIKey
	err := decode(apiCall(r.c.api.ApiKeysDetail(ctx, state.ID.ValueString())), "apiKeysDetail", &key)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading API key", err)
		return
	}
	if !key.IsActive {
		resp.State.RemoveResource(ctx)
		return
	}
	next := apiKeyFromAPI(key, state)
	next.Key = state.Key
	if next.Key.IsUnknown() {
		next.Key = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *apiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state apiKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.ApiKeysUpdateJSONRequestBody{
		Name:           strPtr(plan.Name),
		RoleIds:        setStrings(ctx, plan.RoleIDs, &resp.Diagnostics),
		Scopes:         setStrings(ctx, plan.Scopes, &resp.Diagnostics),
		EnvironmentIds: setStrings(ctx, plan.EnvironmentIDs, &resp.Diagnostics),
	}
	var key client.ApiKeysAPIKey
	if err := decode(apiCall(r.c.api.ApiKeysUpdate(ctx, state.ID.ValueString(), body)), "apiKeysUpdate", &key); err != nil {
		addError(&resp.Diagnostics, "Updating API key", err)
		return
	}
	next := apiKeyFromAPI(key, plan)
	next.Key = state.Key
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.ApiKeysRevoke(ctx, state.ID.ValueString())), "apiKeysRevoke", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Revoking API key", err)
	}
}

func (r *apiKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// apiKeyFromAPI maps the server record, keeping expires_at from prior so a
// server-reformatted timestamp never reads as drift.
func apiKeyFromAPI(key client.ApiKeysAPIKey, prior apiKeyModel) apiKeyModel {
	roleIDs := make([]string, 0, len(key.Roles))
	for _, role := range key.Roles {
		roleIDs = append(roleIDs, role.Id)
	}
	expires := prior.ExpiresAt
	if expires.IsUnknown() || (expires.IsNull() && key.ExpiresAt != nil) {
		expires = optString(key.ExpiresAt)
	}
	return apiKeyModel{
		ID:             types.StringValue(key.Id),
		Name:           types.StringValue(key.Name),
		RoleIDs:        stringSet(roleIDs),
		Scopes:         stringSet(key.Scopes),
		EnvironmentIDs: stringSet(key.EnvironmentIds),
		ExpiresAt:      expires,
		KeyPrefix:      types.StringValue(key.KeyPrefix),
		UserID:         types.StringValue(key.UserId),
		Key:            prior.Key,
	}
}
