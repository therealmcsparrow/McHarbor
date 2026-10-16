// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package pushmsg holds the Web Push payload and audience types, so a module
// that raises an on-call notification declares it without importing push.
package pushmsg

import "github.com/therealmcsparrow/mcharbor/core/rbac"

// Notification is the whole payload: it transits a third-party push service and
// is stored there until delivery, so it never carries more than the alert text
// the operator already chose, and a link the app fetches after the tap.
type Notification struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
	URL      string `json:"url"`
	Tag      string `json:"tag,omitempty"`
	// Key names a catalog message rendered in each recipient's language; Args
	// fill its placeholders. Neither leaves the server.
	Key  string            `json:"-"`
	Args map[string]string `json:"-"`
}

// Audience picks recipients: subscribed users who hold Permission for EnvID
// (empty for a global permission). The pipeline has no recipient list of its
// own, so the permission is the list.
type Audience struct {
	Permission rbac.Permission
	EnvID      string
}

// Message keys a Notification can carry instead of fixed text; push owns the
// translated catalog they resolve against.
const (
	// MsgAlertTriggered renders "Alert triggered: {name}".
	MsgAlertTriggered = "alertTriggered"
	// MsgAlertResolved renders "Alert resolved: {name}".
	MsgAlertResolved = "alertResolved"
	// MsgElevationRequested renders an elevation request for approvers.
	MsgElevationRequested = "elevationRequested"
	// MsgTest renders the test notification a user sends to their own devices.
	MsgTest = "test"
)
