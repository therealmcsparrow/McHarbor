// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

func unusedImagePruneFilters() filters.Args {
	return filters.NewArgs(filters.Arg("dangling", "false"))
}

// Service wraps Docker SDK image operations.
type Service struct {
	pool *docker.ClientPool
}

// NewService creates a new image service.
func NewService(pool *docker.ClientPool) *Service {
	return &Service{pool: pool}
}

// getClient returns a Docker client for the given environment.
func (s *Service) getClient(envID string) (*client.Client, error) {
	c, err := s.pool.Get(envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

// List returns all images.
func (s *Service) List(ctx context.Context, envID string, all bool) ([]ImageSummary, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	images, err := cli.ImageList(ctx, image.ListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	result := make([]ImageSummary, 0, len(images))
	for _, img := range images {
		result = append(result, ImageSummary{
			ID:          img.ID,
			ParentID:    img.ParentID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     img.Created,
			Size:        img.Size,
			SharedSize:  img.SharedSize,
			Containers:  img.Containers,
			Labels:      img.Labels,
		})
	}

	return result, nil
}

// Inspect returns detailed image information.
func (s *Service) Inspect(ctx context.Context, envID, id string) (types.ImageInspect, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return types.ImageInspect{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	info, _, err := cli.ImageInspectWithRaw(ctx, id)
	if err != nil {
		return types.ImageInspect{}, fmt.Errorf("inspecting image %s: %w", id, err)
	}

	return info, nil
}

// Pull pulls an image from a registry.
func (s *Service) Pull(ctx context.Context, envID string, req PullRequest) (string, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ref := req.Image
	if req.Tag != "" {
		ref += ":" + req.Tag
	} else if ref != "" && !hasTag(ref) {
		ref += ":latest"
	}

	reader, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("pulling image %s: %w", ref, err)
	}
	defer reader.Close()

	// Consume the pull output to completion
	output, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading pull response: %w", err)
	}

	return string(output), nil
}

// Remove removes an image.
func (s *Service) Remove(ctx context.Context, envID, id string, force, noPrune bool) ([]image.DeleteResponse, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := cli.ImageRemove(ctx, id, image.RemoveOptions{
		Force:         force,
		PruneChildren: !noPrune,
	})
	if err != nil {
		return nil, fmt.Errorf("removing image %s: %w", id, err)
	}

	return resp, nil
}

// Tag tags an image with a new repository and tag.
func (s *Service) Tag(ctx context.Context, envID, id string, req TagRequest) error {
	cli, err := s.getClient(envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ref := req.Repo
	if req.Tag != "" {
		ref += ":" + req.Tag
	}

	if err := cli.ImageTag(ctx, id, ref); err != nil {
		return fmt.Errorf("tagging image %s as %s: %w", id, ref, err)
	}

	return nil
}

// History returns the history of an image.
func (s *Service) History(ctx context.Context, envID, id string) ([]image.HistoryResponseItem, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	history, err := cli.ImageHistory(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("fetching image history for %s: %w", id, err)
	}

	return history, nil
}

// Prune removes unused images.
func (s *Service) Prune(ctx context.Context, envID string) (image.PruneReport, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return image.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	images, err := cli.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return image.PruneReport{}, fmt.Errorf("listing images for prune: %w", err)
	}

	report := image.PruneReport{}
	for _, img := range images {
		if img.Containers != 0 {
			continue
		}
		responses, err := cli.ImageRemove(ctx, img.ID, image.RemoveOptions{PruneChildren: true})
		if err != nil {
			return image.PruneReport{}, fmt.Errorf("pruning image %s: %w", img.ID, err)
		}
		report.ImagesDeleted = append(report.ImagesDeleted, responses...)
		if img.Size > 0 {
			report.SpaceReclaimed += uint64(img.Size)
		}
	}
	return report, nil
}

// Export streams an image as a tar archive (docker save).
func (s *Service) Export(ctx context.Context, envID, id string) (io.ReadCloser, string, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return nil, "", err
	}

	// Quick inspect to derive a human-readable filename.
	filename := sanitizeFilename(id) + ".tar"
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer inspectCancel()
	info, _, inspectErr := cli.ImageInspectWithRaw(inspectCtx, id)
	if inspectErr == nil && len(info.RepoTags) > 0 {
		filename = sanitizeFilename(info.RepoTags[0]) + ".tar"
	}

	// ImageSave streams — no timeout; the request context governs lifetime.
	reader, err := cli.ImageSave(ctx, []string{id})
	if err != nil {
		return nil, "", fmt.Errorf("exporting image %s: %w", id, err)
	}

	return reader, filename, nil
}

// Import loads an image from a tar archive (docker load).
func (s *Service) Import(ctx context.Context, envID string, reader io.Reader) (string, error) {
	cli, err := s.getClient(envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := cli.ImageLoad(ctx, reader)
	if err != nil {
		return "", fmt.Errorf("importing image: %w", err)
	}
	defer resp.Body.Close()

	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading import response: %w", err)
	}

	return string(output), nil
}

// sanitizeFilename replaces characters unsafe for filenames.
func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "sha256:", "")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// hasTag checks if an image reference already contains a tag or digest.
func hasTag(ref string) bool {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == ':' || ref[i] == '@' {
			return true
		}
		if ref[i] == '/' {
			return false
		}
	}
	return false
}
