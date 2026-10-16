// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// frontendDir is the relative path inside an installed plugin
// directory that holds the plugin's frontend bundle. The plugin
// tarball's `frontend/dist/` is extracted at
// <dataDir>/plugins/<slug>@<version>/frontend/dist/, which is what
// the host serves. The Dockerfile's `plugin-frontend` stage
// produces this layout — see docker/Dockerfile.
const frontendDir = "frontend"

// MountPluginStatic registers the HTTP handler that delivers
// plugin frontend bundles to the browser. The bundle URL is
// `/plugins/<slug>/<rest>`, where `<rest>` is anything under
// the plugin's installed `frontend/` directory
// (e.g. `/plugins/podman/dist/podman.umd.cjs`).
//
// The host's `core/plugins/PluginPageHost.tsx` lazy-loads the
// bundle via a <script> tag, then calls the
// `window.McharborPlugin_<slug>` lifecycle. See that file for
// the consumer side.
//
// Path traversal is blocked with filepath.Clean + a prefix
// check: the resolved file path must live under
// <Loaded.Path>/<frontendDir>/. A request for
// `/plugins/podman/../../etc/passwd` is rejected.
func (r *Runtime) MountPluginStatic(parent chi.Router) {
	if r == nil {
		return
	}
	parent.Get("/plugins/{slug}/*", func(w http.ResponseWriter, req *http.Request) {
		slug := chi.URLParam(req, "slug")
		rest := chi.URLParam(req, "*")

		// The runtime's Loaded() map is read under the
		// package-level mu; calling it here avoids racing
		// with a Boot/Shutdown pass that adds or removes
		// entries.
		loaded, ok := r.Loaded()[slug]
		if !ok || !loaded.Loaded {
			http.NotFound(w, req)
			return
		}

		// Clean and join: the user-supplied rest is split on
		// '/' by chi and joined back with filepath.Join so
		// backslashes / mixed slashes normalize to the host
		// separator. We still need to defensively reject any
		// path that resolves outside the plugin's frontend
		// dir, because a misbehaving chi or middleware
		// upstream could leak `..` segments.
		cleaned := filepath.Clean("/" + rest)
		if cleaned == "/" {
			http.NotFound(w, req)
			return
		}
		// Strip leading slash so filepath.Join doesn't treat
		// it as an absolute path.
		rel := strings.TrimPrefix(cleaned, string(filepath.Separator))
		pluginRoot := filepath.Join(loaded.Path, frontendDir)
		target := filepath.Join(pluginRoot, rel)

		// Path-traversal guard: target must live under
		// pluginRoot. filepath.Rel returns a path starting
		// with `..` if the join escaped the parent.
		relCheck, err := filepath.Rel(pluginRoot, target)
		if err != nil || strings.HasPrefix(relCheck, "..") || relCheck == ".." {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// No cache: the plugin can be upgraded in place
		// (the installer rewrites the dir on update, see
		// appstore/plugins/installer_default.go) and the
		// old bundle hash would otherwise stick. The
		// frontend has its own asset hashing via Vite if
		// the plugin wants long-cache support; that path is
		// out of scope here.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, target)
	})
}
