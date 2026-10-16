// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"github.com/rs/xid"
)

// newPolicyID returns a fresh xid. Centralised so tests can stub it.
func newPolicyID() (string, error) {
	return xid.New().String(), nil
}

// newHistoryID returns a fresh xid for history rows.
func newHistoryID() string {
	return xid.New().String()
}
