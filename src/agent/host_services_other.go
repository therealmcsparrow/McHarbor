// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package main

import "errors"

// newHostServiceExecutor refuses: systemd exists only on Linux.
func newHostServiceExecutor() (*hostServiceExecutor, error) {
	return nil, errors.New("host services are only supported on Linux hosts running systemd")
}

// hostServicesCapable is always false off Linux.
func hostServicesCapable() bool {
	return false
}
