// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import "context"

// StackInstallInput captures the stack payload needed for an app installation.
type StackInstallInput struct {
	Name          string
	Compose       string
	EnvVars       map[string]string
	Description   *string
	EnvironmentID *string
	AutoStart     bool
}

// StackInstallOutput describes the created stack.
type StackInstallOutput struct {
	ID     string
	Name   string
	Status string
}

// InstallScannerResult summarizes a completed vulnerability scan.
type InstallScannerResult struct {
	TotalVulns    int
	CriticalCount int
	HighCount     int
	MediumCount   int
	LowCount      int
}

// StackInstaller creates managed stacks for app installs.
type StackInstaller interface {
	CreateInstalledStack(context.Context, StackInstallInput) (*StackInstallOutput, error)
}

// InstallScanner runs the optional scan-on-install flow.
type InstallScanner interface {
	ScanOnInstall(context.Context, string, string, string) (*InstallScannerResult, error)
}
