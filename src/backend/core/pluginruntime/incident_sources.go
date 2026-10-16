// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"github.com/therealmcsparrow/mcharbor/core/incidentsources"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incidentsource"
)

// registerIncidentSource publishes a plugin that opted into the incidentsource
// contract; a plugin without it is dropped, so a reload that removed it takes effect.
func registerIncidentSource(slug string, p any) {
	provider, ok := p.(incidentsource.Provider)
	if !ok {
		incidentsources.Unregister(slug)
		return
	}
	incidentsources.Register(slug, provider)
}
