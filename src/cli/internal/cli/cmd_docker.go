// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
)

const jsonContent = "application/json"

func addImageCommands(root *cobra.Command, g *globals) {
	img := &cobra.Command{Use: "image", Aliases: []string{"images"}, Short: "Images in the selected environment"}
	img.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List images", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "imagesList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ImagesList(ctx, &client.ImagesListParams{Env: a.env()})
			}, []Column{
				{Header: "ID", Path: "Id", Format: shortID}, {Header: "TAGS", Path: "RepoTags"},
				{Header: "SIZE", Path: "Size", Format: bytesHuman}, {Header: "CONTAINERS", Path: "Containers"},
				{Header: "CREATED", Path: "Created", Format: age},
			})
		},
	})
	var revealImage bool
	imgGet := &cobra.Command{
		Use: "get <id|ref>", Aliases: []string{"inspect"}, Short: "Show one image", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "imagesInspect", func(ctx context.Context, a *API) (*http.Response, error) {
				params := &client.ImagesInspectParams{Env: a.env()}
				if revealImage {
					params.Reveal = strPtr("true")
				}
				return a.c.ImagesInspect(ctx, args[0], params)
			}, []Column{
				{Header: "ID", Path: "Id", Format: shortID}, {Header: "Tags", Path: "RepoTags"},
				{Header: "Size", Path: "Size", Format: bytesHuman}, {Header: "Architecture", Path: "Architecture"},
				{Header: "OS", Path: "Os"}, {Header: "Created", Path: "Created", Format: age},
			})
		},
	}
	imgGet.Flags().BoolVar(&revealImage, "reveal", false, "show credential-looking Config.Env values in the clear (needs images.manage; audit-logged)")
	img.AddCommand(imgGet)
	img.AddCommand(&cobra.Command{
		Use: "pull <ref>", Short: "Pull an image, such as nginx:1.27", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			image, tag := args[0], ""
			if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") && !strings.Contains(image, "@") {
				image, tag = image[:i], image[i+1:]
			}
			return g.runAction(cmd, "imagesPull", Reversible, "pull", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(map[string]string{"image": image, "tag": tag})
				if err != nil {
					return nil, err
				}
				return a.c.ImagesPullWithBody(ctx, &client.ImagesPullParams{Env: a.env()}, jsonContent, body)
			})
		},
	})
	var force bool
	rm := &cobra.Command{
		Use: "remove <id|ref>", Aliases: []string{"rm"}, Short: "Remove an image (irreversible)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "imagesRemove", Irreversible, "remove", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.ImagesRemove(ctx, args[0], &client.ImagesRemoveParams{Env: a.env(), Force: boolParam(force)})
			})
		},
	}
	rm.Flags().BoolVarP(&force, "force", "f", false, "remove it even when containers use it")
	img.AddCommand(rm)
	root.AddCommand(img)
}

func addVolumeCommands(root *cobra.Command, g *globals) {
	vol := &cobra.Command{Use: "volume", Aliases: []string{"volumes"}, Short: "Volumes in the selected environment"}
	vol.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List volumes", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "volumesList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.VolumesList(ctx, &client.VolumesListParams{Env: a.env()})
			}, []Column{
				{Header: "NAME", Path: "Name"}, {Header: "DRIVER", Path: "Driver"}, {Header: "IN USE BY", Path: "UsedBy"},
				{Header: "CREATED", Path: "CreatedAt", Format: age},
			})
		},
	})
	vol.AddCommand(&cobra.Command{
		Use: "get <name>", Aliases: []string{"inspect"}, Short: "Show one volume", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "volumesInspect", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.VolumesInspect(ctx, args[0], &client.VolumesInspectParams{Env: a.env()})
			}, nil)
		},
	})
	var driver string
	var labels []string
	create := &cobra.Command{
		Use: "create <name>", Short: "Create a volume", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lbl, err := parseLabels(labels)
			if err != nil {
				return err
			}
			return g.runAction(cmd, "volumesCreate", Reversible, "create", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				body, err := jsonBody(map[string]any{"name": args[0], "driver": driver, "labels": lbl})
				if err != nil {
					return nil, err
				}
				return a.c.VolumesCreateWithBody(ctx, &client.VolumesCreateParams{Env: a.env()}, jsonContent, body)
			})
		},
	}
	create.Flags().StringVar(&driver, "driver", "", "volume driver; local when empty")
	create.Flags().StringArrayVar(&labels, "label", nil, "label as key=value; repeatable")
	vol.AddCommand(create)
	var force bool
	rm := &cobra.Command{
		Use: "remove <name>", Aliases: []string{"rm"}, Short: "Remove a volume and its data (irreversible)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "volumesRemove", Irreversible, "remove", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.VolumesRemove(ctx, args[0], &client.VolumesRemoveParams{Env: a.env(), Force: boolParam(force)})
			})
		},
	}
	rm.Flags().BoolVarP(&force, "force", "f", false, "remove it even when it is in use")
	vol.AddCommand(rm)
	root.AddCommand(vol)
}

func addNetworkCommands(root *cobra.Command, g *globals) {
	nw := &cobra.Command{Use: "network", Aliases: []string{"networks"}, Short: "Networks in the selected environment"}
	nw.AddCommand(&cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List networks", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return g.runList(cmd, "networksList", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.NetworksList(ctx, &client.NetworksListParams{Env: a.env()})
			}, []Column{
				{Header: "ID", Path: "Id", Format: shortID}, {Header: "NAME", Path: "Name"}, {Header: "DRIVER", Path: "Driver"},
				{Header: "SCOPE", Path: "Scope"}, {Header: "INTERNAL", Path: "Internal"}, {Header: "CONTAINERS", Path: "ContainerNames"},
			})
		},
	})
	nw.AddCommand(&cobra.Command{
		Use: "get <id|name>", Aliases: []string{"inspect"}, Short: "Show one network", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "networksInspect", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.NetworksInspect(ctx, args[0], &client.NetworksInspectParams{Env: a.env()})
			}, nil)
		},
	})
	var driver string
	var internal, attachable, ipv6 bool
	var labels, opts, subnets []string
	var gateway, ipRange, hlName, hlIPv4, hlIPv6 string
	create := &cobra.Command{
		Use: "create <name>", Short: "Create a network", Args: cobra.ExactArgs(1),
		Long: "Creates a network. --gateway and --ip-range apply to the first --subnet.\n" +
			"--host-link-name also creates a host interface on a macvlan network's parent so the host can reach the containers on it;\n" +
			"give it an address with --host-link-ipv4 and/or --host-link-ipv6 (a CIDR inside a --subnet, or \"auto\" for SLAAC with --ipv6).",
		RunE: func(cmd *cobra.Command, args []string) error {
			lbl, err := parseLabels(labels)
			if err != nil {
				return err
			}
			opt, err := parseLabels(opts)
			if err != nil {
				return err
			}
			body, err := networkCreateBody(args[0], networkCreateFlags{
				driver: driver, internal: internal, attachable: attachable, ipv6: ipv6, labels: lbl, options: opt,
				subnets: subnets, gateway: gateway, ipRange: ipRange, hlName: hlName, hlIPv4: hlIPv4, hlIPv6: hlIPv6,
			})
			if err != nil {
				return err
			}
			return g.runAction(cmd, "networksCreate", Reversible, "create", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				reader, err := jsonBody(body)
				if err != nil {
					return nil, err
				}
				return a.c.NetworksCreateWithBody(ctx, &client.NetworksCreateParams{Env: a.env()}, jsonContent, reader)
			})
		},
	}
	create.Flags().StringVar(&driver, "driver", "", "network driver; bridge when empty")
	create.Flags().BoolVar(&internal, "internal", false, "no external connectivity")
	create.Flags().BoolVar(&attachable, "attachable", false, "allow standalone containers to attach")
	create.Flags().BoolVar(&ipv6, "ipv6", false, "enable IPv6 on the network")
	create.Flags().StringArrayVar(&labels, "label", nil, "label as key=value; repeatable")
	create.Flags().StringArrayVar(&opts, "opt", nil, "driver option as key=value, such as parent=eth0; repeatable")
	create.Flags().StringArrayVar(&subnets, "subnet", nil, "IPAM subnet in CIDR notation; repeatable")
	create.Flags().StringVar(&gateway, "gateway", "", "gateway of the first subnet")
	create.Flags().StringVar(&ipRange, "ip-range", "", "address range of the first subnet, in CIDR notation")
	create.Flags().StringVar(&hlName, "host-link-name", "", "create a host interface with this name on the macvlan parent")
	create.Flags().StringVar(&hlIPv4, "host-link-ipv4", "", "IPv4 address of the host interface, in CIDR notation")
	create.Flags().StringVar(&hlIPv6, "host-link-ipv6", "", "IPv6 address of the host interface in CIDR notation, or \"auto\" for SLAAC")
	nw.AddCommand(create)
	addHostLinkCommands(nw, g)
	nw.AddCommand(&cobra.Command{
		Use: "remove <id|name>", Aliases: []string{"rm"}, Short: "Remove a network (irreversible)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "networksRemove", Irreversible, "remove", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.NetworksRemove(ctx, args[0], &client.NetworksRemoveParams{Env: a.env()})
			})
		},
	})
	root.AddCommand(nw)
}

type networkCreateFlags struct {
	driver                     string
	internal, attachable, ipv6 bool
	labels, options            map[string]string
	subnets                    []string
	gateway, ipRange           string
	hlName, hlIPv4, hlIPv6     string
}

// networkCreateBody builds the POST /networks body, leaving out whatever the flags did not set so the
// server's own defaults apply.
func networkCreateBody(name string, f networkCreateFlags) (map[string]any, error) {
	body := map[string]any{"name": name, "driver": f.driver, "internal": f.internal, "attachable": f.attachable, "labels": f.labels}
	if f.ipv6 {
		body["enableIPv6"] = true
	}
	if len(f.options) > 0 {
		body["options"] = f.options
	}
	if len(f.subnets) == 0 && (f.gateway != "" || f.ipRange != "") {
		return nil, usageError("--gateway and --ip-range need a --subnet")
	}
	if len(f.subnets) > 0 {
		cfg := make([]map[string]string, 0, len(f.subnets))
		for i, s := range f.subnets {
			block := map[string]string{"Subnet": s}
			if i == 0 && f.gateway != "" {
				block["Gateway"] = f.gateway
			}
			if i == 0 && f.ipRange != "" {
				block["IPRange"] = f.ipRange
			}
			cfg = append(cfg, block)
		}
		body["ipam"] = map[string]any{"Config": cfg}
	}
	if f.hlName == "" {
		if f.hlIPv4 != "" || f.hlIPv6 != "" {
			return nil, usageError("--host-link-ipv4 and --host-link-ipv6 need --host-link-name")
		}
		return body, nil
	}
	hl := map[string]any{"name": f.hlName}
	if f.hlIPv4 != "" {
		hl["ipv4"] = f.hlIPv4
	}
	switch f.hlIPv6 {
	case "":
	case "auto":
		hl["ipv6Auto"] = true
	default:
		hl["ipv6"] = f.hlIPv6
	}
	body["hostLink"] = hl
	return body, nil
}

// addHostLinkCommands adds `network host-link get|apply`.
func addHostLinkCommands(nw *cobra.Command, g *globals) {
	hl := &cobra.Command{Use: "host-link", Short: "The host interface of a macvlan network"}
	hl.AddCommand(&cobra.Command{
		Use: "get <id|name>", Short: "Show whether the host interface exists on the host", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runGet(cmd, "networksHostLink", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.NetworksHostLink(ctx, args[0], &client.NetworksHostLinkParams{Env: a.env()})
			}, nil)
		},
	})
	hl.AddCommand(&cobra.Command{
		Use: "apply <id|name>", Short: "Re-create the host interface from the network's recorded settings", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return g.runAction(cmd, "networksHostLinkApply", Reversible, "apply host link of", args[0], func(ctx context.Context, a *API) (*http.Response, error) {
				return a.c.NetworksHostLinkApply(ctx, args[0], &client.NetworksHostLinkApplyParams{Env: a.env()})
			})
		},
	})
	nw.AddCommand(hl)
}

func parseLabels(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, usageError("label %q is not key=value", p)
		}
		out[k] = v
	}
	return out, nil
}
