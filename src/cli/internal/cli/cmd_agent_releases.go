// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// agentVersionPattern bounds the version argument before it becomes a path
// segment, the same shape the server accepts.
var agentVersionPattern = regexp.MustCompile(`^v?[0-9]{1,4}\.[0-9]{1,3}\.[0-9]{1,4}$`)

// agentReleasesCommand is `mcharbor agent releases`: McHarbor's own store of
// agent releases, which every agent install and update is served from.
// Reading needs updates.view; everything else needs updates.manage.
func agentReleasesCommand(g *globals) *cobra.Command {
	releases := &cobra.Command{
		Use:   "releases",
		Short: "Agent releases stored in McHarbor, which every install and update is served from",
		Long: "McHarbor pulls agent releases from the mcharbor.eu catalog, an operator's mirror\n" +
			"or GitHub, verifies each against its signed SHA256SUMS, and serves agents only\n" +
			"from what it stored.\n\n" +
			"The target is the release installs, deploys and update sweeps hand out. With no\n" +
			"target, agents get the locally built agent, else the one the image shipped with.",
	}

	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "Stored releases, the target and the last check", Args: cobra.NoArgs,
		Example: "  mcharbor agent releases list\n  mcharbor agent releases list -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "agentReleasesList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/updates/agents/releases", url.Values{}, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			return p.List(listItemsPreferring(v, "releases"), []Column{
				{Header: "VERSION", Path: "version"},
				{Header: "BUILD", Path: "build"},
				{Header: "STATUS", Path: "status"},
				{Header: "TARGET", Path: "isTarget"},
				{Header: "IN USE", Path: "inUse"},
				{Header: "SIGNATURE", Path: "signatureStatus"},
				{Header: "SIZE", Path: "sizeBytes", Format: bytesHuman},
				{Header: "IMAGE", Path: "imageRef"},
				{Header: "SOURCE", Path: "sourceUrl"},
				{Header: "PULLED", Path: "pulledAt", Format: age},
				{Header: "ERROR", Path: "lastError"},
			})
		},
	}

	check := &cobra.Command{
		Use: "check", Short: "Ask the configured sources which agent version is newest", Args: cobra.NoArgs,
		Long:    "Downloads nothing. The answer is cached and shown in Settings > Updates > Agents.",
		Example: "  mcharbor agent releases check",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runGet(cmd, "agentReleasesCheck", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPost, "/api/updates/agents/releases/check", nil, nil)
			}, []Column{
				{Header: "LATEST", Path: "latestVersion"},
				{Header: "BUILD", Path: "latestBuild"},
				{Header: "SOURCE", Path: "latestSource"},
				{Header: "CHECKED", Path: "lastCheckedAt"},
				{Header: "ERROR", Path: "lastError"},
			})
		},
	}

	pull := &cobra.Command{
		Use: "pull <version>", Short: "Download and verify a release into McHarbor's store", Args: cobra.ExactArgs(1),
		Long: "Starts the download and returns; the release reads \"pulling\" until it is\n" +
			"verified and stored or refused. A release whose signature or checksums do not\n" +
			"verify is never stored, and `list` shows why. A verified download becomes the\n" +
			"target; builds stored before it stay, so `target --build` can step back.\n" +
			"--build picks one of the builds `check` lists; one older than the newest is pinned.",
		Example: "  mcharbor agent releases pull 2026.09.02\n" +
			"  mcharbor agent releases pull 2026.10.01 --build 202610050818",
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := agentVersionArg(args[0])
			if err != nil {
				return err
			}
			build, err := cmd.Flags().GetString("build")
			if err != nil {
				return err
			}
			var query url.Values
			if build = strings.TrimSpace(build); build != "" {
				query = url.Values{"build": {build}}
			}
			return g.runAction(cmd, "agentReleasesPull", Reversible, "pull", version, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPost, "/api/updates/agents/releases/"+url.PathEscape(version)+"/pull", query, nil)
			})
		},
	}
	pull.Flags().String("build", "", "pull this build (yyyyMMddHHmm) instead of the newest; an older build is pinned until another is pulled")

	del := &cobra.Command{
		Use: "delete <version>", Aliases: []string{"rm"}, Short: "Delete a stored release (irreversible)", Args: cobra.ExactArgs(1),
		Long: "The target and any build a connected agent runs are refused. A version with\n" +
			"more than one stored build needs --build to say which one goes.",
		Example: "  mcharbor agent releases delete 2026.09.01 --confirm 2026.09.01\n" +
			"  mcharbor agent releases delete 2026.10.01 --build 202610050818 --confirm 2026.10.01",
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := agentVersionArg(args[0])
			if err != nil {
				return err
			}
			build, err := cmd.Flags().GetString("build")
			if err != nil {
				return err
			}
			var query url.Values
			if build = strings.TrimSpace(build); build != "" {
				query = url.Values{"build": {build}}
			}
			return g.runAction(cmd, "agentReleasesDelete", Irreversible, "delete", version, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodDelete, "/api/updates/agents/releases/"+url.PathEscape(version), query, nil)
			})
		},
	}
	del.Flags().String("build", "", "the stored build (yyyyMMddHHmm) to delete, when the version holds several")

	var clearTarget bool
	target := &cobra.Command{
		Use: "target [<version>]", Short: "Make a stored release the one agents are served", Args: cobra.MaximumNArgs(1),
		Long: "Installs, deploys and update sweeps hand out the target from then on. It does\n" +
			"not restart any agent by itself; run the update sweep for that. --clear goes back\n" +
			"to the locally built or image-bundled agent. Without --build the newest stored\n" +
			"build of the version is chosen.",
		Example: "  mcharbor agent releases target 2026.09.02\n" +
			"  mcharbor agent releases target 2026.10.01 --build 202610050818\n" +
			"  mcharbor agent releases target --clear",
		RunE: func(cmd *cobra.Command, args []string) error {
			version := ""
			switch {
			case clearTarget && len(args) == 0:
			case !clearTarget && len(args) == 1:
				v, err := agentVersionArg(args[0])
				if err != nil {
					return err
				}
				version = v
			default:
				return usageError("give a version or --clear, not both and not neither")
			}
			build, err := cmd.Flags().GetString("build")
			if err != nil {
				return err
			}
			if build = strings.TrimSpace(build); build != "" && version == "" {
				return usageError("--build names a build of a version; it cannot go with --clear")
			}
			payload := map[string]string{"version": version}
			if build != "" {
				payload["build"] = build
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			label := version
			if build != "" {
				label += " build " + build
			}
			if label == "" {
				label = "built-in agent"
			}
			return g.runAction(cmd, "agentReleasesTarget", Disruptive, "target", label, func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawRequest(ctx, http.MethodPut, "/api/updates/agents/releases/target", nil, body)
			})
		},
	}
	target.Flags().BoolVar(&clearTarget, "clear", false, "clear the target and serve the built-in agent")
	target.Flags().String("build", "", "the stored build (yyyyMMddHHmm) to target instead of the newest")

	releases.AddCommand(list, check, pull, del, target)
	return releases
}

func agentVersionArg(raw string) (string, error) {
	if !agentVersionPattern.MatchString(raw) {
		return "", usageError("%q is not an agent version such as 2026.09.02", raw)
	}
	return strings.TrimPrefix(raw, "v"), nil
}
