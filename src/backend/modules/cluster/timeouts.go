// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cluster

import "time"

// clusterCallTimeout bounds one coordination round-trip (lock, registry write, readiness
// probe) so a stalled database cannot hold the HA loop or startup.
const clusterCallTimeout = 5 * time.Second
