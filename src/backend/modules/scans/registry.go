// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

// ScannerRegistry manages available vulnerability scanners.
type ScannerRegistry struct {
	scanners map[string]Scanner
}

// NewScannerRegistry creates a registry with all supported scanners.
func NewScannerRegistry(clairURL string) *ScannerRegistry {
	r := &ScannerRegistry{
		scanners: make(map[string]Scanner),
	}

	r.scanners["trivy"] = &TrivyScanner{}
	r.scanners["grype"] = &GrypeScanner{}

	if clairURL != "" {
		r.scanners["clair"] = NewClairScanner(clairURL)
	}

	return r
}

// Get returns a scanner by name.
func (r *ScannerRegistry) Get(name string) (Scanner, bool) {
	s, ok := r.scanners[name]
	return s, ok
}

// Available returns info about all registered scanners.
func (r *ScannerRegistry) Available() []ScannerInfo {
	var infos []ScannerInfo
	for _, s := range r.scanners {
		infos = append(infos, ScannerInfo{
			Name:      s.Name(),
			Available: s.Available(),
		})
	}
	return infos
}
