// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// TestSecretMountIsPartOfTheReveal guards an ordinary inspect: the secret-files mount, and the
// volume behind it, appear only to a caller who revealed the container.
func TestSecretMountIsPartOfTheReveal(t *testing.T) {
	info := container.InspectResponse{
		Mounts: []container.MountPoint{
			{Type: "volume", Name: "mcharbor-secrets-abc", Destination: "/run/secrets"},
			{Type: "bind", Source: "/srv/app", Destination: "/data"},
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "mcharbor-secrets-abc", Target: "/run/secrets"}},
			Binds:  []string{"/srv/app:/data", "/srv/keys:/run/secrets:ro"},
		},
	}
	if !stripSecretMounts(&info) {
		t.Fatal("nothing reported stripped")
	}
	if len(info.Mounts) != 1 || info.Mounts[0].Destination != "/data" {
		t.Errorf("mounts = %+v", info.Mounts)
	}
	if len(info.HostConfig.Mounts) != 0 {
		t.Errorf("host config mounts = %+v", info.HostConfig.Mounts)
	}
	if len(info.HostConfig.Binds) != 1 || info.HostConfig.Binds[0] != "/srv/app:/data" {
		t.Errorf("binds = %v", info.HostConfig.Binds)
	}
	if got := stripSummarySecretMounts([]ContainerMountSummary{{Destination: "/run/secrets"}, {Destination: "/data"}}); len(got) != 1 {
		t.Errorf("list row kept the secret mount: %+v", got)
	}
}
