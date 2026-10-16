// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package health serves the health, version and about endpoints.
package health

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/google/uuid"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/clusterinfo"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

var readBuildInfo = debug.ReadBuildInfo

// directDeps maps Go module paths to their display names for /about.
var directDeps = map[string]string{
	"github.com/go-chi/cors":           "chi-cors",
	"github.com/moby/moby/client":      "docker-sdk",
	"github.com/docker/go-connections": "docker-connections",
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
	Build        string           `json:"build"`
	GoVersion    string           `json:"goVersion"`
	Platform     string           `json:"platform"`
	Dependencies []dependencyInfo `json:"dependencies"`
	// Native is true when McHarbor runs directly on its host under systemd, so the interface
	// can name the native way to do something instead of a container restart or compose file.
	Native bool `json:"native"`
	// Sandboxed is true for a native install running as the mcharbor account (installed with
	// --no-run-as-root), where systemd lets it write only under its data directory.
	Sandboxed bool `json:"sandboxed"`
}

// nativeHost is set once at start-up from main.go; health may not import modules/system.
var nativeHost bool

// SetNativeHost records whether McHarbor runs directly on its host rather than in a container.
func SetNativeHost(native bool) { nativeHost = native }

type dependencyInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type healthResponse struct {
	Status     string `json:"status"`
	InstanceID string `json:"instanceId"`
	NodeID     string `json:"nodeId,omitempty"`
	// Role lets every signed-in page show the standby banner without cluster.view.
	Role string `json:"role,omitempty"`
}

// probeResponse is what /livez and /readyz return: a verdict per check and nothing
// else, because the probes are public.
type probeResponse struct {
	Status string                   `json:"status"`
	NodeID string                   `json:"nodeId,omitempty"`
	Checks []clusterinfo.ReadyCheck `json:"checks,omitempty"`
}

// readiness is the slice of the cluster service the probes use.
type readiness = clusterinfo.Readiness

// Mount registers public health and runtime metadata routes.
func Mount(app *router.AppDeps) {
	instanceID := uuid.NewString()
	app.RegisterPublicRoutes(func(r *apispec.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			handleHealthFor(w, r, instanceID, nodeID(app), nodeRole(app))
		})
		r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
			response.OK(w, probeResponse{Status: "alive", NodeID: nodeID(app)})
		})
		r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
			handleReady(w, r, lookupReadiness(app))
		})
		r.Get("/about", handleAbout)
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request, instanceID string) {
	handleHealthFor(w, r, instanceID, "", "")
}

func handleHealthFor(w http.ResponseWriter, _ *http.Request, instanceID, node, role string) {
	response.OK(w, healthResponse{Status: "ok", InstanceID: instanceID, NodeID: node, Role: role})
}

func nodeRole(app *router.AppDeps) string {
	if rd := lookupReadiness(app); rd != nil && rd.Membership() != nil {
		return rd.Membership().Role()
	}
	return ""
}

func lookupReadiness(app *router.AppDeps) readiness {
	svc, ok := app.LookupService("cluster")
	if !ok {
		return nil
	}
	p, ok := svc.(clusterinfo.Provider)
	if !ok {
		return nil
	}
	return p.Info()
}

func nodeID(app *router.AppDeps) string {
	if rd := lookupReadiness(app); rd != nil && rd.Membership() != nil {
		return rd.Membership().NodeID()
	}
	return ""
}

// handleReady answers 503 until the node can take traffic, which is what lets a load
// balancer keep a standby, draining or database-less node out of rotation.
func handleReady(w http.ResponseWriter, r *http.Request, rd readiness) {
	if rd == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "data": probeResponse{Status: "starting"}}); err != nil {
			return
		}
		return
	}
	ok, checks := rd.Ready(r.Context())
	body := probeResponse{Status: "ready", Checks: checks}
	if reg := rd.Membership(); reg != nil {
		body.NodeID = reg.NodeID()
	}
	if ok {
		response.OK(w, body)
		return
	}
	body.Status = "not_ready"
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(map[string]any{"success": false, "data": body}); err != nil {
		return
	}
}

func handleAbout(w http.ResponseWriter, r *http.Request) {
	goVer := runtime.Version()
	platform := runtime.GOOS + "/" + runtime.GOARCH

	var deps []dependencyInfo
	if info, ok := readBuildInfo(); ok {
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
		Version:      appversion.Current(),
		Build:        appversion.Build(),
		GoVersion:    goVer,
		Platform:     platform,
		Dependencies: deps,
		Native:       nativeHost,
		Sandboxed:    nativeHost && os.Geteuid() != 0,
	})
}
