// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// Service wraps Docker SDK image operations.
type Service struct {
	pool *docker.ClientPool
}

// NewService creates a new image service.
func NewService(pool *docker.ClientPool) *Service {
	return &Service{pool: pool}
}

// getClient returns a Docker client for the given environment.
func (s *Service) getClient(ctx context.Context, envID string) (*client.Client, error) {
	c, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("docker connection failed: %w", err)
	}
	return c, nil
}

// List returns all images.
func (s *Service) List(ctx context.Context, envID string, all bool) ([]ImageSummary, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	imagesRes, err := cli.ImageList(ctx, client.ImageListOptions{All: all})
	images := imagesRes.Items
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	// One extra ContainerList per request, not per image: the Docker SDK's
	// image list carries a Containers count but not the names using each
	// image, so a single container pass builds that cross-reference for
	// every image at once.
	usedBy := make(map[string][]string)
	containersRes, cerr := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	containers := containersRes.Items
	if cerr != nil {
		// Best-effort: a failed cross-reference must not fail the image
		// list itself, but a silently empty UsedBy column is worth a
		// server-side trace when something is actually wrong.
		slog.Warn("images: listing containers for used-by cross-reference failed", "error", cerr, "envID", envID)
	} else {
		for _, c := range containers {
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			if name == "" || c.ImageID == "" {
				continue
			}
			usedBy[c.ImageID] = append(usedBy[c.ImageID], name)
		}
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
			UsedBy:      usedBy[img.ID],
		})
	}

	return result, nil
}

// Inspect returns detailed image information.
func (s *Service) Inspect(ctx context.Context, envID, id string) (image.InspectResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return image.InspectResponse{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	info, err := cli.ImageInspect(ctx, id)
	if err != nil {
		return image.InspectResponse{}, fmt.Errorf("inspecting image %s: %w", id, err)
	}

	return info.InspectResponse, nil
}

// Pull pulls an image from a registry.
func (s *Service) Pull(ctx context.Context, envID string, req PullRequest) (string, error) {
	cli, err := s.getClient(ctx, envID)
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

	reader, err := cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("pulling image %s: %w", ref, err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Debug("closing image stream failed", "error", err)
		}
	}()

	// Consume the pull output to completion
	output, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading pull response: %w", err)
	}

	return string(output), nil
}

// Remove removes an image.
func (s *Service) Remove(ctx context.Context, envID, id string, force, noPrune bool) ([]image.DeleteResponse, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	respRes, err := cli.ImageRemove(ctx, id, client.ImageRemoveOptions{
		Force:         force,
		PruneChildren: !noPrune,
	})
	if err != nil {
		return nil, fmt.Errorf("removing image %s: %w", id, err)
	}
	resp := respRes.Items

	return resp, nil
}

// Tag tags an image with a new repository and tag.
func (s *Service) Tag(ctx context.Context, envID, id string, req TagRequest) error {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	ref := req.Repo
	if req.Tag != "" {
		ref += ":" + req.Tag
	}

	if _, err := cli.ImageTag(ctx, client.ImageTagOptions{Source: id, Target: ref}); err != nil {
		return fmt.Errorf("tagging image %s as %s: %w", id, ref, err)
	}

	return nil
}

// History returns the history of an image.
func (s *Service) History(ctx context.Context, envID, id string) ([]image.HistoryResponseItem, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()

	historyRes, err := cli.ImageHistory(ctx, id)
	history := historyRes.Items
	if err != nil {
		return nil, fmt.Errorf("fetching image history for %s: %w", id, err)
	}

	return history, nil
}

// Prune removes unused images.
func (s *Service) Prune(ctx context.Context, envID string) (image.PruneReport, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return image.PruneReport{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	imagesRes, err := cli.ImageList(ctx, client.ImageListOptions{All: true})
	images := imagesRes.Items
	if err != nil {
		return image.PruneReport{}, fmt.Errorf("listing images for prune: %w", err)
	}

	report := image.PruneReport{}
	for _, img := range images {
		if img.Containers != 0 {
			continue
		}
		responsesRes, err := cli.ImageRemove(ctx, img.ID, client.ImageRemoveOptions{PruneChildren: true})
		if err != nil {
			return image.PruneReport{}, fmt.Errorf("pruning image %s: %w", img.ID, err)
		}
		responses := responsesRes.Items
		report.ImagesDeleted = append(report.ImagesDeleted, responses...)
		if img.Size > 0 {
			report.SpaceReclaimed += uint64(img.Size)
		}
	}
	return report, nil
}

// Export streams an image as a tar archive (docker save).
func (s *Service) Export(ctx context.Context, envID, id string) (io.ReadCloser, string, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return nil, "", err
	}

	// Quick inspect to derive a human-readable filename.
	filename := sanitizeFilename(id) + ".tar"
	inspectCtx, inspectCancel := context.WithTimeout(ctx, 10*time.Second)
	defer inspectCancel()
	info, inspectErr := cli.ImageInspect(inspectCtx, id)
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
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := cli.ImageLoad(ctx, reader)
	if err != nil {
		return "", fmt.Errorf("importing image: %w", err)
	}
	defer func() {
		if err := resp.Close(); err != nil {
			slog.Debug("closing image load response failed", "error", err)
		}
	}()

	output, err := io.ReadAll(resp)
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
