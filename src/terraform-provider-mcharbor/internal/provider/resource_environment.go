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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type environmentResource struct {
	c *apiClient
}

type environmentModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ConnectionType types.String `tfsdk:"connection_type"`
	Host           types.String `tfsdk:"host"`
	Port           types.Int64  `tfsdk:"port"`
	SocketPath     types.String `tfsdk:"socket_path"`
	SSHHost        types.String `tfsdk:"ssh_host"`
	SSHPort        types.Int64  `tfsdk:"ssh_port"`
	SSHUser        types.String `tfsdk:"ssh_user"`
	SSHKey         types.String `tfsdk:"ssh_key"`
	TLSCA          types.String `tfsdk:"tls_ca"`
	TLSCert        types.String `tfsdk:"tls_cert"`
	TLSKey         types.String `tfsdk:"tls_key"`
	IsDefault      types.Bool   `tfsdk:"is_default"`
	Timezone       types.String `tfsdk:"timezone"`
	AgentToken     types.String `tfsdk:"agent_token"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

// environmentCreated is what an agent environment's create returns: the new
// row plus the install token, which exists nowhere else afterwards.
type environmentCreated struct {
	Environment *client.EnvironmentsEnvironment `json:"environment"`
	AgentToken  string                          `json:"agentToken"`
}

var (
	_ resource.ResourceWithConfigure   = &environmentResource{}
	_ resource.ResourceWithImportState = &environmentResource{}
)

func newEnvironmentResource() resource.Resource { return &environmentResource{} }

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A Docker environment McHarbor manages: the local socket, a TCP or SSH endpoint, or a remote agent. " +
			"Requires environments.view to read and environments.manage to change.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{Required: true},
			"connection_type": schema.StringAttribute{
				Required:      true,
				Description:   "socket, tcp, ssh or agent. Changing it replaces the environment; McHarbor's in-place migration is a UI flow.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:    []validator.String{oneOf("socket", "tcp", "ssh", "agent")},
			},
			"host":        schema.StringAttribute{Optional: true},
			"port":        schema.Int64Attribute{Optional: true},
			"socket_path": schema.StringAttribute{Optional: true},
			"ssh_host":    schema.StringAttribute{Optional: true},
			"ssh_port":    schema.Int64Attribute{Optional: true},
			"ssh_user":    schema.StringAttribute{Optional: true},
			"ssh_key":     schema.StringAttribute{Optional: true, Sensitive: true, Description: "Stored in Terraform state. Never read back from McHarbor."},
			"tls_ca":      schema.StringAttribute{Optional: true, Sensitive: true, Description: "Stored in Terraform state. Never read back from McHarbor."},
			"tls_cert":    schema.StringAttribute{Optional: true, Sensitive: true, Description: "Stored in Terraform state. Never read back from McHarbor."},
			"tls_key":     schema.StringAttribute{Optional: true, Sensitive: true, Description: "Stored in Terraform state. Never read back from McHarbor."},
			"is_default": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
			},
			"timezone": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: keep,
				Description: "IANA zone for this environment's schedules; McHarbor fills a default when unset.",
			},
			"agent_token": schema.StringAttribute{
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: keep,
				Description:   "For connection_type = agent: the install token, returned once at creation and stored in Terraform state. Null after import.",
			},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.EnvironmentsCreateJSONRequestBody{
		Name:           plan.Name.ValueString(),
		ConnectionType: plan.ConnectionType.ValueString(),
		IsDefault:      plan.IsDefault.ValueBool(),
		Host:           strPtr(plan.Host),
		Port:           intPtr(plan.Port),
		SocketPath:     strPtr(plan.SocketPath),
		SshHost:        strPtr(plan.SSHHost),
		SshPort:        intPtr(plan.SSHPort),
		SshUser:        strPtr(plan.SSHUser),
		SshKey:         strPtr(plan.SSHKey),
		TlsCa:          strPtr(plan.TLSCA),
		TlsCert:        strPtr(plan.TLSCert),
		TlsKey:         strPtr(plan.TLSKey),
	}
	var raw rawData
	if err := decode(apiCall(r.c.api.EnvironmentsCreate(ctx, body)), "environmentsCreate", &raw); err != nil {
		addError(&resp.Diagnostics, "Creating environment", err)
		return
	}
	var created environmentCreated
	var env client.EnvironmentsEnvironment
	if err := raw.into(&created); err != nil {
		addError(&resp.Diagnostics, "Decoding the created environment", err)
		return
	}
	if created.Environment != nil {
		env = *created.Environment
	} else if err := raw.into(&env); err != nil {
		addError(&resp.Diagnostics, "Decoding the created environment", err)
		return
	}

	state := environmentFromAPI(env, plan)
	state.AgentToken = types.StringNull()
	if created.AgentToken != "" {
		state.AgentToken = types.StringValue(created.AgentToken)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var env client.EnvironmentsEnvironment
	err := decode(apiCall(r.c.api.EnvironmentsGet(ctx, state.ID.ValueString())), "environmentsGet", &env)
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading environment", err)
		return
	}
	next := environmentFromAPI(env, state)
	next.AgentToken = state.AgentToken
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := client.EnvironmentsUpdateJSONRequestBody{
		Name:       strPtr(plan.Name),
		IsDefault:  boolPtr(plan.IsDefault),
		Host:       strPtr(plan.Host),
		Port:       intPtr(plan.Port),
		SocketPath: strPtr(plan.SocketPath),
		SshHost:    strPtr(plan.SSHHost),
		SshPort:    intPtr(plan.SSHPort),
		SshUser:    strPtr(plan.SSHUser),
		SshKey:     strPtr(plan.SSHKey),
		TlsCa:      strPtr(plan.TLSCA),
		TlsCert:    strPtr(plan.TLSCert),
		TlsKey:     strPtr(plan.TLSKey),
		Timezone:   strPtr(plan.Timezone),
	}
	var env client.EnvironmentsEnvironment
	if err := decode(apiCall(r.c.api.EnvironmentsUpdate(ctx, state.ID.ValueString(), body)), "environmentsUpdate", &env); err != nil {
		addError(&resp.Diagnostics, "Updating environment", err)
		return
	}
	next := environmentFromAPI(env, plan)
	next.AgentToken = state.AgentToken
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.EnvironmentsDelete(ctx, state.ID.ValueString())), "environmentsDelete", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Deleting environment", err)
	}
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// environmentFromAPI maps the server row, keeping the secret inputs from prior
// because McHarbor never returns them in usable form.
func environmentFromAPI(env client.EnvironmentsEnvironment, prior environmentModel) environmentModel {
	return environmentModel{
		ID:             types.StringValue(env.Id),
		Name:           types.StringValue(env.Name),
		ConnectionType: types.StringValue(env.ConnectionType),
		Host:           optString(env.Host),
		Port:           optInt(env.Port),
		SocketPath:     optString(env.SocketPath),
		SSHHost:        optString(env.SshHost),
		SSHPort:        optInt(env.SshPort),
		SSHUser:        optString(env.SshUser),
		SSHKey:         prior.SSHKey,
		TLSCA:          prior.TLSCA,
		TLSCert:        prior.TLSCert,
		TLSKey:         prior.TLSKey,
		IsDefault:      types.BoolValue(env.IsDefault),
		Timezone:       types.StringValue(env.Timezone),
		AgentToken:     prior.AgentToken,
		CreatedAt:      types.StringValue(env.CreatedAt),
	}
}
