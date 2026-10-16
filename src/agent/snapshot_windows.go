// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// snapshotDetect reports the one driver a Windows host can offer.
// The shape matches the Linux detect reply so the server and the
// settings UI never branch on the agent's platform.
func snapshotDetect(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	probe := payload
	probe.Driver = "vss"
	capability, err := snapshotCapability(ctx, probe)
	if err != nil || capability == nil {
		message := "probe failed"
		if err != nil {
			message = err.Error()
		}
		return &SnapshotPayload{
			Operation: "detect",
			Success:   true,
			Detected:  []SnapshotDriverOut{{Driver: "vss", Available: false, Error: message}},
		}, nil
	}
	return &SnapshotPayload{
		Operation: "detect",
		Success:   true,
		Detected: []SnapshotDriverOut{{
			Driver:    "vss",
			ToolPath:  capability.ToolPath,
			Version:   capability.Version,
			Available: capability.Available,
			Error:     capability.Error,
			Targets:   capability.TargetsOut,
		}},
	}, nil
}

// snapshotCapability probes Windows for the Volume Shadow Copy
// Service. When vssadmin is on PATH we return one target per shadow
// copy volume; admins use the volume to map a container's bind mount
// to a snapshot unit.
func snapshotCapability(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	_ = payload
	toolPath, ok := which("vssadmin")
	if !ok {
		return &SnapshotPayload{
			Driver:    "vss",
			Available: false,
			Error:     "vssadmin is not on PATH (Windows Server or Desktop only)",
		}, nil
	}
	out, err := runCmd(ctx, toolPath, "list", "shadows")
	if err != nil {
		return &SnapshotPayload{ //nolint:nilerr // the failure travels to the server inside the payload Error field by protocol
			Driver:    "vss",
			ToolPath:  toolPath,
			Available: false,
			Error:     "vssadmin list shadows failed: " + err.Error(),
		}, nil
	}
	targets := parseVssShadows(string(out))
	return &SnapshotPayload{
		Driver:     "vss",
		ToolPath:   toolPath,
		Available:  len(targets) > 0,
		TargetsOut: targets,
	}, nil
}

// snapshotCreate runs wbadmin against the given volumes. The created
// shadow id is scraped from stdout and returned as SnapshotName so
// Remove can target it. MountPath stays empty: wbadmin does not expose
// the shadow at a stable path, so the server keeps pausing the
// container on Windows and treats the shadow as a restore point.
func snapshotCreate(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if len(payload.Targets) == 0 {
		return &SnapshotPayload{Driver: "vss", Success: false, Error: "no targets"}, nil
	}
	toolPath, ok := which("wbadmin")
	if !ok {
		return &SnapshotPayload{Driver: "vss", Success: false, Error: "wbadmin is not on PATH"}, nil
	}
	out, err := runCmd(ctx, toolPath, "start", "backup", "-include:"+strings.Join(payload.Targets, ","))
	if err != nil {
		return &SnapshotPayload{ //nolint:nilerr // the failure travels to the server inside the payload Error field by protocol
			Driver:  "vss",
			Success: false,
			Error:   "wbadmin failed: " + err.Error() + ": " + strings.TrimSpace(string(out)),
		}, nil
	}
	return &SnapshotPayload{
		Driver:       "vss",
		Success:      true,
		SnapshotName: parseWbadminShadowID(string(out)),
		Targets:      payload.Targets,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// snapshotRemove deletes a shadow copy by id.
func snapshotRemove(ctx context.Context, payload SnapshotPayload) (*SnapshotPayload, error) {
	if payload.SnapshotName == "" {
		return &SnapshotPayload{Driver: "vss", Success: false, Error: "shadow id required"}, nil
	}
	toolPath, ok := which("vssadmin")
	if !ok {
		return &SnapshotPayload{Driver: "vss", Success: false, Error: "vssadmin is not on PATH"}, nil
	}
	out, err := runCmd(ctx, toolPath, "delete", "shadows", "/shadow="+payload.SnapshotName, "/quiet")
	if err != nil {
		return &SnapshotPayload{ //nolint:nilerr // the failure travels to the server inside the payload Error field by protocol
			Driver:  "vss",
			Success: false,
			Error:   "vssadmin delete failed: " + err.Error() + ": " + strings.TrimSpace(string(out)),
		}, nil
	}
	return &SnapshotPayload{Driver: "vss", Success: true}, nil
}

// parseVssShadows extracts volumes from `vssadmin list shadows`. The
// output is verbose, so we key on the "Shadow Copy Volume:" lines.
func parseVssShadows(out string) []SnapshotTargetOut {
	seen := map[string]bool{}
	var rows []SnapshotTargetOut
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Shadow Copy Volume:") {
			continue
		}
		volume := strings.TrimSpace(strings.TrimPrefix(line, "Shadow Copy Volume:"))
		if volume == "" || seen[volume] {
			continue
		}
		seen[volume] = true
		rows = append(rows, SnapshotTargetOut{Name: volume, Mountpoint: volume, Driver: "vss"})
	}
	return rows
}

// parseWbadminShadowID scrapes a shadow id out of `wbadmin start
// backup` stdout, which reports "Shadow copy ID: {<guid>}".
func parseWbadminShadowID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		start := strings.Index(line, "{")
		if start < 0 {
			continue
		}
		end := strings.Index(line[start:], "}")
		if end < 0 {
			continue
		}
		return line[start : start+end+1]
	}
	return ""
}

// which looks up a CLI tool on PATH. A missing tool means an
// unavailable driver, not an error worth propagating.
func which(name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

// runCmd runs a CLI tool and captures combined output. The context
// carries the caller's deadline so a wedged tool cannot hang the
// agent's read loop.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("empty command path")
	}
	// #nosec G204 -- name is resolved through exec.LookPath in `which`,
	// and args are built from driver-native identifiers, never from raw
	// user input.
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
