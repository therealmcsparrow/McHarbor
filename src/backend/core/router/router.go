// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/therealmcsparrow/mcharbor/core/apikeyusage"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/demo"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/requestid"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// App holds all shared dependencies for the application.
type App struct {
	DB interface {
		QueryRow(query string, args ...interface{}) interface{}
	} // placeholder
	RawDB      interface{}
	DockerPool interface{}
	Auth       *auth.Service
	Encryption interface{}
	Config     interface{}
	Logger     interface{}
}

// New creates the master chi router with all middleware and routes mounted.
func New(app *AppDeps) chi.Router {
	r := chi.NewRouter()

	audit.SetAPIKeyResolver(func(ctx context.Context) (string, string) {
		if p := rbac.APIKeyFromContext(ctx); p != nil {
			return p.KeyID, p.Name
		}
		return "", ""
	})

	// Global middleware
	// Outside Recovery, so a recovered panic is still counted as the 500 it became.
	r.Use(requestid.Middleware)
	r.Use(telemetry.HTTPMiddleware)
	r.Use(middleware.Recovery(app.Logger))
	r.Use(middleware.Logger(app.Logger))
	r.Use(middleware.SecurityHeaders(app.Config.HSTSMaxAge))
	r.Use(middleware.ContentSecurityPolicy(app.Config.ContentSecurityPolicy, app.Config.CSPReportOnly))
	// Build allowed origins from config or use safe defaults
	allowedOrigins := buildAllowedOrigins(app)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Accept-Language", "Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(i18n.Middleware)

	// API routes (authenticated)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.MaxJSONBodySize(8 << 20))
		// Carries the node that executed a request's remote work, which the
		// forwarding transport writes into and the audit row reads back out.
		r.Use(audit.ExecutorMiddleware)
		// Pauses mutating requests while the database is being copied to
		// another SQL backend. A no-op at every other moment.
		r.Use(middleware.Maintenance())
		// Refuses writes and agent connections while this node is an
		// active/passive standby. A no-op on every other node.
		r.Use(middleware.Standby())
		// Refuses the routes that reach past the simulated estate while the
		// instance runs in demo mode. A no-op on every other boot.
		r.Use(demo.Guard(func(w http.ResponseWriter, r *http.Request) {
			response.ForbiddenCode(w, r, i18n.ErrForbidden)
		}))

		// Public endpoints (no auth middleware or auth rate limit)
		app.MountPublicRoutes(r)

		// Auth endpoints (rate-limited, no auth middleware)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit(10, app.ClientKey))
			app.MountAuthRoutes(r)
		})

		// Protected API routes (API key auth falls through to session auth)
		r.Group(func(r chi.Router) {
			r.Use(rbac.APIKeyMiddleware(app.DB, app.AuthService, apiKeyUsageRecording(app)))
			r.Use(auth.Middleware(app.AuthService))
			r.Use(middleware.MFAEnrollmentGate(app.mfaEnrollmentOwed))
			r.Use(selfguard.Middleware(app.RBACService, app.AuditLog))
			// After authentication, so the principal exists; before the
			// audit fallback, so the recorded duration covers the handler
			// rather than the audit write.
			r.Use(apikeyusage.Middleware(app.APIKeyUsage, apiKeyIDFromContext, app.ResolveClientAddress))
			// Ahead of the audit fallback: a held request is audited by the
			// gate as a request, and only its replay is audited as the action.
			if app.ApprovalGate != nil {
				r.Use(app.ApprovalGate)
			}
			r.Use(audit.FallbackMiddleware(app.DB, app.AuditLog))
			app.MountProtectedRoutes(r)
		})

		// Plugin routes.
		//
		// Mounted as one catch-all rather than inside the three groups
		// above, for two reasons. chi panics on more than one `/*` in a
		// mux, so three group-level catch-alls is not an option; and a
		// catch-all inside the protected group would put the session
		// middleware in front of every mistyped /api path, answering 401
		// where the host has always answered 404. So the groups hand
		// their chains to the plugin router instead, and it applies the
		// right one per route — same middleware, same order, decided at
		// rebuild time instead of at mount time.
		//
		// Last in the /api subrouter: chi prefers a static match over a
		// wildcard regardless of registration order, but reading order
		// should still say that host modules own their paths first.
		if app.PluginRoutes != nil {
			app.PluginRoutes.SetChains(
				nil, // public: the /api subrouter's own middleware is all it gets
				middleware.RateLimit(10, app.ClientKey),
				chain(
					rbac.APIKeyMiddleware(app.DB, app.AuthService, apiKeyUsageRecording(app)),
					auth.Middleware(app.AuthService),
					middleware.MFAEnrollmentGate(app.mfaEnrollmentOwed),
					selfguard.Middleware(app.RBACService, app.AuditLog),
					apikeyusage.Middleware(app.APIKeyUsage, apiKeyIDFromContext, app.ResolveClientAddress),
					audit.FallbackMiddleware(app.DB, app.AuditLog),
				),
			)
			r.Mount("/", app.PluginRoutes)
		}
	})

	app.Root = r

	// Serve React static files
	staticDir := app.StaticDir
	if staticDir == "" {
		staticDir = "./static"
	}

	// Check if static directory exists
	if _, err := os.Stat(staticDir); err == nil {
		fileServer(r, "/", staticDir)
	}

	return r
}

// apiKeyIDFromContext adapts the RBAC principal to the usage middleware's
// resolver. Defined here rather than in core/apikeyusage so that package can
// stay free of a core/rbac import — rbac depends on apikeyusage for the Sink,
// and the reverse edge would be a cycle.
func apiKeyIDFromContext(ctx context.Context) string {
	principal := rbac.APIKeyFromContext(ctx)
	if principal == nil {
		return ""
	}
	return principal.KeyID
}

// apiKeyUsageRecording bundles the telemetry hooks for the Bearer middleware.
// Returns nil when nothing is wired, which the middleware treats as "record
// nothing" rather than as an error.
func apiKeyUsageRecording(app *AppDeps) *rbac.UsageRecording {
	if app == nil || app.APIKeyUsage == nil {
		return nil
	}
	return &rbac.UsageRecording{
		Sink:           app.APIKeyUsage,
		ResolveAddress: app.ResolveClientAddress,
	}
}

// buildAllowedOrigins returns the list of allowed CORS origins.
// If ALLOWED_ORIGINS is set in config, it is split by comma.
// Otherwise, defaults to the app's own dev and production origins.
func buildAllowedOrigins(app *AppDeps) []string {
	return app.Config.AllowedOriginList()
}

// setStaticCacheControl picks a caching policy for a static file.
//
// Vite content-hashes everything under /assets/, so those URLs can never
// change meaning and are safe to pin for a year. Every other static path is
// stable across builds - index.html names the current asset hashes, and
// /themes/*.css is rewritten in place whenever the theme generator runs - so
// it must be revalidated on each load. "no-cache" permits the browser to keep
// its copy; it just has to ask first, and Last-Modified answers with a 304.
func setStaticCacheControl(w http.ResponseWriter, requestedPath string) {
	if strings.HasPrefix(strings.TrimPrefix(requestedPath, "/"), "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// fileServer serves static files and falls back to index.html for SPA routing.
func fileServer(r chi.Router, path, root string) {
	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}

	r.Get(path+"*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(http.Dir(root)))

		// Try to serve the actual file first
		requestedPath := strings.TrimPrefix(r.URL.Path, pathPrefix)
		fullPath := filepath.Join(root, requestedPath)

		// Go sends Last-Modified but no Cache-Control, which makes every
		// response heuristically cacheable: with no explicit lifetime a
		// browser invents one, ~10% of the response's age. A shell that
		// was already a few hours old therefore stayed "fresh" for hours,
		// so a new build was never fetched at all.
		//
		// Only /assets/ is safe to cache, because Vite content-hashes those
		// filenames. Everything else lives at a stable URL and must be
		// revalidated - index.html names the current asset hashes, and
		// /themes/*.css is rewritten in place by the theme generator.
		// Last-Modified makes the revalidation a 304, not a re-download.
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// Serve index.html for SPA client-side routing. The header is
			// chosen here rather than above so a miss under /assets/ - which
			// falls through to the shell - is not stamped immutable.
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}

		setStaticCacheControl(w, requestedPath)
		fs.ServeHTTP(w, r)
	})
}
