// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import "github.com/therealmcsparrow/mcharbor/core/pluginruntime"

// loadedReporter is the slice of the plugin runtime the store's installed list
// needs, asserted rather than typed because AppDeps holds the runtime as any.
type loadedReporter interface {
	Loaded() map[string]pluginruntime.Loaded
}

// PluginLoadedView returns the plugin store's source for each installed plugin's
// "loaded" flag. The runtime is read through current on every call because it
// boots after the store handler is built, and a plugin counts as loaded only
// when the loader loaded it without an error.
func PluginLoadedView(current func() any) func() map[string]bool {
	return func() map[string]bool {
		out := map[string]bool{}
		runtime, ok := current().(loadedReporter)
		if !ok || runtime == nil {
			return out
		}
		for slug, entry := range runtime.Loaded() {
			out[slug] = entry.Loaded && entry.Err == ""
		}
		return out
	}
}
