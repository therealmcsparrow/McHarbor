// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

var userColumns = []Column{
	{Header: "ID", Path: "id"}, {Header: "USERNAME", Path: "username"}, {Header: "NAME", Path: "displayName"},
	{Header: "ROLE", Path: "role"}, {Header: "ACTIVE", Path: "isActive"}, {Header: "LAST LOGIN", Path: "lastLogin", Format: age},
}

func addUserCommands(root *cobra.Command, g *globals) {
	u := &cobra.Command{Use: "user", Aliases: []string{"users"}, Short: "User accounts"}
	u.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List users", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "usersList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UsersList(ctx, &client.UsersListParams{})
			}, userColumns)
		},
	})
	u.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a user", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "usersGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.UsersGet(ctx, args[0]) }, userColumns)
		},
	})
	var displayName, email, role string
	var passwordStdin bool
	create := &cobra.Command{
		Use: "create <username>", Short: "Create a local user; the password is read without echo, never from an argument", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			password, err := readSecret(g, passwordStdin, "Password: ")
			if err != nil {
				return err
			}
			return g.runAction(cmd, "usersCreate", Reversible, "create", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(client.UsersCreateUserInput{
					Username: args[0], Password: password, DisplayName: strPtr(displayName), Email: strPtr(email), RoleId: strPtr(role),
				})
				if err != nil {
					return nil, err
				}
				return a.c.UsersCreateWithBody(ctx, jsonContent, body)
			})
		},
	}
	create.Flags().StringVar(&displayName, "display-name", "", "display name")
	create.Flags().StringVar(&email, "email", "", "email address")
	create.Flags().StringVar(&role, "role", "", "role id to assign globally")
	create.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	u.AddCommand(create)

	var reason string
	disable := &cobra.Command{
		Use: "disable <id>", Short: "Disable a user; their sessions end and they cannot sign in", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "usersDisable", Disruptive, "disable", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UsersDisable(ctx, args[0], client.UsersDisableJSONRequestBody{Reason: reason})
			})
		},
	}
	disable.Flags().StringVar(&reason, "reason", "", "reason recorded in the audit log")
	u.AddCommand(disable)
	u.AddCommand(simpleAction(g, "enable <id>", "Re-enable a disabled user", "usersEnable", Reversible, "enable",
		func(ctx context.Context, a *API, id string) (*http.Response, error) { return a.c.UsersEnable(ctx, id) }))
	u.AddCommand(simpleAction(g, "unlock <id>", "Clear a lockout after failed sign-ins", "usersUnlock", Reversible, "unlock",
		func(ctx context.Context, a *API, id string) (*http.Response, error) { return a.c.UsersUnlock(ctx, id) }))
	u.AddCommand(simpleAction(g, "delete <id>", "Delete a user", "usersDelete", Irreversible, "delete",
		func(ctx context.Context, a *API, id string) (*http.Response, error) { return a.c.UsersDelete(ctx, id) }))

	u.AddCommand(&cobra.Command{
		Use: "roles <id>", Short: "List a user's role assignments", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runList(cmd, "usersListRoles", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.UsersListRoles(ctx, args[0]) }, []Column{
				{Header: "ASSIGNMENT", Path: "id"}, {Header: "ROLE", Path: "roleName"}, {Header: "ENV", Path: "environmentName"}, {Header: "STACK", Path: "stackName"},
			})
		},
	})
	var env, stack string
	assign := &cobra.Command{
		Use: "assign-role <user-id> <role-id>", Short: "Give a user a role, globally or scoped to an environment or stack", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "usersAssignRole", Reversible, "assign", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(client.UsersAssignRoleInput{RoleId: args[1], EnvironmentId: strPtr(env), StackName: strPtr(stack)})
				if err != nil {
					return nil, err
				}
				return a.c.UsersAssignRoleWithBody(ctx, args[0], jsonContent, body)
			})
		},
	}
	assign.Flags().StringVar(&env, "env-scope", "", "limit the role to this environment id")
	assign.Flags().StringVar(&stack, "stack", "", "limit the role to this stack (needs --env-scope)")
	u.AddCommand(assign)
	u.AddCommand(&cobra.Command{
		Use: "unassign-role <user-id> <assignment-id>", Short: "Remove one role assignment (see user roles)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "usersUnassignRole", Disruptive, "unassign", args[1], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.UsersUnassignRole(ctx, args[0], args[1])
			})
		},
	})
	root.AddCommand(u)

	grp := &cobra.Command{Use: "group", Aliases: []string{"groups"}, Short: "User groups"}
	grp.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List groups", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "groupsList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.GroupsList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "MEMBERS", Path: "memberCount"}, {Header: "SYSTEM", Path: "isSystem"},
			})
		},
	})
	grp.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a group", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "groupsGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.GroupsGet(ctx, args[0]) }, nil)
		},
	})
	var description string
	gc := &cobra.Command{
		Use: "create <name>", Short: "Create a group", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "groupsCreate", Reversible, "create", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(client.GroupsCreateGroupInput{Name: args[0], Description: description})
				if err != nil {
					return nil, err
				}
				return a.c.GroupsCreateWithBody(ctx, jsonContent, body)
			})
		},
	}
	gc.Flags().StringVar(&description, "description", "", "description")
	grp.AddCommand(gc)
	grp.AddCommand(&cobra.Command{
		Use: "add-member <group-id> <user-id>", Short: "Add a user to a group", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "groupsAddMember", Reversible, "add", args[1], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(client.GroupsAddMemberInput{UserId: args[1]})
				if err != nil {
					return nil, err
				}
				return a.c.GroupsAddMemberWithBody(ctx, args[0], jsonContent, body)
			})
		},
	})
	grp.AddCommand(&cobra.Command{
		Use: "remove-member <group-id> <user-id>", Short: "Remove a user from a group", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "groupsRemoveMember", Disruptive, "remove", args[1], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.GroupsRemoveMember(ctx, args[0], args[1])
			})
		},
	})
	grp.AddCommand(simpleAction(g, "delete <id>", "Delete a group", "groupsDelete", Irreversible, "delete",
		func(ctx context.Context, a *API, id string) (*http.Response, error) { return a.c.GroupsDelete(ctx, id) }))
	root.AddCommand(grp)

	r := &cobra.Command{Use: "role", Aliases: []string{"roles"}, Short: "Roles and the permission catalog"}
	r.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List roles", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "rolesList", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.RolesList(ctx) }, []Column{
				{Header: "ID", Path: "id"}, {Header: "NAME", Path: "name"}, {Header: "DESCRIPTION", Path: "description"}, {Header: "SYSTEM", Path: "isSystem"},
			})
		},
	})
	r.AddCommand(&cobra.Command{
		Use: "get <id>", Short: "Show a role and its permissions", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "rolesGet", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.RolesGet(ctx, args[0]) }, nil)
		},
	})
	var roleDescription string
	var permissionKeys []string
	rc := &cobra.Command{
		Use: "create <name>", Short: "Create a role from permission keys (see role permissions)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(permissionKeys) == 0 {
				return usageError("--permission is required at least once")
			}
			return g.runAction(cmd, "rolesCreate", Reversible, "create", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(client.RolesCreateRoleInput{Name: args[0], Description: roleDescription, Permissions: permissionKeys})
				if err != nil {
					return nil, err
				}
				return a.c.RolesCreateWithBody(ctx, jsonContent, body)
			})
		},
	}
	rc.Flags().StringVar(&roleDescription, "description", "", "description")
	rc.Flags().StringArrayVar(&permissionKeys, "permission", nil, "permission key; repeat for more")
	r.AddCommand(rc)
	r.AddCommand(simpleAction(g, "delete <id>", "Delete a role; its assignments go with it", "rolesDelete", Irreversible, "delete",
		func(ctx context.Context, a *API, id string) (*http.Response, error) { return a.c.RolesDelete(ctx, id) }))
	r.AddCommand(&cobra.Command{
		Use: "permissions", Short: "List every permission key a role can hold", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "authPermissionCatalog", func(ctx context.Context, a *API) (*http.Response, error) { return a.c.AuthPermissionCatalog(ctx) }, []Column{
				{Header: "KEY", Path: "key"}, {Header: "CATEGORY", Path: "category"}, {Header: "SCOPE", Path: "scope"}, {Header: "DESCRIPTION", Path: "description"},
			})
		},
	})
	root.AddCommand(r)
}

// simpleAction is a one-argument mutation with no body.
func simpleAction(g *globals, use, short, op string, risk Risk, action string, call func(ctx context.Context, a *API, id string) (*http.Response, error)) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, op, risk, action, args[0], func(ctx context.Context, a *API) (*http.Response, error) { return call(ctx, a, args[0]) })
		},
	}
}
