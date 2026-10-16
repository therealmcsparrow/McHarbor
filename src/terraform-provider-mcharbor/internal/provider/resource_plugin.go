// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type pluginResource struct{ c *apiClient }

type pluginModel struct {
	ID      types.String `tfsdk:"id"`
	Slug    types.String `tfsdk:"slug"`
	Version types.String `tfsdk:"version"`
	Enabled types.Bool   `tfsdk:"enabled"`
	Purge   types.Bool   `tfsdk:"purge_on_destroy"`
	Loaded  types.Bool   `tfsdk:"loaded"`
	Signed  types.Bool   `tfsdk:"signed"`
}

var (
	_ resource.ResourceWithConfigure   = &pluginResource{}
	_ resource.ResourceWithImportState = &pluginResource{}
)

func newPluginResource() resource.Resource { return &pluginResource{} }

func (r *pluginResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin"
}

func (r *pluginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A plugin installed from the configured catalogs at an exact version. Changing the version installs " +
			"that version through the same cycle as a Store update, keeping settings and role grants. The version must " +
			"be one the catalog serves; see the mcharbor_plugin_catalog data source. Requires plugins.manage, and " +
			"plugins.delete or plugins.manage to remove.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"slug": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"version": schema.StringAttribute{Required: true, Description: "The exact catalog version to run."},
			"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"purge_on_destroy": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Also delete the plugin's data and tables when the resource is destroyed. Off by default, " +
					"so removing the resource keeps what the plugin stored for a later reinstall.",
			},
			"loaded": schema.BoolAttribute{
				Computed: true, Description: "Whether the running McHarbor has the plugin loaded.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"signed": schema.BoolAttribute{
				Computed: true, Description: "Whether the installed package carried a trusted signature.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *pluginResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

type installedPlugin struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
	Loaded  bool   `json:"loaded"`
	Signed  bool   `json:"signed"`
}

func (r *pluginResource) reconcile(ctx context.Context, plan pluginModel) error {
	enabled := plan.Enabled.ValueBool()
	return decode(apiCall(r.c.api.AppstorePluginsReconcile(ctx, plan.Slug.ValueString(), client.AppstorePluginsReconcileJSONRequestBody{
		Version: plan.Version.ValueString(), Enabled: &enabled,
	})), "appstorePluginsReconcile", nil)
}

func (r *pluginResource) get(ctx context.Context, slug string) (installedPlugin, error) {
	var p installedPlugin
	err := decode(apiCall(r.c.api.AppstorePluginsGet(ctx, slug)), "appstorePluginsGet", &p)
	return p, err
}

func pluginFromAPI(p installedPlugin, purge types.Bool) pluginModel {
	if purge.IsNull() || purge.IsUnknown() {
		purge = types.BoolValue(false)
	}
	return pluginModel{
		ID: types.StringValue(p.Slug), Slug: types.StringValue(p.Slug), Version: types.StringValue(p.Version),
		Enabled: types.BoolValue(p.Enabled), Purge: purge, Loaded: types.BoolValue(p.Loaded), Signed: types.BoolValue(p.Signed),
	}
}

func (r *pluginResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pluginModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.reconcile(ctx, plan); err != nil {
		addError(&resp.Diagnostics, "Installing plugin", err)
		return
	}
	p, err := r.get(ctx, plan.Slug.ValueString())
	if err != nil {
		addError(&resp.Diagnostics, "Reading plugin", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, pluginFromAPI(p, plan.Purge))...)
}

func (r *pluginResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pluginModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.get(ctx, state.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading plugin", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, pluginFromAPI(p, state.Purge))...)
}

func (r *pluginResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pluginModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.reconcile(ctx, plan); err != nil {
		addError(&resp.Diagnostics, "Updating plugin", err)
		return
	}
	p, err := r.get(ctx, plan.Slug.ValueString())
	if err != nil {
		addError(&resp.Diagnostics, "Reading plugin", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, pluginFromAPI(p, plan.Purge))...)
}

func (r *pluginResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pluginModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	purge := state.Purge.ValueBool()
	err := decode(apiCall(r.c.api.AppstorePluginsUninstall(ctx, client.AppstorePluginsUninstallJSONRequestBody{
		Slug: state.Slug.ValueString(), PurgeData: &purge,
	})), "appstorePluginsUninstall", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Uninstalling plugin", err)
	}
}

func (r *pluginResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("slug"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("purge_on_destroy"), false)...)
}
