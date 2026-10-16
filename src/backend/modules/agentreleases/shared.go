// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

var (
	sharedMu sync.Mutex
	shared   *Store
)

// Shared returns the process-wide store, creating it on first use. The agent
// module serves from it and the updates module manages it, and both must see
// the same in-flight pulls, so there is exactly one.
func Shared(app *router.AppDeps) *Store {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared != nil {
		return shared
	}
	if app == nil || app.DB == nil || app.Logger == nil {
		return nil
	}
	images, err := NewDockerImageStore()
	if err != nil {
		app.Logger.Warn("agent-releases: no local Docker daemon; container agent images cannot be stored", "error", err)
		images = nil
	}
	store, err := NewStore(Options{
		DB:         app.DB,
		Logger:     app.Logger,
		Root:       filepath.Join(apppaths.DataDir(), "agent-releases"),
		Images:     images,
		InUse:      runningVersions(app),
		Background: app.ContextOrBackground(),
	})
	if err != nil {
		app.Logger.Error("agent-releases: store unavailable; agents are served only built-in binaries", "error", err)
		return nil
	}
	shared = store
	return shared
}

// Current returns the shared store, or nil before Shared has run. Package-level
// helpers such as the binary route's precedence read it, so a test that never
// mounts the module simply sees no store.
func Current() *Store {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	return shared
}

// SetCurrentForTest installs s as the shared store and returns a restore func.
func SetCurrentForTest(s *Store) func() {
	sharedMu.Lock()
	prev := shared
	shared = s
	sharedMu.Unlock()
	return func() {
		sharedMu.Lock()
		shared = prev
		sharedMu.Unlock()
	}
}

// TargetBinaryPath is Store.TargetBinaryPath on the shared store.
func TargetBinaryPath(ctx context.Context, osName, arch string) (path, version, build string) {
	s := Current()
	if s == nil {
		return "", "", ""
	}
	return s.TargetBinaryPath(ctx, osName, arch)
}

// runningVersions lists what connected agents run, as "version@build" when the
// agent reports its build, which retention and delete must never remove.
func runningVersions(app *router.AppDeps) func() []string {
	return func() []string {
		if app.AgentPool == nil {
			return nil
		}
		conns := app.AgentPool.List()
		out := make([]string, 0, len(conns))
		for i := range conns {
			v := strings.TrimSpace(conns[i].Version)
			if v == "" {
				continue
			}
			if b := strings.TrimSpace(conns[i].Build); b != "" {
				v += "@" + b
			}
			out = append(out, v)
		}
		return out
	}
}
