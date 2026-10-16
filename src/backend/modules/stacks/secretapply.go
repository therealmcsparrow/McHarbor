// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"errors"

	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// secretApplier redeploys one stack so it runs with a secret's current value.
// Registered into the vault's applier registry at mount time, because
// core/secrets cannot import this package.
type secretApplier struct {
	service *Service
}

// Apply runs `docker compose up -d`, which re-resolves every `${secret:NAME}`
// in the stack's stored environment on the way through.
//
// A stack needs no file-mode special case the way a container does: compose
// replaces the services whose configuration changed, so the volume is released
// and reseeded as part of the ordinary deploy.
//
// The reference addresses a stack by id and carries the name; `Up` takes the
// name, and that is the identifier a compose project actually has.
func (a secretApplier) Apply(ctx context.Context, target secrets.ApplyTarget) error {
	name := stackName(target)

	result := a.service.Up(ctx, name)
	if result == nil {
		return errors.New("compose returned no result")
	}
	if !result.Success {
		if result.Error != "" {
			return errors.New(result.Error)
		}
		return errors.New("compose deploy failed")
	}
	return nil
}

// Resolve returns the stack's database id and whether it is still defined.
//
// A stack that was deleted leaves its reference rows behind, and without this
// the impact list would keep asking the operator to redeploy something that no
// longer has a compose file. The id is returned for the same reason the
// container applier returns one: a reference recorded under the stack's name
// and another under its id are the same stack, and should read as one row.
func (a secretApplier) Resolve(ctx context.Context, target secrets.ApplyTarget) (string, bool, error) {
	st, err := a.service.ByName(ctx, stackName(target))
	if err != nil {
		return "", false, err
	}
	if st == nil {
		return "", false, nil
	}
	return st.ID, true, nil
}

// stackName picks the identifier compose actually understands. References are
// keyed on the stack's database id; the name is what `docker compose` and the
// operator both use.
func stackName(target secrets.ApplyTarget) string {
	if target.ResourceName != "" {
		return target.ResourceName
	}
	return target.ResourceID
}
