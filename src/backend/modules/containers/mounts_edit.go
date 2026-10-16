// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// ErrInvalidMount is returned when an edited mount list cannot be applied as written.
var ErrInvalidMount = errors.New("invalid mount")

// MountEdit is one mount as the configuration editor sends it.
type MountEdit struct {
	// Type is bind, volume or tmpfs.
	Type string `json:"type"`
	// Source is a host path for bind and a volume name for volume; tmpfs has none.
	Source      string `json:"source,omitempty"`
	Target      string `json:"target"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	Propagation string `json:"propagation,omitempty"`
}

// RecreateEditRequest is the recreate body the configuration editor sends: the shared
// RecreateRequest plus the edited mount list. It lives here rather than in core/docker because
// plugins link core/docker, and only this module's recreate route accepts mounts.
type RecreateEditRequest struct {
	RecreateRequest
	// Mounts replaces every mount of the container when present; absent leaves them as they are.
	Mounts *[]MountEdit `json:"mounts,omitempty"`
}

var (
	volumeNameRe       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	allowedPropagation = map[string]bool{
		"": true, "private": true, "rprivate": true, "shared": true,
		"rshared": true, "slave": true, "rslave": true,
	}
)

// validateMountEdits checks what Docker would otherwise reject halfway through a recreate, after
// the old container was already stopped and renamed.
func validateMountEdits(mounts []MountEdit) error {
	seen := map[string]bool{}
	for i, m := range mounts {
		target := strings.TrimSpace(m.Target)
		if !path.IsAbs(target) || path.Clean(target) != target || target == "/" {
			return fmt.Errorf("%w: mount %d target %q must be an absolute container path", ErrInvalidMount, i, m.Target)
		}
		if seen[target] {
			return fmt.Errorf("%w: two mounts target %q", ErrInvalidMount, target)
		}
		if target == secrets.SecretMountPath {
			return fmt.Errorf("%w: %s is managed by secret file mode", ErrInvalidMount, target)
		}
		seen[target] = true
		source := strings.TrimSpace(m.Source)
		switch m.Type {
		case "bind":
			if !path.IsAbs(source) || strings.Contains(source, ":") {
				return fmt.Errorf("%w: bind source %q must be an absolute host path", ErrInvalidMount, m.Source)
			}
			if !allowedPropagation[m.Propagation] {
				return fmt.Errorf("%w: unknown propagation %q", ErrInvalidMount, m.Propagation)
			}
		case "volume":
			if !volumeNameRe.MatchString(source) {
				return fmt.Errorf("%w: volume name %q is not valid", ErrInvalidMount, m.Source)
			}
		case "tmpfs":
		default:
			return fmt.Errorf("%w: unknown mount type %q", ErrInvalidMount, m.Type)
		}
	}
	return nil
}

// applyMountEdits replaces hc's mounts with the edited list. Binds and volumes are written as
// Binds strings, not structured mounts, because a bind in Binds still creates a missing host
// directory the way the original container's did, where a structured bind mount refuses to start.
func applyMountEdits(hc *container.HostConfig, mounts []MountEdit) {
	previousTmpfs := hc.Tmpfs
	hc.Mounts = nil
	hc.Binds = nil
	hc.Tmpfs = nil
	for _, m := range mounts {
		target := strings.TrimSpace(m.Target)
		// The secret-files mount is not editable: recreate strips and re-adds it from the
		// container's secret file mode, so an edited copy would only collide with it.
		if target == secrets.SecretMountPath {
			continue
		}
		switch m.Type {
		case "tmpfs":
			if hc.Tmpfs == nil {
				hc.Tmpfs = map[string]string{}
			}
			// Size and mode options set at creation survive an edit that keeps the target.
			hc.Tmpfs[target] = previousTmpfs[target]
		default:
			var opts []string
			if m.ReadOnly {
				opts = append(opts, "ro")
			}
			if m.Type == "bind" && m.Propagation != "" {
				opts = append(opts, m.Propagation)
			}
			bind := strings.TrimSpace(m.Source) + ":" + target
			if len(opts) > 0 {
				bind += ":" + strings.Join(opts, ",")
			}
			hc.Binds = append(hc.Binds, bind)
		}
	}
}
