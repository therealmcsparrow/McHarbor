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

type storageLocationResource struct{ c *apiClient }

type storageLocationModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	LocationType       types.String `tfsdk:"location_type"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	Host               types.String `tfsdk:"host"`
	Port               types.Int64  `tfsdk:"port"`
	BasePath           types.String `tfsdk:"base_path"`
	Region             types.String `tfsdk:"region"`
	Bucket             types.String `tfsdk:"bucket"`
	Endpoint           types.String `tfsdk:"endpoint"`
	TenantID           types.String `tfsdk:"tenant_id"`
	SiteURL            types.String `tfsdk:"site_url"`
	DriveID            types.String `tfsdk:"drive_id"`
	ShareName          types.String `tfsdk:"share_name"`
	Domain             types.String `tfsdk:"domain"`
	Username           types.String `tfsdk:"username"`
	AuthMethod         types.String `tfsdk:"auth_method"`
	TLSMode            types.String `tfsdk:"tls_mode"`
	PassiveMode        types.Bool   `tfsdk:"passive_mode"`
	CredentialsVersion types.Int64  `tfsdk:"credentials_version"`
	Password           types.String `tfsdk:"password_wo"`
	PrivateKey         types.String `tfsdk:"private_key_wo"`
	Passphrase         types.String `tfsdk:"passphrase_wo"`
	CACertificate      types.String `tfsdk:"ca_certificate_wo"`
	ClientCertificate  types.String `tfsdk:"client_certificate_wo"`
	ClientKey          types.String `tfsdk:"client_key_wo"`
	AccessKeyID        types.String `tfsdk:"access_key_id_wo"`
	SecretAccessKey    types.String `tfsdk:"secret_access_key_wo"`
	ClientID           types.String `tfsdk:"client_id_wo"`
	ClientSecret       types.String `tfsdk:"client_secret_wo"`
	RefreshToken       types.String `tfsdk:"refresh_token_wo"`
	Token              types.String `tfsdk:"token_wo"`
	Config             types.Map    `tfsdk:"config_wo"`
}

var storageCredentialAttrs = []string{
	"password_wo", "private_key_wo", "passphrase_wo", "ca_certificate_wo", "client_certificate_wo", "client_key_wo",
	"access_key_id_wo", "secret_access_key_wo", "client_id_wo", "client_secret_wo", "refresh_token_wo", "token_wo",
}

var storagePlainAttrs = []string{
	"host", "base_path", "region", "bucket", "endpoint", "tenant_id", "site_url", "drive_id", "share_name",
	"domain", "username", "auth_method", "tls_mode",
}

var (
	_ resource.ResourceWithConfigure   = &storageLocationResource{}
	_ resource.ResourceWithImportState = &storageLocationResource{}
)

func newStorageLocationResource() resource.Resource { return &storageLocationResource{} }

func (r *storageLocationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_storage_location"
}

func (r *storageLocationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{Required: true},
		"location_type": schema.StringAttribute{
			Required:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			Description:   "For example sftp, s3, smb, webdav or onedrive_business; the list depends on installed storage plugins, plus local.",
		},
		"enabled": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "The built-in local location (default-local-backup) cannot be disabled."},
		"port":    schema.Int64Attribute{Optional: true},
		"passive_mode": schema.BoolAttribute{
			Optional: true, Computed: true,
			PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
		},
		"credentials_version": schema.Int64Attribute{
			Optional:    true,
			Description: "Bump to send the *_wo credentials and config_wo again; they are write-only and cannot be diffed.",
		},
		"config_wo": schema.MapAttribute{
			Optional: true, Sensitive: true, WriteOnly: true, ElementType: types.StringType,
			Description: "Plugin-owned fields the storage plugin's form emits. Stored encrypted by McHarbor, never read back.",
		},
	}
	for _, name := range storagePlainAttrs {
		attrs[name] = schema.StringAttribute{Optional: true, Description: "Omit rather than set to \"\"; McHarbor does not echo empty values."}
	}
	for _, name := range storageCredentialAttrs {
		attrs[name] = schema.StringAttribute{
			Optional: true, Sensitive: true, WriteOnly: true,
			Description: "Write-only (Terraform 1.11+): sent on create and when credentials_version changes, never stored in state.",
		}
	}
	resp.Schema = schema.Schema{
		Description: "A storage location for backups. Credentials are write-only and never land in state. Requires " +
			"settings.view or storage_locations.view to read, settings.manage or storage_locations.manage to create " +
			"and change, and settings.manage or storage_locations.delete to remove.",
		Attributes: attrs,
	}
}

func (r *storageLocationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *storageLocationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg storageLocationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.StorageLocationsCreateJSONRequestBody{
		Name: plan.Name.ValueString(), LocationType: plan.LocationType.ValueString(), Enabled: boolPtr(plan.Enabled),
		Host: plan.Host.ValueString(), BasePath: plan.BasePath.ValueString(), Region: plan.Region.ValueString(),
		Bucket: plan.Bucket.ValueString(), Endpoint: plan.Endpoint.ValueString(), TenantId: plan.TenantID.ValueString(),
		SiteUrl: plan.SiteURL.ValueString(), DriveId: plan.DriveID.ValueString(), ShareName: plan.ShareName.ValueString(),
		Domain: plan.Domain.ValueString(), Username: plan.Username.ValueString(), AuthMethod: plan.AuthMethod.ValueString(),
		TlsMode: plan.TLSMode.ValueString(), PassiveMode: plan.PassiveMode.ValueBool(),
		Password: cfg.Password.ValueString(), PrivateKey: cfg.PrivateKey.ValueString(), Passphrase: cfg.Passphrase.ValueString(),
		CaCertificate: cfg.CACertificate.ValueString(), ClientCertificate: cfg.ClientCertificate.ValueString(),
		ClientKey: cfg.ClientKey.ValueString(), AccessKeyId: cfg.AccessKeyID.ValueString(),
		SecretAccessKey: cfg.SecretAccessKey.ValueString(), ClientId: cfg.ClientID.ValueString(),
		ClientSecret: cfg.ClientSecret.ValueString(), RefreshToken: cfg.RefreshToken.ValueString(), Token: cfg.Token.ValueString(),
	}
	if n := intPtr(plan.Port); n != nil {
		body.Port = *n
	}
	if m := stringMap(ctx, cfg.Config, &resp.Diagnostics); m != nil {
		body.Config = &m
	}
	var loc client.StorageLocationsStorageLocation
	if err := decode(apiCall(r.c.api.StorageLocationsCreate(ctx, body)), "storageLocationsCreate", &loc); err != nil {
		addError(&resp.Diagnostics, "Creating storage location", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, storageLocationState(loc, plan))...)
	// McHarbor before 2026-09-25 ignored enabled=false on create; correcting it
	// here keeps the provider consistent against those instances too.
	if loc.Enabled != plan.Enabled.ValueBool() {
		enabled := plan.Enabled.ValueBool()
		fix := client.StorageLocationsUpdateJSONRequestBody{Enabled: &enabled}
		if err := decode(apiCall(r.c.api.StorageLocationsUpdate(ctx, loc.Id, fix)), "storageLocationsUpdate", &loc); err != nil {
			addError(&resp.Diagnostics, "Setting storage location enabled", err)
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, storageLocationState(loc, plan))...)
	}
}

func (r *storageLocationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state storageLocationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var loc client.StorageLocationsStorageLocation
	err := decode(apiCall(r.c.api.StorageLocationsGet(ctx, state.ID.ValueString())), "storageLocationsGet", &loc)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading storage location", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, storageLocationState(loc, state))...)
}

func (r *storageLocationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg storageLocationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	empty := ""
	orEmpty := func(v types.String) *string {
		if p := strPtr(v); p != nil {
			return p
		}
		return &empty
	}
	body := client.StorageLocationsUpdateJSONRequestBody{
		Name: strPtr(plan.Name), Enabled: boolPtr(plan.Enabled), Port: intPtr(plan.Port),
		Host: orEmpty(plan.Host), BasePath: orEmpty(plan.BasePath), Region: orEmpty(plan.Region),
		Bucket: orEmpty(plan.Bucket), Endpoint: orEmpty(plan.Endpoint), TenantId: orEmpty(plan.TenantID),
		SiteUrl: orEmpty(plan.SiteURL), DriveId: orEmpty(plan.DriveID), ShareName: orEmpty(plan.ShareName),
		Domain: orEmpty(plan.Domain), Username: orEmpty(plan.Username), AuthMethod: orEmpty(plan.AuthMethod),
		TlsMode: orEmpty(plan.TLSMode), PassiveMode: boolPtr(plan.PassiveMode),
	}
	if !plan.CredentialsVersion.Equal(state.CredentialsVersion) && !plan.CredentialsVersion.IsNull() {
		body.Password, body.PrivateKey, body.Passphrase = strPtr(cfg.Password), strPtr(cfg.PrivateKey), strPtr(cfg.Passphrase)
		body.CaCertificate, body.ClientCertificate, body.ClientKey = strPtr(cfg.CACertificate), strPtr(cfg.ClientCertificate), strPtr(cfg.ClientKey)
		body.AccessKeyId, body.SecretAccessKey = strPtr(cfg.AccessKeyID), strPtr(cfg.SecretAccessKey)
		body.ClientId, body.ClientSecret = strPtr(cfg.ClientID), strPtr(cfg.ClientSecret)
		body.RefreshToken, body.Token = strPtr(cfg.RefreshToken), strPtr(cfg.Token)
		if m := stringMap(ctx, cfg.Config, &resp.Diagnostics); m != nil {
			body.Config = &m
		}
	}
	var loc client.StorageLocationsStorageLocation
	if err := decode(apiCall(r.c.api.StorageLocationsUpdate(ctx, state.ID.ValueString(), body)), "storageLocationsUpdate", &loc); err != nil {
		addError(&resp.Diagnostics, "Updating storage location", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, storageLocationState(loc, plan))...)
}

func (r *storageLocationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state storageLocationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.StorageLocationsDelete(ctx, state.ID.ValueString())), "storageLocationsDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting storage location", err)
	}
}

func (r *storageLocationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// storageLocationState maps the server's view onto state; write-only
// credentials stay null, as the framework requires and the design promises.
func storageLocationState(loc client.StorageLocationsStorageLocation, prior storageLocationModel) storageLocationModel {
	null := types.StringNull()
	return storageLocationModel{
		ID: types.StringValue(loc.Id), Name: types.StringValue(loc.Name), LocationType: types.StringValue(loc.LocationType),
		Enabled: types.BoolValue(loc.Enabled), Host: optString(loc.Host), Port: optInt(loc.Port),
		BasePath: optString(loc.BasePath), Region: optString(loc.Region), Bucket: optString(loc.Bucket),
		Endpoint: optString(loc.Endpoint), TenantID: optString(loc.TenantId), SiteURL: optString(loc.SiteUrl),
		DriveID: optString(loc.DriveId), ShareName: optString(loc.ShareName), Domain: optString(loc.Domain),
		Username: optString(loc.Username), AuthMethod: optString(loc.AuthMethod), TLSMode: optString(loc.TlsMode),
		PassiveMode: types.BoolValue(loc.PassiveMode), CredentialsVersion: prior.CredentialsVersion,
		Password: null, PrivateKey: null, Passphrase: null, CACertificate: null, ClientCertificate: null, ClientKey: null,
		AccessKeyID: null, SecretAccessKey: null, ClientID: null, ClientSecret: null, RefreshToken: null, Token: null,
		Config: types.MapNull(types.StringType),
	}
}
