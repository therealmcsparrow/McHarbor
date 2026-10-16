// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"

	"github.com/therealmcsparrow/mcharbor/core/notify"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

// releaseNotifier announces a new agent release on the channels the McHarbor
// self-update uses, falling back to the in-app channel, so both kinds of
// release reach the operator the same way.
func releaseNotifier(dispatcher *notify.Dispatcher, app *router.AppDeps) agentreleases.Notifier {
	return func(ctx context.Context, title, message string) {
		if dispatcher == nil {
			return
		}
		checker := NewSelfUpdateChecker(app.DB, dispatcher, app.Logger)
		settings, err := checker.Settings(ctx)
		if err != nil {
			app.Logger.Warn("agent-releases: reading notification channels", "error", err)
		}
		checker.notifyText(ctx, settings, title, message)
	}
}

// releaseLeader limits the scheduled release check to the HA leader, the same
// election the self-update checker uses, so two nodes never pull one release.
func releaseLeader(app *router.AppDeps) agentreleases.LeaderFunc {
	registered, ok := app.LookupService("coordinator")
	if !ok {
		return nil
	}
	coord, ok := registered.(checkerCoordinator)
	if !ok {
		return nil
	}
	return func(ctx context.Context) bool { return coord.LeaderOf(ctx, "agent-release-checker") }
}
