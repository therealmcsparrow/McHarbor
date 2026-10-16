// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// The install's own logo, as the default mark on every report.
//
// An operator who has rebranded McHarbor in Settings → Customization should
// not have to upload the same two images again here. So when this plugin's
// own marks are unset, the cover falls back to the host's — and only then to
// the McHarbor mark compiled into the plugin.
//
// Two details make this workable:
//
//   - The **light** marks, not the dark ones. A report is printed,
//     photocopied and pasted into decks; it renders on a light ground, which
//     is the same reason theme.go builds a light palette rather than
//     mirroring the dark-first UI.
//   - The **PNG renditions**, not the stored `data:` URIs. The PDF and DOCX
//     writers here embed raster images only — `frontend/src/pages/mark-image.ts`
//     explains why an SVG rasteriser is the wrong dependency for a cover logo
//     — and a logo is usually uploaded as SVG. The host rasterises at upload
//     time in the browser and publishes the result on `/api/branding`, so
//     there is a PNG waiting here with no rasteriser on either side.
//
// `GET /api/branding` is a public host endpoint, so unlike the collectors in
// hostapi.go this needs no session cookie. It is also not user-specific:
// there is one logo per install, and every report carries it.

// hostBranding is the subset of the host's branding response this plugin
// reads. The `*Png` fields are base64 with no `data:` prefix, which is the
// same shape the plugin stores its own marks in — so the fallback is a
// straight substitution with no format conversion anywhere.
type hostBranding struct {
	LogoWidePNG string `json:"logoWidePng"`
	LogoIconPNG string `json:"logoIconPng"`
}

// brandingTTL bounds how stale a cover logo may be.
//
// Short enough that an operator who changes the logo and immediately exports
// sees the new one; long enough that a batch of scheduled reports does not
// make one loopback request each. The logo is not data a report is read for,
// so a minute of staleness costs nothing.
const brandingTTL = time.Minute

var (
	brandingMu      sync.Mutex
	brandingCache   hostBranding
	brandingFetched time.Time
)

// fetchHostBranding returns the install's marks, cached for brandingTTL.
//
// Every failure path returns the zero value rather than an error: a cover
// page that falls back to the compiled-in McHarbor mark is a good report,
// and failing an export because a logo lookup timed out would not be.
func fetchHostBranding(ctx context.Context) hostBranding {
	brandingMu.Lock()
	if time.Since(brandingFetched) < brandingTTL {
		defer brandingMu.Unlock()
		return brandingCache
	}
	brandingMu.Unlock()

	got, ok := requestHostBranding(ctx)

	brandingMu.Lock()
	defer brandingMu.Unlock()
	// The timestamp moves even on failure, so a host that is down does not
	// earn a fresh request per export.
	brandingFetched = time.Now()
	if ok {
		brandingCache = got
	}
	return brandingCache
}

// requestHostBranding tries each candidate base URL once.
func requestHostBranding(ctx context.Context) (hostBranding, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			// Loopback to this same process; see newHostAPI for why.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // loopback to this same process
		},
	}

	for _, base := range resolveBaseURL() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/branding", nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		var env struct {
			Success bool         `json:"success"`
			Data    hostBranding `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		if err != nil || !env.Success {
			continue
		}
		return env.Data, true
	}
	return hostBranding{}, false
}
