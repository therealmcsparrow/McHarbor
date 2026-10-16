// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import "github.com/rs/xid"

// newXID returns a short unique id used for new endpoint / sync-log
// rows. XIDs are 20-character base32 identifiers — short enough for
// URLs and stable for ordering.
func newXID() (string, error) {
	return xid.New().String(), nil
}
