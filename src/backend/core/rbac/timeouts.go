// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import "time"

// authzQueryTimeout bounds one RBAC lookup or usage write made on the request path, so a
// slow database degrades authorization rather than stalling every request behind it.
const authzQueryTimeout = 2 * time.Second
