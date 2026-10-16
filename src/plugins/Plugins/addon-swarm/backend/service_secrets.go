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

// maxSecretBytes mirrors SwarmKit's own 500 KB limit. Rejecting here gives a
// readable error instead of a raw daemon rejection halfway through an upload.
const maxSecretBytes = 500 * 1024

// ListSecrets returns secret metadata plus which services consume each one.
//
// There is no payload in this response and no endpoint that returns one:
// Docker's secret API is write-only for the value, and McHarbor does not add
// a reveal affordance on top of it.
func (s *Service) ListSecrets(ctx context.Context, envID string) ([]Secret, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	secretsRes, err := cli.SecretList(ctx, client.SecretListOptions{})
	if err != nil {
		return nil, fmt.Errorf("secret list: %w", err)
	}
	secrets := secretsRes.Items

	usage := map[string]map[string]struct{}{}
	if services, sErr := cli.ServiceList(ctx, client.ServiceListOptions{}); sErr == nil {
		for _, svc := range services.Items {
			if svc.Spec.TaskTemplate.ContainerSpec == nil {
				continue
			}
			for _, ref := range svc.Spec.TaskTemplate.ContainerSpec.Secrets {
				if usage[ref.SecretID] == nil {
					usage[ref.SecretID] = map[string]struct{}{}
				}
				usage[ref.SecretID][svc.Spec.Name] = struct{}{}
			}
		}
	}

	out := make([]Secret, 0, len(secrets))
	for _, sec := range secrets {
		out = append(out, mapSecret(sec, sortedNames(usage[sec.ID])))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// InspectSecret returns one secret's metadata.
func (s *Service) InspectSecret(ctx context.Context, envID, id string) (Secret, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return Secret{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	secRes, err := cli.SecretInspect(ctx, id, client.SecretInspectOptions{})
	if err != nil {
		return Secret{}, fmt.Errorf("secret inspect %s: %w", id, err)
	}
	sec := secRes.Secret

	usage := map[string]struct{}{}
	if services, sErr := cli.ServiceList(ctx, client.ServiceListOptions{}); sErr == nil {
		for _, svc := range services.Items {
			if svc.Spec.TaskTemplate.ContainerSpec == nil {
				continue
			}
			for _, ref := range svc.Spec.TaskTemplate.ContainerSpec.Secrets {
				if ref.SecretID == sec.ID {
					usage[svc.Spec.Name] = struct{}{}
				}
			}
		}
	}

	return mapSecret(sec, sortedNames(usage)), nil
}

// CreateSecret stores a new secret.
//
// The plaintext is zeroed as soon as the daemon has it. Go will not do this
// for us — the backing array would otherwise sit in the heap until the GC
// happens to reuse it, readable in any core dump taken in between.
func (s *Service) CreateSecret(ctx context.Context, envID string, req PayloadWriteRequest) (string, error) {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return "", err
	}

	payload, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return "", fmt.Errorf("secret payload is not valid base64: %w", err)
	}
	defer zero(payload)

	if len(payload) == 0 {
		return "", fmt.Errorf("secret payload is empty")
	}
	if len(payload) > maxSecretBytes {
		return "", fmt.Errorf("secret payload is %d bytes, the maximum is %d", len(payload), maxSecretBytes)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	spec := swarm.SecretSpec{Data: payload}
	spec.Name = req.Name
	spec.Labels = req.Labels

	resp, err := cli.SecretCreate(ctx, client.SecretCreateOptions{Spec: spec})
	// Zero the copy inside the spec too — the SDK marshalled it, but the
	// slice header still points at our buffer.
	zero(spec.Data)
	if err != nil {
		return "", fmt.Errorf("secret create %s: %w", req.Name, err)
	}
	return resp.ID, nil
}

// UpdateSecretLabels updates the only mutable part of a secret.
//
// Docker rejects a SecretUpdate that changes anything else, so this path does
// not accept a payload at all: rotating a secret means creating a new one and
// pointing the services at it, which is also the only way to do it safely.
func (s *Service) UpdateSecretLabels(ctx context.Context, envID, id string, labels map[string]string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	secRes, err := cli.SecretInspect(ctx, id, client.SecretInspectOptions{})
	if err != nil {
		return fmt.Errorf("secret inspect %s: %w", id, err)
	}
	sec := secRes.Secret

	spec := sec.Spec
	if labels == nil {
		labels = map[string]string{}
	}
	spec.Labels = labels
	// Never echo a payload back: inspect returns none, and sending an empty
	// Data on update is what tells Docker to leave the stored value alone.
	spec.Data = nil

	if _, err := cli.SecretUpdate(ctx, sec.ID, client.SecretUpdateOptions{Version: sec.Version, Spec: spec}); err != nil {
		return classifyUpdateErr(fmt.Errorf("secret update %s: %w", id, err))
	}
	return nil
}

// RemoveSecret deletes a secret.
func (s *Service) RemoveSecret(ctx context.Context, envID, id string) error {
	cli, err := s.requireManager(ctx, envID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if _, err := cli.SecretRemove(ctx, id, client.SecretRemoveOptions{}); err != nil {
		return fmt.Errorf("secret remove %s: %w", id, err)
	}
	return nil
}

// zero overwrites a byte slice in place.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
