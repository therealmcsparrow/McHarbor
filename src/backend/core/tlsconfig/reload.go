// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package tlsconfig

import (
	"errors"
	"os"
	"sync"
)

// isNotExist returns true for any *PathError wrapping os.ErrNotExist
// (including os.ErrNotExist itself). Centralized here so the
// "missing file is OK" semantics are obvious at the call sites.
func isNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

// Reloader holds the active *tls.Config for a TLS listener and exposes
// a thread-safe `Reload` that rebuilds the config from on-disk PEM
// files whenever the operator uploads a replacement certificate.
//
// Why this exists: Go's `http.Server.TLSConfig` is read once at the
// start of each TLS handshake. Rotating the cert on disk does NOT
// automatically swap the in-memory `Certificates` slice — a fresh
// `*tls.Config` with the new `Certificate` must be installed. The
// watcher polls the cert's SHA-256; when it changes, `Reload` parses
// the new keypair and swaps the pointer under a mutex so concurrent
// handshakes never observe a half-loaded config.
//
// `certPath` and `keyPath` are absolute paths to the PEM files. They
// are captured at construction so the watcher can re-read on every
// poll without going through main.go.
//
// `minVersion` mirrors the host's `MCHARBOR_TLS_MIN_VERSION` env
// (`tls.VersionTLS12` / `tls.VersionTLS13`). A minVersion below 1.2
// is rejected by `Build` and surfaces as an error from `Reload`.
//
// `CertFingerprint` returns the colon-separated SHA-256 fingerprint
// of the cert currently loaded into the in-memory config. Used by the
// settings UI to detect whether the operator has uploaded a new cert
// and to populate the "Serial" / "SHA-256 fingerprint" fields.
type Reloader struct {
	certPath   string
	keyPath    string
	minVersion uint16

	mu        sync.RWMutex
	cachedCfg any
	fp        string
}

// NewReloader creates a Reloader bound to `certPath`/`keyPath`.
// Callers should invoke `Reload()` once before handing the result to
// `http.Server.TLSConfig` so the first request never sees a nil
// config.
func NewReloader(certPath, keyPath string, minVersion uint16) *Reloader {
	return &Reloader{
		certPath:   certPath,
		keyPath:    keyPath,
		minVersion: minVersion,
	}
}

// Reload re-reads the on-disk cert + key and, if either has changed
// since the last call, atomically swaps the cached config. Returns
// the new SHA-256 fingerprint of the cert (or the unchanged value if
// nothing changed).
//
// A nil error with an empty fingerprint means the cert file does not
// exist yet — the caller should treat this as "TLS disabled".
func (r *Reloader) Reload() (string, error) {
	cfg, fp, err := BuildWithFingerprint(r.certPath, r.keyPath, r.minVersion)
	if err != nil {
		return "", err
	}
	if fp == "" {
		// Cert file missing — leave the cached config as-is so the
		// listener keeps serving with whatever was loaded last.
		return "", nil
	}
	r.mu.Lock()
	r.cachedCfg = cfg
	r.fp = fp
	r.mu.Unlock()
	return fp, nil
}

// Current returns the last successfully loaded *tls.Config (returned
// as `any` so callers type-assert). Returns nil if Reload has not
// been called yet.
func (r *Reloader) Current() any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cachedCfg
}

// CertFingerprint returns the SHA-256 fingerprint of the cert
// currently held in memory. Empty if no cert has been loaded yet.
func (r *Reloader) CertFingerprint() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fp
}

// OnDiskFingerprint returns the SHA-256 fingerprint of the cert file
// on disk right now (independent of what's loaded in memory). The
// watcher compares this against the last loaded fingerprint to detect
// external writes (admin upload, key regeneration).
func (r *Reloader) OnDiskFingerprint() (string, error) {
	fp, err := Fingerprint(r.certPath)
	if err != nil {
		// Missing file is a non-error for the watcher — fall back to
		// an empty fingerprint so the poll loop keeps ticking.
		if isNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return fp, nil
}

// CertPath exposes the bound cert path so the cert reload
// watcher can write new files into the same location without a
// separate registry.
func (r *Reloader) CertPath() string { return r.certPath }

// KeyPath returns the bound key path; see CertPath.
func (r *Reloader) KeyPath() string { return r.keyPath }
