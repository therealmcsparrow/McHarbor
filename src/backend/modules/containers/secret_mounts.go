// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// stripSecretMounts removes the secret-files mount from an inspect response. It is part of the
// reveal: which container receives secret files, and from which volume, is shown only to a
// caller who revealed the container, and the editor never offers it for editing.
func stripSecretMounts(info *container.InspectResponse) bool {
	stripped := false
	if len(info.Mounts) > 0 {
		kept := info.Mounts[:0]
		for _, m := range info.Mounts {
			if m.Destination == secrets.SecretMountPath {
				stripped = true
				continue
			}
			kept = append(kept, m)
		}
		info.Mounts = kept
	}
	if info.HostConfig != nil {
		if len(info.HostConfig.Mounts) > 0 {
			kept := make([]mount.Mount, 0, len(info.HostConfig.Mounts))
			for _, m := range info.HostConfig.Mounts {
				if m.Target == secrets.SecretMountPath {
					stripped = true
					continue
				}
				kept = append(kept, m)
			}
			info.HostConfig.Mounts = kept
		}
		if len(info.HostConfig.Binds) > 0 {
			kept := make([]string, 0, len(info.HostConfig.Binds))
			for _, b := range info.HostConfig.Binds {
				if bindTarget(b) == secrets.SecretMountPath {
					stripped = true
					continue
				}
				kept = append(kept, b)
			}
			info.HostConfig.Binds = kept
		}
	}
	return stripped
}

// stripSummarySecretMounts is stripSecretMounts for a list row, which has no reveal.
func stripSummarySecretMounts(mounts []ContainerMountSummary) []ContainerMountSummary {
	kept := mounts[:0]
	for _, m := range mounts {
		if m.Destination == secrets.SecretMountPath {
			continue
		}
		kept = append(kept, m)
	}
	return kept
}

// bindTarget is the container path of a "source:target[:options]" bind string.
func bindTarget(bind string) string {
	parts := strings.Split(bind, ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
