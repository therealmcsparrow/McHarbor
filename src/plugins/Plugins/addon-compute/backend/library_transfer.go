// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

// libraryGrantTTL bounds how long a pull grant stays usable. Long enough for a
// multi-gigabyte image over a slow link, short enough that a leaked token is
// not a standing key to the image library.
const libraryGrantTTL = 6 * time.Hour

// libraryGrant authorizes one agent to pull one uploaded image.
type libraryGrant struct {
	token   string
	path    string
	imageID string
	expires time.Time
}

// libraryGrants issues and redeems the grants.
//
// A grant is not single-use: a transfer that drops half way is retried by the
// agent, and invalidating the token on first contact would turn every retry
// into a failure the operator has to chase. It expires instead.
type libraryGrants struct {
	mu     sync.Mutex
	grants map[string]libraryGrant
}

func newLibraryGrants() *libraryGrants {
	return &libraryGrants{grants: make(map[string]libraryGrant)}
}

// issue records a grant for one image file and returns its id and token.
func (g *libraryGrants) issue(imageID, path string) (string, string, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", err
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	id := hex.EncodeToString(idBytes)
	token := hex.EncodeToString(tokenBytes)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()
	g.grants[id] = libraryGrant{
		token:   token,
		path:    path,
		imageID: imageID,
		expires: time.Now().Add(libraryGrantTTL),
	}
	return id, token, nil
}

// redeem returns the file path a grant authorizes, and whether the token is
// the one that was issued.
//
// The comparison is constant-time: this is the only thing standing between an
// unauthenticated route and the image library, so it must not leak the token
// through how long a wrong guess takes to reject.
func (g *libraryGrants) redeem(id, token string) (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()

	grant, ok := g.grants[id]
	if !ok {
		return "", false
	}
	if subtle.ConstantTimeCompare([]byte(grant.token), []byte(token)) != 1 {
		return "", false
	}
	return grant.path, true
}

// revoke drops a grant once its transfer has finished.
func (g *libraryGrants) revoke(id string) {
	g.mu.Lock()
	delete(g.grants, id)
	g.mu.Unlock()
}

// sweepLocked discards expired grants. Called on every issue and redeem, which
// is often enough for a map that holds one entry per in-flight transfer.
func (g *libraryGrants) sweepLocked() {
	now := time.Now()
	for id, grant := range g.grants {
		if now.After(grant.expires) {
			delete(g.grants, id)
		}
	}
}
