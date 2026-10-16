// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package firstrun guards the routes a fresh install answers before any account
// exists. They are unauthenticated by necessity, so possession of a token printed
// to the container log at boot stands in for "is the person installing this".
package firstrun

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
)

// Header carries the setup token on every first-run request.
const Header = "X-McHarbor-Setup-Token"

// Gate holds the boot-time setup token and serialises the first-run routes, so
// a database choice and the first admin cannot race each other or an attacker.
type Gate struct {
	mu    sync.Mutex
	token string
}

// New mints a token only when needsSetup reports that no account exists yet, and
// logs it once: the log is the only place it appears.
func New(needsSetup func() bool, logger *slog.Logger) *Gate {
	g := &Gate{}
	if needsSetup == nil || !needsSetup() {
		return g
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		if logger != nil {
			logger.Error("first-run: could not mint a setup token; setup stays closed", "error", err)
		}
		return g
	}
	g.token = hex.EncodeToString(raw)
	if logger != nil {
		logger.Warn("first-run setup token: enter it in the installer to set up this instance", "token", g.token)
	}
	return g
}

// Valid reports whether r carries the setup token. A nil gate, or one that
// minted no token, accepts nothing.
func (g *Gate) Valid(r *http.Request) bool {
	if g == nil || g.token == "" {
		return false
	}
	got := r.Header.Get(Header)
	return subtle.ConstantTimeCompare([]byte(got), []byte(g.token)) == 1
}

// Lock serialises a first-run route's check-then-act; Unlock releases it.
func (g *Gate) Lock() { g.mu.Lock() }

// Unlock releases the lock Lock took.
func (g *Gate) Unlock() { g.mu.Unlock() }

// NewWithToken builds a gate around a known token, for tests that drive the
// first-run routes; production code uses New, which mints and logs one.
func NewWithToken(token string) *Gate {
	return &Gate{token: token}
}
