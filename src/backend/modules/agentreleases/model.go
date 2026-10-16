// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package agentreleases keeps McHarbor's own store of remote-agent releases:
// it pulls them from GitHub or an operator's mirror, verifies them against a
// pinned signing key, and is the only place an agent install or update reads from.
package agentreleases

import "errors"

// Release status values. A release is served only while it is StatusReady.
const (
	StatusPulling = "pulling"
	StatusReady   = "ready"
	StatusFailed  = "failed"
)

// Signature status values recorded per release.
const (
	SignatureVerified = "verified"
	SignatureRefused  = "refused"
)

// Pull modes, mirroring the McHarbor self-update's notify / install choice.
const (
	ModeNotify   = "notify"
	ModeDownload = "download"
	ModeRollout  = "rollout"
)

// Source kinds. A github source reads a GitHub repository's release assets; a
// mirror source reads the plain directory layout documented for operators.
const (
	SourceGitHub = "github"
	SourceMirror = "mirror"
)

// OfficialSourceURL is the mcharbor.eu agent catalog index the default source reads. It
// is the only address on the internet McHarbor contacts for agents unless an operator adds a mirror.
const OfficialSourceURL = "https://www.mcharbor.eu/catalog/agents/index.json"

// DefaultImageRepository is where McHarbor pulls agent images from when a
// release does not name a digest-pinned image of its own.
const DefaultImageRepository = "ghcr.io/therealmcsparrow/mcharbor-agent"

// LocalImageName is the repository an image tarball is saved under, so an
// agent that loads it knows the reference to start without asking a registry.
const LocalImageName = "mcharbor-agent"

// Asset kinds.
const (
	AssetBinary = "binary"
	AssetImage  = "image"
)

// Asset is one verified file of a stored release.
type Asset struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Release is one row of agent_releases as the API returns it.
type Release struct {
	ID              string  `json:"id"`
	Version         string  `json:"version"`
	Build           string  `json:"build"`
	SourceKind      string  `json:"sourceKind"`
	SourceURL       string  `json:"sourceUrl"`
	Status          string  `json:"status"`
	SignatureStatus string  `json:"signatureStatus"`
	SignatureKey    string  `json:"signatureKey"`
	Assets          []Asset `json:"assets"`
	SizeBytes       int64   `json:"sizeBytes"`
	ImageRef        string  `json:"imageRef"`
	ImageError      string  `json:"imageError"`
	LastError       string  `json:"lastError"`
	PulledAt        string  `json:"pulledAt"`
	PulledBy        string  `json:"pulledBy"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	// IsTarget and InUse are computed, not stored: they are what makes the
	// delete action refuse, and the table shows them for the same reason.
	IsTarget bool `json:"isTarget"`
	InUse    bool `json:"inUse"`
	// Pinned marks a build the operator chose over a newer one of the same
	// version; checks leave it in place until another build is pulled.
	Pinned bool `json:"pinned,omitempty"`
}

// Source is one place releases are pulled from, tried in list order.
type Source struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// Settings is the operator-facing configuration under Settings → Updates → Agents.
type Settings struct {
	Enabled         bool     `json:"enabled"`
	Mode            string   `json:"mode"`
	CheckCron       string   `json:"checkCron"`
	Keep            int      `json:"keep"`
	Sources         []Source `json:"sources"`
	ImageRepository string   `json:"imageRepository"`
	ImagePlatforms  []string `json:"imagePlatforms"`
}

// State is the cached outcome of the most recent release check.
type State struct {
	LatestVersion   string `json:"latestVersion,omitempty"`
	LatestBuild     string `json:"latestBuild,omitempty"`
	LatestSource    string `json:"latestSource,omitempty"`
	LastCheckedAt   string `json:"lastCheckedAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	NextCheckAt     string `json:"nextCheckAt,omitempty"`
	NotifiedVersion string `json:"notifiedVersion,omitempty"`
	NotifiedBuild   string `json:"notifiedBuild,omitempty"`
	// Available is every version the answering source said it carries, newest
	// first, so an operator can pull an older one by path.
	Available []AvailableVersion `json:"available,omitempty"`
}

// AvailableVersion is one version a source's index lists. It is a hint like the
// latest version: nothing is trusted until that version's SHA256SUMS verifies.
type AvailableVersion struct {
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
}

// Overview is what GET /updates/agents/releases returns.
type Overview struct {
	Releases []Release `json:"releases"`
	Target   string    `json:"target"`
	// TargetBuild is the build of Target agents are moved to; several builds of
	// one version can be stored at once.
	TargetBuild string   `json:"targetBuild,omitempty"`
	State       State    `json:"state"`
	Settings    Settings `json:"settings"`
}

// SetTargetRequest names the release agents should move to; empty clears it.
// Build picks one of several stored builds; empty means the newest ready one.
type SetTargetRequest struct {
	Version string `json:"version"`
	Build   string `json:"build,omitempty"`
}

// Sentinel errors the handler maps to i18n codes.
var (
	ErrSignatureMissing  = errors.New("release has no SHA256SUMS.sig")
	ErrSignatureInvalid  = errors.New("SHA256SUMS signature does not verify against any pinned agent signing key")
	ErrChecksumMismatch  = errors.New("asset checksum does not match SHA256SUMS")
	ErrSumsMalformed     = errors.New("SHA256SUMS is malformed")
	ErrNoAssets          = errors.New("SHA256SUMS lists no agent binaries")
	ErrReleaseNotFound   = errors.New("agent release not found")
	ErrReleaseNotReady   = errors.New("agent release is not ready")
	ErrReleaseIsTarget   = errors.New("agent release is the current target")
	ErrReleaseInUse      = errors.New("agent release is running on a connected agent")
	ErrReleaseBusy       = errors.New("agent release is already being pulled")
	ErrInvalidVersion    = errors.New("invalid agent version")
	ErrInvalidSettings   = errors.New("invalid agent release settings")
	ErrNoSourceAvailable = errors.New("no release source could provide this version")
	// ErrInvalidBuild is a requested build that is not a yyyyMMddHHmm stamp.
	ErrInvalidBuild = errors.New("invalid agent build")
	// ErrBuildMismatch is a source whose signed BUILD file names another build
	// than the one asked for, so the operator never stores a build they did not pick.
	ErrBuildMismatch = errors.New("source serves a different build than requested")
)

// SettingsError is a refused setting, named by a stable reason the interface
// translates rather than by prose the handler would have to echo.
type SettingsError struct {
	Reason string
}

// Error names the reason, which is what the server log needs; the client only sees the code.
func (e *SettingsError) Error() string { return "invalid agent release settings: " + e.Reason }

// Is makes every SettingsError match ErrInvalidSettings.
func (e *SettingsError) Is(target error) bool { return target == ErrInvalidSettings }

func settingsError(reason string) error { return &SettingsError{Reason: reason} }
