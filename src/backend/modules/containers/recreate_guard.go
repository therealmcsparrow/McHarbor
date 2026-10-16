// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// ErrMaskedValueMoved is returned when a recreate keeps a value nobody may reveal (a pinned
// key, or a McHarbor container's own keys) while changing what the container runs.
var ErrMaskedValueMoved = errors.New("recreate keeps a withheld value while changing the image, command, entrypoint or labels")

// SetPinnedSecrets records the root-of-trust values (envredact.PinnedFromConfig) a recreate
// must not let an editor move.
func (s *Service) SetPinnedSecrets(values []string) { s.pinned = values }

// guardMovedWithheld refuses a recreate request that keeps an unrevealable value by its
// marker while changing the image, command, entrypoint, labels or healthcheck: the editor
// cannot read that value, so they must not choose the program it is handed to.
func (s *Service) guardMovedWithheld(req RecreateRequest, info container.InspectResponse) error {
	if info.Config == nil {
		return nil
	}
	policy := envredact.Policy{McHarbor: docker.IsMcHarborInstance(info), Pinned: s.pinned}
	moved := policy.MovedWithheld(req.Env, info.Config.Env)
	if len(moved) == 0 || !recreateChangesExecution(req, info.Config) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrMaskedValueMoved, strings.Join(moved, ","))
}

// recreateChangesExecution reports whether req changes what the container runs.
func recreateChangesExecution(req RecreateRequest, cfg *container.Config) bool {
	if req.Image != "" && req.Image != cfg.Image {
		return true
	}
	if len(req.Cmd) > 0 && !slices.Equal(req.Cmd, []string(cfg.Cmd)) {
		return true
	}
	if len(req.Entrypoint) > 0 && !slices.Equal(req.Entrypoint, []string(cfg.Entrypoint)) {
		return true
	}
	if req.Labels != nil && !maps.Equal(req.Labels, cfg.Labels) {
		return true
	}
	return req.Healthcheck != nil
}
