// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

// addPreviewCommands registers `gitops preview`. It is a separate file, and a
// separate call, because the GitOps command group is edited by several people at
// once; register it from NewRoot beside addGitopsCommands.
func addPreviewCommands(root *cobra.Command, g *globals) {
	gx := findCommand(root, "gitops")
	if gx == nil {
		gx = &cobra.Command{Use: "gitops", Short: "GitOps pipelines and promotions"}
		root.AddCommand(gx)
	}

	pv := &cobra.Command{Use: "preview", Aliases: []string{"previews"}, Short: "Ephemeral pull-request preview environments"}

	var pipelineID string
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List pull-request previews", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "gitopsListPRPreviews", func(ctx context.Context, a *API) (*http.Response, error) {
				params := &client.GitopsListPRPreviewsParams{}
				if pipelineID != "" {
					params.PipelineId = &pipelineID
				}
				return a.c.GitopsListPRPreviews(ctx, params)
			}, []Column{
				{Header: "ID", Path: "id"},
				{Header: "PR", Path: "prNumber"},
				{Header: "BRANCH", Path: "sourceBranch"},
				{Header: "STATUS", Path: "status"},
				{Header: "URL", Path: "url"},
				{Header: "EXPIRES", Path: "expiresAt", Format: age},
				{Header: "TORN DOWN BY", Path: "tornDownBy"},
			})
		},
	}
	list.Flags().StringVar(&pipelineID, "pipeline", "", "only previews for this pipeline")
	pv.AddCommand(list)

	// Destroying a preview deletes an environment, so it carries Disruptive risk
	// and the route it reaches is gated on gitops.delete.
	pv.AddCommand(&cobra.Command{
		Use: "destroy <preview-id>", Short: "Destroy a preview environment now, before its TTL", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "gitopsDestroyPRPreview", Disruptive, "destroy preview", args[0],
				func(ctx context.Context, a *API) (*http.Response, error) {
					return a.rawRequest(ctx, http.MethodDelete, "/api/gitops/previews/"+args[0], nil, nil)
				})
		},
	})

	gx.AddCommand(pv)
}
