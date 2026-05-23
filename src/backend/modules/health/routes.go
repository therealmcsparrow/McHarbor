// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package health

import (
	"net/http"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

const version = "1.1.8"

// directDeps maps Go module paths to their display names for /about.
var directDeps = map[string]string{
	"github.com/go-chi/chi/v5":         "chi",
	"github.com/go-chi/cors":           "chi-cors",
	"github.com/docker/docker":         "docker-sdk",
	"github.com/docker/go-connections": "docker-connections",
	"k8s.io/client-go":                 "k8s-client-go",
	"k8s.io/api":                       "k8s-api",
	"k8s.io/apimachinery":              "k8s-apimachinery",
	"modernc.org/sqlite":               "sqlite",
	"github.com/gorilla/websocket":     "websocket",
	"github.com/dop251/goja":           "goja",
	"github.com/rs/xid":                "xid",
	"golang.org/x/crypto":              "x/crypto",
	"golang.org/x/oauth2":              "x/oauth2",
	"github.com/caarlos0/env/v11":      "env",
	"github.com/google/uuid":           "uuid",
	"github.com/PuerkitoBio/goquery":   "goquery",
	"gopkg.in/yaml.v3":                 "yaml",
}

type aboutResponse struct {
	Version      string           `json:"version"`
	GoVersion    string           `json:"goVersion"`
	Platform     string           `json:"platform"`
	Dependencies []dependencyInfo `json:"dependencies"`
}

type dependencyInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Mount registers health check routes (no authentication required).
func Mount(app *router.AppDeps) {
	app.RegisterAuthRoutes(func(r chi.Router) {
		r.Get("/health", handleHealth)
		r.Get("/about", handleAbout)
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	response.OK(w, map[string]string{"status": "ok"})
}

func handleAbout(w http.ResponseWriter, r *http.Request) {
	goVer := runtime.Version()
	platform := runtime.GOOS + "/" + runtime.GOARCH

	var deps []dependencyInfo
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if label, ok := directDeps[dep.Path]; ok {
				ver := dep.Version
				// Strip +incompatible suffix for cleaner display
				ver = strings.TrimSuffix(ver, "+incompatible")
				deps = append(deps, dependencyInfo{Name: label, Version: ver})
			}
		}
	}
	if deps == nil {
		deps = []dependencyInfo{}
	}

	response.OK(w, aboutResponse{
		Version:      version,
		GoVersion:    goVer,
		Platform:     platform,
		Dependencies: deps,
	})
}
