// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package debugshell places a static busybox inside a shell-less container so an
// operator can debug a distroless image, and guarantees the binary is removed again.
package debugshell

import (
	"errors"
	"time"
)

// InstallDir is the one directory McHarbor creates inside the container; the
// root always exists, where /tmp in a distroless image frequently does not.
const InstallDir = "/.mcharbor-debug"

// InstallPath is the uploaded binary. Applets run as `busybox <applet>`, so the
// container gains exactly this one path and no symlinks.
const InstallPath = InstallDir + "/busybox"

// SessionTTL bounds how long a busybox may stay in a container without a
// cleanup; the reaper removes anything older whether or not a shell is open.
const SessionTTL = 15 * time.Minute

// ShellPATH is the PATH the debug shell starts with: the busybox directory first,
// then the usual places, so binaries the image does carry still resolve.
const ShellPATH = "PATH=" + InstallDir + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// Trigger names which of the four independent cleanup points removed a session.
type Trigger string

// The four cleanup triggers of design 02d §5, recorded on every removal row.
const (
	TriggerSessionEnd Trigger = "session_end"
	TriggerExpiry     Trigger = "expiry"
	TriggerReaper     Trigger = "reaper"
	TriggerStartup    Trigger = "startup"
)

// Session is one row of container_debug_sessions: an install (or a reuse of a
// live one) that a single shell may attach to until it is removed or expires.
type Session struct {
	ID             string     `json:"sessionId"`
	EnvironmentID  string     `json:"environmentId"`
	ContainerID    string     `json:"containerId"`
	ContainerName  string     `json:"containerName"`
	UserID         string     `json:"userId"`
	APIKeyID       string     `json:"apiKeyId,omitempty"`
	Path           string     `json:"path"`
	Arch           string     `json:"arch"`
	BusyboxDigest  string     `json:"busyboxDigest"`
	SizeBytes      int64      `json:"sizeBytes"`
	Uploaded       bool       `json:"installed"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	RemovedAt      *time.Time `json:"removedAt,omitempty"`
	RemovalTrigger string     `json:"removalTrigger,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// InstallRequest identifies who asked for a debug shell in which container.
type InstallRequest struct {
	EnvironmentID string
	ContainerID   string
	UserID        string
	APIKeyID      string
}

// SessionStatus is what GET /containers/{id}/debug-shell reports: every session
// still open, and every one whose cleanup failed, so a leftover busybox is visible.
type SessionStatus struct {
	Sessions   []Session `json:"sessions"`
	LeftBehind bool      `json:"leftBehind"`
}

// RemoveResult reports how many sessions a DELETE closed.
type RemoveResult struct {
	Removed int `json:"removed"`
}

// Refusals the handler maps onto their own i18n codes and HTTP statuses.
var (
	ErrHasShell          = errors.New("debugshell: container already has a shell")
	ErrReadonlyRootfs    = errors.New("debugshell: container root filesystem is read-only")
	ErrNotRunning        = errors.New("debugshell: container is not running")
	ErrUnsupportedArch   = errors.New("debugshell: no bundled binary for the daemon architecture")
	ErrBinaryUnavailable = errors.New("debugshell: bundled busybox not found")
	ErrProtected         = errors.New("debugshell: container is protected")
	ErrSessionInvalid    = errors.New("debugshell: session missing, expired, removed or not yours")
)
