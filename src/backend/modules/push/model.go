// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package push delivers Web Push notifications to the on-call PWA. It is core
// rather than a comm plugin because the VAPID key is instance identity and the
// on-call channel must not be removable by the plugin store.
package push

import "github.com/therealmcsparrow/mcharbor/core/pushmsg"

// Subscription is what a client may see of a device: never the endpoint or the
// keys, which are credentials for the push service.
type Subscription struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	UserAgent     string  `json:"userAgent"`
	LastSuccessAt *string `json:"lastSuccessAt,omitempty"`
	FailureCount  int     `json:"failureCount"`
	CreatedAt     string  `json:"createdAt"`
}

// SubscribeRequest is the browser's PushSubscription plus a label the user chose.
type SubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Label     string `json:"label"`
	UserAgent string `json:"userAgent"`
}

// PublicKey is the VAPID application server key the browser subscribes with.
type PublicKey struct {
	PublicKey string `json:"publicKey"`
}

// TestResult reports how a test notification fared on each of the caller's devices.
type TestResult struct {
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
	Removed int `json:"removed"`
}

// Notification is the push payload; it lives in core/pushmsg so a module can
// raise one without importing push.
type Notification = pushmsg.Notification

// Audience picks recipients by permission; it lives in core/pushmsg so a
// module can address one without importing push.
type Audience = pushmsg.Audience
