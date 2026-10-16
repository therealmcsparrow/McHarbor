// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import "time"

const (
	// hostCallTimeout bounds one Docker or host-command call; the agent transport has no
	// deadline of its own, so a wedged remote host would otherwise hold the request.
	hostCallTimeout = 30 * time.Second
	// hostProbeTimeout bounds a cheap liveness or inspect probe that should answer at once.
	hostProbeTimeout = 10 * time.Second
)
