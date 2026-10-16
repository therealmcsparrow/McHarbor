// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package dockerapi answers the Docker Engine API from a simulated estate host.
// The demo agent feeds it every proxied request, so the Docker SDK inside
// McHarbor sees a real-looking daemon while nothing real is touched.
package dockerapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/swarm"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// APIVersion is the Engine API version the simulated daemon advertises.
const APIVersion = "1.47"

var versionPrefix = regexp.MustCompile(`^/v[0-9]+\.[0-9]+`)

// SwarmBackend answers the swarm half of the Engine API for a host that is a
// simulated swarm manager; ServeSwarm returns false for a path it does not own.
type SwarmBackend interface {
	ServeSwarm(w http.ResponseWriter, r *http.Request, path string) bool
	Info() *swarm.Info
}

// Server is the simulated daemon for one host.
type Server struct {
	host  *estate.Host
	now   func() time.Time
	swarm SwarmBackend
}

// SetSwarm makes this host a swarm manager answered by b. It must be called
// before the server handles requests.
func (s *Server) SetSwarm(b SwarmBackend) { s.swarm = b }

// New returns a Server over host.
func New(host *estate.Host) *Server {
	return &Server{host: host, now: time.Now}
}

// ServeHTTP routes a Docker Engine API request to the simulator.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := versionPrefix.ReplaceAllString(r.URL.Path, "")
	w.Header().Set("Api-Version", APIVersion)
	w.Header().Set("Docker-Experimental", "false")
	w.Header().Set("Ostype", "linux")
	w.Header().Set("Server", "Docker/"+s.host.DockerVersion+" (linux)")
	switch {
	case path == "/_ping":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		if r.Method != http.MethodHead {
			if _, err := w.Write([]byte("OK")); err != nil {
				return
			}
		}
	case path == "/version":
		s.version(w)
	case path == "/info":
		s.info(w)
	case path == "/system/df":
		s.diskUsage(w)
	case path == "/events":
		s.events(w, r)
	case s.swarm != nil && s.swarm.ServeSwarm(w, r, path):
	case strings.HasPrefix(path, "/swarm") || strings.HasPrefix(path, "/services") || strings.HasPrefix(path, "/nodes") || strings.HasPrefix(path, "/tasks") || strings.HasPrefix(path, "/secrets") || strings.HasPrefix(path, "/configs"):
		writeError(w, http.StatusServiceUnavailable, "This node is not a swarm manager. Use \"docker swarm init\" or \"docker swarm join\" to connect this node to swarm and try again.")
	case strings.HasPrefix(path, "/containers"):
		s.containers(w, r, strings.TrimPrefix(path, "/containers"))
	case strings.HasPrefix(path, "/exec/"):
		s.exec(w, r, strings.TrimPrefix(path, "/exec/"))
	case strings.HasPrefix(path, "/images"):
		s.images(w, r, strings.TrimPrefix(path, "/images"))
	case strings.HasPrefix(path, "/volumes"):
		s.volumes(w, r, strings.TrimPrefix(path, "/volumes"))
	case strings.HasPrefix(path, "/networks"):
		s.networks(w, r, strings.TrimPrefix(path, "/networks"))
	case strings.HasPrefix(path, "/distribution/"):
		s.distribution(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/distribution/"), "/json"))
	default:
		notInDemo(w)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}

func atoiOr(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

func notInDemo(w http.ResponseWriter) {
	writeError(w, http.StatusNotImplemented, "This operation is not available in demo mode.")
}

func writeEstateError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, estate.ErrNotFound):
		writeError(w, http.StatusNotFound, "No such "+what)
	case errors.Is(err, estate.ErrConflict):
		writeError(w, http.StatusConflict, "The "+what+" is in a state that does not allow this operation")
	default:
		writeError(w, http.StatusInternalServerError, "The simulated daemon could not complete the request")
	}
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func queryBool(r *http.Request, key string) bool {
	v := r.URL.Query().Get(key)
	return v == "1" || strings.EqualFold(v, "true")
}

// parseFilters parses the Engine API filters parameter in both of its encodings,
// {"k":{"v":true}} and the older {"k":["v"]}.
func parseFilters(r *http.Request) map[string][]string {
	raw := r.URL.Query().Get("filters")
	out := map[string][]string{}
	if raw == "" {
		return out
	}
	var modern map[string]map[string]bool
	if err := json.Unmarshal([]byte(raw), &modern); err == nil {
		for k, set := range modern {
			for v, on := range set {
				if on {
					out[k] = append(out[k], v)
				}
			}
		}
		return out
	}
	var legacy map[string][]string
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil {
		return legacy
	}
	return out
}

func labelsMatch(labels map[string]string, wanted []string) bool {
	for _, want := range wanted {
		k, v, hasValue := strings.Cut(want, "=")
		got, ok := labels[k]
		if !ok || (hasValue && got != v) {
			return false
		}
	}
	return true
}
