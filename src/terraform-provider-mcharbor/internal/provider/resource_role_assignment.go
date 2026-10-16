// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
)

type roleAssignmentResource struct {
	c *apiClient
}

type roleAssignmentModel struct {
	ID            types.String `tfsdk:"id"`
	UserID        types.String `tfsdk:"user_id"`
	RoleID        types.String `tfsdk:"role_id"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	StackName     types.String `tfsdk:"stack_name"`
}

var (
	_ resource.ResourceWithConfigure   = &roleAssignmentResource{}
	_ resource.ResourceWithImportState = &roleAssignmentResource{}
)

func newRoleAssignmentResource() resource.Resource { return &roleAssignmentResource{} }

func (r *roleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_assignment"
}

func (r *roleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Grants a role to a user, globally, on one environment, or on one stack. " +
			"Every attribute forces replacement: an assignment is created or removed, never edited. Requires users.view to read and users.manage to change.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The assignment id. Import with <user_id>/<assignment_id>.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id":        schema.StringAttribute{Required: true, PlanModifiers: replace},
			"role_id":        schema.StringAttribute{Required: true, PlanModifiers: replace},
			"environment_id": schema.StringAttribute{Optional: true, PlanModifiers: replace, Description: "Scope the grant to one environment; omit for a global grant."},
			"stack_name":     schema.StringAttribute{Optional: true, PlanModifiers: replace, Description: "Scope the grant to one stack; requires environment_id."},
		},
	}
}

func (r *roleAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*apiClient); ok {
		r.c = c
	}
}

func (r *roleAssignmentResource) list(ctx context.Context, userID string) ([]client.UsersUserRole, error) {
	var roles []client.UsersUserRole
	err := decode(apiCall(r.c.api.UsersListRoles(ctx, userID)), "usersListRoles", &roles)
	return roles, err
}

func (r *roleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	userID := plan.UserID.ValueString()

	before, err := r.list(ctx, userID)
	if err != nil {
		addError(&resp.Diagnostics, "Listing the user's roles before assigning", err)
		return
	}
	body := client.UsersAssignRoleJSONRequestBody{
		RoleId:        plan.RoleID.ValueString(),
		EnvironmentId: strPtr(plan.EnvironmentID),
		StackName:     strPtr(plan.StackName),
	}
	if err := decode(apiCall(r.c.api.UsersAssignRole(ctx, userID, body)), "usersAssignRole", nil); err != nil {
		addError(&resp.Diagnostics, "Assigning role", err)
		return
	}

	after, err := r.list(ctx, userID)
	if err != nil {
		addError(&resp.Diagnostics, "Listing the user's roles after assigning", err)
		return
	}
	known := map[string]bool{}
	for _, a := range before {
		known[a.Id] = true
	}
	for _, a := range after {
		if !known[a.Id] && matches(a, plan) {
			plan.ID = types.StringValue(a.Id)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			return
		}
	}
	for _, a := range after {
		if matches(a, plan) {
			plan.ID = types.StringValue(a.Id)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			return
		}
	}
	resp.Diagnostics.AddError("Assigning role", "McHarbor accepted the assignment but it does not appear in the user's role list.")
}

func matches(a client.UsersUserRole, m roleAssignmentModel) bool {
	return a.RoleId == m.RoleID.ValueString() &&
		deref(a.EnvironmentId) == m.EnvironmentID.ValueString() &&
		deref(a.StackName) == m.StackName.ValueString()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (r *roleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	roles, err := r.list(ctx, state.UserID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addError(&resp.Diagnostics, "Reading role assignment", err)
		return
	}
	for _, a := range roles {
		if a.Id == state.ID.ValueString() {
			state.RoleID = types.StringValue(a.RoleId)
			state.EnvironmentID = optString(a.EnvironmentId)
			state.StackName = optString(a.StackName)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *roleAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *roleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := decode(apiCall(r.c.api.UsersUnassignRole(ctx, state.UserID.ValueString(), state.ID.ValueString())), "usersUnassignRole", nil)
	if err != nil && !isNotFound(err) {
		addError(&resp.Diagnostics, "Removing role assignment", err)
	}
}

func (r *roleAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userID, assignmentID, ok := strings.Cut(req.ID, "/")
	if !ok || userID == "" || assignmentID == "" {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <user_id>/<assignment_id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, roleAssignmentModel{
		ID:            types.StringValue(assignmentID),
		UserID:        types.StringValue(userID),
		RoleID:        types.StringNull(),
		EnvironmentID: types.StringNull(),
		StackName:     types.StringNull(),
	})...)
}
