// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

// maxConfigBytes mirrors SwarmKit's 1000 KB config limit.
const maxConfigBytes = 1000 * 1024

// ListConfigs returns config metadata. The payload is deliberately absent:
// ConfigInspect does return it, but a list response that carried every
// config's contents would put the whole set into every browser cache and
// every proxy log for a page that only needs names.
func (s *Service) ListConfigs(ctx context.Context, envID string) ([]Config, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	configsRes, err := cli.ConfigList(ctx, client.ConfigListOptions{})
	if err != nil {
		return nil, fmt.Errorf("config list: %w", err)
	}
	configs := configsRes.Items

	usage := map[string]map[string]struct{}{}
	if services, sErr := cli.ServiceList(ctx, client.ServiceListOptions{}); sErr == nil {
		for _, svc := range services.Items {
			if svc.Spec.TaskTemplate.ContainerSpec == nil {
				continue
			}
			for _, ref := range svc.Spec.TaskTemplate.ContainerSpec.Configs {
				if usage[ref.ConfigID] == nil {
					usage[ref.ConfigID] = map[string]struct{}{}
				}
				usage[ref.ConfigID][svc.Spec.Name] = struct{}{}
			}
		}
	}

	out := make([]Config, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, mapConfig(cfg, sortedNames(usage[cfg.ID]), false))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// InspectConfig returns one config including its payload.
//
// This is the asymmetry with secrets, and it is intentional: Docker's config
// API is readable by design (configs are non-sensitive by definition), so the
// viewer dialog gets real content instead of a permanently empty panel. The
// route is still gated on swarm_configs.view.
func (s *Service) InspectConfig(ctx context.Context, envID, id string) (Config, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return Config{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfgRes, err := cli.ConfigInspect(ctx, id, client.ConfigInspectOptions{})
	if err != nil {
		return Config{}, fmt.Errorf("config inspect %s: %w", id, err)
	}
	cfg := cfgRes.Config

	usage := map[string]struct{}{}
	if services, sErr := cli.ServiceList(ctx, client.ServiceListOptions{}); sErr == nil {
		for _, svc := range services.Items {
			if svc.Spec.TaskTemplate.ContainerSpec == nil {
				continue
			}
			for _, ref := range svc.Spec.TaskTemplate.ContainerSpec.Configs {
				if ref.ConfigID == cfg.ID {
					usage[svc.Spec.Name] = struct{}{}
				}
			}
		}
	}

	return mapConfig(cfg, sortedNames(usage), true), nil
}

// CreateConfig stores a new config.
func (s *Service) CreateConfig(ctx context.Context, envID string, req PayloadWriteRequest) (string, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return "", err
	}

	payload, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return "", fmt.Errorf("config payload is not valid base64: %w", err)
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("config payload is empty")
	}
	if len(payload) > maxConfigBytes {
		return "", fmt.Errorf("config payload is %d bytes, the maximum is %d", len(payload), maxConfigBytes)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	spec := swarm.ConfigSpec{Data: payload}
	spec.Name = req.Name
	spec.Labels = req.Labels

	resp, err := cli.ConfigCreate(ctx, client.ConfigCreateOptions{Spec: spec})
	if err != nil {
		return "", fmt.Errorf("config create %s: %w", req.Name, err)
	}
	return resp.ID, nil
}

// UpdateConfigLabels updates a config's labels. As with secrets, the payload
// is immutable — rotating a config means creating a new one.
func (s *Service) UpdateConfigLabels(ctx context.Context, envID, id string, labels map[string]string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfgRes, err := cli.ConfigInspect(ctx, id, client.ConfigInspectOptions{})
	if err != nil {
		return fmt.Errorf("config inspect %s: %w", id, err)
	}
	cfg := cfgRes.Config

	spec := cfg.Spec
	if labels == nil {
		labels = map[string]string{}
	}
	spec.Labels = labels

	if _, err := cli.ConfigUpdate(ctx, cfg.ID, client.ConfigUpdateOptions{Version: cfg.Version, Spec: spec}); err != nil {
		return classifyUpdateErr(fmt.Errorf("config update %s: %w", id, err))
	}
	return nil
}

// RemoveConfig deletes a config.
func (s *Service) RemoveConfig(ctx context.Context, envID, id string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := cli.ConfigRemove(ctx, id, client.ConfigRemoveOptions{}); err != nil {
		return fmt.Errorf("config remove %s: %w", id, err)
	}
	return nil
}
