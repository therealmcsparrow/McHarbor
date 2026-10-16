// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"context"
	"errors"
	"strings"
)

// GuestFilesystem is one mounted filesystem as the guest reports it. Untrusted:
// the mount point is sanitized and the numbers are only compared, never trusted.
type GuestFilesystem struct {
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint"`
	Type       string `json:"type"`
	UsedBytes  *int64 `json:"used-bytes,omitempty"`
	TotalBytes *int64 `json:"total-bytes,omitempty"`
}

// GuestLoad is the guest's own load average (qemu-ga 9.1+, Linux guests only).
type GuestLoad struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// pseudoFilesystems never fill in a way an operator can act on, and reporting
// them would drown the one real disk that is full.
var pseudoFilesystems = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "overlay": true, "squashfs": true, "proc": true, "sysfs": true,
	"cgroup": true, "cgroup2": true, "devpts": true, "mqueue": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "pstore": true, "bpf": true, "autofs": true, "hugetlbfs": true, "fusectl": true,
	"configfs": true, "efivarfs": true, "ramfs": true, "nsfs": true, "binfmt_misc": true, "iso9660": true,
}

// FsInfo asks the guest for its mounted filesystems with usage, keeping only real
// ones that report a size; older guest agents omit the sizes and yield nothing.
func (g *GuestAgent) FsInfo(ctx context.Context) ([]GuestFilesystem, error) {
	var raw []GuestFilesystem
	if err := g.run(ctx, "guest-get-fsinfo", nil, &raw); err != nil {
		return nil, err
	}
	return UsableFilesystems(raw), nil
}

// UsableFilesystems filters and sanitizes a guest-fsinfo reply: pseudo and
// sizeless filesystems are dropped, one entry per mount point, at most 64.
func UsableFilesystems(raw []GuestFilesystem) []GuestFilesystem {
	seen := map[string]bool{}
	out := []GuestFilesystem{}
	for _, fs := range raw {
		if pseudoFilesystems[strings.ToLower(fs.Type)] || fs.TotalBytes == nil || fs.UsedBytes == nil || *fs.TotalBytes <= 0 || *fs.UsedBytes < 0 {
			continue
		}
		mount := sanitizeGuestString(fs.Mountpoint, 256)
		if mount == "" || seen[mount] {
			continue
		}
		seen[mount] = true
		used, total := *fs.UsedBytes, *fs.TotalBytes
		if used > total {
			used = total
		}
		out = append(out, GuestFilesystem{Name: sanitizeGuestString(fs.Name, 128), Mountpoint: mount,
			Type: sanitizeGuestString(fs.Type, 32), UsedBytes: &used, TotalBytes: &total})
		if len(out) == 64 {
			break
		}
	}
	return out
}

// Load asks the guest for its load average.
func (g *GuestAgent) Load(ctx context.Context) (GuestLoad, error) {
	var out GuestLoad
	err := g.run(ctx, "guest-get-load", nil, &out)
	return out, err
}

// Time asks the guest for its clock, in nanoseconds since the epoch; comparing it
// with the host's catches a guest whose clock has drifted.
func (g *GuestAgent) Time(ctx context.Context) (int64, error) {
	var ns int64
	err := g.run(ctx, "guest-get-time", nil, &ns)
	return ns, err
}

// IsUnsupportedCommand reports whether err is the guest agent saying it does not
// implement or has disabled a command, which means an old or locked-down agent
// rather than a fault. The class is matched; the description only for "disabled",
// which qemu-ga reports as a GenericError.
func IsUnsupportedCommand(err error) bool {
	var qe *Error
	if !errors.As(err, &qe) {
		return false
	}
	return qe.Class == "CommandNotFound" || strings.Contains(strings.ToLower(qe.Desc), "disabled")
}
