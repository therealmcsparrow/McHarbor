// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package versions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/nativeupdate"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// nativeApplyTimeout bounds downloading and verifying a native release package.
const nativeApplyTimeout = 15 * time.Minute

// errSelfUpdateNotLocal aliases the core error so the handler can keep
// matching it with errors.Is.
var errSelfUpdateNotLocal = coredocker.ErrSelfUpdateNotLocal

// Refusals SelfUpdate returns before it starts anything.
var (
	// ErrDowngradeRefused is a target that is older than a prerelease host, or one that does not name a version.
	ErrDowngradeRefused = errors.New("the update would move to an older or unnamed version")
	// ErrPrereleaseOff is a prerelease target while the McHarbor prerelease switch is off.
	ErrPrereleaseOff = errors.New("prerelease updates are switched off")
	// ErrDigestPinned is an image reference carrying a digest, on any host, while the McHarbor prerelease switch is off: Docker pulls the digest and ignores the tag, so the tag proves nothing.
	ErrDigestPinned = errors.New("an image pinned by digest names no version and prerelease updates are switched off")
)

// Service collects version information for the local instance and agents.
type Service struct {
	db        *sql.DB
	agentPool *coreagent.AgentPool
	// native installs a release package in place; nil on a container install.
	native *nativeupdate.Updater
	// current, applyNative and applyImage are seams for tests; nil means the real thing.
	current     func() string
	applyNative func(ctx context.Context, version string) (nativeupdate.Result, error)
	applyImage  func(ctx context.Context, image string) (SelfUpdateResult, error)
}

// SetNativeUpdater makes SelfUpdate install the release package on this host instead of replacing a container.
func (s *Service) SetNativeUpdater(u *nativeupdate.Updater) { s.native = u }

// NewService creates a version service.
func NewService(db *sql.DB, agentPool *coreagent.AgentPool) *Service {
	return &Service{db: db, agentPool: agentPool}
}

// Info returns the local McHarbor version plus all agent environment versions.
func (s *Service) Info(ctx context.Context) (*VersionInfo, error) {
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	return &VersionInfo{
		McHarbor: ComponentVersion{
			Name:    "mcharbor",
			Version: appversion.Current(),
		},
		Agents: agents,
	}, nil
}

// ListAgents returns version information for all agent-type environments.
func (s *Service) ListAgents(ctx context.Context) ([]AgentVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, agent_status, agent_hostname, agent_os, agent_arch,
		       agent_version, docker_version, agent_last_seen
		FROM environments
		WHERE connection_type = 'agent'
		ORDER BY name ASC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("querying agent versions: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	var agents []AgentVersion
	for rows.Next() {
		var a AgentVersion
		var status, hostname, os, arch, version, dockerVer, lastSeen sql.NullString
		if err := rows.Scan(&a.EnvID, &a.EnvName, &status, &hostname, &os, &arch, &version, &dockerVer, &lastSeen); err != nil {
			return nil, fmt.Errorf("scanning agent version: %w", err)
		}
		a.Status = "disconnected"
		if status.Valid {
			a.Status = status.String
		}
		if hostname.Valid {
			a.Hostname = hostname.String
		}
		if os.Valid {
			a.OS = os.String
		}
		if arch.Valid {
			a.Arch = arch.String
		}
		if version.Valid {
			a.AgentVersion = version.String
		}
		if dockerVer.Valid {
			a.DockerVersion = dockerVer.String
		}
		if lastSeen.Valid {
			a.LastSeen = lastSeen.String
		}
		s.applyLiveAgentMetadata(&a)
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []AgentVersion{}
	}
	return agents, rows.Err()
}

func (s *Service) applyLiveAgentMetadata(a *AgentVersion) {
	if a == nil || s.agentPool == nil {
		return
	}
	conn, ok := s.agentPool.Get(a.EnvID)
	if !ok {
		return
	}
	a.Status = "connected"
	a.Hostname = conn.Hostname
	a.OS = conn.OS
	a.Arch = conn.Arch
	a.AgentVersion = conn.Version
	a.DockerVersion = conn.DockerVer
}

// SelfUpdate schedules a McHarbor self-update. A native install installs version (empty reads the
// latest release, and only a stable host may pass that); a container install replaces itself with
// image. The target is checked first, so a refusal changes nothing: a prerelease host never moves to
// an older, floating or unparsable target, and a prerelease installs only with its switch on.
func (s *Service) SelfUpdate(ctx context.Context, image, version string) (SelfUpdateResult, error) {
	native := s.native != nil || s.applyNative != nil
	if err := s.checkTarget(ctx, native, image, version); err != nil {
		return SelfUpdateResult{}, err
	}
	if native {
		// The package download can outlast the request on a slow link; the install must not be
		// abandoned with it, and the page already waits for the restart if the response is lost.
		applyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nativeApplyTimeout)
		defer cancel()
		apply := s.applyNative
		if apply == nil {
			apply = s.native.Apply
		}
		res, err := apply(applyCtx, version)
		if err != nil {
			return SelfUpdateResult{}, err
		}
		return SelfUpdateResult{NativeVersion: res.Version, NativePackage: res.Asset, NativeUnit: res.Unit, NativeLog: res.LogPath}, nil
	}
	if s.applyImage != nil {
		return s.applyImage(ctx, image)
	}
	res, err := coredocker.ApplyLocalSelfUpdate(ctx, image)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	return SelfUpdateResult{
		ContainerID:   res.ContainerID,
		ContainerName: res.ContainerName,
		TargetImage:   res.TargetImage,
		Output:        res.Output,
	}, nil
}

// checkTarget decides from the request alone, before anything is touched. The target of a native
// install is version and of a container install the image's tag, never the other field, so a
// caller cannot pass a high version beside a low image.
func (s *Service) checkTarget(ctx context.Context, native bool, image, version string) error {
	target, pinned := version, false
	if !native {
		target, pinned = imageTag(image)
	}
	if versionorder.IsPrerelease(s.currentVersion()) {
		if _, err := versionorder.Parse(target); err != nil {
			return ErrDowngradeRefused
		}
		if versionorder.Compare(target, s.currentVersion()) < 0 {
			return ErrDowngradeRefused
		}
	}
	if !pinned && (carriesNoVersion(native, image, target) || versionorder.IsPlainStable(target)) {
		return nil
	}
	channels, err := updatechannel.Load(ctx, s.db)
	if err != nil {
		return fmt.Errorf("reading the update channel switches: %w", err)
	}
	switch {
	case versionorder.IsPrerelease(target):
		if !channels.Allows(updatechannel.PathMcHarbor, target) {
			return ErrPrereleaseOff
		}
		return nil
	case pinned:
		if !channels.McHarborPrereleases {
			return ErrDigestPinned
		}
		return nil
	case prereleaseShape.MatchString(target):
		// Shaped like a prerelease but spelled so the strict parser refuses it: never installed.
		if !channels.McHarborPrereleases {
			return ErrPrereleaseOff
		}
	}
	return nativeupdate.ErrInvalidVersion
}

// prereleaseShape matches a version with a "-" suffix in any spelling, including the ones
// Parse refuses (rc-1, rc_1, an uppercase V), so they are named a prerelease and not noise.
var prereleaseShape = regexp.MustCompile(`^[vV]?[0-9]+\.[0-9]+\.[0-9]+-`)

// carriesNoVersion is a target a stable host may take with the switch off because it names no
// version to compare: the native latest, an image reusing the current tag, or a floating tag
// that prerelease builds are never published under.
func carriesNoVersion(native bool, image, target string) bool {
	if native {
		return target == ""
	}
	return strings.TrimSpace(image) == "" || target == "" || target == "latest"
}

func (s *Service) currentVersion() string {
	if s.current != nil {
		return s.current()
	}
	return appversion.Current()
}

// imageTag returns the bare version an image reference names, with any "@digest" cut off first so
// a pinned prerelease tag is still read. pinned is true for any reference with a digest: Docker
// pulls by digest and ignores the tag, so the tag proves nothing about what installs.
func imageTag(image string) (tag string, pinned bool) {
	image = strings.TrimSpace(image)
	if at := strings.Index(image, "@"); at >= 0 {
		image, pinned = image[:at], true
	}
	colon := strings.LastIndex(image, ":")
	if colon <= strings.LastIndex(image, "/") {
		return "", pinned
	}
	return strings.TrimPrefix(image[colon+1:], "v"), pinned
}
