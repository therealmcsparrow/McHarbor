// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

// ErrSelfUpdateNotLocal is returned when the McHarbor container cannot be
// reached via the local Docker socket (e.g. only remote agent environments
// are registered), so the detached helper cannot be scheduled.
var ErrSelfUpdateNotLocal = errors.New("mcharbor self-update requires a local Docker socket")

// LocalSelfUpdateResult describes the scheduled self-update.
type LocalSelfUpdateResult struct {
	ContainerID   string
	ContainerName string
	TargetImage   string
	Output        string
}

// ApplyLocalSelfUpdate schedules a McHarbor self-update by spawning the
// detached helper container (see ScheduleDetachedSelfUpdateHelperForImage).
// The helper stops and recreates the current McHarbor container with the
// requested image, or the current image when targetImage is empty. It is
// shared by the manual "Update now" route and the automatic installer.
func ApplyLocalSelfUpdate(ctx context.Context, targetImage string) (LocalSelfUpdateResult, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return LocalSelfUpdateResult{}, fmt.Errorf("creating local docker client: %w", err)
	}
	defer closeQuietly(cli, "local docker client")

	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	current, err := CurrentMcHarborContainer(inspectCtx, cli)
	if err != nil {
		return LocalSelfUpdateResult{}, ErrSelfUpdateNotLocal
	}

	name := strings.TrimPrefix(current.Name, "/")

	target := strings.TrimSpace(targetImage)
	if target == "" {
		target = current.Config.Image
	}

	dataDir := os.Getenv("DATA_DIR")
	dockerHost := os.Getenv("DOCKER_HOST")

	scheduleCtx, cancelSchedule := context.WithTimeout(ctx, CallTimeout)
	defer cancelSchedule()

	output, err := ScheduleDetachedSelfUpdateHelperForImage(
		scheduleCtx, cli, current, dataDir, dockerHost, "update", target,
	)
	if err != nil {
		return LocalSelfUpdateResult{}, fmt.Errorf("scheduling self-update helper: %w", err)
	}

	return LocalSelfUpdateResult{
		ContainerID:   current.ID,
		ContainerName: name,
		TargetImage:   target,
		Output:        output,
	}, nil
}

// CurrentMcHarborImage returns the image reference the running McHarbor
// container was created from, or "" when the container is not reachable
// over the local socket.
func CurrentMcHarborImage(ctx context.Context) string {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return ""
	}
	defer closeQuietly(cli, "local docker client")
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	current, err := CurrentMcHarborContainer(inspectCtx, cli)
	if err != nil || current.Config == nil {
		return ""
	}
	return current.Config.Image
}

var semverTagPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+([-.][0-9A-Za-z.-]+)?$`)

// ImageForVersion returns the image reference to pull for `version`, derived
// from the reference the container currently runs. A pinned semver tag
// (`repo:2.0.8` or `repo:v2.0.8`) is swapped for the new version, keeping
// the `v` prefix style; a floating tag such as `latest` or a digest-pinned
// reference is returned unchanged so a re-pull refreshes it.
func ImageForVersion(currentImage, version string) string {
	currentImage = strings.TrimSpace(currentImage)
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if currentImage == "" || version == "" {
		return currentImage
	}
	if strings.Contains(currentImage, "@") {
		return currentImage
	}
	slash := strings.LastIndex(currentImage, "/")
	colon := strings.LastIndex(currentImage, ":")
	if colon <= slash {
		return currentImage
	}
	tag := currentImage[colon+1:]
	if !semverTagPattern.MatchString(tag) {
		return currentImage
	}
	prefix := ""
	if strings.HasPrefix(tag, "v") {
		prefix = "v"
	}
	return currentImage[:colon+1] + prefix + version
}
