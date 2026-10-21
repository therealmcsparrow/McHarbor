// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
)

const (
	// versionProbeTTL bounds how long one successful /version answer vouches for a cluster
	// address; a connection edit drops it sooner through ClientPool.Remove.
	versionProbeTTL     = 5 * time.Minute
	versionProbeTimeout = 10 * time.Second
	// maxStatusBody caps the reply the transport will buffer to look for a Kubernetes Status.
	maxStatusBody = 64 << 10
)

// errAPIUnverified marks an address that answered the /version probe with a 2xx that is not a
// Kubernetes version document, so nothing it sends may be streamed or echoed to the caller.
var errAPIUnverified = errors.New("kubernetes: the address did not answer like a Kubernetes API server")

// apiTrust records, per cluster connection, that the address answered a real /version probe
// recently. The body-stripping transport reads it to decide whether a Status message may pass.
type apiTrust struct {
	mu      sync.Mutex
	until   time.Time
	now     func() time.Time
	probeMu sync.Mutex
}

func newAPITrust(now func() time.Time) *apiTrust {
	if now == nil {
		now = time.Now
	}
	return &apiTrust{now: now}
}

// confirmed reports whether a probe succeeded within the TTL; a nil trust is never confirmed.
func (t *apiTrust) confirmed() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.now().Before(t.until)
}

func (t *apiTrust) confirm() {
	t.mu.Lock()
	t.until = t.now().Add(versionProbeTTL)
	t.mu.Unlock()
}

func (t *apiTrust) revoke() {
	t.mu.Lock()
	t.until = time.Time{}
	t.mu.Unlock()
}

// ensure runs probe unless a recent one succeeded; a nil trust probes every time. Concurrent callers share one probe so a
// dashboard that fires a dozen requests at once costs the cluster a single /version.
func (t *apiTrust) ensure(ctx context.Context, probe func(context.Context) error) error {
	if t == nil {
		return probe(ctx)
	}
	if t.confirmed() {
		return nil
	}
	t.probeMu.Lock()
	defer t.probeMu.Unlock()
	if t.confirmed() {
		return nil
	}
	if err := probe(ctx); err != nil {
		t.revoke()
		return err
	}
	t.confirm()
	return nil
}

// probeAPIServer asks the address for /version and accepts only a 200 whose JSON carries a
// non-empty gitVersion and major. A transport failure or non-2xx comes back as the plain
// client error (an unreachable cluster); any other 2xx is errAPIUnverified, and the body is
// never quoted in either case.
func probeAPIServer(ctx context.Context, cs *kubernetes.Clientset) error {
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	var code int
	raw, err := cs.Discovery().RESTClient().Get().AbsPath("/version").Do(ctx).StatusCode(&code).Raw()
	if err != nil {
		return fmt.Errorf("probing Kubernetes API server: %w", err)
	}
	if code != http.StatusOK {
		return errAPIUnverified
	}
	var info struct {
		Major      string `json:"major"`
		GitVersion string `json:"gitVersion"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return errAPIUnverified
	}
	if strings.TrimSpace(info.GitVersion) == "" || strings.TrimSpace(info.Major) == "" {
		return errAPIUnverified
	}
	return nil
}
