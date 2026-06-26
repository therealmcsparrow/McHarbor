// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"sync"
)

// ScannerRegistry manages available vulnerability scanners.
type ScannerRegistry struct {
	mu        sync.RWMutex
	scanners  map[string]Scanner
	clairURL  string
}

// NewScannerRegistry creates a registry with all supported scanners.
func NewScannerRegistry(clairURL string) *ScannerRegistry {
	r := &ScannerRegistry{
		scanners: make(map[string]Scanner),
		clairURL: clairURL,
	}

	r.scanners["trivy"] = &TrivyScanner{}
	r.scanners["grype"] = &GrypeScanner{}

	if clairURL != "" {
		r.scanners["clair"] = NewClairScanner(clairURL)
	}

	return r
}

// Reload rebuilds the scanner list using the supplied Clair URL. It is
// safe to call concurrently; in-flight Get/Available calls are protected
// by an RWMutex so a concurrent scan request sees the previous snapshot
// until the new map is installed.
func (r *ScannerRegistry) Reload(clairURL string) {
	scanners := make(map[string]Scanner)
	scanners["trivy"] = &TrivyScanner{}
	scanners["grype"] = &GrypeScanner{}
	if clairURL != "" {
		scanners["clair"] = NewClairScanner(clairURL)
	}
	r.mu.Lock()
	r.scanners = scanners
	r.clairURL = clairURL
	r.mu.Unlock()
}

// Get returns a scanner by name.
func (r *ScannerRegistry) Get(name string) (Scanner, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.scanners[name]
	return s, ok
}

// Available returns info about all registered scanners.
func (r *ScannerRegistry) Available() []ScannerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var infos []ScannerInfo
	for _, s := range r.scanners {
		infos = append(infos, ScannerInfo{
			Name:      s.Name(),
			Available: s.Available(),
		})
	}
	return infos
}
