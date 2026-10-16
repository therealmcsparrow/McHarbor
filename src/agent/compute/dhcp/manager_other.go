// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package dhcp

import "log/slog"

// Manager is the non-Linux stub. Compute is Linux-host-only, and every method
// reports that rather than silently doing nothing.
type Manager struct{}

// NewManager returns a stub Manager.
func NewManager(_ string, _ func(Lease), _ *slog.Logger) *Manager { return &Manager{} }

// Ensure reports that guest networking needs a Linux host.
func (m *Manager) Ensure(_ Network) error { return ErrNotLinux }

// RestoreAll restores nothing off Linux, where no server ever ran.
func (m *Manager) RestoreAll() (int, error) { return 0, nil }

// Remove reports that guest networking needs a Linux host.
func (m *Manager) Remove(_ string) error { return ErrNotLinux }

// Serving always reports false off Linux.
func (m *Manager) Serving(_ string) bool { return false }

// Close is a no-op off Linux.
func (m *Manager) Close() error { return nil }
