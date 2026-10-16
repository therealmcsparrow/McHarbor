// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// errPathInUse refuses a pool delete while a process still has something under
// the path open. A running guest keeps a deleted disk alive only until it exits,
// so the delete would look harmless and destroy the disk later.
var errPathInUse = errors.New("storage: in use by a running process")

var guestUnitPattern = regexp.MustCompile(`mcharbor-vm-([a-z0-9]+)\.service`)

// procReader is the slice of /proc the in-use check reads, behind an interface
// so the matching logic is tested without a Linux process table.
type procReader interface {
	PIDs() ([]int, error)
	FDTargets(pid int) ([]string, error)
	Cgroup(pid int) (string, error)
	Comm(pid int) (string, error)
}

// openHolder is one process with a file open at or under a delete target;
// VMID is set when the process belongs to a McHarbor guest unit.
type openHolder struct {
	PID     int
	VMID    string
	Command string
}

// holdersUnder lists every process with a file open at target or beneath it.
// Processes that exit or refuse inspection mid-scan are skipped, not fatal.
func holdersUnder(proc procReader, target string) ([]openHolder, error) {
	target = path.Clean(target)
	pids, err := proc.PIDs()
	if err != nil {
		return nil, fmt.Errorf("storage: list processes: %w", err)
	}
	sort.Ints(pids)
	var out []openHolder
	for _, pid := range pids {
		links, err := proc.FDTargets(pid)
		if err != nil || !anyWithin(links, target) {
			continue
		}
		holder := openHolder{PID: pid}
		if cgroup, err := proc.Cgroup(pid); err == nil {
			if m := guestUnitPattern.FindStringSubmatch(cgroup); m != nil {
				holder.VMID = m[1]
			}
		}
		if comm, err := proc.Comm(pid); err == nil {
			holder.Command = strings.TrimSpace(comm)
		}
		out = append(out, holder)
	}
	return out, nil
}

func anyWithin(links []string, target string) bool {
	for _, link := range links {
		link = strings.TrimSuffix(link, " (deleted)")
		if link == target || strings.HasPrefix(link, target+"/") {
			return true
		}
	}
	return false
}

// inUseError names what is holding rel open, guests first, so the operator
// learns which VM to stop rather than which pid.
func inUseError(rel string, holders []openHolder) error {
	names := make([]string, 0, len(holders))
	for _, h := range holders {
		switch {
		case h.VMID != "":
			names = append(names, "running guest "+h.VMID)
		case h.Command != "":
			names = append(names, fmt.Sprintf("%s (pid %d)", h.Command, h.PID))
		default:
			names = append(names, fmt.Sprintf("pid %d", h.PID))
		}
	}
	return fmt.Errorf("%w: %s is open by %s; stop it before deleting", errPathInUse, rel, strings.Join(names, ", "))
}
