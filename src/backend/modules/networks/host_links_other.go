// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package networks

import (
	"errors"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// localLinks is Linux-only: macvlan and ipvlan parents exist only on a Linux Docker host.
func localLinks() ([]coreagent.ComputeHostLink, error) {
	return nil, errors.New("listing host links is supported on Linux only")
}
