// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"github.com/therealmcsparrow/mcharbor/core/desiredstatesources"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/desiredstate"
)

// registerDesiredStateSource publishes a plugin that opted into the desiredstate
// contract; one without it is dropped, so a reload that removed it takes effect.
func registerDesiredStateSource(slug string, p any) {
	provider, ok := p.(desiredstate.Provider)
	if !ok {
		desiredstatesources.Unregister(slug)
		return
	}
	desiredstatesources.Register(slug, provider)
}
