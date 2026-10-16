// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Image operation bounds. A pull or save moves an image of a hundred megabytes
// or more through the registry and the daemon, which the usual 30-second Docker
// bound cannot cover; each is still finite so a wedged daemon frees the pull.
const (
	imagePullTimeout = 15 * time.Minute
	imageSaveTimeout = 10 * time.Minute
	imageOpTimeout   = 30 * time.Second
)

// ImageStore is the slice of a Docker daemon the store needs to turn a
// registry image into a tarball. Tests supply a fake; production uses the
// daemon McHarbor itself runs on.
type ImageStore interface {
	Pull(ctx context.Context, ref, platform string) error
	Tag(ctx context.Context, source, target string) error
	Save(ctx context.Context, ref, platform string, w io.Writer) error
	Remove(ctx context.Context, ref string) error
}

// dockerImageStore is ImageStore over the local Docker socket.
type dockerImageStore struct {
	cli *client.Client
}

// NewDockerImageStore connects to the Docker daemon McHarbor runs on, the same
// way the self-update does. It never dials a remote environment: agents
// receive the saved tarball, they never pull.
func NewDockerImageStore() (ImageStore, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("connecting to the local Docker daemon: %w", err)
	}
	return &dockerImageStore{cli: cli}, nil
}

// Pull pulls ref for one platform and drains the progress stream, because the
// daemon reports a failed pull inside the stream rather than as a status.
func (d *dockerImageStore) Pull(ctx context.Context, ref, platform string) error {
	ctx, cancel := context.WithTimeout(ctx, imagePullTimeout)
	defer cancel()
	rc, err := d.cli.ImagePull(ctx, ref, client.ImagePullOptions{Platforms: pullPlatforms(platform)})
	if err != nil {
		return fmt.Errorf("pulling %s (%s): %w", ref, platform, err)
	}
	defer func() {
		if err := rc.Close(); err != nil {
			slog.Warn("closing image stream failed", "error", err)
		}
	}()
	dec := json.NewDecoder(rc)
	for {
		var line struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("reading pull progress for %s: %w", ref, err)
		}
		if line.Error != "" {
			return fmt.Errorf("pulling %s (%s): %s", ref, platform, line.Error)
		}
	}
}

// Tag names source as target on the local daemon.
func (d *dockerImageStore) Tag(ctx context.Context, source, target string) error {
	ctx, cancel := context.WithTimeout(ctx, imageOpTimeout)
	defer cancel()
	if _, err := d.cli.ImageTag(ctx, client.ImageTagOptions{Source: source, Target: target}); err != nil {
		return fmt.Errorf("tagging %s as %s: %w", source, target, err)
	}
	return nil
}

// Save writes `docker save ref` to w. The reference was tagged from a pull of
// exactly one platform, so the tarball carries that platform's image.
func (d *dockerImageStore) Save(ctx context.Context, ref, _ string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, imageSaveTimeout)
	defer cancel()
	rc, err := d.cli.ImageSave(ctx, []string{ref})
	if err != nil {
		return fmt.Errorf("saving %s: %w", ref, err)
	}
	defer func() {
		if err := rc.Close(); err != nil {
			slog.Warn("closing image stream failed", "error", err)
		}
	}()
	if _, err := io.Copy(w, rc); err != nil {
		return fmt.Errorf("writing saved image %s: %w", ref, err)
	}
	return nil
}

// Remove drops a reference McHarbor created only to save it.
func (d *dockerImageStore) Remove(ctx context.Context, ref string) error {
	ctx, cancel := context.WithTimeout(ctx, imageOpTimeout)
	defer cancel()
	if _, err := d.cli.ImageRemove(ctx, ref, client.ImageRemoveOptions{PruneChildren: true}); err != nil {
		return fmt.Errorf("removing %s: %w", ref, err)
	}
	return nil
}

// LocalImageRef is the reference a stored tarball carries and an agent starts.
func LocalImageRef(version string) string {
	return LocalImageName + ":" + version
}

// saveImages turns the release's image into one tarball per configured
// platform inside dir and returns the assets. The source is the release's
// signed, digest-pinned image.txt when it has one, else repository:version.
func (s *Store) saveImages(ctx context.Context, dir, version, pinnedRef, repository string, platforms []string) ([]Asset, error) {
	if s.images == nil {
		return nil, errors.New("no local Docker daemon is available to pull the agent image")
	}
	source := pinnedRef
	if source == "" {
		source = repository + ":" + version
	}
	local := LocalImageRef(version)
	// The tag is shared across platforms, so saves run one at a time.
	s.imageMu.Lock()
	defer s.imageMu.Unlock()

	var assets []Asset
	for _, platform := range platforms {
		osName, arch, _ := strings.Cut(platform, "/")
		asset, err := s.saveOneImage(ctx, dir, source, local, platform, osName, arch)
		if err != nil {
			return assets, err
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func (s *Store) saveOneImage(ctx context.Context, dir, source, local, platform, osName, arch string) (Asset, error) {
	if err := s.images.Pull(ctx, source, platform); err != nil {
		return Asset{}, err
	}
	defer func() {
		for _, ref := range []string{local, source} {
			if err := s.images.Remove(context.WithoutCancel(ctx), ref); err != nil {
				s.logger.Debug("agent-releases: leaving image reference behind", "ref", ref, "error", err)
			}
		}
	}()
	if err := s.images.Tag(ctx, source, local); err != nil {
		return Asset{}, err
	}
	name := ImageAssetName(osName, arch)
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return Asset{}, fmt.Errorf("creating %s: %w", path, err)
	}
	h := sha256.New()
	counter := &countingWriter{}
	saveErr := s.images.Save(ctx, local, platform, io.MultiWriter(f, h, counter))
	if closeErr := f.Close(); saveErr == nil {
		saveErr = closeErr
	}
	if saveErr == nil && counter.n == 0 {
		saveErr = fmt.Errorf("the daemon saved an empty image for %s", platform)
	}
	if saveErr != nil {
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			s.logger.Warn("agent-releases: removing failed image save", "path", path, "error", rmErr)
		}
		return Asset{}, saveErr
	}
	return Asset{Name: name, Kind: AssetImage, OS: osName, Arch: arch, SHA256: hex.EncodeToString(h.Sum(nil)), Size: counter.n}, nil
}

// countingWriter counts bytes, so the recorded size comes from what was written.
type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// pullPlatforms turns an "os/arch[/variant]" string into the platform list the
// client sends as the pull's platform query; an empty string pulls the daemon's default.
func pullPlatforms(platform string) []ocispec.Platform {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
	}
	parts := strings.SplitN(platform, "/", 3)
	p := ocispec.Platform{OS: parts[0]}
	if len(parts) > 1 {
		p.Architecture = parts[1]
	}
	if len(parts) > 2 {
		p.Variant = parts[2]
	}
	return []ocispec.Platform{p}
}
