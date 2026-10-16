// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"sync/atomic"

	"github.com/therealmcsparrow/mcharbor/core/alertnotice"
)

// AlertNotice is one alert state change as a notification sink sees it; it is
// core/alertnotice's Notice so a sink renders it without importing alerts.
type AlertNotice = alertnotice.Notice

// AlertNotifier receives every notice. It must return quickly: it runs on the
// evaluation loop, so a slow sink enqueues and delivers elsewhere.
type AlertNotifier func(AlertNotice)

// SourceContainer is the source type of alerts whose subject is a container.
const SourceContainer = sourceContainer

// SetAlertNotifier installs fn to hear every alert state change; nil removes it.
func (e *Engine) SetAlertNotifier(fn AlertNotifier) {
	if fn == nil {
		e.alertNotifier.Store(nil)
		return
	}
	e.alertNotifier.Store(&fn)
}

type notifierSlot = atomic.Pointer[AlertNotifier]
