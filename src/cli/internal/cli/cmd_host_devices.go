// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"context"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// hostDevicesCommand is `mcharbor host devices`: the device nodes on an
// environment's host that a container can map, the same list the device
// picker shows. It needs host.view.
func hostDevicesCommand(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:     "devices",
		Aliases: []string{"device", "dev"},
		Short:   "Device nodes on the host that a container can map",
		Long: "Lists the character and block devices under /dev on the environment's host:\n" +
			"serial adapters, GPUs, audio, video, input, /dev/net/tun, /dev/fuse, /dev/kvm,\n" +
			"USB bus nodes and disks. STABLE is the by-id or by-path name that survives a\n" +
			"replug; use it with --device. Block devices are marked DANGEROUS. --env is required.",
		Args:    cobra.NoArgs,
		Example: "  mcharbor host devices --env prod-01\n  mcharbor host devices --env prod-01 -o json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := hostEnv(g)
			if err != nil {
				return err
			}
			p, err := g.printer()
			if err != nil {
				return err
			}
			v, err := g.fetch(cmd, "systemHostDevices", func(ctx context.Context, a *API) (*http.Response, error) {
				return a.rawGet(ctx, "/api/system/host-devices", url.Values{"env": {env}}, map[string]string{"Accept": jsonContent})
			})
			if err != nil {
				return err
			}
			if p.format != "table" {
				return p.Object(v, nil)
			}
			if err := p.List(listItemsPreferring(v, "devices"), []Column{
				{Header: "PATH", Path: "path"},
				{Header: "KIND", Path: "category"},
				{Header: "TYPE", Path: "type"},
				{Header: "MAJOR", Path: "major"},
				{Header: "MINOR", Path: "minor"},
				{Header: "LABEL", Path: "label"},
				{Header: "STABLE", Path: "stablePath"},
				{Header: "DANGEROUS", Path: "dangerous"},
			}); err != nil {
				return err
			}
			if m, ok := v.(map[string]any); ok && m["truncated"] == true {
				emitln(g.stderr, "# the host has more devices than the server returns; the list is truncated")
			}
			return nil
		},
	}
}
