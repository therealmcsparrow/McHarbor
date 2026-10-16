// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"path/filepath"
	"strconv"
)

// linuxProc reads the live process table; the agent runs as root, so every
// process's descriptors are visible to it.
type linuxProc struct{ root string }

// PIDs lists the numeric entries of /proc.
func (p linuxProc) PIDs() ([]int, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(entries))
	for _, entry := range entries {
		if pid, err := strconv.Atoi(entry.Name()); err == nil {
			out = append(out, pid)
		}
	}
	return out, nil
}

// FDTargets resolves every open descriptor of pid to the path it names.
func (p linuxProc) FDTargets(pid int) ([]string, error) {
	dir := filepath.Join(p.root, strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if link, err := os.Readlink(filepath.Join(dir, entry.Name())); err == nil {
			out = append(out, link)
		}
	}
	return out, nil
}

// Cgroup returns pid's cgroup membership, which names its systemd unit.
func (p linuxProc) Cgroup(pid int) (string, error) {
	b, err := os.ReadFile(filepath.Join(p.root, strconv.Itoa(pid), "cgroup"))
	return string(b), err
}

// Comm returns pid's short command name.
func (p linuxProc) Comm(pid int) (string, error) {
	b, err := os.ReadFile(filepath.Join(p.root, strconv.Itoa(pid), "comm"))
	return string(b), err
}
