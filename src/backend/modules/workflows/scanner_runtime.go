// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import "context"

// ImageScanInput contains the scan request data needed by workflow nodes.
type ImageScanInput struct {
	ImageRef      string
	Scanner       string
	EnvironmentID string
}

// ImageScanResult contains the scan fields exposed to workflow messages.
type ImageScanResult struct {
	ID            string
	ImageRef      string
	Scanner       string
	Status        string
	Severity      string
	TotalVulns    int
	CriticalCount int
	HighCount     int
	MediumCount   int
	LowCount      int
}

// ImageScanner adapts vulnerability scanning into workflows without coupling
// the workflows module to the concrete scans module.
type ImageScanner interface {
	StartScan(ctx context.Context, input ImageScanInput) (*ImageScanResult, error)
	StartScanSync(ctx context.Context, input ImageScanInput) (*ImageScanResult, error)
}
