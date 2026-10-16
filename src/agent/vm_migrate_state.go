// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

// maxMigrationTargets bounds how many prepared targets one host holds.
const maxMigrationTargets = 32

// errMigrationUnknown covers an unknown migration id and a wrong token alike, so
// a caller probing ids learns nothing about which ones exist.
var errMigrationUnknown = errors.New("compute: no such migration on this host, or the token does not match")

// migratedVolume is one volume a migration target made, kept so an abort can
// remove exactly what the migration created and nothing else.
type migratedVolume struct {
	Backend string
	PoolID  string
	Target  string
	Path    string
}

// migrationTarget is one prepared receiving half.
type migrationTarget struct {
	VMID    string
	Token   string
	Volumes []migratedVolume
	// Started is set once a live target's QEMU is up, so an abort knows it has a
	// guest to stop as well as volumes to remove.
	Started bool
	TLSDir  string
}

// migrationTargets is the set of prepared targets on this host.
type migrationTargets struct {
	mu   sync.Mutex
	byID map[string]*migrationTarget
}

// newMigrationTargets returns an empty set.
func newMigrationTargets() *migrationTargets {
	return &migrationTargets{byID: make(map[string]*migrationTarget)}
}

// add records a prepared target under its migration id.
func (m *migrationTargets) add(migrationID string, target *migrationTarget) error {
	if err := validateGuestID(migrationID); err != nil {
		return fmt.Errorf("compute: invalid migration id: %w", err)
	}
	if len(target.Token) < 32 {
		return errors.New("compute: a migration token must be at least 32 characters")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.byID[migrationID]; exists {
		return fmt.Errorf("compute: migration %s is already prepared on this host", migrationID)
	}
	if len(m.byID) >= maxMigrationTargets {
		return errors.New("compute: too many migrations are in flight on this host")
	}
	m.byID[migrationID] = target
	return nil
}

// take removes and returns a prepared target, but only for the token it was
// prepared with. Compared in constant time: the token is the only thing that
// ties a finish or an abort to the McHarbor that prepared it.
func (m *migrationTargets) take(migrationID, token string) (*migrationTarget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	target, ok := m.byID[migrationID]
	if !ok || token == "" || subtle.ConstantTimeCompare([]byte(target.Token), []byte(token)) != 1 {
		return nil, errMigrationUnknown
	}
	delete(m.byID, migrationID)
	return target, nil
}

// validateMigrationPeer checks a live target address before it reaches a QMP
// URI, where a stray character would change what QEMU dials.
func validateMigrationPeer(host string, ports ...int) error {
	if host == "" {
		return errors.New("compute: a live migration needs the target's address")
	}
	if net.ParseIP(host) == nil {
		if len(host) > 253 {
			return fmt.Errorf("compute: %q is not a host name", host)
		}
		for _, r := range host {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '.' {
				return fmt.Errorf("compute: %q is not a host name", host)
			}
		}
		if strings.HasPrefix(host, "-") || strings.HasPrefix(host, ".") {
			return fmt.Errorf("compute: %q is not a host name", host)
		}
	}
	for _, port := range ports {
		if port <= 0 || port > 65535 {
			return fmt.Errorf("compute: %d is not a port", port)
		}
	}
	return nil
}

// migrationURI renders the tcp: URI QEMU dials, bracketing an IPv6 address.
func migrationURI(host string, port int) string {
	return "tcp:" + net.JoinHostPort(host, fmt.Sprint(port))
}

// sourceDiskFor finds the source guest's disk a migration entry names, so the
// agent only ever sends a file the guest's own spec says it owns.
func sourceDiskFor(spec *VMSpec, disk VMMigrateDisk) (VMDiskSpec, error) {
	if spec == nil {
		return VMDiskSpec{}, errors.New("compute: a migration send needs the guest's spec")
	}
	for _, candidate := range spec.Disks {
		if candidate.Slot == disk.Slot && !candidate.IsCDROM {
			if candidate.VolumePath == "" || candidate.VolumePath != disk.SourcePath {
				break
			}
			return candidate, nil
		}
	}
	return VMDiskSpec{}, fmt.Errorf("compute: slot %d is not a disk of this guest at %s", disk.Slot, disk.SourcePath)
}
