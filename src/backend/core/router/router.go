// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/middleware"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// App holds all shared dependencies for the application.
type App struct {
	DB         interface{ QueryRow(query string, args ...interface{}) interface{} } // placeholder
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

	// Global middleware
	r.Use(middleware.Recovery(app.Logger))
	r.Use(middleware.Logger(app.Logger))
	r.Use(middleware.SecurityHeaders())
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

	// Health check (no auth)
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"status":"ok"}}`))
	})

	// API routes (authenticated)
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.MaxJSONBodySize(8 << 20))

		// Auth endpoints (rate-limited, no auth middleware)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit(10))
			app.MountAuthRoutes(r)
		})

		// Protected API routes (API key auth falls through to session auth)
		r.Group(func(r chi.Router) {
			r.Use(rbac.APIKeyMiddleware(app.DB, app.AuthService))
			r.Use(auth.Middleware(app.AuthService))
			app.MountProtectedRoutes(r)
		})
	})

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

// buildAllowedOrigins returns the list of allowed CORS origins.
// If ALLOWED_ORIGINS is set in config, it is split by comma.
// Otherwise, defaults to the app's own dev and production origins.
func buildAllowedOrigins(app *AppDeps) []string {
	return app.Config.AllowedOriginList()
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

		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// Serve index.html for SPA client-side routing
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}

		fs.ServeHTTP(w, r)
	})
}
